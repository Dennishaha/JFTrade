package adk

import (
	"context"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

// AssistantExecutionResult is the exported assistant turn result.
type AssistantExecutionResult = jfadkmodel.AssistantExecutionResult

// WorkflowExecution is the injectable workflow execution contract.
type WorkflowExecution = jfadkmodel.WorkflowExecution

// SetWorkflowExecutor wires the injected workflow execution implementation.
func (r *Runtime) SetWorkflowExecutor(executor WorkflowExecution) {
	if r == nil {
		return
	}
	r.executor = executor
	r.adkMu.Lock()
	pending := r.startupReconcile
	r.startupReconcile = false
	r.adkMu.Unlock()
	if pending {
		besteffort.LogError(r.reconcileStaleRuns(context.Background()))
	}
}

// RegisterWorkflowExecution tracks a live workflow execution handle so resume
// and reconcile passes can continue the same ADK execution by run ID.
func (r *Runtime) RegisterWorkflowExecution(parentID string, childRunIDs []string, execution WorkflowExecutionHandle) {
	if r == nil || execution == nil {
		return
	}
	execution.DetachDeltaSink()
	concrete, ok := execution.(*googleADKExecution)
	if !ok {
		return
	}
	r.adkMu.Lock()
	defer r.adkMu.Unlock()
	if r.adkRuns == nil {
		r.adkRuns = map[string]*googleADKExecution{}
	}
	r.adkRuns[parentID] = concrete
	for _, childID := range childRunIDs {
		r.adkRuns[childID] = concrete
	}
}

// WithWorkflowChildLock serializes workflow child execution with the runtime's
// child-run critical section.
func (r *Runtime) WithWorkflowChildLock(ctx context.Context, fn func() error) error {
	if r == nil {
		if fn != nil {
			return fn()
		}
		return nil
	}
	r.workflowChildMu.Lock()
	defer r.workflowChildMu.Unlock()
	if fn == nil {
		return nil
	}
	return fn()
}
