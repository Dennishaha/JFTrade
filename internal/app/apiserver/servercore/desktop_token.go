package servercore

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const desktopWebSocketProtocol = "jftrade.desktop.v1"

func (s *Server) desktopTokenMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		required := strings.TrimSpace(s.desktopAPIToken)
		if required == "" || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		if constantTimeEqual(desktopTokenFromRequest(c.Request), required) {
			c.Next()
			return
		}
		s.writeError(c, http.StatusUnauthorized, "DESKTOP_TOKEN_REQUIRED", "valid desktop API credentials are required")
	}
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
	for _, protocol := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		protocol = strings.TrimSpace(protocol)
		if protocol != "" && protocol != desktopWebSocketProtocol {
			return protocol
		}
	}
	return ""
}
