package servercore

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
)

func TestServerInitializesResearchDatabaseAndPresetRoutes(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "runtime", "settings.json")
	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	response, err := http.Post(srv.URL+"/api/v1/research/screens/presets", "application/json",
		bytes.NewBufferString(`{"name":"美股价值","definition":{"brokerId":"futu","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"columns":[{"columnId":"price","factor":{"instanceId":"price","factorKey":"simple.price"}}]}}`))
	if err != nil {
		t.Fatalf("POST research preset: %v", err)
	}
	defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST research preset status = %d", response.StatusCode)
	}
	if info, err := os.Stat(apiruntime.DeriveResearchDBPath(settingsPath)); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("research database info=%v err=%v", info, err)
	}
}

func TestResearchPresetRoutesReturn503WhenDatabaseCannotOpen(t *testing.T) {
	root := t.TempDir()
	blockedPath := filepath.Join(root, "research-as-directory")
	if err := os.MkdirAll(blockedPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Setenv("JFTRADE_RESEARCH_DB", blockedPath)
	store, err := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	response, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/research/screens/presets")
	if err != nil {
		t.Fatalf("GET research presets: %v", err)
	}
	defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET research presets status = %d, want 503", response.StatusCode)
	}
}
