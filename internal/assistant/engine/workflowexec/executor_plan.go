package workflowexec

import (
	"context"
	"fmt"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

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

func errADKMissingFinalReply() error {
	return fmt.Errorf("工具调用完成后模型未返回最终回复")
}

func (e *WorkflowExecutor) WorkflowTasks(ctx context.Context, parent Run, known []Task) ([]Task, error) {
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
		task, ok, err := e.runtime.WorkflowStore().Task(ctx, state.TaskID)
		if err != nil {
			return nil, err
		}
		if ok {
			byID[task.ID] = task
		}
	}
	parentTasks, _, err := e.runtime.WorkflowStore().ListTasksPage(ctx, "", "", parent.ID, 1000, 0)
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
	jfadkmodel.SortWorkflowTasks(tasks)
	return tasks, nil
}

func (e *WorkflowExecutor) AddRuntimeWorkflowTask(ctx context.Context, parent Run, current Task, req workflowRuntimeTaskRequest) (Task, error) {
	tasks, err := e.WorkflowTasks(ctx, parent, nil)
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
		message = jfadkmodel.DefaultString(description, title)
	}
	dependsOn := jfadkmodel.NormalizeStringSlice(req.DependsOn)
	for _, dep := range dependsOn {
		if !taskIDs[dep] {
			return Task{}, fmt.Errorf("runtime task dependency not found: %s", dep)
		}
	}
	nextRuntime := runtimeCount + 1
	task, err := e.runtime.WorkflowStore().SaveTask(ctx, TaskWriteRequest{
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
	if jfadkmodel.WorkflowTasksHaveCycle(tasks) {
		jftradeErr3 := e.runtime.WorkflowStore().DeleteTask(ctx, task.ID)
		besteffort.LogError(jftradeErr3)
		return Task{}, fmt.Errorf("runtime task dependencies contain a cycle")
	}
	return task, nil
}
