package assistant

import (
	"testing"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestServiceLifecycleAndTimeoutBoundaryHelpers(t *testing.T) {
	service := NewService(nil)
	if got := service.StreamIdleTimeoutMillis(); got != 0 {
		t.Fatalf("StreamIdleTimeoutMillis() without option = %d, want 0", got)
	}

	service.workflowScheduler = &WorkflowScheduler{}
	if err := service.Close(); err != nil {
		t.Fatalf("Close() with scheduler only: %v", err)
	}
	if service.workflowScheduler != nil {
		t.Fatal("Close() should clear workflowScheduler reference")
	}
}

func TestApprovalWaitDurationMsHandlesBoundaryTimestamps(t *testing.T) {
	now := time.Date(2026, 7, 3, 1, 2, 3, 0, time.UTC)
	cases := []struct {
		name     string
		approval jfadk.Approval
		wantMs   int64
	}{
		{name: "missing createdAt", approval: jfadk.Approval{}, wantMs: 0},
		{name: "invalid createdAt", approval: jfadk.Approval{CreatedAt: "not-a-time"}, wantMs: 0},
		{
			name: "pending uses current time",
			approval: jfadk.Approval{
				Status:    jfadk.ApprovalStatusPending,
				CreatedAt: now.Add(-1500 * time.Millisecond).Format(time.RFC3339Nano),
			},
			wantMs: 1500,
		},
		{
			name: "resolved uses updated time",
			approval: jfadk.Approval{
				Status:    jfadk.ApprovalStatusApproved,
				CreatedAt: now.Add(-2 * time.Second).Format(time.RFC3339Nano),
				UpdatedAt: now.Add(-500 * time.Millisecond).Format(time.RFC3339Nano),
			},
			wantMs: 1500,
		},
		{
			name: "updated before created clamps to zero",
			approval: jfadk.Approval{
				Status:    jfadk.ApprovalStatusDenied,
				CreatedAt: now.Format(time.RFC3339Nano),
				UpdatedAt: now.Add(-time.Second).Format(time.RFC3339Nano),
			},
			wantMs: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := approvalWaitDurationMs(tc.approval, now); got != tc.wantMs {
				t.Fatalf("approvalWaitDurationMs(%+v) = %d, want %d", tc.approval, got, tc.wantMs)
			}
		})
	}
}

func TestServicePreviewSessionFallsBackWhenRequestedSessionIsMissing(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()

	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-preview-fallback", Name: "Preview Fallback", Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	preview, err := service.PreviewSession(ctx, jfadk.ChatRequest{
		AgentID:   agent.ID,
		SessionID: "missing-session",
		Message:   "Use fallback preview",
	})
	if err != nil {
		t.Fatalf("PreviewSession fallback: %v", err)
	}
	if preview.ID != "" || preview.AgentID != agent.ID || preview.Title != "Use fallback preview" {
		t.Fatalf("fallback preview = %#v", preview)
	}
}

func TestServiceRecoverTerminalChatResponseHandlesBlankRunIDAndMissingProjection(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()

	if response, err := service.RecoverTerminalChatResponse(ctx, " \t "); err != nil || response != nil {
		t.Fatalf("RecoverTerminalChatResponse(blank) = %#v, %v, want nil nil", response, err)
	}

	agent, err := runtime.Store().SaveAgent(ctx, jfadk.AgentWriteRequest{
		ID: "agent-recover-empty", Name: "Recover Empty", Status: jfadk.AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	session, err := runtime.Store().CreateSession(ctx, agent.ID, "Recover Empty Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	run := jfadk.Run{
		ID:        "run-recover-empty",
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    jfadk.RunStatusCompleted,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	response, err := service.RecoverTerminalChatResponse(ctx, run.ID)
	if err != nil {
		t.Fatalf("RecoverTerminalChatResponse(empty projection): %v", err)
	}
	if response == nil {
		t.Fatal("RecoverTerminalChatResponse(empty projection) = nil")
	}
	if response.Reply != "" || response.ReasoningContent != "" {
		t.Fatalf("empty projection recovery should not invent reply: %+v", response)
	}
	if response.Run.ID != run.ID || response.Session.ID != session.ID {
		t.Fatalf("recovered identifiers = run %q session %q", response.Run.ID, response.Session.ID)
	}
}
