package persistence

import (
	"path/filepath"
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
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

	if err := savePreparedRunWithExecutor(t.Context(), store.DB(), jfadkmodel.Run{
		ID: "prepared-bad-json", SessionID: "session", AgentID: "agent", Status: jfadkmodel.RunStatusRunning,
		ToolCalls: []jfadkmodel.ToolCall{{ID: "tool", Output: func() {}}},
	}); err == nil {
		t.Fatal("savePreparedRunWithExecutor marshal err = nil, want error")
	}
}
