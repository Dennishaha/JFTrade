package servercore

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
)

func TestServerInitializesWatchlistDatabaseAndDefaultGroup(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "runtime", "settings.json")
	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	response, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/watchlist/groups")
	if err != nil {
		t.Fatalf("GET watchlist groups: %v", err)
	}
	defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET watchlist groups status = %d", response.StatusCode)
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Groups []struct {
				ID        string `json:"groupId"`
				Name      string `json:"name"`
				Protected bool   `json:"protected"`
			} `json:"groups"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode groups: %v", err)
	}
	if !payload.OK || len(payload.Data.Groups) != 1 || payload.Data.Groups[0].Name != "自选股" || !payload.Data.Groups[0].Protected {
		t.Fatalf("watchlist groups payload = %#v", payload)
	}
	if info, err := os.Stat(apiruntime.DeriveWatchlistDBPath(settingsPath)); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("watchlist database info=%v err=%v", info, err)
	}
}

func TestWatchlistRoutesReturn503WhenDatabaseCannotOpen(t *testing.T) {
	root := t.TempDir()
	blockedPath := filepath.Join(root, "watchlists-as-directory")
	if err := os.MkdirAll(blockedPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Setenv("JFTRADE_WATCHLIST_DB", blockedPath)
	store, err := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	response, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/watchlist/groups")
	if err != nil {
		t.Fatalf("GET watchlist groups: %v", err)
	}
	defer func() { jftradeCheckTestError(t, response.Body.Close()) }()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET watchlist groups status = %d, want 503", response.StatusCode)
	}
	var payload struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode 503: %v", err)
	}
	if payload.OK || payload.Error.Code != "DATABASE_INCOMPATIBLE" {
		t.Fatalf("watchlist 503 payload = %#v", payload)
	}
}
