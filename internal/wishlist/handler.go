package wishlist

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
	w := rg.Group("/wishlists", middleware.Auth(jwtSecret))
	{
		w.Get("", h.list)
		w.Get("/count", h.count)
		w.Post("", h.add)
		w.Delete("/:id", h.remove)
	}
}

type addRequest struct {
	ProductID uint   `json:"productId"`
	Note      string `json:"note"`
}

func (h *Handler) add(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
	}
	var req addRequest
	if err := response.BindJSON(c, &req); err != nil {
		return response.Fail(c, http.StatusBadRequest, "invalid request body")
	}
	item, err := h.svc.Add(c.UserContext(), AddInput{
		UserID:    userID,
		ProductID: req.ProductID,
		Note:      req.Note,
	})
	if err != nil {
		return mapErr(c, err)
	}
	return response.Created(c, "added to wishlist", item)
}

func (h *Handler) list(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
	}
	items, err := h.svc.List(c.UserContext(), userID)
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "success", items)
}

func (h *Handler) count(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
	}
	n, err := h.svc.Count(c.UserContext(), userID)
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "success", fiber.Map{"count": n})
}

func (h *Handler) remove(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return response.Fail(c, http.StatusBadRequest, "invalid id")
	}
	if err := h.svc.Remove(c.UserContext(), userID, uint(id)); err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "removed from wishlist", nil)
}

func mapErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return response.FailCode(c, http.StatusNotFound, err.Error(), domain.ErrorCode(err))
	case errors.Is(err, domain.ErrInvalid):
		return response.FailCode(c, http.StatusBadRequest, err.Error(), domain.ErrorCode(err))
	case errors.Is(err, domain.ErrConflict):
		return response.FailCode(c, http.StatusConflict, "product already in wishlist", domain.ErrorCode(err))
	default:
		return response.Fail(c, http.StatusInternalServerError, "internal error")
	}
}
