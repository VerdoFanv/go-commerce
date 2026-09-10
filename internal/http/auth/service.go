package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/http/middleware"
	appredis "github.com/verdofanv/golang-be/internal/platform/redis"
	"golang.org/x/crypto/bcrypt"
)

const minPasswordLen = 8

type Service struct {
	repo  Repository
	cfg   config.Config
	cache *appredis.Client
}

func NewService(repo Repository, cfg config.Config, cache *appredis.Client) *Service {
	return &Service{repo: repo, cfg: cfg, cache: cache}
}

type RegisterInput struct {
	Name     string
	Email    string
	Password string
}

type LoginInput struct {
	Email    string
	Password string
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*domain.AuthResult, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Name == "" || in.Email == "" || len(in.Password) < minPasswordLen {
		return nil, domain.ErrInvalid
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), s.cfg.BcryptCost)
	if err != nil {
		return nil, err
	}

	model := &UserModel{
		Name:         in.Name,
		Email:        in.Email,
		PasswordHash: string(hash),
		Role:         domain.RoleUser,
	}
	if err := s.repo.Create(ctx, model); err != nil {
		return nil, err
	}

	tokens, err := s.issueTokens(ctx, model.ID, model.Role)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResult{
		User:   toDomain(model),
		Tokens: tokens,
	}, nil
}

func (s *Service) Login(ctx context.Context, in LoginInput) (*domain.AuthResult, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Email == "" || in.Password == "" {
		return nil, domain.ErrInvalid
	}

	model, err := s.repo.FindByEmail(ctx, in.Email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}

	if bcrypt.CompareHashAndPassword([]byte(model.PasswordHash), []byte(in.Password)) != nil {
		return nil, domain.ErrUnauthorized
	}

	tokens, err := s.issueTokens(ctx, model.ID, model.Role)
	if err != nil {
		return nil, err
	}

	return &domain.AuthResult{
		User:   toDomain(model),
		Tokens: tokens,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*domain.AuthTokens, error) {
	if refreshToken == "" {
		return nil, domain.ErrInvalid
	}

	claims, err := s.parseRefresh(refreshToken)
	if err != nil {
		return nil, err
	}
	if err := s.consumeRefreshJTI(ctx, claims.ID); err != nil {
		return nil, err
	}

	user, err := s.repo.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	tokens, err := s.issueTokens(ctx, user.ID, user.Role)
	if err != nil {
		return nil, err
	}
	return &tokens, nil
}

// Logout revokes the presented refresh token (rotation store delete).
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return domain.ErrInvalid
	}
	claims, err := s.parseRefresh(refreshToken)
	if err != nil {
		return err
	}
	return s.revokeRefreshJTI(ctx, claims.ID)
}

func (s *Service) Me(ctx context.Context, userID uint) (*domain.User, error) {
	model, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	u := toDomain(model)
	return &u, nil
}

func (s *Service) parseRefresh(refreshToken string) (*middleware.Claims, error) {
	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(refreshToken, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, domain.ErrUnauthorized
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil || token == nil || !token.Valid {
		if err != nil && strings.Contains(err.Error(), "token is expired") {
			return nil, domain.ErrTokenExpired
		}
		return nil, domain.ErrUnauthorized
	}
	if claims.Type != "refresh" {
		return nil, domain.ErrUnauthorized
	}
	return claims, nil
}

func (s *Service) issueTokens(ctx context.Context, userID uint, role string) (domain.AuthTokens, error) {
	now := time.Now()

	accessClaims := middleware.Claims{
		UserID: userID,
		Role:   role,
		Type:   "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.JWTAccessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return domain.AuthTokens{}, err
	}

	jti := uuid.NewString()
	refreshClaims := middleware.Claims{
		UserID: userID,
		Role:   role,
		Type:   "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.JWTRefreshTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	refresh, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return domain.AuthTokens{}, err
	}

	if err := s.storeRefreshJTI(ctx, jti, userID); err != nil {
		return domain.AuthTokens{}, err
	}

	return domain.AuthTokens{
		AccessToken:  access,
		RefreshToken: refresh,
	}, nil
}

func refreshKey(jti string) string {
	return "refresh:" + jti
}

func (s *Service) storeRefreshJTI(ctx context.Context, jti string, userID uint) error {
	if s.cache == nil || jti == "" {
		return nil
	}
	return s.cache.Set(ctx, refreshKey(jti), fmt.Sprintf("%d", userID), s.cfg.JWTRefreshTTL)
}

func (s *Service) consumeRefreshJTI(ctx context.Context, jti string) error {
	if s.cache == nil {
		return nil
	}
	if jti == "" {
		return domain.ErrUnauthorized
	}
	key := refreshKey(jti)
	n, err := s.cache.Raw().Exists(ctx, key).Result()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrUnauthorized
	}
	return s.cache.Del(ctx, key)
}

func (s *Service) revokeRefreshJTI(ctx context.Context, jti string) error {
	if s.cache == nil || jti == "" {
		return nil
	}
	return s.cache.Del(ctx, refreshKey(jti))
}

func toDomain(m *UserModel) domain.User {
	return domain.User{
		ID:        m.ID,
		Name:      m.Name,
		Email:     m.Email,
		Role:      m.Role,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
