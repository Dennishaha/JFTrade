package adk

import (
	"encoding/json"
	"iter"

	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/workflowagent"
	adksession "google.golang.org/adk/v2/session"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

var googleADKWorkflowRerunOnResume = true

type googleADKNodeRunner interface {
	RunNode(adkagent.Context, any) iter.Seq2[*adksession.Event, error]
}

type googleADKWorkflowAgentConfig struct {
	Name           string
	Description    string
	Edges          []adkworkflow.Edge
	MaxConcurrency int
}

type googleADKWorkflowAgent struct {
	workflow *adkworkflow.Workflow
}

func newGoogleADKWorkflowAgent(cfg googleADKWorkflowAgentConfig) (adkagent.Agent, error) {
	if cfg.MaxConcurrency <= 0 {
		return workflowagent.New(workflowagent.Config{
			Name:        cfg.Name,
			Description: cfg.Description,
			Edges:       cfg.Edges,
		})
	}
	options := []adkworkflow.Option{}
	if cfg.MaxConcurrency > 0 {
		options = append(options, adkworkflow.WithMaxConcurrency(cfg.MaxConcurrency))
	}
	wf, err := adkworkflow.New(cfg.Name, cfg.Edges, options...)
	if err != nil {
		return nil, err
	}
	adapter := &googleADKWorkflowAgent{workflow: wf}
	return adkagent.New(adkagent.Config{
		Name:        cfg.Name,
		Description: cfg.Description,
		Run:         adapter.run,
	})
}

func (a *googleADKWorkflowAgent) run(ctx adkagent.InvocationContext) iter.Seq2[*adksession.Event, error] {
	return func(yield func(*adksession.Event, error) bool) {
		containsResponse := googleADKWorkflowHasFunctionResponse(ctx.UserContent())
		if containsResponse {
			state, err := a.workflow.ReconstructRunState(ctx.Session(), ctx.InvocationID())
			if err != nil {
				yield(nil, err)
				return
			}
			responses := googleADKWorkflowResumeResponses(ctx.UserContent(), state, ctx.Session())
			if len(responses) > 0 && state != nil {
				for event, err := range a.workflow.Resume(adkagent.Promote(ctx), state, responses) {
					if !yield(event, err) {
						return
					}
				}
				return
			}
			yield(nil, adkworkflow.ErrNothingToResume)
			return
		}
		for event, err := range a.workflow.Run(ctx) {
			if !yield(event, err) {
				return
			}
		}
	}
}

func newGoogleADKWorkflowAgentNode(a adkagent.Agent) (adkworkflow.Node, error) {
	if a == nil {
		return nil, errNilGoogleADKWorkflowAgentNode()
	}
	return adkworkflow.NewDynamicNode(a.Name(), googleADKWorkflowAgentNodeBody(a), adkworkflow.NodeConfig{
		EmitsOwnSpan:  true,
		RerunOnResume: &googleADKWorkflowRerunOnResume,
	}), nil
}

func googleADKWorkflowAgentNodeBody(a adkagent.Agent) adkworkflow.DynamicFn[any, any] {
	return func(ctx adkagent.Context, input any, emit func(*adksession.Event) error) (any, error) {
		isResume := len(googleADKWorkflowAnsweredOpenInterrupts(ctx.Session())) > 0
		if runner, ok := a.(googleADKNodeRunner); ok {
			return googleADKWorkflowRunNode(runner, ctx, input, isResume, emit)
		}
		return googleADKWorkflowRunGenericAgent(a, ctx, input, isResume, emit)
	}
}

func errNilGoogleADKWorkflowAgentNode() error {
	return &googleADKWorkflowNodeError{message: "GO-ADK workflow node: agent cannot be nil"}
}

type googleADKWorkflowNodeError struct {
	message string
}

func (e *googleADKWorkflowNodeError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func googleADKWorkflowRunNode(
	runner googleADKNodeRunner,
	ctx adkagent.Context,
	input any,
	isResume bool,
	emit func(*adksession.Event) error,
) (any, error) {
	var nodeInput any
	if !isResume {
		nodeInput = input
	}
	paused := false
	for event, err := range runner.RunNode(ctx, nodeInput) {
		if err != nil {
			return nil, err
		}
		if event == nil {
			continue
		}
		if len(event.LongRunningToolIDs) > 0 {
			paused = true
		}
		if emitErr := emit(event); emitErr != nil {
			return nil, emitErr
		}
	}
	if paused {
		return nil, adkworkflow.ErrNodeInterrupted
	}
	return nil, nil
}

func googleADKWorkflowRunGenericAgent(
	a adkagent.Agent,
	ctx adkagent.Context,
	input any,
	isResume bool,
	emit func(*adksession.Event) error,
) (any, error) {
	var userContent *genai.Content
	if !isResume {
		userContent = googleADKWorkflowInputToUserContent(input)
	}
	agentCtx := googleADKWorkflowAgentContext(ctx, a, userContent)
	paused := false
	for event, err := range a.Run(agentCtx) {
		if err != nil {
			return nil, err
		}
		if event == nil {
			continue
		}
		if len(event.LongRunningToolIDs) > 0 {
			paused = true
		}
		if emitErr := emit(event); emitErr != nil {
			return nil, emitErr
		}
	}
	if paused {
		return nil, adkworkflow.ErrNodeInterrupted
	}
	return nil, nil
}

func googleADKWorkflowAgentContext(ctx adkagent.Context, a adkagent.Agent, userContent *genai.Content) adkagent.InvocationContext {
	path := ""
	runID := ""
	outputForAncestors := []string{}
	delta := &adkagent.CommonContextDelta{
		InvocationContextDelta: &adkagent.InvocationContextDelta{
			Agent:       &a,
			UserContent: &userContent,
		},
		RunID:              &runID,
		Path:               &path,
		OutputForAncestors: &outputForAncestors,
	}
	return ctx.WithDelta(delta)
}

func googleADKWorkflowInputToUserContent(input any) *genai.Content {
	switch value := input.(type) {
	case nil:
		return nil
	case *genai.Content:
		return value
	case string:
		if value == "" {
			return nil
		}
		return genai.NewContentFromText(value, genai.RoleUser)
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		return genai.NewContentFromText(string(raw), genai.RoleUser)
	}
}

func googleADKWorkflowHasFunctionResponse(content *genai.Content) bool {
	for _, response := range googleADKWorkflowFunctionResponses(content) {
		if response != nil {
			return true
		}
	}
	return false
}

func googleADKWorkflowResumeResponses(content *genai.Content, state *adkworkflow.RunState, sess adksession.Session) map[string]any {
	pending := make(map[string]struct{})
	for id := range googleADKWorkflowWaitingInterruptIDs(state) {
		pending[id] = struct{}{}
	}
	for id := range googleADKWorkflowOpenLongRunningCallIDs(sess) {
		pending[id] = struct{}{}
	}
	if len(pending) == 0 {
		return nil
	}
	responses := make(map[string]any)
	for _, response := range googleADKWorkflowFunctionResponses(content) {
		if response == nil || response.ID == "" {
			continue
		}
		if _, ok := pending[response.ID]; !ok {
			continue
		}
		responses[response.ID] = googleADKDecodeWorkflowInputResponse(response)
	}
	if len(responses) == 0 {
		return nil
	}
	return responses
}

func googleADKWorkflowInputResponses(content *genai.Content) map[string]any {
	if content == nil {
		return nil
	}
	responses := make(map[string]any)
	for _, response := range googleADKWorkflowFunctionResponses(content) {
		if response.Name != adkworkflow.WorkflowInputFunctionCallName || response.ID == "" {
			continue
		}
		responses[response.ID] = googleADKDecodeWorkflowInputResponse(response)
	}
	if len(responses) == 0 {
		return nil
	}
	return responses
}

func googleADKWorkflowFunctionResponses(content *genai.Content) []*genai.FunctionResponse {
	if content == nil {
		return nil
	}
	responses := make([]*genai.FunctionResponse, 0, len(content.Parts))
	for _, part := range content.Parts {
		if part == nil || part.FunctionResponse == nil {
			continue
		}
		responses = append(responses, part.FunctionResponse)
	}
	return responses
}

func googleADKWorkflowWaitingInterruptIDs(state *adkworkflow.RunState) map[string]struct{} {
	ids := make(map[string]struct{})
	if state == nil {
		return ids
	}
	for _, nodeState := range state.Nodes {
		if nodeState == nil || nodeState.Status != adkworkflow.NodeWaiting {
			continue
		}
		for _, id := range nodeState.Interrupts {
			if id != "" {
				ids[id] = struct{}{}
			}
		}
	}
	return ids
}

func googleADKWorkflowOpenLongRunningCallIDs(sess adksession.Session) map[string]struct{} {
	open := make(map[string]struct{})
	if sess == nil {
		return open
	}
	answered := make(map[string]struct{})
	events := sess.Events()
	for index := 0; index < events.Len(); index++ {
		event := events.At(index)
		if event == nil {
			continue
		}
		for _, id := range event.LongRunningToolIDs {
			if id != "" {
				open[id] = struct{}{}
			}
		}
		for _, response := range googleADKWorkflowFunctionResponses(event.Content) {
			if response != nil && response.ID != "" {
				answered[response.ID] = struct{}{}
			}
		}
	}
	for id := range answered {
		delete(open, id)
	}
	return open
}

func googleADKWorkflowAnsweredOpenInterrupts(sess adksession.Session) map[string]bool {
	answered := make(map[string]bool)
	if sess == nil {
		return answered
	}
	longRunning := make(map[string]struct{})
	events := sess.Events()
	for index := 0; index < events.Len(); index++ {
		event := events.At(index)
		if event == nil {
			continue
		}
		for _, id := range event.LongRunningToolIDs {
			if id != "" {
				longRunning[id] = struct{}{}
			}
		}
		for _, response := range googleADKWorkflowFunctionResponses(event.Content) {
			if response == nil || response.ID == "" {
				continue
			}
			if _, ok := longRunning[response.ID]; ok {
				answered[response.ID] = true
			}
		}
	}
	return answered
}

func googleADKDecodeWorkflowInputResponse(response *genai.FunctionResponse) any {
	if response == nil {
		return nil
	}
	if raw, ok := response.Response["response"]; ok {
		if text, isText := raw.(string); isText {
			var decoded any
			if err := json.Unmarshal([]byte(text), &decoded); err == nil {
				return decoded
			}
			return text
		}
		return raw
	}
	if payload, ok := response.Response["payload"]; ok {
		return payload
	}
	return response.Response
}
