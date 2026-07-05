package adk

import (
	"errors"
	"strings"
	"testing"
)

func TestOpenAIMessageNormalizationAndStreamingBoundaryBranches(t *testing.T) {
	long := strings.Repeat("你", 10)
	truncated := truncateBytes(long, 8)
	if !strings.Contains(truncated, "truncated") || strings.Contains(truncated, "\ufffd") {
		t.Fatalf("truncateBytes multibyte = %q, want marker without replacement", truncated)
	}
	callA := openAITestToolCall("a", "tool_a")
	callB := openAITestToolCall("b", "tool_b")
	normalized := normalizeMessagesForProvider([]openAIChatMessage{
		{Role: " user ", Content: "hi"},
		{Role: "assistant", ToolCalls: []openAIToolCall{callA, callB}},
		{Role: "user", Content: "interrupt active pairing"},
		{Role: "tool", ToolCallID: "b", Content: "late b"},
		{Role: "tool", ToolCallID: "a", Content: "late a"},
		{Role: "tool", ToolCallID: "", Content: "drop blank"},
		{Role: "tool", ToolCallID: "missing", Content: "drop missing"},
		{Role: "assistant", ToolCalls: []openAIToolCall{openAITestToolCall("", "ignored")}},
	})
	if len(normalized) != 7 {
		t.Fatalf("normalized messages len=%d messages=%#v, want 7", len(normalized), normalized)
	}
	if normalized[0].Role != "user" {
		t.Fatalf("normalized user role = %q, want trimmed user", normalized[0].Role)
	}
	if normalized[1].Role != "user" || normalized[1].Content != "interrupt active pairing" {
		t.Fatalf("interrupt message = %#v", normalized[1])
	}
	if normalized[2].Role != "assistant" || len(normalized[2].ToolCalls) != 1 || normalized[2].ToolCalls[0].ID != "b" {
		t.Fatalf("inserted assistant for b = %#v", normalized[2])
	}
	if normalized[4].Role != "assistant" || len(normalized[4].ToolCalls) != 1 || normalized[4].ToolCalls[0].ID != "a" {
		t.Fatalf("inserted assistant for a = %#v", normalized[4])
	}
	trimmed := trimMessagesForProvider([]openAIChatMessage{
		{Role: "system", Content: "system"},
		{Role: "user", Content: strings.Repeat("x", 50000)},
		{Role: "assistant", Content: "older"},
		{Role: "user", Content: "newer"},
	}, 300)
	if len(trimmed) == 0 || trimmed[0].Role != "system" {
		t.Fatalf("trimmed messages = %#v, want system retained", trimmed)
	}
	c := openAIClient{}
	if _, err := c.readStreamingResponse(strings.NewReader("data: {bad-json}\n\n"), nil); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("bad stream err = %v, want decode", err)
	}
	if _, err := c.readStreamingResponse(strings.NewReader("data: {\"error\":{\"message\":\"boom\"}}\n\n"), nil); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error stream err = %v, want boom", err)
	}
	if _, err := c.readStreamingResponse(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"\"}}]}\n\ndata: [DONE]\n\n"), nil); err == nil || !strings.Contains(err.Error(), "empty reply") {
		t.Fatalf("empty stream err = %v, want empty reply", err)
	}
	var deltas []ChatDelta
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"think "}}]}`,
		`data: {"choices":[{"message":{"content":"answer"}}]}`,
		`data: [DONE]`,
		``,
	}, "\n\n")
	result, err := c.readStreamingResponse(strings.NewReader(stream), func(delta ChatDelta) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil || result.Reply != "answer" || result.ReasoningContent != "think" || len(deltas) == 0 {
		t.Fatalf("stream result=%+v deltas=%#v err=%v", result, deltas, err)
	}
	if err := appendStreamChoice(&legacyAssistantContentSplitter{}, &strings.Builder{}, &strings.Builder{}, "x", "", "", func(ChatDelta) error {
		return errors.New("delta failed")
	}); err == nil || !strings.Contains(err.Error(), "delta failed") {
		t.Fatalf("appendStreamChoice err = %v, want delta failed", err)
	}
}

func openAITestToolCall(id string, name string) openAIToolCall {
	call := openAIToolCall{ID: id}
	call.Function.Name = name
	call.Function.Arguments = "{}"
	return call
}
