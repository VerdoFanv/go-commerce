package product

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"
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

func (h *Handler) RegisterRoutes(rg fiber.Router, jwtSecret string) {
	products := rg.Group("/products", middleware.Auth(jwtSecret))
	{
		products.Get("", h.list)
		products.Get("/search", h.search) // before /:id so "search" never parses as an ID
		products.Post("", h.create)
		products.Get("/:id", h.getByID)
		products.Put("/:id", h.update)
		products.Delete("/:id", h.delete)
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

func (h *Handler) create(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return response.Fail(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
	}

	var req createRequest
	if err := response.BindJSON(c, &req); err != nil {
		return response.Fail(c, http.StatusBadRequest, "invalid request body")
	}

	product, err := h.svc.Create(c.UserContext(), CreateInput{
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		Stock:       req.Stock,
	})
	if err != nil {
		return mapErr(c, err)
	}
	return response.Created(c, "product created", product)
}

func (h *Handler) list(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
	}

	cursor, err := parseCursor(c.Query("cursor"))
	if err != nil {
		return response.FailCode(c, http.StatusBadRequest, "invalid cursor", domain.ErrorCode(domain.ErrInvalid))
	}
	limit := c.QueryInt("limit", 20)

	result, err := h.svc.List(c.UserContext(), userID, cursor, limit)
	if err != nil {
		return mapErr(c, err)
	}
	return response.OKWithMeta(c, "success", result.Items, response.PageMeta{
		Limit:      limit,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	})
}

// search is the Elasticsearch-backed full-text endpoint: GET /products/search?q=kopi&limit=20
func (h *Handler) search(c *fiber.Ctx) error {
	query := c.Query("q")
	limit := c.QueryInt("limit", 20)

	docs, err := h.svc.Search(c.UserContext(), query, limit)
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "success", docs)
}

func (h *Handler) getByID(c *fiber.Ctx) error {
	id, err := parseID(c.Params("id"))
	if err != nil {
		return response.Fail(c, http.StatusBadRequest, "invalid id")
	}

	product, err := h.svc.GetByID(c.UserContext(), id)
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "success", product)
}

func (h *Handler) update(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return response.Fail(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
	}

	id, err := parseID(c.Params("id"))
	if err != nil {
		return response.Fail(c, http.StatusBadRequest, "invalid id")
	}

	var req updateRequest
	if err := response.BindJSON(c, &req); err != nil {
		return response.Fail(c, http.StatusBadRequest, "invalid request body")
	}

	product, err := h.svc.Update(c.UserContext(), userID, id, UpdateInput(req))
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "product updated", product)
}

func (h *Handler) delete(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
	}
	role, _ := middleware.UserRole(c)

	id, err := parseID(c.Params("id"))
	if err != nil {
		return response.Fail(c, http.StatusBadRequest, "invalid id")
	}

	if err := h.svc.Delete(c.UserContext(), userID, role, id); err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "product deleted", nil)
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

func mapErr(c *fiber.Ctx, err error) error {
	code := domain.ErrorCode(err)
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return response.FailCode(c, http.StatusBadRequest, err.Error(), code)
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, domain.ErrTokenExpired):
		return response.FailCode(c, http.StatusUnauthorized, err.Error(), code)
	case errors.Is(err, domain.ErrForbidden):
		return response.FailCode(c, http.StatusForbidden, err.Error(), code)
	case errors.Is(err, domain.ErrNotFound):
		return response.FailCode(c, http.StatusNotFound, err.Error(), code)
	case errors.Is(err, domain.ErrUnavailable):
		return response.FailCode(c, http.StatusServiceUnavailable, err.Error(), code)
	default:
		return response.FailCode(c, http.StatusInternalServerError, "internal error", code)
	}
}
