package adk

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestRunLeaseUsesExpiryAndFencingTokens(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	ctx := t.Context()
	now := time.Date(2026, 7, 22, 8, 0, 0, 0, time.UTC)

	first, err := store.ClaimRunLease(ctx, "run-fenced", "executor-a", now, time.Minute)
	if err != nil {
		t.Fatalf("ClaimRunLease first: %v", err)
	}
	if first.FencingToken != 1 {
		t.Fatalf("first fencing token = %d, want 1", first.FencingToken)
	}
	if _, err := store.ClaimRunLease(ctx, first.RunID, "executor-b", now.Add(30*time.Second), time.Minute); !errors.Is(err, ErrRunLeaseHeld) {
		t.Fatalf("fresh foreign claim err = %v, want ErrRunLeaseHeld", err)
	}

	second, err := store.ClaimRunLease(ctx, first.RunID, "executor-b", now.Add(2*time.Minute), time.Minute)
	if err != nil {
		t.Fatalf("ClaimRunLease takeover: %v", err)
	}
	if second.FencingToken != 2 || second.OwnerID != "executor-b" {
		t.Fatalf("takeover lease = %#v, want executor-b token 2", second)
	}
	if _, err := store.HeartbeatRunLease(ctx, first, now.Add(2*time.Minute), time.Minute); !errors.Is(err, ErrRunLeaseLost) {
		t.Fatalf("stale heartbeat err = %v, want ErrRunLeaseLost", err)
	}
	if err := store.ReleaseRunLease(ctx, first); err != nil {
		t.Fatalf("ReleaseRunLease stale: %v", err)
	}
	current, ok, err := store.RunLease(ctx, first.RunID)
	if err != nil || !ok || current.FencingToken != second.FencingToken {
		t.Fatalf("current lease = %#v ok=%v err=%v, want takeover lease", current, ok, err)
	}
}

