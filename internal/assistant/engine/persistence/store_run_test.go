package persistence

import (
	"encoding/json"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavePreparedRunWithExecutorRejectsUnsupportedPayload(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStoreCore(
		filepath.Join(dir, "adk.db"),
		filepath.Join(dir, "secrets", "adk.json"),
		filepath.Join(dir, "skills"),
	)
	if err != nil {
		t.Fatalf("NewStoreCore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := savePreparedRunWithExecutor(t.Context(), store.DB(), assistantmodel.Run{
		ID: "prepared-bad-json", SessionID: "session", AgentID: "agent", Status: assistantmodel.RunStatusRunning,
		ToolCalls: []assistantmodel.ToolCall{{ID: "tool", Output: func() {}}},
	}); err == nil {
		t.Fatal("savePreparedRunWithExecutor marshal err = nil, want error")
	}
}

func TestRunReasoningSnapshotIsPrivateAndRestored(t *testing.T) {
	store := newReasoningTestStore(t)
	run := assistantmodel.Run{
		ID: "private-reasoning-run", SessionID: "session", AgentID: "agent",
		Status: assistantmodel.RunStatusPending, ReasoningEffort: assistantmodel.ReasoningEffortHigh,
		ReasoningEffortField: "provider.reasoning.level", ReasoningEffortValue: "DEEP",
	}
	if err := store.SaveRun(t.Context(), run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	var payload string
	if err := store.DB().GetContext(t.Context(), &payload, `SELECT payload_json FROM `+tableRuns+` WHERE id = ?`, run.ID); err != nil {
		t.Fatalf("read run payload: %v", err)
	}
	if !strings.Contains(payload, `"reasoningEffortField":"provider.reasoning.level"`) ||
		!strings.Contains(payload, `"reasoningEffortValue":"DEEP"`) {
		t.Fatalf("private snapshot missing from persistence payload: %s", payload)
	}
	restored, ok, err := store.Run(t.Context(), run.ID)
	if err != nil || !ok || restored.ReasoningEffortField != run.ReasoningEffortField || restored.ReasoningEffortValue != run.ReasoningEffortValue {
		t.Fatalf("restored run = %+v ok=%v err=%v", restored, ok, err)
	}
	publicJSON, err := json.Marshal(restored)
	if err != nil {
		t.Fatalf("marshal public run: %v", err)
	}
	if strings.Contains(string(publicJSON), "reasoningEffortField") || strings.Contains(string(publicJSON), "reasoningEffortValue") {
		t.Fatalf("private snapshot leaked through public Run JSON: %s", publicJSON)
	}
}
