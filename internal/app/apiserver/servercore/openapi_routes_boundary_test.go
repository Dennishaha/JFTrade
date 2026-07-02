package servercore

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSwaggerRoutesRedirectToBrowsableDocumentation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := &Server{}
	router := gin.New()
	router.GET("/swagger", server.handleSwaggerRoot)
	router.GET("/swagger/*any", server.handleSwaggerUI)

	for _, path := range []string{"/swagger", "/swagger/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusTemporaryRedirect {
			t.Fatalf("%s status = %d, want 307", path, rec.Code)
		}
		if location := rec.Header().Get("Location"); location != "/swagger/index.html" {
			t.Fatalf("%s Location = %q", path, location)
		}
	}
}
