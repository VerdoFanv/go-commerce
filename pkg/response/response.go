package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
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

func Success(c *gin.Context, status int, message string, data any) {
	c.JSON(status, Envelope{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func SuccessWithMeta(c *gin.Context, status int, message string, data any, meta any) {
	c.JSON(status, Envelope{
		Success: true,
		Message: message,
		Data:    data,
		Meta:    meta,
	})
}

func Fail(c *gin.Context, status int, message string) {
	c.JSON(status, Envelope{
		Success: false,
		Message: message,
	})
}

func FailCode(c *gin.Context, status int, message, code string) {
	c.JSON(status, Envelope{
		Success:   false,
		Message:   message,
		ErrorCode: code,
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

func OKWithMeta(c *gin.Context, message string, data any, meta any) {
	SuccessWithMeta(c, http.StatusOK, message, data, meta)
}

func Created(c *gin.Context, message string, data any) {
	Success(c, http.StatusCreated, message, data)
}
