package adk

import (
	"context"
	"errors"
	"iter"
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

func TestGoogleADKWorkflowResumeResponsesIgnoreAlreadyConsumedInterrupt(t *testing.T) {
	state := adkworkflow.NewRunState()
	state.Nodes["write_step"] = &adkworkflow.NodeState{
		Status:        adkworkflow.NodePending,
		Interrupts:    []string{"next-approval"},
		ResumedInputs: map[string]any{"prior-approval": map[string]any{"confirmed": true}},
	}
	content := genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID: "prior-approval", Name: toolconfirmation.FunctionCallName,
			Response: map[string]any{"confirmed": true},
		},
	}}, genai.RoleUser)

	if responses := googleADKWorkflowResumeResponses(content, state, nil); responses != nil {
		t.Fatalf("responses = %#v, want nil for an already consumed interrupt", responses)
	}
}

func TestGoogleADKWorkflowResumeBoundaryHelpersFailClosed(t *testing.T) {
	if got := googleADKWorkflowSessionBeforeCurrentResponse(nil, nil); got != nil {
		t.Fatalf("nil session boundary = %#v, want nil", got)
	}
	sess := workflowAgentTestSession{id: "boundary"}
	if got := googleADKWorkflowSessionBeforeCurrentResponse(sess, genai.NewContentFromText("plain", genai.RoleUser)); got.ID() != sess.ID() {
		t.Fatal("content without a response ID should preserve the session")
	}
	if googleADKWorkflowEventAnswers(&adksession.Event{Author: "user"}, map[string]struct{}{"answer": {}}) {
		t.Fatal("event without a matching function response was treated as an answer")
	}
	state := adkworkflow.NewRunState()
	state.Nodes["missing"] = nil
	if got := googleADKWorkflowPendingInterruptIDs(state); len(got) != 0 {
		t.Fatalf("pending interrupt IDs = %#v, want none", got)
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

func TestNewGoogleADKWorkflowAgentUsesNativeWorkflowAgentWithoutConcurrencyCap(t *testing.T) {
	asker := adkworkflow.NewEmittingFunctionNode("asker", func(ctx adkagent.Context, _ any, emit func(*adksession.Event) error) (any, error) {
		if reply, ok := ctx.ResumedInput("ask-native"); ok {
			return reply, nil
		}
		if err := emit(adkworkflow.NewRequestInputEvent(ctx, adksession.RequestInput{
			InterruptID: "ask-native",
			Message:     "approve?",
		})); err != nil {
			return nil, err
		}
		return nil, adkworkflow.ErrNodeInterrupted
	}, adkworkflow.NodeConfig{RerunOnResume: &googleADKWorkflowRerunOnResume})
	handler := adkworkflow.NewFunctionNode("handler", func(_ adkagent.Context, input any) (any, error) {
		return map[string]any{"handled": input}, nil
	}, adkworkflow.NodeConfig{})
	root, err := newGoogleADKWorkflowAgent(googleADKWorkflowAgentConfig{
		Name:        "native_workflow",
		Description: "native workflow",
		Edges: []adkworkflow.Edge{
			{From: adkworkflow.Start, To: asker},
			{From: asker, To: handler},
		},
	})
	if err != nil {
		t.Fatalf("newGoogleADKWorkflowAgent: %v", err)
	}
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
	var requestID string
	for event, err := range runner.Run(ctx, "user", "session", genai.NewContentFromText("start", genai.RoleUser), adkagent.RunConfig{}) {
		if err != nil {
			t.Fatalf("fresh workflow run: %v", err)
		}
		if event == nil {
			continue
		}
		if event.RequestedInput != nil {
			requestID = event.RequestedInput.InterruptID
		}
	}
	if requestID != "ask-native" {
		t.Fatalf("requestID = %q, want ask-native", requestID)
	}
	var sawHandled bool
	for event, err := range runner.Run(ctx, "user", "session", genai.NewContentFromParts([]*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{
			ID:       "ask-native",
			Name:     adkworkflow.WorkflowInputFunctionCallName,
			Response: map[string]any{"response": "approved"},
		},
	}}, genai.RoleUser), adkagent.RunConfig{}) {
		if err != nil {
			t.Fatalf("resume workflow run: %v", err)
		}
		if event != nil && event.Output != nil {
			if output, ok := event.Output.(map[string]any); ok && output["handled"] == "approved" {
				sawHandled = true
			}
		}
	}
	if !sawHandled {
		t.Fatal("native workflowagent resume did not deliver output to handler")
	}
}

