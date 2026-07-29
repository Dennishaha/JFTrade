package adk

import (
	"encoding/json"
	"iter"
	"strings"

	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/workflowagent"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/toolconfirmation"
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
			if len(responses) == 0 {
				fallbackState, fallbackErr := a.workflow.ReconstructRunState(ctx.Session(), "")
				if fallbackErr != nil {
					yield(nil, fallbackErr)
					return
				}
				if fallbackState != nil {
					if fallbackResponses := googleADKWorkflowResumeResponses(ctx.UserContent(), fallbackState, ctx.Session()); len(fallbackResponses) > 0 {
						state = fallbackState
						responses = fallbackResponses
					}
				}
			}
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
	var observedReply strings.Builder
	sawPartialText := false
	for event, err := range runner.RunNode(ctx, nodeInput) {
		if err != nil {
			return nil, err
		}
		if event == nil {
			continue
		}
		googleADKWorkflowObserveVisibleReply(&observedReply, &sawPartialText, event)
		if ids := googleADKWorkflowInterruptIDs(event); len(ids) > 0 {
			for _, id := range ids {
				event.LongRunningToolIDs = appendUniqueString(event.LongRunningToolIDs, id)
			}
			paused = true
		}
		if emitErr := emit(event); emitErr != nil {
			return nil, emitErr
		}
	}
	if !paused {
		if interrupted, err := googleADKWorkflowMaybeInterruptForImplicitInput(ctx, observedReply.String(), emit); err != nil {
			return nil, err
		} else if interrupted {
			return nil, adkworkflow.ErrNodeInterrupted
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
	var observedReply strings.Builder
	sawPartialText := false
	for event, err := range a.Run(agentCtx) {
		if err != nil {
			return nil, err
		}
		if event == nil {
			continue
		}
		googleADKWorkflowObserveVisibleReply(&observedReply, &sawPartialText, event)
		if ids := googleADKWorkflowInterruptIDs(event); len(ids) > 0 {
			for _, id := range ids {
				event.LongRunningToolIDs = appendUniqueString(event.LongRunningToolIDs, id)
			}
			paused = true
		}
		if emitErr := emit(event); emitErr != nil {
			return nil, emitErr
		}
	}
	if !paused {
		if interrupted, err := googleADKWorkflowMaybeInterruptForImplicitInput(ctx, observedReply.String(), emit); err != nil {
			return nil, err
		} else if interrupted {
			return nil, adkworkflow.ErrNodeInterrupted
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

func googleADKWorkflowInterruptIDs(event *adksession.Event) []string {
	if event == nil {
		return nil
	}
	ids := normalizeStringSlice(event.LongRunningToolIDs)
	if event.Content == nil {
		return ids
	}
	for _, part := range event.Content.Parts {
		if part == nil || part.FunctionCall == nil {
			continue
		}
		if part.FunctionCall.Name == toolconfirmation.FunctionCallName ||
			part.FunctionCall.Name == adkworkflow.WorkflowInputFunctionCallName {
			ids = appendUniqueString(ids, part.FunctionCall.ID)
		}
	}
	return normalizeStringSlice(ids)
}

func googleADKWorkflowObserveVisibleReply(builder *strings.Builder, sawPartialText *bool, event *adksession.Event) {
	if builder == nil || sawPartialText == nil {
		return
	}
	if event == nil || event.Content == nil {
		if event != nil && !event.Partial {
			*sawPartialText = false
		}
		return
	}
	emitText := true
	if event.Partial {
		*sawPartialText = *sawPartialText || contentHasText(event.Content)
	} else if *sawPartialText {
		emitText = false
	}
	if emitText {
		reply, _ := visibleTextFromParts(event.Content.Parts)
		builder.WriteString(reply)
	}
	if !event.Partial {
		*sawPartialText = false
	}
}

func googleADKWorkflowMaybeInterruptForImplicitInput(
	ctx adkagent.Context,
	reply string,
	emit func(*adksession.Event) error,
) (bool, error) {
	_ = ctx
	_ = reply
	_ = emit
	return false, nil
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
