package jftradeapi

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

func TestRemoteRequestWithoutOriginDoesNotBypassAuthentication(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/adk", nil)
	req.RemoteAddr = "192.0.2.20:12345"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestForgedLocalhostOriginDoesNotAuthenticateRequest(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/adk/agents", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAdministratorBearerKeyAllowsSensitiveRequests(t *testing.T) {
	server, srv := newAuthenticatedSecurityServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/adk", nil)
	req.Header.Set("Authorization", "Bearer "+server.auth.key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestWrongAdministratorBearerKeyIsRejected(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/adk", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAdministratorCookieSessionRequiresCSRFForWrites(t *testing.T) {
	server, srv := newAuthenticatedSecurityServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := loginAdministrator(t, client, srv.URL, server.auth.key)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/adk/agents", bytes.NewReader([]byte(`{"name":"csrf-agent","status":"ENABLED"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST without CSRF: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("without CSRF status = %d, want 403", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/v1/adk/agents", bytes.NewReader([]byte(`{"name":"csrf-agent","status":"ENABLED"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST with CSRF: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		t.Fatalf("with CSRF status = %d", resp.StatusCode)
	}
}

func TestExpiredAdministratorSessionIsRejected(t *testing.T) {
	server, srv := newAuthenticatedSecurityServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginAdministrator(t, client, srv.URL, server.auth.key)

	server.auth.mu.Lock()
	for id, session := range server.auth.sessions {
		session.ExpiresAt = time.Now().Add(-time.Minute)
		server.auth.sessions[id] = session
	}
	server.auth.mu.Unlock()

	resp, err := client.Get(srv.URL + "/api/v1/adk")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAdministratorCookieSessionRejectsUntrustedReadOrigin(t *testing.T) {
	server, srv := newAuthenticatedSecurityServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginAdministrator(t, client, srv.URL, server.auth.key)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/adk", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestUnavailableAdministratorAuthFailsClosed(t *testing.T) {
	server, srv := newAuthenticatedSecurityServer(t)
	server.auth.unavailable = true

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/adk", nil)
	req.Header.Set("Authorization", "Bearer "+server.auth.key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
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
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/system/status", nil)
		req.Header.Set("Origin", test.origin)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != test.want {
			t.Fatalf("origin %q allowed as %q, want %q", test.origin, got, test.want)
		}
	}
}

func TestLegacyTokenEndpointIsGone(t *testing.T) {
	_, srv := newAuthenticatedSecurityServer(t)
	resp, err := http.Get(srv.URL + "/api/v1/auth/token")
	if err != nil {
		t.Fatalf("GET token: %v", err)
	}
	defer resp.Body.Close()
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
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/auth/login", bytes.NewReader([]byte(`{"key":"`+key+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", baseURL)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
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
