package assistant

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/adk"
)

func TestServicePropagatesCancelledPersistenceContextAcrossAssistantResources(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cases := []struct {
		name string
		call func() error
	}{
		{"snapshot", func() error { _, err := service.Snapshot(ctx); return err }},
		{"list tasks", func() error { _, err := service.ListTasks(ctx, TaskQuery{}); return err }},
		{"get task", func() error { _, err := service.GetTask(ctx, "task"); return err }},
		{"save task", func() error {
			_, err := service.SaveTask(ctx, adk.TaskWriteRequest{ID: "cancelled-task", Title: "Cancelled task", Status: "TODO"})
			return err
		}},
		{"update task", func() error { _, err := service.UpdateTask(ctx, "task", adk.TaskPatchRequest{}); return err }},
		{"delete task", func() error { return service.DeleteTask(ctx, "task") }},
		{"list memory", func() error { _, err := service.ListMemory(ctx, MemoryQuery{}); return err }},
		{"save memory", func() error {
			_, err := service.SaveMemory(ctx, adk.MemoryWriteRequest{Scope: "global", Key: "cancelled", Value: "value"})
			return err
		}},
		{"delete memory", func() error { return service.DeleteMemory(ctx, "memory") }},
		{"list providers", func() error { _, err := service.ListProviders(ctx); return err }},
		{"save provider", func() error {
			_, err := service.SaveProvider(ctx, adk.ProviderWriteRequest{ID: "cancelled-provider", DisplayName: "Cancelled", Enabled: true})
			return err
		}},
		{"set default provider", func() error { _, err := service.SetDefaultProvider(ctx, "provider"); return err }},
		{"list agents", func() error { _, err := service.ListAgents(ctx, AgentQuery{}); return err }},
		{"save agent", func() error {
			_, err := service.SaveAgent(ctx, adk.AgentWriteRequest{ID: "cancelled-agent", Name: "Cancelled", Status: adk.AgentStatusDisabled})
			return err
		}},
		{"delete agent", func() error { return service.DeleteAgent(ctx, "agent") }},
		{"list sessions", func() error { _, err := service.ListSessions(ctx, SessionQuery{}); return err }},
		{"create session", func() error { _, err := service.CreateSession(ctx, CreateSessionRequest{AgentID: "agent"}); return err }},
		{"get session", func() error { _, err := service.GetSession(ctx, "session"); return err }},
		{"get session detail", func() error { _, err := service.GetSessionDetail(ctx, "session"); return err }},
		{"rename session", func() error { _, err := service.RenameSession(ctx, "session", "title"); return err }},
		{"update composer", func() error {
			_, err := service.UpdateSessionComposerState(ctx, "session", adk.SessionComposerStatePatch{})
			return err
		}},
		{"delete session", func() error { return service.DeleteSession(ctx, "session") }},
		{"preview default session", func() error { _, err := service.PreviewSession(ctx, adk.ChatRequest{Message: "hello"}); return err }},
		{"preview selected agent", func() error {
			_, err := service.PreviewSession(ctx, adk.ChatRequest{AgentID: "agent", Message: "hello"})
			return err
		}},
		{"recover terminal response", func() error { _, err := service.RecoverTerminalChatResponse(ctx, "run"); return err }},
		{"list runs", func() error { _, err := service.ListRuns(ctx, RunQuery{}); return err }},
		{"get run", func() error { _, err := service.GetRun(ctx, "run"); return err }},
		{"cancel run", func() error { _, err := service.CancelRun(ctx, "run"); return err }},
		{"pause goal run", func() error { _, err := service.PauseGoalRun(ctx, "run"); return err }},
		{"resume goal run", func() error { _, err := service.ResumeGoalRun(ctx, "run"); return err }},
		{"update run objective", func() error { _, err := service.UpdateRunObjective(ctx, "run", "objective"); return err }},
		{"list approvals", func() error { _, err := service.ListApprovals(ctx, ApprovalQuery{}); return err }},
		{"resolve approval", func() error { _, err := service.ResolveApproval(ctx, "approval", true); return err }},
		{"resolve approval asynchronously", func() error { _, err := service.ResolveApprovalAsync(ctx, "approval", false); return err }},
		{"resolve input asynchronously", func() error {
			_, err := service.ResolveInputAsync(ctx, "run", adk.InputResponseRequest{})
			return err
		}},
		{"get metrics", func() error { _, err := service.GetMetrics(ctx); return err }},
		{"list optimization tasks", func() error { _, err := service.ListOptimizationTasks(ctx); return err }},
		{"get optimization task", func() error { _, err := service.GetOptimizationTask(ctx, "task"); return err }},
		{"cancel optimization task", func() error { _, err := service.CancelOptimizationTask(ctx, "task"); return err }},
		{"get audit", func() error { _, err := service.GetAudit(ctx, AuditQuery{}); return err }},
		{"list workflows", func() error { _, err := service.ListWorkflows(ctx, WorkflowQuery{}); return err }},
		{"get workflow", func() error { _, err := service.GetWorkflow(ctx, "workflow"); return err }},
		{"save workflow", func() error {
			_, err := service.SaveWorkflow(ctx, "workflow", adk.WorkflowDefinitionWriteRequest{Name: "Workflow", AgentID: "agent", PromptTemplate: "run"})
			return err
		}},
		{"list workflow triggers", func() error { _, err := service.ListWorkflowTriggers(ctx, "workflow"); return err }},
		{"get workflow trigger", func() error { _, err := service.GetWorkflowTrigger(ctx, "workflow", "trigger"); return err }},
		{"save workflow trigger", func() error {
			_, err := service.SaveWorkflowTrigger(ctx, "workflow", "", adk.WorkflowTriggerWriteRequest{Type: adk.WorkflowTriggerTypeManual})
			return err
		}},
		{"delete workflow trigger", func() error { _, err := service.DeleteWorkflowTrigger(ctx, "workflow", "trigger"); return err }},
		{"list workflow logs", func() error { _, err := service.ListWorkflowTriggerLogs(ctx, WorkflowTriggerLogQuery{}); return err }},
		{"get workflow log", func() error { _, err := service.GetWorkflowTriggerLog(ctx, "log"); return err }},
		{"run workflow", func() error { _, err := service.RunWorkflow(ctx, "workflow", nil); return err }},
		{"start workflow", func() error { _, err := service.StartWorkflow(ctx, "workflow", nil); return err }},
		{"run workflow trigger", func() error { _, err := service.RunWorkflowTrigger(ctx, "trigger", nil); return err }},
		{"start workflow trigger", func() error { _, err := service.StartWorkflowTrigger(ctx, "trigger", nil); return err }},
		{"run workflow webhook", func() error { _, err := service.RunWorkflowWebhook(ctx, "trigger", "secret", nil); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatalf("%s accepted a cancelled request context", tc.name)
			}
		})
	}

	if runtime == nil {
		t.Fatal("test harness returned a nil runtime")
	}
}

