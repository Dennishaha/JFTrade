package adk

import (
	"context"
	"errors"
	"iter"
	"testing"

	adkagent "google.golang.org/adk/v2/agent"
	adksession "google.golang.org/adk/v2/session"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

type googleADKWorkflowNodeRunnerErrorDouble struct {
	err    error
	events []*adksession.Event
}

func (r *googleADKWorkflowNodeRunnerErrorDouble) RunNode(_ adkagent.Context, _ any) iter.Seq2[*adksession.Event, error] {
	return func(yield func(*adksession.Event, error) bool) {
		for _, event := range r.events {
			if !yield(event, nil) {
				return
			}
		}
		yield(nil, r.err)
	}
}

func TestGoogleWorkflowAgentAdditionalBoundaryBranches(t *testing.T) {
	t.Run("helper decoding and interrupt collection cover nil and fallback branches", func(t *testing.T) {
		content := genai.NewContentFromParts([]*genai.Part{
			nil,
			{FunctionResponse: &genai.FunctionResponse{ID: "req", Name: adkworkflow.WorkflowInputFunctionCallName, Response: map[string]any{"response": true}}},
		}, genai.RoleUser)
		if !googleADKWorkflowHasFunctionResponse(content) {
			t.Fatal("googleADKWorkflowHasFunctionResponse = false, want true")
		}
		if googleADKWorkflowHasFunctionResponse(nil) {
			t.Fatal("googleADKWorkflowHasFunctionResponse(nil) = true, want false")
		}

		if googleADKWorkflowInputToUserContent(nil) != nil {
			t.Fatal("googleADKWorkflowInputToUserContent(nil) should be nil")
		}
		original := genai.NewContentFromText("hello", genai.RoleUser)
		if got := googleADKWorkflowInputToUserContent(original); got != original {
			t.Fatalf("googleADKWorkflowInputToUserContent(content) = %#v, want original pointer", got)
		}
		if googleADKWorkflowInputToUserContent("") != nil {
			t.Fatal("googleADKWorkflowInputToUserContent(empty string) should be nil")
		}
		if googleADKWorkflowInputToUserContent(make(chan int)) != nil {
			t.Fatal("googleADKWorkflowInputToUserContent(unmarshalable) should be nil")
		}

		waiting := googleADKWorkflowWaitingInterruptIDs(&adkworkflow.RunState{
			Nodes: map[string]*adkworkflow.NodeState{
				"done":   {Status: adkworkflow.NodeCompleted, Interrupts: []string{"ignored"}},
				"wait":   {Status: adkworkflow.NodeWaiting, Interrupts: []string{"ask", ""}},
				"nilone": nil,
			},
		})
		if len(waiting) != 1 || waiting["ask"] != struct{}{} {
			t.Fatalf("googleADKWorkflowWaitingInterruptIDs = %#v, want ask only", waiting)
		}

		if open := googleADKWorkflowOpenLongRunningCallIDs(nil); len(open) != 0 {
			t.Fatalf("googleADKWorkflowOpenLongRunningCallIDs(nil) = %#v, want empty", open)
		}
		if answered := googleADKWorkflowAnsweredOpenInterrupts(nil); len(answered) != 0 {
			t.Fatalf("googleADKWorkflowAnsweredOpenInterrupts(nil) = %#v, want empty", answered)
		}

		if got := googleADKDecodeWorkflowInputResponse(nil); got != nil {
			t.Fatalf("googleADKDecodeWorkflowInputResponse(nil) = %#v, want nil", got)
		}
		if got := googleADKDecodeWorkflowInputResponse(&genai.FunctionResponse{Response: map[string]any{"response": 12}}); got != 12 {
			t.Fatalf("googleADKDecodeWorkflowInputResponse(non-string response) = %#v, want 12", got)
		}
		if got := googleADKDecodeWorkflowInputResponse(&genai.FunctionResponse{Response: map[string]any{"payload": map[string]any{"ok": true}}}); got.(map[string]any)["ok"] != true {
			t.Fatalf("googleADKDecodeWorkflowInputResponse(payload) = %#v", got)
		}
		if got := googleADKDecodeWorkflowInputResponse(&genai.FunctionResponse{Response: map[string]any{"other": "value"}}); got.(map[string]any)["other"] != "value" {
			t.Fatalf("googleADKDecodeWorkflowInputResponse(fallback) = %#v", got)
		}
	})

	t.Run("run node and generic agent surface runner and emitter failures", func(t *testing.T) {
		testCtx := &googleADKWorkflowAgentTestContext{
			StrictContextMock: adkagent.NewStrictContextMock(context.Background()),
			invocationID:      "invocation",
		}
		runner := &googleADKWorkflowNodeRunnerErrorDouble{
			events: []*adksession.Event{{}, nil},
			err:    errors.New("runner failed"),
		}
		if _, err := googleADKWorkflowRunNode(runner, testCtx, "task", false, func(*adksession.Event) error { return nil }); err == nil || err.Error() != "runner failed" {
			t.Fatalf("googleADKWorkflowRunNode runner err = %v, want runner failed", err)
		}
		emitErrRunner := &googleADKWorkflowNodeRunnerErrorDouble{events: []*adksession.Event{{}}}
		if _, err := googleADKWorkflowRunNode(emitErrRunner, testCtx, "task", false, func(*adksession.Event) error { return errors.New("emit failed") }); err == nil || err.Error() != "emit failed" {
			t.Fatalf("googleADKWorkflowRunNode emit err = %v, want emit failed", err)
		}

		agent, err := adkagent.New(adkagent.Config{
			Name: "generic-error",
			Run: func(adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
				return func(yield func(*adksession.Event, error) bool) {
					yield(nil, nil)
					yield(&adksession.Event{}, nil)
				}
			},
		})
		if err != nil {
			t.Fatalf("agent.New: %v", err)
		}
		if _, err := googleADKWorkflowRunGenericAgent(agent, testCtx, "task", false, func(*adksession.Event) error { return errors.New("generic emit failed") }); err == nil || err.Error() != "generic emit failed" {
			t.Fatalf("googleADKWorkflowRunGenericAgent emit err = %v, want generic emit failed", err)
		}
	})

	t.Run("workflow agent returns nothing-to-resume when state cannot be reconstructed", func(t *testing.T) {
		workflowAdapter := &googleADKWorkflowAgent{
			workflow: mustGoogleADKWorkflow(t, []adkworkflow.Edge{{From: adkworkflow.Start, To: &workflowCompilerTestNode{BaseNode: adkworkflow.NewBaseNode("node", "", adkworkflow.NodeConfig{})}}}),
		}
		ctx := &googleADKWorkflowAgentTestContext{
			StrictContextMock: adkagent.NewStrictContextMock(context.Background()),
			invocationID:      "missing",
			userContent: genai.NewContentFromParts([]*genai.Part{{
				FunctionResponse: &genai.FunctionResponse{ID: "unknown", Name: adkworkflow.WorkflowInputFunctionCallName, Response: map[string]any{"response": "ok"}},
			}}, genai.RoleUser),
		}
		var gotErr error
		for _, err := range workflowAdapter.run(ctx) {
			if err != nil {
				gotErr = err
				break
			}
		}
		if !errors.Is(gotErr, adkworkflow.ErrNothingToResume) {
			t.Fatalf("workflowAdapter.run err = %v, want ErrNothingToResume", gotErr)
		}
	})
}
