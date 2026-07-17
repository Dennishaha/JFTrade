package adk

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"
	"time"

	adkagent "google.golang.org/adk/v2/agent"
	adkrunner "google.golang.org/adk/v2/runner"
	adksession "google.golang.org/adk/v2/session"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

type workflowAgentTestEvents []*adksession.Event

func (events workflowAgentTestEvents) All() iter.Seq[*adksession.Event] {
	return func(yield func(*adksession.Event) bool) {
		for _, event := range events {
			if !yield(event) {
				return
			}
		}
	}
}

func (events workflowAgentTestEvents) Len() int {
	return len(events)
}

func (events workflowAgentTestEvents) At(index int) *adksession.Event {
	return events[index]
}

type workflowAgentTestSession struct {
	id        string
	appName   string
	userID    string
	state     adksession.State
	events    workflowAgentTestEvents
	updatedAt time.Time
}

func (session workflowAgentTestSession) ID() string {
	return session.id
}

func (session workflowAgentTestSession) AppName() string {
	return session.appName
}

func (session workflowAgentTestSession) UserID() string {
	return session.userID
}

func (session workflowAgentTestSession) State() adksession.State {
	return session.state
}

func (session workflowAgentTestSession) Events() adksession.Events {
	return session.events
}

func (session workflowAgentTestSession) LastUpdateTime() time.Time {
	return session.updatedAt
}

type googleADKWorkflowNodeRunnerErrorDouble struct {
	events []*adksession.Event
	errors []error
}

func (runner *googleADKWorkflowNodeRunnerErrorDouble) RunNode(_ adkagent.Context, _ any) iter.Seq2[*adksession.Event, error] {
	return func(yield func(*adksession.Event, error) bool) {
		limit := max(len(runner.errors), len(runner.events))
		for index := range limit {
			var event *adksession.Event
			var err error
			if index < len(runner.events) {
				event = runner.events[index]
			}
			if index < len(runner.errors) {
				err = runner.errors[index]
			}
			if !yield(event, err) {
				return
			}
		}
	}
}

