package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	adkrunner "google.golang.org/adk/v2/runner"
	adksession "google.golang.org/adk/v2/session"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/genai"
	"gorm.io/gorm"
)

const (
	workflowPlannerAgentSuffix = "__workflow_planner"
	workflowPlanResetTool      = jfadkmodel.WorkflowPlanResetTool
	workflowPlanAddStepTool    = jfadkmodel.WorkflowPlanAddStepTool
	workflowPlanFinishTool     = jfadkmodel.WorkflowPlanFinishTool
)

type workflowPlanDraft = jfadkmodel.WorkflowPlanDraft

type workflowPlanDraftStep = jfadkmodel.WorkflowPlanDraftStep

type workflowPlannerToolset struct {
	draft *workflowPlanDraft
}

type WorkflowMapToolSpec = jfadkmodel.WorkflowMapToolSpec

func (r *Runtime) PlanWorkflowWithADK(
	ctx context.Context,
	definition Agent,
	productSession Session,
	mode string,
	message string,
	objective string,
	options RunOptions,
) ([]workflowStep, []string, error) {
	draft := &workflowPlanDraft{Mode: jfadkmodel.NormalizeWorkMode(mode), Objective: strings.TrimSpace(objective)}
	plannerDefinition := definition
	plannerDefinition.ID = definition.ID + workflowPlannerAgentSuffix
	plannerDefinition.Name = definition.Name + " Workflow Planner"
	plannerDefinition.WorkMode = WorkModeChat
	plannerDefinition.Tools = nil
	plannerDefinition.Skills = nil
	llm, err := r.GoogleADKModelForAgent(ctx, plannerDefinition)
	if err != nil {
		return nil, nil, err
	}
	planner, err := llmagent.New(llmagent.Config{
		Name:                  googleADKWorkflowPlannerName(definition.ID),
		Description:           "Plans a fixed ADK workflow agent tree before execution.",
		InstructionProvider: func(adkagent.ReadonlyContext) (string, error) {
			return workflowPlannerInstruction(mode, objective, message, options), nil
		},
		Model:           llm,
		Toolsets:        []adktool.Toolset{newWorkflowPlannerToolset(draft)},
		IncludeContents: llmagent.IncludeContentsNone,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create GO-ADK workflow planner agent: %w", err)
	}
	service := r.sessionService
	if service == nil {
		service = adksession.InMemoryService()
	}
	appName := GoogleADKAppName(definition.ID)
	plannerSessionID := googleADKWorkflowPlannerSessionID(productSession.ID)
	if _, err := service.Get(ctx, &adksession.GetRequest{
		AppName: appName, UserID: googleADKUserID, SessionID: plannerSessionID,
	}); err != nil {
		lowerErr := strings.ToLower(err.Error())
		if !errors.Is(err, gorm.ErrRecordNotFound) && !strings.Contains(lowerErr, "record not found") && !strings.Contains(lowerErr, "not found") {
			return nil, nil, fmt.Errorf("get GO-ADK planner session: %w", err)
		}
		if _, createErr := service.Create(ctx, &adksession.CreateRequest{
			AppName: appName, UserID: googleADKUserID, SessionID: plannerSessionID,
		}); createErr != nil {
			return nil, nil, fmt.Errorf("create GO-ADK planner session: %w", createErr)
		}
	}
	runner, err := adkrunner.New(adkrunner.Config{
		AppName: appName, Agent: planner, SessionService: service,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create GO-ADK workflow planner runner: %w", err)
	}
	runCtx := googleADKTaskRunnerContext(ctx)
	for event, runErr := range runner.Run(runCtx, googleADKUserID, plannerSessionID, genai.NewContentFromText(workflowPlannerUserMessage(mode, objective, message), genai.RoleUser), adkagent.RunConfig{
		StreamingMode: adkagent.StreamingModeSSE,
	}) {
		if runErr != nil {
			return nil, draft.Warnings, runErr
		}
		_ = event
	}
	steps, warnings, err := compileWorkflowPlanDraft(*draft, mode, message, objective, options)
	return steps, warnings, err
}

func googleADKWorkflowPlannerName(agentID string) string {
	return "workflow_planner_" + normalizeID(agentID)
}

func googleADKWorkflowPlannerSessionID(sessionID string) string {
	return strings.TrimSpace(sessionID) + "__workflow_planner"
}

func workflowPlannerInstruction(mode string, objective string, message string, options RunOptions) string {
	return jfadkmodel.WorkflowPlannerInstruction(mode, objective, message, options)
}

func workflowPlannerUserMessage(mode string, objective string, message string) string {
	return jfadkmodel.WorkflowPlannerUserMessage(mode, objective, message)
}

func newWorkflowPlannerToolset(draft *workflowPlanDraft) adktool.Toolset {
	return &workflowPlannerToolset{draft: draft}
}

func (t *workflowPlannerToolset) Name() string { return "jftrade-workflow-planner-tools" }

func (t *workflowPlannerToolset) Tools(adkagent.ReadonlyContext) ([]adktool.Tool, error) {
	if t == nil || t.draft == nil {
		return nil, nil
	}
	return NewWorkflowMapFunctionTools(
		workflowPlannerResetSpec(t.draft),
		workflowPlannerAddStepSpec(t.draft),
		workflowPlannerFinishSpec(t.draft),
	)
}

func workflowPlannerResetSpec(draft *workflowPlanDraft) WorkflowMapToolSpec {
	return WorkflowMapToolSpec{
		Name:        workflowPlanResetTool,
		Description: "Reset the in-memory workflow plan draft.",
		Schema:      map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		Run: func(map[string]any) (map[string]any, error) {
			if len(draft.Steps) > 0 && !draft.Finished {
				draft.Warnings = append(draft.Warnings, "planner reset ignored after steps were added")
				return map[string]any{"success": true, "ignored": true, "count": len(draft.Steps)}, nil
			}
			mode := draft.Mode
			objective := draft.Objective
			*draft = workflowPlanDraft{Mode: mode, Objective: objective}
			return map[string]any{"success": true}, nil
		},
	}
}

func workflowPlannerAddStepSpec(draft *workflowPlanDraft) WorkflowMapToolSpec {
	return WorkflowMapToolSpec{
		Name:        workflowPlanAddStepTool,
		Description: "Add one task step to the workflow plan draft. This does not execute the step.",
		Schema:      workflowPlannerAddStepSchema(),
		Run: func(args map[string]any) (map[string]any, error) {
			draft.Steps = append(draft.Steps, workflowPlannerDraftStepFromArgs(args))
			return map[string]any{"success": true, "count": len(draft.Steps)}, nil
		},
	}
}

func workflowPlannerAddStepSchema() map[string]any {
	return jfadkmodel.WorkflowPlannerAddStepSchema()
}

func workflowPlannerFinishSpec(draft *workflowPlanDraft) WorkflowMapToolSpec {
	return WorkflowMapToolSpec{
		Name:        workflowPlanFinishTool,
		Description: "Mark the workflow plan draft as complete.",
		Schema:      workflowPlannerFinishSchema(),
		Run: func(args map[string]any) (map[string]any, error) {
			if mode := jfadkmodel.NormalizeWorkMode(jfadkmodel.PlannerStringArg(args, "mode")); mode != WorkModeChat {
				draft.Mode = mode
			}
			if objective := jfadkmodel.PlannerStringArg(args, "objective"); objective != "" {
				draft.Objective = objective
			}
			draft.Warnings = append(draft.Warnings, jfadkmodel.PlannerStringListArg(args, "warnings")...)
			draft.Finished = true
			return map[string]any{"success": true, "steps": len(draft.Steps)}, nil
		},
	}
}

func workflowPlannerFinishSchema() map[string]any {
	return jfadkmodel.WorkflowPlannerFinishSchema()
}

func NewWorkflowMapFunctionTools(specs ...WorkflowMapToolSpec) ([]adktool.Tool, error) {
	return jfadkmodel.NewWorkflowMapFunctionTools(specs...)
}

func NewWorkflowMapFunctionTool(spec WorkflowMapToolSpec) (adktool.Tool, error) {
	return jfadkmodel.NewWorkflowMapFunctionTool(spec)
}

func workflowPlannerDraftStepFromArgs(args map[string]any) workflowPlanDraftStep {
	return jfadkmodel.WorkflowPlannerDraftStepFromArgs(args)
}

func compileWorkflowPlanDraft(draft workflowPlanDraft, mode string, message string, objective string, options RunOptions) ([]workflowStep, []string, error) {
	return jfadkmodel.CompileWorkflowPlanDraft(draft, mode, message, objective, options)
}

func workflowStepsHaveDependencies(steps []workflowStep) bool {
	return jfadkmodel.WorkflowStepsHaveDependencies(steps)
}

func normalizeSequentialPlannerDependencies(steps []workflowStep) error {
	return jfadkmodel.NormalizeSequentialPlannerDependencies(steps)
}

func resolveWorkflowStepDependencies(raw []string, aliases map[string]int, steps []workflowStep, currentIndex int) ([]string, error) {
	return jfadkmodel.ResolveWorkflowStepDependencies(raw, aliases, steps, currentIndex)
}

func applyWorkflowStepPlanningMetadata(steps []workflowStep, mode string, objective string, warnings []string) []workflowStep {
	return jfadkmodel.ApplyWorkflowStepPlanningMetadata(steps, mode, objective, warnings)
}

func sanitizeWorkflowPlanStep(step workflowStep, userRequest string, index int) workflowStep {
	return jfadkmodel.SanitizeWorkflowPlanStep(step, userRequest, index)
}
