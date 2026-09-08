package product

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/http/middleware"
	"github.com/verdofanv/golang-be/pkg/response"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, jwtSecret string) {
	products := rg.Group("/products")
	products.Use(middleware.Auth(jwtSecret))
	{
		products.GET("", h.list)
		products.GET("/search", h.search) // before /:id so "search" never parses as an ID
		products.POST("", h.create)
		products.GET("/:id", h.getByID)
		products.PUT("/:id", h.update)
		products.DELETE("/:id", h.delete)
	}
}

type createRequest struct {
	Name        string  `json:"name" validate:"required"`
	Description string  `json:"description"`
	Price       float64 `json:"price" validate:"required"`
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
	if err := response.BindJSON(c, &req); err != nil {
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
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Created(c, "product created", product)
}

func (h *Handler) list(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		return
	}

	cursor, err := parseCursor(c.Query("cursor"))
	if err != nil {
		response.FailCode(c, http.StatusBadRequest, "invalid cursor", domain.ErrorCode(domain.ErrInvalid))
		return
	}
	limit := queryInt(c, "limit", 20)

	result, err := h.svc.List(c.Request.Context(), userID, cursor, limit)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OKWithMeta(c, "success", result.Items, response.PageMeta{
		Limit:      limit,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	})
}

// search is the Elasticsearch-backed full-text endpoint: GET /products/search?q=kopi&limit=20
func (h *Handler) search(c *gin.Context) {
	query := c.Query("q")
	limit := queryInt(c, "limit", 20)

	docs, err := h.svc.Search(c.Request.Context(), query, limit)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "success", docs)
}

func (h *Handler) getByID(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	product, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		mapErr(c, err)
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
	if err := response.BindJSON(c, &req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}

	product, err := h.svc.Update(c.Request.Context(), userID, id, UpdateInput(req))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "product updated", product)
}

func (h *Handler) delete(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		return
	}
	role, _ := middleware.UserRole(c)

	id, err := parseID(c.Param("id"))
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), userID, role, id); err != nil {
		mapErr(c, err)
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

func parseCursor(raw string) (uint, error) {
	if raw == "" {
		return 0, nil
	}
	return parseID(raw)
}

// queryInt reads an int query param, falling back to def when absent or malformed.
func queryInt(c *gin.Context, key string, def int) int {
	raw := c.Query(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

func mapErr(c *gin.Context, err error) {
	code := domain.ErrorCode(err)
	switch {
	case errors.Is(err, domain.ErrInvalid):
		response.FailCode(c, http.StatusBadRequest, err.Error(), code)
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, domain.ErrTokenExpired):
		response.FailCode(c, http.StatusUnauthorized, err.Error(), code)
	case errors.Is(err, domain.ErrForbidden):
		response.FailCode(c, http.StatusForbidden, err.Error(), code)
	case errors.Is(err, domain.ErrNotFound):
		response.FailCode(c, http.StatusNotFound, err.Error(), code)
	case errors.Is(err, domain.ErrUnavailable):
		response.FailCode(c, http.StatusServiceUnavailable, err.Error(), code)
	default:
		response.FailCode(c, http.StatusInternalServerError, "internal error", code)
	}
}
