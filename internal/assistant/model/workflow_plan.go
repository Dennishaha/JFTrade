package model

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// WorkflowEngineForMode maps a normalized work mode to its ADK workflow
// engine identifier.
func WorkflowEngineForMode(mode string) string {
	switch NormalizeWorkMode(mode) {
	case WorkModeLoop:
		return WorkflowEngineADK2Loop
	default:
		return ""
	}
}

// WorkflowPlanFromTasks projects tasks into the run's step plan, preserving
// runtime-only state from the existing plan by task ID.
func WorkflowPlanFromTasks(tasks []Task, existing []WorkflowStepState) []WorkflowStepState {
	existingByTaskID := make(map[string]WorkflowStepState, len(existing))
	for _, state := range existing {
		if strings.TrimSpace(state.TaskID) != "" {
			existingByTaskID[state.TaskID] = state
		}
	}
	ordered := append([]Task(nil), tasks...)
	SortWorkflowTasks(ordered)
	plan := make([]WorkflowStepState, 0, len(ordered))
	for index, task := range ordered {
		prior := existingByTaskID[task.ID]
		state := WorkflowStepState{
			TaskID:              task.ID,
			Title:               task.Title,
			Description:         task.Description,
			Message:             task.Message,
			Status:              DefaultString(task.Status, "TODO"),
			DependsOn:           append([]string(nil), task.DependsOn...),
			Iteration:           index + 1,
			Order:               task.Order,
			ModeHint:            task.ModeHint,
			AgentRole:           task.AgentRole,
			ChildAgentID:        task.ChildAgentID,
			ChildProviderID:     task.ChildProviderID,
			ChildModel:          task.ChildModel,
			ChildPermissionMode: task.ChildPermissionMode,
			PlannerStepID:       task.PlannerStepID,
			PlanSource:          task.PlanSource,
			WorkflowMode:        task.WorkflowMode,
			Objective:           task.Objective,
			Executor:            task.Executor,
			ResultSummary:       task.ResultSummary,
			PlannerWarnings:     append([]string(nil), task.PlannerWarnings...),
		}
		if strings.TrimSpace(state.Title) == "" {
			state.Title = prior.Title
		}
		if strings.TrimSpace(state.Description) == "" {
			state.Description = prior.Description
		}
		if strings.TrimSpace(state.Message) == "" {
			state.Message = prior.Message
		}
		if strings.TrimSpace(state.PlanSource) == "" {
			state.PlanSource = prior.PlanSource
		}
		if strings.TrimSpace(state.WorkflowMode) == "" {
			state.WorkflowMode = prior.WorkflowMode
		}
		if strings.TrimSpace(state.Objective) == "" {
			state.Objective = prior.Objective
		}
		state.ChildRunID = prior.ChildRunID
		state.NodeName = prior.NodeName
		state.NodeStatus = prior.NodeStatus
		state.Routes = NormalizeStringSlice(prior.Routes)
		state.OutputSummary = prior.OutputSummary
		plan = append(plan, state)
	}
	return plan
}

// IsDirectWorkflowChild reports whether child is a direct child run of parent.
func IsDirectWorkflowChild(parent Run, child Run) bool {
	return strings.TrimSpace(parent.ID) != "" &&
		strings.TrimSpace(child.ID) != "" &&
		child.ID != parent.ID &&
		strings.TrimSpace(child.ParentRunID) == parent.ID
}

// WorkflowTasksComplete reports whether every task in the plan is DONE.
func WorkflowTasksComplete(tasks []Task) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, task := range tasks {
		if task.Status != "DONE" {
			return false
		}
	}
	return true
}

// FirstTerminalWorkflowTask returns the first BLOCKED or CANCELLED task.
func FirstTerminalWorkflowTask(tasks []Task) (Task, bool) {
	for _, task := range tasks {
		if task.Status == "BLOCKED" || task.Status == "CANCELLED" {
			return task, true
		}
	}
	return Task{}, false
}

