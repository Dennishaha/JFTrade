package adk

import (
	"context"
	"errors"
	"iter"
	"testing"

	adkagent "google.golang.org/adk/v2/agent"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

func TestGoogleADKWorkflowInputResponsesDecodeADKRequestInput(t *testing.T) {
	content := genai.NewContentFromParts([]*genai.Part{
		{FunctionResponse: &genai.FunctionResponse{
			ID: "ask-json", Name: adkworkflow.WorkflowInputFunctionCallName,
			Response: map[string]any{"response": `{"approved":true}`},
		}},
		{FunctionResponse: &genai.FunctionResponse{
			ID: "ask-text", Name: adkworkflow.WorkflowInputFunctionCallName,
			Response: map[string]any{"response": "plain yes"},
		}},
		{FunctionResponse: &genai.FunctionResponse{
			ID: "ask-payload", Name: adkworkflow.WorkflowInputFunctionCallName,
			Response: map[string]any{"payload": map[string]any{"choice": "continue"}},
		}},
		{FunctionResponse: &genai.FunctionResponse{
			ID: "ignored", Name: "other_tool",
			Response: map[string]any{"response": "ignore"},
		}},
	}, genai.RoleUser)

	responses := googleADKWorkflowInputResponses(content)
	if len(responses) != 3 {
		t.Fatalf("responses = %#v, want 3 request-input responses", responses)
	}
	approved, ok := responses["ask-json"].(map[string]any)
	if !ok || approved["approved"] != true {
		t.Fatalf("json response = %#v", responses["ask-json"])
	}
	if responses["ask-text"] != "plain yes" {
		t.Fatalf("text response = %#v", responses["ask-text"])
	}
	payload, ok := responses["ask-payload"].(map[string]any)
	if !ok || payload["choice"] != "continue" {
		t.Fatalf("payload response = %#v", responses["ask-payload"])
	}
}

func TestGoogleADKWorkflowInputResponsesIgnoreMissingContent(t *testing.T) {
	if got := googleADKWorkflowInputResponses(nil); got != nil {
		t.Fatalf("nil content responses = %#v", got)
	}
	content := genai.NewContentFromText("hello", genai.RoleUser)
	if got := googleADKWorkflowInputResponses(content); got != nil {
		t.Fatalf("text content responses = %#v", got)
	}
}

func TestGoogleADKWorkflowResumeResponsesMatchToolConfirmationByInterruptID(t *testing.T) {
	state := adkworkflow.NewRunState()
	state.Nodes["write_step"] = &adkworkflow.NodeState{
		Status:     adkworkflow.NodeWaiting,
		Interrupts: []string{"approval-call"},
	}
	content := genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID:   "approval-call",
			Name: toolconfirmation.FunctionCallName,
			Response: map[string]any{
				"confirmed": true,
			},
		},
	}}, genai.RoleUser)

	responses := googleADKWorkflowResumeResponses(content, state, nil)
	if len(responses) != 1 {
		t.Fatalf("responses = %#v, want one matched tool confirmation", responses)
	}
	payload, ok := responses["approval-call"].(map[string]any)
	if !ok || payload["confirmed"] != true {
		t.Fatalf("tool confirmation payload = %#v", responses["approval-call"])
	}
}

func TestGoogleADKWorkflowResumeResponsesMatchOpenLongRunningCall(t *testing.T) {
	ctx := context.Background()
	service := adksession.InMemoryService()
	created, err := service.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "session",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	event := adksession.NewEvent(ctx, "invocation")
	event.Author = "agent"
	event.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionCall: &genai.FunctionCall{
			ID: "approval-call", Name: toolconfirmation.FunctionCallName,
			Args: map[string]any{"hint": "approve"},
		},
	}}, genai.RoleModel)
	event.LongRunningToolIDs = []string{"approval-call"}
	if err := service.AppendEvent(ctx, created.Session, event); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	content := genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID: "approval-call", Name: toolconfirmation.FunctionCallName,
			Response: map[string]any{"confirmed": true},
		},
	}}, genai.RoleUser)

	responses := googleADKWorkflowResumeResponses(content, nil, created.Session)
	if len(responses) != 1 {
		t.Fatalf("responses = %#v, want one open long-running response", responses)
	}
}

func TestGoogleADKWorkflowResumeResponsesIgnoreUnmatchedFunctionResponse(t *testing.T) {
	state := adkworkflow.NewRunState()
	state.Nodes["write_step"] = &adkworkflow.NodeState{
		Status:     adkworkflow.NodeWaiting,
		Interrupts: []string{"approval-call"},
	}
	content := genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID: "other-call", Name: toolconfirmation.FunctionCallName,
			Response: map[string]any{"confirmed": true},
		},
	}}, genai.RoleUser)

	if responses := googleADKWorkflowResumeResponses(content, state, nil); responses != nil {
		t.Fatalf("responses = %#v, want nil for unmatched function response", responses)
	}
}

