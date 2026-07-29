package adk

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

type executionClaimResultStub struct {
	rows int64
	err  error
}

func (result executionClaimResultStub) LastInsertId() (int64, error) { return 0, nil }
func (result executionClaimResultStub) RowsAffected() (int64, error) { return result.rows, result.err }

type executionClaimTxStub struct {
	result    sql.Result
	execErr   error
	getErr    error
	commitErr error
}

func (tx executionClaimTxStub) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return tx.result, tx.execErr
}

func (tx executionClaimTxStub) GetContext(context.Context, any, string, ...any) error {
	return tx.getErr
}

func (tx executionClaimTxStub) Commit() error { return tx.commitErr }

func TestExecutionClaimValidationAndReleaseLifecycle(t *testing.T) {
	ctx := t.Context()
	var nilStore *Store
	if _, err := nilStore.ClaimRunLease(ctx, "run", "owner", time.Time{}, time.Minute); err == nil {
		t.Fatal("nil store claimed a run lease")
	}
	if err := nilStore.ReleaseRunLease(ctx, RunLease{}); err != nil {
		t.Fatalf("nil store release: %v", err)
	}
	if lease, ok, err := nilStore.RunLease(ctx, "run"); err != nil || ok || lease.RunID != "" {
		t.Fatalf("nil store RunLease = %#v, %v, %v", lease, ok, err)
	}
	if _, err := nilStore.ClaimToolInvocation(ctx, ToolInvocationClaim{}); err == nil {
		t.Fatal("nil store claimed a tool invocation")
	}

	store := newExecutionClaimTestStore(t)
	if _, err := store.ClaimRunLease(ctx, "run-invalid", "owner", time.Now(), 0); err == nil || !strings.Contains(err.Error(), "TTL") {
		t.Fatalf("non-positive run TTL error = %v", err)
	}
	if _, err := store.ClaimRunLease(ctx, " ", "owner", time.Now(), time.Minute); err == nil {
		t.Fatal("blank run id claimed a lease")
	}
	if err := store.ReleaseRunLease(ctx, RunLease{}); err != nil {
		t.Fatalf("empty release: %v", err)
	}
	if lease, ok, err := store.RunLease(ctx, "missing"); err != nil || ok || lease.RunID != "" {
		t.Fatalf("missing RunLease = %#v, %v, %v", lease, ok, err)
	}

	lease, err := store.ClaimRunLease(ctx, "run-zero-time", "executor-a", time.Time{}, time.Minute)
	if err != nil || lease.HeartbeatAt.IsZero() || lease.ExpiresAt.IsZero() {
		t.Fatalf("zero-time run claim = %#v, err=%v", lease, err)
	}
	if _, err := store.HeartbeatRunLease(ctx, lease, time.Now(), 0); err == nil || !strings.Contains(err.Error(), "TTL") {
		t.Fatalf("non-positive heartbeat TTL error = %v", err)
	}
	refreshed, err := store.HeartbeatRunLease(ctx, lease, time.Time{}, time.Minute)
	if err != nil || !refreshed.ExpiresAt.After(refreshed.HeartbeatAt) {
		t.Fatalf("zero-time heartbeat = %#v, err=%v", refreshed, err)
	}
	if err := store.ReleaseRunLease(ctx, refreshed); err != nil {
		t.Fatalf("release current lease: %v", err)
	}
}

