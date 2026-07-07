package adk

import (
	"errors"
	"fmt"

	adkagent "google.golang.org/adk/v2/agent"
	adkmodel "google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/plugin"
	adksession "google.golang.org/adk/v2/session"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/genai"
)

func (e *googleADKExecution) plugin() (*plugin.Plugin, error) {
	if e == nil {
		return nil, fmt.Errorf("GO-ADK execution is unavailable")
	}
	return plugin.New(plugin.Config{
		Name:                 "jftrade_execution_projection",
		BeforeRunCallback:    e.beforeRunCallback,
		AfterRunCallback:     e.afterRunCallback,
		OnEventCallback:      e.onEventCallback,
		AfterModelCallback:   e.afterModelCallback,
		OnModelErrorCallback: e.onModelErrorCallback,
		BeforeToolCallback:   e.beforeToolCallback,
		AfterToolCallback:    e.afterToolCallback,
		OnToolErrorCallback:  e.onToolErrorCallback,
	})
}

func (e *googleADKExecution) beforeRunCallback(_ adkagent.InvocationContext) (*genai.Content, error) {
	return nil, nil
}

func (e *googleADKExecution) afterRunCallback(_ adkagent.InvocationContext) {
	_ = e
}

func (e *googleADKExecution) onEventCallback(_ adkagent.InvocationContext, event *adksession.Event) (*adksession.Event, error) {
	e.observeWorkflowEvent(event)
	return event, nil
}

func (e *googleADKExecution) afterModelCallback(
	_ adkagent.Context,
	_ *adkmodel.LLMResponse,
	_ error,
) (*adkmodel.LLMResponse, error) {
	return nil, nil
}

func (e *googleADKExecution) onModelErrorCallback(
	_ adkagent.Context,
	_ *adkmodel.LLMRequest,
	_ error,
) (*adkmodel.LLMResponse, error) {
	return nil, nil
}

func (e *googleADKExecution) onToolErrorCallback(
	_ adkagent.Context,
	_ adktool.Tool,
	_ map[string]any,
	_ error,
) (map[string]any, error) {
	return nil, nil
}

func (e *googleADKExecution) beforeToolCallback(ctx adkagent.Context, tool adktool.Tool, args map[string]any) (map[string]any, error) {
	if e.shouldInterruptForUserGoalPause(e.runIDForAgentName(ctx.AgentName())) {
		return nil, errUserGoalPauseRequested
	}
	descriptor, ok := e.descriptorForTool(tool)
	if !ok {
		return nil, nil
	}
	call := e.ensureCallForAgent(ctx.FunctionCallID(), descriptor, args, ctx.AgentName())
	e.emitToolProgress(call.ID, tool.Name())
	if !ToolAllowedInMode(descriptor, e.agent.PermissionMode) {
		return nil, fmt.Errorf("tool is not allowed in permission mode %s", e.agent.PermissionMode)
	}
	return nil, nil
}

func (e *googleADKExecution) afterToolCallback(
	ctx adkagent.Context,
	tool adktool.Tool,
	args map[string]any,
	result map[string]any,
	err error,
) (map[string]any, error) {
	descriptor, ok := e.descriptorForTool(tool)
	if !ok {
		return nil, nil
	}
	call := e.ensureCallForAgent(ctx.FunctionCallID(), descriptor, args, ctx.AgentName())
	switch {
	case err == nil:
		if structuredErr, ok := structuredToolError(result); ok {
			e.finishCall(call.ID, nil, errors.New(structuredErr))
			// Return the result with the error so the ADK includes it in the
			// function response content. This lets the LLM see the failure and
			// decide whether to retry, use a different tool or report to the user.
			return result, nil
		}
		e.finishCall(call.ID, result, nil)
		return result, nil
	case errors.Is(err, adktool.ErrConfirmationRequired):
		// ADK will emit a tool confirmation function response that transitions the
		// tracked call into PENDING_APPROVAL; keep the call open until then.
	case errors.Is(err, adktool.ErrConfirmationRejected):
		e.finishCall(call.ID, nil, err)
	default:
		e.finishCall(call.ID, result, err)
	}
	return nil, nil
}
