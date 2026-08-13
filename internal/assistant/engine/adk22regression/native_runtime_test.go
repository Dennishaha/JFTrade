package adk22regression_test

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"
	"testing"
	"time"

	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/agent/workflowagent"
	adkmodel "google.golang.org/adk/v2/model"
	adkrunner "google.golang.org/adk/v2/runner"
	adksession "google.golang.org/adk/v2/session"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

func TestConfirmedToolsResumeInRequestOrder(t *testing.T) {
	var toolRuns atomic.Int32
	first := newConfirmedTool(t, "ordered_secure_first", &toolRuns)
	second := newConfirmedTool(t, "ordered_secure_second", &toolRuns)
	root, err := llmagent.New(llmagent.Config{
		Name: "ordered_confirmation_agent", Model: &orderedConfirmationModel{}, Tools: []adktool.Tool{first, second},
	})
	if err != nil {
		t.Fatalf("llmagent.New: %v", err)
	}
	runner := newRunner(t, root, "ordered-session")

	confirmationIDs := map[string]string{}
	for event, runErr := range runner.Run(t.Context(), "user", "ordered-session", genai.NewContentFromText("start", genai.RoleUser), adkagent.RunConfig{}) {
		if runErr != nil {
			t.Fatalf("fresh runner.Run: %v", runErr)
		}
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part == nil || part.FunctionCall == nil || part.FunctionCall.Name != toolconfirmation.FunctionCallName {
				continue
			}
			original, decodeErr := toolconfirmation.OriginalCallFrom(part.FunctionCall)
			if decodeErr != nil {
				t.Fatalf("decode original confirmation call: %v", decodeErr)
			}
			confirmationIDs[original.Name] = part.FunctionCall.ID
		}
	}
	firstID, secondID := confirmationIDs["ordered_secure_first"], confirmationIDs["ordered_secure_second"]
	if firstID == "" || secondID == "" {
		t.Fatalf("confirmation ids = %#v, want both calls", confirmationIDs)
	}

	response := genai.NewContentFromParts([]*genai.Part{
		{FunctionResponse: &genai.FunctionResponse{ID: secondID, Name: toolconfirmation.FunctionCallName, Response: map[string]any{"confirmed": true}}},
		{FunctionResponse: &genai.FunctionResponse{ID: firstID, Name: toolconfirmation.FunctionCallName, Response: map[string]any{"confirmed": true}}},
	}, genai.RoleUser)
	var responseOrder []string
	for event, runErr := range runner.Run(t.Context(), "user", "ordered-session", response, adkagent.RunConfig{}) {
		if runErr != nil {
			t.Fatalf("resumed runner.Run: %v", runErr)
		}
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part != nil && part.FunctionResponse != nil && (part.FunctionResponse.Name == "ordered_secure_first" || part.FunctionResponse.Name == "ordered_secure_second") {
				responseOrder = append(responseOrder, part.FunctionResponse.Name)
			}
		}
	}
	if len(responseOrder) != 2 || responseOrder[0] != "ordered_secure_first" || responseOrder[1] != "ordered_secure_second" {
		t.Fatalf("confirmed response order = %#v, want first then second", responseOrder)
	}
	if toolRuns.Load() != 2 {
		t.Fatalf("tool runs = %d, want 2", toolRuns.Load())
	}
}

