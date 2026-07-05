package adk

import (
	"context"
	"iter"
	"sort"
	"strings"
	"testing"

	adkagent "google.golang.org/adk/v2/agent"
	adkrunner "google.golang.org/adk/v2/runner"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func TestGoogleADKSequentialWorkflowAgentRunsSubAgentsInOrder(t *testing.T) {
	first := newGoogleADKFixedWorkflowTestAgent(t, "first")
	second := newGoogleADKFixedWorkflowTestAgent(t, "second")
	root, err := newGoogleADKSequentialWorkflowAgent(" sequence ", " fixed order ", []adkagent.Agent{first, second})
	if err != nil {
		t.Fatalf("newGoogleADKSequentialWorkflowAgent: %v", err)
	}
	if root.Name() != "sequence" || root.Description() != "fixed order" {
		t.Fatalf("root metadata = %q/%q", root.Name(), root.Description())
	}

	outputs := runGoogleADKFixedWorkflowAgent(t, root)
	if strings.Join(outputs, ",") != "first,second" {
		t.Fatalf("sequential outputs = %#v, want first then second", outputs)
	}
}

func TestGoogleADKParallelWorkflowAgentRunsIndependentBranches(t *testing.T) {
	alpha := newGoogleADKFixedWorkflowTestAgent(t, "alpha")
	beta := newGoogleADKFixedWorkflowTestAgent(t, "beta")
	root, err := newGoogleADKParallelWorkflowAgent("parallel", "fan out", []adkagent.Agent{alpha, beta})
	if err != nil {
		t.Fatalf("newGoogleADKParallelWorkflowAgent: %v", err)
	}

	outputs := runGoogleADKFixedWorkflowAgent(t, root)
	sort.Strings(outputs)
	if strings.Join(outputs, ",") != "alpha,beta" {
		t.Fatalf("parallel outputs = %#v, want both independent branches", outputs)
	}
}

func TestGoogleADKLoopWorkflowAgentHonorsMaxIterations(t *testing.T) {
	worker := newGoogleADKFixedWorkflowTestAgent(t, "worker")
	root, err := newGoogleADKLoopWorkflowAgent("loop", "bounded loop", []adkagent.Agent{worker}, 3)
	if err != nil {
		t.Fatalf("newGoogleADKLoopWorkflowAgent: %v", err)
	}

	outputs := runGoogleADKFixedWorkflowAgent(t, root)
	if strings.Join(outputs, ",") != "worker,worker,worker" {
		t.Fatalf("loop outputs = %#v, want one worker output per iteration", outputs)
	}
}

func TestGoogleADKFixedWorkflowAgentValidation(t *testing.T) {
	worker := newGoogleADKFixedWorkflowTestAgent(t, "worker")
	if _, err := newGoogleADKSequentialWorkflowAgent(" ", "missing name", []adkagent.Agent{worker}); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("blank name err = %v, want validation error", err)
	}
	if _, err := newGoogleADKParallelWorkflowAgent("parallel", "empty", nil); err == nil || !strings.Contains(err.Error(), "requires at least one sub-agent") {
		t.Fatalf("empty sub-agent err = %v, want validation error", err)
	}
	if _, err := newGoogleADKLoopWorkflowAgent("loop", "unbounded", []adkagent.Agent{worker}, 0); err == nil || !strings.Contains(err.Error(), "requires max iterations") {
		t.Fatalf("zero max iteration err = %v, want validation error", err)
	}
	if _, err := newGoogleADKLoopWorkflowAgent("loop", "empty", nil, 1); err == nil || !strings.Contains(err.Error(), "requires at least one sub-agent") {
		t.Fatalf("empty loop sub-agent err = %v, want validation error", err)
	}
}

func newGoogleADKFixedWorkflowTestAgent(t *testing.T, name string) adkagent.Agent {
	t.Helper()
	agent, err := adkagent.New(adkagent.Config{
		Name: name,
		Run: func(ctx adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
			return func(yield func(*adksession.Event, error) bool) {
				event := adksession.NewEvent(ctx, ctx.InvocationID())
				event.Author = name
				event.Output = name
				yield(event, nil)
			}
		},
	})
	if err != nil {
		t.Fatalf("agent.New(%q): %v", name, err)
	}
	return agent
}

func runGoogleADKFixedWorkflowAgent(t *testing.T, root adkagent.Agent) []string {
	t.Helper()
	ctx := context.Background()
	service := adksession.InMemoryService()
	if _, err := service.Create(ctx, &adksession.CreateRequest{AppName: "app", UserID: "user", SessionID: "session"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	runner, err := adkrunner.New(adkrunner.Config{
		AppName:        "app",
		Agent:          root,
		SessionService: service,
	})
	if err != nil {
		t.Fatalf("runner.New: %v", err)
	}
	var outputs []string
	for event, err := range runner.Run(ctx, "user", "session", genai.NewContentFromText("start", genai.RoleUser), adkagent.RunConfig{}) {
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if event == nil || event.Output == nil {
			continue
		}
		output, ok := event.Output.(string)
		if !ok {
			t.Fatalf("event output = %#v, want string", event.Output)
		}
		outputs = append(outputs, output)
	}
	return outputs
}