func TestCanvasWorkflowNodeProjectionPreservesNodeSpecificBusinessState(t *testing.T) {
	workflow := adk.WorkflowDefinition{
		ID:   "canvas-projection",
		Name: "Canvas projection",
		CanvasGraph: &adk.WorkflowCanvasGraph{Nodes: []adk.WorkflowCanvasNode{
			{ID: "trigger-node", Type: "trigger", Data: map[string]any{"title": "Incoming event"}},
			{ID: "start-node", Type: "start", Data: map[string]any{"title": "Prepare inputs"}},
			{ID: "agent-node", Type: "agent", Data: map[string]any{"title": "Research"}},
			{ID: "monitor-node", Type: "monitor", Data: map[string]any{"title": "Review"}},
		}},
	}
	trigger := &adk.WorkflowTrigger{ID: "trigger-1", Title: "Manual review", Type: adk.WorkflowTriggerTypeManual, Status: adk.WorkflowTriggerStatusEnabled}
	response := &adk.ChatResponse{Run: adk.Run{WorkflowPlan: []adk.WorkflowStepState{{
		PlannerStepID: "agent-node", Status: "BLOCKED", ChildRunID: "child-run", ChildAgentID: "research-agent",
		ChildProviderID: "provider", ChildModel: "model", Message: "research US.AAPL", ResultSummary: "blocked by limit",
	}}}}
	runs := workflowCanvasNodeRuns(workflow, trigger, trigger.Type, map[string]any{"symbol": "US.AAPL"}, map[string]any{"type": "manual"}, "research", "objective", response, adk.WorkflowTriggerLogStatusFailed, "risk limit", "started", "finished")
	if len(runs) != 4 || runs[0].Title != "Incoming event" || runs[1].Title != "Prepare inputs" || runs[2].Title != "Research" || runs[3].Title != "Review" {
		t.Fatalf("canvas projection titles = %#v", runs)
	}
	agentRun := runs[2]
	if agentRun.Status != adk.WorkflowTriggerLogStatusFailed || agentRun.Outputs["runId"] != "child-run" || agentRun.Outputs["reply"] != "blocked by limit" || agentRun.Outputs["error"] != "risk limit" {
		t.Fatalf("canvas agent projection = %#v", agentRun)
	}

	for _, tc := range []struct {
		stepStatus string
		want       string
	}{
		{"DONE", adk.WorkflowTriggerLogStatusSucceeded},
		{"IN_PROGRESS", adk.WorkflowTriggerLogStatusRunning},
		{"TODO", adk.WorkflowTriggerLogStatusQueued},
		{"CANCELLED", adk.WorkflowTriggerLogStatusFailed},
		{"custom", "CUSTOM"},
		{"", adk.WorkflowTriggerLogStatusSkipped},
	} {
		t.Run(tc.stepStatus, func(t *testing.T) {
			run := canvasAgentNodeRun(adk.WorkflowCanvasNode{ID: "agent", Type: "agent"}, adk.WorkflowStepState{Status: tc.stepStatus}, "failure", "started", "finished")
			if run.Status != tc.want {
				t.Fatalf("status %q = %q, want %q", tc.stepStatus, run.Status, tc.want)
			}
		})
	}
}

