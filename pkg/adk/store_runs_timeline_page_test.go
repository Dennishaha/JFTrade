package adk

import (
	"context"
	"testing"
)

func TestSessionTimelineHandlesEmptyAndMissingSessions(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "agent-empty-timeline", "empty timeline")

	var nilStore *Store
	if timeline, ok, err := nilStore.SessionTimeline(ctx, session.ID); err != nil || ok || timeline != nil {
		t.Fatalf("nil store SessionTimeline timeline=%+v ok=%v err=%v, want nil false nil", timeline, ok, err)
	}
	if timeline, ok, err := runtime.Store().SessionTimeline(ctx, "   "); err != nil || ok || timeline != nil {
		t.Fatalf("blank SessionTimeline timeline=%+v ok=%v err=%v, want nil false nil", timeline, ok, err)
	}
	if timeline, ok, err := runtime.Store().SessionTimeline(ctx, "session-missing"); err != nil || ok || timeline != nil {
		t.Fatalf("missing SessionTimeline timeline=%+v ok=%v err=%v, want nil false nil", timeline, ok, err)
	}
	if timeline, ok, err := runtime.Store().SessionTimeline(ctx, session.ID); err != nil || ok || timeline != nil {
		t.Fatalf("empty SessionTimeline timeline=%+v ok=%v err=%v, want nil false nil", timeline, ok, err)
	}
}
