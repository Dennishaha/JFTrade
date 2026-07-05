package adk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	"google.golang.org/genai"
)

func TestSessionContextProjectionBucketsAndTrimsProtectedToolOutput(t *testing.T) {
	largeResponse := strings.Repeat("tool-output ", MaxToolOutputBytes)
	toolEvent := adksession.NewEvent(context.Background(), "ctx-tool-large")
	toolEvent.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID:   "call-large",
			Name: "strategy.research_backtest",
			Response: map[string]any{
				"stdout": largeResponse,
			},
		},
	}}, genai.RoleModel)
	events := []*adksession.Event{
		newContextTextEvent("ctx-old", "old model context", genai.RoleModel),
		newContextTextEvent("ctx-user-recent", "recent user asks about risk", genai.RoleUser),
		toolEvent,
		newContextTextEvent("ctx-tail-user", "approval pending", genai.RoleUser),
		newContextApprovalEvent("ctx-approval"),
	}

	projected := projectVisibleSessionEvents(events, true, 1, 1, 2)
	if len(projected.events) != 4 {
		t.Fatalf("projected events = %d, want recent user plus protected tail", len(projected.events))
	}
	if projected.rawBreakdown.RecentUserTokens == 0 || projected.rawBreakdown.ProtectedTailTokens == 0 {
		t.Fatalf("raw breakdown = %+v", projected.rawBreakdown)
	}
	if projected.trimmedToolResponseCount != 1 {
		t.Fatalf("trimmed tool responses = %d, want 1", projected.trimmedToolResponseCount)
	}
	if projected.rawBreakdown.ProtectedTailTokens <= projected.effectiveBreakdown.ProtectedTailTokens {
		t.Fatalf("tool output was not reduced: raw=%+v effective=%+v", projected.rawBreakdown, projected.effectiveBreakdown)
	}
	if toolEvent.Content.Parts[0].FunctionResponse.Response["stdout"] != largeResponse {
		t.Fatal("projection mutated the raw ADK event")
	}

	unmapped := asToolResponseMap("plain")
	if unmapped["result"] != "plain" {
		t.Fatalf("asToolResponseMap string = %#v", unmapped)
	}
	addProjectedEventTokens(nil, contextProjectionBucketRecentUser, events[1])
	addProjectedEventTokens(&projected.rawBreakdown, contextProjectionBucketRecentUser, nil)
}

func TestSessionContextSummaryAndBoundaryHelpersUseBusinessSemantics(t *testing.T) {
	events := []*adksession.Event{
		newContextTextEvent("ctx-user", strings.Repeat("用户目标 ", 80), genai.RoleUser),
		newContextFunctionCallEvent("ctx-tool-call", "call-tool"),
		newContextApprovalEvent("ctx-approval"),
		newContextFunctionResponseEvent("ctx-tool-result", "call-tool", "strategy.research_backtest"),
	}
	segments := []HandoffSegment{
		{Summary: " first prior handoff ", Sequence: 2, EndEventIndex: 3, Active: true},
		{Summary: "", Sequence: 5, EndEventIndex: 10, Active: false},
		{Summary: "latest active handoff", Sequence: 7, EndEventIndex: 6, Active: true},
	}

	summary := buildHandoffSummary(segments, events, "aggressive")
	for _, fragment := range []string{"Prior handoff:", "first prior handoff", "latest active handoff", "Conversation material:", "User:", "Approval requested"} {
		if !strings.Contains(summary, fragment) {
			t.Fatalf("summary = %q, missing %q", summary, fragment)
		}
	}
	if got := joinSegmentSummaries(segments); !strings.Contains(got, "first prior handoff\n\nlatest active handoff") {
		t.Fatalf("joinSegmentSummaries = %q", got)
	}
	if latest := latestSegmentPreview(segments); latest != "latest active handoff" {
		t.Fatalf("latestSegmentPreview = %q", latest)
	}
	if got := maxActiveSegmentEnd(segments); got != 6 {
		t.Fatalf("maxActiveSegmentEnd = %d, want active max only", got)
	}
	if got := nextHandoffSequence(segments); got != 8 {
		t.Fatalf("nextHandoffSequence = %d, want max sequence plus one", got)
	}
	if got := marshalCompactJSON(complex(1, 2)); !strings.Contains(got, "1+2i") {
		t.Fatalf("marshalCompactJSON fallback = %q", got)
	}

	if contextStatus(0, 0) != ContextStatusUnknown ||
		contextStatus(60, 100) != ContextStatusHealthy ||
		contextStatus(70, 100) != ContextStatusWarning ||
		contextStatus(85, 100) != ContextStatusNearLimit ||
		contextStatus(93, 100) != ContextStatusCritical {
		t.Fatal("contextStatus thresholds changed")
	}
	if compactionModeLabel(SessionCompactRequest{Mode: "normal", Trigger: "auto"}) != "auto" ||
		compactionModeLabel(SessionCompactRequest{Mode: "aggressive", Trigger: "auto"}) != "aggressive" ||
		compactionModeLabel(SessionCompactRequest{Mode: "normal"}) != "manual" {
		t.Fatal("compactionModeLabel business labels changed")
	}
	if normalizeCompactMode(" aggressive ") != "aggressive" || normalizeCompactMode("manual") != "normal" {
		t.Fatal("normalizeCompactMode changed")
	}
	if got := compactionCutoff(events, 1); got != 2 {
		t.Fatalf("compactionCutoff = %d, want protected approval original boundary", got)
	}
	if got := retainedRecentUserCount(events, len(events), 0); got != 0 {
		t.Fatalf("retainedRecentUserCount out of range = %d", got)
	}
}

