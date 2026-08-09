package servercoretest

import (
	"context"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/webaccess"
	"github.com/jftrade/jftrade-main/internal/frontendassets"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/security/passwordhash"
	strategystore "github.com/jftrade/jftrade-main/internal/store/strategy"
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

	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	handler := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		FrontendFS:  os.DirFS(frontendDir),
		DesktopMode: true,
	})
	t.Cleanup(func() { jftradeCheckTestError(t, handler.Close()) })
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	assertBodyContains := func(path string, accept string, want string) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
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
		defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
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

	apiResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/not-found")
	if err != nil {
		t.Fatalf("GET api not found: %v", err)
	}
	defer func() { jftradeCheckTestError(t, apiResp.Body.Close()) }()
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

func TestDesktopWebAccessHandlerServesProxiedDevelopmentUIAtRoot(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte("JFTrade Vite UI"))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(target.Close)
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	hash, err := passwordhash.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("passwordhash.Hash: %v", err)
	}
	security := jfsettings.SecuritySettings{
		WebAccessEnabled:    true,
		PublicAccessEnabled: true,
		PasswordHash:        hash,
	}
	if _, err := store.SaveSecuritySettings(security); err != nil {
		t.Fatalf("SaveSecuritySettings: %v", err)
	}
	handler := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		FrontendDevURL:  target.URL,
		DesktopMode:     true,
		DesktopAPIToken: "desktop-token",
	})
	t.Cleanup(func() { jftradeCheckTestError(t, handler.Close()) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Accept", "text/html")
	handler.WebAccessHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "JFTrade Vite UI" {
		t.Fatalf("Web root response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestFrontendServerBoundaryHelpers(t *testing.T) {
	expectedFS, available, err := frontendassets.FileSystem()
	if err != nil {
		t.Fatalf("frontendassets.FileSystem(): %v", err)
	}
	loaded := webaccess.LoadFrontendFS()
	if !available {
		if loaded != nil {
			t.Fatalf("webaccess.LoadFrontendFS() = %#v, want nil when embedded assets are unavailable", loaded)
		}
	} else {
		if loaded == nil {
			t.Fatal("webaccess.LoadFrontendFS() returned nil despite embedded assets being available")
		}
		if _, err := fs.Stat(expectedFS, "."); err != nil {
			t.Fatalf("expected embedded frontend fs stat: %v", err)
		}
	}
}

func TestStartForRunArgsInitializesRuntimeLayout(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "var", "jftrade-api")
	settingsPath := filepath.Join(runtimeDir, "settings.json")
	backtestDBPath := filepath.Join(runtimeDir, "backtest.db")

	t.Setenv("JFTRADE_SETTINGS_PATH", settingsPath)
	t.Setenv("JFTRADE_BACKTEST_DB", backtestDBPath)
	t.Setenv("JFTRADE_API_BIND", "127.0.0.1:0")

	ctx := t.Context()

	shutdown, err := servercore.StartForRunArgs(ctx, []string{"api"})
	if err != nil {
		t.Fatalf("StartForRunArgs: %v", err)
	}
	defer func() {
		func() {
			jftradeErr3 := shutdown(context.Background())
			jftradeCheckTestError(t, jftradeErr3)
		}()
	}()

	for _, path := range []string{
		runtimeDir,
		settingsPath,
		apruntime.DeriveStrategyPluginTargetDir(settingsPath),
		apruntime.DeriveStrategyRuntimeDBPath(settingsPath),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	if _, err := os.Stat(filepath.Dir(backtestDBPath)); err != nil {
		t.Fatalf("expected backtest directory to exist: %v", err)
	}
	if _, err := os.Stat(strategystore.DerivePath(settingsPath)); err == nil {
		t.Fatalf("strategy design definition file should not be eagerly created")
	}
}

func TestRunAPIOnlyStopsAfterCallerCancellation(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve API port: %v", err)
	}
	apiBind := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved API port: %v", err)
	}

	runtimeDir := filepath.Join(t.TempDir(), "var", "jftrade-api")
	t.Setenv("JFTRADE_SETTINGS_PATH", filepath.Join(runtimeDir, "settings.json"))
	t.Setenv("JFTRADE_BACKTEST_DB", filepath.Join(runtimeDir, "backtest.db"))
	t.Setenv("JFTRADE_API_BIND", apiBind)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- servercore.RunAPIOnly(ctx)
	}()

	statusURL := "http://" + apiBind + "/api/v1/system/status"
	deadline := time.Now().Add(3 * time.Second)
	for {
		response, requestErr := jftradeTestHTTPGet(t, statusURL)
		if requestErr == nil {
			jftradeErr1 := response.Body.Close()
			jftradeCheckTestError(t, jftradeErr1)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("API-only server did not become reachable at %s: %v", statusURL, requestErr)
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("RunAPIOnly returned shutdown error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunAPIOnly did not return after caller cancellation")
	}
}

func TestStartForRunArgsUsesInterfaceSettingsForAPIBindWhileWebIsDisabled(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	apiBind := listener.Addr().String()
	jftradeErr1 := listener.Close()
	jftradeCheckTestError(t, jftradeErr1)

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

	ctx := t.Context()

	shutdown, err := servercore.StartForRunArgs(ctx, []string{"api"})
	if err != nil {
		t.Fatalf("StartForRunArgs: %v", err)
	}
	defer func() {
		func() {
			jftradeErr2 := shutdown(context.Background())
			jftradeCheckTestError(t, jftradeErr2)
		}()
	}()

	statusURL := "http://" + apiBind + "/api/v1/system/status"
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := jftradeTestHTTPGet(t, statusURL)
		if err == nil {
			if resp.StatusCode != http.StatusForbidden {
				jftradeErr2 := resp.Body.Close()
				jftradeCheckTestError(t, jftradeErr2)
				t.Fatalf("GET status code = %d, want 403 while Web access is disabled", resp.StatusCode)
			}
			jftradeErr3 := resp.Body.Close()
			jftradeCheckTestError(t, jftradeErr3)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("GET %s: %v", statusURL, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
