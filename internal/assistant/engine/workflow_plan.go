package adk

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

func workflowPlanFromTasks(tasks []Task, existing []WorkflowStepState) []WorkflowStepState {
	existingByTaskID := make(map[string]WorkflowStepState, len(existing))
	for _, state := range existing {
		if strings.TrimSpace(state.TaskID) != "" {
			existingByTaskID[state.TaskID] = state
		}
	}
	ordered := append([]Task(nil), tasks...)
	sortWorkflowTasks(ordered)
	plan := make([]WorkflowStepState, 0, len(ordered))
	for index, task := range ordered {
		prior := existingByTaskID[task.ID]
		state := WorkflowStepState{
			TaskID:              task.ID,
			Title:               task.Title,
			Description:         task.Description,
			Message:             task.Message,
			Status:              defaultString(task.Status, "TODO"),
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
		state.Routes = normalizeStringSlice(prior.Routes)
		state.OutputSummary = prior.OutputSummary
		plan = append(plan, state)
	}
	return plan
}

func isDirectWorkflowChild(parent Run, child Run) bool {
	return strings.TrimSpace(parent.ID) != "" &&
		strings.TrimSpace(child.ID) != "" &&
		child.ID != parent.ID &&
		strings.TrimSpace(child.ParentRunID) == parent.ID
}

func (e *WorkflowExecutor) workflowTasks(ctx context.Context, parent Run, known []Task) ([]Task, error) {
	byID := make(map[string]Task, len(known)+len(parent.WorkflowPlan))
	for _, task := range known {
		if strings.TrimSpace(task.ID) != "" {
			byID[task.ID] = task
		}
	}
	for _, state := range parent.WorkflowPlan {
		if strings.TrimSpace(state.TaskID) == "" {
			continue
		}
		task, ok, err := e.runtime.store.Task(ctx, state.TaskID)
		if err != nil {
			return nil, err
		}
		if ok {
			byID[task.ID] = task
		}
	}
	parentTasks, _, err := e.runtime.store.ListTasksPage(ctx, "", "", parent.ID, 1000, 0)
	if err != nil {
		return nil, err
	}
	for _, task := range parentTasks {
		byID[task.ID] = task
	}
	tasks := make([]Task, 0, len(byID))
	for _, task := range byID {
		tasks = append(tasks, task)
	}
	sortWorkflowTasks(tasks)
	return tasks, nil
}

func workflowTasksComplete(tasks []Task) bool {
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

func firstTerminalWorkflowTask(tasks []Task) (Task, bool) {
	for _, task := range tasks {
		if task.Status == "BLOCKED" || task.Status == "CANCELLED" {
			return task, true
		}
	}
	return Task{}, false
}

func executableWorkflowTasks(tasks []Task, _ string) []Task {
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
	sortWorkflowTasks(ready)
	if len(ready) > 1 {
		return ready[:1]
	}
	return ready
}

func workflowStepFromTask(task Task) workflowStep {
	message := strings.TrimSpace(task.Message)
	if message == "" {
		message = defaultString(task.Description, task.Title)
	}
	return workflowStep{
		Order:               task.Order,
		DependencyID:        task.PlannerStepID,
		Title:               task.Title,
		Description:         workflowDescriptionWithoutAgentRole(task.Description),
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

func workflowTaskIteration(task Task) int {
	if task.Order > 0 {
		return task.Order
	}
	return 1
}

func workflowPlanIndexForTask(plan []WorkflowStepState, taskID string) int {
	for index, state := range plan {
		if state.TaskID == taskID {
			return index
		}
	}
	return -1
}

func workflowSelfTaskSummary(task Task) string {
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

type workflowRuntimeTaskRequest struct {
	Title               string
	Message             string
	Description         string
	DependsOn           []string
	AgentRole           string
	ModeHint            string
	ChildAgentID        string
	ChildProviderID     string
	ChildModel          string
	ChildPermissionMode string
}

func (e *WorkflowExecutor) addRuntimeWorkflowTask(ctx context.Context, parent Run, current Task, req workflowRuntimeTaskRequest) (Task, error) {
	tasks, err := e.workflowTasks(ctx, parent, nil)
	if err != nil {
		return Task{}, err
	}
	runtimeCount := 0
	maxOrder := 0
	taskIDs := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		taskIDs[task.ID] = true
		if task.PlanSource == workflowPlanSourceRuntime {
			runtimeCount++
		}
		if task.Order > maxOrder {
			maxOrder = task.Order
		}
	}
	if runtimeCount >= maxRuntimeWorkflowTasks {
		return Task{}, fmt.Errorf("runtime workflow task limit reached")
	}
	title := strings.TrimSpace(req.Title)
	message := strings.TrimSpace(req.Message)
	description := strings.TrimSpace(req.Description)
	if title == "" {
		title = message
	}
	if title == "" {
		return Task{}, fmt.Errorf("runtime task title is required")
	}
	if message == "" {
		message = defaultString(description, title)
	}
	dependsOn := normalizeStringSlice(req.DependsOn)
	for _, dep := range dependsOn {
		if !taskIDs[dep] {
			return Task{}, fmt.Errorf("runtime task dependency not found: %s", dep)
		}
	}
	nextRuntime := runtimeCount + 1
	task, err := e.runtime.store.SaveTask(ctx, TaskWriteRequest{
		Title:               title,
		Description:         description,
		Message:             message,
		Status:              "TODO",
		AgentID:             parent.AgentID,
		RunID:               parent.ID,
		DependsOn:           dependsOn,
		Order:               maxOrder + 1,
		ModeHint:            req.ModeHint,
		AgentRole:           req.AgentRole,
		ChildAgentID:        req.ChildAgentID,
		ChildProviderID:     req.ChildProviderID,
		ChildModel:          req.ChildModel,
		ChildPermissionMode: req.ChildPermissionMode,
		PlannerStepID:       fmt.Sprintf("runtime-%d", nextRuntime),
		PlanSource:          workflowPlanSourceRuntime,
		WorkflowMode:        parent.WorkMode,
		Objective:           parent.Objective,
	})
	if err != nil {
		return Task{}, err
	}
	tasks = append(tasks, task)
	if workflowTasksHaveCycle(tasks) {
		jftradeErr3 := e.runtime.store.DeleteTask(ctx, task.ID)
		besteffort.LogError(jftradeErr3)
		return Task{}, fmt.Errorf("runtime task dependencies contain a cycle")
	}
	return task, nil
}

func workflowTasksHaveCycle(tasks []Task) bool {
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

func sortWorkflowTasks(tasks []Task) {
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

func workflowStepDescription(step workflowStep) string {
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

func workflowDescriptionWithoutAgentRole(description string) string {
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

func pauseParentForChild(parent Run, child Run, cursor int) Run {
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = workflowEngineForMode(parent.WorkMode)
	}
	parent.Status = child.Status
	parent.Message = child.Message
	parent.PendingApprovals = pendingApprovalsOnly(child.PendingApprovals)
	parent.InputRequest = normalizeInputRequest(child.InputRequest)
	parent.WorkflowStatus = workflowStatusPaused
	parent.WorkflowCursor = cursor
	parent = updateWorkflowPlanForChildAt(parent, child, cursor)
	return parent
}

func childRunIDs(children []Run) []string {
	ids := make([]string, 0, len(children))
	for _, child := range children {
		ids = appendUniqueString(ids, child.ID)
	}
	return ids
}

func approvalsForRun(approvals []Approval, runID string) []Approval {
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
	return pendingApprovalsOnly(filtered)
}

func workflowPendingReply(parent Run) string {
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

func updateWorkflowPlanForChild(parent Run, child Run) Run {
	return updateWorkflowPlanForChildAt(parent, child, -1)
}

func updateWorkflowPlanForChildAt(parent Run, child Run, stepIndex int) Run {
	if len(parent.WorkflowPlan) == 0 || strings.TrimSpace(child.ID) == "" {
		return parent
	}
	if stepIndex >= 0 && stepIndex < len(parent.WorkflowPlan) {
		applyWorkflowChildState(&parent.WorkflowPlan[stepIndex], child)
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
		applyWorkflowChildState(step, child)
		parent.WorkflowCursor = index
		break
	}
	return parent
}

func applyWorkflowChildState(step *WorkflowStepState, child Run) {
	if step == nil {
		return
	}
	step.ChildRunID = child.ID
	step.Executor = workflowTaskExecutorChild
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

func workflowChildAgentForStep(agent Agent, step workflowStep) Agent {
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

func (r *Runtime) workflowChildAgentForStep(ctx context.Context, agent Agent, step workflowStep) (Agent, error) {
	child := agent
	if agentID := strings.TrimSpace(step.ChildAgentID); agentID != "" && agentID != agent.ID {
		resolved, err := r.resolveAgentDefinition(ctx, agentID)
		if err != nil {
			return Agent{}, err
		}
		child = resolved
	}
	child = workflowChildAgentForStep(child, step)
	child.WorkMode = WorkModeChat
	if strings.TrimSpace(child.PermissionMode) == "" {
		child.PermissionMode = agent.PermissionMode
	}
	return r.prepareAgent(ctx, child)
}

func workflowSummary(parent Run, replies []string) string {
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

func errADKMissingFinalReply() error {
	return fmt.Errorf("工具调用完成后模型未返回最终回复")
}

func isWorkflowBlockingStatus(status string) bool {
	switch status {
	case RunStatusPending, RunStatusPendingInput, RunStatusFailed, RunStatusTimedOut, RunStatusCancelled, RunStatusDenied:
		return true
	default:
		return false
	}
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}
