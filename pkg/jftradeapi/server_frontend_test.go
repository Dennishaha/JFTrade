package jftradeapi

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerServesFrontendAssetsAndSPAFallback(t *testing.T) {
	frontendDir := t.TempDir()
	assetsDir := filepath.Join(frontendDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(frontendDir, "index.html"), []byte("<html><body>JFTrade UI</body></html>"), 0o644); err != nil {
		t.Fatalf("WriteFile index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("console.log('jftrade');"), 0o644); err != nil {
		t.Fatalf("WriteFile app.js: %v", err)
	}

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newServerWithFrontend(store, newFrontendServer(os.DirFS(frontendDir)))
	if server.auth != nil {
		server.auth.enabled = false
	}
	t.Cleanup(func() { _ = server.Close() })
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	assertBodyContains := func(path string, accept string, want string) {
		req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("NewRequest %s: %v", path, err)
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d", path, resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("ReadAll %s: %v", path, err)
		}
		if !strings.Contains(string(body), want) {
			t.Fatalf("GET %s body = %q, want substring %q", path, string(body), want)
		}
	}

	assertBodyContains("/", "text/html", "JFTrade UI")
	assertBodyContains("/strategy", "text/html", "JFTrade UI")
	assertBodyContains("/assets/app.js", "application/javascript", "console.log('jftrade')")

	apiResp, err := http.Get(srv.URL + "/api/v1/not-found")
	if err != nil {
		t.Fatalf("GET api not found: %v", err)
	}
	defer apiResp.Body.Close()
	if apiResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET api not found status = %d", apiResp.StatusCode)
	}
	apiBody, err := io.ReadAll(apiResp.Body)
	if err != nil {
		t.Fatalf("ReadAll api not found: %v", err)
	}
	if !strings.Contains(string(apiBody), "NOT_FOUND") {
		t.Fatalf("GET api not found body = %q, want JSON error", string(apiBody))
	}
}

func TestFrontendServerServesRuntimeConfigScript(t *testing.T) {
	frontendDir := t.TempDir()
	srv := httptest.NewServer(newFrontendServerWithRuntimeConfig(os.DirFS(frontendDir), "http://127.0.0.1:6699"))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/runtime-config.js")
	if err != nil {
		t.Fatalf("GET runtime config: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("runtime config status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll runtime config: %v", err)
	}
	if !strings.Contains(string(body), "http://127.0.0.1:6699") {
		t.Fatalf("runtime config body = %q", string(body))
	}
}

func TestStartForRunArgsInitializesRuntimeLayout(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "var", "jftrade-api")
	settingsPath := filepath.Join(runtimeDir, "settings.json")
	backtestDBPath := filepath.Join(runtimeDir, "backtest.db")

	t.Setenv("JFTRADE_SETTINGS_PATH", settingsPath)
	t.Setenv("JFTRADE_BACKTEST_DB", backtestDBPath)
	t.Setenv("JFTRADE_API_BIND", "127.0.0.1:0")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdown, err := StartForRunArgs(ctx, []string{"api"})
	if err != nil {
		t.Fatalf("StartForRunArgs: %v", err)
	}
	defer func() {
		_ = shutdown(context.Background())
	}()

	for _, path := range []string{
		runtimeDir,
		settingsPath,
		deriveStrategyPluginTargetDir(settingsPath),
		deriveStrategyRuntimeDBPath(settingsPath),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	if _, err := os.Stat(filepath.Dir(backtestDBPath)); err != nil {
		t.Fatalf("expected backtest directory to exist: %v", err)
	}
	if _, err := os.Stat(deriveStrategyDesignPath(settingsPath)); err == nil {
		t.Fatalf("strategy design definition file should not be eagerly created")
	}
}

func TestStartForRunArgsUsesInterfaceSettingsForAPIBind(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	apiBind := listener.Addr().String()
	_ = listener.Close()

	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	settingsBody := `{
  "interfaces": {
    "apiBind": "` + apiBind + `"
  }
}`
	if err := os.WriteFile(settingsPath, []byte(settingsBody), 0o600); err != nil {
		t.Fatalf("WriteFile settings: %v", err)
	}
	t.Setenv("JFTRADE_SETTINGS_PATH", settingsPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdown, err := StartForRunArgs(ctx, []string{"api"})
	if err != nil {
		t.Fatalf("StartForRunArgs: %v", err)
	}
	defer func() {
		_ = shutdown(context.Background())
	}()

	statusURL := "http://" + apiBind + "/api/v1/system/status"
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := http.Get(statusURL)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET status code = %d", resp.StatusCode)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("GET %s: %v", statusURL, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestResolveGUIAPIBaseURLFollowsRuntimeAPIBindOverride(t *testing.T) {
	t.Setenv("JFTRADE_GUI_API_BASE_URL", "")

	settings := InterfaceSettings{
		APIBind:       defaultReleaseAPIBind,
		GUIAPIBaseURL: apiBaseURLForBind(defaultReleaseAPIBind),
	}

	got := resolveGUIAPIBaseURL(settings, "127.0.0.1:16699")
	if got != "http://127.0.0.1:16699" {
		t.Fatalf("resolveGUIAPIBaseURL() = %q", got)
	}
}

func TestResolveGUIAPIBaseURLPreservesExplicitSetting(t *testing.T) {
	t.Setenv("JFTRADE_GUI_API_BASE_URL", "")

	settings := InterfaceSettings{
		APIBind:       defaultReleaseAPIBind,
		GUIAPIBaseURL: "http://127.0.0.1:18080",
	}

	got := resolveGUIAPIBaseURL(settings, "127.0.0.1:16699")
	if got != "http://127.0.0.1:18080" {
		t.Fatalf("resolveGUIAPIBaseURL() = %q", got)
	}
}

func TestResolveGUIRuntimeAPIBaseURLDefaultsToSameOrigin(t *testing.T) {
	t.Setenv("JFTRADE_GUI_API_BASE_URL", "")

	settings := InterfaceSettings{
		APIBind:       defaultReleaseAPIBind,
		GUIAPIBaseURL: apiBaseURLForBind(defaultReleaseAPIBind),
	}

	got := resolveGUIRuntimeAPIBaseURL(settings, "127.0.0.1:16699")
	if got != "" {
		t.Fatalf("resolveGUIRuntimeAPIBaseURL() = %q, want same-origin empty value", got)
	}
}

func TestResolveGUIRuntimeAPIBaseURLPreservesExplicitEnv(t *testing.T) {
	t.Setenv("JFTRADE_GUI_API_BASE_URL", "http://127.0.0.1:18080")

	settings := InterfaceSettings{
		APIBind:       defaultReleaseAPIBind,
		GUIAPIBaseURL: apiBaseURLForBind(defaultReleaseAPIBind),
	}

	got := resolveGUIRuntimeAPIBaseURL(settings, "127.0.0.1:16699")
	if got != "http://127.0.0.1:18080" {
		t.Fatalf("resolveGUIRuntimeAPIBaseURL() = %q", got)
	}
}
