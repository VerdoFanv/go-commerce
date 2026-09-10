package auth

import (
	"errors"
	"net/http"

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
	authGroup := rg.Group("/authentication")
	{
		authGroup.POST("/register", h.register)
		authGroup.POST("/login", h.login)
		authGroup.POST("/refresh-token", h.refresh)
		authGroup.POST("/logout", h.logout)
		authGroup.GET("/me", middleware.Auth(jwtSecret), h.me)
	}
}

type registerRequest struct {
	Name     string `json:"name" validate:"required"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken" validate:"required"`
}

func (h *Handler) register(c *gin.Context) {
	var req registerRequest
	if err := response.BindJSON(c, &req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.svc.Register(c.Request.Context(), RegisterInput(req))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Created(c, "registered", result)
}

func (h *Handler) login(c *gin.Context) {
	var req loginRequest
	if err := response.BindJSON(c, &req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.svc.Login(c.Request.Context(), LoginInput(req))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "login success", result)
}

func (h *Handler) refresh(c *gin.Context) {
	var req refreshRequest
	if err := response.BindJSON(c, &req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}

	tokens, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "token refreshed", tokens)
}

func (h *Handler) logout(c *gin.Context) {
	var req refreshRequest
	if err := response.BindJSON(c, &req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.svc.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "logged out", nil)
}

func (h *Handler) me(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.Fail(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
		return
	}

	user, err := h.svc.Me(c.Request.Context(), userID)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "success", user)
}

func mapErr(c *gin.Context, err error) {
	code := domain.ErrorCode(err)
	switch {
	case errors.Is(err, domain.ErrInvalid):
		response.FailCode(c, http.StatusBadRequest, err.Error(), code)
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, domain.ErrTokenExpired):
		response.FailCode(c, http.StatusUnauthorized, err.Error(), code)
	case errors.Is(err, domain.ErrEmailTaken):
		response.FailCode(c, http.StatusConflict, err.Error(), code)
	case errors.Is(err, domain.ErrNotFound):
		response.FailCode(c, http.StatusNotFound, err.Error(), code)
	case errors.Is(err, domain.ErrForbidden):
		response.FailCode(c, http.StatusForbidden, err.Error(), code)
	default:
		response.FailCode(c, http.StatusInternalServerError, "internal error", code)
	}
}
