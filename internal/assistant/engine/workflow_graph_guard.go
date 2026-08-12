package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	adksession "google.golang.org/adk/v2/session"
	"gorm.io/gorm"
)

const googleADKWorkflowGraphStatePrefix = "jftrade:workflow_graph:"

func (r *Runtime) googleADKWorkflowGraphFingerprint(ctx context.Context, root Agent, parent Run, fallback []workflowStep) (string, error) {
	root = jfadkmodel.WorkflowGraphRootAgent(root, parent)
	steps := googleADKWorkflowFingerprintSteps(parent.WorkflowPlan, fallback)
	children := make([]Agent, 0, len(steps))
	for _, step := range steps {
		child, err := r.WorkflowChildAgentForStep(ctx, root, step)
		if err != nil {
			return "", err
		}
		child.Instruction = workflowChildInstruction(child.Instruction, workflowChildInstructionTask(step))
		children = append(children, child)
	}
	return jfadkmodel.WorkflowGraphFingerprint(parent, root, steps, children)
}

func googleADKWorkflowFingerprintSteps(plan []WorkflowStepState, fallback []workflowStep) []workflowStep {
	if len(plan) == 0 {
		return append([]workflowStep(nil), fallback...)
	}
	steps := make([]workflowStep, 0, len(plan))
	for _, state := range plan {
		steps = append(steps, workflowStep{
			Order: state.Order, DependencyID: state.TaskID, Title: state.Title,
			Description: jfadkmodel.WorkflowDescriptionWithoutAgentRole(state.Description), Message: state.Message,
			DependsOn: append([]string(nil), state.DependsOn...), AgentRole: state.AgentRole,
			ChildAgentID: state.ChildAgentID, ChildProviderID: state.ChildProviderID, ChildModel: state.ChildModel,
			ChildPermissionMode: state.ChildPermissionMode, ModeHint: state.ModeHint, Objective: state.Objective,
			PlanSource: state.PlanSource, WorkflowMode: state.WorkflowMode,
		})
	}
	return steps
}

func (r *Runtime) persistGoogleADKWorkflowGraph(ctx context.Context, service adksession.Service, session adksession.Session, execution *googleADKExecution) error {
	if execution == nil || strings.TrimSpace(execution.workflowGraphFingerprint) == "" {
		return nil
	}
	key := googleADKWorkflowGraphStatePrefix + execution.runID
	stored, err := session.State().Get(key)
	if err == nil {
		if strings.TrimSpace(fmt.Sprint(stored)) != execution.workflowGraphFingerprint {
			return fmt.Errorf("GO-ADK workflow graph changed for run %s", execution.runID)
		}
		return nil
	}
	if !errors.Is(err, adksession.ErrStateKeyNotExist) {
		return fmt.Errorf("read GO-ADK workflow graph: %w", err)
	}
	event := adksession.NewEvent(ctx, execution.runID)
	event.Author = "jftrade"
	event.Actions.SkipSummarization = true
	event.Actions.StateDelta = map[string]any{key: execution.workflowGraphFingerprint}
	if err := service.AppendEvent(ctx, session, event); err != nil {
		return fmt.Errorf("persist GO-ADK workflow graph: %w", err)
	}
	return nil
}

func (r *Runtime) validateGoogleADKWorkflowResume(ctx context.Context, run Run) error {
	if r == nil || r.store == nil || strings.TrimSpace(run.ParentRunID) == "" {
		return nil
	}
	parent, ok, err := r.store.Run(ctx, run.ParentRunID)
	if err != nil || !ok {
		return err
	}
	service := r.rawSessionService
	if service == nil {
		service = r.sessionService
	}
	if service == nil {
		return nil
	}
	response, err := service.Get(ctx, &adksession.GetRequest{
		AppName: GoogleADKAppName(parent.AgentID), UserID: googleADKUserID, SessionID: parent.SessionID,
	})
	if err != nil {
		lowerErr := strings.ToLower(err.Error())
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(lowerErr, "record not found") || strings.Contains(lowerErr, "not found") {
			return nil
		}
		return fmt.Errorf("get GO-ADK workflow graph session: %w", err)
	}
	if response == nil || response.Session == nil || response.Session.State() == nil {
		return nil
	}
	key := googleADKWorkflowGraphStatePrefix + parent.ID
	stored, err := response.Session.State().Get(key)
	if errors.Is(err, adksession.ErrStateKeyNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read GO-ADK workflow graph: %w", err)
	}
	root, err := r.resolveAgent(ctx, parent.AgentID)
	if err == nil {
		root, err = r.prepareAgent(ctx, root)
	}
	if err != nil {
		return err
	}
	root = jfadkmodel.ApplyRunModelSnapshot(root, parent)
	fingerprint, err := r.googleADKWorkflowGraphFingerprint(ctx, root, parent, nil)
	if err != nil {
		return err
	}
	if strings.TrimSpace(fmt.Sprint(stored)) != fingerprint {
		return fmt.Errorf("GO-ADK workflow graph changed for run %s", parent.ID)
	}
	return nil
}