func TestRunAndToolClaimsSerializeAcrossStoreConnections(t *testing.T) {
	firstStore, secondStore := newExecutionClaimTestStores(t)
	ctx := t.Context()
	now := time.Date(2026, 7, 22, 8, 30, 0, 0, time.UTC)

	t.Run("one run owner wins an atomic claim", func(t *testing.T) {
		type result struct {
			lease RunLease
			err   error
		}
		start := make(chan struct{})
		results := make(chan result, 2)
		var ready sync.WaitGroup
		ready.Add(2)
		for index, store := range []*Store{firstStore, secondStore} {
			ownerID := []string{"executor-a", "executor-b"}[index]
			go func() {
				ready.Done()
				<-start
				lease, err := store.ClaimRunLease(ctx, "run-cross-process", ownerID, now, time.Minute)
				results <- result{lease: lease, err: err}
			}()
		}
		ready.Wait()
		close(start)
		var successes int
		var held int
		for range 2 {
			result := <-results
			switch {
			case result.err == nil:
				successes++
				if result.lease.FencingToken != 1 {
					t.Fatalf("winning lease token = %d, want 1", result.lease.FencingToken)
				}
			case errors.Is(result.err, ErrRunLeaseHeld):
				held++
			default:
				t.Fatalf("concurrent run claim: %v", result.err)
			}
		}
		if successes != 1 || held != 1 {
			t.Fatalf("concurrent run claims successes=%d held=%d, want 1 and 1", successes, held)
		}
	})

	lease, err := firstStore.ClaimRunLease(ctx, "run-cross-tool", "executor-shared", now, time.Minute)
	if err != nil {
		t.Fatalf("ClaimRunLease for tool race: %v", err)
	}
	t.Run("one tool invocation wins across connections", func(t *testing.T) {
		claim := ToolInvocationClaim{
			RunID: lease.RunID, IdempotencyKey: "call-cross-process", ToolName: "market.read",
			OwnerID: lease.OwnerID, RunLeaseToken: lease.FencingToken,
			Input: map[string]any{"symbol": "AAPL"}, Mode: ToolIdempotencyReplaySafe,
			Now: now, TTL: time.Minute,
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		var ready sync.WaitGroup
		ready.Add(2)
		for _, store := range []*Store{firstStore, secondStore} {
			go func() {
				ready.Done()
				<-start
				ticket, claimErr := store.ClaimToolInvocation(ctx, claim)
				if claimErr == nil && !ticket.Execute {
					claimErr = errors.New("winning tool claim did not request execution")
				}
				results <- claimErr
			}()
		}
		ready.Wait()
		close(start)
		var successes int
		var inFlight int
		for range 2 {
			claimErr := <-results
			switch {
			case claimErr == nil:
				successes++
			case errors.Is(claimErr, ErrToolInvocationInFlight):
				inFlight++
			default:
				t.Fatalf("concurrent tool claim: %v", claimErr)
			}
		}
		if successes != 1 || inFlight != 1 {
			t.Fatalf("concurrent tool claims successes=%d in-flight=%d, want 1 and 1", successes, inFlight)
		}
	})
}

func TestToolInvocationClaimReplaysCompletedOutput(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	ctx := t.Context()
	now := time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)
	lease, err := store.ClaimRunLease(ctx, "run-replay", "executor-a", now, 10*time.Minute)
	if err != nil {
		t.Fatalf("ClaimRunLease: %v", err)
	}
	claim := ToolInvocationClaim{
		RunID: lease.RunID, IdempotencyKey: "call-1", ToolName: "market.read",
		OwnerID: lease.OwnerID, RunLeaseToken: lease.FencingToken,
		Input: map[string]any{"symbol": "AAPL"}, Mode: ToolIdempotencyReplaySafe,
		Now: now, TTL: time.Minute,
	}
	ticket, err := store.ClaimToolInvocation(ctx, claim)
	if err != nil || !ticket.Execute || ticket.Replayed {
		t.Fatalf("first tool claim = %#v err=%v", ticket, err)
	}
	if _, err := store.ClaimToolInvocation(ctx, claim); !errors.Is(err, ErrToolInvocationInFlight) {
		t.Fatalf("duplicate in-flight claim err = %v, want ErrToolInvocationInFlight", err)
	}
	if err := store.HeartbeatToolInvocation(ctx, ticket, now.Add(30*time.Second), time.Minute); err != nil {
		t.Fatalf("HeartbeatToolInvocation: %v", err)
	}
	heartbeatClaim := claim
	heartbeatClaim.Now = now.Add(70 * time.Second)
	if _, err := store.ClaimToolInvocation(ctx, heartbeatClaim); !errors.Is(err, ErrToolInvocationInFlight) {
		t.Fatalf("claim within renewed tool lease err = %v, want ErrToolInvocationInFlight", err)
	}
	want := map[string]any{"price": 123.5, "source": "test"}
	if err := store.CompleteToolInvocation(ctx, ticket, want, now.Add(71*time.Second)); err != nil {
		t.Fatalf("CompleteToolInvocation: %v", err)
	}
	claim.Now = now.Add(72 * time.Second)
	replayed, err := store.ClaimToolInvocation(ctx, claim)
	if err != nil || replayed.Execute || !replayed.Replayed {
		t.Fatalf("replayed tool claim = %#v err=%v", replayed, err)
	}
	if replayed.Output["price"] != want["price"] || replayed.Output["source"] != want["source"] {
		t.Fatalf("replayed output = %#v, want %#v", replayed.Output, want)
	}
}