func TestGoogleADKWorkflowHelperCoverageBranches(t *testing.T) {
	t.Run("workflow helper functions cover nil events and partial text handling", func(t *testing.T) {
		var builder strings.Builder
		sawPartial := false

		googleADKWorkflowObserveVisibleReply(nil, nil, nil)
		if got := googleADKWorkflowInterruptIDs(nil); got != nil {
			t.Fatalf("googleADKWorkflowInterruptIDs(nil) = %#v, want nil", got)
		}
		if got := googleADKDecodeWorkflowInputResponse(nil); got != nil {
			t.Fatalf("googleADKDecodeWorkflowInputResponse(nil) = %#v, want nil", got)
		}
		if got := googleADKDecodeWorkflowInputResponse(&genai.FunctionResponse{Response: map[string]any{"response": true}}); got != true {
			t.Fatalf("googleADKDecodeWorkflowInputResponse(raw) = %#v, want true", got)
		}

		sawPartial = true
		googleADKWorkflowObserveVisibleReply(&builder, &sawPartial, &adksession.Event{})
		if sawPartial {
			t.Fatal("sawPartial was not reset after a non-partial empty event")
		}

		partialEvent := adksession.NewEvent(context.Background(), "partial")
		partialEvent.Partial = true
		partialEvent.Content = genai.NewContentFromText("partial", genai.RoleModel)
		googleADKWorkflowObserveVisibleReply(&builder, &sawPartial, partialEvent)
		if !sawPartial || builder.String() != "partial" {
			t.Fatalf("partial visible reply builder=%q sawPartial=%v", builder.String(), sawPartial)
		}

		ignoredFinalEvent := adksession.NewEvent(context.Background(), "ignored-final")
		ignoredFinalEvent.Content = genai.NewContentFromText("ignored-final", genai.RoleModel)
		googleADKWorkflowObserveVisibleReply(&builder, &sawPartial, ignoredFinalEvent)
		if sawPartial || builder.String() != "partial" {
			t.Fatalf("partial suppression builder=%q sawPartial=%v", builder.String(), sawPartial)
		}

		doneEvent := adksession.NewEvent(context.Background(), "done")
		doneEvent.Content = genai.NewContentFromText("done", genai.RoleModel)
		googleADKWorkflowObserveVisibleReply(&builder, &sawPartial, doneEvent)
		if builder.String() != "partialdone" {
			t.Fatalf("builder = %q, want partialdone", builder.String())
		}

		session := workflowAgentTestSession{
			id:      "workflow-helper-session",
			appName: "app",
			userID:  "user",
			events: workflowAgentTestEvents{
				nil,
				func() *adksession.Event {
					event := adksession.NewEvent(context.Background(), "answered")
					event.LongRunningToolIDs = []string{"keep-open", "answered"}
					event.Content = genai.NewContentFromParts([]*genai.Part{{
						FunctionResponse: &genai.FunctionResponse{
							ID:       "answered",
							Response: map[string]any{"ok": true},
						},
					}}, genai.RoleUser)
					return event
				}(),
			},
			updatedAt: time.Now().UTC(),
		}
		open := googleADKWorkflowOpenLongRunningCallIDs(session)
		if _, ok := open["keep-open"]; !ok || len(open) != 1 {
			t.Fatalf("googleADKWorkflowOpenLongRunningCallIDs = %#v, want keep-open only", open)
		}
		answered := googleADKWorkflowAnsweredOpenInterrupts(session)
		if !answered["answered"] || len(answered) != 1 {
			t.Fatalf("googleADKWorkflowAnsweredOpenInterrupts = %#v, want answered only", answered)
		}

		ignoredSession := workflowAgentTestSession{
			id: "workflow-helper-ignored-session",
			events: workflowAgentTestEvents{
				func() *adksession.Event {
					event := adksession.NewEvent(context.Background(), "ignored-response")
					event.LongRunningToolIDs = []string{"pending"}
					event.Content = genai.NewContentFromParts([]*genai.Part{{
						FunctionResponse: &genai.FunctionResponse{
							ID:       "",
							Response: map[string]any{"ignored": true},
						},
					}}, genai.RoleUser)
					return event
				}(),
			},
			updatedAt: time.Now().UTC(),
		}
		if answered := googleADKWorkflowAnsweredOpenInterrupts(ignoredSession); len(answered) != 0 {
			t.Fatalf("googleADKWorkflowAnsweredOpenInterrupts ignored = %#v, want empty", answered)
		}
	})

	t.Run("implicit input interruption is disabled", func(t *testing.T) {
		testCtx := &googleADKWorkflowAgentTestContext{
			StrictContextMock: adkagent.NewStrictContextMock(context.Background()),
			agent:             newNoopADKAgent(t, "workflow_helper_agent"),
			invocationID:      "invocation",
		}

		interrupted, err := googleADKWorkflowMaybeInterruptForImplicitInput(testCtx, "thanks for the update", func(*adksession.Event) error {
			return nil
		})
		if err != nil || interrupted {
			t.Fatalf("googleADKWorkflowMaybeInterruptForImplicitInput no-op interrupted=%v err=%v", interrupted, err)
		}
	})

	t.Run("workflow agent node body and implicit interruption error paths stay covered", func(t *testing.T) {
		testCtx := &googleADKWorkflowAgentTestContext{
			StrictContextMock: adkagent.NewStrictContextMock(context.Background()),
			invocationID:      "invocation",
		}

		generic, err := adkagent.New(adkagent.Config{
			Name: "workflow_node_generic",
			Run: func(adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
				return func(yield func(*adksession.Event, error) bool) {}
			},
		})
		if err != nil {
			t.Fatalf("agent.New generic: %v", err)
		}
		body := googleADKWorkflowAgentNodeBody(generic)
		if _, err := body(testCtx, "input", func(*adksession.Event) error { return nil }); err != nil {
			t.Fatalf("googleADKWorkflowAgentNodeBody generic err = %v", err)
		}

		nilEventRunner := &googleADKWorkflowNodeRunnerErrorDouble{
			events: []*adksession.Event{nil},
		}
		if _, err := googleADKWorkflowRunNode(nilEventRunner, testCtx, "input", false, func(*adksession.Event) error { return nil }); err != nil {
			t.Fatalf("googleADKWorkflowRunNode nil event err = %v", err)
		}

		errRunner := &googleADKWorkflowNodeRunnerErrorDouble{
			errors: []error{errors.New("runner failed")},
		}
		if _, err := googleADKWorkflowRunNode(errRunner, testCtx, "input", false, func(*adksession.Event) error { return nil }); err == nil || err.Error() != "runner failed" {
			t.Fatalf("googleADKWorkflowRunNode err = %v, want runner failed", err)
		}

		// A client may disconnect while the workflow adapter is forwarding an
		// event. Propagate that write failure so the caller can stop the run
		// rather than continuing with an invisible node result.
		sinkErr := errors.New("workflow event sink disconnected")
		emittingRunner := &googleADKWorkflowNodeRunnerErrorDouble{
			events: []*adksession.Event{adksession.NewEvent(context.Background(), "node-emits")},
		}
		if _, err := googleADKWorkflowRunNode(emittingRunner, testCtx, "input", false, func(*adksession.Event) error { return sinkErr }); !errors.Is(err, sinkErr) {
			t.Fatalf("googleADKWorkflowRunNode sink error = %v, want %v", err, sinkErr)
		}

		nilEventAgent, err := adkagent.New(adkagent.Config{
			Name: "workflow_generic_nil",
			Run: func(adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
				return func(yield func(*adksession.Event, error) bool) {
					yield(nil, nil)
				}
			},
		})
		if err != nil {
			t.Fatalf("agent.New nil generic: %v", err)
		}
		if _, err := googleADKWorkflowRunGenericAgent(nilEventAgent, testCtx, "input", false, func(*adksession.Event) error { return nil }); err != nil {
			t.Fatalf("googleADKWorkflowRunGenericAgent nil event err = %v", err)
		}

		errorAgent, err := adkagent.New(adkagent.Config{
			Name: "workflow_generic_error",
			Run: func(adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
				return func(yield func(*adksession.Event, error) bool) {
					yield(nil, errors.New("generic failed"))
				}
			},
		})
		if err != nil {
			t.Fatalf("agent.New error generic: %v", err)
		}
		if _, err := googleADKWorkflowRunGenericAgent(errorAgent, testCtx, "input", false, func(*adksession.Event) error { return nil }); err == nil || err.Error() != "generic failed" {
			t.Fatalf("googleADKWorkflowRunGenericAgent err = %v, want generic failed", err)
		}

		emittingAgent, err := adkagent.New(adkagent.Config{
			Name: "workflow_generic_sink_error",
			Run: func(adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
				return func(yield func(*adksession.Event, error) bool) {
					yield(adksession.NewEvent(context.Background(), "generic-emits"), nil)
				}
			},
		})
		if err != nil {
			t.Fatalf("agent.New emitting generic: %v", err)
		}
		if _, err := googleADKWorkflowRunGenericAgent(emittingAgent, testCtx, "input", false, func(*adksession.Event) error { return sinkErr }); !errors.Is(err, sinkErr) {
			t.Fatalf("googleADKWorkflowRunGenericAgent sink error = %v, want %v", err, sinkErr)
		}
	})

	t.Run("new workflow agent surfaces invalid bounded workflow definitions", func(t *testing.T) {
		if _, err := newGoogleADKWorkflowAgent(googleADKWorkflowAgentConfig{
			Name:           "broken_workflow",
			MaxConcurrency: 1,
		}); err == nil {
			t.Fatal("newGoogleADKWorkflowAgent accepted an invalid bounded workflow definition")
		}
	})

	t.Run("workflow adapter stops when the caller stops consuming run events", func(t *testing.T) {
		node := adkworkflow.NewEmittingFunctionNode("fresh", func(ctx adkagent.Context, _ any, emit func(*adksession.Event) error) (any, error) {
			event := adksession.NewEvent(context.Background(), ctx.InvocationID())
			event.Content = genai.NewContentFromText("fresh", genai.RoleModel)
			if err := emit(event); err != nil {
				return nil, err
			}
			return nil, nil
		}, adkworkflow.NodeConfig{})
		workflowAdapter := &googleADKWorkflowAgent{workflow: mustGoogleADKWorkflow(t, []adkworkflow.Edge{{From: adkworkflow.Start, To: node}})}
		testCtx := &googleADKWorkflowAgentTestContext{
			StrictContextMock: adkagent.NewStrictContextMock(context.Background()),
			session: workflowAgentTestSession{
				id:        "fresh-run-session",
				appName:   "app",
				userID:    "user",
				updatedAt: time.Now().UTC(),
			},
			invocationID: "fresh-run-invocation",
		}
		yielded := 0
		workflowAdapter.run(testCtx)(func(*adksession.Event, error) bool {
			yielded++
			return false
		})
		if yielded != 1 {
			t.Fatalf("fresh workflow yielded %d events, want 1 before caller stop", yielded)
		}
	})

	t.Run("workflow adapter resumes from persisted waiting input and stops when caller stops consuming", func(t *testing.T) {
		asker := adkworkflow.NewEmittingFunctionNode("asker", func(ctx adkagent.Context, _ any, emit func(*adksession.Event) error) (any, error) {
			if reply, ok := ctx.ResumedInput("ask-resume"); ok {
				return reply, nil
			}
			if err := emit(adkworkflow.NewRequestInputEvent(ctx, adksession.RequestInput{
				InterruptID: "ask-resume",
				Message:     "resume?",
			})); err != nil {
				return nil, err
			}
			return nil, adkworkflow.ErrNodeInterrupted
		}, adkworkflow.NodeConfig{RerunOnResume: &googleADKWorkflowRerunOnResume})
		workflowAdapter := &googleADKWorkflowAgent{
			workflow: mustGoogleADKWorkflow(t, []adkworkflow.Edge{{From: adkworkflow.Start, To: asker}}),
		}
		root, err := adkagent.New(adkagent.Config{Name: "resume_root", Run: workflowAdapter.run})
		if err != nil {
			t.Fatalf("agent.New resume root: %v", err)
		}
		service := adksession.InMemoryService()
		if _, err := service.Create(context.Background(), &adksession.CreateRequest{
			AppName: "app", UserID: "user", SessionID: "resume-session",
		}); err != nil {
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
		invocationID := ""
		for event, err := range runner.Run(context.Background(), "user", "resume-session", genai.NewContentFromText("start", genai.RoleUser), adkagent.RunConfig{}) {
			if err != nil {
				t.Fatalf("fresh run: %v", err)
			}
			if event != nil && event.RequestedInput != nil {
				invocationID = event.InvocationID
				break
			}
		}
		if invocationID == "" {
			t.Fatal("fresh run did not produce a resumable invocation id")
		}
		testCtx := &googleADKWorkflowAgentTestContext{
			StrictContextMock: adkagent.NewStrictContextMock(context.Background()),
			session: func() adksession.Session {
				resp, err := service.Get(context.Background(), &adksession.GetRequest{
					AppName: "app", UserID: "user", SessionID: "resume-session",
				})
				if err != nil {
					t.Fatalf("Get session: %v", err)
				}
				return resp.Session
			}(),
			invocationID: invocationID,
			userContent: genai.NewContentFromParts([]*genai.Part{{
				FunctionResponse: &genai.FunctionResponse{
					ID:       "ask-resume",
					Name:     adkworkflow.WorkflowInputFunctionCallName,
					Response: map[string]any{"response": "approved"},
				},
			}}, genai.RoleUser),
		}
		yielded := 0
		workflowAdapter.run(testCtx)(func(event *adksession.Event, err error) bool {
			if err != nil {
				t.Fatalf("resume yielded err = %v", err)
			}
			yielded++
			return false
		})
		if yielded != 1 {
			t.Fatalf("resume yielded %d events, want 1 before caller stop", yielded)
		}
	})
}