func TestToolInvocationAbandonAndCorruptionBoundaries(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	ctx := t.Context()
	now := time.Now().UTC()
	lease, err := store.ClaimRunLease(ctx, "run-tool-boundaries", "executor-a", now, time.Hour)
	if err != nil {
		t.Fatalf("ClaimRunLease: %v", err)
	}
	base := ToolInvocationClaim{
		RunID: lease.RunID, IdempotencyKey: "call-abandon", ToolName: "orders.submit",
		OwnerID: lease.OwnerID, RunLeaseToken: lease.FencingToken,
		Input: map[string]any{"quantity": 1}, Mode: ToolIdempotencyKeyed,
		Now: now, TTL: time.Minute,
	}
	invalidClaims := []ToolInvocationClaim{
		{},
		{RunID: lease.RunID, IdempotencyKey: "call", ToolName: "tool", OwnerID: lease.OwnerID, TTL: 0},
		{RunID: lease.RunID, IdempotencyKey: "call-json", ToolName: "tool", OwnerID: lease.OwnerID, TTL: time.Minute, Input: map[string]any{"bad": make(chan int)}},
	}
	for index, claim := range invalidClaims {
		if _, err := store.ClaimToolInvocation(ctx, claim); err == nil {
			t.Fatalf("invalid tool claim %d succeeded", index)
		}
	}

	ticket, err := store.ClaimToolInvocation(ctx, base)
	if err != nil || !ticket.Execute {
		t.Fatalf("ClaimToolInvocation = %#v, err=%v", ticket, err)
	}
	mismatch := base
	mismatch.Input = map[string]any{"quantity": 2}
	if _, err := store.ClaimToolInvocation(ctx, mismatch); err == nil || !strings.Contains(err.Error(), "reused with different") {
		t.Fatalf("mismatched invocation error = %v", err)
	}
	if err := store.HeartbeatToolInvocation(ctx, ticket, now, 0); err == nil || !strings.Contains(err.Error(), "TTL") {
		t.Fatalf("non-positive tool heartbeat error = %v", err)
	}
	if err := store.HeartbeatToolInvocation(ctx, ticket, time.Time{}, time.Minute); err != nil {
		t.Fatalf("zero-time tool heartbeat: %v", err)
	}
	if err := store.CompleteToolInvocation(ctx, ticket, map[string]any{"bad": make(chan int)}, time.Now()); err == nil || !strings.Contains(err.Error(), "encode") {
		t.Fatalf("unencodable tool output error = %v", err)
	}
	if err := store.AbandonToolInvocation(ctx, ticket); err != nil {
		t.Fatalf("AbandonToolInvocation: %v", err)
	}
	if err := store.AbandonToolInvocation(ctx, ticket); !errors.Is(err, ErrToolInvocationLost) {
		t.Fatalf("second abandon error = %v, want lost", err)
	}

	base.IdempotencyKey = "call-corrupt-output"
	ticket, err = store.ClaimToolInvocation(ctx, base)
	if err != nil {
		t.Fatalf("claim corrupt-output seed: %v", err)
	}
	if err := store.CompleteToolInvocation(ctx, ticket, map[string]any{"ok": true}, time.Time{}); err != nil {
		t.Fatalf("complete corrupt-output seed: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE `+tableToolInvocations+` SET output_json = '{' WHERE run_id = ? AND idempotency_key = ?`, ticket.RunID, ticket.IdempotencyKey); err != nil {
		t.Fatalf("corrupt replay output: %v", err)
	}
	if _, err := store.ClaimToolInvocation(ctx, base); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("corrupt replay error = %v", err)
	}

	base.IdempotencyKey = "call-unknown-status"
	ticket, err = store.ClaimToolInvocation(ctx, base)
	if err != nil {
		t.Fatalf("claim unknown-status seed: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE `+tableToolInvocations+` SET status = 'UNKNOWN' WHERE run_id = ? AND idempotency_key = ?`, ticket.RunID, ticket.IdempotencyKey); err != nil {
		t.Fatalf("write unknown status: %v", err)
	}
	if _, err := store.ClaimToolInvocation(ctx, base); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unknown status error = %v", err)
	}

	base.IdempotencyKey = "call-indeterminate"
	ticket, err = store.ClaimToolInvocation(ctx, base)
	if err != nil {
		t.Fatalf("claim indeterminate seed: %v", err)
	}
	if err := store.MarkToolInvocationIndeterminate(ctx, ticket, time.Time{}); err != nil {
		t.Fatalf("MarkToolInvocationIndeterminate: %v", err)
	}
	if _, err := store.ClaimToolInvocation(ctx, base); !errors.Is(err, ErrToolOutcomeUnknown) {
		t.Fatalf("indeterminate replay error = %v", err)
	}
}

