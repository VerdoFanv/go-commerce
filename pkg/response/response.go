package response

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
)

// Envelope matches Wisteria mobile ApiResponse shape, extended with a stable
// machine-readable errorCode and a meta block for pagination cursors.
type Envelope struct {
	Success   bool   `json:"success"`
	Message   string `json:"message,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
	Data      any    `json:"data,omitempty"`
	Meta      any    `json:"meta,omitempty"`
	Errors    any    `json:"errors,omitempty"`
}

// PageMeta accompanies cursor-paginated list responses.
type PageMeta struct {
	Limit      int  `json:"limit"`
	NextCursor uint `json:"nextCursor,omitempty"`
	HasMore    bool `json:"hasMore"`
}

func Success(c *fiber.Ctx, status int, message string, data any) error {
	return c.Status(status).JSON(Envelope{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// SuccessWithMeta is Success plus a pagination metadata block.
func SuccessWithMeta(c *fiber.Ctx, status int, message string, data any, meta any) error {
	return c.Status(status).JSON(Envelope{
		Success: true,
		Message: message,
		Data:    data,
		Meta:    meta,
	})
}

// Fail keeps the original signature for simple call sites; prefer FailCode in handlers.
func Fail(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(Envelope{
		Success: false,
		Message: message,
	})
}

// FailCode attaches a stable machine code so clients can branch on it.
func FailCode(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(Envelope{
		Success:   false,
		Message:   message,
		ErrorCode: code,
	})
}

func FailWithErrors(c *fiber.Ctx, status int, message string, errors any) error {
	return c.Status(status).JSON(Envelope{
		Success: false,
		Message: message,
		Errors:  errors,
	})
}

func OK(c *fiber.Ctx, message string, data any) error {
	return Success(c, http.StatusOK, message, data)
}

func OKWithMeta(c *fiber.Ctx, message string, data any, meta any) error {
	return SuccessWithMeta(c, http.StatusOK, message, data, meta)
}

func Created(c *fiber.Ctx, message string, data any) error {
	return Success(c, http.StatusCreated, message, data)
}