func TestGoogleADKWorkflowAgentDoesNotFreshRunUnmatchedFunctionResponse(t *testing.T) {
	node := &workflowCompilerTestNode{BaseNode: adkworkflow.NewBaseNode("node", "", adkworkflow.NodeConfig{})}
	workflowAdapter := &googleADKWorkflowAgent{
		workflow: mustGoogleADKWorkflow(t, []adkworkflow.Edge{{From: adkworkflow.Start, To: node}}),
	}
	root, err := adkagent.New(adkagent.Config{Name: "root", Run: workflowAdapter.run})
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	sessResp, err := adksession.InMemoryService().Create(context.Background(), &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "session",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	ctx := &googleADKWorkflowAgentTestContext{
		StrictContextMock: adkagent.NewStrictContextMock(context.Background()),
		agent:             root,
		session:           sessResp.Session,
		invocationID:      "invocation",
		userContent: genai.NewContentFromParts([]*genai.Part{{
			FunctionResponse: &genai.FunctionResponse{
				ID: "unknown", Name: toolconfirmation.FunctionCallName,
				Response: map[string]any{"confirmed": true},
			},
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
		t.Fatalf("Run error = %v, want ErrNothingToResume", gotErr)
	}
}

func TestGoogleADKWorkflowRunNodeOmitsOriginalInputOnResume(t *testing.T) {
	ctx := context.Background()
	service := adksession.InMemoryService()
	created, err := service.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "session",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	request := adksession.NewEvent(ctx, "invocation")
	request.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionCall: &genai.FunctionCall{ID: "approval-call", Name: toolconfirmation.FunctionCallName},
	}}, genai.RoleModel)
	request.LongRunningToolIDs = []string{"approval-call"}
	if err := service.AppendEvent(ctx, created.Session, request); err != nil {
		t.Fatalf("Append request: %v", err)
	}
	response := adksession.NewEvent(ctx, "invocation")
	response.Content = genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID: "approval-call", Name: toolconfirmation.FunctionCallName,
			Response: map[string]any{"confirmed": true},
		},
	}}, genai.RoleUser)
	if err := service.AppendEvent(ctx, created.Session, response); err != nil {
		t.Fatalf("Append response: %v", err)
	}
	testCtx := &googleADKWorkflowAgentTestContext{
		StrictContextMock: adkagent.NewStrictContextMock(ctx),
		session:           created.Session,
		invocationID:      "invocation",
	}
	runner := &googleADKWorkflowNodeRunnerTestDouble{}

	output, err := googleADKWorkflowRunNode(runner, testCtx, "original task", false, func(*adksession.Event) error { return nil })
	if err != nil {
		t.Fatalf("fresh RunNode: %v", err)
	}
	if output != nil {
		t.Fatalf("fresh output = %#v, want nil because forwarded events carry node output", output)
	}
	if runner.inputs[len(runner.inputs)-1] != "original task" {
		t.Fatalf("fresh input = %#v, want original task", runner.inputs[len(runner.inputs)-1])
	}
	output, err = googleADKWorkflowRunNode(runner, testCtx, "original task", true, func(*adksession.Event) error { return nil })
	if err != nil {
		t.Fatalf("resume RunNode: %v", err)
	}
	if output != nil {
		t.Fatalf("resume output = %#v, want nil because forwarded events carry node output", output)
	}
	if runner.inputs[len(runner.inputs)-1] != nil {
		t.Fatalf("resume input = %#v, want nil", runner.inputs[len(runner.inputs)-1])
	}
}

func mustGoogleADKWorkflow(t *testing.T, edges []adkworkflow.Edge) *adkworkflow.Workflow {
	t.Helper()
	workflow, err := adkworkflow.New("root", edges)
	if err != nil {
		t.Fatalf("workflow.New: %v", err)
	}
	return workflow
}

type googleADKWorkflowNodeRunnerTestDouble struct {
	inputs []any
}

func (r *googleADKWorkflowNodeRunnerTestDouble) RunNode(_ adkagent.Context, input any) iter.Seq2[*adksession.Event, error] {
	r.inputs = append(r.inputs, input)
	return func(yield func(*adksession.Event, error) bool) {
		yield(&adksession.Event{Output: "forwarded-output"}, nil)
	}
}

type googleADKWorkflowAgentTestContext struct {
	adkagent.StrictContextMock
	agent        adkagent.Agent
	session      adksession.Session
	invocationID string
	userContent  *genai.Content
}

func (c *googleADKWorkflowAgentTestContext) Agent() adkagent.Agent {
	return c.agent
}

func (c *googleADKWorkflowAgentTestContext) Session() adksession.Session {
	return c.session
}

func (c *googleADKWorkflowAgentTestContext) InvocationID() string {
	return c.invocationID
}

func (c *googleADKWorkflowAgentTestContext) UserContent() *genai.Content {
	return c.userContent
}

func (c *googleADKWorkflowAgentTestContext) WithContext(ctx context.Context) adkagent.InvocationContext {
	clone := *c
	clone.Ctx = ctx
	return &clone
}

func (c *googleADKWorkflowAgentTestContext) WithICDelta(d *adkagent.InvocationContextDelta) adkagent.InvocationContext {
	clone := *c
	if d == nil {
		return &clone
	}
	if d.Context != nil {
		clone.Ctx = *d.Context
	}
	if d.Agent != nil {
		clone.agent = *d.Agent
	}
	if d.UserContent != nil {
		clone.userContent = *d.UserContent
	}
	return &clone
}

func (c *googleADKWorkflowAgentTestContext) WithDelta(d *adkagent.CommonContextDelta) adkagent.Context {
	clone := *c
	if d == nil {
		return &clone
	}
	if d.InvocationContextDelta != nil {
		return clone.WithICDelta(d.InvocationContextDelta).(adkagent.Context)
	}
	return &clone
}

func (c *googleADKWorkflowAgentTestContext) RunNode(any, any, any) (any, error) {
	return nil, nil
}

func (c *googleADKWorkflowAgentTestContext) Events() iter.Seq[*adksession.Event] {
	return nil
}
