package adk

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	adkmodel "google.golang.org/adk/model"
	adkrunner "google.golang.org/adk/runner"
	adksession "google.golang.org/adk/session"
	adktool "google.golang.org/adk/tool"
	"google.golang.org/genai"
	"gorm.io/gorm"
)

const (
	workflowPlannerAgentSuffix = "__workflow_planner"
	workflowPlanResetTool      = "workflow.plan.reset"
	workflowPlanAddStepTool    = "workflow.plan.add_step"
	workflowPlanFinishTool     = "workflow.plan.finish"
)

type workflowPlanDraft struct {
	Mode      string
	Objective string
	Steps     []workflowPlanDraftStep
	Warnings  []string
	Finished  bool
}

type workflowPlanDraftStep struct {
	Order       int
	Title       string
	Message     string
	Description string
	ModeHint    string
	DependsOn   []string
	AgentRole   string
}

type workflowPlannerToolset struct {
	draft *workflowPlanDraft
}

type workflowPlannerTool struct {
	name        string
	description string
	schema      map[string]any
	run         func(map[string]any) (map[string]any, error)
}

func (r *Runtime) planWorkflowWithADK(
	ctx context.Context,
	definition Agent,
	productSession Session,
	mode string,
	message string,
	objective string,
	options RunOptions,
) ([]workflowStep, []string, error) {
	draft := &workflowPlanDraft{Mode: normalizeWorkMode(mode), Objective: strings.TrimSpace(objective)}
	plannerDefinition := definition
	plannerDefinition.ID = definition.ID + workflowPlannerAgentSuffix
	plannerDefinition.Name = definition.Name + " Workflow Planner"
	plannerDefinition.WorkMode = WorkModeChat
	plannerDefinition.Tools = nil
	plannerDefinition.Skills = nil
	llm, err := r.googleADKModelForAgent(ctx, plannerDefinition)
	if err != nil {
		return nil, nil, err
	}
	planner, err := llmagent.New(llmagent.Config{
		Name:        googleADKWorkflowPlannerName(definition.ID),
		Description: "Plans a fixed ADK workflow agent tree before execution.",
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
	appName := googleADKAppName(definition.ID)
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
	for event, runErr := range runner.Run(ctx, googleADKUserID, plannerSessionID, genai.NewContentFromText(workflowPlannerUserMessage(mode, objective, message), genai.RoleUser), adkagent.RunConfig{
		StreamingMode: adkagent.StreamingModeSSE,
	}) {
		if runErr != nil {
			return nil, draft.Warnings, runErr
		}
		_ = event
	}
	steps, warnings, err := compileWorkflowPlanDraft(*draft, mode, message, objective, options)
	return steps, append(draft.Warnings, warnings...), err
}

func googleADKWorkflowPlannerName(agentID string) string {
	return "workflow_planner_" + normalizeID(agentID)
}

func googleADKWorkflowPlannerSessionID(sessionID string) string {
	return strings.TrimSpace(sessionID) + "__workflow_planner"
}

func workflowPlannerInstruction(mode string, objective string, message string, options RunOptions) string {
	return strings.TrimSpace(fmt.Sprintf(`You are an ADK workflow planner.
Create a fixed workflow plan before execution. Use only these tools:
- %s to clear any previous draft.
- %s once per task step, including a 1-based order value.
- %s when the plan is complete.

Do not execute the task. Do not call business tools. Do not start child agents.
Prefer 2-5 concrete steps for broad user goals. Preserve explicit user constraints.
For task workflows, create an initial TODO DAG; execution will be decided by a later ADK task orchestrator.
For loop workflows, produce one observe-plan-act-check step.

Requested mode: %s
Max loop iterations: %d
Objective: %s
User message: %s`, workflowPlanResetTool, workflowPlanAddStepTool, workflowPlanFinishTool, normalizeWorkMode(mode), normalizeLoopMaxIterations(options.LoopMaxIterations), strings.TrimSpace(objective), strings.TrimSpace(message)))
}

func workflowPlannerUserMessage(mode string, objective string, message string) string {
	return fmt.Sprintf("Plan an ADK workflow.\nMode: %s\nObjective: %s\nUser message: %s", normalizeWorkMode(mode), strings.TrimSpace(objective), strings.TrimSpace(message))
}

func newWorkflowPlannerToolset(draft *workflowPlanDraft) adktool.Toolset {
	return &workflowPlannerToolset{draft: draft}
}

func (t *workflowPlannerToolset) Name() string { return "jftrade-workflow-planner-tools" }

func (t *workflowPlannerToolset) Tools(adkagent.ReadonlyContext) ([]adktool.Tool, error) {
	if t == nil || t.draft == nil {
		return nil, nil
	}
	return []adktool.Tool{
		&workflowPlannerTool{
			name:        workflowPlanResetTool,
			description: "Reset the in-memory workflow plan draft.",
			schema:      map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
			run: func(map[string]any) (map[string]any, error) {
				if len(t.draft.Steps) > 0 && !t.draft.Finished {
					t.draft.Warnings = append(t.draft.Warnings, "planner reset ignored after steps were added")
					return map[string]any{"success": true, "ignored": true, "count": len(t.draft.Steps)}, nil
				}
				mode := t.draft.Mode
				objective := t.draft.Objective
				*t.draft = workflowPlanDraft{Mode: mode, Objective: objective}
				return map[string]any{"success": true}, nil
			},
		},
		&workflowPlannerTool{
			name:        workflowPlanAddStepTool,
			description: "Add one task step to the workflow plan draft. This does not execute the step.",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":       map[string]any{"type": "string"},
					"order":       map[string]any{"type": "integer", "minimum": 1},
					"message":     map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"},
					"modeHint":    map[string]any{"type": "string", "enum": []string{"task", "loop", "chat", ""}},
					"dependsOn":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"agentRole":   map[string]any{"type": "string"},
				},
				"required":             []string{"title", "message"},
				"additionalProperties": false,
			},
			run: func(args map[string]any) (map[string]any, error) {
				step := workflowPlanDraftStep{
					Order:       plannerIntArg(args, "order"),
					Title:       plannerStringArg(args, "title"),
					Message:     plannerStringArg(args, "message"),
					Description: plannerStringArg(args, "description"),
					ModeHint:    plannerStringArg(args, "modeHint"),
					AgentRole:   plannerStringArg(args, "agentRole"),
				}
				if values, ok := args["dependsOn"].([]any); ok {
					for _, value := range values {
						if dep := strings.TrimSpace(fmt.Sprint(value)); dep != "" {
							step.DependsOn = append(step.DependsOn, dep)
						}
					}
				}
				t.draft.Steps = append(t.draft.Steps, step)
				return map[string]any{"success": true, "count": len(t.draft.Steps)}, nil
			},
		},
		&workflowPlannerTool{
			name:        workflowPlanFinishTool,
			description: "Mark the workflow plan draft as complete.",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"mode":      map[string]any{"type": "string", "enum": []string{"task", "loop", ""}},
					"objective": map[string]any{"type": "string"},
					"warnings":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"additionalProperties": false,
			},
			run: func(args map[string]any) (map[string]any, error) {
				if mode := normalizeWorkMode(plannerStringArg(args, "mode")); mode != WorkModeChat {
					t.draft.Mode = mode
				}
				if objective := plannerStringArg(args, "objective"); objective != "" {
					t.draft.Objective = objective
				}
				if values, ok := args["warnings"].([]any); ok {
					for _, value := range values {
						if warning := strings.TrimSpace(fmt.Sprint(value)); warning != "" {
							t.draft.Warnings = append(t.draft.Warnings, warning)
						}
					}
				}
				t.draft.Finished = true
				return map[string]any{"success": true, "steps": len(t.draft.Steps)}, nil
			},
		},
	}, nil
}

