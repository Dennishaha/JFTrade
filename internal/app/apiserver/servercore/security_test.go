package servercore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
)

const testWebPassword = "correct horse battery staple"

func newAuthenticatedSecurityServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.auth.enforceAccess = true
	server.ApplySecuritySettings(webSecuritySettings(t, false))
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	server.auth.configureOrigins(srv.URL)
	return server, srv
}

func webSecuritySettings(t *testing.T, public bool) SecuritySettings {
	t.Helper()
	hash, err := passwordhash.Hash(testWebPassword)
	if err != nil {
		t.Fatalf("passwordhash.Hash: %v", err)
	}
	return SecuritySettings{
		WebAccessEnabled:    true,
		PublicAccessEnabled: public,
		PasswordHash:        hash,
	}
}

func TestWebPasswordIsRequiredForProtectedAPI(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	resp, err := http.Get(srv.URL + "/api/v1/system/status")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	assertErrorCode(t, resp, "WEB_AUTH_REQUIRED")
}

func TestBrowserNavigationGetsFriendlyDisabledWebPage(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	jftradeCheckTestError(t, err)
	server := newTestServer(t, store)
	server.auth.enforceAccess = true
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", contentType)
	}
	if !strings.Contains(recorder.Body.String(), "Web 访问尚未开启") || !strings.Contains(recorder.Body.String(), "设置 → Web 访问") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestSameHostHTTPSProxyUsesSecureSessionCookie(t *testing.T) {
	server, _ := newAuthenticatedSecurityServer(t)
	payload, err := json.Marshal(map[string]string{"password": testWebPassword})
	jftradeCheckTestError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	request.Host = "trade.example"
	request.RemoteAddr = "127.0.0.1:42000"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://trade.example")
	request.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	response := recorder.Result()
	defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie = %#v, want Secure HttpOnly SameSite=Strict", cookies)
	}
	if got := response.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestNetworkClientCannotSpoofHTTPSProxyScheme(t *testing.T) {
	server, _ := newAuthenticatedSecurityServer(t)
	server.ApplySecuritySettings(webSecuritySettings(t, true))
	payload, err := json.Marshal(map[string]string{"password": testWebPassword})
	jftradeCheckTestError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	request.Host = "trade.example"
	request.RemoteAddr = "192.0.2.20:42000"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://trade.example")
	request.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSameHostProxyCannotBypassPublicAccessSetting(t *testing.T) {
	server, _ := newAuthenticatedSecurityServer(t)
	payload, err := json.Marshal(map[string]string{"password": testWebPassword})
	jftradeCheckTestError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	request.Host = "trade.example"
	request.RemoteAddr = "127.0.0.1:42000"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://trade.example")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "192.0.2.20")
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"REMOTE_WEB_ACCESS_DISABLED"`) {
		t.Fatalf("body = %s, want REMOTE_WEB_ACCESS_DISABLED", recorder.Body.String())
	}
}

func TestAdminBearerMechanismNoLongerAuthenticates(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/system/status", nil)
	jftradeCheckTestError(t, err)
	req.Header.Set("Authorization", "Bearer legacy-admin-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET with legacy bearer: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestWebPasswordSessionSupportsReadAndCSRFProtectedWrite(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	client := newCookieClient(t)
	csrf := loginWeb(t, client, srv.URL, testWebPassword)

	readResp, err := client.Get(srv.URL + "/api/v1/system/status")
	if err != nil {
		t.Fatalf("authenticated GET: %v", err)
	}
	jftradeCheckTestError(t, readResp.Body.Close())
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated GET status = %d", readResp.StatusCode)
	}

	write := func(token string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/adk/agents", bytes.NewReader([]byte(`{"name":"csrf-agent","status":"ENABLED"}`)))
		jftradeCheckTestError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", srv.URL)
		if token != "" {
			req.Header.Set("X-CSRF-Token", token)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST: %v", err)
		}
		return resp
	}

	withoutCSRF := write("")
	jftradeCheckTestError(t, withoutCSRF.Body.Close())
	if withoutCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("without CSRF status = %d, want 403", withoutCSRF.StatusCode)
	}
	withCSRF := write(csrf)
	defer func() { jftradeCheckTestError(t, withCSRF.Body.Close()) }()
	if withCSRF.StatusCode == http.StatusUnauthorized || withCSRF.StatusCode == http.StatusForbidden {
		t.Fatalf("with CSRF status = %d", withCSRF.StatusCode)
	}
}

func TestWebLoginRejectsWrongPasswordAndRateLimits(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	client := newCookieClient(t)
	for attempt := range maxLoginFailures {
		resp := requestWebLogin(t, client, srv.URL, "wrong password")
		jftradeCheckTestError(t, resp.Body.Close())
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", attempt+1, resp.StatusCode)
		}
	}
	limited := requestWebLogin(t, client, srv.URL, testWebPassword)
	defer func() { jftradeCheckTestError(t, limited.Body.Close()) }()
	if limited.StatusCode != http.StatusTooManyRequests || limited.Header.Get("Retry-After") == "" {
		t.Fatalf("limited response = %d retry-after=%q", limited.StatusCode, limited.Header.Get("Retry-After"))
	}
}

func TestWebSessionExpiresAndPasswordChangesInvalidateSessions(t *testing.T) {
	server, srv := newAuthenticatedSecurityServer(t)
	client := newCookieClient(t)
	loginWeb(t, client, srv.URL, testWebPassword)

	server.auth.mu.Lock()
	for id, session := range server.auth.sessions {
		session.ExpiresAt = server.auth.now().Add(-time.Second)
		server.auth.sessions[id] = session
	}
	server.auth.mu.Unlock()
	if session := readWebSession(t, client, srv.URL); session.Authenticated {
		t.Fatal("expired Web session remained authenticated")
	}

	loginWeb(t, client, srv.URL, testWebPassword)
	server.ApplySecuritySettings(webSecuritySettingsForPassword(t, "a replacement password phrase", false))
	if session := readWebSession(t, client, srv.URL); session.Authenticated {
		t.Fatal("password change did not invalidate existing Web sessions")
	}
}

func TestSecurityChangeCancelsExistingWebStream(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	jftradeCheckTestError(t, err)
	server := newTestServer(t, store)
	server.auth.enforceAccess = true
	server.ApplySecuritySettings(webSecuritySettings(t, false))
	streamStarted := make(chan struct{})
	streamClosed := make(chan struct{})
	server.router.GET("/api/v1/test/security-stream", func(c *gin.Context) {
		close(streamStarted)
		c.Header("Content-Type", "text/event-stream")
		c.Status(http.StatusOK)
		c.Writer.Flush()
		<-c.Request.Context().Done()
		close(streamClosed)
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	server.auth.configureOrigins(srv.URL)
	client := newCookieClient(t)
	loginWeb(t, client, srv.URL, testWebPassword)

	responseResult := make(chan *http.Response, 1)
	errorResult := make(chan error, 1)
	go func() {
		response, requestErr := client.Get(srv.URL + "/api/v1/test/security-stream")
		if requestErr != nil {
			errorResult <- requestErr
			return
		}
		responseResult <- response
	}()

	select {
	case <-streamStarted:
	case requestErr := <-errorResult:
		t.Fatalf("open stream: %v", requestErr)
	case <-time.After(5 * time.Second):
		t.Fatal("Web stream did not start")
	}
	server.ApplySecuritySettings(webSecuritySettingsForPassword(t, "replacement browser password", false))
	select {
	case <-streamClosed:
	case <-time.After(5 * time.Second):
		t.Fatal("security change did not cancel Web stream")
	}
	select {
	case response := <-responseResult:
		jftradeCheckTestError(t, response.Body.Close())
	case requestErr := <-errorResult:
		t.Fatalf("stream response: %v", requestErr)
	case <-time.After(5 * time.Second):
		t.Fatal("stream response was not released")
	}
}

func TestPasswordChangeDuringLoginCannotCreateOldPasswordSession(t *testing.T) {
	server, srv := newAuthenticatedSecurityServer(t)
	verificationStarted := make(chan struct{})
	continueVerification := make(chan struct{})
	server.auth.verifyPassword = func(_ string, _ string) (bool, error) {
		close(verificationStarted)
		<-continueVerification
		return true, nil
	}

	payload, err := json.Marshal(map[string]string{"password": testWebPassword})
	jftradeCheckTestError(t, err)
	request, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login", bytes.NewReader(payload))
	jftradeCheckTestError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", srv.URL)
	responseResult := make(chan *http.Response, 1)
	errorResult := make(chan error, 1)
	go func() {
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			errorResult <- requestErr
			return
		}
		responseResult <- response
	}()

	select {
	case <-verificationStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("password verification did not start")
	}
	server.ApplySecuritySettings(webSecuritySettingsForPassword(t, "replacement browser password", false))
	close(continueVerification)

	select {
	case requestErr := <-errorResult:
		t.Fatalf("login request: %v", requestErr)
	case response := <-responseResult:
		defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
		if response.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 409", response.StatusCode)
		}
		assertErrorCode(t, response, "WEB_AUTH_CONFIGURATION_CHANGED")
	case <-time.After(5 * time.Second):
		t.Fatal("login request did not finish")
	}
	server.auth.mu.Lock()
	defer server.auth.mu.Unlock()
	if len(server.auth.sessions) != 0 {
		t.Fatalf("sessions = %d, want 0", len(server.auth.sessions))
	}
}

func TestProductionWebDoesNotTrustDevelopmentOrigin(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	payload, err := json.Marshal(map[string]string{"password": testWebPassword})
	jftradeCheckTestError(t, err)
	request, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login", bytes.NewReader(payload))
	jftradeCheckTestError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:3003")
	response, err := http.DefaultClient.Do(request)
	jftradeCheckTestError(t, err)
	defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.StatusCode)
	}
	assertErrorCode(t, response, "ORIGIN_FORBIDDEN")
}

func TestForwardedClientUsesProxyAppendedAddress(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "127.0.0.1:42000"
	request.Header.Set("X-Forwarded-For", "198.51.100.50, 192.0.2.20")
	request.Header.Set("X-Forwarded-Proto", "http, https")
	if got := requestClientIP(request); got == nil || got.String() != "192.0.2.20" {
		t.Fatalf("client IP = %v, want 192.0.2.20", got)
	}
	if got := requestScheme(request); got != "https" {
		t.Fatalf("scheme = %q, want https", got)
	}
}

func TestWebAuthStateMapsStayBounded(t *testing.T) {
	auth := newWebAuth(SecuritySettings{})
	now := time.Now()
	auth.now = func() time.Time { return now }
	for index := range maxLoginAttempts + 20 {
		auth.recordLoginFailure(fmt.Sprintf("192.0.2.%d", index))
	}
	auth.mu.Lock()
	defer auth.mu.Unlock()
	if len(auth.attempts) > maxLoginAttempts {
		t.Fatalf("login attempts = %d, max %d", len(auth.attempts), maxLoginAttempts)
	}
	for index := range maxWebSessions + 20 {
		auth.sessions[fmt.Sprintf("session-%d", index)] = webSession{ExpiresAt: now.Add(time.Duration(index+1) * time.Minute)}
	}
	auth.pruneSessionsLocked(now)
	for len(auth.sessions) > maxWebSessions {
		auth.evictOldestSessionLocked()
	}
	if len(auth.sessions) > maxWebSessions {
		t.Fatalf("sessions = %d, max %d", len(auth.sessions), maxWebSessions)
	}
}

func TestWebLogoutClearsSessionCookie(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	client := newCookieClient(t)
	csrf := loginWeb(t, client, srv.URL, testWebPassword)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/auth/logout", nil)
	jftradeCheckTestError(t, err)
	req.Header.Set("Origin", srv.URL)
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	jftradeCheckTestError(t, resp.Body.Close())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d", resp.StatusCode)
	}
	if session := readWebSession(t, client, srv.URL); session.Authenticated {
		t.Fatal("logout left session authenticated")
	}
}

func TestWebLoginCookieIsHttpOnlyAndSameSiteStrict(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	resp := requestWebLogin(t, http.DefaultClient, srv.URL, testWebPassword)
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	var found bool
	for _, cookie := range resp.Cookies() {
		if cookie.Name != webSessionCookie {
			continue
		}
		found = true
		if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
			t.Fatalf("session cookie flags = %#v", cookie)
		}
	}
	if !found {
		t.Fatal("Web session cookie missing")
	}
}

func TestUntrustedOriginIsRejectedButSameOriginLANHostWorks(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	disallowed := requestWebLoginWithOrigin(t, http.DefaultClient, srv.URL, testWebPassword, "https://evil.example.com")
	jftradeCheckTestError(t, disallowed.Body.Close())
	if disallowed.StatusCode != http.StatusForbidden {
		t.Fatalf("disallowed origin status = %d", disallowed.StatusCode)
	}

	allowed := requestWebLoginWithOrigin(t, http.DefaultClient, srv.URL, testWebPassword, srv.URL)
	defer func() { jftradeCheckTestError(t, allowed.Body.Close()) }()
	if allowed.StatusCode != http.StatusOK {
		t.Fatalf("same origin status = %d", allowed.StatusCode)
	}
}

func TestLoopbackPolicyBlocksRemoteBrowserUntilExplicitlyEnabled(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	jftradeCheckTestError(t, err)
	server := newTestServer(t, store)
	server.auth.enforceAccess = true
	server.ApplySecuritySettings(webSecuritySettings(t, false))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.20:12345"
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "REMOTE_WEB_ACCESS_DISABLED") {
		t.Fatalf("private access response = %d %s", recorder.Code, recorder.Body.String())
	}

	server.ApplySecuritySettings(webSecuritySettings(t, true))
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusForbidden {
		t.Fatalf("explicit network access still blocked: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestDesktopCapabilityStaysPasswordlessWhenWebAccessIsDisabled(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	jftradeCheckTestError(t, err)
	server := newTestServer(t, store)
	server.auth.enforceAccess = true
	server.desktopAPIToken = "ephemeral-desktop-token"
	server.ApplySecuritySettings(SecuritySettings{})

	desktopRequest := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	desktopRequest.RemoteAddr = "192.0.2.20:12345"
	desktopRequest.Header.Set("Authorization", "Bearer ephemeral-desktop-token")
	desktopResponse := httptest.NewRecorder()
	server.ServeHTTP(desktopResponse, desktopRequest)
	if desktopResponse.Code != http.StatusOK {
		t.Fatalf("desktop response = %d %s", desktopResponse.Code, desktopResponse.Body.String())
	}

	browserRequest := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	browserRequest.RemoteAddr = "127.0.0.1:12345"
	browserResponse := httptest.NewRecorder()
	server.ServeHTTP(browserResponse, browserRequest)
	if browserResponse.Code != http.StatusForbidden || !strings.Contains(browserResponse.Body.String(), "WEB_ACCESS_DISABLED") {
		t.Fatalf("disabled browser response = %d %s", browserResponse.Code, browserResponse.Body.String())
	}
}

func TestWebSocketUsesCookieSessionWithoutDesktopToken(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	client := newCookieClient(t)
	loginWeb(t, client, srv.URL, testWebPassword)
	parsed, err := url.Parse(srv.URL)
	jftradeCheckTestError(t, err)
	headers := http.Header{"Origin": []string{srv.URL}}
	for _, cookie := range client.Jar.Cookies(parsed) {
		headers.Add("Cookie", cookie.String())
	}
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/api/v1/ws/live", headers)
	if response != nil && response.Body != nil {
		defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
	}
	if err != nil {
		t.Fatalf("WebSocket dial: %v", err)
	}
	defer func() { jftradeCheckTestError(t, conn.Close()) }()
}

func TestRemovedAuthTokenRouteReturnsNotFound(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	client := newCookieClient(t)
	loginWeb(t, client, srv.URL, testWebPassword)
	resp, err := client.Get(srv.URL + "/api/v1/auth/token")
	if err != nil {
		t.Fatalf("GET removed token route: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("removed token route status = %d, want 404", resp.StatusCode)
	}
}

func TestLegacyApplicationAdminKeyIsRemovedWithoutFollowingEnvironmentPath(t *testing.T) {
	runtimeDir := t.TempDir()
	legacyPath := filepath.Join(runtimeDir, "secrets", "admin.key")
	jftradeCheckTestError(t, os.MkdirAll(filepath.Dir(legacyPath), 0o700))
	jftradeCheckTestError(t, os.WriteFile(legacyPath, []byte("obsolete"), 0o600))
	externalPath := filepath.Join(t.TempDir(), "external-admin.key")
	jftradeCheckTestError(t, os.WriteFile(externalPath, []byte("must remain"), 0o600))
	t.Setenv("JFTRADE_ADMIN_KEY_FILE", externalPath)

	store, err := NewSettingsStore(filepath.Join(runtimeDir, "settings.json"))
	jftradeCheckTestError(t, err)
	server := NewServer(store)
	t.Cleanup(func() { jftradeCheckTestError(t, server.Close()) })
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy application key still exists: %v", err)
	}
	if data, err := os.ReadFile(externalPath); err != nil || string(data) != "must remain" {
		t.Fatalf("external environment path was changed: %q, %v", data, err)
	}
}

type webSessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	CSRFToken     string `json:"csrfToken"`
}

func newCookieClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	jftradeCheckTestError(t, err)
	return &http.Client{Jar: jar}
}

func requestWebLogin(t *testing.T, client *http.Client, baseURL string, password string) *http.Response {
	t.Helper()
	return requestWebLoginWithOrigin(t, client, baseURL, password, baseURL)
}

func requestWebLoginWithOrigin(t *testing.T, client *http.Client, baseURL string, password string, requestOrigin string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{"password": password})
	jftradeCheckTestError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/api/v1/auth/login", bytes.NewReader(body))
	jftradeCheckTestError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if requestOrigin != "" {
		req.Header.Set("Origin", requestOrigin)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Web login: %v", err)
	}
	return resp
}

func loginWeb(t *testing.T, client *http.Client, baseURL string, password string) string {
	t.Helper()
	resp := requestWebLogin(t, client, baseURL, password)
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	var envelope struct {
		Data webSessionResponse `json:"data"`
	}
	jftradeCheckTestError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	if !envelope.Data.Authenticated || envelope.Data.CSRFToken == "" {
		t.Fatalf("login response = %#v", envelope.Data)
	}
	return envelope.Data.CSRFToken
}

func readWebSession(t *testing.T, client *http.Client, baseURL string) webSessionResponse {
	t.Helper()
	resp, err := client.Get(baseURL + "/api/v1/auth/session")
	if err != nil {
		t.Fatalf("GET Web session: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	var envelope struct {
		Data webSessionResponse `json:"data"`
	}
	jftradeCheckTestError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	return envelope.Data
}

func webSecuritySettingsForPassword(t *testing.T, password string, public bool) SecuritySettings {
	t.Helper()
	hash, err := passwordhash.Hash(password)
	jftradeCheckTestError(t, err)
	return SecuritySettings{WebAccessEnabled: true, PublicAccessEnabled: public, PasswordHash: hash}
}

func assertErrorCode(t *testing.T, resp *http.Response, expected string) {
	t.Helper()
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	jftradeCheckTestError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	if envelope.Error == nil || envelope.Error.Code != expected {
		t.Fatalf("error envelope = %#v, want %s", envelope, expected)
	}
}
