package adk

import (
	"context"
	"errors"
	"testing"
	"time"
)

type leaseContextValueKey struct{}

func TestRunExecutionLeaseContextAndReuseBoundaries(t *testing.T) {
	if got := withoutRunExecutionLease(nil); got == nil { //nolint:staticcheck // Exercise the documented nil-context fallback.
		t.Fatal("nil context did not become a background context")
	}
	plain := context.WithValue(t.Context(), leaseContextValueKey{}, "retained")
	if got := withoutRunExecutionLease(plain); got != plain {
		t.Fatal("context without a lease was unnecessarily wrapped")
	}
	if lease, ok := runExecutionLeaseFromContext(nil); ok || lease.RunID != "" { //nolint:staticcheck // Exercise nil lookup directly.
		t.Fatalf("nil context lease = %#v, %v", lease, ok)
	}
	emptyLeaseCtx := context.WithValue(plain, runExecutionLeaseContextKey{}, RunLease{})
	if _, ok := runExecutionLeaseFromContext(emptyLeaseCtx); ok {
		t.Fatal("empty run lease was accepted")
	}
	leased := context.WithValue(plain, runExecutionLeaseContextKey{}, RunLease{RunID: "run-context", FencingToken: 1})
	stripped := withoutRunExecutionLease(leased)
	if _, ok := runExecutionLeaseFromContext(stripped); ok || stripped.Value(leaseContextValueKey{}) != "retained" {
		t.Fatalf("stripped context leaked lease or lost parent value")
	}

	var nilRuntime *Runtime
	if _, _, _, err := nilRuntime.beginRunExecutionLease(t.Context(), "run"); err == nil {
		t.Fatal("nil runtime began a lease")
	}
	if _, _, err := nilRuntime.beginOrReuseRunExecutionLease(t.Context(), "run"); err == nil {
		t.Fatal("nil runtime reused a lease")
	}
	if lease, ok := nilRuntime.currentRunLease("run"); ok || lease.RunID != "" {
		t.Fatalf("nil runtime current lease = %#v, %v", lease, ok)
	}
	withoutStore := &Runtime{}
	leaseCtx, cancel, wait, err := withoutStore.beginRunExecutionLease(t.Context(), "run-no-store")
	if err != nil || leaseCtx == nil {
		t.Fatalf("storeless lease context = %v, err=%v", leaseCtx, err)
	}
	cancel()
	wait()

	store := newExecutionClaimTestStore(t)
	runtime := NewRuntime(store, NewToolRegistry())
	t.Cleanup(runtime.backgroundCancel)
	leaseCtx, cancel, wait, err = runtime.beginRunExecutionLease(t.Context(), "run-reuse")
	if err != nil {
		t.Fatalf("begin run lease: %v", err)
	}
	reusedCtx, finishReused, err := runtime.beginOrReuseRunExecutionLease(leaseCtx, "run-reuse")
	if err != nil || reusedCtx != leaseCtx {
		t.Fatalf("reuse owning context = %v, err=%v", reusedCtx, err)
	}
	finishReused()
	adoptedCtx, finishAdopted, err := runtime.beginOrReuseRunExecutionLease(t.Context(), "run-reuse")
	adoptedLease, adopted := runExecutionLeaseFromContext(adoptedCtx)
	if err != nil || !adopted || adoptedLease.RunID != "run-reuse" {
		t.Fatalf("adopt active lease = %#v, %v, err=%v", adoptedLease, adopted, err)
	}
	finishAdopted()
	if activeCtx, err := runtime.activeRunExecutionContext(leaseCtx, "run-reuse"); err != nil || activeCtx != leaseCtx {
		t.Fatalf("active owning context = %v, err=%v", activeCtx, err)
	}
	if activeCtx, err := runtime.activeRunExecutionContext(t.Context(), "run-reuse"); err != nil {
		t.Fatalf("adopt current active context: %v", err)
	} else if activeLease, ok := runExecutionLeaseFromContext(activeCtx); !ok || activeLease.RunID != "run-reuse" {
		t.Fatalf("active adopted lease = %#v, %v", activeLease, ok)
	}
	cancel()
	wait()
	if activeCtx, err := runtime.activeRunExecutionContext(t.Context(), "run-reuse"); err != nil || activeCtx == nil {
		t.Fatalf("plain context without active lease = %v, err=%v", activeCtx, err)
	}
	if _, err := runtime.activeRunExecutionContext(leaseCtx, "run-reuse"); !errors.Is(err, ErrRunLeaseLost) {
		t.Fatalf("stale leased context error = %v", err)
	}

	foreignLease, err := store.ClaimRunLease(t.Context(), "run-foreign-reuse", "executor-other", time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatalf("claim foreign lease: %v", err)
	}
	if _, _, err := runtime.beginOrReuseRunExecutionLease(t.Context(), foreignLease.RunID); !errors.Is(err, ErrRunLeaseHeld) {
		t.Fatalf("foreign lease reuse error = %v", err)
	}
	if foreign, err := runtime.freshForeignRunLease(t.Context(), foreignLease.RunID, time.Now().UTC()); err != nil || !foreign {
		t.Fatalf("fresh foreign lease = %v, err=%v", foreign, err)
	}
	if foreign, err := runtime.freshForeignRunLease(t.Context(), "missing", time.Now().UTC()); err != nil || foreign {
		t.Fatalf("missing foreign lease = %v, err=%v", foreign, err)
	}
	if foreign, err := nilRuntime.freshForeignRunLease(t.Context(), "run", time.Now().UTC()); err != nil || foreign {
		t.Fatalf("nil runtime foreign lease = %v, err=%v", foreign, err)
	}
	if !isRunLeaseHeld(ErrRunLeaseHeld) || isRunLeaseHeld(errors.New("other")) {
		t.Fatal("run lease held classification mismatch")
	}
}

