package model

import (
	"fmt"
	"sort"
	"strings"
)

// Planner tool names shared by the planner prompt and the ADK toolset.
const (
	WorkflowPlanResetTool   = "workflow.plan.reset"
	WorkflowPlanAddStepTool = "workflow.plan.add_step"
	WorkflowPlanFinishTool  = "workflow.plan.finish"
)

// WorkflowPlanDraft is the in-memory planner output accumulated through the
// ADK workflow planner toolset before it is compiled into executable steps.
type WorkflowPlanDraft struct {
	Mode      string
	Objective string
	Steps     []WorkflowPlanDraftStep
	Warnings  []string
	Finished  bool
}

// WorkflowPlanDraftStep is one raw step submitted by the planner model.
type WorkflowPlanDraftStep struct {
	Order           int
	Title           string
	Message         string
	Description     string
	ModeHint        string
	DependsOn       []string
	AgentRole       string
	ChildProviderID string
	ChildModel      string
}

// WorkflowPlannerDraftStepFromArgs decodes a raw planner tool argument map
// into a draft step.
func WorkflowPlannerDraftStepFromArgs(args map[string]any) WorkflowPlanDraftStep {
	return WorkflowPlanDraftStep{
		Order:           PlannerIntArg(args, "order"),
		Title:           PlannerStringArg(args, "title"),
		Message:         PlannerStringArg(args, "message"),
		Description:     PlannerStringArg(args, "description"),
		ModeHint:        PlannerStringArg(args, "modeHint"),
		DependsOn:       PlannerStringListArg(args, "dependsOn"),
		AgentRole:       PlannerStringArg(args, "agentRole"),
		ChildProviderID: PlannerStringArg(args, "childProviderId"),
		ChildModel:      PlannerStringArg(args, "childModel"),
	}
}

