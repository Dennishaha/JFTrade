package assistant

import (
	"testing"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestTimelineStreamStateTracksSessionRunAndToolTiming(t *testing.T) {
	state := &adkTimelineStreamState{}
	state.observeSession(jfadk.Session{ID: "session-1"})
	state.observeTimeline(jfadk.TimelineEntry{SessionID: "session-2", RunID: "run-1", Sequence: 3})

	run := &jfadk.Run{
		ID:        "run-2",
		SessionID: "session-3",
		ToolCalls: []jfadk.ToolCall{
			{ID: "tool-1", CreatedAt: "2026-06-21T00:00:03Z", UpdatedAt: "2026-06-21T00:00:04Z"},
			{ID: "tool-2", CreatedAt: "2026-06-21T00:00:01Z", UpdatedAt: "2026-06-21T00:00:02Z"},
		},
	}
	state.observeRun(run)
	if state.sessionID != "session-3" || state.runID != "run-2" {
		t.Fatalf("state after observeRun = %+v", state)
	}
	toolGroup := state.toolGroupSnapshot()
	if toolGroup == nil || toolGroup.CreatedAt != "2026-06-21T00:00:01Z" || toolGroup.Sequence != 4 {
		t.Fatalf("toolGroup = %+v, want earliest tool time and next sequence", toolGroup)
	}

	reasoning := state.appendReasoning(run, "先推理")
	message := state.appendMessage(run, "再回答")
	if reasoning == nil || reasoning.Kind != jfadk.TimelineKindAssistantReasoning || reasoning.Text != "先推理" {
		t.Fatalf("reasoning timeline = %+v", reasoning)
	}
	if message == nil || message.Kind != jfadk.TimelineKindAssistantMessage || message.Text != "再回答" {
		t.Fatalf("message timeline = %+v", message)
	}

	if got := firstTimelineToolTime([]jfadk.ToolCall{{UpdatedAt: "2026-06-21T00:00:05Z"}}, ""); got != "2026-06-21T00:00:05Z" {
		t.Fatalf("firstTimelineToolTime() = %q, want updatedAt fallback", got)
	}
}

func TestChatStreamRecordCurrentRunID(t *testing.T) {
	hub := newADKChatStreamHub()
	record := hub.create()
	hub.publish(record, adkChatStreamEvent{Type: "run", Run: &jfadk.Run{ID: "run-current"}})

	if got := record.currentRunID(); got != "run-current" {
		t.Fatalf("currentRunID() = %q, want run-current", got)
	}
}
