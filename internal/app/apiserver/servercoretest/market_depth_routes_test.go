package servercoretest

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
)

// newHTTPTestServerWithoutProviderOverride keeps the shared fixture's DB
// isolation but does not force a market-data provider. The depth routing
// tests only need the route-registration contract, not a specific provider
// selection.
func newHTTPTestServerWithoutProviderOverride(t *testing.T, store *servercore.SettingsStore) *httptest.Server {
	t.Helper()
	isolateTestBacktestDatabase(t, store)
	disableTestExchangeCalendarAutoRefresh(t, store)
	handler := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode: true,
	})
	t.Cleanup(func() {
		jftradeCheckTestError(t, handler.Close())
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestMarketDepthEndpointRouting(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServerWithoutProviderOverride(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/market-data/depth/US/NVDA?num=5")
	if err != nil {
		t.Fatalf("GET depth: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	// Without an acquired ORDER_BOOK subscription the production assembly
	// returns 409 (subscription required), NOT 404 — the route is registered.
	if resp.StatusCode == http.StatusNotFound {
		t.Fatal("depth endpoint returned 404 — route not registered")
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("depth endpoint returned %d, want 409 without an acquired subscription", resp.StatusCode)
	}
}

func TestMarketDepthEndpointMethodNotAllowed(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServerWithoutProviderOverride(t, store)

	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/market-data/depth/US/NVDA", "application/json", nil)
	if err != nil {
		t.Fatalf("POST depth: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST to depth endpoint should return 404, got %d", resp.StatusCode)
	}
}

func TestMarketDepthEndpointPutNotAllowed(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServerWithoutProviderOverride(t, store)

	req, jftradeErr1 := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/market-data/depth/US/NVDA", nil)
	jftradeCheckTestError(t, jftradeErr1)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT depth: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("PUT to depth endpoint should return 404, got %d", resp.StatusCode)
	}
}

func TestMarketDepthRouteDoesNotCollide(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServerWithoutProviderOverride(t, store)

	// /api/v1/market-data/depths should NOT match the depth route (different prefix)
	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/market-data/depths")
	if err != nil {
		t.Fatalf("GET depths: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("/api/v1/market-data/depths returned %d, want 404 (should not collide with depth route)", resp.StatusCode)
	}
}
