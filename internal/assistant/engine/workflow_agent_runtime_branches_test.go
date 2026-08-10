package adk

import (
	"context"
	"iter"
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

func (events workflowAgentTestEvents) Len() int { return len(events) }

func (events workflowAgentTestEvents) At(index int) *adksession.Event { return events[index] }

type workflowAgentTestSession struct {
	id        string
	appName   string
	userID    string
	state     adksession.State
	events    workflowAgentTestEvents
	updatedAt time.Time
}

func (session workflowAgentTestSession) ID() string { return session.id }

func (session workflowAgentTestSession) AppName() string { return session.appName }

func (session workflowAgentTestSession) UserID() string { return session.userID }

func (session workflowAgentTestSession) State() adksession.State { return session.state }

func (session workflowAgentTestSession) Events() adksession.Events { return session.events }

func (session workflowAgentTestSession) LastUpdateTime() time.Time { return session.updatedAt }

func TestGoogleADKWorkflowRootAdapterRuntimeBranches(t *testing.T) {
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
				id: "fresh-run-session", appName: "app", userID: "user", updatedAt: time.Now().UTC(),
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
			if err := emit(adkworkflow.NewRequestInputEvent(ctx, adksession.RequestInput{InterruptID: "ask-resume", Message: "resume?"})); err != nil {
				return nil, err
			}
			return nil, adkworkflow.ErrNodeInterrupted
		}, adkworkflow.NodeConfig{RerunOnResume: &googleADKWorkflowRerunOnResume})
		workflowAdapter := &googleADKWorkflowAgent{workflow: mustGoogleADKWorkflow(t, []adkworkflow.Edge{{From: adkworkflow.Start, To: asker}})}
		root, err := adkagent.New(adkagent.Config{Name: "resume_root", Run: workflowAdapter.run})
		if err != nil {
			t.Fatalf("agent.New resume root: %v", err)
		}
		service := adksession.InMemoryService()
		if _, err := service.Create(context.Background(), &adksession.CreateRequest{AppName: "app", UserID: "user", SessionID: "resume-session"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		runner, err := adkrunner.New(adkrunner.Config{AppName: "app", Agent: root, SessionService: service})
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
		resp, err := service.Get(context.Background(), &adksession.GetRequest{AppName: "app", UserID: "user", SessionID: "resume-session"})
		if err != nil {
			t.Fatalf("Get session: %v", err)
		}
		testCtx := &googleADKWorkflowAgentTestContext{
			StrictContextMock: adkagent.NewStrictContextMock(context.Background()),
			session:           resp.Session,
			invocationID:      invocationID,
			userContent: genai.NewContentFromParts([]*genai.Part{{FunctionResponse: &genai.FunctionResponse{
				ID: "ask-resume", Name: adkworkflow.WorkflowInputFunctionCallName, Response: map[string]any{"response": "approved"},
			}}}, genai.RoleUser),
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