func TestExecutionClaimUpdateResultFailures(t *testing.T) {
	claim := ToolInvocationClaim{RunID: "run", ToolName: "tool"}
	forced := errors.New("forced result failure")
	if err := requireToolClaimUpdate(nil, forced, claim); !errors.Is(err, forced) {
		t.Fatalf("direct update error = %v", err)
	}
	if err := requireToolClaimUpdate(executionClaimResultStub{err: forced}, nil, claim); !errors.Is(err, forced) {
		t.Fatalf("RowsAffected error = %v", err)
	}
	if err := requireToolClaimUpdate(executionClaimResultStub{}, nil, claim); !errors.Is(err, ErrToolInvocationInFlight) {
		t.Fatalf("zero-row update error = %v", err)
	}
	if err := requireToolClaimUpdate(executionClaimResultStub{rows: 1}, nil, claim); err != nil {
		t.Fatalf("one-row update: %v", err)
	}

	tx := executionClaimTxStub{execErr: forced}
	if err := lockRunLease(t.Context(), tx, "run", "owner", 1, time.Now()); !errors.Is(err, forced) {
		t.Fatalf("lease lock exec error = %v", err)
	}
	tx = executionClaimTxStub{result: executionClaimResultStub{err: forced}}
	if err := lockRunLease(t.Context(), tx, "run", "owner", 1, time.Now()); !errors.Is(err, forced) {
		t.Fatalf("lease lock RowsAffected error = %v", err)
	}
	tx = executionClaimTxStub{result: executionClaimResultStub{}}
	if err := lockRunLease(t.Context(), tx, "run", "owner", 1, time.Now()); !errors.Is(err, ErrRunLeaseLost) {
		t.Fatalf("lease lock zero-row error = %v", err)
	}
}

func TestExecutionClaimsSurfaceClosedDatabaseFailures(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	ctx := t.Context()
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	now := time.Now().UTC()
	lease := RunLease{RunID: "run-closed", OwnerID: "owner", FencingToken: 1, ExpiresAt: now.Add(time.Minute)}
	if _, err := store.ClaimRunLease(ctx, lease.RunID, lease.OwnerID, now, time.Minute); err == nil {
		t.Fatal("closed database claimed run lease")
	}
	if _, err := store.HeartbeatRunLease(ctx, lease, now, time.Minute); err == nil {
		t.Fatal("closed database heartbeated run lease")
	}
	if err := store.ReleaseRunLease(ctx, lease); err == nil {
		t.Fatal("closed database released run lease")
	}
	if _, _, err := store.RunLease(ctx, lease.RunID); err == nil {
		t.Fatal("closed database read run lease")
	}
	claim := ToolInvocationClaim{
		RunID: lease.RunID, IdempotencyKey: "call-closed", ToolName: "tool", OwnerID: lease.OwnerID,
		RunLeaseToken: lease.FencingToken, Input: map[string]any{"value": 1}, Now: now, TTL: time.Minute,
	}
	if _, err := store.ClaimToolInvocation(ctx, claim); err == nil {
		t.Fatal("closed database claimed tool invocation")
	}
	ticket := ToolInvocationTicket{
		RunID: claim.RunID, IdempotencyKey: claim.IdempotencyKey, OwnerID: claim.OwnerID,
		FencingToken: 1, RunLeaseToken: lease.FencingToken,
	}
	if err := store.HeartbeatToolInvocation(ctx, ticket, now, time.Minute); err == nil {
		t.Fatal("closed database heartbeated tool invocation")
	}
	if err := store.CompleteToolInvocation(ctx, ticket, map[string]any{"ok": true}, now); err == nil {
		t.Fatal("closed database completed tool invocation")
	}
	if err := store.MarkToolInvocationIndeterminate(ctx, ticket, now); err == nil {
		t.Fatal("closed database marked tool invocation")
	}
	if err := store.AbandonToolInvocation(ctx, ticket); err == nil {
		t.Fatal("closed database abandoned tool invocation")
	}
}
