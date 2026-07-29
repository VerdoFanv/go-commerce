package domain

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrInvalid       = errors.New("invalid input")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrEmailTaken    = errors.New("email already registered")
	ErrTokenExpired  = errors.New("Unauthorized: Token expired")
	ErrInvalidAPIKey = errors.New("invalid api key")
)
