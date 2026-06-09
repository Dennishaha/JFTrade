package adk

import (
	"context"
	"fmt"
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
