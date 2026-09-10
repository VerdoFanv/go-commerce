package notify

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/http/middleware"
	"github.com/verdofanv/golang-be/pkg/response"
)

// Handler exposes the WebSocket endpoint. Auth uses ?token=<jwt> because
// browser WebSocket clients cannot set custom headers. Optional ?apikey= mirrors HTTP.
type Handler struct {
	hub      *Hub
	cfg      config.Config
	upgrader websocket.Upgrader
}

func NewHandler(hub *Hub, cfg config.Config) *Handler {
	h := &Handler{hub: hub, cfg: cfg}
	h.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     h.checkOrigin,
	}
	return h
}

func (h *Handler) checkOrigin(r *http.Request) bool {
	origins := h.cfg.CORSOrigins
	if len(origins) == 0 || (len(origins) == 1 && origins[0] == "*") {
		return !h.cfg.IsProduction()
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser clients
	}
	for _, o := range origins {
		if o == origin {
			return true
		}
	}
	return false
}

// RegisterRoutes wires:
//
//	GET /ws/products — upgrade to WebSocket, stream product.* events
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	ws := r.Group("/ws")
	ws.Use(h.authenticate)
	{
		ws.GET("/products", h.products)
	}
}

// authenticate rejects non-upgrade requests and validates apikey + access JWT.
func (h *Handler) authenticate(c *gin.Context) {
	if !websocket.IsWebSocketUpgrade(c.Request) {
		response.Fail(c, http.StatusUpgradeRequired, "upgrade required")
		c.Abort()
		return
	}
	if h.cfg.APIKey != "" {
		key := strings.TrimSpace(c.Query("apikey"))
		if key == "" {
			key = c.GetHeader("apikey")
		}
		if key == "" {
			key = c.GetHeader("X-API-Key")
		}
		if key != h.cfg.APIKey {
			response.Fail(c, http.StatusUnauthorized, "invalid api key")
			c.Abort()
			return
		}
	}
	token := strings.TrimSpace(c.Query("token"))
	if _, err := middleware.ParseAccessToken(h.cfg.JWTSecret, token); err != nil {
		response.Fail(c, http.StatusUnauthorized, "invalid token")
		c.Abort()
		return
	}
	c.Next()
}

func (h *Handler) products(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		slog.Debug("ws upgrade failed", "err", err)
		return
	}

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
}
