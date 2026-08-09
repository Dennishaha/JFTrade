package servercoretest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func TestRuntimeDefaultsAndLayoutBoundaries(t *testing.T) {
	development := servercore.ResolveLaunchDefaults(false)
	if development.APIBind != apiruntime.DefaultDevelopmentAPIBind || development.GUIBind != "" {
		t.Fatalf("development defaults = %#v", development)
	}
	release := servercore.ResolveLaunchDefaults(true)
	if release.APIBind != apiruntime.DefaultReleaseAPIBind || release.GUIBind != apiruntime.DefaultReleaseGUIBind {
		t.Fatalf("release defaults = %#v", release)
	}
	if got := servercore.APIBaseURLForBind(":3000"); got != "http://127.0.0.1:3000" {
		t.Fatalf("APIBaseURLForBind(:3000) = %q", got)
	}
	if got := servercore.PortFromBind("127.0.0.1:3003", 3000); got != 3003 {
		t.Fatalf("PortFromBind = %d, want 3003", got)
	}
	if got := servercore.PortFromBind("invalid", 3000); got != 3000 {
		t.Fatalf("PortFromBind invalid = %d, want default", got)
	}

	root := t.TempDir()
	settingsPath := filepath.Join(root, "runtime", "settings.json")
	backtestPath := filepath.Join(root, "data", "backtest.db")
	if err := servercore.EnsureRuntimeLayout(settingsPath, backtestPath); err != nil {
		t.Fatalf("EnsureRuntimeLayout: %v", err)
	}
	for _, dir := range []string{filepath.Dir(settingsPath), filepath.Dir(backtestPath)} {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("runtime directory %s was not created", dir)
		}
	}
}

func TestServerSidecarBoundaryMethodsAreNilSafe(t *testing.T) {
	var server *servercore.Server
	server.SetAPIPort(3001)
	server.ConfigureAuthOrigins("http://127.0.0.1:3003")
	server.SetFrontendFS(os.DirFS(t.TempDir()), "http://127.0.0.1:3000")
	server.ApplySecuritySettings(jfsettings.SecuritySettings{WebAccessEnabled: true})
	if err := server.Close(); err != nil {
		t.Fatalf("nil Close = %v", err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	server.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("nil ServeHTTP status = %d, want 404", recorder.Code)
	}

	empty := &servercore.Server{}
	recorder = httptest.NewRecorder()
	empty.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("empty ServeHTTP status = %d, want 404", recorder.Code)
	}
}
