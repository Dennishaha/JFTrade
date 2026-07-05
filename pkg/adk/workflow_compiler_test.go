package adk

import (
	"iter"
	"strings"
	"testing"

	adkagent "google.golang.org/adk/v2/agent"
	adksession "google.golang.org/adk/v2/session"
	adkworkflow "google.golang.org/adk/v2/workflow"
)

type workflowCompilerTestNode struct {
	adkworkflow.BaseNode
}

func newWorkflowCompilerTestNode(name string) *workflowCompilerTestNode {
	return &workflowCompilerTestNode{BaseNode: adkworkflow.NewBaseNode(name, "", adkworkflow.NodeConfig{})}
}

func (n *workflowCompilerTestNode) Run(adkagent.Context, any) iter.Seq2[*adksession.Event, error] {
	return func(yield func(*adksession.Event, error) bool) {}
}

func TestWorkflowCompilerBuildsJoinForFanIn(t *testing.T) {
	nodes := []adkworkflow.Node{
		newWorkflowCompilerTestNode("fetch"),
		newWorkflowCompilerTestNode("risk"),
		newWorkflowCompilerTestNode("report"),
	}
	steps := []workflowStep{
		{DependencyID: "fetch"},
		{DependencyID: "risk"},
		{DependencyID: "report", DependsOn: []string{"fetch", "risk"}},
	}
	edges, err := newWorkflowCompiler().CompileEdges(steps, nodes)
	if err != nil {
		t.Fatalf("CompileEdges: %v", err)
	}
	if len(edges) != 5 {
		t.Fatalf("edges = %d, want 5: %+v", len(edges), edges)
	}
	joinName := "report_join"
	assertWorkflowEdge(t, edges, adkworkflow.Start.Name(), "fetch")
	assertWorkflowEdge(t, edges, adkworkflow.Start.Name(), "risk")
	assertWorkflowEdge(t, edges, "fetch", joinName)
	assertWorkflowEdge(t, edges, "risk", joinName)
	assertWorkflowEdge(t, edges, joinName, "report")
}

func TestWorkflowCompilerKeepsDefaultSequentialDependencies(t *testing.T) {
	nodes := []adkworkflow.Node{
		newWorkflowCompilerTestNode("plan"),
		newWorkflowCompilerTestNode("build"),
		newWorkflowCompilerTestNode("verify"),
	}
	steps := []workflowStep{
		{DependencyID: "plan"},
		{DependencyID: "build"},
		{DependencyID: "verify"},
	}
	if err := normalizeSequentialPlannerDependencies(steps); err != nil {
		t.Fatalf("normalizeSequentialPlannerDependencies: %v", err)
	}
	edges, err := newWorkflowCompiler().CompileEdges(steps, nodes)
	if err != nil {
		t.Fatalf("CompileEdges: %v", err)
	}
	if len(edges) != 3 {
		t.Fatalf("edges = %d, want 3: %+v", len(edges), edges)
	}
	assertWorkflowEdge(t, edges, adkworkflow.Start.Name(), "plan")
	assertWorkflowEdge(t, edges, "plan", "build")
	assertWorkflowEdge(t, edges, "build", "verify")
}

func TestWorkflowCompilerDeduplicatesAndIgnoresBlankDependencies(t *testing.T) {
	nodes := []adkworkflow.Node{
		newWorkflowCompilerTestNode("fetch"),
		newWorkflowCompilerTestNode("report"),
	}
	steps := []workflowStep{
		{DependencyID: "fetch"},
		{DependencyID: "report", DependsOn: []string{"fetch", "fetch", " "}},
	}
	edges, err := newWorkflowCompiler().CompileEdges(steps, nodes)
	if err != nil {
		t.Fatalf("CompileEdges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("edges = %d, want 2: %+v", len(edges), edges)
	}
	assertWorkflowEdge(t, edges, adkworkflow.Start.Name(), "fetch")
	assertWorkflowEdge(t, edges, "fetch", "report")
}

func TestWorkflowCompilerRejectsUnknownDependencies(t *testing.T) {
	nodes := []adkworkflow.Node{
		newWorkflowCompilerTestNode("fetch"),
		newWorkflowCompilerTestNode("report"),
	}
	steps := []workflowStep{
		{DependencyID: "fetch"},
		{DependencyID: "report", DependsOn: []string{"fetch", "missing"}},
	}
	_, err := newWorkflowCompiler().CompileEdges(steps, nodes)
	if err == nil || !strings.Contains(err.Error(), `unknown dependency "missing"`) {
		t.Fatalf("CompileEdges error = %v, want unknown dependency", err)
	}
}

func assertWorkflowEdge(t *testing.T, edges []adkworkflow.Edge, from, to string) {
	t.Helper()
	for _, edge := range edges {
		if edge.From != nil && edge.To != nil && edge.From.Name() == from && edge.To.Name() == to {
			return
		}
	}
	t.Fatalf("edge %s -> %s not found in %+v", from, to, edges)
}
