package adk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func TestEventProjectionHandlesEmptyStorageOrderingAndFallbackEntries(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	entries, err := nilStore.TranscriptEntries(ctx, " ")
	if err != nil || len(entries) != 0 {
		t.Fatalf("blank transcript on nil store = %#v, err=%v", entries, err)
	}
	projection, ok, err := (&Store{}).SessionProjection(ctx, " session-without-adk-service ")
	if err != nil || ok || projection.SessionID != "session-without-adk-service" {
		t.Fatalf("projection without ADK service = %+v ok=%v err=%v", projection, ok, err)
	}

	at := time.Date(2026, 7, 16, 11, 0, 0, 0, time.UTC)
	events := []*adksession.Event{
		nil,
		newProjectionEvent("b", "run", "agent", genai.RoleModel, []*genai.Part{{Text: "b"}}, at, false),
		newProjectionEvent("a", "run", "agent", genai.RoleModel, []*genai.Part{{Text: "a"}}, at, false),
	}
	sortProjectedEvents(events)
	if events[0] == nil || events[0].ID != "a" || events[1] == nil || events[1].ID != "b" || events[2] != nil {
		t.Fatalf("stable projected event ordering = %#v", events)
	}
	if eventHasFunctionResponse(nil) || eventHasFunctionResponse(&adksession.Event{}) {
		t.Fatal("empty events must not be mistaken for tool-response events")
	}

	if projection := buildSessionProjection(nil, map[string]*projectedRunState{"missing": nil}, []string{"missing"}); len(projection.Messages) != 0 {
		t.Fatalf("projection with nil run state = %+v", projection)
	}
	if ensureProjectedToolCall(nil, &genai.FunctionCall{Name: "market.price"}, "now") != nil {
		t.Fatal("tool call cannot be projected without a run state")
	}
	state := &projectedRunState{runID: "run", toolCalls: map[string]*ToolCall{}, toolCallOrder: []string{}}
	state.reply.WriteString("before tool")
	call := ensureProjectedToolCall(state, &genai.FunctionCall{Name: "market.price", Args: map[string]any{"symbol": "AAPL"}}, "time")
	if call == nil || call.IdempotencyKey != "market.price:time" || state.preToolContent != "before tool" {
		t.Fatalf("fallback tool-call id = %+v state=%+v", call, state)
	}
	if same := ensureProjectedToolCall(state, &genai.FunctionCall{Name: "market.price"}, "time"); same != call || len(state.toolCallOrder) != 1 {
		t.Fatalf("duplicate fallback call = %+v order=%+v", same, state.toolCallOrder)
	}
	projectedToolResponse(nil, nil, "now")
	pruneProjectedToolCall(nil, "id", "tool", "time")

	if _, ok := transcriptEntryFromADKEvent(nil); ok {
		t.Fatal("nil event should not project a transcript entry")
	}
	empty := newProjectionEvent("empty", "run", "agent", genai.RoleModel, []*genai.Part{{FunctionCall: &genai.FunctionCall{Name: "tool"}}}, at, false)
	if _, ok := transcriptEntryFromADKEvent(empty); ok {
		t.Fatal("function-only event should not project a user-visible transcript entry")
	}
	fallback := newProjectionEvent("", "", "user", genai.RoleUser, []*genai.Part{{Text: " user reply "}}, at, false)
	entry, ok := transcriptEntryFromADKEvent(fallback)
	if !ok || entry.Role != "user" || entry.Content != "user reply" || !strings.HasPrefix(entry.ID, "event-message-") {
		t.Fatalf("fallback transcript entry = %+v ok=%v", entry, ok)
	}
}

func TestSessionProjectionPropagatesADKSessionReadFailures(t *testing.T) {
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "projection-read-error-agent", "projection read error")
	store := runtime.Store()
	want := errors.New("ADK session store unavailable")
	store.mu.Lock()
	store.sessions = getErrorADKSessionService{Service: store.sessions, err: want}
	store.mu.Unlock()

	if _, _, err := store.SessionProjection(t.Context(), session.ID); !errors.Is(err, want) {
		t.Fatalf("SessionProjection error = %v, want %v", err, want)
	}
	if _, err := store.TranscriptEntries(t.Context(), session.ID); !errors.Is(err, want) {
		t.Fatalf("TranscriptEntries error = %v, want %v", err, want)
	}
}

func TestSessionProjectionTreatsMissingADKSessionAsEmpty(t *testing.T) {
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "projection-missing-session-agent", "projection missing session")

	projection, ok, err := runtime.Store().SessionProjection(t.Context(), session.ID)
	if err != nil || ok || projection.SessionID != session.ID {
		t.Fatalf("SessionProjection = %+v ok=%v err=%v, want empty projection", projection, ok, err)
	}
	entries, err := runtime.Store().TranscriptEntries(t.Context(), session.ID)
	if err != nil || len(entries) != 0 {
		t.Fatalf("TranscriptEntries = %+v err=%v, want empty entries", entries, err)
	}
}
