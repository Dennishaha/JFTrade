package adk

import (
	"strings"
	"testing"
	"time"
)

func TestExpiredRunReconciliationReturnsTerminalPersistenceFailure(t *testing.T) {
	runtime := newTestRuntime(t)
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	run := mustSaveRun(t, runtime, Run{
		ID: "expired-persist-failure", Status: RunStatusRunning, WorkMode: WorkModeChat,
		StartedAt: old, CreatedAt: old, UpdatedAt: old, MaxDurationMs: 1, Usage: &RunUsage{},
	})
	if _, err := runtime.Store().DB().ExecContext(t.Context(), `
		CREATE TRIGGER fail_expired_terminal_state
		BEFORE UPDATE ON `+tableRuns+`
		WHEN NEW.id = 'expired-persist-failure'
		BEGIN SELECT RAISE(FAIL, 'timeout state unavailable'); END
	`); err != nil {
		t.Fatalf("create timeout-state trigger: %v", err)
	}

	err := runtime.ReconcileExpiredRuns(t.Context())
	if err == nil || !strings.Contains(err.Error(), "persist timed-out run "+run.ID) {
		t.Fatalf("ReconcileExpiredRuns error = %v, want terminal persistence failure", err)
	}
	stored, ok, loadErr := runtime.Store().Run(t.Context(), run.ID)
	if loadErr != nil || !ok || stored.Status != RunStatusRunning {
		t.Fatalf("stored run = %+v ok=%v err=%v, want unchanged running state", stored, ok, loadErr)
	}
}