func (t *workflowPlannerTool) Name() string        { return t.name }
func (t *workflowPlannerTool) Description() string { return t.description }
func (t *workflowPlannerTool) IsLongRunning() bool { return false }

func (t *workflowPlannerTool) Declaration() *genai.FunctionDeclaration {
	return &genai.FunctionDeclaration{Name: t.Name(), Description: t.Description(), ParametersJsonSchema: sanitizeSchemaForOpenAI(t.schema)}
}

func (t *workflowPlannerTool) ProcessRequest(_ adktool.Context, req *adkmodel.LLMRequest) error {
	if req.Tools == nil {
		req.Tools = make(map[string]any)
	}
	if _, exists := req.Tools[t.Name()]; exists {
		return fmt.Errorf("duplicate tool: %q", t.Name())
	}
	req.Tools[t.Name()] = t
	if req.Config == nil {
		req.Config = &genai.GenerateContentConfig{}
	}
	var functionTools *genai.Tool
	for _, item := range req.Config.Tools {
		if item != nil && item.FunctionDeclarations != nil {
			functionTools = item
			break
		}
	}
	if functionTools == nil {
		req.Config.Tools = append(req.Config.Tools, &genai.Tool{FunctionDeclarations: []*genai.FunctionDeclaration{t.Declaration()}})
	} else {
		functionTools.FunctionDeclarations = append(functionTools.FunctionDeclarations, t.Declaration())
	}
	return nil
}