func TestWorkflowGraphResumesByInterruptIDAndPreservesEventOrder(t *testing.T) {
	var handled atomic.Value
	asker := &requestInputNode{BaseNode: adkworkflow.NewBaseNode("request_review", "", adkworkflow.NodeConfig{}), interruptID: "review-1"}
	handler := adkworkflow.NewFunctionNode("record_review", func(_ adkagent.Context, input string) (string, error) {
		handled.Store(input)
		return "handled:" + input, nil
	}, adkworkflow.NodeConfig{})
	root, err := workflowagent.New(workflowagent.Config{Name: "resume_graph", Edges: adkworkflow.Chain(adkworkflow.Start, asker, handler)})
	if err != nil {
		t.Fatalf("workflowagent.New: %v", err)
	}
	service := adksession.InMemoryService()
	if _, err := service.Create(t.Context(), &adksession.CreateRequest{AppName: "app", UserID: "user", SessionID: "workflow-resume"}); err != nil {
		t.Fatalf("session.Create: %v", err)
	}
	runner, err := adkrunner.New(adkrunner.Config{AppName: "app", Agent: root, SessionService: service})
	if err != nil {
		t.Fatalf("runner.New: %v", err)
	}

	turnOne := collectEvents(t, runner, "workflow-resume", genai.NewContentFromText("draft", genai.RoleUser))
	callID, callName := workflowInterrupt(turnOne)
	if callID != "review-1" || callName != adkworkflow.WorkflowInputFunctionCallName {
		t.Fatalf("workflow interrupt = %q/%q", callID, callName)
	}
	if handled.Load() != nil {
		t.Fatal("successor ran before workflow input was resumed")
	}

	resume := genai.NewContentFromParts([]*genai.Part{{FunctionResponse: &genai.FunctionResponse{
		ID: callID, Name: callName, Response: map[string]any{"payload": "approved"},
	}}}, genai.RoleUser)
	turnTwo := collectEvents(t, runner, "workflow-resume", resume)
	if nextID, _ := workflowInterrupt(turnTwo); nextID != "" {
		t.Fatalf("resume emitted another interrupt %q", nextID)
	}
	if got := handled.Load(); got != "approved" {
		t.Fatalf("successor input = %v, want approved", got)
	}

	stored, err := service.Get(t.Context(), &adksession.GetRequest{AppName: "app", UserID: "user", SessionID: "workflow-resume"})
	if err != nil {
		t.Fatalf("session.Get: %v", err)
	}
	callIndex, responseIndex, outputIndex := -1, -1, -1
	index := 0
	for event := range stored.Session.Events().All() {
		if event != nil {
			if event.Output == "handled:approved" {
				outputIndex = index
			}
			if event.Content != nil {
				for _, part := range event.Content.Parts {
					switch {
					case part != nil && part.FunctionCall != nil && part.FunctionCall.ID == callID:
						callIndex = index
					case part != nil && part.FunctionResponse != nil && part.FunctionResponse.ID == callID:
						responseIndex = index
					}
				}
			}
		}
		index++
	}
	if callIndex < 0 || responseIndex <= callIndex || outputIndex <= responseIndex {
		t.Fatalf("persisted event order call=%d response=%d output=%d", callIndex, responseIndex, outputIndex)
	}
}

func TestWorkflowPropagatesExternalCancellationWithoutSuccessorOrRetry(t *testing.T) {
	tests := []struct {
		name      string
		deadline  bool
		wantError error
	}{{name: "cancelled", wantError: context.Canceled}, {name: "timed-out", deadline: true, wantError: context.DeadlineExceeded}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var attempts, successorCalls atomic.Int32
			started := make(chan struct{}, 1)
			retry := adkworkflow.DefaultRetryConfig()
			retry.MaxAttempts = 3
			retry.InitialDelay = time.Hour
			retry.ShouldRetry = func(error) bool { return true }
			blocking := adkworkflow.NewFunctionNode("blocking", func(ctx adkagent.Context, _ any) (any, error) {
				attempts.Add(1)
				started <- struct{}{}
				<-ctx.Done()
				return nil, ctx.Err()
			}, adkworkflow.NodeConfig{RetryConfig: retry})
			successor := adkworkflow.NewFunctionNode("successor", func(_ adkagent.Context, _ any) (any, error) {
				successorCalls.Add(1)
				return nil, nil
			}, adkworkflow.NodeConfig{})
			root, err := workflowagent.New(workflowagent.Config{Name: "external_" + test.name, Edges: []adkworkflow.Edge{
				{From: adkworkflow.Start, To: blocking}, {From: blocking, To: successor},
			}})
			if err != nil {
				t.Fatalf("workflowagent.New: %v", err)
			}
			runner := newRunner(t, root, test.name+"-session")

			var runCtx context.Context
			var cancel context.CancelFunc
			if test.deadline {
				runCtx, cancel = context.WithTimeout(t.Context(), 100*time.Millisecond)
			} else {
				runCtx, cancel = context.WithCancel(t.Context())
			}
			defer cancel()
			errCh := make(chan error, 1)
			go func() {
				var firstErr error
				for _, runErr := range runner.Run(runCtx, "user", test.name+"-session", genai.NewContentFromText("start", genai.RoleUser), adkagent.RunConfig{}) {
					if firstErr == nil && runErr != nil {
						firstErr = runErr
					}
				}
				errCh <- firstErr
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("workflow node did not start")
			}
			if !test.deadline {
				cancel()
			}
			select {
			case runErr := <-errCh:
				if !errors.Is(runErr, test.wantError) {
					t.Fatalf("runner error = %v, want %v", runErr, test.wantError)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("workflow did not stop after external cancellation")
			}
			if attempts.Load() != 1 || successorCalls.Load() != 0 {
				t.Fatalf("attempts=%d successorCalls=%d, want 1/0", attempts.Load(), successorCalls.Load())
			}
		})
	}
}

