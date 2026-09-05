package domain

import "time"

// RBAC roles. Keep them as plain strings so they survive JSON, JWT claims, and SQL.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

type User struct {
	ID           uint      `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type AuthTokens struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

type AuthResult struct {
	User   User       `json:"user"`
	Tokens AuthTokens `json:"tokens"`
}
