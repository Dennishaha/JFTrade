package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
)

func newExecutionClaimTestStore(t *testing.T) *ClaimStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "adk.db")
	if err := sqliteschema.ValidateCurrentFile(context.Background(), dbPath, sqliteschema.DatabaseADK); err != nil {
		t.Fatalf("ValidateCurrentFile: %v", err)
	}
	db, err := sqliteconn.OpenX(dbPath)
	if err != nil {
		t.Fatalf("OpenX: %v", err)
	}
	if err := sqliteschema.InitializeCurrent(context.Background(), db, dbPath, sqliteschema.DatabaseADK); err != nil {
		_ = db.Close()
		t.Fatalf("InitializeCurrent: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewClaimStore(db)
}

func newExecutionClaimTestStores(t *testing.T) (*ClaimStore, *ClaimStore) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "adk.db")
	firstDB, err := sqliteconn.OpenX(dbPath)
	if err != nil {
		t.Fatalf("OpenX first: %v", err)
	}
	if err := sqliteschema.InitializeCurrent(context.Background(), firstDB, dbPath, sqliteschema.DatabaseADK); err != nil {
		_ = firstDB.Close()
		t.Fatalf("InitializeCurrent first: %v", err)
	}
	secondDB, err := sqliteconn.OpenX(dbPath)
	if err != nil {
		_ = firstDB.Close()
		t.Fatalf("OpenX second: %v", err)
	}
	t.Cleanup(func() {
		_ = secondDB.Close()
		_ = firstDB.Close()
	})
	return NewClaimStore(firstDB), NewClaimStore(secondDB)
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
		for index, store := range []*ClaimStore{firstStore, secondStore} {
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
		for _, store := range []*ClaimStore{firstStore, secondStore} {
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