// WorkflowPlannerInstruction renders the planner agent system prompt.
func WorkflowPlannerInstruction(mode string, objective string, message string, options RunOptions) string {
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
User message: %s`, WorkflowPlanResetTool, WorkflowPlanAddStepTool, WorkflowPlanFinishTool, NormalizeWorkMode(mode), NormalizeLoopMaxIterations(options.LoopMaxIterations), strings.TrimSpace(objective), strings.TrimSpace(message)))
}

// WorkflowPlannerUserMessage renders the planner runner user message.
func WorkflowPlannerUserMessage(mode string, objective string, message string) string {
	return fmt.Sprintf("Plan an ADK workflow.\nMode: %s\nObjective: %s\nUser message: %s", NormalizeWorkMode(mode), strings.TrimSpace(objective), strings.TrimSpace(message))
}

// WorkflowPlannerAddStepSchema is the JSON schema for the planner add-step tool.
func WorkflowPlannerAddStepSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":           map[string]any{"type": "string"},
			"order":           map[string]any{"type": "integer", "minimum": 1},
			"message":         map[string]any{"type": "string"},
			"description":     map[string]any{"type": "string"},
			"modeHint":        map[string]any{"type": "string", "enum": []string{"loop", "chat", ""}},
			"dependsOn":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"agentRole":       map[string]any{"type": "string"},
			"childProviderId": map[string]any{"type": "string"},
			"childModel":      map[string]any{"type": "string"},
		},
		"required":             []string{"title", "message"},
		"additionalProperties": false,
	}
}

// WorkflowPlannerFinishSchema is the JSON schema for the planner finish tool.
func WorkflowPlannerFinishSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode":      map[string]any{"type": "string", "enum": []string{"loop", ""}},
			"objective": map[string]any{"type": "string"},
			"warnings":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"additionalProperties": false,
	}
}

// PlannerStringListArg extracts a trimmed string slice from a tool argument.
func PlannerStringListArg(args map[string]any, key string) []string {
	values, ok := args[key].([]any)
	if !ok {
		return nil
	}
	items := make([]string, 0, len(values))
	for _, value := range values {
		if item := strings.TrimSpace(fmt.Sprint(value)); item != "" {
			items = append(items, item)
		}
	}
	return items
}

// PlannerStringArg extracts a trimmed string from a tool argument.
func PlannerStringArg(args map[string]any, key string) string {
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

// PlannerIntArg extracts an integer from a tool argument, tolerating numeric
// JSON decodings and numeric strings.
func PlannerIntArg(args map[string]any, key string) int {
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

// CompileWorkflowPlanDraft validates and normalizes a finished planner draft
// into the executable step projection.
func CompileWorkflowPlanDraft(draft WorkflowPlanDraft, mode string, message string, objective string, options RunOptions) ([]WorkflowStep, []string, error) {
	if !draft.Finished {
		return nil, draft.Warnings, fmt.Errorf("planner did not finish")
	}
	steps := make([]WorkflowStep, 0, len(draft.Steps))
	for index, item := range draft.Steps {
		step := WorkflowStep{
			Order:           item.Order,
			Title:           strings.TrimSpace(item.Title),
			Description:     strings.TrimSpace(item.Description),
			Message:         strings.TrimSpace(item.Message),
			DependsOn:       append([]string(nil), item.DependsOn...),
			AgentRole:       strings.TrimSpace(item.AgentRole),
			ChildProviderID: strings.TrimSpace(item.ChildProviderID),
			ChildModel:      strings.TrimSpace(item.ChildModel),
			ModeHint:        strings.TrimSpace(item.ModeHint),
			PlanSource:      WorkflowPlanSourcePlanner,
			WorkflowMode:    NormalizeWorkMode(mode),
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
		step = SanitizeWorkflowPlanStep(step, message, index)
		if strings.TrimSpace(objective) != strings.TrimSpace(message) {
			step = SanitizeWorkflowPlanStep(step, objective, index)
		}
		steps = append(steps, step)
	}
	if len(steps) == 0 {
		return nil, draft.Warnings, fmt.Errorf("planner produced no valid steps")
	}
	if NormalizeWorkflowPlannerDuplicateOrders(steps) {
		draft.Warnings = append(draft.Warnings, "planner step orders were duplicated and normalized")
	}
	SortWorkflowDraftSteps(steps)
	AssignWorkflowPlannerDependencyIDs(steps)
	normalizedMode := NormalizeWorkMode(mode)
	if normalizedMode == WorkModeLoop && len(steps) > 1 {
		draft.Warnings = append(draft.Warnings, "loop workflow uses the first planner step")
		steps = steps[:1]
	}
	if normalizedMode == WorkModeLoop && WorkflowStepsHaveDependencies(steps) {
		return nil, draft.Warnings, fmt.Errorf("loop planner step must not depend on another step")
	}
	return steps, draft.Warnings, nil
}

// NormalizeWorkflowPlannerDuplicateOrders rewrites duplicate step orders to
// sequential values and reports whether normalization was needed.
func NormalizeWorkflowPlannerDuplicateOrders(steps []WorkflowStep) bool {
	seen := make(map[int]struct{}, len(steps))
	for _, step := range steps {
		if step.Order <= 0 {
			continue
		}
		if _, exists := seen[step.Order]; exists {
			SortWorkflowDraftSteps(steps)
			for index := range steps {
				steps[index].Order = index + 1
			}
			return true
		}
		seen[step.Order] = struct{}{}
	}
	return false
}

// SortWorkflowDraftSteps orders draft steps by their explicit order value.
func SortWorkflowDraftSteps(steps []WorkflowStep) {
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

// AssignWorkflowPlannerDependencyIDs fills missing planner step IDs and
// 1-based orders.
func AssignWorkflowPlannerDependencyIDs(steps []WorkflowStep) {
	for index := range steps {
		if strings.TrimSpace(steps[index].DependencyID) == "" {
			steps[index].DependencyID = fmt.Sprintf("__planner_step_%d", index+1)
		}
		if steps[index].Order <= 0 {
			steps[index].Order = index + 1
		}
	}
}

// WorkflowStepsHaveDependencies reports whether any step declares a non-empty
// dependency.
func WorkflowStepsHaveDependencies(steps []WorkflowStep) bool {
	for _, step := range steps {
		for _, dep := range step.DependsOn {
			if strings.TrimSpace(dep) != "" {
				return true
			}
		}
	}
	return false
}

// NormalizeSequentialPlannerDependencies resolves planner dependency aliases
// into step IDs and falls back to sequential chaining.
func NormalizeSequentialPlannerDependencies(steps []WorkflowStep) error {
	aliases := make(map[string]int, len(steps)*4)
	for index, step := range steps {
		for _, alias := range WorkflowStepDependencyAliases(step, index) {
			if previous, exists := aliases[alias]; exists && previous != index {
				return fmt.Errorf("planner dependency alias %q is ambiguous", alias)
			}
			aliases[alias] = index
		}
	}
	var previousID string
	for index := range steps {
		if previousID != "" && len(TrimWorkflowDependencies(steps[index].DependsOn)) == 0 {
			steps[index].DependsOn = []string{previousID}
		} else {
			resolved, err := ResolveWorkflowStepDependencies(steps[index].DependsOn, aliases, steps, index)
			if err != nil {
				return err
			}
			steps[index].DependsOn = resolved
		}
		previousID = steps[index].DependencyID
	}
	return nil
}

// WorkflowStepDependencyAliases returns every alias that can reference a step.
func WorkflowStepDependencyAliases(step WorkflowStep, index int) []string {
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
	return NormalizeStringSlice(aliases)
}

// ResolveWorkflowStepDependencies maps raw dependency aliases to step IDs,
// rejecting unknown aliases and forward references.
func ResolveWorkflowStepDependencies(raw []string, aliases map[string]int, steps []WorkflowStep, currentIndex int) ([]string, error) {
	deps := TrimWorkflowDependencies(raw)
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

// TrimWorkflowDependencies removes blank dependency entries.
func TrimWorkflowDependencies(raw []string) []string {
	deps := make([]string, 0, len(raw))
	for _, dep := range raw {
		if trimmed := strings.TrimSpace(dep); trimmed != "" {
			deps = append(deps, trimmed)
		}
	}
	return deps
}

// ApplyWorkflowStepPlanningMetadata fills planner-derived step metadata and
// sanitizes echoed user text.
func ApplyWorkflowStepPlanningMetadata(steps []WorkflowStep, mode string, objective string, warnings []string) []WorkflowStep {
	normalizedWarnings := NormalizeStringSlice(warnings)
	normalizedMode := NormalizeWorkMode(mode)
	for index := range steps {
		if steps[index].Order <= 0 {
			steps[index].Order = index + 1
		}
		if strings.TrimSpace(steps[index].WorkflowMode) == "" {
			steps[index].WorkflowMode = normalizedMode
		}
		steps[index].Objective = ""
		if strings.TrimSpace(steps[index].PlanSource) == "" {
			steps[index].PlanSource = WorkflowPlanSourcePlanner
		}
		if len(normalizedWarnings) > 0 {
			steps[index].PlannerWarnings = append([]string(nil), normalizedWarnings...)
		}
		steps[index] = SanitizeWorkflowPlanStep(steps[index], objective, index)
	}
	return steps
}

// SanitizeWorkflowPlanStep rewrites steps that echo the user request back
// verbatim into stable placeholders.
func SanitizeWorkflowPlanStep(step WorkflowStep, userRequest string, index int) WorkflowStep {
	original := strings.TrimSpace(userRequest)
	if original == "" {
		return step
	}
	if strings.TrimSpace(step.Title) == original {
		step.Title = fmt.Sprintf("执行计划步骤 %d", index+1)
	}
	if strings.TrimSpace(step.Description) == original {
		step.Description = ""
	}
	if strings.TrimSpace(step.Message) == original {
		if description := strings.TrimSpace(step.Description); description != "" {
			step.Message = description
		} else {
			step.Message = fmt.Sprintf("推进计划中的第 %d 步。", index+1)
		}
	}
	return step
}
