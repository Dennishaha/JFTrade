package adk

import (
	"strings"
	"testing"

	adkmodel "google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
)

func TestWorkflowObservationProjectsNodeLifecycleOntoParentAndChildRuns(t *testing.T) {
	parent := Run{
		ID: "parent-run",
		WorkflowPlan: []WorkflowStepState{{
			TaskID: "research", ChildRunID: "child-run", NodeName: "planned-worker", Status: "IN_PROGRESS",
		}},
	}
	child := Run{ID: "child-run", ParentRunID: parent.ID}
	var deltas []ChatDelta
	execution := &googleADKExecution{
		runID:               parent.ID,
		runIDByAgentName:    map[string]string{"worker-agent": child.ID, "worker": child.ID},
		runSnapshotBaseByID: map[string]Run{parent.ID: parent, child.ID: child},
		onDelta: func(delta ChatDelta) error {
			deltas = append(deltas, delta)
			return nil
		},
	}

	execution.observeWorkflowEvent(&adksession.Event{
		Author:   "worker-agent",
		NodeInfo: &adksession.NodeInfo{Path: "worker/child@attempt-1"},
		Routes:   []string{" next ", "next", "audit"},
		Output:   map[string]any{"result": "ready"},
	})

	updatedParent := execution.runSnapshotBaseByID[parent.ID]
	if got := updatedParent.WorkflowPlan[0]; got.NodeName != "worker" || got.NodeStatus != "COMPLETED" || got.OutputSummary != `{"result":"ready"}` || strings.Join(got.Routes, ",") != "audit,next" {
		t.Fatalf("projected workflow step = %+v", got)
	}
	updatedChild := execution.runSnapshotBaseByID[child.ID]
	if got := updatedChild.WorkflowPlan[0]; got.NodeName != "worker" || got.NodeStatus != "COMPLETED" {
		t.Fatalf("child snapshot did not receive parent plan projection: %+v", got)
	}
	if !execution.workflowRunObserved(child.ID) {
		t.Fatal("child workflow run should be marked observed after its node emits output")
	}
	if len(deltas) != 2 {
		t.Fatalf("snapshot deltas = %d, want parent and child", len(deltas))
	}

	execution.observeWorkflowEvent(&adksession.Event{
		Author:         "worker-agent",
		NodeInfo:       &adksession.NodeInfo{Path: "worker"},
		RequestedInput: &adksession.RequestInput{Message: "need a decision"},
	})
	if got := execution.runSnapshotBaseByID[parent.ID].WorkflowPlan[0].NodeStatus; got != "FAILED" {
		t.Fatalf("requested-input node status = %q, want FAILED", got)
	}
}

func TestWorkflowObservationHelperBoundaries(t *testing.T) {
	if got := workflowEventNodeName(nil); got != "" {
		t.Fatalf("nil event node name = %q", got)
	}
	if got := workflowEventNodeName(&adksession.Event{Author: " author "}); got != "author" {
		t.Fatalf("author fallback node name = %q", got)
	}
	if got := workflowEventNodeName(&adksession.Event{Author: "author", NodeInfo: &adksession.NodeInfo{Path: " root@run/child "}}); got != "root" {
		t.Fatalf("node path name = %q", got)
	}

	step := WorkflowStepState{ChildRunID: "child", NodeName: "workflow-worker"}
	for _, match := range []struct {
		runID    string
		nodeName string
		want     bool
	}{
		{runID: " child ", want: true},
		{nodeName: "workflow-worker", want: true},
		{nodeName: "worker", want: true},
		{nodeName: "other", want: false},
	} {
		if got := workflowObservationMatchesStep(step, match.runID, match.nodeName); got != match.want {
			t.Fatalf("workflowObservationMatchesStep(%q, %q) = %v, want %v", match.runID, match.nodeName, got, match.want)
		}
	}

	if got := workflowNodeStatus(nil); got != "" {
		t.Fatalf("nil workflow node status = %q", got)
	}
	if got := workflowNodeStatus(&adksession.Event{LLMResponse: adkmodel.LLMResponse{Partial: true}}); got != "RUNNING" {
		t.Fatalf("partial workflow node status = %q", got)
	}
	if got := workflowNodeStatus(&adksession.Event{}); got != "RUNNING" {
		t.Fatalf("default workflow node status = %q", got)
	}

	if got := summarizeWorkflowOutput(nil); got != "" {
		t.Fatalf("nil output summary = %q", got)
	}
	if got := summarizeWorkflowOutput(make(chan int)); got != "" {
		t.Fatalf("unmarshallable output summary = %q", got)
	}
	long := summarizeWorkflowOutput(strings.Repeat("a", 700))
	if !strings.HasSuffix(long, "...(truncated)") || len(long) > 615 {
		t.Fatalf("long output summary length/suffix = %d/%q", len(long), long)
	}
	if got := jsonFallbackString(make(chan int)); got != "" {
		t.Fatalf("unmarshallable fallback = %q", got)
	}

	execution := &googleADKExecution{runID: "parent", runSnapshotBaseByID: map[string]Run{"parent": {ID: "parent"}}}
	execution.observeWorkflowEvent(nil)
	execution.observeWorkflowEvent(&adksession.Event{Author: "worker"})
	if execution.workflowRunObserved("missing") || execution.workflowRunObserved("  ") || ((*googleADKExecution)(nil)).workflowRunObserved("child") {
		t.Fatal("unobserved or nil execution should not report a workflow child event")
	}
}