// ExecutableWorkflowTasks returns TODO tasks whose dependencies are all DONE,
// limited to the single next ready task for deterministic execution.
func ExecutableWorkflowTasks(tasks []Task, _ string) []Task {
	taskByID := make(map[string]Task, len(tasks))
	for _, task := range tasks {
		taskByID[task.ID] = task
	}
	ready := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if task.Status != "TODO" {
			continue
		}
		depsDone := true
		for _, dep := range task.DependsOn {
			depTask, ok := taskByID[dep]
			if !ok || depTask.Status != "DONE" {
				depsDone = false
				break
			}
		}
		if depsDone {
			ready = append(ready, task)
		}
	}
	SortWorkflowTasks(ready)
	if len(ready) > 1 {
		return ready[:1]
	}
	return ready
}

// WorkflowStepFromTask derives the executable step projection from a task.
func WorkflowStepFromTask(task Task) WorkflowStep {
	message := strings.TrimSpace(task.Message)
	if message == "" {
		message = DefaultString(task.Description, task.Title)
	}
	return WorkflowStep{
		Order:               task.Order,
		DependencyID:        task.PlannerStepID,
		Title:               task.Title,
		Description:         WorkflowDescriptionWithoutAgentRole(task.Description),
		Message:             message,
		DependsOn:           append([]string(nil), task.DependsOn...),
		AgentRole:           task.AgentRole,
		ChildAgentID:        task.ChildAgentID,
		ChildProviderID:     task.ChildProviderID,
		ChildModel:          task.ChildModel,
		ChildPermissionMode: task.ChildPermissionMode,
		ModeHint:            task.ModeHint,
		Objective:           task.Objective,
		PlanSource:          task.PlanSource,
		WorkflowMode:        task.WorkflowMode,
		PlannerWarnings:     append([]string(nil), task.PlannerWarnings...),
	}
}

// WorkflowTaskIteration returns the task's 1-based plan iteration.
func WorkflowTaskIteration(task Task) int {
	if task.Order > 0 {
		return task.Order
	}
	return 1
}

// WorkflowPlanIndexForTask returns the plan index of a task ID, or -1.
func WorkflowPlanIndexForTask(plan []WorkflowStepState, taskID string) int {
	for index, state := range plan {
		if state.TaskID == taskID {
			return index
		}
	}
	return -1
}

// WorkflowSelfTaskSummary builds the completion summary for a task executed
// by the parent agent itself.
func WorkflowSelfTaskSummary(task Task) string {
	if strings.TrimSpace(task.ResultSummary) != "" {
		return task.ResultSummary
	}
	subject := strings.TrimSpace(task.Title)
	if subject == "" {
		subject = "任务"
	}
	detail := strings.TrimSpace(task.Description)
	if detail == "" {
		detail = strings.TrimSpace(task.Message)
	}
	if detail == "" {
		return subject + " 已由父智能体完成。"
	}
	if len([]rune(detail)) > 120 {
		detail = string([]rune(detail)[:120]) + "..."
	}
	return fmt.Sprintf("%s 已由父智能体完成：%s", subject, detail)
}

// WorkflowTasksHaveCycle reports whether task dependencies form a cycle.
func WorkflowTasksHaveCycle(tasks []Task) bool {
	graph := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		graph[task.ID] = append([]string(nil), task.DependsOn...)
	}
	visiting := make(map[string]bool, len(tasks))
	visited := make(map[string]bool, len(tasks))
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, dep := range graph[id] {
			if graph[dep] == nil {
				continue
			}
			if visit(dep) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for id := range graph {
		if visit(id) {
			return true
		}
	}
	return false
}

// SortWorkflowTasks orders tasks by order, created time, then ID.
func SortWorkflowTasks(tasks []Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Order != tasks[j].Order {
			if tasks[i].Order == 0 {
				return false
			}
			if tasks[j].Order == 0 {
				return true
			}
			return tasks[i].Order < tasks[j].Order
		}
		if tasks[i].CreatedAt != tasks[j].CreatedAt {
			return tasks[i].CreatedAt < tasks[j].CreatedAt
		}
		return tasks[i].ID < tasks[j].ID
	})
}

