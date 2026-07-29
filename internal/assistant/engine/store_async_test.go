package adk

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveApprovalAsyncDenialRejectsSiblingApprovalsWithoutExecutingTools(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	var executions atomic.Int64
	registry := NewToolRegistry()
	for _, name := range []string{"approval.required.one", "approval.required.two"} {
		registry.Register(ToolDescriptor{
			Name:               name,
			Permission:         "write_strategy",
			AllowedModes:       []string{PermissionModeApproval},
			RequiresApprovalIn: []string{PermissionModeApproval},
		}, func(context.Context, map[string]any) (any, error) {
			executions.Add(1)
			return map[string]any{"ok": true}, nil
		})
	}
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent-async-deny", Name: "Agent", ProviderID: testProviderID, Tools: []string{"approval.required.one", "approval.required.two"},
		PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID,
		Message: `<execute-tool name="approval.required.one" /><execute-tool name="approval.required.two" />`,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(response.PendingApprovals) != 2 {
		t.Fatalf("pending approvals = %d, want 2", len(response.PendingApprovals))
	}

	resolution, err := runtime.ResolveApprovalAsync(ctx, response.PendingApprovals[0].ID, false)
	if err != nil {
		t.Fatalf("ResolveApprovalAsync deny: %v", err)
	}
	if resolution.Run == nil || resolution.Run.ResumeState != "approval_resuming" {
		t.Fatalf("initial async denial = %+v, want approval_resuming run", resolution.Run)
	}
	if len(resolution.Run.PendingApprovals) != 2 {
		t.Fatalf("pending approvals after async denial = %+v, want both approvals embedded", resolution.Run.PendingApprovals)
	}
	for _, approval := range resolution.Run.PendingApprovals {
		if approval.Status != ApprovalStatusDenied {
			t.Fatalf("approval after async denial = %+v, want denied", approval)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		stored, ok, err := runtime.Store().Run(ctx, response.Run.ID)
		if err != nil || !ok {
			t.Fatalf("Run lookup err=%v ok=%v", err, ok)
		}
		if stored.Status == RunStatusDenied {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run stayed non-terminal after async denial: %+v", stored)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := executions.Load(); got != 0 {
		t.Fatalf("executions after async denial = %d, want 0", got)
	}
	for _, approvalID := range []string{response.PendingApprovals[0].ID, response.PendingApprovals[1].ID} {
		approval, ok, err := runtime.Store().Approval(ctx, approvalID)
		if err != nil || !ok {
			t.Fatalf("Approval(%s) ok=%v err=%v", approvalID, ok, err)
		}
		if approval.Status != ApprovalStatusDenied {
			t.Fatalf("approval %s status=%q, want denied", approvalID, approval.Status)
		}
	}
}

func TestResolveApprovalAsyncDoesNotResumeCompletedRunOnRetry(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	run := mustSaveRun(t, runtime, Run{
		ID:        "run-async-retry-complete",
		SessionID: "session-async-retry",
		AgentID:   "agent-async-retry",
		Status:    RunStatusCompleted,
		PendingApprovals: []Approval{{
			ID:        "approval-async-retry",
			RunID:     "run-async-retry-complete",
			AgentID:   "agent-async-retry",
			ToolName:  "strategy.save_draft",
			Status:    ApprovalStatusApproved,
			CreatedAt: nowString(),
			UpdatedAt: nowString(),
		}},
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
		Usage:     &RunUsage{},
	})
	if err := runtime.Store().SaveApproval(ctx, run.PendingApprovals[0]); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	resolution, err := runtime.ResolveApprovalAsync(ctx, run.PendingApprovals[0].ID, true)
	if err != nil {
		t.Fatalf("ResolveApprovalAsync completed retry: %v", err)
	}
	if resolution.Run != nil || resolution.Message != nil {
		t.Fatalf("completed retry resolution = %+v, want no resumed run/message", resolution)
	}
	if resolution.Approval.Status != ApprovalStatusApproved {
		t.Fatalf("completed retry approval = %+v, want approved approval passthrough", resolution.Approval)
	}
}
