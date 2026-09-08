package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/http/middleware"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo Repository
	cfg  config.Config
}

func NewService(repo Repository, cfg config.Config) *Service {
	return &Service{repo: repo, cfg: cfg}
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
	if in.Name == "" || in.Email == "" || len(in.Password) < 6 {
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

	tokens, err := s.issueTokens(model.ID, model.Role)
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

	tokens, err := s.issueTokens(model.ID, model.Role)
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

	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(refreshToken, claims, func(t *jwt.Token) (any, error) {
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		if err != nil && strings.Contains(err.Error(), "token is expired") {
			return nil, domain.ErrTokenExpired
		}
		return nil, domain.ErrUnauthorized
	}
	if claims.Type != "refresh" {
		return nil, domain.ErrUnauthorized
	}

	user, err := s.repo.FindByID(ctx, claims.UserID)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	tokens, err := s.issueTokens(user.ID, user.Role)
	if err != nil {
		return nil, err
	}
	return &tokens, nil
}

func (s *Service) Me(ctx context.Context, userID uint) (*domain.User, error) {
	model, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	u := toDomain(model)
	return &u, nil
}

func (s *Service) issueTokens(userID uint, role string) (domain.AuthTokens, error) {
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

	refreshClaims := middleware.Claims{
		UserID: userID,
		Role:   role,
		Type:   "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.JWTRefreshTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	refresh, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return domain.AuthTokens{}, err
	}

	return domain.AuthTokens{
		AccessToken:  access,
		RefreshToken: refresh,
	}, nil
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
