package adk

import (
	"context"
	"fmt"
	"iter"
	"sync/atomic"
	"testing"

	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	adkmodel "google.golang.org/adk/v2/model"
	adkrunner "google.golang.org/adk/v2/runner"
	adksession "google.golang.org/adk/v2/session"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

func TestGoogleADKWorkflowNativeAgentNodeForwardsPartialAndFinalOutput(t *testing.T) {
	child, err := adkagent.New(adkagent.Config{
		Name: "native_streaming_child",
		Run: func(ctx adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
			return func(yield func(*adksession.Event, error) bool) {
				partial := adksession.NewEvent(ctx, ctx.InvocationID())
				partial.Partial = true
				partial.Content = genai.NewContentFromText("par", genai.RoleModel)
				if !yield(partial, nil) {
					return
				}
				final := adksession.NewEvent(ctx, ctx.InvocationID())
				final.Content = genai.NewContentFromText("final", genai.RoleModel)
				yield(final, nil)
			}
		},
	})
	if err != nil {
		t.Fatalf("agent.New child: %v", err)
	}
	childNode, err := adkworkflow.NewAgentNode(child, adkworkflow.NodeConfig{})
	if err != nil {
		t.Fatalf("workflow.NewAgentNode: %v", err)
	}
	var successorCalls int
	successor := adkworkflow.NewFunctionNode("stream_successor", func(_ adkagent.Context, input any) (any, error) {
		successorCalls++
		if input != "final" {
			return nil, fmt.Errorf("successor input = %#v, want final output", input)
		}
		return "done", nil
	}, adkworkflow.NodeConfig{})
	runner := newNativeIntegrationWorkflowRunner(t, "native_streaming_workflow", []adkworkflow.Edge{
		{From: adkworkflow.Start, To: childNode},
		{From: childNode, To: successor},
	})
	partialEvents := 0
	for event, runErr := range runner.Run(t.Context(), "user", "session", genai.NewContentFromText("start", genai.RoleUser), adkagent.RunConfig{}) {
		if runErr != nil {
			t.Fatalf("runner.Run: %v", runErr)
		}
		if event != nil && event.Partial {
			partialEvents++
			if event.Output != nil {
				t.Fatalf("partial output = %#v, want nil", event.Output)
			}
		}
	}
	if partialEvents != 1 || successorCalls != 1 {
		t.Fatalf("partialEvents=%d successorCalls=%d, want 1/1", partialEvents, successorCalls)
	}
}

func TestGoogleADKWorkflowNativeAgentNodeStopsWithConsumer(t *testing.T) {
	var afterFirst atomic.Int32
	var successorCalls atomic.Int32
	child, err := adkagent.New(adkagent.Config{
		Name: "native_stoppable_child",
		Run: func(ctx adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
			return func(yield func(*adksession.Event, error) bool) {
				first := adksession.NewEvent(ctx, ctx.InvocationID())
				first.Output = "first"
				if !yield(first, nil) {
					return
				}
				afterFirst.Add(1)
				second := adksession.NewEvent(ctx, ctx.InvocationID())
				second.Output = "second"
				yield(second, nil)
			}
		},
	})
	if err != nil {
		t.Fatalf("agent.New child: %v", err)
	}
	childNode, err := adkworkflow.NewAgentNode(child, adkworkflow.NodeConfig{})
	if err != nil {
		t.Fatalf("workflow.NewAgentNode: %v", err)
	}
	successor := adkworkflow.NewFunctionNode("stop_successor", func(_ adkagent.Context, _ any) (any, error) {
		successorCalls.Add(1)
		return nil, nil
	}, adkworkflow.NodeConfig{})
	runner := newNativeIntegrationWorkflowRunner(t, "native_stoppable_workflow", []adkworkflow.Edge{{From: adkworkflow.Start, To: childNode}, {From: childNode, To: successor}})
	stoppedOnChild := false
	runner.Run(t.Context(), "user", "session", genai.NewContentFromText("start", genai.RoleUser), adkagent.RunConfig{})(func(event *adksession.Event, runErr error) bool {
		if runErr != nil {
			t.Fatalf("runner yielded error: %v", runErr)
		}
		if event != nil && event.Author == child.Name() {
			stoppedOnChild = true
			return false
		}
		return true
	})
	if !stoppedOnChild || afterFirst.Load() != 0 || successorCalls.Load() != 0 {
		t.Fatalf("stoppedOnChild=%v afterFirst=%d successorCalls=%d, want true/0/0", stoppedOnChild, afterFirst.Load(), successorCalls.Load())
	}
}

func TestGoogleADKWorkflowNativeAgentNodePreservesBranchAndIsolation(t *testing.T) {
	var branch, scope, eventScope string
	child, err := adkagent.New(adkagent.Config{
		Name: "native_scoped_child",
		Run: func(ctx adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
			branch = ctx.Branch()
			scope = ctx.IsolationScope()
			return func(yield func(*adksession.Event, error) bool) {
				event := adksession.NewEvent(ctx, ctx.InvocationID())
				event.Output = "scoped"
				if yield(event, nil) {
					eventScope = event.IsolationScope
				}
			}
		},
	})
	if err != nil {
		t.Fatalf("agent.New child: %v", err)
	}
	childNode, err := adkworkflow.NewAgentNode(child, adkworkflow.NodeConfig{})
	if err != nil {
		t.Fatalf("workflow.NewAgentNode: %v", err)
	}
	parent := adkworkflow.NewDynamicNode("scoped_parent", func(ctx adkagent.Context, _ any, _ func(*adksession.Event) error) (any, error) {
		return adkworkflow.RunNode[string](ctx, childNode, "input",
			adkworkflow.WithOverrideBranch("workflow-branch"),
			adkworkflow.WithIsolationScope("workflow-scope"),
		)
	}, adkworkflow.NodeConfig{})
	runner := newNativeIntegrationWorkflowRunner(t, "native_scoped_workflow", []adkworkflow.Edge{{From: adkworkflow.Start, To: parent}})
	for _, runErr := range runNativeIntegrationWorkflowContent(t, runner, "session", genai.NewContentFromText("start", genai.RoleUser)) {
		if runErr != nil {
			t.Fatalf("runner.Run: %v", runErr)
		}
	}
	if branch != "workflow-branch" || scope != "workflow-scope" || eventScope != "workflow-scope" {
		t.Fatalf("branch=%q scope=%q eventScope=%q, want workflow-branch/workflow-scope/workflow-scope", branch, scope, eventScope)
	}
}

func TestGoogleADKWorkflowNativeAgentNodeResumesConfirmationAfterRecreation(t *testing.T) {
	var toolRuns atomic.Int32
	service := adksession.InMemoryService()
	if _, err := service.Create(t.Context(), &adksession.CreateRequest{AppName: "app", UserID: "user", SessionID: "session"}); err != nil {
		t.Fatalf("session.Create: %v", err)
	}
	freshRunner := newNativeIntegrationConfirmationRunner(t, service, "native_restart_workflow", false, &toolRuns, nil)
	var confirmationID string
	for event, runErr := range freshRunner.Run(t.Context(), "user", "session", genai.NewContentFromText("start", genai.RoleUser), adkagent.RunConfig{}) {
		if runErr != nil {
			t.Fatalf("fresh runner.Run: %v", runErr)
		}
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part != nil && part.FunctionCall != nil && part.FunctionCall.Name == toolconfirmation.FunctionCallName {
				confirmationID = part.FunctionCall.ID
			}
		}
	}
	if confirmationID == "" || toolRuns.Load() != 0 {
		t.Fatalf("confirmationID=%q toolRuns=%d, want pending confirmation and no execution", confirmationID, toolRuns.Load())
	}
	var successorCalls atomic.Int32
	resumedRunner := newNativeIntegrationConfirmationRunner(t, service, "native_restart_workflow", true, &toolRuns, &successorCalls)
	response := genai.NewContentFromParts([]*genai.Part{{FunctionResponse: &genai.FunctionResponse{
		ID: confirmationID, Name: toolconfirmation.FunctionCallName, Response: map[string]any{"confirmed": true},
	}}}, genai.RoleUser)
	for _, runErr := range runNativeIntegrationWorkflowContent(t, resumedRunner, "session", response) {
		if runErr != nil {
			t.Fatalf("resumed runner.Run: %v", runErr)
		}
	}
	if toolRuns.Load() != 1 || successorCalls.Load() != 1 {
		t.Fatalf("toolRuns=%d successorCalls=%d, want 1/1 after recreation", toolRuns.Load(), successorCalls.Load())
	}
}
func newNativeIntegrationWorkflowRunner(t *testing.T, name string, edges []adkworkflow.Edge) *adkrunner.Runner {
	t.Helper()
	workflow := mustGoogleADKWorkflow(t, edges)
	root, err := adkagent.New(adkagent.Config{Name: name, Run: (&googleADKWorkflowAgent{workflow: workflow}).run})
	if err != nil {
		t.Fatalf("agent.New root: %v", err)
	}
	service := adksession.InMemoryService()
	if _, err := service.Create(t.Context(), &adksession.CreateRequest{AppName: "app", UserID: "user", SessionID: "session"}); err != nil {
		t.Fatalf("session.Create: %v", err)
	}
	runner, err := adkrunner.New(adkrunner.Config{AppName: "app", Agent: root, SessionService: service})
	if err != nil {
		t.Fatalf("runner.New: %v", err)
	}
	return runner
}
func runNativeIntegrationWorkflowContent(t *testing.T, runner *adkrunner.Runner, sessionID string, content *genai.Content) []error {
	t.Helper()
	var errs []error
	for _, runErr := range runner.Run(t.Context(), "user", sessionID, content, adkagent.RunConfig{}) {
		errs = append(errs, runErr)
	}
	return errs
}

