package adk

import (
	"errors"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

const (
	workflowStatusRunning  = jfadkmodel.WorkflowStatusRunning
	workflowStatusPaused   = jfadkmodel.WorkflowStatusPaused
	workflowStatusComplete = jfadkmodel.WorkflowStatusComplete
	workflowStatusFailed   = jfadkmodel.WorkflowStatusFailed

	workflowPlanSourcePlanner = jfadkmodel.WorkflowPlanSourcePlanner
	workflowPlanSourceRuntime = jfadkmodel.WorkflowPlanSourceRuntime
	workflowPlanSourceCanvas  = jfadkmodel.WorkflowPlanSourceCanvas

	workflowTaskExecutorSelf  = jfadkmodel.WorkflowTaskExecutorSelf
	workflowTaskExecutorChild = jfadkmodel.WorkflowTaskExecutorChild

	workflowTasksListTool    = jfadkmodel.WorkflowTasksListTool
	workflowTaskAddTool      = jfadkmodel.WorkflowTaskAddTool
	workflowTaskClaimTool    = jfadkmodel.WorkflowTaskClaimTool
	workflowTaskCompleteTool = jfadkmodel.WorkflowTaskCompleteTool
	workflowTaskBlockTool    = jfadkmodel.WorkflowTaskBlockTool
	workflowTaskDelegateTool = jfadkmodel.WorkflowTaskDelegateTool
	workflowModelsListTool   = jfadkmodel.WorkflowModelsListTool

	workflowGoalCompleteTool = jfadkmodel.WorkflowGoalCompleteTool
	workflowGoalContinueTool = jfadkmodel.WorkflowGoalContinueTool
)

// WorkflowRequest is the exported workflow execution contract input.
type WorkflowRequest = workflowRequest

type workflowRequest = jfadkmodel.WorkflowRequest

// WorkflowStep is the exported planner step projection.
type WorkflowStep = jfadkmodel.WorkflowStep

type workflowStep = WorkflowStep

// WorkflowGoalDecision is the exported goal-loop decision state.
type WorkflowGoalDecision = jfadkmodel.WorkflowGoalDecision

type workflowGoalDecision = WorkflowGoalDecision

// workflowExecutor returns the workflow execution implementation injected by
// the composition layer. Engine-root orchestration never constructs its own
// executor; assembly wires internal/assistant/engine/workflowruntime.
func (r *Runtime) workflowExecutor() (WorkflowExecution, error) {
	if r == nil || r.executor == nil {
		return nil, errors.New("adk workflow executor is not wired")
	}
	return r.executor, nil
}
