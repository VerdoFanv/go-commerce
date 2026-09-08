package wishlist

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
	w := rg.Group("/wishlists")
	w.Use(middleware.Auth(jwtSecret))
	{
		w.GET("", h.list)
		w.GET("/count", h.count)
		w.POST("", h.add)
		w.DELETE("/:id", h.remove)
	}
}

type addRequest struct {
	ProductID uint   `json:"productId"`
	Note      string `json:"note"`
}

func (h *Handler) add(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		return
	}
	var req addRequest
	if err := response.BindJSON(c, &req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	item, err := h.svc.Add(c.Request.Context(), AddInput{
		UserID:    userID,
		ProductID: req.ProductID,
		Note:      req.Note,
	})
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Created(c, "added to wishlist", item)
}

func (h *Handler) list(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		return
	}
	items, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "success", items)
}

func (h *Handler) count(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		return
	}
	n, err := h.svc.Count(c.Request.Context(), userID)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "success", gin.H{"count": n})
}

func (h *Handler) remove(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Remove(c.Request.Context(), userID, uint(id)); err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "removed from wishlist", nil)
}

func mapErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		response.FailCode(c, http.StatusNotFound, err.Error(), domain.ErrorCode(err))
	case errors.Is(err, domain.ErrInvalid):
		response.FailCode(c, http.StatusBadRequest, err.Error(), domain.ErrorCode(err))
	case errors.Is(err, domain.ErrConflict):
		response.FailCode(c, http.StatusConflict, "product already in wishlist", domain.ErrorCode(err))
	default:
		response.Fail(c, http.StatusInternalServerError, "internal error")
	}
}
