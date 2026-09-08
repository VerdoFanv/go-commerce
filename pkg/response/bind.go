package response

import (
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// BindJSON parses the request body and validates struct tags (`validate`).
func BindJSON(c *gin.Context, dest any) error {
	if err := c.ShouldBindJSON(dest); err != nil {
		return err
	}
	return validate.Struct(dest)
}
