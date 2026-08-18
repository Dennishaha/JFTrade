package webaccess_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
)

const securityTestWebPassword = "correct horse battery staple"

var securityTestHashCache sync.Map

func securityTestHash(t *testing.T, password string) string {
	t.Helper()
	if cached, ok := securityTestHashCache.Load(password); ok {
		return cached.(string)
	}
	hash, err := passwordhash.Hash(password)
	if err != nil {
		t.Fatalf("passwordhash.Hash: %v", err)
	}
	securityTestHashCache.Store(password, hash)
	return hash
}

func securityTestSettings(t *testing.T, public bool) jfsettings.SecuritySettings {
	t.Helper()
	return jfsettings.SecuritySettings{
		WebAccessEnabled:    true,
		PublicAccessEnabled: public,
		PasswordHash:        securityTestHash(t, securityTestWebPassword),
	}
}

func securityTestSettingsForPassword(t *testing.T, password string, public bool) jfsettings.SecuritySettings {
	t.Helper()
	return jfsettings.SecuritySettings{WebAccessEnabled: true, PublicAccessEnabled: public, PasswordHash: securityTestHash(t, password)}
}

// forceWebAccessTestMarketDataProvider keeps web-access integration servers on
// the in-process Futu data plane. The default provider is AKShare, whose
// embedded Python sidecar would otherwise be cold-started once per test case.
func forceWebAccessTestMarketDataProvider(t *testing.T, store *servercore.SettingsStore) {
	t.Helper()
	if store == nil {
		return
	}
	if err := store.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderFutu); err != nil {
		t.Fatalf("SaveActiveMarketDataProvider: %v", err)
	}
}

func securityTestCheckError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func newSecurityServer(t *testing.T) servercore.SidecarHandler {
	t.Helper()
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	forceWebAccessTestMarketDataProvider(t, store)
	handler := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{})
	handler.ApplySecuritySettings(securityTestSettings(t, false))
	t.Cleanup(func() {
		securityTestCheckError(t, handler.Close())
	})
	return handler
}

func newSecurityHTTPServer(t *testing.T) (servercore.SidecarHandler, *httptest.Server) {
	t.Helper()
	handler := newSecurityServer(t)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	handler.ConfigureAuthOrigins(srv.URL)
	return handler, srv
}

type securityWebSessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	CSRFToken     string `json:"csrfToken"`
}

func securityCookieClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	securityTestCheckError(t, err)
	return &http.Client{Jar: jar}
}

func securityRequestWebLoginWithOrigin(t *testing.T, client *http.Client, baseURL string, password string, requestOrigin string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{"password": password})
	securityTestCheckError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/api/v1/auth/login", bytes.NewReader(body))
	securityTestCheckError(t, err)
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

func securityRequestWebLogin(t *testing.T, client *http.Client, baseURL string, password string) *http.Response {
	t.Helper()
	return securityRequestWebLoginWithOrigin(t, client, baseURL, password, baseURL)
}

func securityLoginWeb(t *testing.T, client *http.Client, baseURL string, password string) string {
	t.Helper()
	resp := securityRequestWebLogin(t, client, baseURL, password)
	defer func() { securityTestCheckError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	var envelope struct {
		Data securityWebSessionResponse `json:"data"`
	}
	securityTestCheckError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	if !envelope.Data.Authenticated || envelope.Data.CSRFToken == "" {
		t.Fatalf("login response = %#v", envelope.Data)
	}
	return envelope.Data.CSRFToken
}

func securityReadWebSession(t *testing.T, client *http.Client, baseURL string) securityWebSessionResponse {
	t.Helper()
	resp, err := client.Get(baseURL + "/api/v1/auth/session")
	if err != nil {
		t.Fatalf("GET Web session: %v", err)
	}
	defer func() { securityTestCheckError(t, resp.Body.Close()) }()
	var envelope struct {
		Data securityWebSessionResponse `json:"data"`
	}
	securityTestCheckError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	return envelope.Data
}

func securityAssertErrorCode(t *testing.T, resp *http.Response, expected string) {
	t.Helper()
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	securityTestCheckError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	if envelope.Error == nil || envelope.Error.Code != expected {
		t.Fatalf("error envelope = %#v, want %s", envelope, expected)
	}
}

func TestWebPasswordIsRequiredForProtectedAPI(t *testing.T) {
	_, srv := newSecurityHTTPServer(t)
	resp, err := http.Get(srv.URL + "/api/v1/system/status")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	defer func() { securityTestCheckError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	securityAssertErrorCode(t, resp, "WEB_AUTH_REQUIRED")
}

func TestBrowserNavigationGetsFriendlyDisabledWebPage(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	securityTestCheckError(t, err)
	forceWebAccessTestMarketDataProvider(t, store)
	handler := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{})
	t.Cleanup(func() { securityTestCheckError(t, handler.Close()) })
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)
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
	handler, _ := newSecurityHTTPServer(t)
	payload, err := json.Marshal(map[string]string{"password": securityTestWebPassword})
	securityTestCheckError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	request.Host = "trade.example"
	request.RemoteAddr = "127.0.0.1:42000"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://trade.example")
	request.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	response := recorder.Result()
	defer func() { securityTestCheckError(t, response.Body.Close()) }()
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie = %#v, want Secure HttpOnly SameSite=Strict", cookies)
	}
	if got := response.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestNetworkClientCannotSpoofHTTPSProxyScheme(t *testing.T) {
	handler, _ := newSecurityHTTPServer(t)
	handler.ApplySecuritySettings(securityTestSettings(t, true))
	payload, err := json.Marshal(map[string]string{"password": securityTestWebPassword})
	securityTestCheckError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	request.Host = "trade.example"
	request.RemoteAddr = "192.0.2.20:42000"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://trade.example")
	request.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSameHostProxyCannotBypassPublicAccessSetting(t *testing.T) {
	handler, _ := newSecurityHTTPServer(t)
	payload, err := json.Marshal(map[string]string{"password": securityTestWebPassword})
	securityTestCheckError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	request.Host = "trade.example"
	request.RemoteAddr = "127.0.0.1:42000"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://trade.example")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "192.0.2.20")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"REMOTE_WEB_ACCESS_DISABLED"`) {
		t.Fatalf("body = %s, want REMOTE_WEB_ACCESS_DISABLED", recorder.Body.String())
	}
}