func TestSessionContextManagerAndWrappedEventsBoundaryHelpers(t *testing.T) {
	if NewSessionContextManager(nil, adksession.InMemoryService(), openAIClient{}, nil) != nil {
		t.Fatal("manager with nil store should be unavailable")
	}
	if NewSessionContextManager(&Store{}, nil, openAIClient{}, nil) != nil {
		t.Fatal("manager with nil raw session service should be unavailable")
	}
	base := adksession.InMemoryService()
	manager := &SessionContextManager{}
	if got := manager.WrapService(nil); got != nil {
		t.Fatalf("WrapService nil service = %#v", got)
	}
	if got := (*SessionContextManager)(nil).WrapService(base); got != base {
		t.Fatalf("nil manager WrapService = %#v, want base", got)
	}
	if suffix, err := (*SessionContextManager)(nil).InstructionSuffix(context.Background(), "session"); err != nil || suffix != "" {
		t.Fatalf("nil InstructionSuffix = %q/%v", suffix, err)
	}
	if stateTextValue(nil, "missing") != "" {
		t.Fatal("nil ADK state should read as empty")
	}
	state := &emptyState{values: map[string]any{"key": " value "}}
	if stateTextValue(state, "key") != "value" || stateTextValue(state, "missing") != "" {
		t.Fatal("stateTextValue should trim present values and ignore missing keys")
	}

	first := newContextTextEvent("ctx-first", "first", genai.RoleUser)
	first.ID = "ctx-first"
	first.Timestamp = time.Unix(10, 0)
	second := newContextTextEvent("ctx-second", "second", genai.RoleModel)
	second.ID = "ctx-second"
	second.Timestamp = time.Unix(20, 0)
	filtered := filterEvents([]*adksession.Event{nil, first, second}, time.Unix(15, 0), 1)
	if len(filtered) != 1 || filtered[0].ID != "ctx-second" {
		t.Fatalf("filterEvents = %#v", filtered)
	}

	events := &wrappedEvents{items: []*adksession.Event{first}}
	if events.Len() != 1 || events.At(-1) != nil || events.At(2) != nil || events.At(0).ID != "ctx-first" {
		t.Fatalf("wrappedEvents initial state len=%d at0=%#v", events.Len(), events.At(0))
	}
	events.Append(nil)
	events.Append(first)
	events.Append(second)
	if events.Len() != 2 {
		t.Fatalf("wrappedEvents should append non-duplicate non-nil events, len=%d", events.Len())
	}
	seen := []string{}
	for event := range events.All() {
		seen = append(seen, event.ID)
	}
	if strings.Join(seen, ",") != "ctx-first,ctx-second" {
		t.Fatalf("wrappedEvents All = %#v", seen)
	}
	wrapped := &wrappedSession{base: &emptySession{id: "session", appName: "app", userID: "user", state: state, events: events}, events: events}
	if wrapped.ID() != "session" || wrapped.AppName() != "app" || wrapped.UserID() != "user" || wrapped.State() != state || wrapped.Events().Len() != 2 {
		t.Fatalf("wrapped session = %#v", wrapped)
	}
	if !isSyntheticADKSession(wrapped) || !isSyntheticADKSession(&emptySession{}) || isSyntheticADKSession(nil) {
		t.Fatal("synthetic ADK session detection changed")
	}
}

func TestSessionContextApprovalResolutionAndEventIndexBoundaries(t *testing.T) {
	approval := newContextApprovalEvent("ctx-approval")
	response := adksession.NewEvent(context.Background(), "ctx-approval-response")
	response.Content = genai.NewContentFromParts([]*genai.Part{
		nil,
		{FunctionResponse: &genai.FunctionResponse{ID: "ctx-approval-call", Name: toolconfirmation.FunctionCallName, Response: map[string]any{"approved": true}}},
	}, genai.RoleUser)
	events := []*adksession.Event{
		nil,
		newContextFunctionCallEvent("ctx-call", "call-live"),
		approval,
		response,
	}
	if _, ok := resolvedApprovalIDs(events)["ctx-approval-call"]; !ok {
		t.Fatalf("resolvedApprovalIDs = %#v", resolvedApprovalIDs(events))
	}
	if got := functionCallEventIndex(events, "call-live", 99); got != 1 {
		t.Fatalf("functionCallEventIndex = %d, want clamped search hit", got)
	}
	if got := functionCallEventIndex(events, " ", 99); got != -1 {
		t.Fatalf("blank functionCallEventIndex = %d", got)
	}
	if protected := protectedTailStart(events); protected != len(events) {
		t.Fatalf("resolved approval should not protect tail, got %d", protected)
	}
}

