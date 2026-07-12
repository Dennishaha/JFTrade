package servercore

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDesktopTokenMiddlewareProtectsHTTPAndWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{
		desktopAPIToken: "desktop-token",
		auth:            newWebAuth(SecuritySettings{}),
	}
	server.auth.enforceAccess = true
	router := gin.New()
	router.Use(server.desktopTokenMiddleware())
	router.Use(server.webAccessMiddleware())
	router.GET("/api/v1/status", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	assertStatus := func(request *http.Request, want int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Fatalf("status = %d, want %d; body=%s", recorder.Code, want, recorder.Body.String())
		}
	}

	assertStatus(httptest.NewRequest(http.MethodGet, "/api/v1/status", nil), http.StatusForbidden)
	authorized := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	authorized.Header.Set("Authorization", "Bearer desktop-token")
	assertStatus(authorized, http.StatusNoContent)

	webSocket := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	webSocket.Header.Set("Upgrade", "websocket")
	webSocket.Header.Set("Sec-WebSocket-Protocol", desktopWebSocketProtocol+", desktop-token")
	assertStatus(webSocket, http.StatusNoContent)
}

func TestDesktopSidecarDoesNotDoubleAsBrowserListener(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{
		desktopMode: true,
		auth: newWebAuth(SecuritySettings{
			WebAccessEnabled:    true,
			PublicAccessEnabled: true,
			PasswordConfigured:  true,
			PasswordHash:        "configured",
		}),
	}
	server.auth.enforceAccess = true
	router := gin.New()
	router.Use(server.desktopTokenMiddleware())
	router.Use(server.webAccessMiddleware())
	router.GET("/api/v1/status", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	server.router = router

	desktopRecorder := httptest.NewRecorder()
	server.ServeHTTP(desktopRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	if desktopRecorder.Code != http.StatusForbidden {
		t.Fatalf("desktop sidecar status = %d, want 403", desktopRecorder.Code)
	}

	webRecorder := httptest.NewRecorder()
	server.WebAccessHandler().ServeHTTP(webRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	if webRecorder.Code != http.StatusNoContent {
		t.Fatalf("Web listener status = %d, want 204; body=%s", webRecorder.Code, webRecorder.Body.String())
	}
}

func TestDesktopDevelopmentWithoutInjectedTokenRemainsTrusted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{
		desktopMode: true,
		auth:        newWebAuth(SecuritySettings{}),
	}
	server.auth.enforceAccess = false
	router := gin.New()
	router.Use(server.desktopTokenMiddleware())
	router.PUT("/api/v1/settings/security", func(c *gin.Context) {
		if !isDesktopRequest(c.Request) {
			c.Status(http.StatusForbidden)
			return
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/v1/settings/security", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestDesktopDevelopmentWebListenerStillRequiresPasswordSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{
		desktopMode: true,
		auth: newWebAuth(SecuritySettings{
			WebAccessEnabled:    true,
			PublicAccessEnabled: true,
			PasswordConfigured:  true,
			PasswordHash:        "configured",
		}),
	}
	server.auth.enforceAccess = false
	router := gin.New()
	router.Use(server.desktopTokenMiddleware())
	router.Use(server.webAccessMiddleware())
	router.Use(server.authMiddleware())
	router.GET("/api/v1/status", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	server.router = router

	recorder := httptest.NewRecorder()
	server.WebAccessHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("Web listener status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStandaloneServerWithoutDesktopTokenIsNotTrusted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{auth: newWebAuth(SecuritySettings{})}
	server.auth.enforceAccess = true
	router := gin.New()
	router.Use(server.desktopTokenMiddleware())
	router.GET("/api/v1/status", func(c *gin.Context) {
		if isDesktopRequest(c.Request) {
			c.Status(http.StatusNoContent)
			return
		}
		c.Status(http.StatusForbidden)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}