func TestAdminBearerMechanismNoLongerAuthenticates(t *testing.T) {
	_, srv := newSecurityHTTPServer(t)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/system/status", nil)
	securityTestCheckError(t, err)
	req.Header.Set("Authorization", "Bearer legacy-admin-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET with legacy bearer: %v", err)
	}
	defer func() { securityTestCheckError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestWebPasswordSessionSupportsReadAndCSRFProtectedWrite(t *testing.T) {
	_, srv := newSecurityHTTPServer(t)
	client := securityCookieClient(t)
	csrf := securityLoginWeb(t, client, srv.URL, securityTestWebPassword)

	readResp, err := client.Get(srv.URL + "/api/v1/system/status")
	if err != nil {
		t.Fatalf("authenticated GET: %v", err)
	}
	securityTestCheckError(t, readResp.Body.Close())
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated GET status = %d", readResp.StatusCode)
	}

	write := func(token string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/adk/agents", bytes.NewReader([]byte(`{"name":"csrf-agent","status":"ENABLED"}`)))
		securityTestCheckError(t, err)
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
	securityTestCheckError(t, withoutCSRF.Body.Close())
	if withoutCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("without CSRF status = %d, want 403", withoutCSRF.StatusCode)
	}
	withCSRF := write(csrf)
	defer func() { securityTestCheckError(t, withCSRF.Body.Close()) }()
	if withCSRF.StatusCode == http.StatusUnauthorized || withCSRF.StatusCode == http.StatusForbidden {
		t.Fatalf("with CSRF status = %d", withCSRF.StatusCode)
	}
}

func TestWebLoginRejectsWrongPasswordAndRateLimits(t *testing.T) {
	_, srv := newSecurityHTTPServer(t)
	client := securityCookieClient(t)
	for attempt := range 8 {
		resp := securityRequestWebLogin(t, client, srv.URL, "wrong password")
		securityTestCheckError(t, resp.Body.Close())
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", attempt+1, resp.StatusCode)
		}
	}
	limited := securityRequestWebLogin(t, client, srv.URL, securityTestWebPassword)
	defer func() { securityTestCheckError(t, limited.Body.Close()) }()
	if limited.StatusCode != http.StatusTooManyRequests || limited.Header.Get("Retry-After") == "" {
		t.Fatalf("limited response = %d retry-after=%q", limited.StatusCode, limited.Header.Get("Retry-After"))
	}
}

func TestPasswordChangesInvalidateWebSessions(t *testing.T) {
	handler, srv := newSecurityHTTPServer(t)
	client := securityCookieClient(t)
	securityLoginWeb(t, client, srv.URL, securityTestWebPassword)
	handler.ApplySecuritySettings(securityTestSettingsForPassword(t, "a replacement password phrase", false))
	if session := securityReadWebSession(t, client, srv.URL); session.Authenticated {
		t.Fatal("password change did not invalidate existing Web sessions")
	}
}

func TestProductionWebDoesNotTrustDevelopmentOrigin(t *testing.T) {
	_, srv := newSecurityHTTPServer(t)
	payload, err := json.Marshal(map[string]string{"password": securityTestWebPassword})
	securityTestCheckError(t, err)
	request, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login", bytes.NewReader(payload))
	securityTestCheckError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:3003")
	response, err := http.DefaultClient.Do(request)
	securityTestCheckError(t, err)
	defer func() { securityTestCheckError(t, response.Body.Close()) }()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.StatusCode)
	}
	securityAssertErrorCode(t, response, "ORIGIN_FORBIDDEN")
}

