package adk

import (
	"strings"
	"testing"
)

func TestCoverage98GoogleRunnerPreservesResumedFailureDiagnostics(t *testing.T) {
	if got := prefixedToolError("", "tool execution failed"); got != "tool execution failed" {
		t.Fatalf("blank prefixed tool error = %q", got)
	}
	if got := toolCallFailureMessage(&ToolCall{Status: "CANCELLED"}); got != "tool execution cancelled" {
		t.Fatalf("cancelled tool failure = %q", got)
	}
	if got := toolCallFailureMessage(&ToolCall{Status: "UNKNOWN"}); got != "tool execution failed" {
		t.Fatalf("unknown tool failure = %q", got)
	}
	if got := firstToolCallFailure(nil); got != "" {
		t.Fatalf("nil run failure = %q", got)
	}

	seedResumedExecutionState(nil, Run{ID: "ignored"})
	execution := &googleADKExecution{}
	seedResumedExecutionState(execution, Run{
		ID: "coverage98-resumed", ToolCalls: []ToolCall{
			{ID: "persisted-without-key", Status: "FAILED"},
			{IdempotencyKey: "persisted-key", Status: "CANCELLED"},
		}, ToolSummaries: []string{"first persisted failure", "second persisted failure"},
	})
	if execution.runSnapshotBaseByID["coverage98-resumed"].ID != "coverage98-resumed" || len(execution.calls) != 2 || len(execution.summaries) != 2 {
		t.Fatalf("resumed state seed = %+v calls=%+v summaries=%+v", execution.runSnapshotBaseByID, execution.calls, execution.summaries)
	}
}

func TestCoverage98GoogleRunnerSurfacesModelAndChildConstructionFailures(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-runner-agent", Name: "Coverage Runner", ProviderID: testProviderID,
		Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "coverage runner construction")

	if _, err := runtime.googleADKModelForAgent(ctx, Agent{ID: "coverage98-unknown-provider", ProviderID: "coverage98-provider-missing"}); err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("googleADKModelForAgent unknown provider = %v", err)
	}
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "coverage98-no-key-provider", DisplayName: "No Key", BaseURL: "https://example.test/v1", Model: "test-model", Enabled: true,
	})
	if _, err := runtime.googleADKModelForAgent(ctx, Agent{ID: "coverage98-no-key-agent", ProviderID: "coverage98-no-key-provider", Model: "test-model"}); err == nil || !strings.Contains(err.Error(), "API key is not configured") {
		t.Fatalf("googleADKModelForAgent no key = %v", err)
	}

	noKeyAgent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage98-rehydrate-no-key", Name: "Rehydrate No Key", ProviderID: "coverage98-no-key-provider", Status: AgentStatusEnabled,
	})
	noKeySession := mustCreateSession(t, runtime, noKeyAgent.ID, "rehydrate no key")
	if _, err := runtime.newResumedGoogleADKExecution(ctx, Run{ID: "coverage98-resume-no-key", AgentID: noKeyAgent.ID, SessionID: noKeySession.ID}); err == nil || !strings.Contains(err.Error(), "API key is not configured") {
		t.Fatalf("newResumedGoogleADKExecution no key = %v", err)
	}

	parent := Run{ID: "coverage98-child-parent", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning, Usage: &RunUsage{}}
	child := Run{ID: "coverage98-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID, Status: RunStatusRunning, Usage: &RunUsage{}}
	if _, err := runtime.newGoogleADKWorkflowChildNodes(ctx, agent, parent.ID, []Run{child}, []workflowStep{{
		Title: "missing child", Message: "must fail", ChildAgentID: "coverage98-child-agent-missing",
	}}, newBareGoogleADKExecution(parent.ID)); err == nil || !strings.Contains(err.Error(), "agent not found") {
		t.Fatalf("newGoogleADKWorkflowChildNodes missing agent = %v", err)
	}
	if _, err := runtime.newGoogleADKWorkflowChildNode(ctx, agent, parent.ID, child, workflowStep{
		Title: "missing provider", Message: "must fail", ChildProviderID: "coverage98-child-provider-missing",
	}, 0, newBareGoogleADKExecution(parent.ID)); err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("newGoogleADKWorkflowChildNode missing provider = %v", err)
	}

	loaderRuntime := newTestRuntime(t)
	if _, err := loaderRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableRuns); err != nil {
		t.Fatalf("drop runs for loader: %v", err)
	}
	if _, _, err := loaderRuntime.googleADKExecutionRunLoader()(ctx, "coverage98-missing-run"); err == nil || !strings.Contains(err.Error(), tableRuns) {
		t.Fatalf("googleADKExecutionRunLoader dropped table = %v", err)
	}
}
