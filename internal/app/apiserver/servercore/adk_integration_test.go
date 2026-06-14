package servercore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

const (
	realADKIntegrationEnv    = "JFTRADE_REAL_ADK_INTEGRATION"
	realADKIntegrationAgent  = "agent-3a69dc12-e075-4567-9305-7b9a6509418e"
	realADKIntegrationPrompt = "查看系统状态"
)

func TestRealADKChatStreamWithSavedProvider(t *testing.T) {
	if os.Getenv(realADKIntegrationEnv) != "1" {
		t.Skipf("set %s=1 to run the real ADK integration test", realADKIntegrationEnv)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	settingsPath := filepath.Clean(filepath.Join(wd, "..", "..", "var", "jftrade-api", "settings.json"))
	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}

	server := newServerWithFrontend(store, nil)
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Fatalf("server.Close: %v", err)
		}
	})
	if server.adkRuntime == nil {
		t.Fatal("expected adk runtime to be available")
	}

	agent, ok, err := server.adkRuntime.Store().Agent(context.Background(), realADKIntegrationAgent)
	if err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if !ok {
		t.Fatalf("agent %q not found", realADKIntegrationAgent)
	}
	if agent.Status != jfadk.AgentStatusEnabled {
		t.Fatalf("agent %q status = %s, want %s", realADKIntegrationAgent, agent.Status, jfadk.AgentStatusEnabled)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var deltas []jfadk.ChatDelta
	response, err := server.adkRuntime.ChatStream(ctx, jfadk.ChatRequest{
		AgentID: realADKIntegrationAgent,
		Message: realADKIntegrationPrompt,
	}, func(delta jfadk.ChatDelta) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	t.Logf("run_id=%s status=%s tool_calls=%d approvals=%d deltas=%d", response.Run.ID, response.Run.Status, len(response.Run.ToolCalls), len(response.PendingApprovals), len(deltas))
	t.Logf("reply=%s", response.Reply)
	if strings.TrimSpace(response.ReasoningContent) != "" {
		t.Logf("reasoning=%s", response.ReasoningContent)
	}
	for _, call := range response.Run.ToolCalls {
		t.Logf("tool_call id=%s name=%s status=%s duration_ms=%d", call.ID, call.ToolName, call.Status, call.DurationMs)
		if call.Error != nil {
			t.Logf("tool_call_error %s=%s", call.ToolName, *call.Error)
		}
	}

	if response.Run.Status != jfadk.RunStatusCompleted {
		t.Fatalf("run status = %s, want %s; message=%s failure=%s", response.Run.Status, jfadk.RunStatusCompleted, response.Run.Message, response.Run.FailureReason)
	}
	if strings.TrimSpace(response.Reply) == "" {
		t.Fatal("expected non-empty assistant reply")
	}
	if len(response.Run.ToolCalls) == 0 {
		t.Fatal("expected at least one tool call for the system status prompt")
	}
}
