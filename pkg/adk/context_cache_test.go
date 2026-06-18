package adk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type capturedChatProvider struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []openAIChatRequest
}

func newCapturedChatProvider(t *testing.T, runtime *Runtime, id string) *capturedChatProvider {
	t.Helper()
	captured := &capturedChatProvider{}
	captured.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		defer r.Body.Close()
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		captured.mu.Lock()
		captured.requests = append(captured.requests, req)
		captured.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIChatResponse{
			Choices: []struct {
				Message openAIChatMessage `json:"message"`
			}{{Message: openAIChatMessage{Role: "assistant", Content: testProviderFinalReply(req)}}},
		})
	}))
	t.Cleanup(captured.server.Close)
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID:          id,
		DisplayName: id,
		BaseURL:     captured.server.URL,
		Model:       "test-model",
		APIKey:      "sk-test",
		Enabled:     true,
	})
	return captured
}

func (p *capturedChatProvider) Requests() []openAIChatRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]openAIChatRequest, len(p.requests))
	copy(out, p.requests)
	return out
}

func TestProviderPayloadKeepsStablePrefixAcrossTurns(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	provider := newCapturedChatProvider(t, runtime, "cache-prefix-provider")
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:            "cache-prefix-agent",
		Name:          "Cache Prefix Agent",
		ProviderID:    "cache-prefix-provider",
		Instruction:   "Stable instruction for automatic prompt cache.",
		Tools:         []string{"missing.none"},
		WorkMode:      WorkModeChat,
		Status:        AgentStatusEnabled,
		MemoryEnabled: false,
	})

	first, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "first cacheable turn"})
	if err != nil {
		t.Fatalf("first Chat: %v", err)
	}
	if _, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, SessionID: first.Session.ID, Message: "second cacheable turn"}); err != nil {
		t.Fatalf("second Chat: %v", err)
	}

	requests := provider.Requests()
	if len(requests) != 2 {
		t.Fatalf("captured requests = %d, want 2", len(requests))
	}
	firstReq, secondReq := requests[0], requests[1]
	if len(firstReq.Messages) < 2 || len(secondReq.Messages) < 4 {
		t.Fatalf("messages first=%+v second=%+v", firstReq.Messages, secondReq.Messages)
	}
	if firstReq.Messages[0].Role != "system" || secondReq.Messages[0].Role != "system" {
		t.Fatalf("first message roles = %q/%q, want system/system", firstReq.Messages[0].Role, secondReq.Messages[0].Role)
	}
	if firstReq.Messages[0].Content != secondReq.Messages[0].Content {
		t.Fatalf("system prefix changed:\nfirst=%q\nsecond=%q", firstReq.Messages[0].Content, secondReq.Messages[0].Content)
	}
	if !strings.Contains(firstReq.Messages[0].Content, "Stable instruction for automatic prompt cache.") {
		t.Fatalf("system prefix = %q", firstReq.Messages[0].Content)
	}
	if !reflect.DeepEqual(firstReq.Tools, secondReq.Tools) {
		t.Fatalf("tools changed across turns:\nfirst=%+v\nsecond=%+v", firstReq.Tools, secondReq.Tools)
	}
	if firstReq.Messages[1].Role != "user" || firstReq.Messages[1].Content != "first cacheable turn" {
		t.Fatalf("first request first user = %+v", firstReq.Messages[1])
	}
	if !reflect.DeepEqual(firstReq.Messages[1], secondReq.Messages[1]) {
		t.Fatalf("prior user message changed in second payload:\nfirst=%+v\nsecond=%+v", firstReq.Messages[1], secondReq.Messages[1])
	}
	if secondReq.Messages[len(secondReq.Messages)-1].Role != "user" || secondReq.Messages[len(secondReq.Messages)-1].Content != "second cacheable turn" {
		t.Fatalf("second payload latest message = %+v", secondReq.Messages[len(secondReq.Messages)-1])
	}
	assertProviderMessagesDoNotContain(t, secondReq.Messages, "run-", "stream", "replay", "contextRevisionId", "rawBreakdown")
}

