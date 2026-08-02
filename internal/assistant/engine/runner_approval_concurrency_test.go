package adk

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentResolveApprovalExecutesApprovedToolOnce(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	var executions atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	releaseTool := sync.OnceFunc(func() { close(release) })
	defer releaseTool()

	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name:               "approval.concurrent",
		Permission:         "write_strategy",
		AllowedModes:       []string{PermissionModeApproval},
		RequiresApprovalIn: []string{PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		if executions.Add(1) == 1 {
			close(started)
			<-release
		}
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "approval-concurrent-agent", Name: "Approval Concurrent Agent", ProviderID: testProviderID,
		Tools: []string{"approval.concurrent"}, PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@approval.concurrent save"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(response.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want 1", len(response.PendingApprovals))
	}

	results := make(chan error, 2)
	resolve := func() {
		_, resolveErr := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, true)
		results <- resolveErr
	}
	go resolve()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("approved tool did not start")
	}
	go resolve()

	select {
	case err := <-results:
		if err != nil {
			t.Fatalf("concurrent ResolveApproval: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("duplicate ResolveApproval blocked behind the active continuation")
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("executions while first continuation is active = %d, want 1", got)
	}

	releaseTool()
	select {
	case err := <-results:
		if err != nil {
			t.Fatalf("initial ResolveApproval: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("initial ResolveApproval did not finish")
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("executions = %d, want 1", got)
	}
	runtime.approvalMu.Lock()
	_, inFlight := runtime.approvalRuns[response.Run.ID]
	runtime.approvalMu.Unlock()
	if inFlight {
		t.Fatal("approval continuation remained in flight after completion")
	}
}

func TestConcurrentSiblingApprovalsAreMergedBeforeContinuation(t *testing.T) {
	ctx := context.Background()
	runtime, response, executions := newSiblingApprovalRuntime(t)
	start := make(chan struct{})
	results := make(chan error, len(response.PendingApprovals))
	for _, pending := range response.PendingApprovals {
		go func() {
			<-start
			_, err := runtime.ResolveApproval(ctx, pending.ID, true)
			results <- err
		}()
	}
	close(start)
	for range response.PendingApprovals {
		if err := <-results; err != nil {
			t.Fatalf("concurrent sibling approval: %v", err)
		}
	}
	stored, ok, err := runtime.Store().Run(ctx, response.Run.ID)
	if err != nil || !ok || stored.Status != RunStatusCompleted {
		t.Fatalf("sibling approval run = %+v/%v/%v, want completed", stored, ok, err)
	}
	if got := executions.Load(); got != int64(len(response.PendingApprovals)) {
		t.Fatalf("sibling tool executions = %d, want %d", got, len(response.PendingApprovals))
	}
}

func TestConcurrentSiblingAsyncApprovalsEnqueueOneContinuation(t *testing.T) {
	ctx := context.Background()
	runtime, response, executions := newSiblingApprovalRuntime(t)
	start := make(chan struct{})
	results := make(chan error, len(response.PendingApprovals))
	for _, pending := range response.PendingApprovals {
		go func() {
			<-start
			_, err := runtime.ResolveApprovalAsync(ctx, pending.ID, true)
			results <- err
		}()
	}
	close(start)
	for range response.PendingApprovals {
		if err := <-results; err != nil {
			t.Fatalf("concurrent sibling async approval: %v", err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stored, ok, err := runtime.Store().Run(ctx, response.Run.ID)
		if err != nil || !ok {
			t.Fatalf("sibling async run lookup: %+v/%v/%v", stored, ok, err)
		}
		if stored.Status == RunStatusCompleted {
			if got := executions.Load(); got != int64(len(response.PendingApprovals)) {
				t.Fatalf("sibling async tool executions = %d, want %d", got, len(response.PendingApprovals))
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("sibling async run did not complete; executions=%d", executions.Load())
}

func TestAsyncApprovalWaitsForLocalInputContinuationLease(t *testing.T) {
	ctx := t.Context()
	runtime, executions := newWorkflowApprovalRuntime(t, WorkModeChat)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "approval-local-lease-agent", Name: "Approval Local Lease", ProviderID: testProviderID,
		Tools: []string{"approval.required"}, PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	response, err := runtime.Chat(ctx, ChatRequest{AgentID: agent.ID, Message: "@approval.required save"})
	if err != nil || len(response.PendingApprovals) != 1 {
		t.Fatalf("Chat response=%+v err=%v", response, err)
	}

	_, cancelLease, waitForLease, err := runtime.beginRunExecutionLease(ctx, response.Run.ID)
	if err != nil {
		t.Fatalf("hold local input continuation lease: %v", err)
	}
	resolved, err := runtime.ResolveApprovalAsync(ctx, response.PendingApprovals[0].ID, true)
	if err != nil || resolved.Run == nil || resolved.Run.ResumeState != "approval_resuming" {
		cancelLease()
		waitForLease()
		t.Fatalf("ResolveApprovalAsync=%+v err=%v", resolved, err)
	}
	time.Sleep(50 * time.Millisecond)
	if got := executions.Load(); got != 0 {
		cancelLease()
		waitForLease()
		t.Fatalf("approved tool executed %d times before local lease release", got)
	}

	cancelLease()
	waitForLease()
	completed := waitForRunStatus(t, runtime, response.Run.ID, RunStatusCompleted)
	if completed.ResumeState != "adk_confirmation_resolved" || executions.Load() != 1 {
		t.Fatalf("completed run=%+v executions=%d", completed, executions.Load())
	}
}

func TestApprovalLeaseWaitStopsWhenRuntimeContextIsCancelled(t *testing.T) {
	runtime := newTestRuntime(t)
	ctx, cancel := context.WithCancel(t.Context())
	_, cancelLease, waitForLease, err := runtime.beginRunExecutionLease(t.Context(), "approval-cancelled-wait")
	if err != nil {
		t.Fatalf("hold local lease: %v", err)
	}
	cancel()
	if err := runtime.waitForLocalRunLeaseRelease(ctx, "approval-cancelled-wait"); !errors.Is(err, context.Canceled) {
		cancelLease()
		waitForLease()
		t.Fatalf("wait error = %v, want context cancellation", err)
	}
	cancelLease()
	waitForLease()
}

func newSiblingApprovalRuntime(t *testing.T) (*Runtime, ChatResponse, *atomic.Int64) {
	t.Helper()
	runtime := newTestRuntime(t)
	executions := new(atomic.Int64)
	registry := NewToolRegistry()
	for _, name := range []string{"approval.sibling.one", "approval.sibling.two"} {
		toolName := name
		registry.Register(ToolDescriptor{
			Name:               toolName,
			Permission:         "write_strategy",
			AllowedModes:       []string{PermissionModeApproval},
			RequiresApprovalIn: []string{PermissionModeApproval},
		}, func(context.Context, map[string]any) (any, error) {
			executions.Add(1)
			return map[string]any{"saved": true}, nil
		})
	}
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "approval-sibling-agent", Name: "Approval Sibling Agent", ProviderID: testProviderID,
		Tools: []string{"approval.sibling.one", "approval.sibling.two"}, PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	response, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID: agent.ID,
		Message: `<execute-tool name="approval.sibling.one" /><execute-tool name="approval.sibling.two" />`,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(response.PendingApprovals) != 2 {
		t.Fatalf("pending approvals = %d, want 2", len(response.PendingApprovals))
	}
	return runtime, response, executions
}