// WorkflowStepDescription appends the agent role to a step description.
func WorkflowStepDescription(step WorkflowStep) string {
	description := strings.TrimSpace(step.Description)
	if strings.TrimSpace(step.AgentRole) == "" {
		return description
	}
	role := "Agent role: " + strings.TrimSpace(step.AgentRole)
	if description == "" {
		return role
	}
	return description + "\n\n" + role
}

// WorkflowDescriptionWithoutAgentRole strips the generated agent role suffix
// from a task description.
func WorkflowDescriptionWithoutAgentRole(description string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return ""
	}
	if index := strings.LastIndex(description, "\n\nAgent role:"); index >= 0 {
		return strings.TrimSpace(description[:index])
	}
	if strings.HasPrefix(description, "Agent role:") {
		return ""
	}
	return description
}

// PauseParentForChild projects a child run's blocking state onto its parent.
func PauseParentForChild(parent Run, child Run, cursor int) Run {
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = WorkflowEngineForMode(parent.WorkMode)
	}
	parent.Status = child.Status
	parent.Message = child.Message
	parent.PendingApprovals = PendingApprovalsOnly(child.PendingApprovals)
	parent.InputRequest = NormalizeInputRequest(child.InputRequest)
	parent.WorkflowStatus = WorkflowStatusPaused
	parent.WorkflowCursor = cursor
	parent = UpdateWorkflowPlanForChildAt(parent, child, cursor)
	return parent
}

// ChildRunIDs returns the unique, non-empty child run IDs in order.
func ChildRunIDs(children []Run) []string {
	ids := make([]string, 0, len(children))
	for _, child := range children {
		ids = AppendUniqueString(ids, child.ID)
	}
	return ids
}

// ApprovalsForRun filters pending approvals for one run.
func ApprovalsForRun(approvals []Approval, runID string) []Approval {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	filtered := make([]Approval, 0, len(approvals))
	for _, approval := range approvals {
		if approval.RunID == runID {
			filtered = append(filtered, approval)
		}
	}
	return PendingApprovalsOnly(filtered)
}

// WorkflowPendingReply returns the user-facing message for a blocked workflow.
func WorkflowPendingReply(parent Run) string {
	if parent.Status == RunStatusPendingInput {
		return "工作流正在等待用户回答。"
	}
	if parent.Status != RunStatusPending {
		if strings.TrimSpace(parent.FailureReason) != "" {
			return parent.FailureReason
		}
		return parent.Message
	}
	switch parent.WorkMode {
	case WorkModeLoop:
		return "目标模式正在等待审批。"
	default:
		return "工作流正在等待审批。"
	}
}

// UpdateWorkflowPlanForChild projects a child run onto the parent plan.
func UpdateWorkflowPlanForChild(parent Run, child Run) Run {
	return UpdateWorkflowPlanForChildAt(parent, child, -1)
}

// UpdateWorkflowPlanForChildAt projects a child run onto the parent plan at a
// specific step index, falling back to the matching child run ID.
func UpdateWorkflowPlanForChildAt(parent Run, child Run, stepIndex int) Run {
	if len(parent.WorkflowPlan) == 0 || strings.TrimSpace(child.ID) == "" {
		return parent
	}
	if stepIndex >= 0 && stepIndex < len(parent.WorkflowPlan) {
		ApplyWorkflowChildState(&parent.WorkflowPlan[stepIndex], child)
		parent.WorkflowCursor = stepIndex
		return parent
	}
	matched := false
	for index := range parent.WorkflowPlan {
		step := &parent.WorkflowPlan[index]
		if step.ChildRunID == child.ID {
			matched = true
		}
	}
	for index := range parent.WorkflowPlan {
		step := &parent.WorkflowPlan[index]
		if matched && step.ChildRunID != child.ID {
			continue
		}
		if !matched {
			break
		}
		ApplyWorkflowChildState(step, child)
		parent.WorkflowCursor = index
		break
	}
	return parent
}

