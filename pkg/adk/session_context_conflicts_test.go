package adk

import (
	"context"
	"strings"
	"testing"

	adksession "google.golang.org/adk/session"
)

func TestSessionContextAndCompactionRejectMissingResourcesAndConflicts(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	if _, err := runtime.SessionContext(ctx, "session-missing"); err == nil || !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("SessionContext missing session err = %v, want session not found", err)
	}
	sessionWithMissingAgent, err := runtime.Store().CreateSession(ctx, "agent-missing", "Missing Agent Session")
	if err != nil {
		t.Fatalf("CreateSession missing agent: %v", err)
	}
	if _, err := runtime.SessionContext(ctx, sessionWithMissingAgent.ID); err == nil || !strings.Contains(err.Error(), "agent not found") {
		t.Fatalf("SessionContext missing agent err = %v, want agent not found", err)
	}
	if _, err := runtime.CompactSessionContext(ctx, "session-missing", "normal", "manual", "missing"); err == nil || !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("CompactSessionContext missing session err = %v, want session not found", err)
	}

	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:               "context-conflict-agent",
		Name:             "Context Conflict Agent",
		Instruction:      "Test agent",
		RecentUserWindow: 1,
		PermissionMode:   PermissionModeApproval,
		Status:           AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "Context Conflict Session")

	release, acquired := runtime.beginSessionCompaction(session.ID)
	if !acquired {
		t.Fatal("beginSessionCompaction acquired = false, want true")
	}
	if _, err := runtime.CompactSessionContext(ctx, session.ID, "normal", "manual", "already running"); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("CompactSessionContext gate conflict err = %v, want already running", err)
	}
	release()

	created, err := runtime.rawSessionService.Create(ctx, &adksession.CreateRequest{
		AppName:   googleADKAppName(agent.ID),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("Create raw session: %v", err)
	}
	appendLargeContextEvents(t, runtime.rawSessionService, created.Session, 0, 40)
	mustSaveRun(t, runtime, Run{
		ID:        "run-context-active",
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    RunStatusRunning,
		WorkMode:  WorkModeLoop,
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
	})

	if _, err := runtime.CompactSessionContext(ctx, session.ID, "normal", "manual", "active run"); err == nil || !strings.Contains(err.Error(), "active run") {
		t.Fatalf("CompactSessionContext active run err = %v, want active run", err)
	}
	timeline, ok, err := runtime.Store().SessionTimeline(ctx, session.ID)
	if err != nil || !ok {
		t.Fatalf("SessionTimeline ok=%v err=%v", ok, err)
	}
	foundErrorNotice := false
	for _, entry := range timeline {
		if entry.Kind != TimelineKindContextNotice || entry.Status != TimelineStatusError {
			continue
		}
		foundErrorNotice = true
		if entry.Text != contextCompactionFailedText {
			t.Fatalf("error context notice = %+v, want failed text", entry)
		}
	}
	if !foundErrorNotice {
		t.Fatalf("timeline = %+v, want error context notice after active run conflict", timeline)
	}
}
