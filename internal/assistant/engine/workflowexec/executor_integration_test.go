package workflowexec

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	"github.com/jftrade/jftrade-main/internal/assistant/engine/providers"
	adksession "google.golang.org/adk/v2/session"
)

func newTestRuntime(t *testing.T) *jfadk.Runtime {
	t.Helper()
	dir := t.TempDir()
	sessionService, err := enginepersistence.NewSQLiteSessionService(filepath.Join(dir, "adk-session.db"))
	if err != nil {
		t.Fatalf("NewSQLiteSessionService: %v", err)
	}
	if err := enginepersistence.ValidateSQLiteSessionService(sessionService); err != nil {
		t.Fatalf("ValidateSQLiteSessionService: %v", err)
	}
	return newTestRuntimeWithSessionService(t, sessionService)
}

func newTestRuntimeWithSessionService(t *testing.T, sessionService adksession.Service) *jfadk.Runtime {
	t.Helper()
	dir := t.TempDir()
	store, err := jfadk.NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatCompletionResponse{
			Choices: []struct {
				Message providers.OpenAIChatMessage `json:"message"`
			}{{Message: providers.OpenAIChatMessage{Role: "assistant", Content: "目标推进正常。"}}},
		})
	}))
	t.Cleanup(server.Close)
	if _, err := store.SaveProvider(context.Background(), jfadk.ProviderWriteRequest{
		ID: testProviderID, DisplayName: "Test OpenAI Compatible", BaseURL: server.URL,
		Model: "test-model", APIKey: "sk-test", Enabled: true,
	}); err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	runtime := jfadk.NewRuntimeWithSessionService(store, jfadk.NewToolRegistry(), sessionService)
	runtime.SetWorkflowExecutor(NewWorkflowExecutor(runtime))
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

type chatCompletionResponse struct {
	Choices []struct {
		Message providers.OpenAIChatMessage `json:"message"`
	} `json:"choices"`
}

func TestWorkflowExecutorRunsLoopChatEndToEnd(t *testing.T) {
	runtime := newTestRuntime(t)
	agent, err := runtime.Store().SaveAgent(context.Background(), jfadk.AgentWriteRequest{
		ID: "loop-agent", Name: "Loop", Status: jfadk.AgentStatusEnabled, WorkMode: jfadk.WorkModeChat,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(context.Background(), jfadk.ChatRequest{
		AgentID:          agent.ID,
		Message:          "完成一次目标推进",
		WorkModeOverride: jfadk.WorkModeLoop,
		RunOptions:       &jfadk.RunOptions{LoopMaxIterations: 2},
	})
	if err != nil {
		t.Fatalf("Chat loop workflow: %v", err)
	}
	if response.Run.ID == "" || response.Run.WorkMode != jfadk.WorkModeLoop {
		t.Fatalf("loop run = %+v, want started loop workflow", response.Run)
	}
	if response.Run.Status != jfadk.RunStatusPaused && response.Run.Status != jfadk.RunStatusCompleted {
		t.Fatalf("loop run status = %q, want paused or completed", response.Run.Status)
	}
	tasks, total, err := runtime.Store().ListTasksPage(context.Background(), "", "", response.Run.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListTasksPage: %v", err)
	}
	if total == 0 || len(tasks) == 0 {
		t.Fatalf("loop workflow left no persisted goal task (total=%d)", total)
	}
	if len(response.Run.WorkflowPlan) == 0 {
		t.Fatalf("loop run has no workflow plan projection")
	}
}
