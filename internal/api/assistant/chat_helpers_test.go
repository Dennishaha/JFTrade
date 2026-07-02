package assistant

import (
	"errors"
	"testing"
	"time"

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

func TestTimelineStreamStateEmptyAndCloneBoundaries(t *testing.T) {
	state := &adkTimelineStreamState{}
	if state.appendReasoning(nil, "") != nil {
		t.Fatal("empty reasoning delta should not create a timeline entry")
	}
	if state.appendMessage(nil, "") != nil {
		t.Fatal("empty message delta should not create a timeline entry")
	}
	if cloneTimelineEntry(nil) != nil {
		t.Fatal("cloneTimelineEntry(nil) should be nil")
	}
	if got := defaultTimelineRunID("  "); got != "stream" {
		t.Fatalf("defaultTimelineRunID(blank) = %q, want stream", got)
	}
	if got := firstTimelineToolTime(nil, ""); got == "" {
		t.Fatal("firstTimelineToolTime without candidates should fall back to current time")
	}

	run := &jfadk.Run{ID: "run-empty-tools", SessionID: "session-empty-tools"}
	state.observeRun(run)
	message := state.appendMessage(run, "first")
	message.Text = "mutated clone"
	again := state.appendMessage(run, " second")
	if again.Text != "first second" {
		t.Fatalf("appendMessage should clone without sharing caller mutations, got %q", again.Text)
	}
}

func TestChatStreamHubRetentionRunLookupAndCloneBoundaries(t *testing.T) {
	hub := newADKChatStreamHub()
	if _, ok := hub.get("missing"); ok {
		t.Fatal("missing stream should not be found")
	}
	if _, ok := hub.getByRunID("missing-run"); ok {
		t.Fatal("missing run stream should not be found")
	}

	hub.publish(nil, adkChatStreamEvent{Type: "run", RunID: "nil-record"})
	record := hub.create()
	timeline := &jfadk.TimelineEntry{RunID: "run-clone", Text: "original"}
	hub.publish(record, adkChatStreamEvent{Type: "timeline", Timeline: timeline})
	timeline.Text = "mutated after publish"
	events, terminal, _ := record.snapshot(0)
	if terminal || len(events) != 1 || events[0].Timeline.Text != "original" {
		t.Fatalf("snapshot after cloned publish = events=%+v terminal=%v", events, terminal)
	}
	if _, ok := hub.getByRunID("run-clone"); !ok {
		t.Fatal("published timeline run id should be indexed")
	}

	for range adkChatStreamEventLimit + 5 {
		hub.publish(record, adkChatStreamEvent{Type: "run", RunID: "run-clone"})
	}
	events, _, _ = record.snapshot(0)
	if len(events) != adkChatStreamEventLimit {
		t.Fatalf("event retention len = %d, want %d", len(events), adkChatStreamEventLimit)
	}

	terminalRecord := hub.create()
	hub.publish(terminalRecord, adkChatStreamEvent{Type: "final", RunID: "run-terminal"})
	terminalRecord.mu.Lock()
	terminalRecord.expiresAt = time.Now().Add(-time.Second)
	terminalRecord.mu.Unlock()
	hub.cleanup()
	if _, ok := hub.get(terminalRecord.id); ok {
		t.Fatal("expired terminal stream should be removed")
	}

	unknownRun := hub.create()
	unknownRun.startedAt = time.Now().Add(-jfadk.DefaultRunTimeout).Add(-2 * adkChatStreamRetention)
	hub.cleanup()
	if _, ok := hub.get(unknownRun.id); ok {
		t.Fatal("stream without run id should expire after runtime timeout plus retention")
	}

	running := jfadk.Run{ID: "run-active", Status: jfadk.RunStatusRunning, StartedAt: time.Now().Format(time.RFC3339Nano)}
	if streamRunExpired(time.Now(), running, time.Now()) {
		t.Fatal("fresh running stream should not be expired")
	}
	completed := jfadk.Run{ID: "run-complete", Status: jfadk.RunStatusCompleted}
	if streamRunExpired(time.Now(), completed, time.Now()) {
		t.Fatal("recent terminal stream should stay during retention")
	}
	if !streamRunExpired(time.Now(), completed, time.Now().Add(-adkChatStreamRetention-time.Second)) {
		t.Fatal("old terminal stream should expire")
	}
	unparseable := jfadk.Run{ID: "run-unparseable", Status: jfadk.RunStatusRunning, StartedAt: "bad-time"}
	if !streamRunExpired(time.Now(), unparseable, time.Now().Add(-jfadk.DefaultRunTimeout-adkChatStreamRetention-time.Second)) {
		t.Fatal("unparseable run time should fall back to last event timeout")
	}
}

func TestStreamHelpersRunIDAndBestEffortLogging(t *testing.T) {
	(*Handler)(nil).cleanupADKChatStreams()
	(&Handler{}).cleanupADKChatStreams()

	if got := streamEventRunID(adkChatStreamEvent{Response: &jfadk.ChatResponse{Run: jfadk.Run{ID: " response-run "}}}); got != "response-run" {
		t.Fatalf("streamEventRunID(response) = %q", got)
	}
	if got := streamEventRunID(adkChatStreamEvent{Timeline: &jfadk.TimelineEntry{RunID: " timeline-run "}}); got != "timeline-run" {
		t.Fatalf("streamEventRunID(timeline) = %q", got)
	}
	if got := streamEventRunID(adkChatStreamEvent{RunID: " explicit-run "}); got != "explicit-run" {
		t.Fatalf("streamEventRunID(explicit) = %q", got)
	}
	jftradeLogError(nil, errors.New("expected best-effort test log"))
}
