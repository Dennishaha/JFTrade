package adk

import (
	"context"
	"fmt"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func errADKMissingFinalReply() error {
	return fmt.Errorf("工具调用完成后模型未返回最终回复")
}

func (r *Runtime) WorkflowChildAgentForStep(ctx context.Context, agent Agent, step workflowStep) (Agent, error) {
	child := agent
	if agentID := strings.TrimSpace(step.ChildAgentID); agentID != "" && agentID != agent.ID {
		resolved, err := r.resolveAgentDefinition(ctx, agentID)
		if err != nil {
			return Agent{}, err
		}
		child = resolved
	}
	child = jfadkmodel.WorkflowChildAgentForStep(child, step)
	child.ReasoningEffort = jfadkmodel.NormalizeReasoningEffort(child.ReasoningEffort)
	child.WorkMode = WorkModeChat
	if strings.TrimSpace(child.PermissionMode) == "" {
		child.PermissionMode = agent.PermissionMode
	}
	return r.prepareAgent(ctx, child)
}
