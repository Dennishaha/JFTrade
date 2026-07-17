package adk

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/genai"
)

func TestCoverage98SynchronousApprovalDenialCancelsSiblingActions(t *testing.T) {
	ctx := t.Context()
	base := newTestRuntime(t)
	registry := NewToolRegistry()
	var executions atomic.Int64
	for _, name := range []string{"approval.sync.one", "approval.sync.two"} {
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
	runtime := newRuntimeWithRegistry(t, base.Store(), registry)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-sync-deny-agent", Name: "Synchronous Approval Denial", ProviderID: testProviderID,
		Tools: []string{"approval.sync.one", "approval.sync.two"}, PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})

	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID,
		Message: `<execute-tool name="approval.sync.one" /><execute-tool name="approval.sync.two" />`,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(response.PendingApprovals) != 2 {
		t.Fatalf("pending approvals = %d, want 2", len(response.PendingApprovals))
	}

	// The synchronous HTTP/API path must have the same all-or-nothing denial
	// semantics as the async handler: a rejected first action revokes its
	// still-pending sibling before the suspended run is resumed.
	resolution, err := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, false)
	if err != nil {
		t.Fatalf("ResolveApproval deny: %v", err)
	}
	if resolution.Run == nil || resolution.Run.Status != RunStatusDenied {
		t.Fatalf("synchronous denial run = %+v, want denied terminal run", resolution.Run)
	}
	if got := executions.Load(); got != 0 {
		t.Fatalf("tool executions after synchronous denial = %d, want 0", got)
	}
	for _, approvalID := range []string{response.PendingApprovals[0].ID, response.PendingApprovals[1].ID} {
		approval, ok, err := runtime.Store().Approval(ctx, approvalID)
		if err != nil || !ok || approval.Status != ApprovalStatusDenied {
			t.Fatalf("Approval(%s) = %+v/%v/%v, want denied", approvalID, approval, ok, err)
		}
	}
}

func TestCoverage98ApprovalBusyRetryStopsWhenRequestIsCancelled(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-busy-retry-agent", Name: "Busy Retry Cancellation", Status: AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "busy approval retry")
	run := mustSaveRun(t, runtime, Run{
		ID: "coverage98-busy-retry-run", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusPending,
		ResumeState: "approval_resuming", UserMessage: "resume a confirmed write",
		PendingApprovals: []Approval{{
			ID: "coverage98-busy-retry-approval", RunID: "coverage98-busy-retry-run", AgentID: agent.ID,
			ToolName: "strategy.save_draft", Status: ApprovalStatusApproved,
			FunctionCallID: "coverage98-busy-retry-call", ConfirmationCallID: "coverage98-busy-retry-confirmation",
			CreatedAt: nowString(), UpdatedAt: nowString(),
		}},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})

	execution := newBareGoogleADKExecution(run.ID)
	execution.sessionID = session.ID
	execution.agent = agent
	started := make(chan struct{})
	release := make(chan struct{})
	execution.runBlocking = func(context.Context, *genai.Content) error {
		close(started)
		<-release
		return errors.New("append event to SessionService: database is locked")
	}
	runtime.adkRuns[run.ID] = execution

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	type outcome struct {
		handled bool
		err     error
	}
	result := make(chan outcome, 1)
	go func() {
		_, _, handled, err := runtime.resumeGoogleADKWithBusyRetry(ctx, run)
		result <- outcome{handled: handled, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("approval resume never attempted the active execution")
	}
	close(release)

	// The first attempt reports a retriable SQLite session append failure. Give
	// the retry loop a short head start (well below its 120ms retry delay), then
	// prove the caller cancellation stops it instead of waiting through retries.
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case got := <-result:
		if !got.handled || !errors.Is(got.err, context.Canceled) || !strings.Contains(got.err.Error(), "database is locked") {
			t.Fatalf("cancelled busy retry = handled:%v err:%v", got.handled, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled approval retry did not stop promptly")
	}
}
