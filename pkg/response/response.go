package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Envelope matches Wisteria mobile ApiResponse shape.
type Envelope struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Errors  any    `json:"errors,omitempty"`
}

func Success(c *gin.Context, status int, message string, data any) {
	c.JSON(status, Envelope{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func Fail(c *gin.Context, status int, message string) {
	c.JSON(status, Envelope{
		Success: false,
		Message: message,
	})
}

func FailWithErrors(c *gin.Context, status int, message string, errors any) {
	c.JSON(status, Envelope{
		Success: false,
		Message: message,
		Errors:  errors,
	})
}

func OK(c *gin.Context, message string, data any) {
	Success(c, http.StatusOK, message, data)
}

func Created(c *gin.Context, message string, data any) {
	Success(c, http.StatusCreated, message, data)
}