func TestWorkflowUtilityAndUnavailableInputResolutionBoundaries(t *testing.T) {
	if _, err := (&Service{}).ResolveInputAsync(t.Context(), "run", adk.InputResponseRequest{}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("ResolveInputAsync without runtime error = %v", err)
	}
	if verifyWorkflowSecret("", hashWorkflowSecret("secret")) || verifyWorkflowSecret("secret", "") || verifyWorkflowSecret("wrong", hashWorkflowSecret("secret")) {
		t.Fatal("workflow secret verifier accepted an invalid secret pair")
	}
	secret, err := newWorkflowSecret()
	if err != nil || !strings.HasPrefix(secret, "wfsec-") || !verifyWorkflowSecret(secret, hashWorkflowSecret(secret)) {
		t.Fatalf("new workflow secret = %q, %v", secret, err)
	}
	if got := nextRunAtString(map[string]any{"cron": "bad"}, time.Now().UTC()); got != "" {
		t.Fatalf("invalid schedule next run = %q", got)
	}
}

func TestWorkflowEntryPointsRejectUnavailableRuntimeBeforePersistingAnything(t *testing.T) {
	service := &Service{}
	ctx := t.Context()
	workflow := adk.WorkflowDefinition{ID: "workflow", Name: "Workflow", Status: adk.WorkflowStatusEnabled, AgentID: "agent", PromptTemplate: "run"}
	trigger := &adk.WorkflowTrigger{ID: "trigger", WorkflowID: workflow.ID, Type: adk.WorkflowTriggerTypeManual, Status: adk.WorkflowTriggerStatusEnabled}

	cases := []struct {
		name string
		call func() error
	}{
		{"get workflow", func() error { _, err := service.GetWorkflow(ctx, workflow.ID); return err }},
		{"save workflow", func() error {
			_, err := service.SaveWorkflow(ctx, workflow.ID, adk.WorkflowDefinitionWriteRequest{Name: workflow.Name, AgentID: workflow.AgentID, PromptTemplate: workflow.PromptTemplate})
			return err
		}},
		{"delete workflow", func() error { _, err := service.DeleteWorkflow(ctx, workflow.ID); return err }},
		{"list triggers", func() error { _, err := service.ListWorkflowTriggers(ctx, workflow.ID); return err }},
		{"get trigger", func() error { _, err := service.GetWorkflowTrigger(ctx, workflow.ID, trigger.ID); return err }},
		{"save trigger", func() error {
			_, err := service.SaveWorkflowTrigger(ctx, workflow.ID, trigger.ID, adk.WorkflowTriggerWriteRequest{Type: trigger.Type})
			return err
		}},
		{"delete trigger", func() error { _, err := service.DeleteWorkflowTrigger(ctx, workflow.ID, trigger.ID); return err }},
		{"list logs", func() error { _, err := service.ListWorkflowTriggerLogs(ctx, WorkflowTriggerLogQuery{}); return err }},
		{"get log", func() error { _, err := service.GetWorkflowTriggerLog(ctx, "log"); return err }},
		{"run workflow", func() error { _, err := service.RunWorkflow(ctx, workflow.ID, nil); return err }},
		{"start workflow", func() error { _, err := service.StartWorkflow(ctx, workflow.ID, nil); return err }},
		{"run trigger", func() error { _, err := service.RunWorkflowTrigger(ctx, trigger.ID, nil); return err }},
		{"start trigger", func() error { _, err := service.StartWorkflowTrigger(ctx, trigger.ID, nil); return err }},
		{"run webhook", func() error { _, err := service.RunWorkflowWebhook(ctx, trigger.ID, "secret", nil); return err }},
		{"start queued workflow", func() error {
			_, err := service.startWorkflowAsync(ctx, workflow, trigger, trigger.Type, nil, nil)
			return err
		}},
		{"run canvas", func() error {
			_, err := service.runWorkflowCanvas(ctx, workflow, trigger, "session", "message", "", nil, nil)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil || !strings.Contains(err.Error(), "unavailable") {
				t.Fatalf("%s error = %v, want unavailable runtime", tc.name, err)
			}
		})
	}
}