func (t *workflowPlannerTool) Run(_ adktool.Context, args any) (map[string]any, error) {
	input, ok := args.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("tool %s received invalid input %T", t.Name(), args)
	}
	return t.run(input)
}

func plannerStringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}

func plannerIntArg(args map[string]any, key string) int {
	if args == nil {
		return 0
	}
	switch value := args[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return 0
		}
		var parsed int
		if _, err := fmt.Sscan(text, &parsed); err != nil {
			return 0
		}
		return parsed
	}
}

func compileWorkflowPlanDraft(draft workflowPlanDraft, mode string, message string, objective string, options RunOptions) ([]workflowStep, []string, error) {
	if !draft.Finished {
		return nil, draft.Warnings, fmt.Errorf("planner did not finish")
	}
	steps := make([]workflowStep, 0, len(draft.Steps))
	for index, item := range draft.Steps {
		step := workflowStep{
			Order:        item.Order,
			Objective:    strings.TrimSpace(objective),
			Title:        strings.TrimSpace(item.Title),
			Description:  strings.TrimSpace(item.Description),
			Message:      strings.TrimSpace(item.Message),
			DependsOn:    append([]string(nil), item.DependsOn...),
			AgentRole:    strings.TrimSpace(item.AgentRole),
			ModeHint:     strings.TrimSpace(item.ModeHint),
			PlanSource:   workflowPlanSourcePlanner,
			WorkflowMode: normalizeWorkMode(mode),
		}
		if step.Message == "" {
			step.Message = step.Description
		}
		if step.Message == "" {
			continue
		}
		if step.Title == "" {
			step.Title = fmt.Sprintf("步骤 %d", index+1)
		}
		steps = append(steps, step)
	}
	if len(steps) == 0 {
		return nil, draft.Warnings, fmt.Errorf("planner produced no valid steps")
	}
	if err := validateWorkflowPlannerOrders(steps); err != nil {
		return nil, draft.Warnings, err
	}
	sortWorkflowDraftSteps(steps)
	assignWorkflowPlannerDependencyIDs(steps)
	normalizedMode := normalizeWorkMode(mode)
	if normalizedMode == WorkModeLoop && len(steps) > 1 {
		draft.Warnings = append(draft.Warnings, "loop workflow uses the first planner step")
		steps = steps[:1]
	}
	if normalizedMode == WorkModeTask {
		if err := normalizeSequentialPlannerDependencies(steps); err != nil {
			return nil, draft.Warnings, err
		}
	} else if normalizedMode == WorkModeLoop && workflowStepsHaveDependencies(steps) {
		return nil, draft.Warnings, fmt.Errorf("loop planner step must not depend on another step")
	}
	_ = message
	return steps, draft.Warnings, nil
}