// ApplyWorkflowChildState projects one child run onto a plan step.
func ApplyWorkflowChildState(step *WorkflowStepState, child Run) {
	if step == nil {
		return
	}
	step.ChildRunID = child.ID
	step.Executor = WorkflowTaskExecutorChild
	if strings.TrimSpace(step.ChildAgentID) == "" {
		step.ChildAgentID = strings.TrimSpace(child.AgentID)
	}
	if strings.TrimSpace(step.ChildProviderID) == "" {
		step.ChildProviderID = strings.TrimSpace(child.ProviderID)
	}
	if strings.TrimSpace(step.ChildModel) == "" {
		step.ChildModel = strings.TrimSpace(child.Model)
	}
	if strings.TrimSpace(step.ChildPermissionMode) == "" {
		step.ChildPermissionMode = strings.TrimSpace(child.PermissionMode)
	}
	switch child.Status {
	case RunStatusCompleted:
		step.Status = "DONE"
		step.ResultSummary = strings.TrimSpace(child.Message)
	case RunStatusPending, RunStatusPendingInput:
		step.Status = "BLOCKED"
	case RunStatusRunning:
		step.Status = "IN_PROGRESS"
	default:
		step.Status = "BLOCKED"
	}
	if child.Iteration > 0 {
		step.Iteration = child.Iteration
	}
}

// WorkflowChildAgentForStep derives a chat-mode child agent from a plan step.
func WorkflowChildAgentForStep(agent Agent, step WorkflowStep) Agent {
	child := agent
	child.WorkMode = WorkModeChat
	if agentID := strings.TrimSpace(step.ChildAgentID); agentID != "" {
		child.ID = agentID
	}
	if providerID := strings.TrimSpace(step.ChildProviderID); providerID != "" {
		child.ProviderID = providerID
	}
	if model := strings.TrimSpace(step.ChildModel); model != "" {
		child.Model = model
	}
	if mode := strings.TrimSpace(step.ChildPermissionMode); mode != "" {
		child.PermissionMode = mode
	}
	return child
}

// WorkflowSummary builds the final user-facing summary for a completed run.
func WorkflowSummary(parent Run, replies []string) string {
	var builder strings.Builder
	switch parent.WorkMode {
	case WorkModeLoop:
		builder.WriteString("目标模式已完成。")
	default:
		builder.WriteString("工作流已完成。")
	}
	if strings.TrimSpace(parent.Objective) != "" {
		builder.WriteString("\n\n目标：")
		builder.WriteString(parent.Objective)
	}
	if len(parent.ChildRunIDs) > 0 {
		fmt.Fprintf(&builder, "\n\n子运行：%d 个", len(parent.ChildRunIDs))
	}
	if len(replies) > 0 {
		builder.WriteString("\n\n结果摘要：")
		for _, reply := range replies {
			reply = strings.TrimSpace(reply)
			if reply == "" {
				continue
			}
			builder.WriteString("\n- ")
			if len([]rune(reply)) > 180 {
				reply = string([]rune(reply)[:180]) + "..."
			}
			builder.WriteString(reply)
		}
	}
	return builder.String()
}

// IsWorkflowBlockingStatus reports whether a run status blocks workflow
// progression and needs user or child-run intervention.
func IsWorkflowBlockingStatus(status string) bool {
	switch status {
	case RunStatusPending, RunStatusPendingInput, RunStatusFailed, RunStatusTimedOut, RunStatusCancelled, RunStatusDenied:
		return true
	default:
		return false
	}
}

// AppendUniqueString appends a trimmed value unless it is empty or present.
func AppendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

// WorkflowResponsePlanIndex maps a fallback response index to the plan index
// based on the child run iteration.
func WorkflowResponsePlanIndex(fallback int, child Run) int {
	if child.Iteration > 0 {
		return child.Iteration - 1
	}
	return fallback
}