func TestWebLogoutClearsSessionCookie(t *testing.T) {
	_, srv := newSecurityHTTPServer(t)
	client := securityCookieClient(t)
	csrf := securityLoginWeb(t, client, srv.URL, securityTestWebPassword)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/auth/logout", nil)
	securityTestCheckError(t, err)
	req.Header.Set("Origin", srv.URL)
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	securityTestCheckError(t, resp.Body.Close())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d", resp.StatusCode)
	}
	if session := securityReadWebSession(t, client, srv.URL); session.Authenticated {
		t.Fatal("logout left session authenticated")
	}
}

func TestWebLoginCookieIsHttpOnlyAndSameSiteStrict(t *testing.T) {
	_, srv := newSecurityHTTPServer(t)
	resp := securityRequestWebLogin(t, http.DefaultClient, srv.URL, securityTestWebPassword)
	defer func() { securityTestCheckError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	var found bool
	for _, cookie := range resp.Cookies() {
		if cookie.Name != "jftrade_web_session" {
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
	_, srv := newSecurityHTTPServer(t)
	disallowed := securityRequestWebLoginWithOrigin(t, http.DefaultClient, srv.URL, securityTestWebPassword, "https://evil.example.com")
	securityTestCheckError(t, disallowed.Body.Close())
	if disallowed.StatusCode != http.StatusForbidden {
		t.Fatalf("disallowed origin status = %d", disallowed.StatusCode)
	}

	allowed := securityRequestWebLoginWithOrigin(t, http.DefaultClient, srv.URL, securityTestWebPassword, srv.URL)
	defer func() { securityTestCheckError(t, allowed.Body.Close()) }()
	if allowed.StatusCode != http.StatusOK {
		t.Fatalf("same origin status = %d", allowed.StatusCode)
	}
}

func TestLoopbackPolicyBlocksRemoteBrowserUntilExplicitlyEnabled(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	securityTestCheckError(t, err)
	forceWebAccessTestMarketDataProvider(t, store)
	handler := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{})
	t.Cleanup(func() { securityTestCheckError(t, handler.Close()) })
	handler.ApplySecuritySettings(securityTestSettings(t, false))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.20:12345"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "REMOTE_WEB_ACCESS_DISABLED") {
		t.Fatalf("private access response = %d %s", recorder.Code, recorder.Body.String())
	}

	handler.ApplySecuritySettings(securityTestSettings(t, true))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusForbidden {
		t.Fatalf("explicit network access still blocked: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestDesktopCapabilityStaysPasswordlessWhenWebAccessIsDisabled(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	securityTestCheckError(t, err)
	forceWebAccessTestMarketDataProvider(t, store)
	handler := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode:     true,
		DesktopAPIToken: "ephemeral-desktop-token",
	})
	t.Cleanup(func() { securityTestCheckError(t, handler.Close()) })
	handler.ApplySecuritySettings(jfsettings.SecuritySettings{})

	desktopRequest := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	desktopRequest.RemoteAddr = "192.0.2.20:12345"
	desktopRequest.Header.Set("Authorization", "Bearer ephemeral-desktop-token")
	desktopResponse := httptest.NewRecorder()
	handler.ServeHTTP(desktopResponse, desktopRequest)
	if desktopResponse.Code != http.StatusOK {
		t.Fatalf("desktop response = %d %s", desktopResponse.Code, desktopResponse.Body.String())
	}

	browserRequest := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	browserRequest.RemoteAddr = "127.0.0.1:12345"
	browserResponse := httptest.NewRecorder()
	handler.WebAccessHandler().ServeHTTP(browserResponse, browserRequest)
	if browserResponse.Code != http.StatusForbidden || !strings.Contains(browserResponse.Body.String(), "WEB_ACCESS_DISABLED") {
		t.Fatalf("disabled browser response = %d %s", browserResponse.Code, browserResponse.Body.String())
	}
}

func TestWebSocketUsesCookieSessionWithoutDesktopToken(t *testing.T) {
	_, srv := newSecurityHTTPServer(t)
	client := securityCookieClient(t)
	securityLoginWeb(t, client, srv.URL, securityTestWebPassword)
	parsed, err := url.Parse(srv.URL)
	securityTestCheckError(t, err)
	headers := http.Header{"Origin": []string{srv.URL}}
	for _, cookie := range client.Jar.Cookies(parsed) {
		headers.Add("Cookie", cookie.String())
	}
	conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/api/v1/ws/live", headers)
	if response != nil && response.Body != nil {
		defer func() { securityTestCheckError(t, response.Body.Close()) }()
	}
	if err != nil {
		t.Fatalf("WebSocket dial: %v", err)
	}
	defer func() { securityTestCheckError(t, conn.Close()) }()
}

func TestRemovedAuthTokenRouteReturnsNotFound(t *testing.T) {
	_, srv := newSecurityHTTPServer(t)
	client := securityCookieClient(t)
	securityLoginWeb(t, client, srv.URL, securityTestWebPassword)
	resp, err := client.Get(srv.URL + "/api/v1/auth/token")
	if err != nil {
		t.Fatalf("GET removed token route: %v", err)
	}
	defer func() { securityTestCheckError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("removed token route status = %d, want 404", resp.StatusCode)
	}
}
