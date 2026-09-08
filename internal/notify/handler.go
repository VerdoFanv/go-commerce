package notify

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/middleware"
	"github.com/verdofanv/golang-be/pkg/response"
)

// upgrader turns the plain HTTP request into a WebSocket connection. Origin is
// unrestricted because this is a public API guarded by the JWT query param.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

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
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	ws := r.Group("/ws")
	ws.Use(h.authenticate)
	{
		ws.GET("/products", h.products)
	}
}

// authenticate rejects non-upgrade requests and validates the ?token= JWT.
func (h *Handler) authenticate(c *gin.Context) {
	if !websocket.IsWebSocketUpgrade(c.Request) {
		response.Fail(c, http.StatusUpgradeRequired, "upgrade required")
		c.Abort()
		return
	}
	token := strings.TrimSpace(c.Query("token"))
	if _, err := middleware.ParseToken(h.cfg.JWTSecret, token); err != nil {
		response.Fail(c, http.StatusUnauthorized, "invalid token")
		c.Abort()
		return
	}
	c.Next()
}

func (h *Handler) products(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
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
