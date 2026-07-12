package servercore

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebAccessSettingsDefaultToDesktopOnly(t *testing.T) {
	server, srv, _ := newDesktopSecuritySettingsServer(t)

	browserResponse, err := http.Get(srv.URL + "/api/v1/system/status")
	if err != nil {
		t.Fatalf("browser GET: %v", err)
	}
	defer func() { jftradeCheckTestError(t, browserResponse.Body.Close()) }()
	if browserResponse.StatusCode != http.StatusForbidden || !responseContains(t, browserResponse, "WEB_ACCESS_DISABLED") {
		t.Fatalf("browser response = %d", browserResponse.StatusCode)
	}

	desktopResponse := desktopSecurityRequest(t, server, srv.URL, http.MethodGet, "/api/v1/settings/security", nil)
	defer func() { jftradeCheckTestError(t, desktopResponse.Body.Close()) }()
	if desktopResponse.StatusCode != http.StatusOK {
		t.Fatalf("desktop GET = %d", desktopResponse.StatusCode)
	}
	body := readResponseBody(t, desktopResponse)
	if !strings.Contains(body, `"webAccessEnabled":false`) || !strings.Contains(body, `"passwordConfigured":false`) {
		t.Fatalf("default settings response = %s", body)
	}
}

func TestDesktopCanEnablePasswordProtectedWebWithoutExposingPassword(t *testing.T) {
	server, srv, settingsPath := newDesktopSecuritySettingsServer(t)
	password := "a memorable Web passphrase"
	payload := []byte(`{"webAccessEnabled":true,"publicAccessEnabled":false,"newPassword":"` + password + `"}`)
	enabled := desktopSecurityRequest(t, server, srv.URL, http.MethodPut, "/api/v1/settings/security", payload)
	if enabled.StatusCode != http.StatusOK {
		defer func() { jftradeCheckTestError(t, enabled.Body.Close()) }()
		t.Fatalf("enable Web = %d %s", enabled.StatusCode, readResponseBody(t, enabled))
	}
	enabledBody := readResponseBody(t, enabled)
	jftradeCheckTestError(t, enabled.Body.Close())
	for _, forbidden := range []string{password, "newPassword", "passwordHash", "adminAuthRequired"} {
		if strings.Contains(enabledBody, forbidden) {
			t.Fatalf("enable response leaked %q: %s", forbidden, enabledBody)
		}
	}
	if !strings.Contains(enabledBody, `"webAccessEnabled":true`) || !strings.Contains(enabledBody, `"passwordConfigured":true`) {
		t.Fatalf("enable response = %s", enabledBody)
	}

	persisted, err := os.ReadFile(settingsPath)
	jftradeCheckTestError(t, err)
	if strings.Contains(string(persisted), password) || strings.Contains(string(persisted), "adminAuthRequired") {
		t.Fatalf("settings file contains plaintext or legacy admin setting: %s", persisted)
	}
	if !strings.Contains(string(persisted), `"passwordHash": "$argon2id$`) {
		t.Fatalf("settings file lacks Argon2id verifier: %s", persisted)
	}

	unauthenticated, err := http.Get(srv.URL + "/api/v1/settings/security")
	jftradeCheckTestError(t, err)
	defer func() { jftradeCheckTestError(t, unauthenticated.Body.Close()) }()
	if unauthenticated.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated Web GET = %d, want 401", unauthenticated.StatusCode)
	}

	client := newCookieClient(t)
	loginWeb(t, client, srv.URL, password)
	authenticated, err := client.Get(srv.URL + "/api/v1/settings/security")
	jftradeCheckTestError(t, err)
	defer func() { jftradeCheckTestError(t, authenticated.Body.Close()) }()
	if authenticated.StatusCode != http.StatusOK {
		t.Fatalf("authenticated Web GET = %d", authenticated.StatusCode)
	}
	if body := readResponseBody(t, authenticated); strings.Contains(body, "passwordHash") || strings.Contains(body, password) {
		t.Fatalf("GET leaked password material: %s", body)
	}
}

