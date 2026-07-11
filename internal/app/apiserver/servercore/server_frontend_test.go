package servercore

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

	"github.com/jftrade/jftrade-main/internal/frontendassets"
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
	t.Cleanup(func() { jftradeErr1 := server.Close(); jftradeCheckTestError(t, jftradeErr1) })
	srv := httptest.NewServer(server)
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

func TestFrontendServerServesRuntimeConfigScript(t *testing.T) {
	frontendDir := t.TempDir()
	srv := httptest.NewServer(newFrontendServerWithRuntimeConfig(os.DirFS(frontendDir), "http://127.0.0.1:6699"))
	t.Cleanup(srv.Close)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/runtime-config.js")
	if err != nil {
		t.Fatalf("GET runtime config: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
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

func TestFrontendServerBoundaryHelpers(t *testing.T) {
	expectedFS, available, err := frontendassets.FileSystem()
	if err != nil {
		t.Fatalf("frontendassets.FileSystem(): %v", err)
	}
	loaded := loadFrontendFS()
	if !available {
		if loaded != nil {
			t.Fatalf("loadFrontendFS() = %#v, want nil when embedded assets are unavailable", loaded)
		}
	} else {
		if loaded == nil {
			t.Fatal("loadFrontendFS() returned nil despite embedded assets being available")
		}
		if _, err := fs.Stat(expectedFS, "."); err != nil {
			t.Fatalf("expected embedded frontend fs stat: %v", err)
		}
	}

	frontendDir := t.TempDir()
	assetsDir := filepath.Join(frontendDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(frontendDir, "index.html"), []byte("<html>fallback</html>"), 0o644); err != nil {
		t.Fatalf("WriteFile index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(frontendDir, "app.js"), []byte("console.log('app');"), 0o644); err != nil {
		t.Fatalf("WriteFile app.js: %v", err)
	}

	server := newFrontendServer(os.DirFS(frontendDir))
	if server == nil {
		t.Fatal("newFrontendServer() returned nil")
	}
	if server.hasFile("assets") {
		t.Fatal("hasFile() should reject directories")
	}
	if server.hasFile("missing.txt") {
		t.Fatal("hasFile() should reject missing assets")
	}

	if normalizeFrontendPath("  /alpha/../beta  ") != "/beta" {
		t.Fatalf("normalizeFrontendPath() did not clean parent traversal")
	}

	recorder := httptest.NewRecorder()
	if server.serveRequest(recorder, httptest.NewRequest(http.MethodPost, "/strategy", nil)) {
		t.Fatal("serveRequest() accepted unsupported POST")
	}
	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("unsupported method recorder mutated unexpectedly: %d %q", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/missing.json", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("ServeHTTP missing asset status = %d, want 404", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	server.serveFile(recorder, httptest.NewRequest(http.MethodGet, "/missing.js", nil), "/missing.js")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("serveFile missing asset status = %d, want 404", recorder.Code)
	}
}

func TestShouldServeFrontendIndexRequestBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		path   string
		accept string
		want   bool
	}{
		{name: "root always allowed", url: "/", path: "/", accept: "", want: true},
		{name: "spa html route", url: "/strategy/live", path: "/strategy/live", accept: "text/html", want: true},
		{name: "spa wildcard route", url: "/strategy/live", path: "/strategy/live", accept: "*/*", want: true},
		{name: "blank path rejected", url: "/", path: "", accept: "text/html", want: false},
		{name: "api route rejected", url: "/api/v1/status", path: "/api/v1/status", accept: "text/html", want: false},
		{name: "swagger route rejected", url: "/swagger/index.html", path: "/swagger/index.html", accept: "text/html", want: false},
		{name: "assets directory rejected", url: "/assets", path: "/assets", accept: "text/html", want: false},
		{name: "assets file rejected", url: "/assets/app.js", path: "/assets/app.js", accept: "text/html", want: false},
		{name: "file extension rejected", url: "/favicon.ico", path: "/favicon.ico", accept: "text/html", want: false},
		{name: "json accept rejected", url: "/strategy/live", path: "/strategy/live", accept: "application/json", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			if got := shouldServeFrontendIndex(req, tc.path); got != tc.want {
				t.Fatalf("shouldServeFrontendIndex(%q, %q) = %v, want %v", tc.path, tc.accept, got, tc.want)
			}
		})
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

	shutdown, err := StartForRunArgs(ctx, []string{"api"})
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

	shutdown, err := StartForRunArgs(ctx, []string{"api"})
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
			if resp.StatusCode != http.StatusOK {
				jftradeErr2 := resp.Body.Close()
				jftradeCheckTestError(t, jftradeErr2)
				t.Fatalf("GET status code = %d", resp.StatusCode)
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
