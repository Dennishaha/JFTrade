package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

const (
	defaultADKRunLeaseTTL       = 30 * time.Second
	defaultADKRunLeaseHeartbeat = 10 * time.Second
	defaultADKToolClaimTTL      = 30 * time.Second
)

type runExecutionLeaseContextKey struct{}

type contextWithoutRunExecutionLease struct {
	context.Context
}

func (ctx contextWithoutRunExecutionLease) Value(key any) any {
	if _, isLeaseKey := key.(runExecutionLeaseContextKey); isLeaseKey {
		return nil
	}
	return ctx.Context.Value(key)
}

func withoutRunExecutionLease(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	if _, ok := runExecutionLeaseFromContext(ctx); !ok {
		return ctx
	}
	return contextWithoutRunExecutionLease{Context: ctx}
}

func runExecutionLeaseFromContext(ctx context.Context) (RunLease, bool) {
	if ctx == nil {
		return RunLease{}, false
	}
	lease, ok := ctx.Value(runExecutionLeaseContextKey{}).(RunLease)
	return lease, ok && strings.TrimSpace(lease.RunID) != ""
}

func (r *Runtime) beginRunExecutionLease(
	ctx context.Context,
	runID string,
) (context.Context, context.CancelFunc, func(), error) {
	runID = strings.TrimSpace(runID)
	if r == nil || runID == "" {
		return nil, nil, nil, fmt.Errorf("ADK run execution lease requires a runtime and run id")
	}
	leaseBaseCtx, cancel := context.WithCancel(ctx)
	if r.store == nil {
		return leaseBaseCtx, cancel, func() {}, nil
	}
	ttl := r.runLeaseTTL
	if ttl <= 0 {
		ttl = defaultADKRunLeaseTTL
	}
	heartbeat := r.runLeaseHeartbeat
	if heartbeat <= 0 || heartbeat >= ttl {
		heartbeat = min(defaultADKRunLeaseHeartbeat, ttl/3)
		if heartbeat <= 0 {
			heartbeat = time.Millisecond
		}
	}
	lease, err := r.store.ClaimRunLease(leaseBaseCtx, runID, r.executorID, time.Now().UTC(), ttl)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	leasedCtx := context.WithValue(leaseBaseCtx, runExecutionLeaseContextKey{}, lease)
	r.activeMu.Lock()
	if r.runLeases == nil {
		r.runLeases = make(map[string]RunLease)
	}
	r.runLeases[runID] = lease
	r.activeMu.Unlock()
	done := make(chan struct{})
	r.runLeaseWG.Go(func() {
		defer close(done)
		defer func() {
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), min(ttl, 5*time.Second))
			defer releaseCancel()
			besteffort.LogError(r.store.ReleaseRunLease(releaseCtx, lease))
			r.activeMu.Lock()
			if current, ok := r.runLeases[runID]; ok && current.FencingToken == lease.FencingToken {
				delete(r.runLeases, runID)
			}
			r.activeMu.Unlock()
		}()
		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-leasedCtx.Done():
				return
			case <-ticker.C:
				updated, heartbeatErr := r.refreshRunExecutionLease(lease, ttl)
				if heartbeatErr != nil {
					cancel()
					return
				}
				lease = updated
				r.activeMu.Lock()
				if current, ok := r.runLeases[runID]; ok && current.FencingToken == lease.FencingToken {
					r.runLeases[runID] = lease
				}
				r.activeMu.Unlock()
			}
		}
	})
	wait := func() {
		cancel()
		<-done
	}
	return leasedCtx, cancel, wait, nil
}

func (r *Runtime) refreshRunExecutionLease(lease RunLease, ttl time.Duration) (RunLease, error) {
	now := time.Now().UTC()
	remaining := lease.ExpiresAt.Sub(now)
	if remaining <= 0 {
		return RunLease{}, ErrRunLeaseLost
	}
	heartbeatTimeout := min(ttl/3, 5*time.Second)
	if remaining < heartbeatTimeout {
		heartbeatTimeout = remaining
	}
	heartbeatCtx, heartbeatCancel := context.WithTimeout(context.Background(), heartbeatTimeout)
	defer heartbeatCancel()
	return r.store.HeartbeatRunLease(heartbeatCtx, lease, now, ttl)
}

