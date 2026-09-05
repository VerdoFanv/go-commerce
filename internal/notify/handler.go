package notify

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/middleware"
	"github.com/verdofanv/golang-be/pkg/response"
)

// Handler exposes the WebSocket endpoint. Auth uses ?token=<jwt> because
// browser WebSocket clients cannot set custom headers.
type Handler struct {
	hub *Hub
	cfg config.Config
}

func NewHandler(hub *Hub, cfg config.Config) *Handler {
	return &Handler{hub: hub, cfg: cfg}
}

// RegisterRoutes wires:
//
//	GET /ws/products — upgrade to WebSocket, stream product.* events
func (h *Handler) RegisterRoutes(app *fiber.App) {
	app.Use("/ws", func(c *fiber.Ctx) error {
		if !websocket.IsWebSocketUpgrade(c) {
			return fiber.ErrUpgradeRequired
		}
		token := strings.TrimSpace(c.Query("token"))
		if _, err := middleware.ParseToken(h.cfg.JWTSecret, token); err != nil {
			return response.Fail(c, http.StatusUnauthorized, "invalid token")
		}
		return c.Next()
	})

	app.Get("/ws/products", websocket.New(func(conn *websocket.Conn) {
		h.hub.Add(conn)
		defer func() {
			h.hub.Remove(conn)
			_ = conn.Close()
		}()

		// Read pump: we never expect client messages, but reading detects closure.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
}
