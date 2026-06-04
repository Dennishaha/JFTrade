package jftradeapi

import (
	"net/http/httptest"
	"testing"
)

// newTestServer creates a Server with the given SettingsStore and registers its
// Close on t.Cleanup. Prefer this over bare NewServer(store) in tests so that
// SQLite database connections are released even when tests fail.
func newTestServer(t *testing.T, store *SettingsStore) *Server {
	t.Helper()
	server := NewServer(store)
	t.Cleanup(func() {
		_ = server.Close()
	})
	return server
}

// newHTTPTestServer creates an httptest.Server wrapping NewServer(store) and
// registers cleanup for both the HTTP test server and the JFTrade Server.
// Cleanup runs in the correct order: httptest.Server.Close() first, then
// Server.Close(), so that in-flight HTTP handlers complete before SQLite
// connections are released.
func newHTTPTestServer(t *testing.T, store *SettingsStore) *httptest.Server {
	t.Helper()
	server := NewServer(store)
	t.Cleanup(func() {
		_ = server.Close()
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	return srv
}
