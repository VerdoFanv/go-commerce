package lab

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

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, jwtSecret string, enabled bool) {
	if !enabled {
		return
	}
	lab := rg.Group("/lab")
	lab.Use(middleware.Auth(jwtSecret), middleware.RequireRole(domain.RoleAdmin))
	{
		lab.GET("/overview", h.overview)

		lab.GET("/postgres/summary", h.postgresSummary)
		lab.GET("/postgres/samples", h.postgresSamples)

		lab.GET("/redis/product/:id", h.redisPeek)
		lab.DELETE("/redis/product/:id", h.redisInvalidate)

		lab.GET("/mongo/events", h.mongoEvents)

		lab.POST("/kafka/ping", h.kafkaPing)

		lab.GET("/typesense", h.typesenseExplore)
		lab.POST("/typesense/reindex", h.typesenseReindex)

		lab.GET("/commerce/failure-matrix", h.failureMatrix)
		lab.GET("/outbox/pending", h.outboxPending)
		lab.POST("/outbox/relay-once", h.outboxRelayOnce)
		lab.POST("/outbox/pause", h.outboxPause)
		lab.POST("/outbox/resume", h.outboxResume)
	}
}

func (h *Handler) overview(c *gin.Context) {
	data, err := h.svc.Overview(c.Request.Context())
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "lab overview", data)
}

func (h *Handler) postgresSummary(c *gin.Context) {
	data, err := h.svc.PostgresSummary(c.Request.Context())
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "postgres summary", data)
}

func (h *Handler) postgresSamples(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	data, err := h.svc.PostgresSamples(c.Request.Context(), limit)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "postgres samples", data)
}

func (h *Handler) redisPeek(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	data, err := h.svc.PeekProductCache(c.Request.Context(), id)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "redis cache peek", data)
}

func (h *Handler) redisInvalidate(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	data, err := h.svc.InvalidateProductCache(c.Request.Context(), id)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "redis cache invalidated", data)
}

func (h *Handler) mongoEvents(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	data, err := h.svc.MongoEvents(c.Request.Context(), limit)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "mongo audit events", data)
}

func (h *Handler) kafkaPing(c *gin.Context) {
	userID, ok := middleware.UserID(c)
	if !ok {
		response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		return
	}
	data, err := h.svc.KafkaPing(c.Request.Context(), userID)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.Created(c, "kafka ping published", data)
}

func (h *Handler) typesenseExplore(c *gin.Context) {
	data, err := h.svc.TypesenseExplore(c.Request.Context(), c.Query("q"))
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "typesense explore", data)
}

func (h *Handler) typesenseReindex(c *gin.Context) {
	data, err := h.svc.TypesenseReindex(c.Request.Context())
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "typesense reindex done", data)
}

func (h *Handler) failureMatrix(c *gin.Context) {
	response.OK(c, "commerce failure matrix", h.svc.FailureMatrix(c.Request.Context()))
}

func (h *Handler) outboxPending(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	data, err := h.svc.OutboxPending(c.Request.Context(), limit)
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "outbox pending", data)
}

func (h *Handler) outboxRelayOnce(c *gin.Context) {
	data, err := h.svc.OutboxRelayOnce(c.Request.Context())
	if err != nil {
		mapErr(c, err)
		return
	}
	response.OK(c, "outbox relay once", data)
}

func (h *Handler) outboxPause(c *gin.Context) {
	response.OK(c, "outbox relay paused", h.svc.OutboxSetPaused(true))
}

func (h *Handler) outboxResume(c *gin.Context) {
	response.OK(c, "outbox relay resumed", h.svc.OutboxSetPaused(false))
}

func parseID(c *gin.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	return uint(id), err
}

func mapErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrUnavailable):
		response.FailCode(c, http.StatusServiceUnavailable, err.Error(), domain.ErrorCode(err))
	case errors.Is(err, domain.ErrNotFound):
		response.FailCode(c, http.StatusNotFound, err.Error(), domain.ErrorCode(err))
	default:
		response.Fail(c, http.StatusInternalServerError, "internal error")
	}
}