func TestWebAccessCannotBeEnabledWithoutPassword(t *testing.T) {
	server, srv, _ := newDesktopSecuritySettingsServer(t)
	response := desktopSecurityRequest(t, server, srv.URL, http.MethodPut, "/api/v1/settings/security", []byte(`{"webAccessEnabled":true,"publicAccessEnabled":false}`))
	defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
	if response.StatusCode != http.StatusBadRequest || !responseContains(t, response, "INVALID_WEB_ACCESS_PASSWORD") {
		t.Fatalf("missing password response = %d", response.StatusCode)
	}
}

func TestBrowserSessionCannotChangeWebExposure(t *testing.T) {
	server, srv, _ := newDesktopSecuritySettingsServer(t)
	password := "a memorable Web passphrase"
	enable := desktopSecurityRequest(t, server, srv.URL, http.MethodPut, "/api/v1/settings/security", []byte(`{"webAccessEnabled":true,"newPassword":"`+password+`"}`))
	jftradeCheckTestError(t, enable.Body.Close())

	client := newCookieClient(t)
	csrf := loginWeb(t, client, srv.URL, password)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/settings/security", strings.NewReader(`{"webAccessEnabled":true,"publicAccessEnabled":true}`))
	jftradeCheckTestError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)
	req.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(req)
	jftradeCheckTestError(t, err)
	defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
	if response.StatusCode != http.StatusForbidden || !responseContains(t, response, "WEB_ACCESS_SETTINGS_DESKTOP_ONLY") {
		t.Fatalf("browser settings write = %d", response.StatusCode)
	}
}

func TestDisablingWebImmediatelyInvalidatesBrowserButNotDesktop(t *testing.T) {
	server, srv, _ := newDesktopSecuritySettingsServer(t)
	password := "a memorable Web passphrase"
	enable := desktopSecurityRequest(t, server, srv.URL, http.MethodPut, "/api/v1/settings/security", []byte(`{"webAccessEnabled":true,"newPassword":"`+password+`"}`))
	jftradeCheckTestError(t, enable.Body.Close())
	client := newCookieClient(t)
	loginWeb(t, client, srv.URL, password)

	disable := desktopSecurityRequest(t, server, srv.URL, http.MethodPut, "/api/v1/settings/security", []byte(`{"webAccessEnabled":false,"publicAccessEnabled":false}`))
	jftradeCheckTestError(t, disable.Body.Close())
	browser, err := client.Get(srv.URL + "/api/v1/system/status")
	jftradeCheckTestError(t, err)
	defer func() { jftradeCheckTestError(t, browser.Body.Close()) }()
	if browser.StatusCode != http.StatusForbidden {
		t.Fatalf("browser after disable = %d", browser.StatusCode)
	}
	desktop := desktopSecurityRequest(t, server, srv.URL, http.MethodGet, "/api/v1/system/status", nil)
	defer func() { jftradeCheckTestError(t, desktop.Body.Close()) }()
	if desktop.StatusCode != http.StatusOK {
		t.Fatalf("desktop after disable = %d", desktop.StatusCode)
	}
}

func newDesktopSecuritySettingsServer(t *testing.T) (*Server, *httptest.Server, string) {
	t.Helper()
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewSettingsStore(settingsPath)
	jftradeCheckTestError(t, err)
	server := newTestServer(t, store)
	server.auth.enforceAccess = true
	server.desktopAPIToken = "test-desktop-capability"
	server.ApplySecuritySettings(store.SecuritySettings())
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	server.auth.configureOrigins(srv.URL)
	return server, srv, settingsPath
}

func desktopSecurityRequest(t *testing.T, server *Server, baseURL string, method string, path string, body []byte) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, baseURL+path, reader)
	jftradeCheckTestError(t, err)
	req.Header.Set("Authorization", "Bearer "+server.desktopAPIToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("desktop %s %s: %v", method, path, err)
	}
	return response
}

func readResponseBody(t *testing.T, response *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	jftradeCheckTestError(t, err)
	return string(body)
}

func responseContains(t *testing.T, response *http.Response, expected string) bool {
	t.Helper()
	return strings.Contains(readResponseBody(t, response), expected)
}
