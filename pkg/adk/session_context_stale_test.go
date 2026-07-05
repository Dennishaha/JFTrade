package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	"google.golang.org/genai"
)

func appendContextEvents(t *testing.T, service adksession.Service, session adksession.Session, start int, count int) {
	t.Helper()
	for index := start; index < start+count; index++ {
		role := genai.Role(genai.RoleUser)
		if index%2 == 1 {
			role = genai.Role(genai.RoleModel)
		}
		event := adksession.NewEvent(context.Background(), fmt.Sprintf("ctx-%d", index))
		event.Content = genai.NewContentFromText(fmt.Sprintf("message %d", index), role)
		if err := appendADKEventWithStaleRetry(context.Background(), newADKSessionAppendLockMap(), service, session, event); err != nil {
			t.Fatalf("Append context event %d: %v", index, err)
		}
	}
}

func appendLargeContextEvents(t *testing.T, service adksession.Service, session adksession.Session, start int, count int) {
	t.Helper()
	for index := start; index < start+count; index++ {
		role := genai.Role(genai.RoleUser)
		if index%2 == 1 {
			role = genai.Role(genai.RoleModel)
		}
		event := newContextTextEvent(fmt.Sprintf("large-ctx-%d", index), strings.Repeat(fmt.Sprintf("message %d ", index), 50), role)
		if err := appendADKEventWithStaleRetry(context.Background(), newADKSessionAppendLockMap(), service, session, event); err != nil {
			t.Fatalf("Append large context event %d: %v", index, err)
		}
	}
}

func newContextTextEvent(id string, text string, role genai.Role) *adksession.Event {
	event := adksession.NewEvent(context.Background(), id)
	event.Content = genai.NewContentFromText(text, role)
	return event
}

func newContextApprovalEvent(id string) *adksession.Event {
	return newContextApprovalEventForOriginal(id, "")
}

func newContextApprovalEventForOriginal(id string, originalCallID string) *adksession.Event {
	event := adksession.NewEvent(context.Background(), id)
	args := map[string]any{}
	if originalCallID != "" {
		args["originalFunctionCall"] = &genai.FunctionCall{
			ID: originalCallID, Name: "strategy.research_backtest", Args: map[string]any{"symbol": "TME"},
		}
	}
	event.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionCall: &genai.FunctionCall{
			ID:   id + "-call",
			Name: toolconfirmation.FunctionCallName,
			Args: args,
		},
	}}, genai.RoleModel)
	return event
}

func newContextFunctionCallEvent(id string, functionCallID string) *adksession.Event {
	event := adksession.NewEvent(context.Background(), id)
	event.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionCall: &genai.FunctionCall{
			ID: functionCallID, Name: "strategy.research_backtest", Args: map[string]any{"symbol": "TME"},
		},
	}}, genai.RoleModel)
	return event
}

func newContextFunctionResponseEvent(id string, functionCallID string, name string) *adksession.Event {
	event := adksession.NewEvent(context.Background(), id)
	event.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID: functionCallID, Name: name, Response: map[string]any{"error": "confirmation required"},
		},
	}}, genai.RoleUser)
	return event
}

func newContextApprovalResponseEvent(approvalEventID string) *adksession.Event {
	event := adksession.NewEvent(context.Background(), approvalEventID+"-response")
	event.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID:       approvalEventID + "-call",
			Name:     toolconfirmation.FunctionCallName,
			Response: map[string]any{"approved": true},
		},
	}}, genai.RoleUser)
	return event
}

