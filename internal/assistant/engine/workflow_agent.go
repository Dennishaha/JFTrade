package adk

import (
	"encoding/json"
	"errors"
	"iter"

	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/workflowagent"
	adksession "google.golang.org/adk/v2/session"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

var googleADKWorkflowRerunOnResume = true

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
			resumeSession := googleADKWorkflowSessionBeforeCurrentResponse(ctx.Session(), ctx.UserContent())
			state, err := a.workflow.ReconstructRunState(resumeSession, ctx.InvocationID())
			if err != nil {
				yield(nil, err)
				return
			}
			responses := googleADKWorkflowResumeResponses(ctx.UserContent(), state, nil)
			if len(responses) > 0 && state != nil {
				matched, keepGoing := a.yieldResume(ctx, state, responses, yield)
				if !keepGoing || matched {
					return
				}
			}
			fallbackState, fallbackErr := a.workflow.ReconstructRunState(resumeSession, "")
			if fallbackErr != nil {
				yield(nil, fallbackErr)
				return
			}
			fallbackResponses := googleADKWorkflowResumeResponses(ctx.UserContent(), fallbackState, resumeSession)
			if len(fallbackResponses) > 0 && fallbackState != nil {
				matched, keepGoing := a.yieldResume(ctx, fallbackState, fallbackResponses, yield)
				if !keepGoing || matched {
					return
				}
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

func googleADKWorkflowSessionBeforeCurrentResponse(sess adksession.Session, content *genai.Content) adksession.Session {
	if sess == nil || content == nil {
		return sess
	}
	responseIDs := make(map[string]struct{})
	for _, response := range googleADKWorkflowFunctionResponses(content) {
		if response != nil && response.ID != "" {
			responseIDs[response.ID] = struct{}{}
		}
	}
	if len(responseIDs) == 0 {
		return sess
	}
	events := sess.Events()
	lastIndex := events.Len() - 1
	for lastIndex >= 0 && events.At(lastIndex) == nil {
		lastIndex--
	}
	if lastIndex < 0 || !googleADKWorkflowEventAnswers(events.At(lastIndex), responseIDs) {
		return sess
	}
	items := make([]*adksession.Event, 0, events.Len()-1)
	for index := 0; index < events.Len(); index++ {
		if index != lastIndex {
			items = append(items, events.At(index))
		}
	}
	return &wrappedSession{base: sess, events: &wrappedEvents{items: items}}
}

func googleADKWorkflowEventAnswers(event *adksession.Event, responseIDs map[string]struct{}) bool {
	if event == nil || event.Author != "user" {
		return false
	}
	for _, response := range googleADKWorkflowFunctionResponses(event.Content) {
		if response != nil {
			if _, ok := responseIDs[response.ID]; ok {
				return true
			}
		}
	}
	return false
}

func (a *googleADKWorkflowAgent) yieldResume(
	ctx adkagent.InvocationContext,
	state *adkworkflow.RunState,
	responses map[string]any,
	yield func(*adksession.Event, error) bool,
) (matched bool, keepGoing bool) {
	for event, err := range a.workflow.Resume(adkagent.Promote(ctx), state, responses) {
		if errors.Is(err, adkworkflow.ErrNothingToResume) {
			return false, true
		}
		if !yield(event, err) {
			return true, false
		}
	}
	return true, true
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
	for id := range googleADKWorkflowPendingInterruptIDs(state) {
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

func googleADKWorkflowPendingInterruptIDs(state *adkworkflow.RunState) map[string]struct{} {
	ids := make(map[string]struct{})
	if state == nil {
		return ids
	}
	for _, nodeState := range state.Nodes {
		if nodeState == nil {
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