func TestRunExecutionLeaseHeartbeatFailureCancelsOwner(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	runtime := &Runtime{
		store: store, executorID: "heartbeat-failure-owner",
		runLeaseTTL: 100 * time.Millisecond, runLeaseHeartbeat: 5 * time.Millisecond,
	}
	leaseCtx, cancel, wait, err := runtime.beginRunExecutionLease(t.Context(), "run-heartbeat-failure")
	if err != nil {
		t.Fatalf("begin heartbeat lease: %v", err)
	}
	if err := store.Close(); err != nil {
		cancel()
		wait()
		t.Fatalf("close store: %v", err)
	}
	select {
	case <-leaseCtx.Done():
	case <-time.After(time.Second):
		cancel()
		wait()
		t.Fatal("heartbeat storage failure did not cancel lease owner")
	}
	wait()
}

func TestRefreshRunExecutionLeaseRejectsExpiredLeaseBeforeStoreWrite(t *testing.T) {
	runtime := &Runtime{}
	expired := RunLease{
		RunID:     "run-expired-heartbeat",
		ExpiresAt: time.Now().UTC().Add(-time.Millisecond),
	}
	if _, err := runtime.refreshRunExecutionLease(expired, time.Second); !errors.Is(err, ErrRunLeaseLost) {
		t.Fatalf("expired lease heartbeat error = %v, want ErrRunLeaseLost", err)
	}
}

func TestRefreshRunExecutionLeaseUsesRemainingTTLForNearExpiryLease(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	runtime := &Runtime{store: store}
	lease, err := store.ClaimRunLease(t.Context(), "run-near-expiry-heartbeat", "near-expiry-owner", time.Now().UTC(), 5*time.Second)
	if err != nil {
		t.Fatalf("claim lease: %v", err)
	}
	lease.ExpiresAt = time.Now().UTC().Add(100 * time.Millisecond)
	refreshed, err := runtime.refreshRunExecutionLease(lease, time.Second)
	if err != nil {
		t.Fatalf("refresh near-expiry lease: %v", err)
	}
	if got := refreshed.ExpiresAt.Sub(refreshed.HeartbeatAt); got != time.Second {
		t.Fatalf("refreshed lease TTL = %s, want %s", got, time.Second)
	}
}

