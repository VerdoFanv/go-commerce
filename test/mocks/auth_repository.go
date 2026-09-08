package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/verdofanv/golang-be/internal/http/auth"
	"github.com/verdofanv/golang-be/internal/domain"
)

// AuthRepository is an in-memory auth.Repository for unit/integration tests.
type AuthRepository struct {
	mu           sync.RWMutex
	usersByEmail map[string]*auth.UserModel
	usersByID    map[uint]*auth.UserModel
	nextID       uint
	CreateErr    error
}

func NewAuthRepository() *AuthRepository {
	return &AuthRepository{
		usersByEmail: make(map[string]*auth.UserModel),
		usersByID:    make(map[uint]*auth.UserModel),
		nextID:       1,
	}
}

func (m *AuthRepository) Create(_ context.Context, user *auth.UserModel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.CreateErr != nil {
		return m.CreateErr
	}
	if _, ok := m.usersByEmail[user.Email]; ok {
		return domain.ErrEmailTaken
	}

	user.ID = m.nextID
	m.nextID++
	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	cp := *user
	m.usersByEmail[user.Email] = &cp
	m.usersByID[user.ID] = &cp
	return nil
}

func (m *AuthRepository) FindByEmail(_ context.Context, email string) (*auth.UserModel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	u, ok := m.usersByEmail[email]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (m *AuthRepository) FindByID(_ context.Context, id uint) (*auth.UserModel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	u, ok := m.usersByID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *u
	return &cp, nil
}
