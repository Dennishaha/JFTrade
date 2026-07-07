package adk

import (
	"context"
	"strings"
	"testing"
)

func TestSessionContextManagerRemainingEdgeBranches(t *testing.T) {
	ctx := context.Background()

	if _, err := (*SessionContextManager)(nil).Compact(ctx, Session{}, Agent{}, SessionCompactRequest{}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil Compact err = %v", err)
	}
	if snapshot, compacted, err := (*SessionContextManager)(nil).AutoCompactForModelContext(ctx, Session{ID: "session"}, Agent{}, "pending"); err != nil || compacted || snapshot.SessionID != "" {
		t.Fatalf("nil AutoCompactForModelContext snapshot=%+v compacted=%v err=%v", snapshot, compacted, err)
	}
	if snapshot, compacted, err := (&SessionContextManager{}).AutoCompactForModelContext(ctx, Session{}, Agent{}, "pending"); err != nil || compacted || snapshot.SessionID != "" {
		t.Fatalf("blank-session AutoCompactForModelContext snapshot=%+v compacted=%v err=%v", snapshot, compacted, err)
	}

	runtime := newTestRuntime(t)
	manager := runtime.contextManager
	jftradeCheckTestError(t, runtime.Store().Close())
	if active, err := manager.HasActiveRun(ctx, "session"); err == nil || active {
		t.Fatalf("HasActiveRun closed store active=%v err=%v", active, err)
	}

	runtime = newTestRuntime(t)
	manager = runtime.contextManager
	if got, degraded := manager.mergeSummary(ctx, Agent{ProviderID: "missing-provider"}, "deterministic", "existing", "normal"); got != "deterministic" || !degraded {
		t.Fatalf("mergeSummary missing provider = %q/%v", got, degraded)
	}
	provider := mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "merge-no-key-provider", BaseURL: "https://example.test/v1", Model: "model", Enabled: true,
	})
	if got, degraded := manager.mergeSummary(ctx, Agent{ProviderID: provider.ID}, "deterministic", "existing", "normal"); got != "deterministic" || !degraded {
		t.Fatalf("mergeSummary no api key = %q/%v", got, degraded)
	}
}
