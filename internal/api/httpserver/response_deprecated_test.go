package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDeprecatedSetsDeprecationHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/old", Deprecated("/api/v1/new", func(c *gin.Context) {
		WriteOK(c, map[string]any{"fine": true})
	}))
	router.GET("/plain", Deprecated("", func(c *gin.Context) {
		WriteOK(c, nil)
	}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/old", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Deprecation"); got != "true" {
		t.Fatalf("Deprecation header = %q", got)
	}
	if got := recorder.Header().Get("Link"); got != `</api/v1/new>; rel="successor-version"` {
		t.Fatalf("Link header = %q", got)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/plain", nil))
	if got := recorder.Header().Get("Deprecation"); got != "true" {
		t.Fatalf("Deprecation header = %q", got)
	}
	if got := recorder.Header().Get("Link"); got != "" {
		t.Fatalf("Link header should be empty without replacement, got %q", got)
	}
}
