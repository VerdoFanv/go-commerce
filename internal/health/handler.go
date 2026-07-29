package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/verdofanv/golang-be/pkg/response"
	"gorm.io/gorm"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", h.check)
}

func (h *Handler) check(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil {
		response.Fail(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	if err := sqlDB.Ping(); err != nil {
		response.Fail(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	response.OK(c, "ok", gin.H{
		"status": "healthy",
	})
}
