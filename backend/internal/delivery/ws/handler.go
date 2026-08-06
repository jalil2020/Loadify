package ws

import (
	"context"
	"strings"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
	jwtpkg "github.com/loadify/backend/pkg/jwt"
	"github.com/loadify/backend/pkg/logger"
)

// Handler upgrades HTTP requests to WebSocket connections.
type Handler struct {
	hub    *Hub
	router *Router
	tokens jwtpkg.Manager
	users  domain.UserRepository
	log    logger.Logger
}

// NewHandler builds the WebSocket handler.
func NewHandler(hub *Hub, router *Router, tokens jwtpkg.Manager, users domain.UserRepository, log logger.Logger) *Handler {
	return &Handler{hub: hub, router: router, tokens: tokens, users: users, log: log}
}

// Upgrade authenticates the request and stores the verified identity for the
// socket handler.
//
// Authentication happens here, before the upgrade, because a rejected handshake
// yields a normal HTTP error the client can read — a socket closed after the
// upgrade only produces an opaque close code.
func (h *Handler) Upgrade(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return apperror.ErrBadRequest.WithMessage("This endpoint expects a WebSocket upgrade")
	}

	token := bearerToken(c)
	if token == "" {
		// Browsers cannot set headers on a WebSocket handshake, so the query
		// parameter is the only option for the web client.
		token = c.Query("token")
	}
	if token == "" {
		return apperror.ErrUnauthorized.WithMessage("A token is required to open a socket")
	}

	claims, err := h.tokens.ParseAccess(token)
	if err != nil {
		return err
	}

	// A blocked account must not hold a live socket, so the status is checked
	// against the database rather than trusted from the token.
	user, err := h.users.FindByID(c.UserContext(), claims.UserID)
	if err != nil {
		return apperror.ErrUnauthorized.Wrap(err)
	}
	if !user.IsActive() {
		return apperror.ErrAccountBlocked
	}

	c.Locals("ws_claims", claims)
	return c.Next()
}

// Serve is the socket handler, mounted behind Upgrade.
func (h *Handler) Serve() fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		claims, ok := conn.Locals("ws_claims").(*jwtpkg.Claims)
		if !ok || claims == nil {
			_ = conn.Close()
			return
		}

		client := &Client{
			hub:       h.hub,
			conn:      conn,
			userID:    claims.UserID,
			companyID: claims.CompanyID,
			driverID:  claims.DriverID,
			roles:     claims.Roles,
			send_:     make(chan []byte, sendBuffer),
			log:       h.log,
		}

		h.hub.register(client)
		h.log.Info("socket connected",
			logger.String("user_id", claims.UserID.String()),
			logger.Int("connections", h.hub.Connections()))

		client.sendEvent("connected", map[string]any{
			"user_id":     claims.UserID,
			"server_time": nowUTC(),
		})

		go client.writePump()
		// readPump blocks until the connection closes, keeping this goroutine —
		// which the websocket library owns — alive for the connection's lifetime.
		client.readPump(context.Background(), h.router)

		h.log.Info("socket disconnected",
			logger.String("user_id", claims.UserID.String()),
			logger.Int("connections", h.hub.Connections()))
	}, websocket.Config{
		// Origin checking is handled by the CORS middleware and the token
		// requirement; the handshake itself accepts any origin that gets here.
		EnableCompression: true,
	})
}

// bearerToken extracts a token from the Authorization header.
func bearerToken(c *fiber.Ctx) string {
	header := c.Get(fiber.HeaderAuthorization)
	if header == "" {
		return ""
	}
	after, found := strings.CutPrefix(header, "Bearer ")
	if !found {
		return ""
	}
	return strings.TrimSpace(after)
}
