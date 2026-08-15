package model

import (
	"testing"
	"time"
)

// The final reply of a tool-using run must sort after the run's tool
// activity so the web turn-trace grouping keeps it outside the collapsed
// work block.
func TestBuildSessionTimelinePlacesFinalReplyAfterToolActivity(t *testing.T) {
	base := time.Date(2026, 8, 15, 23, 14, 51, 0, time.UTC)
	at := func(seconds int) string {
		return base.Add(time.Duration(seconds) * time.Second).UTC().Format(time.RFC3339Nano)
	}
	session := Session{ID: "session-1"}
	messages := []TranscriptEntry{
		{ID: "user-1", SessionID: session.ID, RunID: "run-1", Role: "user", Content: "分析下未来1周什么板块有起色？", CreatedAt: at(0)},
		{ID: "msg-final", SessionID: session.ID, RunID: "inv-1", Role: "assistant", Content: "最终回答", CreatedAt: at(4)},
	}
	runs := []Run{{
		ID: "run-1", SessionID: session.ID, Status: RunStatusCompleted,
		UserMessage: "分析下未来1周什么板块有起色？", FinalMessageID: "msg-final",
		CreatedAt: at(0), UpdatedAt: at(4),
		ToolCalls: []ToolCall{{
			ID: "call-1", RunID: "run-1", ToolName: "market.snapshot", Status: "SUCCEEDED",
			CreatedAt: at(2), UpdatedAt: at(3),
		}},
	}}

	timeline := BuildSessionTimeline(session, messages, runs, nil)
	kinds := make([]string, 0, len(timeline))
	for _, entry := range timeline {
		kinds = append(kinds, entry.Kind)
	}
	want := []string{TimelineKindUserMessage, TimelineKindToolGroup, TimelineKindAssistantMessage}
	if len(kinds) != len(want) {
		t.Fatalf("timeline kinds = %v, want %v", kinds, want)
	}
	for index := range want {
		if kinds[index] != want[index] {
			t.Fatalf("timeline kinds = %v, want %v", kinds, want)
		}
	}
	if timeline[len(timeline)-1].Text != "最终回答" {
		t.Fatalf("final entry text = %q", timeline[len(timeline)-1].Text)
	}
}
