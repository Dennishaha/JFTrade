package servercore

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/middleware"
)

const desktopWebSocketProtocol = "jftrade.desktop.v1"

func (s *Server) desktopTokenMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		required := strings.TrimSpace(s.desktopAPIToken)
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		if required == "" {
			// Wails development loads the frontend from Vite, so there is no
			// private asset channel through which to inject the per-process
			// capability used by packaged builds. Preserve the existing
			// loopback-only development experience and let the desktop settings
			// screen configure Web access. Production desktop builds always set
			// a capability and never enter this branch.
			if s.desktopMode && s.auth != nil && !s.auth.enforceAccess && !isWebAccessSurfaceRequest(c.Request) {
				c.Request = middleware.MarkRequestTrustedHost(c.Request)
			}
			c.Next()
			return
		}
		if constantTimeEqual(desktopTokenFromRequest(c.Request), required) {
			c.Request = middleware.MarkRequestTrustedHost(c.Request)
			c.Next()
			return
		}
		// A request without the private desktop capability may still be a
		// password-authenticated browser request. webAccessMiddleware decides
		// whether that surface is enabled and reachable from this address.
		c.Next()
	}
}

func isDesktopRequest(r *http.Request) bool {
	return middleware.IsRequestTrustedHost(r)
}

func desktopTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if token := bearerToken(r.Header.Get("Authorization")); token != "" {
		return token
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return ""
	}
	for protocol := range strings.SplitSeq(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		protocol = strings.TrimSpace(protocol)
		if protocol != "" && protocol != desktopWebSocketProtocol {
			return protocol
		}
	}
	return ""
}
