package servercore

import (
	"encoding/json"
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
	disableTestExchangeCalendarAutoRefresh(t, store)
	server := NewServer(store)
	if server.marketdataSvc != nil {
		server.marketdataSvc.SetSubscriptionReconciler(nil)
	}
	if server.auth != nil {
		server.auth.enabled = false
	}
	if server.strategyRuntimeManager != nil {
		useFakeStrategyRuntimePineWorker(server, newFakeStrategyRuntimePineWorker())
	}
	configureTestADKProvider(t, server)
	t.Cleanup(func() {
		jftradeErr1 := server.Close()
		jftradeCheckTestError(t, jftradeErr1)
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
	disableTestExchangeCalendarAutoRefresh(t, store)
	server := NewServer(store)
	if server.marketdataSvc != nil {
		server.marketdataSvc.SetSubscriptionReconciler(nil)
	}
	if server.auth != nil {
		server.auth.enabled = false
	}
	configureTestADKProvider(t, server)
	t.Cleanup(func() {
		jftradeErr2 := server.Close()
		jftradeCheckTestError(t, jftradeErr2)
	})
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)
	return srv
}

func disableTestExchangeCalendarAutoRefresh(t *testing.T, store *SettingsStore) {
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

func configureTestADKProvider(t *testing.T, server *Server) {
	t.Helper()
	if server == nil || server.adkRuntime == nil {
		return
	}
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { jftradeCheckTestError(t, r.Body.Close()) }()
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		jftradeErr1 := json.NewDecoder(r.Body).Decode(&payload)
		jftradeCheckTestError(t, jftradeErr1)
		var text strings.Builder
		hasToolResponse := false
		for _, message := range payload.Messages {
			if message.Role == "tool" {
				hasToolResponse = true
			}
			text.WriteString("\n" + message.Content)
		}
		message := map[string]any{"role": "assistant", "content": "ok"}
		if !hasToolResponse {
			if tool := testADKToolNameFromText(text.String()); tool != "" {
				message["content"] = ""
				message["tool_calls"] = []map[string]any{{
					"id": "call-" + strings.ReplaceAll(tool, ".", "-"), "type": "function",
					"function": map[string]any{"name": strings.ReplaceAll(tool, ".", "-"), "arguments": `{}`},
				}}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		jftradeErr2 := json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": message}}})
		jftradeCheckTestError(t, jftradeErr2)
	}))
	t.Cleanup(providerServer.Close)
	if _, err := server.adkRuntime.Store().SaveProvider(t.Context(), jfadk.ProviderWriteRequest{
		ID: testADKProviderID, DisplayName: "Test Provider", BaseURL: providerServer.URL, Model: "test-model", APIKey: "sk-test", Enabled: true,
	}); err != nil {
		t.Fatalf("SaveProvider test: %v", err)
	}
	agent, err := server.adkRuntime.Store().DefaultAgent(t.Context())
	if err == nil {
		_, jftradeErr3 := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{
			ID: agent.ID, Name: agent.Name, ProviderID: testADKProviderID, Model: agent.Model, Instruction: agent.Instruction,
			Tools: agent.Tools, PermissionMode: agent.PermissionMode, Status: agent.Status, WorkMode: agent.WorkMode,
			LoopMaxIterations: agent.LoopMaxIterations, RecentUserWindow: agent.RecentUserWindow, MemoryEnabled: agent.MemoryEnabled,
		})
		jftradeCheckTestError(t, jftradeErr3)
	}
}

func testADKToolNameFromText(text string) string {
	for _, name := range []string{
		"approval.required", "strategy.save_draft", "strategy.optimize", "tasks.create", "memory.remember", "contract.write",
	} {
		if strings.Contains(text, "@"+name) || strings.Contains(text, `name="`+name+`"`) {
			return name
		}
	}
	return ""
}