func TestRunExecutionLeaseUsesSafeDefaults(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	runtime := &Runtime{store: store, executorID: "default-lease-owner"}
	leaseCtx, cancel, wait, err := runtime.beginRunExecutionLease(t.Context(), "run-default-lease")
	if err != nil {
		t.Fatalf("begin default lease: %v", err)
	}
	lease, ok := runExecutionLeaseFromContext(leaseCtx)
	if !ok || lease.ExpiresAt.Sub(lease.HeartbeatAt) != defaultADKRunLeaseTTL {
		t.Fatalf("default lease = %#v, ok=%v", lease, ok)
	}
	cancel()
	wait()
}

func TestRuntimeCloseCancelsAndWaitsForInFlightRunLease(t *testing.T) {
	runtimeStore, observerStore := newExecutionClaimTestStores(t)
	runtime := NewRuntime(runtimeStore, NewToolRegistry())
	runtime.runLeaseTTL = time.Hour
	runtime.runLeaseHeartbeat = time.Minute
	leaseCtx, cancel, wait, err := runtime.beginRunExecutionLease(context.Background(), "run-close-in-flight")
	if err != nil {
		t.Fatalf("begin run lease: %v", err)
	}
	blocker, err := observerStore.db.BeginWrite(t.Context(), nil)
	if err != nil {
		cancel()
		wait()
		t.Fatalf("hold lease release write queue: %v", err)
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- runtime.Close()
	}()

	canceledBeforeTimeout := false
	select {
	case <-leaseCtx.Done():
		canceledBeforeTimeout = true
	case <-time.After(time.Second):
	}
	closeReturnedWhileReleaseBlocked := false
	var closeErr error
	select {
	case closeErr = <-closeDone:
		closeReturnedWhileReleaseBlocked = true
	default:
	}
	if err := blocker.Rollback(); err != nil {
		t.Fatalf("release lease write queue: %v", err)
	}
	cancel()
	wait()
	if !closeReturnedWhileReleaseBlocked {
		closeErr = <-closeDone
	}

	if !canceledBeforeTimeout {
		t.Fatal("Close did not cancel the in-flight run lease")
	}
	if closeReturnedWhileReleaseBlocked {
		t.Fatal("Close returned before the run lease release completed")
	}
	if closeErr != nil {
		t.Fatalf("close runtime: %v", closeErr)
	}
	stored, ok, err := observerStore.RunLease(t.Context(), "run-close-in-flight")
	if err != nil || !ok || stored.OwnerID != "" {
		t.Fatalf("released lease = %#v, ok=%v, err=%v", stored, ok, err)
	}
}

func TestRuntimeCloseRejectsRunLeaseWorkAfterClosingStarts(t *testing.T) {
	runtimeStore, observerStore := newExecutionClaimTestStores(t)
	runtime := NewRuntime(runtimeStore, NewToolRegistry())
	backgroundCanceled := make(chan struct{})
	releaseBackground := make(chan struct{})
	if !runtime.goBackground(func(ctx context.Context) {
		<-ctx.Done()
		close(backgroundCanceled)
		<-releaseBackground
	}) {
		t.Fatal("runtime rejected background close barrier")
	}
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- runtime.Close()
	}()
	select {
	case <-backgroundCanceled:
	case <-time.After(time.Second):
		close(releaseBackground)
		t.Fatal("Close did not enter closing state")
	}

	_, cancel, wait, admissionErr := runtime.beginRunExecutionLease(
		context.Background(),
		"run-after-closing",
	)
	if cancel != nil {
		cancel()
	}
	if wait != nil {
		wait()
	}
	close(releaseBackground)
	closeErr := <-closeDone
	_, exists, lookupErr := observerStore.RunLease(t.Context(), "run-after-closing")
	_, _, _, afterCloseErr := runtime.beginRunExecutionLease(context.Background(), "run-after-close")

	if !errors.Is(admissionErr, errRuntimeClosing) {
		t.Fatalf("lease admission while closing error = %v, want errRuntimeClosing", admissionErr)
	}
	if closeErr != nil {
		t.Fatalf("close runtime: %v", closeErr)
	}
	if lookupErr != nil || exists {
		t.Fatalf("closing lease admission persisted a row: exists=%v err=%v", exists, lookupErr)
	}
	if !errors.Is(afterCloseErr, errRuntimeClosing) {
		t.Fatalf("lease admission after Close error = %v, want errRuntimeClosing", afterCloseErr)
	}
}
