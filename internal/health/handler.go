package health

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/verdofanv/golang-be/pkg/response"
	"gorm.io/gorm"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) RegisterRoutes(rg fiber.Router) {
	rg.Get("/health", h.check)
}

func (h *Handler) check(c *fiber.Ctx) error {
	sqlDB, err := h.db.DB()
	if err != nil {
		return response.Fail(c, http.StatusServiceUnavailable, "database unavailable")
	}
	if err := sqlDB.Ping(); err != nil {
		return response.Fail(c, http.StatusServiceUnavailable, "database unavailable")
	}

	return response.OK(c, "ok", fiber.Map{
		"status": "healthy",
	})
}
