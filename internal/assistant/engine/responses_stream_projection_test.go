package adk

import (
	"context"
	"testing"

	adkmodel "google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func TestResponsesExecutionSkipsDuplicateFinalTextAfterPartial(t *testing.T) {
	var deltas []ChatDelta
	execution := &googleADKExecution{runID: "run-whitespace", onDelta: func(delta ChatDelta) error {
		deltas = append(deltas, delta)
		return nil
	}}

	partial := func(id string, parts ...*genai.Part) *adksession.Event {
		event := adksession.NewEvent(context.Background(), id)
		event.LLMResponse = adkmodel.LLMResponse{
			Content: genai.NewContentFromParts(parts, genai.RoleModel), Partial: true,
		}
		return event
	}
	for _, event := range []*adksession.Event{
		partial("partial-a", &genai.Part{Text: "Hello"}, &genai.Part{Text: "  think", Thought: true}),
		partial("partial-b", &genai.Part{Text: " world"}, &genai.Part{Text: " more", Thought: true}),
		partial("partial-c", &genai.Part{Text: "\n\nnext"}, &genai.Part{Text: "\nlast  ", Thought: true}),
	} {
		if err := execution.consumeEvent(event); err != nil {
			t.Fatalf("consume partial event: %v", err)
		}
	}
	final := adksession.NewEvent(context.Background(), "final")
	final.LLMResponse = adkmodel.LLMResponse{Content: genai.NewContentFromParts([]*genai.Part{
		{Text: "Hello world\n\nnext"}, {Text: "think more\nlast", Thought: true},
	}, genai.RoleModel)}
	if err := execution.consumeEvent(final); err != nil {
		t.Fatalf("consume final event: %v", err)
	}

	result := execution.result()
	if result.Reply != "Hello world\n\nnext" || result.ReasoningContent != "think more\nlast" || result.SourceEventID != final.ID {
		t.Fatalf("result = %#v", result)
	}
	if len(deltas) != 6 {
		t.Fatalf("deltas = %#v, want six Responses deltas", deltas)
	}
}
