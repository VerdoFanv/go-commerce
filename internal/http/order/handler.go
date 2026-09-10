package order

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

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
	o := rg.Group("/orders")
	o.Use(middleware.Auth(jwtSecret))
	{
		o.POST("", h.create)
		o.GET("", h.list)
		o.GET("/:id", h.getByID)
		o.POST("/:id/cancel", h.cancel)
		o.POST("/:id/pay", h.pay)
		o.POST("/:id/fulfill", middleware.RequireRole(domain.RoleAdmin), h.fulfill)
	}
}

type createRequest struct {
	Items []CreateItemInput `json:"items" validate:"required,min=1,dive"`
}

type payRequest struct {
	Outcome string `json:"outcome"` // success | fail | timeout
	Key     string `json:"key"`
}

func (h *Handler) create(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		return
	}
	var req createRequest
	if err := response.BindJSON(c, &req); err != nil {
		response.FailCode(c, http.StatusBadRequest, "invalid request body", domain.ErrorCode(domain.ErrInvalid))
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	order, replayStatus, replayBody, err := h.svc.Create(c.Request.Context(), CreateInput{
		UserID:         userID,
		Items:          req.Items,
		IdempotencyKey: key,
		Method:         c.Request.Method,
		Path:           c.FullPath(),
	})
	if err != nil {
		mapErr(c, err)
		return
	}
	if replayBody != nil {
		c.Data(replayStatus, "application/json", replayBody)
		return
	}

	env := response.Envelope{Success: true, Message: "order created", Data: order}
	body, err := json.Marshal(env)
	if err != nil {
		mapErr(c, err)
		return
	}
	_ = h.svc.RememberIdempotent(c.Request.Context(), userID, key, c.Request.Method, c.FullPath(), req.Items, http.StatusCreated, body)
	c.Data(http.StatusCreated, "application/json", body)
}

func (h *Handler) list(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	items, err := h.svc.List(c.Request.Context(), userID, limit)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "success", items)
}

func (h *Handler) getByID(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		response.FailCode(c, http.StatusBadRequest, "invalid id", domain.ErrorCode(domain.ErrInvalid))
		return
	}
	order, err := h.svc.Get(c.Request.Context(), userID, id)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "success", order)
}

func (h *Handler) cancel(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		response.FailCode(c, http.StatusBadRequest, "invalid id", domain.ErrorCode(domain.ErrInvalid))
		return
	}
	order, err := h.svc.Cancel(c.Request.Context(), userID, id)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "order cancelled", order)
}

func (h *Handler) pay(c *gin.Context) {
	// Manual pay is a *lab override* for teaching failure modes (fail/timeout).
	// Canonical production-like path: worker auto-charges on order.created (see worker/payment).
	userID, ok := middleware.UserID(c)
	if !ok {
		response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		response.FailCode(c, http.StatusBadRequest, "invalid id", domain.ErrorCode(domain.ErrInvalid))
		return
	}
	var req payRequest
	_ = c.ShouldBindJSON(&req)
	order, pay, err := h.svc.Pay(c.Request.Context(), PayInput{
		UserID:  userID,
		OrderID: id,
		Outcome: req.Outcome,
		Key:     req.Key,
	})
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "payment processed", gin.H{"order": order, "payment": pay})
}

func (h *Handler) fulfill(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		response.FailCode(c, http.StatusBadRequest, "invalid id", domain.ErrorCode(domain.ErrInvalid))
		return
	}
	order, err := h.svc.Fulfill(c.Request.Context(), id)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "order fulfilled", order)
}

func parseID(raw string) (uint, error) {
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 {
		return 0, domain.ErrInvalid
	}
	return uint(n), nil
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
	case errors.Is(err, domain.ErrConflict):
		response.FailCode(c, http.StatusConflict, err.Error(), code)
	case errors.Is(err, domain.ErrUnavailable):
		response.FailCode(c, http.StatusServiceUnavailable, err.Error(), code)
	default:
		response.FailCode(c, http.StatusInternalServerError, "internal error", code)
	}
}
