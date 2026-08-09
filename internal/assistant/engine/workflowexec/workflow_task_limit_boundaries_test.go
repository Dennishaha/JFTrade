package workflowexec

import (
	"context"
	"fmt"
	"strings"
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestWorkflowExecutorRejectsRuntimeTaskOverflow(t *testing.T) {
	runtime := newTestRuntime(t)
	executor := (&WorkflowExecutor{runtime: runtime})
	parentRun := mustSaveRun(t, runtime, Run{
		ID:             "workflow-helper-parent",
		SessionID:      "session",
		AgentID:        "agent",
		Status:         RunStatusRunning,
		WorkMode:       WorkModeLoop,
		WorkflowStatus: workflowStatusRunning,
		Objective:      "目标",
		CreatedAt:      jfadkmodel.NowString(),
		UpdatedAt:      jfadkmodel.NowString(),
	})
	for i := range maxRuntimeWorkflowTasks {
		if _, err := runtime.Store().SaveTask(context.Background(), TaskWriteRequest{
			ID:           fmt.Sprintf("runtime-task-%d", i),
			Title:        fmt.Sprintf("task-%d", i),
			Status:       "TODO",
			AgentID:      parentRun.AgentID,
			RunID:        parentRun.ID,
			Order:        i + 1,
			PlanSource:   workflowPlanSourceRuntime,
			WorkflowMode: parentRun.WorkMode,
			Objective:    parentRun.Objective,
		}); err != nil {
			t.Fatalf("SaveTask runtime %d: %v", i, err)
		}
	}
	if _, err := executor.AddRuntimeWorkflowTask(context.Background(), parentRun, Task{}, workflowRuntimeTaskRequest{Title: "overflow"}); err == nil || !strings.Contains(err.Error(), "limit reached") {
		t.Fatalf("AddRuntimeWorkflowTask limit err = %v", err)
	}
}
