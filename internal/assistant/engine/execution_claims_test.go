package adk

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	adkagent "google.golang.org/adk/v2/agent"
)

func newExecutionClaimTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(
		filepath.Join(dir, "adk.db"),
		filepath.Join(dir, "secrets", "adk.json"),
		filepath.Join(dir, "skills"),
	)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newExecutionClaimTestStores(t *testing.T) (*Store, *Store) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "adk.db")
	secretsPath := filepath.Join(dir, "secrets", "adk.json")
	skillsPath := filepath.Join(dir, "skills")
	first, err := NewStore(dbPath, secretsPath, skillsPath)
	if err != nil {
		t.Fatalf("NewStore first: %v", err)
	}
	second, err := NewStore(dbPath, secretsPath, skillsPath)
	if err != nil {
		_ = first.Close()
		t.Fatalf("NewStore second: %v", err)
	}
	t.Cleanup(func() {
		_ = second.Close()
		_ = first.Close()
	})
	return first, second
}

func TestGoogleADKToolUsesDurableInvocationKeyAndReplay(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	registry := NewToolRegistry()
	var calls atomic.Int32
	var observedKey string
	registry.Register(ToolDescriptor{
		Name: "test.durable_read", Description: "durable read", Permission: "read_internal",
	}, func(ctx context.Context, input map[string]any) (any, error) {
		calls.Add(1)
		key, ok := ToolInvocationIdempotencyKey(ctx)
		if !ok {
			return nil, errors.New("missing idempotency key")
		}
		observedKey = key
		return map[string]any{"value": input["value"], "key": key}, nil
	})
	runtime := NewRuntime(store, registry)
	leaseCtx, cancel, waitForLease, err := runtime.beginRunExecutionLease(t.Context(), "run-wrapper")
	if err != nil {
		t.Fatalf("beginRunExecutionLease: %v", err)
	}
	defer func() {
		cancel()
		waitForLease()
	}()
	execution := &googleADKExecution{
		runtime: runtime, runID: "run-wrapper",
		runIDByAgentName: map[string]string{"agent-test": "run-wrapper"},
	}
	registered, _ := registry.Get("test.durable_read")
	tool, err := newGoogleADKTool(registered.Descriptor, registered, execution)
	if err != nil {
		t.Fatalf("newGoogleADKTool: %v", err)
	}
	mock := adkagent.NewStrictContextMock(leaseCtx)
	ctx := googleADKToolTestContext{StrictContextMock: &mock}
	input := map[string]any{"value": "once"}
	first, err := tool.Run(ctx, input)
	if err != nil {
		t.Fatalf("first tool Run: %v", err)
	}
	second, err := tool.Run(ctx, input)
	if err != nil {
		t.Fatalf("replayed tool Run: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
	if observedKey == "" || !strings.Contains(observedKey, "run-wrapper") || first["key"] != second["key"] {
		t.Fatalf("stable keys: observed=%q first=%#v second=%#v", observedKey, first, second)
	}
}

func TestGoogleADKToolRejectsStaleContextAfterLeaseTurnover(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	registry := NewToolRegistry()
	var calls atomic.Int32
	registry.Register(ToolDescriptor{
		Name: "test.stale_read", Description: "stale read", Permission: "read_internal",
	}, func(context.Context, map[string]any) (any, error) {
		calls.Add(1)
		return map[string]any{"ok": true}, nil
	})
	runtime := NewRuntime(store, registry)
	oldLeaseCtx, oldCancel, waitForOldLease, err := runtime.beginRunExecutionLease(t.Context(), "run-stale-context")
	if err != nil {
		t.Fatalf("begin old run lease: %v", err)
	}
	oldMock := adkagent.NewStrictContextMock(oldLeaseCtx)
	oldToolCtx := googleADKToolTestContext{StrictContextMock: &oldMock}
	oldCancel()
	waitForOldLease()

	newLeaseCtx, newCancel, waitForNewLease, err := runtime.beginRunExecutionLease(t.Context(), "run-stale-context")
	if err != nil {
		t.Fatalf("begin replacement run lease: %v", err)
	}
	_ = newLeaseCtx
	defer func() {
		newCancel()
		waitForNewLease()
	}()
	execution := &googleADKExecution{
		runtime: runtime, runID: "run-stale-context",
		runIDByAgentName: map[string]string{"agent-test": "run-stale-context"},
	}
	registered, _ := registry.Get("test.stale_read")
	tool, err := newGoogleADKTool(registered.Descriptor, registered, execution)
	if err != nil {
		t.Fatalf("newGoogleADKTool: %v", err)
	}
	if _, err := tool.run(oldToolCtx, map[string]any{"value": "stale"}); !errors.Is(err, enginepersistence.ErrRunLeaseLost) {
		t.Fatalf("stale tool context err = %v, want ErrRunLeaseLost", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("stale context executed handler %d times, want 0", calls.Load())
	}
}

func TestGoogleADKKeyedToolFailsClosedWhenHandlerIgnoresKey(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	registry := NewToolRegistry()
	var calls atomic.Int32
	registry.Register(ToolDescriptor{
		Name: "test.key_ignored", Description: "key ignored", Permission: "write_internal",
		IdempotencyMode: ToolIdempotencyKeyed,
	}, func(context.Context, map[string]any) (any, error) {
		calls.Add(1)
		return map[string]any{"ok": true}, nil
	})
	runtime := NewRuntime(store, registry)
	leaseCtx, cancel, waitForLease, err := runtime.beginRunExecutionLease(t.Context(), "run-key-ignored")
	if err != nil {
		t.Fatalf("begin run lease: %v", err)
	}
	defer func() {
		cancel()
		waitForLease()
	}()
	execution := &googleADKExecution{
		runtime: runtime, runID: "run-key-ignored",
		runIDByAgentName: map[string]string{"agent-test": "run-key-ignored"},
	}
	registered, _ := registry.Get("test.key_ignored")
	tool, err := newGoogleADKTool(registered.Descriptor, registered, execution)
	if err != nil {
		t.Fatalf("newGoogleADKTool: %v", err)
	}
	mock := adkagent.NewStrictContextMock(leaseCtx)
	toolCtx := googleADKToolTestContext{StrictContextMock: &mock}
	for attempt := range 2 {
		if _, err := tool.run(toolCtx, map[string]any{"value": "once"}); !errors.Is(err, enginepersistence.ErrToolOutcomeUnknown) {
			t.Fatalf("attempt %d err = %v, want ErrToolOutcomeUnknown", attempt, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
}

func TestRuntimeReconciliationDoesNotStealFreshForeignLease(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	ctx := t.Context()
	started := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	run := Run{
		ID: "run-owned-elsewhere", SessionID: "session-foreign", AgentID: "agent-foreign",
		Status: RunStatusRunning, CreatedAt: started, StartedAt: started, UpdatedAt: started,
		MaxDurationMs: 1,
	}
	if err := store.SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	if _, err := store.ClaimRunLease(ctx, run.ID, "executor-other-process", time.Now().UTC(), time.Minute); err != nil {
		t.Fatalf("ClaimRunLease foreign: %v", err)
	}
	runtime := NewRuntime(store, NewToolRegistry())
	defer runtime.backgroundCancel()
	if err := runtime.ReconcileExpiredRuns(ctx); err != nil {
		t.Fatalf("ReconcileExpiredRuns: %v", err)
	}
	got, ok, err := store.Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run after reconcile: ok=%v err=%v", ok, err)
	}
	if got.Status != RunStatusRunning {
		t.Fatalf("foreign-owned run status = %s, want RUNNING", got.Status)
	}
}

func TestRuntimeRunLeaseHeartbeatPreventsPrematureTakeover(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	runtime := NewRuntime(store, NewToolRegistry())
	defer runtime.backgroundCancel()
	runtime.runLeaseTTL = 3 * time.Second
	runtime.runLeaseHeartbeat = 100 * time.Millisecond
	leaseCtx, cancel, waitForLease, err := runtime.beginRunExecutionLease(t.Context(), "run-heartbeat")
	if err != nil {
		t.Fatalf("beginRunExecutionLease: %v", err)
	}
	defer func() {
		cancel()
		waitForLease()
	}()
	initial, ok := runExecutionLeaseFromContext(leaseCtx)
	if !ok {
		t.Fatal("lease context did not retain its initial lease")
	}
	deadline := time.Now().Add(2 * time.Second)
	var renewed enginepersistence.RunLease
	for time.Now().Before(deadline) {
		current, active := runtime.currentRunLease(initial.RunID)
		if active && current.ExpiresAt.After(initial.ExpiresAt) {
			renewed = current
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !renewed.ExpiresAt.After(initial.ExpiresAt) {
		t.Fatalf("lease was not renewed before deadline: initial=%#v renewed=%#v", initial, renewed)
	}
	takeoverAt := initial.ExpiresAt.Add(time.Millisecond)
	if !renewed.ExpiresAt.After(takeoverAt) {
		t.Fatalf("renewed lease expires too early: renewed=%#v takeoverAt=%s", renewed, takeoverAt)
	}
	if _, err := store.ClaimRunLease(t.Context(), initial.RunID, "executor-other", takeoverAt, time.Second); !errors.Is(err, enginepersistence.ErrRunLeaseHeld) {
		t.Fatalf("takeover while heartbeat is active err = %v, want ErrRunLeaseHeld", err)
	}
	cancel()
	waitForLease()
	takenOver, err := store.ClaimRunLease(t.Context(), initial.RunID, "executor-other", time.Now().UTC(), time.Second)
	if err != nil {
		t.Fatalf("takeover after release: %v", err)
	}
	if takenOver.FencingToken <= 1 {
		t.Fatalf("takeover fencing token = %d, want a later generation", takenOver.FencingToken)
	}
}

func TestRunSaveIsFencedWithExecutionLeaseContext(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	ctx := t.Context()
	run := Run{
		ID: "run-save-fenced", SessionID: "session-save-fenced", AgentID: "agent-save-fenced",
		Status: RunStatusRunning, Message: "before takeover", CreatedAt: nowString(), UpdatedAt: nowString(),
	}
	if err := store.SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun initial: %v", err)
	}
	now := time.Now().UTC()
	stale, err := store.ClaimRunLease(ctx, run.ID, "executor-stale", now.Add(-2*time.Minute), time.Minute)
	if err != nil {
		t.Fatalf("ClaimRunLease stale generation: %v", err)
	}
	current, err := store.ClaimRunLease(ctx, run.ID, "executor-current", now, time.Minute)
	if err != nil {
		t.Fatalf("ClaimRunLease takeover: %v", err)
	}
	run.Message = "stale write"
	staleCtx := context.WithValue(ctx, runExecutionLeaseContextKey{}, stale)
	if err := store.SaveRun(staleCtx, run); !errors.Is(err, enginepersistence.ErrRunLeaseLost) {
		t.Fatalf("stale leased SaveRun err = %v, want ErrRunLeaseLost", err)
	}
	stored, ok, err := store.Run(ctx, run.ID)
	if err != nil || !ok || stored.Message != "before takeover" {
		t.Fatalf("run after stale write = %+v, ok=%v err=%v", stored, ok, err)
	}
	run.Message = "current write"
	currentCtx := context.WithValue(ctx, runExecutionLeaseContextKey{}, current)
	otherRun := run
	otherRun.ID = "run-save-fenced-other"
	otherRun.Message = "other before mismatched write"
	if err := store.SaveRun(ctx, otherRun); err != nil {
		t.Fatalf("SaveRun other initial: %v", err)
	}
	otherRun.Message = "mismatched leased write"
	if err := store.SaveRun(currentCtx, otherRun); !errors.Is(err, enginepersistence.ErrRunLeaseLost) {
		t.Fatalf("mismatched leased SaveRun err = %v, want ErrRunLeaseLost", err)
	}
	if err := store.SaveRun(currentCtx, run); err != nil {
		t.Fatalf("current leased SaveRun: %v", err)
	}
	stored, ok, err = store.Run(ctx, run.ID)
	if err != nil || !ok || stored.Message != "current write" {
		t.Fatalf("run after current write = %+v, ok=%v err=%v", stored, ok, err)
	}
}
