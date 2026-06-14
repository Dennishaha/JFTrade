package servercore

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
)

// newTestServer creates a Server with the given SettingsStore and registers its
// Close on t.Cleanup. Prefer this over bare NewServer(store) in tests so that
// SQLite database connections are released even when tests fail.
func newTestServer(t *testing.T, store *SettingsStore) *Server {
	t.Helper()
	server := NewServer(store)
	if server.auth != nil {
		server.auth.enabled = false
	}
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
	if server.auth != nil {
		server.auth.enabled = false
	}
	t.Cleanup(func() {
		_ = server.Close()
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	return srv
}

// newTestServerWithAuth 创建一个启用了鉴权的测试 Server。
// 与 newTestServer 不同，此函数保留 auth.enabled = true，
// 适用于测试需要鉴权的路由（如 POST/PUT/DELETE）。
func newTestServerWithAuth(t *testing.T, store *SettingsStore) *Server {
	t.Helper()
	server := NewServer(store)
	t.Cleanup(func() {
		_ = server.Close()
	})
	return server
}

// newHTTPTestServerWithAuth 创建一个启用了鉴权的 httptest.Server。
func newHTTPTestServerWithAuth(t *testing.T, store *SettingsStore) *httptest.Server {
	t.Helper()
	server := NewServer(store)
	t.Cleanup(func() {
		_ = server.Close()
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	return srv
}

// adminAuthHeader 返回包含管理员 Bearer token 的 HTTP Header。
// 仅在 auth 已配置且 enabled 时有效。
func adminAuthHeader(server *Server) (key string, value string) {
	if server.auth == nil || !server.auth.enabled {
		return "", ""
	}
	return "Authorization", "Bearer " + server.auth.key
}

// decodeEnvelope 解码 HTTP 响应中的 envelope。
func decodeEnvelope(t *testing.T, body io.Reader) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.NewDecoder(body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env
}

// requireEnvelopeOK 验证 envelope 的 ok 字段为 true 并返回 data。
func requireEnvelopeOK(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	ok, _ := env["ok"].(bool)
	if !ok {
		t.Fatalf("envelope ok != true: %v", env)
	}
	data, _ := env["data"].(map[string]any)
	return data
}