func TestProviderPayloadUsesOnlyCurrentContextRevisionHandoff(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	provider := newCapturedChatProvider(t, runtime, "cache-handoff-provider")
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:          "cache-handoff-agent",
		Name:        "Cache Handoff Agent",
		ProviderID:  "cache-handoff-provider",
		Instruction: "Base stable instruction.",
		Tools:       []string{"missing.none"},
		WorkMode:    WorkModeChat,
		Status:      AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Cache Handoff")
	if _, err := runtime.Store().SaveSessionContext(ctx, SessionContextState{
		SessionID:                 session.ID,
		ContextRevisionID:         "ctx-current-cache",
		PreviousContextRevisionID: "ctx-old-cache",
		ContextRevisionCreatedAt:  "2026-06-18T10:00:00Z",
	}); err != nil {
		t.Fatalf("SaveSessionContext: %v", err)
	}
	if _, err := runtime.Store().SaveHandoffSegment(ctx, HandoffSegment{
		ID:                "handoff-old-cache",
		SessionID:         session.ID,
		ContextRevisionID: "ctx-old-cache",
		Sequence:          1,
		StartEventIndex:   0,
		EndEventIndex:     2,
		Summary:           "OLD_REVISION_SUMMARY_SHOULD_NOT_BE_SENT",
		Mode:              "manual",
		EstimatedTokens:   8,
		Active:            true,
	}); err != nil {
		t.Fatalf("Save old handoff: %v", err)
	}
	if _, err := runtime.Store().SaveHandoffSegment(ctx, HandoffSegment{
		ID:                "handoff-current-cache",
		SessionID:         session.ID,
		ContextRevisionID: "ctx-current-cache",
		Sequence:          1,
		StartEventIndex:   0,
		EndEventIndex:     2,
		Summary:           "CURRENT_REVISION_SUMMARY_SHOULD_BE_SENT",
		Mode:              "manual",
		EstimatedTokens:   8,
		Active:            true,
	}); err != nil {
		t.Fatalf("Save current handoff: %v", err)
	}

	if _, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, SessionID: session.ID, Message: "use current handoff"}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	requests := provider.Requests()
	if len(requests) != 1 {
		t.Fatalf("captured requests = %d, want 1", len(requests))
	}
	system := firstSystemMessage(requests[0].Messages)
	if !strings.Contains(system, "Base stable instruction.") {
		t.Fatalf("system missing base instruction: %q", system)
	}
	if !strings.Contains(system, "Session handoff summaries:") || !strings.Contains(system, "CURRENT_REVISION_SUMMARY_SHOULD_BE_SENT") {
		t.Fatalf("system missing current handoff: %q", system)
	}
	if strings.Contains(system, "OLD_REVISION_SUMMARY_SHOULD_NOT_BE_SENT") {
		t.Fatalf("system includes old revision handoff: %q", system)
	}
	assertProviderMessagesDoNotContain(t, requests[0].Messages, "jftrade:handoff_summary", "jftrade-handoff-state", "ctx-current-cache", "ctx-old-cache")
}

func TestProviderPayloadSortsToolsByNameIndependentOfAgentInputOrder(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	provider := newCapturedChatProvider(t, runtime, "cache-tools-provider")
	firstAgent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:         "cache-tools-agent-a",
		Name:       "Cache Tools A",
		ProviderID: "cache-tools-provider",
		Tools:      []string{"tools.search", "http.fetch"},
		WorkMode:   WorkModeChat,
		Status:     AgentStatusEnabled,
	})
	secondAgent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:         "cache-tools-agent-b",
		Name:       "Cache Tools B",
		ProviderID: "cache-tools-provider",
		Tools:      []string{"http.fetch", "tools.search"},
		WorkMode:   WorkModeChat,
		Status:     AgentStatusEnabled,
	})

	if _, err := runtime.Chat(ctx, ChatRequest{AgentID: firstAgent.ID, Message: "hello"}); err != nil {
		t.Fatalf("first Chat: %v", err)
	}
	if _, err := runtime.Chat(ctx, ChatRequest{AgentID: secondAgent.ID, Message: "hello"}); err != nil {
		t.Fatalf("second Chat: %v", err)
	}
	requests := provider.Requests()
	if len(requests) != 2 {
		t.Fatalf("captured requests = %d, want 2", len(requests))
	}
	firstTools := restoredToolNames(requests[0].Tools)
	secondTools := restoredToolNames(requests[1].Tools)
	want := []string{"http.fetch", "tools.search"}
	if !reflect.DeepEqual(firstTools, want) {
		t.Fatalf("first tools = %#v, want %#v", firstTools, want)
	}
	if !reflect.DeepEqual(secondTools, want) {
		t.Fatalf("second tools = %#v, want %#v", secondTools, want)
	}
}

func firstSystemMessage(messages []openAIChatMessage) string {
	for _, message := range messages {
		if message.Role == "system" {
			return message.Content
		}
	}
	return ""
}

func restoredToolNames(tools []openAITool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		name := restoreToolNameFromOpenAI(tool.Function.Name)
		if name == "" {
			name = tool.Function.Name
		}
		names = append(names, name)
	}
	return names
}

func assertProviderMessagesDoNotContain(t *testing.T, messages []openAIChatMessage, needles ...string) {
	t.Helper()
	raw, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	payload := string(raw)
	for _, needle := range needles {
		if strings.Contains(payload, needle) {
			t.Fatalf("provider messages unexpectedly contain %q: %s", needle, payload)
		}
	}
}