func TestToolInvocationCrashPolicyFailsClosedOrFencedTakeover(t *testing.T) {
	t.Run("unkeyed write becomes indeterminate", func(t *testing.T) {
		store := newExecutionClaimTestStore(t)
		ctx := t.Context()
		now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
		lease, err := store.ClaimRunLease(ctx, "run-write", "executor-a", now, time.Hour)
		if err != nil {
			t.Fatalf("ClaimRunLease: %v", err)
		}
		claim := ToolInvocationClaim{
			RunID: lease.RunID, IdempotencyKey: "write-1", ToolName: "orders.submit",
			OwnerID: lease.OwnerID, RunLeaseToken: lease.FencingToken,
			Input: map[string]any{"quantity": 1}, Mode: ToolIdempotencyFailClosed,
			Now: now, TTL: time.Second,
		}
		if _, err := store.ClaimToolInvocation(ctx, claim); err != nil {
			t.Fatalf("first write claim: %v", err)
		}
		claim.Now = now.Add(2 * time.Second)
		if _, err := store.ClaimToolInvocation(ctx, claim); !errors.Is(err, ErrToolOutcomeUnknown) {
			t.Fatalf("stale write claim err = %v, want ErrToolOutcomeUnknown", err)
		}
		claim.Now = now.Add(3 * time.Second)
		if _, err := store.ClaimToolInvocation(ctx, claim); !errors.Is(err, ErrToolOutcomeUnknown) {
			t.Fatalf("indeterminate write replay err = %v, want ErrToolOutcomeUnknown", err)
		}
	})

	t.Run("keyed tool can be taken over after both leases expire", func(t *testing.T) {
		store := newExecutionClaimTestStore(t)
		ctx := t.Context()
		now := time.Date(2026, 7, 22, 11, 0, 0, 0, time.UTC)
		firstLease, err := store.ClaimRunLease(ctx, "run-keyed", "executor-a", now, time.Second)
		if err != nil {
			t.Fatalf("ClaimRunLease first: %v", err)
		}
		claim := ToolInvocationClaim{
			RunID: firstLease.RunID, IdempotencyKey: "keyed-1", ToolName: "orders.submit_keyed",
			OwnerID: firstLease.OwnerID, RunLeaseToken: firstLease.FencingToken,
			Input: map[string]any{"quantity": 1}, Mode: ToolIdempotencyKeyed,
			Now: now, TTL: time.Second,
		}
		firstTicket, err := store.ClaimToolInvocation(ctx, claim)
		if err != nil {
			t.Fatalf("ClaimToolInvocation first: %v", err)
		}
		takeoverAt := now.Add(2 * time.Second)
		secondLease, err := store.ClaimRunLease(ctx, firstLease.RunID, "executor-b", takeoverAt, time.Minute)
		if err != nil {
			t.Fatalf("ClaimRunLease takeover: %v", err)
		}
		claim.OwnerID = secondLease.OwnerID
		claim.RunLeaseToken = secondLease.FencingToken
		claim.Now = takeoverAt
		secondTicket, err := store.ClaimToolInvocation(ctx, claim)
		if err != nil || secondTicket.FencingToken != firstTicket.FencingToken+1 {
			t.Fatalf("tool takeover = %#v err=%v", secondTicket, err)
		}
		if err := store.CompleteToolInvocation(ctx, firstTicket, map[string]any{"old": true}, takeoverAt); !errors.Is(err, ErrToolInvocationLost) {
			t.Fatalf("stale completion err = %v, want ErrToolInvocationLost", err)
		}
		if err := store.CompleteToolInvocation(ctx, secondTicket, map[string]any{"ok": true}, takeoverAt); err != nil {
			t.Fatalf("takeover completion: %v", err)
		}
	})
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
	if _, err := tool.run(oldToolCtx, map[string]any{"value": "stale"}); !errors.Is(err, ErrRunLeaseLost) {
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
		if _, err := tool.run(toolCtx, map[string]any{"value": "once"}); !errors.Is(err, ErrToolOutcomeUnknown) {
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
	var renewed RunLease
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
	if _, err := store.ClaimRunLease(t.Context(), initial.RunID, "executor-other", takeoverAt, time.Second); !errors.Is(err, ErrRunLeaseHeld) {
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
	if err := store.SaveRun(staleCtx, run); !errors.Is(err, ErrRunLeaseLost) {
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
	if err := store.SaveRun(currentCtx, otherRun); !errors.Is(err, ErrRunLeaseLost) {
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
