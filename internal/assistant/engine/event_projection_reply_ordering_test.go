package adk

import (
	"testing"
	"time"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// A run whose model narrates before and between tool calls used to pin the
// merged assistant entry at the invocation's first event time. The final
// reply then sorted ahead of the tool activity and the web timeline folded it
// into the collapsed turn-trace block, rendering no visible answer.
func TestProjectedAssistantEntryAnchorsAtLatestTextEvent(t *testing.T) {
	base := time.Date(2026, 8, 15, 23, 14, 51, 0, time.UTC)
	at := func(seconds int) time.Time { return base.Add(time.Duration(seconds) * time.Second) }
	events := []*adksession.Event{
		newProjectionEvent("user-1", "inv-1", "user", genai.RoleUser,
			[]*genai.Part{{Text: "分析下未来1周什么板块有起色？"}}, at(0), false),
		newProjectionEvent("text-early", "inv-1", "jftrade_default", genai.RoleModel,
			[]*genai.Part{{Text: "先查板块行情。"}}, at(1), false),
		newProjectionEvent("call-1", "inv-1", "jftrade_default", genai.RoleModel,
			[]*genai.Part{{FunctionCall: &genai.FunctionCall{ID: "call-1", Name: "market.snapshot"}}}, at(2), false),
		newProjectionEvent("resp-1", "inv-1", "jftrade_default", genai.RoleUser,
			[]*genai.Part{{FunctionResponse: &genai.FunctionResponse{ID: "call-1", Name: "market.snapshot", Response: map[string]any{"ok": true}}}}, at(3), false),
		newProjectionEvent("text-final", "inv-1", "jftrade_default", genai.RoleModel,
			[]*genai.Part{{Text: "最终回答"}}, at(4), false),
	}

	projection := sessionProjectionFromADKEvents(events)
	var assistant *TranscriptEntry
	for index := range projection.Messages {
		if projection.Messages[index].Role == "assistant" {
			assistant = &projection.Messages[index]
		}
	}
	if assistant == nil {
		t.Fatalf("assistant entry missing: %+v", projection.Messages)
	}
	if assistant.ID != "text-final" {
		t.Fatalf("assistant entry ID = %q, want text-final", assistant.ID)
	}
	want := at(4).UTC().Format(time.RFC3339Nano)
	if assistant.CreatedAt != want {
		t.Fatalf("assistant entry CreatedAt = %q, want latest text event %q", assistant.CreatedAt, want)
	}
}
