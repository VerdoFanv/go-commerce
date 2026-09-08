package domain

import "errors"

// Sentinel errors. Every one maps to a stable machine-readable code via ErrorCode,
// so API consumers can switch on `errorCode` instead of parsing message strings.
var (
	ErrNotFound     = errors.New("not found")
	ErrInvalid      = errors.New("invalid input")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrEmailTaken   = errors.New("email already registered")
	ErrConflict     = errors.New("conflict")
	ErrRateLimited  = errors.New("rate limit exceeded")
	ErrUnavailable  = errors.New("service unavailable")
	// Message intentionally matches Wisteria mobile interceptor contract.
	ErrTokenExpired  = errors.New("Unauthorized: Token expired") //nolint:staticcheck // ST1005: deliberate API contract string
	ErrInvalidAPIKey = errors.New("invalid api key")
)

// ErrorCode maps a domain error to a stable, SCREAMING_SNAKE machine code.
func ErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "RESOURCE_NOT_FOUND"
	case errors.Is(err, ErrInvalid):
		return "VALIDATION_ERROR"
	case errors.Is(err, ErrEmailTaken):
		return "AUTH_EMAIL_TAKEN"
	case errors.Is(err, ErrConflict):
		return "RESOURCE_CONFLICT"
	case errors.Is(err, ErrTokenExpired):
		return "AUTH_TOKEN_EXPIRED"
	case errors.Is(err, ErrUnauthorized):
		return "AUTH_UNAUTHORIZED"
	case errors.Is(err, ErrInvalidAPIKey):
		return "AUTH_INVALID_API_KEY"
	case errors.Is(err, ErrForbidden):
		return "AUTH_FORBIDDEN"
	case errors.Is(err, ErrRateLimited):
		return "RATE_LIMIT_EXCEEDED"
	case errors.Is(err, ErrUnavailable):
		return "SERVICE_UNAVAILABLE"
	default:
		return "INTERNAL_ERROR"
	}
}
