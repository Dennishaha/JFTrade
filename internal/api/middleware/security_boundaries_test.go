package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fixedWriteDetector struct {
	write bool
}

func (d fixedWriteDetector) IsWriteMethod(*http.Request) bool { return d.write }

func TestAuthenticationBoundaryDecisions(t *testing.T) {
	if requiresAuthentication(nil) {
		t.Fatal("nil request requires authentication")
	}

	t.Run("failed authenticator", func(t *testing.T) {
		response := performAuthRequest(http.MethodGet, "/api/v1/settings/ui", nil, &stubAuthenticator{}, nil, nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", response.Code)
		}
	})

	t.Run("session origin requires checker", func(t *testing.T) {
		response := performAuthRequest(http.MethodGet, "/api/v1/settings/ui", map[string]string{
			"Origin": "http://localhost:5173",
		}, &stubAuthenticator{ok: true}, nil, nil)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d", response.Code)
		}
	})

	t.Run("session read without origin", func(t *testing.T) {
		response := performAuthRequest(http.MethodGet, "/api/v1/settings/ui", nil, &stubAuthenticator{ok: true}, nil, nil)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d", response.Code)
		}
	})
}

func TestWriteMethodDetectionSupportsOverridesAndNilRequests(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if !isWriteMethod(fixedWriteDetector{write: true}, request) {
		t.Fatal("custom detector result was ignored")
	}
	if isWriteMethod(nil, nil) {
		t.Fatal("nil request is a write")
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		request.Method = method
		if !isWriteMethod(nil, request) {
			t.Fatalf("method %s was not classified as a write", method)
		}
	}
}

func TestCORSAllowsTrustedPreflightAndSameOriginOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS(allowOrigins("https://app.example")))
	router.OPTIONS("/*path", func(c *gin.Context) { c.Status(http.StatusTeapot) })

	tests := []struct {
		name   string
		origin string
	}{
		{name: "trusted cross origin", origin: "https://app.example"},
		{name: "same origin request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/api/v1/settings/ui", nil)
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d", response.Code)
			}
		})
	}
}

func TestRequestOriginUsesRefererAndHandlesNil(t *testing.T) {
	if origin := requestOrigin(nil); origin != "" {
		t.Fatalf("nil origin = %q", origin)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Referer", " HTTPS://Example.COM/app/page ")
	if origin := requestOrigin(request); origin != "https://example.com" {
		t.Fatalf("referer origin = %q", origin)
	}
	request.Header.Set("Origin", "null")
	if origin := requestOrigin(request); origin != "" {
		t.Fatalf("malformed Origin fell back to Referer: %q", origin)
	}
	if !requestOriginProvided(request) {
		t.Fatal("malformed Origin was not recorded as provided")
	}
}

func TestCanonicalOriginRejectsMalformedAndUnsupportedValues(t *testing.T) {
	for _, input := range []string{
		"\t ",
		"example.com",
		"http:example.com",
		"http:/example.com",
		"http://",
		"ftp://example.com/path",
	} {
		if got := canonicalOrigin(input); got != "" {
			t.Fatalf("canonicalOrigin(%q) = %q, want empty", input, got)
		}
	}
	if got := canonicalOrigin("HTTP://EXAMPLE.COM"); got != "http://example.com" {
		t.Fatalf("canonical origin = %q", got)
	}
	if got := canonicalOrigin("wails://LOCALHOST:5173/app"); got != "wails://localhost:5173" {
		t.Fatalf("wails canonical origin = %q", got)
	}
	if got := canonicalOrigin("wails://LOCALHOST/app"); got != "wails://localhost" {
		t.Fatalf("packaged wails canonical origin = %q", got)
	}
	if got := canonicalOrigin("HTTP://WAILS.LOCALHOST/app"); got != "http://wails.localhost" {
		t.Fatalf("windows wails canonical origin = %q", got)
	}
}
