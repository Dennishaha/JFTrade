package servercore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcomposition "github.com/jftrade/jftrade-main/internal/app/apiserver/application"
	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

const testADKProviderID = "test-provider"

// newTestServer creates a Server with the given SettingsStore and registers its
// Close on t.Cleanup. Prefer this over bare NewServer(store) in tests so that
// SQLite database connections are released even when tests fail.
func newTestServer(t *testing.T, store *SettingsStore) *Server {
	t.Helper()
	isolateTestBacktestDatabase(t, store)
	disableTestExchangeCalendarAutoRefresh(t, store)
	forceTestMarketDataProvider(t, store)
	server := NewServer(store)
	if server.marketdataSvc != nil {
		server.marketdataSvc.SetSubscriptionReconciler(nil)
	}
	if server.auth != nil {
		server.auth.enabled = false
	}
	if server.runtimes.StrategyRuntime() != nil {
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
	isolateTestBacktestDatabase(t, store)
	disableTestExchangeCalendarAutoRefresh(t, store)
	forceTestMarketDataProvider(t, store)
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

func forceTestMarketDataProvider(t *testing.T, store *SettingsStore) {
	t.Helper()
	if store == nil {
		return
	}
	// Keep the shared server fixture independent of whether the host has a
	// compatible Python runtime that would make yfinance the default provider.
	if err := store.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderFutu); err != nil {
		t.Fatalf("SaveActiveMarketDataProvider: %v", err)
	}
}

func isolateTestBacktestDatabase(t *testing.T, store *SettingsStore) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("JFTRADE_BACKTEST_DB")) != "" || store == nil {
		return
	}
	t.Setenv("JFTRADE_BACKTEST_DB", filepath.Join(filepath.Dir(store.Path()), "backtest.db"))
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

func serverADKTestStore(t *testing.T, server *Server) *assistanttestkit.Store {
	t.Helper()
	if server == nil || server.store == nil {
		t.Fatal("assistant test store requires an initialized server")
	}
	paths := appcomposition.AssistantPaths(server.store.Path())
	store, err := assistanttestkit.NewStore(paths.Database, paths.Secrets, paths.Skills)
	if err != nil {
		t.Fatalf("open assistant test store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("close assistant test store: %v", closeErr)
		}
	})
	return store
}

func configureTestADKProvider(t *testing.T, server *Server) {
	t.Helper()
	if assistantRuntime(server) == nil {
		return
	}
	adkStore := serverADKTestStore(t, server)
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
	if _, err := adkStore.SaveProvider(t.Context(), assistant.ProviderWriteRequest{
		ID: testADKProviderID, DisplayName: "Test Provider", BaseURL: providerServer.URL, Model: "test-model", APIKey: "sk-test", Enabled: true,
	}); err != nil {
		t.Fatalf("SaveProvider test: %v", err)
	}
	agent, err := adkStore.DefaultAgent(t.Context())
	if err == nil {
		_, jftradeErr3 := adkStore.SaveAgent(t.Context(), assistant.AgentWriteRequest{
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
