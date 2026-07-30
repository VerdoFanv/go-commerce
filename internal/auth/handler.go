package auth

import (
	"errors"
	"net/http"

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
	authGroup := rg.Group("/authentication")
	{
		authGroup.Post("/register", h.register)
		authGroup.Post("/login", h.login)
		authGroup.Post("/refresh-token", h.refresh)
		authGroup.Get("/me", middleware.Auth(jwtSecret), h.me)
	}
}

type registerRequest struct {
	Name     string `json:"name" validate:"required"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken" validate:"required"`
}

func (h *Handler) register(c *fiber.Ctx) error {
	var req registerRequest
	if err := response.BindJSON(c, &req); err != nil {
		return response.Fail(c, http.StatusBadRequest, "invalid request body")
	}

	result, err := h.svc.Register(c.UserContext(), RegisterInput(req))
	if err != nil {
		return mapErr(c, err)
	}
	return response.Created(c, "registered", result)
}

func (h *Handler) login(c *fiber.Ctx) error {
	var req loginRequest
	if err := response.BindJSON(c, &req); err != nil {
		return response.Fail(c, http.StatusBadRequest, "invalid request body")
	}

	result, err := h.svc.Login(c.UserContext(), LoginInput(req))
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "login success", result)
}

func (h *Handler) refresh(c *fiber.Ctx) error {
	var req refreshRequest
	if err := response.BindJSON(c, &req); err != nil {
		return response.Fail(c, http.StatusBadRequest, "invalid request body")
	}

	tokens, err := h.svc.Refresh(c.UserContext(), req.RefreshToken)
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "token refreshed", tokens)
}

func (h *Handler) me(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return response.Fail(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
	}

	user, err := h.svc.Me(c.UserContext(), userID)
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "success", user)
}

func mapErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return response.Fail(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, domain.ErrTokenExpired):
		return response.Fail(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrEmailTaken):
		return response.Fail(c, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return response.Fail(c, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		return response.Fail(c, http.StatusForbidden, err.Error())
	default:
		return response.Fail(c, http.StatusInternalServerError, "internal error")
	}
}