func validateWorkflowPlannerOrders(steps []workflowStep) error {
	seen := make(map[int]struct{}, len(steps))
	for _, step := range steps {
		if step.Order <= 0 {
			continue
		}
		if _, exists := seen[step.Order]; exists {
			return fmt.Errorf("planner step order %d is duplicated", step.Order)
		}
		seen[step.Order] = struct{}{}
	}
	return nil
}

func sortWorkflowDraftSteps(steps []workflowStep) {
	if len(steps) < 2 {
		return
	}
	hasOrder := false
	for _, step := range steps {
		if step.Order > 0 {
			hasOrder = true
			break
		}
	}
	if !hasOrder {
		return
	}
	sort.SliceStable(steps, func(i, j int) bool {
		left := steps[i].Order
		right := steps[j].Order
		switch {
		case left > 0 && right > 0:
			return left < right
		case left > 0:
			return true
		case right > 0:
			return false
		default:
			return false
		}
	})
}

func assignWorkflowPlannerDependencyIDs(steps []workflowStep) {
	for index := range steps {
		if strings.TrimSpace(steps[index].DependencyID) == "" {
			steps[index].DependencyID = fmt.Sprintf("__planner_step_%d", index+1)
		}
		if steps[index].Order <= 0 {
			steps[index].Order = index + 1
		}
	}
}

func workflowStepsHaveDependencies(steps []workflowStep) bool {
	for _, step := range steps {
		for _, dep := range step.DependsOn {
			if strings.TrimSpace(dep) != "" {
				return true
			}
		}
	}
	return false
}

func normalizeSequentialPlannerDependencies(steps []workflowStep) error {
	aliases := make(map[string]int, len(steps)*4)
	for index, step := range steps {
		for _, alias := range workflowStepDependencyAliases(step, index) {
			if previous, exists := aliases[alias]; exists && previous != index {
				return fmt.Errorf("planner dependency alias %q is ambiguous", alias)
			}
			aliases[alias] = index
		}
	}
	var previousID string
	for index := range steps {
		if previousID != "" && len(trimWorkflowDependencies(steps[index].DependsOn)) == 0 {
			steps[index].DependsOn = []string{previousID}
		} else {
			resolved, err := resolveWorkflowStepDependencies(steps[index].DependsOn, aliases, steps, index)
			if err != nil {
				return err
			}
			steps[index].DependsOn = resolved
		}
		previousID = steps[index].DependencyID
	}
	return nil
}

func workflowStepDependencyAliases(step workflowStep, index int) []string {
	aliases := []string{
		strings.TrimSpace(step.DependencyID),
	}
	if step.Order > 0 {
		aliases = append(aliases, fmt.Sprintf("%d", step.Order), fmt.Sprintf("#%d", step.Order), fmt.Sprintf("step-%d", step.Order))
	} else {
		aliases = append(aliases, fmt.Sprintf("%d", index+1), fmt.Sprintf("#%d", index+1), fmt.Sprintf("step-%d", index+1))
	}
	if title := strings.TrimSpace(step.Title); title != "" {
		aliases = append(aliases, title)
	}
	return normalizeStringSlice(aliases)
}

func resolveWorkflowStepDependencies(raw []string, aliases map[string]int, steps []workflowStep, currentIndex int) ([]string, error) {
	deps := trimWorkflowDependencies(raw)
	if len(deps) == 0 {
		return nil, nil
	}
	resolved := make([]string, 0, len(deps))
	seen := make(map[string]struct{}, len(deps))
	for _, dep := range deps {
		depIndex, ok := aliases[dep]
		if !ok {
			return nil, fmt.Errorf("planner dependency %q does not reference a known step", dep)
		}
		if depIndex >= currentIndex {
			return nil, fmt.Errorf("planner dependency %q must reference an earlier step", dep)
		}
		id := strings.TrimSpace(steps[depIndex].DependencyID)
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		resolved = append(resolved, id)
	}
	return resolved, nil
}

func trimWorkflowDependencies(raw []string) []string {
	deps := make([]string, 0, len(raw))
	for _, dep := range raw {
		if trimmed := strings.TrimSpace(dep); trimmed != "" {
			deps = append(deps, trimmed)
		}
	}
	return deps
}