func TestGoogleADKWorkflowChildNodeUsesNativeAgentNodeConfiguration(t *testing.T) {
	child, err := adkagent.New(adkagent.Config{
		Name: "native_child",
		Run: func(adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
			return func(func(*adksession.Event, error) bool) {}
		},
	})
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	node, err := adkworkflow.NewAgentNode(child, adkworkflow.NodeConfig{
		RerunOnResume: &googleADKWorkflowRerunOnResume,
	})
	if err != nil {
		t.Fatalf("workflow.NewAgentNode: %v", err)
	}
	if !node.Config().EmitsOwnSpan || node.Config().RerunOnResume == nil || !*node.Config().RerunOnResume {
		t.Fatalf("native agent node config = %+v, want own span and rerun on resume", node.Config())
	}
}

func TestGoogleADKWorkflowNativeAgentNodeResumesToolConfirmation(t *testing.T) {
	for _, test := range []struct {
		name             string
		confirmed        bool
		finalText        string
		expectedToolRuns int
	}{
		{name: "approved", confirmed: true, finalText: "approved complete", expectedToolRuns: 1},
		{name: "rejected", confirmed: false, finalText: "rejected complete", expectedToolRuns: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			testGoogleADKWorkflowNativeAgentNodeToolConfirmation(t, test.confirmed, test.finalText, test.expectedToolRuns)
		})
	}
}

