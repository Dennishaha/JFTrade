package apiserver

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
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
	if origins := wailsOriginsForDevServer("http://localhost:3003"); !containsString(origins, "wails://127.0.0.1:3003") {
		t.Fatalf("localhost development origins = %#v", origins)
	}
	for _, rawURL := range []string{"", "://invalid", "/relative/path"} {
		if origins := wailsOriginsForDevServer(rawURL); origins != nil {
			t.Fatalf("wails origins for %q = %#v, want nil", rawURL, origins)
		}
	}
	t.Setenv("JFTRADE_DESKTOP_MODE", "")
	if got := desktopFrontendDevURL(); got != "" {
		t.Fatalf("frontend dev URL outside desktop mode = %q", got)
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

func TestLoadFrontendFSPreservesUnavailableAndEmbeddedAssetSemantics(t *testing.T) {
	if got := loadFrontendFSWith(func() (fs.FS, bool, error) {
		return nil, false, errors.New("corrupt frontend archive")
	}); got != nil {
		t.Fatalf("failed asset loader = %#v, want nil", got)
	}
	if got := loadFrontendFSWith(func() (fs.FS, bool, error) {
		return nil, false, nil
	}); got != nil {
		t.Fatalf("unavailable asset loader = %#v, want nil", got)
	}
	assets := fstest.MapFS{"index.html": {Data: []byte("<main>JFTrade</main>")}}
	if got := loadFrontendFSWith(func() (fs.FS, bool, error) {
		return assets, true, nil
	}); got == nil {
		t.Fatal("available embedded assets were discarded")
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

	if err := waitDesktopAPIReady(t.Context(), "http://127.0.0.1:1", ""); err == nil || !strings.Contains(err.Error(), "did not become ready") {
		t.Fatalf("unreachable API error = %v", err)
	}
	if err := waitDesktopAPIReady(t.Context(), "http://[::1", ""); err == nil {
		t.Fatal("malformed readiness URL succeeded")
	}
}

func TestStartDesktopWithConfigClosesSidecarWhenReadinessTargetFails(t *testing.T) {
	previousTimeout := desktopAPIReadyTimeout
	desktopAPIReadyTimeout = 25 * time.Millisecond
	t.Cleanup(func() { desktopAPIReadyTimeout = previousTimeout })

	runtimeDir := t.TempDir()
	apiBind := freeTCPAddr(t)
	runtimeConfig := DesktopRuntimeConfig{
		Defaults: jfsettings.LaunchDefaults{
			APIBind:        apiBind,
			SettingsPath:   filepath.Join(runtimeDir, "settings.json"),
			BacktestDBPath: filepath.Join(runtimeDir, "backtest.db"),
		},
		SettingsPath: filepath.Join(runtimeDir, "settings.json"),
		BacktestPath: filepath.Join(runtimeDir, "backtest.db"),
		APIBind:      apiBind,
		APIBaseURL:   "http://127.0.0.1:1",
	}
	shutdown, err := StartDesktopWithConfig(t.Context(), runtimeConfig, nil)
	if shutdown != nil || err == nil || !strings.Contains(err.Error(), "did not become ready") {
		if shutdown != nil {
			_ = shutdown(context.Background())
		}
		t.Fatalf("shutdown=%v err=%v", shutdown != nil, err)
	}
}
