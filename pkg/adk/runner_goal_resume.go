package adk

import (
	"context"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

func (r *Runtime) resumeUserPausedGoalRun(ctx context.Context, run Run) {
	go func() {
		timeoutCtx, timeoutCancel := context.WithTimeout(context.WithoutCancel(ctx), runTimeoutForRun(run))
		leaseCtx, cancel, waitForLease, leaseErr := r.beginRunExecutionLease(timeoutCtx, run.ID)
		if leaseErr != nil {
			timeoutCancel()
			if isRunLeaseHeld(leaseErr) {
				return
			}
			executor := &WorkflowExecutor{runtime: r}
			_, persistErr := executor.failParent(context.WithoutCancel(ctx), run, leaseErr)
			besteffort.LogError(persistErr)
			return
		}
		defer func() {
			cancel()
			waitForLease()
			timeoutCancel()
		}()
		r.activeMu.Lock()
		r.activeRuns[run.ID] = cancel
		r.activeMu.Unlock()
		defer func() {
			r.activeMu.Lock()
			delete(r.activeRuns, run.ID)
			r.activeMu.Unlock()
		}()
		session, agent, err := r.workflowResumeContext(leaseCtx, run)
		executor := &WorkflowExecutor{runtime: r}
		if err != nil {
			_, persistErr := executor.failParent(leaseCtx, run, err)
			besteffort.LogError(persistErr)
			return
		}
		updated, err := executor.resumeADKGoalWorkflow(leaseCtx, session, agent, run)
		if err != nil {
			_, persistErr := executor.failParent(leaseCtx, run, err)
			besteffort.LogError(persistErr)
			return
		}
		_ = updated
	}()
}
