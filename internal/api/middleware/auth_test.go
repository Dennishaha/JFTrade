package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthSkipsPublicPaths(t *testing.T) {
	for _, path := range []string{
		"/health",
		"/api/v1/auth/login",
		"/api/v1/auth/session",
		"/api/v1/system/status",
	} {
		t.Run(path, func(t *testing.T) {
			resp := performAuthRequest(http.MethodGet, path, nil, nil, nil, nil)
			if resp.Code != http.StatusNoContent {
				t.Fatalf("status = %d", resp.Code)
			}
		})
	}
}

func TestAuthBearerWithoutBrowserOriginBypassesCSRFChecks(t *testing.T) {
	auth := &stubAuthenticator{ok: true, bearer: true}
	resp := performAuthRequest(http.MethodPost, "/api/v1/settings/ui", nil, auth, nil, nil)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d", resp.Code)
	}
}

func TestAuthBearerStillRequiresTrustedBrowserOrigin(t *testing.T) {
	auth := &stubAuthenticator{ok: true, bearer: true}

	denied := performAuthRequest(http.MethodPost, "/api/v1/settings/ui", map[string]string{
		"Origin": "http://evil.example",
	}, auth, nil, allowOrigins("http://localhost:5173"))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("untrusted origin status = %d", denied.Code)
	}

	malformed := performAuthRequest(http.MethodPost, "/api/v1/settings/ui", map[string]string{
		"Origin":  "null",
		"Referer": "http://localhost:5173/app",
	}, auth, nil, allowOrigins("http://localhost:5173"))
	if malformed.Code != http.StatusForbidden {
		t.Fatalf("malformed origin status = %d", malformed.Code)
	}

	allowed := performAuthRequest(http.MethodPost, "/api/v1/settings/ui", map[string]string{
		"Origin": "http://localhost:5173",
	}, auth, nil, allowOrigins("http://localhost:5173"))
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("trusted origin status = %d", allowed.Code)
	}
}

func TestAuthRejectsNilAuthenticator(t *testing.T) {
	resp := performAuthRequest(http.MethodGet, "/api/v1/settings/ui", nil, nil, nil, nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.Code)
	}
}

func TestAuthRejectsUntrustedOrigin(t *testing.T) {
	resp := performAuthRequest(http.MethodGet, "/api/v1/settings/ui", map[string]string{
		"Origin": "http://evil.example",
	}, &stubAuthenticator{ok: true, session: "csrf"}, nil, allowOrigins("http://localhost:5173"))
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d", resp.Code)
	}
}

func TestAuthRequiresOriginAndCSRFForSessionWrites(t *testing.T) {
	auth := &stubAuthenticator{ok: true, session: "csrf"}
	origins := allowOrigins("http://localhost:5173")

	noOrigin := performAuthRequest(http.MethodPost, "/api/v1/settings/ui", nil, auth, &stubCSRFValidator{valid: true}, origins)
	if noOrigin.Code != http.StatusForbidden {
		t.Fatalf("no origin status = %d", noOrigin.Code)
	}

	badCSRF := performAuthRequest(http.MethodPost, "/api/v1/settings/ui", map[string]string{
		"Origin": "http://localhost:5173",
	}, auth, &stubCSRFValidator{valid: false}, origins)
	if badCSRF.Code != http.StatusForbidden {
		t.Fatalf("bad csrf status = %d", badCSRF.Code)
	}

	ok := performAuthRequest(http.MethodPost, "/api/v1/settings/ui", map[string]string{
		"Origin":       "http://localhost:5173",
		"X-CSRF-Token": "csrf",
	}, auth, &stubCSRFValidator{valid: true}, origins)
	if ok.Code != http.StatusNoContent {
		t.Fatalf("ok status = %d", ok.Code)
	}
}

func TestCORSReflectsAllowedOriginsAndRejectsUnknownPreflight(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(CORS(allowOrigins("http://localhost:5173")))
	router.GET("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.OPTIONS("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	allowedReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/settings/ui", nil)
	allowedReq.Header.Set("Origin", "http://localhost:5173")
	allowedResp := httptest.NewRecorder()
	router.ServeHTTP(allowedResp, allowedReq)
	if allowedResp.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("allow origin header = %q", allowedResp.Header().Get("Access-Control-Allow-Origin"))
	}
	if got := allowedResp.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, "X-Request-ID") {
		t.Fatalf("expose headers = %q, want X-Request-ID", got)
	}
	if got := allowedResp.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-Request-ID") {
		t.Fatalf("allow headers = %q, want X-Request-ID", got)
	}
	deniedReq := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/api/v1/settings/ui", nil)
	deniedReq.Header.Set("Origin", "http://evil.example")
	deniedResp := httptest.NewRecorder()
	router.ServeHTTP(deniedResp, deniedReq)
	if deniedResp.Code != http.StatusForbidden {
		t.Fatalf("denied preflight status = %d", deniedResp.Code)
	}

	malformedReq := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/api/v1/settings/ui", nil)
	malformedReq.Header.Set("Origin", "null")
	malformedReq.Header.Set("Referer", "http://localhost:5173/app")
	malformedResp := httptest.NewRecorder()
	router.ServeHTTP(malformedResp, malformedReq)
	if malformedResp.Code != http.StatusForbidden {
		t.Fatalf("malformed preflight status = %d", malformedResp.Code)
	}
}

func performAuthRequest(method string, path string, headers map[string]string, auth Authenticator, csrf CSRFValidator, origins OriginChecker) *httptest.ResponseRecorder {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(Auth(auth, csrf, nil, origins))
	router.Any("/*path", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequestWithContext(context.Background(), method, path, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

type stubAuthenticator struct {
	session string
	ok      bool
	bearer  bool
}

func (a *stubAuthenticator) Authenticate(*http.Request) (string, bool, bool) {
	return a.session, a.ok, a.bearer
}

type stubCSRFValidator struct {
	valid bool
}

func (v *stubCSRFValidator) ValidateCSRF(r *http.Request, session string) bool {
	return v.valid && r.Header.Get("X-CSRF-Token") == session
}

type originSet map[string]struct{}

func allowOrigins(origins ...string) originSet {
	result := originSet{}
	for _, origin := range origins {
		result[origin] = struct{}{}
	}
	return result
}

func (s originSet) IsOriginAllowed(origin string) bool {
	_, ok := s[origin]
	return ok
}