func testGoogleADKWorkflowNativeAgentNodeToolConfirmation(t *testing.T, confirmed bool, finalText string, expectedToolRuns int) {
	t.Helper()
	toolRuns := 0
	secureTool, err := functiontool.New(functiontool.Config{
		Name:                "secure_action",
		Description:         "performs an action after approval",
		RequireConfirmation: true,
	}, func(_ adkagent.Context, _ struct{}) (map[string]any, error) {
		toolRuns++
		return map[string]any{"approved": true}, nil
	})
	if err != nil {
		t.Fatalf("functiontool.New: %v", err)
	}
	model := &nativeConfirmationModel{finalText: finalText}
	child, err := llmagent.New(llmagent.Config{
		Name:        "native_confirmation_child",
		Description: "runs an approved action",
		Model:       model,
		Tools:       []adktool.Tool{secureTool},
	})
	if err != nil {
		t.Fatalf("llmagent.New: %v", err)
	}
	childNode, err := adkworkflow.NewAgentNode(child, adkworkflow.NodeConfig{
		RerunOnResume: &googleADKWorkflowRerunOnResume,
	})
	if err != nil {
		t.Fatalf("workflow.NewAgentNode: %v", err)
	}
	handlerRuns := 0
	handler := adkworkflow.NewFunctionNode("confirmation_handler", func(_ adkagent.Context, input any) (any, error) {
		handlerRuns++
		return map[string]any{"handled": input}, nil
	}, adkworkflow.NodeConfig{})
	edges := []adkworkflow.Edge{
		{From: adkworkflow.Start, To: childNode},
		{From: childNode, To: handler},
	}
	workflow, err := adkworkflow.New("native_confirmation_workflow", edges, adkworkflow.WithMaxConcurrency(1))
	if err != nil {
		t.Fatalf("workflow.New: %v", err)
	}
	adapter := &googleADKWorkflowAgent{workflow: workflow}
	root, err := adkagent.New(adkagent.Config{
		Name: "native_confirmation_workflow", Description: "native confirmation workflow", Run: adapter.run,
	})
	if err != nil {
		t.Fatalf("agent.New root: %v", err)
	}
	ctx := context.Background()
	service := adksession.InMemoryService()
	if _, err := service.Create(ctx, &adksession.CreateRequest{
		AppName: "app", UserID: "user", SessionID: "confirmation-session",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	runner, err := adkrunner.New(adkrunner.Config{
		AppName: "app", Agent: root, SessionService: service,
	})
	if err != nil {
		t.Fatalf("runner.New: %v", err)
	}

	var pendingID string
	for event, runErr := range runner.Run(ctx, "user", "confirmation-session", genai.NewContentFromText("start", genai.RoleUser), adkagent.RunConfig{}) {
		if runErr != nil {
			t.Fatalf("fresh workflow run: %v", runErr)
		}
		if event == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			if part != nil && part.FunctionCall != nil && part.FunctionCall.Name == toolconfirmation.FunctionCallName {
				pendingID = part.FunctionCall.ID
			}
		}
	}
	if pendingID == "" || model.calls != 1 || toolRuns != 0 || handlerRuns != 0 {
		t.Fatalf("fresh workflow pendingID=%q modelCalls=%d toolRuns=%d handlerRuns=%d", pendingID, model.calls, toolRuns, handlerRuns)
	}
	stored, err := service.Get(ctx, &adksession.GetRequest{
		AppName: "app", UserID: "user", SessionID: "confirmation-session",
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	state, err := workflow.ReconstructRunState(stored.Session, "")
	if err != nil {
		t.Fatalf("ReconstructRunState: %v", err)
	}
	nodeState := state.Nodes[childNode.Name()]
	if nodeState == nil || nodeState.Status != adkworkflow.NodeWaiting || len(nodeState.Interrupts) != 1 || nodeState.Interrupts[0] != pendingID {
		t.Fatalf("native child state = %+v, want waiting %q", nodeState, pendingID)
	}

	response := genai.NewContentFromParts([]*genai.Part{{FunctionResponse: &genai.FunctionResponse{
		ID: pendingID, Name: toolconfirmation.FunctionCallName,
		Response: map[string]any{"confirmed": confirmed},
	}}}, genai.RoleUser)
	var sawHandled bool
	for event, runErr := range runner.Run(ctx, "user", "confirmation-session", response, adkagent.RunConfig{}) {
		if runErr != nil {
			t.Fatalf("resume workflow run: %v", runErr)
		}
		if event == nil {
			continue
		}
		output, ok := event.Output.(map[string]any)
		if ok && output["handled"] == finalText {
			sawHandled = true
		}
	}
	if !sawHandled || model.calls != 2 || toolRuns != expectedToolRuns || handlerRuns != 1 {
		t.Fatalf("resumed workflow handled=%v modelCalls=%d toolRuns=%d handlerRuns=%d", sawHandled, model.calls, toolRuns, handlerRuns)
	}
	var duplicateErr error
	for _, runErr := range runner.Run(ctx, "user", "confirmation-session", response, adkagent.RunConfig{}) {
		if runErr != nil {
			duplicateErr = runErr
			break
		}
	}
	if !errors.Is(duplicateErr, adkworkflow.ErrNothingToResume) {
		t.Fatalf("duplicate confirmation error = %v, want ErrNothingToResume", duplicateErr)
	}
	if model.calls != 2 || toolRuns != expectedToolRuns || handlerRuns != 1 {
		t.Fatalf("duplicate confirmation reran work: modelCalls=%d toolRuns=%d handlerRuns=%d", model.calls, toolRuns, handlerRuns)
	}
}

type nativeConfirmationModel struct {
	calls     int
	finalText string
}

func (m *nativeConfirmationModel) Name() string { return "native-confirmation-model" }

func (m *nativeConfirmationModel) GenerateContent(_ context.Context, _ *adkmodel.LLMRequest, _ bool) iter.Seq2[*adkmodel.LLMResponse, error] {
	return func(yield func(*adkmodel.LLMResponse, error) bool) {
		m.calls++
		if m.calls == 1 {
			yield(&adkmodel.LLMResponse{Content: genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
				ID: "secure-call", Name: "secure_action", Args: map[string]any{},
			}}}, genai.RoleModel)}, nil)
			return
		}
		yield(&adkmodel.LLMResponse{Content: genai.NewContentFromText(m.finalText, genai.RoleModel)}, nil)
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

type googleADKWorkflowAgentTestContext struct {
	adkagent.StrictContextMock
	agent        adkagent.Agent
	session      adksession.Session
	invocationID string
	userContent  *genai.Content
	ended        bool
	path         string
	runID        string
}

func (c *googleADKWorkflowAgentTestContext) Agent() adkagent.Agent {
	return c.agent
}

func (c *googleADKWorkflowAgentTestContext) Artifacts() adkagent.Artifacts {
	return nil
}

func (c *googleADKWorkflowAgentTestContext) Memory() adkagent.Memory {
	return nil
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

func (c *googleADKWorkflowAgentTestContext) EndInvocation() {
	c.ended = true
}

func (c *googleADKWorkflowAgentTestContext) Ended() bool {
	return c.ended
}

func (c *googleADKWorkflowAgentTestContext) Path() string {
	return c.path
}

func (c *googleADKWorkflowAgentTestContext) RunID() string {
	return c.runID
}

func (c *googleADKWorkflowAgentTestContext) OutputForAncestors() []string {
	return nil
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
		clone = *clone.WithICDelta(d.InvocationContextDelta).(*googleADKWorkflowAgentTestContext)
	}
	if d.Path != nil {
		clone.path = *d.Path
	}
	if d.RunID != nil {
		clone.runID = *d.RunID
	}
	return &clone
}

func (c *googleADKWorkflowAgentTestContext) RunNode(any, any, any) (any, error) {
	return nil, nil
}

func (c *googleADKWorkflowAgentTestContext) Events() iter.Seq[*adksession.Event] {
	return nil
}