func TestAgentValidationAndSchedulerBoundariesProtectRuntimeResources(t *testing.T) {
	_, service, _ := newAssistantServiceHarness(t)
	ctx := t.Context()
	if _, err := service.SaveAgent(ctx, adk.AgentWriteRequest{ID: adk.DefaultBuiltinAgentID}); err == nil || !strings.Contains(err.Error(), "cannot be edited") {
		t.Fatalf("primary builtin agent update error = %v", err)
	}
	if _, err := service.SaveAgent(ctx, adk.AgentWriteRequest{ID: "invalid-status", Name: "Invalid", Status: "broken"}); err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("invalid agent status error = %v", err)
	}
	if _, err := service.SaveAgent(ctx, adk.AgentWriteRequest{ID: "invalid-mode", Name: "Invalid", WorkMode: "broken"}); err == nil || !strings.Contains(err.Error(), "work mode") {
		t.Fatalf("invalid agent work mode error = %v", err)
	}
	if _, err := service.SaveAgent(ctx, adk.AgentWriteRequest{ID: "unknown-skill", Name: "Unknown skill", Status: adk.AgentStatusDisabled, Skills: []string{"missing"}}); err == nil || !strings.Contains(err.Error(), "unknown ADK skill") {
		t.Fatalf("unknown skill error = %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := service.SaveAgent(cancelled, adk.AgentWriteRequest{ID: "cancelled-provider", Name: "Cancelled provider", ProviderID: "provider"}); err == nil {
		t.Fatal("provider lookup accepted a cancelled context")
	}

	scheduler := &WorkflowScheduler{interval: time.Hour}
	scheduler.Start(ctx)
	scheduler.Stop()
	var nilScheduler *WorkflowScheduler
	nilScheduler.Stop()
}
