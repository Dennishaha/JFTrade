package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestSecuritySettingsDefaultDoesNotRequireAuthentication(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	t.Cleanup(func() {
		_ = server.Close()
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/settings/security")
	if err != nil {
		t.Fatalf("GET security settings: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatal("default security settings unexpectedly require authentication")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET security settings status = %d, want 200", resp.StatusCode)
	}
}

func TestSecuritySettingsToggleAuthenticationImmediately(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := NewServer(store)
	t.Cleanup(func() {
		_ = server.Close()
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	saveSecuritySettings(t, srv.URL, "", true)

	resp, err := http.Get(srv.URL + "/api/v1/settings/security")
	if err != nil {
		t.Fatalf("GET security settings while auth enabled: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET security settings while auth enabled status = %d, want 401", resp.StatusCode)
	}

	saveSecuritySettings(t, srv.URL, server.auth.key, false)

	resp, err = http.Get(srv.URL + "/api/v1/settings/security")
	if err != nil {
		t.Fatalf("GET security settings after disable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatal("security settings still require authentication after disabling auth")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET security settings after disable status = %d, want 200", resp.StatusCode)
	}
}

func saveSecuritySettings(t *testing.T, baseURL string, bearerKey string, required bool) {
	t.Helper()
	body, _ := json.Marshal(SecuritySettings{AdminAuthRequired: required})
	req, err := http.NewRequest(http.MethodPut, baseURL+"/api/v1/settings/security", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest security settings: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearerKey != "" {
		req.Header.Set("Authorization", "Bearer "+bearerKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT security settings: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT security settings status = %d, want 200", resp.StatusCode)
	}
}
