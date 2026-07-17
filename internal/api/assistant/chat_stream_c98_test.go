package assistant

import (
	"errors"
	"testing"
	"time"

	assistantservice "github.com/jftrade/jftrade-main/internal/assistant"
	jadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestCoverage98ChatStreamHubKeepsEventAndTimelineContracts(t *testing.T) {
	t.Run("unserializable transient tool output does not discard the stream event", func(t *testing.T) {
		event := adkChatStreamEvent{Type: "run", Run: &jadk.Run{
			ID: "coverage98-clone-fallback",
			ToolCalls: []jadk.ToolCall{{
				ID: "tool-unserializable", Output: func() {},
			}},
		}}
		cloned := cloneADKChatStreamEvent(event)
		if cloned.Run == nil || cloned.Run.ID != event.Run.ID || len(cloned.Run.ToolCalls) != 1 {
			t.Fatalf("clone fallback discarded live event: %#v", cloned)
		}
	})

	t.Run("timeline-only deltas are consumed without emitting narrative duplicates", func(t *testing.T) {
		hub := newADKChatStreamHub()
		record := hub.create()
		execution := newADKChatStreamExecution(&Handler{streams: hub}, record, jadk.ChatRequest{})
		timeline := jadk.TimelineEntry{RunID: "coverage98-timeline", Kind: jadk.TimelineKindAssistantMessage, Text: "already projected"}
		if err := execution.handleDelta(jadk.ChatDelta{Timeline: &timeline}); err != nil {
			t.Fatalf("handleDelta(timeline): %v", err)
		}
		events, _, _ := record.snapshot(0)
		if len(events) != 1 || events[0].Type != "timeline" || events[0].Timeline == nil || events[0].Timeline.Text != "already projected" {
			t.Fatalf("timeline-only projection = %#v", events)
		}
	})
}

func TestCoverage98ChatStreamExecutionReusesKnownContextAndRecoversTerminalRun(t *testing.T) {
	runtime, _ := newAssistantTestRouter(t)
	ctx := t.Context()
	agent, err := runtime.Store().SaveAgent(ctx, jadk.AgentWriteRequest{
		ID: "coverage98-stream-agent", Name: "Coverage Stream Agent", Status: jadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "coverage stream contracts")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	handler := &Handler{service: assistantservice.NewService(runtime), streams: newADKChatStreamHub()}

	t.Run("existing context does not get re-fetched after preview session", func(t *testing.T) {
		record := handler.streams.create()
		execution := newADKChatStreamExecution(handler, record, jadk.ChatRequest{SessionID: session.ID, Message: "continue"})
		execution.contextSent = true
		execution.ensureSessionAndContext()
		if !execution.sessionSent {
			t.Fatal("preview session was not projected")
		}
		events, _, _ := record.snapshot(0)
		if len(events) != 1 || events[0].Type != "session" {
			t.Fatalf("known-context projection = %#v", events)
		}
	})

	t.Run("persisted terminal run is published as final recovery instead of a second error", func(t *testing.T) {
		run := jadk.Run{
			ID: "coverage98-terminal-run", SessionID: session.ID, AgentID: agent.ID, Status: jadk.RunStatusCompleted,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		if err := runtime.Store().SaveRun(ctx, run); err != nil {
			t.Fatalf("SaveRun: %v", err)
		}
		record := handler.streams.create()
		handler.streams.publish(record, adkChatStreamEvent{Type: "run", Run: &run})
		execution := newADKChatStreamExecution(handler, record, jadk.ChatRequest{})
		execution.publishTerminalError(errors.New("client lost final message"))
		events, terminal, _ := record.snapshot(0)
		if !terminal || len(events) != 2 || events[1].Type != "final" || events[1].Response == nil || events[1].Response.Run.ID != run.ID {
			t.Fatalf("terminal recovery events = %#v terminal=%v", events, terminal)
		}
	})
}
