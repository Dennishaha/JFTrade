package adk

import (
	"context"
	"fmt"
	"strings"
	"testing"

	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestSessionContextCompactionShrinksSessionView(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID:               "context-agent",
		Name:             "Context Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 2,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "Context Session")
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
	for index := 0; index < 10; index++ {
		role := genai.Role(genai.RoleUser)
		if index%2 == 1 {
			role = genai.Role(genai.RoleModel)
		}
		event := adksession.NewEvent(fmt.Sprintf("inv-%d", index))
		event.Content = genai.NewContentFromText(fmt.Sprintf("message %d", index), role)
		if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, event); err != nil {
			t.Fatalf("AppendEvent(%d): %v", index, err)
		}
	}

	snapshotBefore, err := runtime.contextManager.Snapshot(ctx, session, agent)
	if err != nil {
		t.Fatalf("Snapshot before: %v", err)
	}
	if snapshotBefore.RawEventCount != 10 {
		t.Fatalf("RawEventCount before = %d, want 10", snapshotBefore.RawEventCount)
	}

	snapshotAfter, err := runtime.contextManager.Compact(ctx, session, agent, SessionCompactRequest{
		Mode:    "normal",
		Trigger: "manual",
		Reason:  "test compaction",
	})
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if snapshotAfter.CompactedEventCount == 0 {
		t.Fatalf("CompactedEventCount = 0, want > 0")
	}
	if snapshotAfter.ProtectedRecentCount >= snapshotAfter.RawEventCount {
		t.Fatalf("ProtectedRecentCount = %d, want less than raw count %d", snapshotAfter.ProtectedRecentCount, snapshotAfter.RawEventCount)
	}
	if snapshotAfter.SummaryPreview == "" {
		t.Fatalf("SummaryPreview is empty")
	}
	rawAfterCompact, err := runtime.rawSessionService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Get raw session after compact: %v", err)
	}
	stateSummary, err := rawAfterCompact.Session.State().Get(adkSessionHandoffSummaryKey)
	if err != nil {
		t.Fatalf("ADK handoff state missing: %v", err)
	}
	if strings.TrimSpace(fmt.Sprint(stateSummary)) == "" {
		t.Fatalf("ADK handoff state is empty")
	}
	suffix, err := runtime.contextManager.InstructionSuffix(ctx, session.ID)
	if err != nil {
		t.Fatalf("InstructionSuffix: %v", err)
	}
	if !strings.Contains(suffix, strings.TrimSpace(fmt.Sprint(stateSummary))) {
		t.Fatalf("InstructionSuffix does not include ADK handoff state")
	}

	response, err := runtime.sessionService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("wrapped Get: %v", err)
	}
	if got, want := response.Session.Events().Len(), snapshotAfter.RetainedRecentUserCount; got != want {
		t.Fatalf("wrapped view len = %d, want %d", got, want)
	}
	for event := range response.Session.Events().All() {
		if !isUserEvent(event) {
			t.Fatalf("wrapped view contains non-user event after compaction")
		}
	}
}

func TestAppendADKEventWithStaleRetryRefreshesSession(t *testing.T) {
	ctx := context.Background()
	service, err := NewSQLiteSessionService(t.TempDir() + "/adk-session.db")
	if err != nil {
		t.Fatalf("NewSQLiteSessionService: %v", err)
	}
	t.Cleanup(func() { _ = CloseSessionService(service) })
	if err := MigrateSQLiteSessionService(service); err != nil {
		t.Fatalf("MigrateSQLiteSessionService: %v", err)
	}
	created, err := service.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "session-stale-retry",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	stale := created.Session
	fresh, err := service.Get(ctx, &adksession.GetRequest{
		AppName: "app", UserID: "user", SessionID: "session-stale-retry",
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	first := adksession.NewEvent("inv-first")
	first.Author = "agent"
	first.Content = genai.NewContentFromText("first", genai.RoleModel)
	if err := service.AppendEvent(ctx, fresh.Session, first); err != nil {
		t.Fatalf("AppendEvent(first): %v", err)
	}
	second := adksession.NewEvent("inv-second")
	second.Author = "agent"
	second.Content = genai.NewContentFromText("second", genai.RoleModel)
	if err := appendADKEventWithStaleRetry(ctx, service, stale, second); err != nil {
		t.Fatalf("appendADKEventWithStaleRetry: %v", err)
	}
	latest, err := service.Get(ctx, &adksession.GetRequest{
		AppName: "app", UserID: "user", SessionID: "session-stale-retry",
	})
	if err != nil {
		t.Fatalf("Get latest: %v", err)
	}
	if latest.Session.Events().Len() != 2 {
		t.Fatalf("event count = %d, want 2", latest.Session.Events().Len())
	}
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

	userEvent := adksession.NewEvent("inv-user")
	userEvent.Content = genai.NewContentFromText("制作一个 tme 的策略", genai.RoleUser)
	if err := runtime.rawSessionService.AppendEvent(ctx, created.Session, userEvent); err != nil {
		t.Fatalf("Append user event: %v", err)
	}

	oversizedPayload := strings.Repeat("x", MaxToolOutputBytes*2)
	toolEvent := adksession.NewEvent("inv-tool")
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

	toolEvent := adksession.NewEvent("inv-tool-small")
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
