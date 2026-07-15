package servercore

import (
	"bytes"
	stdcontext "context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/middleware"
)

type alwaysErrorReader struct{ err error }

func (r alwaysErrorReader) Read([]byte) (int, error) { return 0, r.err }

func authTestContext(method string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, "http://trade.example/api/v1/auth/login", bytes.NewReader(body))
	context.Request.Host = "trade.example"
	context.Request.RemoteAddr = "127.0.0.1:12345"
	context.Request.Header.Set("Content-Type", "application/json")
	return context, recorder
}

func enabledTestWebAuth() *webAuth {
	auth := newWebAuth(SecuritySettings{})
	auth.enabled = true
	auth.passwordHash = "test-hash"
	auth.verifyPassword = func(_, _ string) (bool, error) { return true, nil }
	return auth
}

func TestWebAuthRemainingNilAndRequestHelpers(t *testing.T) {
	var nilAuth *webAuth
	nilAuth.configure(SecuritySettings{})
	if nilAuth.currentAccessContext() == nil {
		t.Fatal("nil current access context")
	}
	nilAuth.close()
	nilAuth.configureOrigins("http://trade.example")
	if nilAuth.originAllowedForRequest(nil, "http://trade.example") || nilAuth.browserAccessAllowed(nil) || nilAuth.webAccessEnabled() {
		t.Fatal("nil auth unexpectedly allowed access")
	}
	if _, ok, trusted := nilAuth.authenticate(httptest.NewRequest(http.MethodGet, "/", nil)); ok || trusted {
		t.Fatal("nil auth unexpectedly authenticated request")
	}

	auth := newWebAuth(SecuritySettings{})
	auth.configure(SecuritySettings{WebAccessEnabled: true, PasswordHash: "invalid"})
	if !auth.unavailable {
		t.Fatal("invalid enabled password hash was not marked unavailable")
	}
	auth.accessContext = nil
	if auth.currentAccessContext() == nil {
		t.Fatal("missing fallback access context")
	}
	if auth.originAllowedForRequest(nil, " ") {
		t.Fatal("blank origin unexpectedly allowed")
	}

	if sameRequestOrigin(nil, "http://trade.example") {
		t.Fatal("nil request has an origin")
	}
	request := httptest.NewRequest(http.MethodGet, "http://trade.example/", nil)
	request.Host = "trade.example"
	if sameRequestOrigin(request, "://bad") {
		t.Fatal("invalid origin matched")
	}
	request.TLS = &tls.ConnectionState{}
	if requestScheme(request) != "https" {
		t.Fatal("TLS request did not use https")
	}
	if requestOrigin(nil) != "" || requestOriginProvided(nil) {
		t.Fatal("nil request reported an origin")
	}
	request.Header.Del("Origin")
	request.Header.Set("Referer", "https://trade.example/path")
	if requestOrigin(request) != "https://trade.example" {
		t.Fatalf("referer origin = %q", requestOrigin(request))
	}
	if requestRemoteIP(nil) != nil {
		t.Fatal("nil request returned a remote IP")
	}
	request.RemoteAddr = "192.0.2.10"
	if got := requestRemoteIP(request); got == nil || got.String() != "192.0.2.10" {
		t.Fatalf("unqualified remote address = %v", got)
	}
	auth.close()
}

func TestWebAuthRemainingAuthenticationStates(t *testing.T) {
	disabled := newWebAuth(SecuritySettings{})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok, trusted := disabled.authenticate(request); !ok || !trusted {
		t.Fatal("unenforced disabled auth did not preserve test bypass")
	}
	disabled.enforceAccess = true
	if _, ok, trusted := disabled.authenticate(request); ok || trusted {
		t.Fatal("enforced disabled auth authenticated request")
	}
	disabled.enabled = true
	disabled.unavailable = true
	if _, ok, trusted := disabled.authenticate(request); ok || trusted {
		t.Fatal("unavailable auth authenticated request")
	}

	auth := enabledTestWebAuth()
	auth.publicAccessEnabled = true
	if !auth.browserAccessAllowed(httptest.NewRequest(http.MethodGet, "/", nil)) || !auth.webAccessEnabled() {
		t.Fatal("enabled public auth did not allow browser access")
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	auth.now = func() time.Time { return now }
	auth.sessions["expired"] = webSession{CSRF: "old", ExpiresAt: now}
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: webSessionCookie, Value: "expired"})
	if _, ok, _ := auth.authenticate(request); ok {
		t.Fatal("expired session authenticated")
	}

	trustedRequest := middleware.MarkRequestTrustedHost(httptest.NewRequest(http.MethodGet, "/", nil))
	if _, ok, trusted := auth.authenticate(trustedRequest); !ok || !trusted {
		t.Fatal("desktop request was not trusted")
	}
}

