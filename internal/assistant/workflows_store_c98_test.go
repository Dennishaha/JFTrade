package assistant

import (
	"testing"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

// A workflow service must surface an unavailable persistence backend instead of
// interpreting failed reads as missing workflows, triggers, or execution logs.
// Closing the real SQLite store is the closest failure mode to a process whose
// durable runtime has already been shut down while an HTTP request is in flight.
func TestCoverage98WorkflowOperationsPropagateClosedStoreFailures(t *testing.T) {
	runtime, service, _ := newAssistantServiceHarness(t)
	if err := runtime.Store().Close(); err != nil {
		t.Fatalf("close workflow store: %v", err)
	}
	ctx := t.Context()

	cases := []struct {
		name string
		call func() error
	}{
		{"initialize builtin templates", func() error { return service.EnsureBuiltinWorkflowTemplates(ctx) }},
		{"list workflows", func() error { _, err := service.ListWorkflows(ctx, WorkflowQuery{}); return err }},
		{"get workflow", func() error { _, err := service.GetWorkflow(ctx, "workflow"); return err }},
		{"save workflow", func() error {
			_, err := service.SaveWorkflow(ctx, "workflow", jfadk.WorkflowDefinitionWriteRequest{})
			return err
		}},
		{"delete workflow", func() error { _, err := service.DeleteWorkflow(ctx, "workflow"); return err }},
		{"list workflow triggers", func() error { _, err := service.ListWorkflowTriggers(ctx, "workflow"); return err }},
		{"get workflow trigger", func() error { _, err := service.GetWorkflowTrigger(ctx, "workflow", "trigger"); return err }},
		{"save workflow trigger", func() error {
			_, err := service.SaveWorkflowTrigger(ctx, "workflow", "", jfadk.WorkflowTriggerWriteRequest{Type: jfadk.WorkflowTriggerTypeManual})
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
				t.Fatalf("%s succeeded after the workflow store closed", tc.name)
			}
		})
	}

	if watched := service.WatchedWorkflowInstruments(ctx); watched != nil {
		t.Fatalf("WatchedWorkflowInstruments after store closure = %#v, want nil", watched)
	}
	// Event delivery is best-effort. A persistence outage must be absorbed here
	// so a market-data fan-out cannot be brought down by one workflow store.
	service.HandleWorkflowEvent(ctx, jfadk.WorkflowEvent{Type: "market-data.tick"})
}
