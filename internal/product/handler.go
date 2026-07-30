package product

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/middleware"
	"github.com/verdofanv/golang-be/pkg/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, jwtSecret string) {
	products := rg.Group("/products", middleware.Auth(jwtSecret))
	{
		products.GET("", h.list)
		products.POST("", h.create)
		products.GET("/:id", h.getByID)
		products.PUT("/:id", h.update)
		products.DELETE("/:id", h.delete)
	}
}

type createRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description string  `json:"description"`
	Price       float64 `json:"price" binding:"required"`
	Stock       int     `json:"stock"`
}

type updateRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Price       *float64 `json:"price"`
	Stock       *int     `json:"stock"`
}

func (h *Handler) create(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Fail(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
		return
	}

	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}

	product, err := h.svc.Create(c.Request.Context(), CreateInput{
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		Stock:       req.Stock,
	})
	if mapErr(c, err) {
		return
	}
	response.Created(c, "product created", product)
}

func (h *Handler) list(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Fail(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
		return
	}

	products, err := h.svc.List(c.Request.Context(), userID)
	if mapErr(c, err) {
		return
	}
	response.OK(c, "success", products)
}

func (h *Handler) getByID(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	product, err := h.svc.GetByID(c.Request.Context(), id)
	if mapErr(c, err) {
		return
	}
	response.OK(c, "success", product)
}

func (h *Handler) update(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Fail(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
		return
	}

	id, err := parseID(c.Param("id"))
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	var req updateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}

	product, err := h.svc.Update(c.Request.Context(), userID, id, UpdateInput(req))
	if mapErr(c, err) {
		return
	}
	response.OK(c, "product updated", product)
}

func (h *Handler) delete(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Fail(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
		return
	}

	id, err := parseID(c.Param("id"))
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), userID, id); mapErr(c, err) {
		return
	}
	response.OK(c, "product deleted", nil)
}

func parseID(raw string) (uint, error) {
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 {
		return 0, domain.ErrInvalid
	}
	return uint(n), nil
}

func mapErr(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, domain.ErrInvalid):
		response.Fail(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, domain.ErrTokenExpired):
		response.Fail(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		response.Fail(c, http.StatusForbidden, err.Error())
	case errors.Is(err, domain.ErrNotFound):
		response.Fail(c, http.StatusNotFound, err.Error())
	default:
		response.Fail(c, http.StatusInternalServerError, "internal error")
	}
	return true
}
