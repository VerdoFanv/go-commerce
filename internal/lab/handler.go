package lab

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
	lab := rg.Group("/lab", middleware.Auth(jwtSecret))
	{
		lab.Get("/overview", h.overview)

		lab.Get("/postgres/summary", h.postgresSummary)
		lab.Get("/postgres/samples", h.postgresSamples)

		lab.Get("/redis/product/:id", h.redisPeek)
		lab.Delete("/redis/product/:id", h.redisInvalidate)

		lab.Get("/mongo/events", h.mongoEvents)

		lab.Post("/kafka/ping", h.kafkaPing)

		lab.Get("/typesense", h.typesenseExplore)
		lab.Post("/typesense/reindex", h.typesenseReindex)
	}
}

func (h *Handler) overview(c *fiber.Ctx) error {
	data, err := h.svc.Overview(c.UserContext())
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "lab overview", data)
}

func (h *Handler) postgresSummary(c *fiber.Ctx) error {
	data, err := h.svc.PostgresSummary(c.UserContext())
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "postgres summary", data)
}

func (h *Handler) postgresSamples(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	data, err := h.svc.PostgresSamples(c.UserContext(), limit)
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "postgres samples", data)
}

func (h *Handler) redisPeek(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return response.Fail(c, http.StatusBadRequest, "invalid id")
	}
	data, err := h.svc.PeekProductCache(c.UserContext(), id)
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "redis cache peek", data)
}

func (h *Handler) redisInvalidate(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return response.Fail(c, http.StatusBadRequest, "invalid id")
	}
	data, err := h.svc.InvalidateProductCache(c.UserContext(), id)
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "redis cache invalidated", data)
}

func (h *Handler) mongoEvents(c *fiber.Ctx) error {
	limit, _ := strconv.ParseInt(c.Query("limit", "20"), 10, 64)
	data, err := h.svc.MongoEvents(c.UserContext(), limit)
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "mongo audit events", data)
}

func (h *Handler) kafkaPing(c *fiber.Ctx) error {
	userID, ok := middleware.UserID(c)
	if !ok {
		return response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
	}
	data, err := h.svc.KafkaPing(c.UserContext(), userID)
	if err != nil {
		return mapErr(c, err)
	}
	return response.Created(c, "kafka ping published", data)
}

func (h *Handler) typesenseExplore(c *fiber.Ctx) error {
	data, err := h.svc.TypesenseExplore(c.UserContext(), c.Query("q"))
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "typesense explore", data)
}

func (h *Handler) typesenseReindex(c *fiber.Ctx) error {
	data, err := h.svc.TypesenseReindex(c.UserContext())
	if err != nil {
		return mapErr(c, err)
	}
	return response.OK(c, "typesense reindex done", data)
}

func parseID(c *fiber.Ctx) (uint, error) {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	return uint(id), err
}

func mapErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, domain.ErrUnavailable):
		return response.FailCode(c, http.StatusServiceUnavailable, err.Error(), domain.ErrorCode(err))
	case errors.Is(err, domain.ErrNotFound):
		return response.FailCode(c, http.StatusNotFound, err.Error(), domain.ErrorCode(err))
	default:
		return response.Fail(c, http.StatusInternalServerError, "internal error")
	}
}
