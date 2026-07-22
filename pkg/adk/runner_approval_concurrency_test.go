package adk

import (
	"context"
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
		pending := pending
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
		pending := pending
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
