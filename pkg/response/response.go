package response

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
)

// Envelope matches Wisteria mobile ApiResponse shape.
type Envelope struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Errors  any    `json:"errors,omitempty"`
}

func Success(c *fiber.Ctx, status int, message string, data any) error {
	return c.Status(status).JSON(Envelope{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func Fail(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(Envelope{
		Success: false,
		Message: message,
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

func Created(c *fiber.Ctx, message string, data any) error {
	return Success(c, http.StatusCreated, message, data)
}
