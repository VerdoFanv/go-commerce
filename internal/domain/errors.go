package domain

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrInvalid      = errors.New("invalid input")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrEmailTaken   = errors.New("email already registered")
	// Message intentionally matches Wisteria mobile interceptor contract.
	ErrTokenExpired  = errors.New("Unauthorized: Token expired") //nolint:staticcheck // ST1005: deliberate API contract string
	ErrInvalidAPIKey = errors.New("invalid api key")
)
