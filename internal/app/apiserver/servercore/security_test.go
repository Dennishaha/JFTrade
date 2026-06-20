package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func newAuthenticatedSecurityServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	server.auth.enabled = true
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	server.auth.configureOrigins(srv.URL, "http://localhost:5173")
	return server, srv
}

func assertErrorEnvelopeTimestamp(t *testing.T, resp *http.Response) {
	t.Helper()
	var envelope struct {
		OK        bool   `json:"ok"`
		Timestamp string `json:"timestamp"`
		Error     *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.OK || envelope.Error == nil {
		t.Fatalf("unexpected error envelope: %+v", envelope)
	}
	if envelope.Timestamp == "" {
		t.Fatal("error envelope timestamp is empty")
	}
	if _, err := time.Parse(time.RFC3339Nano, envelope.Timestamp); err != nil {
		t.Fatalf("timestamp %q is not RFC3339Nano: %v", envelope.Timestamp, err)
	}
}

func TestRemoteRequestWithoutOriginDoesNotBypassAuthentication(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	req, jftradeErr1 := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/adk", nil)
	jftradeCheckTestError(t, jftradeErr1)
	req.RemoteAddr = "192.0.2.20:12345"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestForgedLocalhostOriginDoesNotAuthenticateRequest(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	req, jftradeErr2 := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/adk/agents", bytes.NewReader([]byte(`{}`)))
	jftradeCheckTestError(t, jftradeErr2)
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAdministratorBearerKeyAllowsSensitiveRequests(t *testing.T) {
	server, srv := newAuthenticatedSecurityServer(t)
	req, jftradeErr3 := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/adk", nil)
	jftradeCheckTestError(t, jftradeErr3)
	req.Header.Set("Authorization", "Bearer "+server.auth.key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestWrongAdministratorBearerKeyIsRejected(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	req, jftradeErr4 := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/adk", nil)
	jftradeCheckTestError(t, jftradeErr4)
	req.Header.Set("Authorization", "Bearer wrong-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAdministratorCookieSessionRequiresCSRFForWrites(t *testing.T) {
	server, srv := newAuthenticatedSecurityServer(t)
	jar, jftradeErr5 := cookiejar.New(nil)
	jftradeCheckTestError(t, jftradeErr5)
	client := &http.Client{Jar: jar}
	csrf := loginAdministrator(t, client, srv.URL, server.auth.key)

	req, jftradeErr6 := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/adk/agents", bytes.NewReader([]byte(`{"name":"csrf-agent","status":"ENABLED"}`)))
	jftradeCheckTestError(t, jftradeErr6)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST without CSRF: %v", err)
	}
	jftradeCheckTestError(t, resp.Body.Close())
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("without CSRF status = %d, want 403", resp.StatusCode)
	}

	req, jftradeErr7 := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/adk/agents", bytes.NewReader([]byte(`{"name":"csrf-agent","status":"ENABLED"}`)))
	jftradeCheckTestError(t, jftradeErr7)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST with CSRF: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		t.Fatalf("with CSRF status = %d", resp.StatusCode)
	}
}

func TestExpiredAdministratorSessionIsRejected(t *testing.T) {
	server, srv := newAuthenticatedSecurityServer(t)
	jar, jftradeErr8 := cookiejar.New(nil)
	jftradeCheckTestError(t, jftradeErr8)
	client := &http.Client{Jar: jar}
	loginAdministrator(t, client, srv.URL, server.auth.key)

	server.auth.mu.Lock()
	for id, session := range server.auth.sessions {
		session.ExpiresAt = time.Now().Add(-time.Minute)
		server.auth.sessions[id] = session
	}
	server.auth.mu.Unlock()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/adk", nil)
	jftradeCheckTestError(t, err)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAdministratorCookieSessionRejectsUntrustedOrigin(t *testing.T) {
	server, srv := newAuthenticatedSecurityServer(t)
	jar, jftradeErr9 := cookiejar.New(nil)
	jftradeCheckTestError(t, jftradeErr9)
	client := &http.Client{Jar: jar}
	csrf := loginAdministrator(t, client, srv.URL, server.auth.key)

	req, jftradeErr10 := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/adk", nil)
	jftradeCheckTestError(t, jftradeErr10)
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("read status = %d, want 403", resp.StatusCode)
	}
	assertErrorEnvelopeTimestamp(t, resp)

	req, jftradeErr11 := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/adk/agents", bytes.NewReader([]byte(`{"name":"csrf-agent","status":"ENABLED"}`)))
	jftradeCheckTestError(t, jftradeErr11)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("write status = %d, want 403", resp.StatusCode)
	}
}

func TestUnavailableAdministratorAuthFailsClosed(t *testing.T) {
	server, srv := newAuthenticatedSecurityServer(t)
	server.auth.unavailable = true

	req, jftradeErr12 := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/adk", nil)
	jftradeCheckTestError(t, jftradeErr12)
	req.Header.Set("Authorization", "Bearer "+server.auth.key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestCORSOnlyReflectsConfiguredOrigins(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	for _, test := range []struct {
		origin string
		want   string
	}{
		{origin: "http://localhost:5173", want: "http://localhost:5173"},
		{origin: "http://localhost:5174", want: "http://localhost:5174"},
		{origin: "https://evil.example.com", want: ""},
	} {
		req, jftradeErr13 := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/system/status", nil)
		jftradeCheckTestError(t, jftradeErr13)
		req.Header.Set("Origin", test.origin)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		jftradeCheckTestError(t, resp.Body.Close())
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != test.want {
			t.Fatalf("origin %q allowed as %q, want %q", test.origin, got, test.want)
		}
	}
}

func TestLegacyTokenEndpointIsGone(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/auth/token")
	if err != nil {
		t.Fatalf("GET token: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("status = %d, want 410", resp.StatusCode)
	}
}

func TestAdministratorKeyPersistsAcrossAuthInstances(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	first, err := newAdminAuth(settingsPath)
	if err != nil {
		t.Fatalf("newAdminAuth first: %v", err)
	}
	second, err := newAdminAuth(settingsPath)
	if err != nil {
		t.Fatalf("newAdminAuth second: %v", err)
	}
	if first.key != second.key {
		t.Fatal("administrator key did not persist")
	}
}

func loginAdministrator(t *testing.T, client *http.Client, baseURL string, key string) string {
	t.Helper()
	req, jftradeErr14 := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/api/v1/auth/login", bytes.NewReader([]byte(`{"key":"`+key+`"}`)))
	jftradeCheckTestError(t, jftradeErr14)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", baseURL)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
	var body struct {
		Data struct {
			CSRFToken string `json:"csrfToken"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if body.Data.CSRFToken == "" {
		t.Fatal("login response did not include a CSRF token")
	}
	return body.Data.CSRFToken
}
