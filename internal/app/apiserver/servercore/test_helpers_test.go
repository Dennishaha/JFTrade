package servercore

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

const testADKProviderID = "test-provider"

// newTestServer creates a Server with the given SettingsStore and registers its
// Close on t.Cleanup. Prefer this over bare NewServer(store) in tests so that
// SQLite database connections are released even when tests fail.
func newTestServer(t *testing.T, store *SettingsStore) *Server {
	t.Helper()
	server := NewServer(store)
	if server.auth != nil {
		server.auth.enabled = false
	}
	configureTestADKProvider(t, server)
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
	configureTestADKProvider(t, server)
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
	configureTestADKProvider(t, server)
	t.Cleanup(func() {
		_ = server.Close()
	})
	return server
}

// newHTTPTestServerWithAuth 创建一个启用了鉴权的 httptest.Server。
func newHTTPTestServerWithAuth(t *testing.T, store *SettingsStore) *httptest.Server {
	t.Helper()
	server := NewServer(store)
	configureTestADKProvider(t, server)
	t.Cleanup(func() {
		_ = server.Close()
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	return srv
}

func configureTestADKProvider(t *testing.T, server *Server) {
	t.Helper()
	if server == nil || server.adkRuntime == nil {
		return
	}
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		var text string
		hasToolResponse := false
		for _, message := range payload.Messages {
			if message.Role == "tool" {
				hasToolResponse = true
			}
			text += "\n" + message.Content
		}
		message := map[string]any{"role": "assistant", "content": "ok"}
		if !hasToolResponse {
			if tool := testADKToolNameFromText(text); tool != "" {
				message["content"] = ""
				message["tool_calls"] = []map[string]any{{
					"id": "call-" + strings.ReplaceAll(tool, ".", "-"), "type": "function",
					"function": map[string]any{"name": strings.ReplaceAll(tool, ".", "-"), "arguments": `{}`},
				}}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": message}}})
	}))
	t.Cleanup(providerServer.Close)
	if _, err := server.adkRuntime.Store().SaveProvider(t.Context(), jfadk.ProviderWriteRequest{
		ID: testADKProviderID, DisplayName: "Test Provider", BaseURL: providerServer.URL, Model: "test-model", APIKey: "sk-test", Enabled: true,
	}); err != nil {
		t.Fatalf("SaveProvider test: %v", err)
	}
	agent, err := server.adkRuntime.Store().DefaultAgent(t.Context())
	if err == nil {
		_, _ = server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
			ID: agent.ID, Name: agent.Name, ProviderID: testADKProviderID, Model: agent.Model, Instruction: agent.Instruction,
			Tools: agent.Tools, PermissionMode: agent.PermissionMode, Status: agent.Status, WorkMode: agent.WorkMode,
			LoopMaxIterations: agent.LoopMaxIterations, RecentUserWindow: agent.RecentUserWindow, MemoryEnabled: agent.MemoryEnabled,
		})
	}
}

func testADKToolNameFromText(text string) string {
	for _, name := range []string{
		"strategy.save_draft", "strategy.optimize", "tasks.create", "memory.remember", "contract.write",
	} {
		if strings.Contains(text, "@"+name) || strings.Contains(text, `name="`+name+`"`) {
			return name
		}
	}
	return ""
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