func newNativeIntegrationConfirmationRunner(t *testing.T, service adksession.Service, name string, resumed bool, toolRuns, successorCalls *atomic.Int32) *adkrunner.Runner {
	t.Helper()
	secureTool, err := functiontool.New(functiontool.Config{Name: "restart_secure_action", Description: "requires approval", RequireConfirmation: true}, func(_ adkagent.Context, _ struct{}) (map[string]any, error) {
		toolRuns.Add(1)
		return map[string]any{"approved": true}, nil
	})
	if err != nil {
		t.Fatalf("functiontool.New: %v", err)
	}
	child, err := llmagent.New(llmagent.Config{
		Name: "native_restart_child", Model: &nativeIntegrationConfirmationModel{resumed: resumed}, Tools: []adktool.Tool{secureTool},
	})
	if err != nil {
		t.Fatalf("llmagent.New: %v", err)
	}
	childNode, err := adkworkflow.NewAgentNode(child, adkworkflow.NodeConfig{RerunOnResume: &googleADKWorkflowRerunOnResume})
	if err != nil {
		t.Fatalf("workflow.NewAgentNode: %v", err)
	}
	successor := adkworkflow.NewFunctionNode("restart_successor", func(_ adkagent.Context, input any) (any, error) {
		if input != "approved complete" {
			return nil, fmt.Errorf("successor input = %#v, want approved complete", input)
		}
		if successorCalls != nil {
			successorCalls.Add(1)
		}
		return input, nil
	}, adkworkflow.NodeConfig{})
	workflow := mustGoogleADKWorkflow(t, []adkworkflow.Edge{{From: adkworkflow.Start, To: childNode}, {From: childNode, To: successor}})
	root, err := adkagent.New(adkagent.Config{Name: name, Run: (&googleADKWorkflowAgent{workflow: workflow}).run})
	if err != nil {
		t.Fatalf("agent.New root: %v", err)
	}
	runner, err := adkrunner.New(adkrunner.Config{AppName: "app", Agent: root, SessionService: service})
	if err != nil {
		t.Fatalf("runner.New: %v", err)
	}
	return runner
}

type nativeIntegrationConfirmationModel struct{ resumed bool }

func (m *nativeIntegrationConfirmationModel) Name() string {
	return "native-integration-confirmation-model"
}

func (m *nativeIntegrationConfirmationModel) GenerateContent(_ context.Context, _ *adkmodel.LLMRequest, _ bool) iter.Seq2[*adkmodel.LLMResponse, error] {
	return func(yield func(*adkmodel.LLMResponse, error) bool) {
		if m.resumed {
			yield(&adkmodel.LLMResponse{Content: genai.NewContentFromText("approved complete", genai.RoleModel)}, nil)
			return
		}
		yield(&adkmodel.LLMResponse{Content: genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
			ID: "restart-secure-call", Name: "restart_secure_action", Args: map[string]any{},
		}}}, genai.RoleModel)}, nil)
	}
}
