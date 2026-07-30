package response

import (
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

var validate = validator.New()

// BindJSON parses the request body and validates struct tags (`validate`).
func BindJSON(c *fiber.Ctx, dest any) error {
	if err := c.BodyParser(dest); err != nil {
		return err
	}
	return validate.Struct(dest)
}