func TestAppendADKEventWithStaleRetrySerializesConcurrentStaleSession(t *testing.T) {
	ctx := context.Background()
	service, err := NewSQLiteSessionService(t.TempDir() + "/adk-session.db")
	if err != nil {
		t.Fatalf("NewSQLiteSessionService: %v", err)
	}
	t.Cleanup(func() { jftradeErr2 := CloseSessionService(service); jftradeCheckTestError(t, jftradeErr2) })
	if err := ValidateSQLiteSessionService(service); err != nil {
		t.Fatalf("ValidateSQLiteSessionService: %v", err)
	}
	created, err := service.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "session-concurrent-stale-retry",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const eventCount = 12
	locks := newADKSessionAppendLockMap()
	var wg sync.WaitGroup
	errs := make(chan error, eventCount)
	for index := range eventCount {
		wg.Go(func() {
			event := adksession.NewEvent(context.Background(), fmt.Sprintf("inv-concurrent-%02d", index))
			event.Author = "agent"
			event.Content = genai.NewContentFromText(fmt.Sprintf("event-%02d", index), genai.RoleModel)
			if err := appendADKEventWithStaleRetry(ctx, locks, service, created.Session, event); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("appendADKEventWithStaleRetry concurrent error: %v", err)
	}

	latest, err := service.Get(ctx, &adksession.GetRequest{
		AppName: "app", UserID: "user", SessionID: "session-concurrent-stale-retry",
	})
	if err != nil {
		t.Fatalf("Get latest: %v", err)
	}
	if latest.Session.Events().Len() != eventCount {
		t.Fatalf("event count = %d, want %d", latest.Session.Events().Len(), eventCount)
	}
	if locks.len() != 0 {
		t.Fatalf("append lock count = %d, want 0", locks.len())
	}
}

func TestAppendADKEventWithStaleRetryReturnsNonStaleError(t *testing.T) {
	ctx := context.Background()
	base := adksession.InMemoryService()
	created, err := base.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "session-non-stale-error",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	appendErr := errors.New("disk full")
	service := &appendErrorSessionService{Service: base, err: appendErr}
	event := adksession.NewEvent(context.Background(), "inv-non-stale")
	event.Author = "agent"
	event.Content = genai.NewContentFromText("non-stale", genai.RoleModel)

	locks := newADKSessionAppendLockMap()
	err = appendADKEventWithStaleRetry(ctx, locks, service, created.Session, event)
	if !errors.Is(err, appendErr) {
		t.Fatalf("append error = %v, want %v", err, appendErr)
	}
	if locks.len() != 0 {
		t.Fatalf("append lock count = %d, want 0", locks.len())
	}
	if service.getCalls != 0 {
		t.Fatalf("Get calls = %d, want 0 for non-stale error", service.getCalls)
	}
}

func TestAppendADKEventWithStaleRetryRefreshesUnexpectedSessionType(t *testing.T) {
	ctx := context.Background()
	base := adksession.InMemoryService()
	created, err := base.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "session-refresh-type",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	service := &refreshSessionTypeService{Service: base}
	event := adksession.NewEvent(context.Background(), "inv-refresh-type")
	event.Author = "agent"
	event.Content = genai.NewContentFromText("refreshed", genai.RoleModel)
	if err := appendADKEventWithStaleRetry(ctx, newADKSessionAppendLockMap(), service, created.Session, event); err != nil {
		t.Fatalf("appendADKEventWithStaleRetry: %v", err)
	}
	if service.getCalls != 1 || service.appendCalls != 2 {
		t.Fatalf("refresh calls get=%d append=%d, want 1 and 2", service.getCalls, service.appendCalls)
	}
}

func TestAppendADKEventWithStaleRetryRefreshesSyntheticSessionBeforeAppend(t *testing.T) {
	ctx := context.Background()
	base := adksession.InMemoryService()
	created, err := base.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "session-refresh-synthetic",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	service := &rejectSyntheticAppendSessionService{Service: base}
	event := adksession.NewEvent(context.Background(), "inv-refresh-synthetic")
	event.Author = "agent"
	event.Content = genai.NewContentFromText("refreshed", genai.RoleModel)
	projected := &wrappedSession{base: created.Session, events: &wrappedEvents{}}
	if err := appendADKEventWithStaleRetry(ctx, newADKSessionAppendLockMap(), service, projected, event); err != nil {
		t.Fatalf("appendADKEventWithStaleRetry: %v", err)
	}
	if service.getCalls != 1 || service.appendCalls != 1 {
		t.Fatalf("refresh calls get=%d append=%d, want 1 and 1", service.getCalls, service.appendCalls)
	}
}

func TestSyncHandoffStateSkipsMissingRawADKSession(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "missing-raw-context-agent", Name: "Missing Raw Context", Instruction: "Test agent",
		Status: AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Missing raw ADK session")
	service := &countingAppendSessionService{Service: runtime.rawSessionService}
	runtime.contextManager.rawService = service
	if _, err := runtime.Store().SaveSessionContext(ctx, SessionContextState{SessionID: session.ID}); err != nil {
		t.Fatalf("SaveSessionContext: %v", err)
	}
	if err := runtime.contextManager.syncHandoffStateForSession(ctx, session); err != nil {
		t.Fatalf("syncHandoffStateForSession: %v", err)
	}
	if service.appendCalls != 0 {
		t.Fatalf("AppendEvent calls = %d, want 0 for missing raw ADK session", service.appendCalls)
	}
}

type appendErrorSessionService struct {
	adksession.Service
	err      error
	getCalls int
}

type countingAppendSessionService struct {
	adksession.Service
	appendCalls int
}

func (s *countingAppendSessionService) AppendEvent(ctx context.Context, session adksession.Session, event *adksession.Event) error {
	s.appendCalls++
	return s.Service.AppendEvent(ctx, session, event)
}

type refreshSessionTypeService struct {
	adksession.Service
	getCalls    int
	appendCalls int
}

type rejectSyntheticAppendSessionService struct {
	adksession.Service
	getCalls    int
	appendCalls int
}

func (s *refreshSessionTypeService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	s.getCalls++
	return s.Service.Get(ctx, req)
}

func (s *refreshSessionTypeService) AppendEvent(ctx context.Context, session adksession.Session, event *adksession.Event) error {
	s.appendCalls++
	if s.appendCalls == 1 {
		return fmt.Errorf("unexpected session type %T", session)
	}
	return s.Service.AppendEvent(ctx, session, event)
}

func (s *rejectSyntheticAppendSessionService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	s.getCalls++
	return s.Service.Get(ctx, req)
}

func (s *rejectSyntheticAppendSessionService) AppendEvent(ctx context.Context, session adksession.Session, event *adksession.Event) error {
	s.appendCalls++
	if isSyntheticADKSession(session) {
		return fmt.Errorf("unexpected session type %T", session)
	}
	return s.Service.AppendEvent(ctx, session, event)
}

func (s *appendErrorSessionService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	s.getCalls++
	return s.Service.Get(ctx, req)
}

func (s *appendErrorSessionService) AppendEvent(context.Context, adksession.Session, *adksession.Event) error {
	return s.err
}

func TestSessionContextProjectionTrimsOversizedToolResponses(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	provider, err := runtime.Store().SaveProvider(ctx, ProviderWriteRequest{
		ID:                  "context-provider",
		DisplayName:         "Context Provider",
		BaseURL:             "https://api.openai.com/v1",
		Model:               "gpt-4o-mini",
		ContextWindowTokens: 200000,
		Enabled:             true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:               "context-trim-agent",
		Name:             "Context Trim Agent",
		Instruction:      "Trim oversized tool responses.",
		ProviderID:       provider.ID,
		RecentUserWindow: 2,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "Oversized Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}

	userEvent := adksession.NewEvent(context.Background(), "inv-user")
	userEvent.Content = genai.NewContentFromText("制作一个 tme 的策略", genai.RoleUser)
	if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, userEvent); err != nil {
		t.Fatalf("Append user event: %v", err)
	}

	oversizedPayload := strings.Repeat("x", MaxToolOutputBytes*2)
	toolEvent := adksession.NewEvent(context.Background(), "inv-tool")
	toolEvent.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID:   "call-oversized",
			Name: "backtest.runs",
			Response: map[string]any{
				"payload": oversizedPayload,
			},
		},
	}}, genai.RoleModel)
	if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, toolEvent); err != nil {
		t.Fatalf("Append tool event: %v", err)
	}

	snapshot, err := runtime.contextManager.Snapshot(ctx, session, agent)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshot.TrimmedToolResponseCount != 1 {
		t.Fatalf("TrimmedToolResponseCount = %d, want 1", snapshot.TrimmedToolResponseCount)
	}
	if snapshot.RawCurrentInputTokens <= snapshot.CurrentInputTokens {
		t.Fatalf("raw current tokens = %d, effective = %d, want raw > effective", snapshot.RawCurrentInputTokens, snapshot.CurrentInputTokens)
	}
	if snapshot.RawBreakdown.OtherVisibleTokens <= snapshot.Breakdown.OtherVisibleTokens {
		t.Fatalf("raw other visible = %d, effective = %d, want raw > effective", snapshot.RawBreakdown.OtherVisibleTokens, snapshot.Breakdown.OtherVisibleTokens)
	}

	response, err := runtime.sessionService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("wrapped Get: %v", err)
	}
	foundTruncated := false
	for event := range response.Session.Events().All() {
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part == nil || part.FunctionResponse == nil {
				continue
			}
			if truncated, ok := part.FunctionResponse.Response["truncated"].(bool); ok && truncated {
				foundTruncated = true
			}
			if payload, ok := part.FunctionResponse.Response["payload"].(string); ok && payload == oversizedPayload {
				t.Fatal("wrapped session still exposes oversized raw payload")
			}
		}
	}
	if !foundTruncated {
		t.Fatal("expected wrapped session to expose truncated tool response preview")
	}
}

