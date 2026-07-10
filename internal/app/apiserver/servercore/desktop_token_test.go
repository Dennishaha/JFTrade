package servercore

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDesktopTokenMiddlewareProtectsHTTPAndWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{desktopAPIToken: "desktop-token"}
	router := gin.New()
	router.Use(server.desktopTokenMiddleware())
	router.GET("/api/v1/status", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	assertStatus := func(request *http.Request, want int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Fatalf("status = %d, want %d; body=%s", recorder.Code, want, recorder.Body.String())
		}
	}

	assertStatus(httptest.NewRequest(http.MethodGet, "/api/v1/status", nil), http.StatusUnauthorized)
	authorized := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	authorized.Header.Set("Authorization", "Bearer desktop-token")
	assertStatus(authorized, http.StatusNoContent)

	webSocket := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	webSocket.Header.Set("Upgrade", "websocket")
	webSocket.Header.Set("Sec-WebSocket-Protocol", desktopWebSocketProtocol+", desktop-token")
	assertStatus(webSocket, http.StatusNoContent)
}
