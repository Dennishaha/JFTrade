package servercoretest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

// newHTTPTestServer creates an httptest.Server around the public sidecar
// handler and registers cleanup for both the HTTP server and the JFTrade
// sidecar. Cleanup runs in the correct order: httptest.Server.Close() first,
// then handler.Close(), so that in-flight HTTP handlers complete before
// SQLite connections are released.
func newHTTPTestServer(t *testing.T, store *servercore.SettingsStore) *httptest.Server {
	t.Helper()
	_, srv := newHTTPTestServerWithHandler(t, store)
	return srv
}

func newHTTPTestServerWithHandler(t *testing.T, store *servercore.SettingsStore) (servercore.SidecarHandler, *httptest.Server) {
	t.Helper()
	isolateTestBacktestDatabase(t, store)
	disableTestExchangeCalendarAutoRefresh(t, store)
	forceTestMarketDataProvider(t, store)
	// Desktop mode keeps the same open-loopback API fixture as the historical
	// servercore test server: no browser listener and no forced access check.
	handler := servercore.NewSidecarHandlerWithOptions(store, servercore.SidecarOptions{
		DesktopMode: true,
	})
	t.Cleanup(func() {
		jftradeCheckTestError(t, handler.Close())
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return handler, srv
}

func forceTestMarketDataProvider(t *testing.T, store *servercore.SettingsStore) {
	t.Helper()
	if store == nil {
		return
	}
	// Keep the shared server fixture independent of whether the host has a
	// usable embedded market-data helper and the AKShare default provider.
	if err := store.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderFutu); err != nil {
		t.Fatalf("SaveActiveMarketDataProvider: %v", err)
	}
}

func isolateTestBacktestDatabase(t *testing.T, store *servercore.SettingsStore) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("JFTRADE_BACKTEST_DB")) != "" || store == nil {
		return
	}
	t.Setenv("JFTRADE_BACKTEST_DB", filepath.Join(filepath.Dir(store.Path()), "backtest.db"))
}

func disableTestExchangeCalendarAutoRefresh(t *testing.T, store *servercore.SettingsStore) {
	t.Helper()
	if store == nil {
		return
	}
	settings := store.ExchangeCalendarSettings()
	settings.AutoRefreshEnabled = false
	if _, err := store.SaveExchangeCalendarSettings(settings); err != nil {
		t.Fatalf("SaveExchangeCalendarSettings: %v", err)
	}
}

func jftradeTestHTTPGet(t testing.TB, url string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func jftradeTestHTTPPost(t testing.TB, url string, contentType string, body io.Reader) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}

func jftradeCheckTestError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}