func TestSessionContextProjectionKeepsSmallToolResponsesUntouched(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:               "context-small-agent",
		Name:             "Context Small Agent",
		Instruction:      "Keep small tool responses intact.",
		RecentUserWindow: 2,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "Small Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}

	toolEvent := adksession.NewEvent(context.Background(), "inv-tool-small")
	toolEvent.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID:   "call-small",
			Name: "strategy.definitions",
			Response: map[string]any{
				"definitions": []map[string]any{{"id": "demo", "name": "Demo"}},
			},
		},
	}}, genai.RoleModel)
	if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, toolEvent); err != nil {
		t.Fatalf("Append tool event: %v", err)
	}

	snapshot, err := runtime.contextManager.Snapshot(ctx, session, agent)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snapshot.TrimmedToolResponseCount != 0 {
		t.Fatalf("TrimmedToolResponseCount = %d, want 0", snapshot.TrimmedToolResponseCount)
	}
	if snapshot.RawCurrentInputTokens != snapshot.CurrentInputTokens {
		t.Fatalf("raw current tokens = %d, effective = %d, want equal", snapshot.RawCurrentInputTokens, snapshot.CurrentInputTokens)
	}

	response, err := runtime.sessionService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("wrapped Get: %v", err)
	}
	for event := range response.Session.Events().All() {
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part == nil || part.FunctionResponse == nil {
				continue
			}
			if _, truncated := part.FunctionResponse.Response["truncated"]; truncated {
				t.Fatal("small tool response should not be truncated")
			}
		}
	}
}
