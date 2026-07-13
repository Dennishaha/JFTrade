package apiserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/lifecycle"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

type incompatibleLifecycleSettingsStore struct{}

func (incompatibleLifecycleSettingsStore) EnsureBootstrapFile(jfsettings.LaunchDefaults) error {
	return nil
}
func (incompatibleLifecycleSettingsStore) Integration() jfsettings.BrokerIntegration {
	return jfsettings.BrokerIntegration{}
}
func (incompatibleLifecycleSettingsStore) SavedIntegration() *jfsettings.BrokerIntegration {
	return nil
}
func (incompatibleLifecycleSettingsStore) InterfaceSettings(jfsettings.LaunchDefaults) jfsettings.InterfaceSettings {
	return jfsettings.InterfaceSettings{}
}
func (incompatibleLifecycleSettingsStore) SecuritySettings() jfsettings.SecuritySettings {
	return jfsettings.SecuritySettings{}
}

var _ lifecycle.SettingsStore = incompatibleLifecycleSettingsStore{}

func TestAPIServerHelperBoundaryCoverage(t *testing.T) {
	if err := validateDesktopAPIBind("127.0.0.1:3008", "", false); err == nil || !strings.Contains(err.Error(), "browser-accessible") {
		t.Fatalf("empty base URL error = %v", err)
	}
	if err := validateDesktopAPIBind("invalid", "http://invalid", false); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("invalid bind error = %v", err)
	}

	t.Setenv("JFTRADE_DESKTOP_MODE", "1")
	t.Setenv("FRONTEND_DEVSERVER_URL", "")
	if got := desktopFrontendDevURL(); got != "http://127.0.0.1:3003" {
		t.Fatalf("default desktop frontend URL = %q", got)
	}
	if got := wailsOriginsForDevServer("https://example.com"); len(got) != 1 || got[0] != "wails://example.com" {
		t.Fatalf("origin without development port = %#v", got)
	}

	if _, err := newHandler(incompatibleLifecycleSettingsStore{}); err == nil || !strings.Contains(err.Error(), "unexpected settings store") {
		t.Fatalf("incompatible settings store error = %v", err)
	}

	t.Setenv("JFTRADE_API_DISABLED", "")
	if !shouldStartForArgs([]string{"serve-api"}) || shouldStartForArgs([]string{"unknown"}) || shouldStartForArgs([]string{"--help", "api"}) {
		t.Fatal("API argument classification mismatch")
	}

	t.Setenv("JFTRADE_API_BIND", "127.0.0.1:0")
	if shutdown, err := StartDesktop(t.Context(), nil); err == nil || shutdown != nil {
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		t.Fatalf("StartDesktop with ephemeral bind = shutdown %v err %v", shutdown != nil, err)
	}
}

func TestWaitDesktopAPIReadyCoversAuthorizationAndTimeout(t *testing.T) {
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer desktop-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(ready.Close)
	if err := waitDesktopAPIReady(t.Context(), ready.URL+"/", " desktop-token "); err != nil {
		t.Fatalf("ready API: %v", err)
	}
	if err := waitDesktopAPIReady(t.Context(), " ", ""); err == nil {
		t.Fatal("empty API base URL succeeded")
	}

	unready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(unready.Close)
	previousTimeout := desktopAPIReadyTimeout
	desktopAPIReadyTimeout = 25 * time.Millisecond
	t.Cleanup(func() { desktopAPIReadyTimeout = previousTimeout })
	if err := waitDesktopAPIReady(t.Context(), unready.URL, ""); err == nil || !strings.Contains(err.Error(), "readiness status 503") {
		t.Fatalf("unready API error = %v", err)
	}
}