func newConfirmedTool(t *testing.T, name string, runs *atomic.Int32) adktool.Tool {
	t.Helper()
	tool, err := functiontool.New(functiontool.Config{Name: name, Description: name, RequireConfirmation: true}, func(_ adkagent.Context, _ struct{}) (map[string]any, error) {
		runs.Add(1)
		return map[string]any{"name": name}, nil
	})
	if err != nil {
		t.Fatalf("functiontool.New(%s): %v", name, err)
	}
	return tool
}

type requestInputNode struct {
	adkworkflow.BaseNode
	interruptID string
}

func (n *requestInputNode) Run(ctx adkagent.Context, input any) iter.Seq2[*adksession.Event, error] {
	return func(yield func(*adksession.Event, error) bool) {
		yield(adkworkflow.NewRequestInputEvent(ctx, adksession.RequestInput{
			InterruptID: n.interruptID,
			Message:     "review the workflow output",
			Payload:     input,
		}), nil)
	}
}

func collectEvents(t *testing.T, runner *adkrunner.Runner, sessionID string, content *genai.Content) []*adksession.Event {
	t.Helper()
	events := make([]*adksession.Event, 0)
	for event, err := range runner.Run(t.Context(), "user", sessionID, content, adkagent.RunConfig{}) {
		if err != nil {
			t.Fatalf("runner.Run: %v", err)
		}
		if event != nil {
			events = append(events, event)
		}
	}
	return events
}

func workflowInterrupt(events []*adksession.Event) (string, string) {
	for _, event := range events {
		if event == nil || event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part != nil && part.FunctionCall != nil && part.FunctionCall.Name == adkworkflow.WorkflowInputFunctionCallName {
				return part.FunctionCall.ID, part.FunctionCall.Name
			}
		}
	}
	return "", ""
}

func newRunner(t *testing.T, root adkagent.Agent, sessionID string) *adkrunner.Runner {
	t.Helper()
	service := adksession.InMemoryService()
	if _, err := service.Create(t.Context(), &adksession.CreateRequest{AppName: "app", UserID: "user", SessionID: sessionID}); err != nil {
		t.Fatalf("session.Create: %v", err)
	}
	runner, err := adkrunner.New(adkrunner.Config{AppName: "app", Agent: root, SessionService: service})
	if err != nil {
		t.Fatalf("runner.New: %v", err)
	}
	return runner
}

type orderedConfirmationModel struct{ calls atomic.Int32 }

func (m *orderedConfirmationModel) Name() string { return "ordered-confirmation-model" }

func (m *orderedConfirmationModel) GenerateContent(_ context.Context, _ *adkmodel.LLMRequest, _ bool) iter.Seq2[*adkmodel.LLMResponse, error] {
	return func(yield func(*adkmodel.LLMResponse, error) bool) {
		if m.calls.Add(1) > 1 {
			yield(&adkmodel.LLMResponse{Content: genai.NewContentFromText("approved complete", genai.RoleModel)}, nil)
			return
		}
		yield(&adkmodel.LLMResponse{Content: genai.NewContentFromParts([]*genai.Part{
			{FunctionCall: &genai.FunctionCall{ID: "first-call", Name: "ordered_secure_first", Args: map[string]any{}}},
			{FunctionCall: &genai.FunctionCall{ID: "second-call", Name: "ordered_secure_second", Args: map[string]any{}}},
		}, genai.RoleModel)}, nil)
	}
}