func TestSessionContextAdditionalPureHelperBoundaries(t *testing.T) {
	nilPart := adksession.NewEvent(context.Background(), "ctx-nil-part")
	nilPart.Content = genai.NewContentFromParts([]*genai.Part{nil}, genai.RoleModel)
	userByAuthor := adksession.NewEvent(context.Background(), "ctx-user-author")
	userByAuthor.Author = " USER "
	userByAuthor.Content = genai.NewContentFromText("author user", genai.RoleModel)
	plainCall := adksession.NewEvent(context.Background(), "ctx-plain-call")
	plainCall.Content = genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
		ID: "plain-call", Name: "tools.search", Args: map[string]any{"query": "risk"},
	}}}, genai.RoleModel)
	response := newContextFunctionResponseEvent("ctx-response", "plain-call", "tools.search")
	events := []*adksession.Event{nilPart, userByAuthor, plainCall, response}

	projected := projectVisibleSessionEvents(events, true, -10, -5, -2)
	if len(projected.events) != len(events) {
		t.Fatalf("projected events = %d, want protected full tail", len(projected.events))
	}
	if got := retainedRecentUserCount(events, 1, 0); got != 0 {
		t.Fatalf("retainedRecentUserCount adjusted protected start = %d, want empty protected window", got)
	}
	if got := retainedRecentUserCount(events, 1, 3); got != 1 {
		t.Fatalf("retainedRecentUserCount visible user = %d, want 1", got)
	}
	emptyContent := adksession.NewEvent(context.Background(), "ctx-empty-content")
	emptyContent.Content = &genai.Content{Parts: []*genai.Part{nil}}
	if got := functionCallEventIndex([]*adksession.Event{nil, emptyContent}, "missing", 99); got != -1 {
		t.Fatalf("missing functionCallEventIndex = %d, want -1", got)
	}
	if got := summarizeEvent(nil, 20); len(got) != 0 {
		t.Fatalf("nil summarizeEvent = %#v", got)
	}
	if got := summarizeEvent(&adksession.Event{}, 20); len(got) != 0 {
		t.Fatalf("empty summarizeEvent = %#v", got)
	}
	summary := strings.Join(summarizeEvent(plainCall, 120), "\n")
	if !strings.Contains(summary, "Tool call tools.search") {
		t.Fatalf("plain call summary = %q", summary)
	}

	if estimateToolDeclarationTokens(Agent{}, nil) != 0 {
		t.Fatal("nil registry should have no tool declaration tokens")
	}
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "ctx.custom", Description: "custom context tool", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	if estimateToolDeclarationTokens(Agent{Tools: []string{"ctx.custom"}}, registry) == 0 {
		t.Fatal("registered tool declaration should add token estimate")
	}
	if got := estimateEventTokens(emptyContent); got != 0 {
		t.Fatalf("nil part event tokens = %d, want 0", got)
	}
	if got := eventSlice(nil); got != nil {
		t.Fatalf("nil eventSlice = %#v", got)
	}

	if isStaleADKSessionError(nil) || isRefreshableADKSessionError(nil) {
		t.Fatal("nil errors should not be refreshable")
	}
	if !isStaleADKSessionError(errors.New("Stale Session Error: old update")) {
		t.Fatal("stale ADK session error was not detected")
	}
	if !isRefreshableADKSessionError(errors.New("unexpected session type *adk.wrappedSession")) {
		t.Fatal("unexpected session type should be refreshable")
	}
	if serviceAppendLocks(nil).len() != 0 || runtimeAppendLocks(nil).len() != 0 {
		t.Fatal("fresh append lock maps should start empty")
	}
	session := &emptySession{id: "s", appName: "a", userID: "u", state: &emptyState{values: map[string]any{}}, events: &wrappedEvents{}}
	var nilMap *adkSessionAppendLockMap
	_, release := nilMap.acquire(session)
	release()
	if nilMap.len() != 0 {
		t.Fatal("nil append lock map len should stay zero")
	}

	state := &emptyState{values: map[string]any{"a": 1, "b": 2}}
	seen := 0
	for range state.All() {
		seen++
		break
	}
	if seen != 1 {
		t.Fatalf("emptyState early stop seen = %d, want 1", seen)
	}
	if maxInt(3, 9) != 9 {
		t.Fatal("maxInt should return second value when larger")
	}
}
