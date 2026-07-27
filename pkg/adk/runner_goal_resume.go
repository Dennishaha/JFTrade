package adk

import (
	"context"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

// resumeUserPausedGoalRun 在后台恢复用户暂停的目标 run。goroutine 由
// Runtime.goBackground 登记，Close 会等待它退出，因此不会出现 store 关闭后
// 仍在写库的情况。
func (r *Runtime) resumeUserPausedGoalRun(run Run) {
	r.goBackground(func(ctx context.Context) {
		r.executeUserPausedGoalResume(ctx, run)
	})
}

func (r *Runtime) executeUserPausedGoalResume(ctx context.Context, run Run) {
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, runTimeoutForRun(run))
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
	if _, err := executor.resumeADKGoalWorkflow(leaseCtx, session, agent, run); err != nil {
		_, persistErr := executor.failParent(leaseCtx, run, err)
		besteffort.LogError(persistErr)
	}
}