func (r *Runtime) currentRunLease(runID string) (RunLease, bool) {
	if r == nil {
		return RunLease{}, false
	}
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	lease, ok := r.runLeases[strings.TrimSpace(runID)]
	return lease, ok
}

// beginOrReuseRunExecutionLease returns a context fenced for runID. Workflow
// orchestration can hold more than one run lease at a time, so a child context
// must not be reused to persist its parent (or the inverse).
func (r *Runtime) beginOrReuseRunExecutionLease(
	ctx context.Context,
	runID string,
) (context.Context, func(), error) {
	runID = strings.TrimSpace(runID)
	if r == nil || runID == "" {
		return nil, nil, fmt.Errorf("ADK run execution lease requires a runtime and run id")
	}
	contextLease, contextOwnsRun := runExecutionLeaseFromContext(ctx)
	currentLease, currentLeaseActive := r.currentRunLease(runID)
	if contextOwnsRun && ctx.Err() == nil && contextLease.RunID == runID &&
		currentLeaseActive && currentLease.FencingToken == contextLease.FencingToken {
		return ctx, func() {}, nil
	}
	if currentLeaseActive {
		return context.WithValue(ctx, runExecutionLeaseContextKey{}, currentLease), func() {}, nil
	}
	leaseCtx, cancel, waitForLease, err := r.beginRunExecutionLease(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	finish := func() {
		cancel()
		waitForLease()
	}
	return leaseCtx, finish, nil
}

// activeRunExecutionContext switches an orchestration context to a run lease
// that is already held by this Runtime. Calls made directly by maintenance and
// tests without any execution lease remain unfenced; an executor carrying a
// different run lease must never silently write the target run.
func (r *Runtime) activeRunExecutionContext(ctx context.Context, runID string) (context.Context, error) {
	runID = strings.TrimSpace(runID)
	contextLease, contextHasLease := runExecutionLeaseFromContext(ctx)
	currentLease, currentLeaseActive := r.currentRunLease(runID)
	if currentLeaseActive {
		if contextHasLease && contextLease.RunID == runID && contextLease.FencingToken == currentLease.FencingToken {
			return ctx, nil
		}
		return context.WithValue(ctx, runExecutionLeaseContextKey{}, currentLease), nil
	}
	if contextHasLease {
		return nil, fmt.Errorf("%w: run %s has no active execution lease", ErrRunLeaseLost, runID)
	}
	return ctx, nil
}

func (r *Runtime) freshForeignRunLease(ctx context.Context, runID string, now time.Time) (bool, error) {
	if r == nil || r.store == nil {
		return false, nil
	}
	lease, ok, err := r.store.RunLease(ctx, runID)
	if err != nil || !ok || !lease.ExpiresAt.After(now.UTC()) {
		return false, err
	}
	return lease.OwnerID != r.executorID, nil
}

func isRunLeaseHeld(err error) bool {
	return errors.Is(err, ErrRunLeaseHeld)
}

func (r *Runtime) beginToolInvocationHeartbeat(
	ctx context.Context,
	ticket ToolInvocationTicket,
) (context.Context, func() error) {
	claimedCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	interval := defaultADKToolClaimTTL / 3
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-claimedCtx.Done():
				done <- nil
				return
			case now := <-ticker.C:
				heartbeatCtx, heartbeatCancel := context.WithTimeout(context.Background(), min(interval, 5*time.Second))
				err := r.store.HeartbeatToolInvocation(heartbeatCtx, ticket, now.UTC(), defaultADKToolClaimTTL)
				heartbeatCancel()
				if err != nil {
					cancel()
					done <- err
					return
				}
			}
		}
	}()
	stop := func() error {
		cancel()
		return <-done
	}
	return claimedCtx, stop
}