func TestWebAuthRemainingLoginResponses(t *testing.T) {
	tests := []struct {
		name   string
		auth   *webAuth
		body   []byte
		status int
		setup  func(*gin.Context)
	}{
		{name: "nil", body: []byte(`{}`), status: http.StatusForbidden},
		{name: "disabled", auth: newWebAuth(SecuritySettings{}), body: []byte(`{}`), status: http.StatusForbidden},
		{name: "unavailable", auth: func() *webAuth { a := enabledTestWebAuth(); a.unavailable = true; return a }(), body: []byte(`{}`), status: http.StatusServiceUnavailable},
		{name: "bad json", auth: enabledTestWebAuth(), body: []byte(`{`), status: http.StatusBadRequest},
		{name: "desktop", auth: newWebAuth(SecuritySettings{}), body: []byte(`{}`), status: http.StatusOK, setup: func(c *gin.Context) { c.Request = middleware.MarkRequestTrustedHost(c.Request) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, recorder := authTestContext(http.MethodPost, test.body)
			if test.setup != nil {
				test.setup(context)
			}
			test.auth.login(context)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.status, recorder.Body.String())
			}
		})
	}

	auth := enabledTestWebAuth()
	auth.verifyPassword = nil
	context, recorder := authTestContext(http.MethodPost, []byte(`{"password":"wrong"}`))
	auth.login(context)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("default verifier status = %d", recorder.Code)
	}

	auth = enabledTestWebAuth()
	cancelContext, cancel := stdcontext.WithCancel(stdcontext.Background())
	auth.verifyPassword = func(_, _ string) (bool, error) {
		cancel()
		return false, errors.New("verification interrupted")
	}
	ginContext, recorder := authTestContext(http.MethodPost, []byte(`{"password":"password"}`))
	ginContext.Request = ginContext.Request.WithContext(cancelContext)
	auth.login(ginContext)
	if recorder.Code != http.StatusRequestTimeout {
		t.Fatalf("canceled login status = %d; body=%s", recorder.Code, recorder.Body.String())
	}

	auth = enabledTestWebAuth()
	auth.generateSecret = func(int) (string, error) { return "", errors.New("entropy unavailable") }
	ginContext, recorder = authTestContext(http.MethodPost, []byte(`{"password":"password"}`))
	auth.login(ginContext)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("session entropy status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestWebAuthRemainingSessionAndPruningErrors(t *testing.T) {
	auth := enabledTestWebAuth()
	auth.generateSecret = func(int) (string, error) { return "", errors.New("first entropy failure") }
	if _, _, err := auth.createSession(auth.generation, auth.passwordHash); err == nil {
		t.Fatal("first session secret failure was ignored")
	}
	calls := 0
	auth.generateSecret = func(int) (string, error) {
		calls++
		if calls == 1 {
			return "session-id", nil
		}
		return "", errors.New("second entropy failure")
	}
	if _, _, err := auth.createSession(auth.generation, auth.passwordHash); err == nil {
		t.Fatal("second session secret failure was ignored")
	}
	auth.generateSecret = nil

	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	auth.now = func() time.Time { return now }
	for index := range maxWebSessions {
		auth.sessions[string(rune(index+1))] = webSession{ExpiresAt: now.Add(time.Duration(index+1) * time.Minute)}
	}
	if _, _, err := auth.createSession(auth.generation, auth.passwordHash); err != nil {
		t.Fatalf("create full session: %v", err)
	}
	if len(auth.sessions) != maxWebSessions {
		t.Fatalf("sessions = %d, want %d", len(auth.sessions), maxWebSessions)
	}

	auth.attempts["expired"] = loginAttempt{WindowStart: now.Add(-loginWindow)}
	auth.pruneLoginAttemptsLocked(now)
	if _, exists := auth.attempts["expired"]; exists {
		t.Fatal("expired login attempt was not pruned")
	}
	auth.sessions["expired"] = webSession{ExpiresAt: now}
	auth.pruneSessionsLocked(now)
	if _, exists := auth.sessions["expired"]; exists {
		t.Fatal("expired session was not pruned")
	}

	if _, err := randomSecretFrom(alwaysErrorReader{err: io.ErrUnexpectedEOF}, 8); err == nil {
		t.Fatal("randomSecretFrom ignored reader failure")
	}
}

func TestWebAuthRemainingCanceledPasswordSlotAndStatus(t *testing.T) {
	for range cap(webPasswordCheckSlots) {
		webPasswordCheckSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for len(webPasswordCheckSlots) > 0 {
			<-webPasswordCheckSlots
		}
	})
	ctx, cancel := stdcontext.WithCancel(stdcontext.Background())
	cancel()
	auth := enabledTestWebAuth()
	if _, err := auth.checkPasswordForLogin(ctx, "hash", "password", auth.verifyPassword); !errors.Is(err, stdcontext.Canceled) {
		t.Fatalf("canceled password check error = %v", err)
	}
	for len(webPasswordCheckSlots) > 0 {
		<-webPasswordCheckSlots
	}

	ginContext, recorder := authTestContext(http.MethodGet, nil)
	ginContext.Request.Header.Set("Origin", "https://evil.example")
	auth.status(ginContext)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("forbidden status request = %d", recorder.Code)
	}

	now := time.Now().UTC()
	auth.now = func() time.Time { return now }
	auth.sessions["valid"] = webSession{CSRF: "csrf", ExpiresAt: now.Add(time.Hour)}
	ginContext, recorder = authTestContext(http.MethodGet, nil)
	ginContext.Request.AddCookie(&http.Cookie{Name: webSessionCookie, Value: "valid"})
	auth.status(ginContext)
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(`"csrfToken":"csrf"`)) {
		t.Fatalf("authenticated status = %d %s", recorder.Code, recorder.Body.String())
	}
}
