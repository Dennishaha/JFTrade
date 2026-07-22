package adk

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrRunLeaseHeld           = errors.New("ADK run lease is held by another executor")
	ErrRunLeaseLost           = errors.New("ADK run lease fencing token is no longer current")
	ErrToolInvocationInFlight = errors.New("ADK tool invocation is already in flight")
	ErrToolOutcomeUnknown     = errors.New("ADK tool invocation outcome is unknown after executor failure")
	ErrToolInvocationLost     = errors.New("ADK tool invocation fencing token is no longer current")
)

const (
	ToolIdempotencyFailClosed = "fail_closed"
	ToolIdempotencyReplaySafe = "replay_safe"
	ToolIdempotencyKeyed      = "keyed"

	toolInvocationStatusRunning       = "RUNNING"
	toolInvocationStatusCompleted     = "COMPLETED"
	toolInvocationStatusIndeterminate = "INDETERMINATE"
)

type RunLease struct {
	RunID        string
	OwnerID      string
	FencingToken uint64
	HeartbeatAt  time.Time
	ExpiresAt    time.Time
}

type ToolInvocationClaim struct {
	RunID          string
	IdempotencyKey string
	ToolName       string
	OwnerID        string
	RunLeaseToken  uint64
	Input          map[string]any
	Mode           string
	Now            time.Time
	TTL            time.Duration
}

type ToolInvocationTicket struct {
	RunID          string
	IdempotencyKey string
	OwnerID        string
	FencingToken   uint64
	RunLeaseToken  uint64
	Execute        bool
	Replayed       bool
	Output         map[string]any
}

type runLeaseRow struct {
	RunID             string `db:"run_id"`
	OwnerID           string `db:"owner_id"`
	FencingToken      uint64 `db:"fencing_token"`
	HeartbeatAtUnixMs int64  `db:"heartbeat_at_unix_ms"`
	ExpiresAtUnixMs   int64  `db:"expires_at_unix_ms"`
}

type toolInvocationRow struct {
	RunID                string `db:"run_id"`
	IdempotencyKey       string `db:"idempotency_key"`
	ToolName             string `db:"tool_name"`
	Status               string `db:"status"`
	OwnerID              string `db:"owner_id"`
	FencingToken         uint64 `db:"fencing_token"`
	RunLeaseToken        uint64 `db:"run_lease_token"`
	InputJSON            string `db:"input_json"`
	OutputJSON           string `db:"output_json"`
	LeaseExpiresAtUnixMs int64  `db:"lease_expires_at_unix_ms"`
}

type executionClaimTx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	GetContext(context.Context, any, string, ...any) error
	Commit() error
}

func (s *Store) ensureExecutionClaimSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS ` + tableRunLeases + ` (` +
			`run_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, fencing_token INTEGER NOT NULL, ` +
			`heartbeat_at_unix_ms INTEGER NOT NULL, expires_at_unix_ms INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_adk_run_leases_expires ON ` + tableRunLeases + ` (expires_at_unix_ms ASC)`,
		`CREATE TABLE IF NOT EXISTS ` + tableToolInvocations + ` (` +
			`run_id TEXT NOT NULL, idempotency_key TEXT NOT NULL, tool_name TEXT NOT NULL, status TEXT NOT NULL, ` +
			`owner_id TEXT NOT NULL, fencing_token INTEGER NOT NULL, run_lease_token INTEGER NOT NULL, ` +
			`input_json TEXT NOT NULL, output_json TEXT NOT NULL, lease_expires_at_unix_ms INTEGER NOT NULL, ` +
			`created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY (run_id, idempotency_key))`,
		`CREATE INDEX IF NOT EXISTS idx_adk_tool_invocations_status ON ` + tableToolInvocations + ` (status, lease_expires_at_unix_ms ASC)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize ADK execution claims: %w", err)
		}
	}
	return nil
}

func (s *Store) ClaimRunLease(
	ctx context.Context,
	runID string,
	ownerID string,
	now time.Time,
	ttl time.Duration,
) (RunLease, error) {
	runID = strings.TrimSpace(runID)
	ownerID = strings.TrimSpace(ownerID)
	if s == nil || s.db == nil || runID == "" || ownerID == "" {
		return RunLease{}, fmt.Errorf("ADK run lease requires store, run id, and owner id")
	}
	if ttl <= 0 {
		return RunLease{}, fmt.Errorf("ADK run lease TTL must be positive")
	}
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	expiresAt := now.Add(ttl)
	tx, err := s.db.BeginWrite(ctx, nil)
	if err != nil {
		return RunLease{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var row runLeaseRow
	err = tx.GetContext(ctx, &row, `INSERT INTO `+tableRunLeases+` (run_id, owner_id, fencing_token, heartbeat_at_unix_ms, expires_at_unix_ms, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET
			owner_id = excluded.owner_id,
			fencing_token = `+tableRunLeases+`.fencing_token + 1,
			heartbeat_at_unix_ms = excluded.heartbeat_at_unix_ms,
			expires_at_unix_ms = excluded.expires_at_unix_ms,
			updated_at = excluded.updated_at
		WHERE `+tableRunLeases+`.expires_at_unix_ms <= excluded.heartbeat_at_unix_ms
		RETURNING run_id, owner_id, fencing_token, heartbeat_at_unix_ms, expires_at_unix_ms`,
		runID, ownerID, now.UnixMilli(), expiresAt.UnixMilli(), formatClaimTime(now), formatClaimTime(now))
	if errors.Is(err, sql.ErrNoRows) {
		return RunLease{}, fmt.Errorf("%w: run %s has an unexpired owner", ErrRunLeaseHeld, runID)
	}
	if err != nil {
		return RunLease{}, err
	}
	if err := tx.Commit(); err != nil {
		return RunLease{}, err
	}
	return runLeaseFromRow(row)
}

func (s *Store) HeartbeatRunLease(ctx context.Context, lease RunLease, now time.Time, ttl time.Duration) (RunLease, error) {
	if ttl <= 0 {
		return RunLease{}, fmt.Errorf("ADK run lease TTL must be positive")
	}
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	expiresAt := now.Add(ttl)
	result, err := s.db.ExecContext(ctx, `UPDATE `+tableRunLeases+` SET heartbeat_at_unix_ms = ?, expires_at_unix_ms = ?, updated_at = ? WHERE run_id = ? AND owner_id = ? AND fencing_token = ? AND expires_at_unix_ms > ?`,
		now.UnixMilli(), expiresAt.UnixMilli(), formatClaimTime(now), lease.RunID, lease.OwnerID, lease.FencingToken, now.UnixMilli())
	if err != nil {
		return RunLease{}, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return RunLease{}, err
	}
	if updated != 1 {
		return RunLease{}, fmt.Errorf("%w: run %s token %d", ErrRunLeaseLost, lease.RunID, lease.FencingToken)
	}
	lease.HeartbeatAt = now
	lease.ExpiresAt = expiresAt
	return lease, nil
}

func (s *Store) ReleaseRunLease(ctx context.Context, lease RunLease) error {
	if s == nil || s.db == nil || strings.TrimSpace(lease.RunID) == "" {
		return nil
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE `+tableRunLeases+` SET owner_id = '', fencing_token = fencing_token + 1, heartbeat_at_unix_ms = ?, expires_at_unix_ms = ?, updated_at = ? WHERE run_id = ? AND owner_id = ? AND fencing_token = ?`,
		now.UnixMilli(), now.UnixMilli(), formatClaimTime(now), lease.RunID, lease.OwnerID, lease.FencingToken)
	return err
}

func (s *Store) RunLease(ctx context.Context, runID string) (RunLease, bool, error) {
	if s == nil || s.db == nil {
		return RunLease{}, false, nil
	}
	var row runLeaseRow
	err := s.db.GetContext(ctx, &row, `SELECT run_id, owner_id, fencing_token, heartbeat_at_unix_ms, expires_at_unix_ms FROM `+tableRunLeases+` WHERE run_id = ?`, strings.TrimSpace(runID))
	if errors.Is(err, sql.ErrNoRows) {
		return RunLease{}, false, nil
	}
	if err != nil {
		return RunLease{}, false, err
	}
	lease, err := runLeaseFromRow(row)
	return lease, err == nil, err
}

func (s *Store) ClaimToolInvocation(ctx context.Context, claim ToolInvocationClaim) (ToolInvocationTicket, error) {
	claim, inputJSON, now, expiresAt, err := prepareToolInvocationClaim(s, claim)
	if err != nil {
		return ToolInvocationTicket{}, err
	}
	tx, err := s.db.BeginWrite(ctx, nil)
	if err != nil {
		return ToolInvocationTicket{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockRunLeaseForToolClaim(ctx, tx, claim, now); err != nil {
		return ToolInvocationTicket{}, err
	}
	var row toolInvocationRow
	err = tx.GetContext(ctx, &row, `SELECT run_id, idempotency_key, tool_name, status, owner_id, fencing_token, run_lease_token, input_json, output_json, lease_expires_at_unix_ms FROM `+tableToolInvocations+` WHERE run_id = ? AND idempotency_key = ?`, claim.RunID, claim.IdempotencyKey)
	if errors.Is(err, sql.ErrNoRows) {
		return insertToolInvocationClaim(ctx, tx, claim, inputJSON, now, expiresAt)
	}
	if err != nil {
		return ToolInvocationTicket{}, err
	}
	return resolveExistingToolInvocationClaim(ctx, tx, claim, inputJSON, row, now, expiresAt)
}

func prepareToolInvocationClaim(
	s *Store,
	claim ToolInvocationClaim,
) (ToolInvocationClaim, string, time.Time, time.Time, error) {
	claim.RunID = strings.TrimSpace(claim.RunID)
	claim.IdempotencyKey = strings.TrimSpace(claim.IdempotencyKey)
	claim.ToolName = strings.TrimSpace(claim.ToolName)
	claim.OwnerID = strings.TrimSpace(claim.OwnerID)
	claim.Mode = normalizeToolIdempotencyMode(claim.Mode, "")
	if s == nil || s.db == nil || claim.RunID == "" || claim.IdempotencyKey == "" || claim.ToolName == "" || claim.OwnerID == "" {
		return ToolInvocationClaim{}, "", time.Time{}, time.Time{}, fmt.Errorf("ADK tool invocation claim is incomplete")
	}
	if claim.TTL <= 0 {
		return ToolInvocationClaim{}, "", time.Time{}, time.Time{}, fmt.Errorf("ADK tool invocation TTL must be positive")
	}
	inputJSON, err := json.Marshal(claim.Input)
	if err != nil {
		return ToolInvocationClaim{}, "", time.Time{}, time.Time{}, fmt.Errorf("encode ADK tool invocation input: %w", err)
	}
	now := claim.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return claim, string(inputJSON), now, now.Add(claim.TTL), nil
}

func lockRunLeaseForToolClaim(
	ctx context.Context,
	tx executionClaimTx,
	claim ToolInvocationClaim,
	now time.Time,
) error {
	return lockRunLease(ctx, tx, claim.RunID, claim.OwnerID, claim.RunLeaseToken, now)
}

func lockRunExecutionLeaseFromContext(ctx context.Context, tx executionClaimTx, runID string) error {
	lease, ok := runExecutionLeaseFromContext(ctx)
	if !ok {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if lease.RunID != runID {
		return fmt.Errorf("%w: run %s cannot be written with lease for run %s", ErrRunLeaseLost, runID, lease.RunID)
	}
	return lockRunLease(ctx, tx, lease.RunID, lease.OwnerID, lease.FencingToken, time.Now().UTC())
}

func lockRunLease(
	ctx context.Context,
	tx executionClaimTx,
	runID string,
	ownerID string,
	fencingToken uint64,
	now time.Time,
) error {
	leaseLock, err := tx.ExecContext(ctx, `UPDATE `+tableRunLeases+` SET updated_at = updated_at WHERE run_id = ? AND owner_id = ? AND fencing_token = ? AND expires_at_unix_ms > ?`,
		runID, ownerID, fencingToken, now.UnixMilli())
	if err != nil {
		return err
	}
	locked, err := leaseLock.RowsAffected()
	if err != nil {
		return err
	}
	if locked != 1 {
		return fmt.Errorf("%w: run %s token %d", ErrRunLeaseLost, runID, fencingToken)
	}
	return nil
}

func insertToolInvocationClaim(
	ctx context.Context,
	tx executionClaimTx,
	claim ToolInvocationClaim,
	inputJSON string,
	now time.Time,
	expiresAt time.Time,
) (ToolInvocationTicket, error) {
	row := toolInvocationRow{
		RunID: claim.RunID, IdempotencyKey: claim.IdempotencyKey, ToolName: claim.ToolName,
		Status: toolInvocationStatusRunning, OwnerID: claim.OwnerID, FencingToken: 1,
		RunLeaseToken: claim.RunLeaseToken, InputJSON: inputJSON, OutputJSON: "",
		LeaseExpiresAtUnixMs: expiresAt.UnixMilli(),
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO `+tableToolInvocations+` (run_id, idempotency_key, tool_name, status, owner_id, fencing_token, run_lease_token, input_json, output_json, lease_expires_at_unix_ms, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.RunID, row.IdempotencyKey, row.ToolName, row.Status, row.OwnerID, row.FencingToken, row.RunLeaseToken,
		row.InputJSON, row.OutputJSON, row.LeaseExpiresAtUnixMs, formatClaimTime(now), formatClaimTime(now))
	if err == nil {
		err = tx.Commit()
	}
	return ticketFromInvocationRow(row, true, false, nil), err
}

func resolveExistingToolInvocationClaim(
	ctx context.Context,
	tx executionClaimTx,
	claim ToolInvocationClaim,
	inputJSON string,
	row toolInvocationRow,
	now time.Time,
	expiresAt time.Time,
) (ToolInvocationTicket, error) {
	if row.ToolName != claim.ToolName || row.InputJSON != inputJSON {
		return ToolInvocationTicket{}, fmt.Errorf("ADK tool invocation key %q was reused with different tool input", claim.IdempotencyKey)
	}
	resolved, handled, err := resolveTerminalToolInvocationClaim(row, claim)
	if handled || err != nil {
		return resolved, err
	}
	currentExpiryUnixMs := row.LeaseExpiresAtUnixMs
	if currentExpiryUnixMs > now.UnixMilli() {
		return ToolInvocationTicket{}, fmt.Errorf("%w: run %s tool %s key %s", ErrToolInvocationInFlight, claim.RunID, claim.ToolName, claim.IdempotencyKey)
	}
	if claim.Mode == ToolIdempotencyFailClosed {
		return failClosedStaleToolInvocation(ctx, tx, claim, row, now)
	}
	return takeoverStaleToolInvocation(ctx, tx, claim, row, now, expiresAt)
}

func resolveTerminalToolInvocationClaim(
	row toolInvocationRow,
	claim ToolInvocationClaim,
) (ToolInvocationTicket, bool, error) {
	switch row.Status {
	case toolInvocationStatusCompleted:
		output := map[string]any{}
		if strings.TrimSpace(row.OutputJSON) != "" {
			if err := json.Unmarshal([]byte(row.OutputJSON), &output); err != nil {
				return ToolInvocationTicket{}, true, fmt.Errorf("decode ADK tool invocation output: %w", err)
			}
		}
		return ticketFromInvocationRow(row, false, true, output), true, nil
	case toolInvocationStatusIndeterminate:
		return ToolInvocationTicket{}, true, fmt.Errorf("%w: run %s tool %s key %s", ErrToolOutcomeUnknown, claim.RunID, claim.ToolName, claim.IdempotencyKey)
	case toolInvocationStatusRunning:
		return ToolInvocationTicket{}, false, nil
	default:
		return ToolInvocationTicket{}, true, fmt.Errorf("unsupported ADK tool invocation status %q", row.Status)
	}
}

func failClosedStaleToolInvocation(
	ctx context.Context,
	tx executionClaimTx,
	claim ToolInvocationClaim,
	row toolInvocationRow,
	now time.Time,
) (ToolInvocationTicket, error) {
	result, err := tx.ExecContext(ctx, `UPDATE `+tableToolInvocations+` SET status = ?, lease_expires_at_unix_ms = ?, updated_at = ? WHERE run_id = ? AND idempotency_key = ? AND fencing_token = ? AND status = ? AND lease_expires_at_unix_ms = ?`,
		toolInvocationStatusIndeterminate, now.UnixMilli(), formatClaimTime(now), row.RunID, row.IdempotencyKey, row.FencingToken, toolInvocationStatusRunning, row.LeaseExpiresAtUnixMs)
	err = requireToolClaimUpdate(result, err, claim)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		return ToolInvocationTicket{}, err
	}
	return ToolInvocationTicket{}, fmt.Errorf("%w: run %s tool %s key %s", ErrToolOutcomeUnknown, claim.RunID, claim.ToolName, claim.IdempotencyKey)
}

func takeoverStaleToolInvocation(
	ctx context.Context,
	tx executionClaimTx,
	claim ToolInvocationClaim,
	row toolInvocationRow,
	now time.Time,
	expiresAt time.Time,
) (ToolInvocationTicket, error) {
	previousFencingToken := row.FencingToken
	previousLeaseExpiresAtUnixMs := row.LeaseExpiresAtUnixMs
	row.OwnerID = claim.OwnerID
	row.FencingToken++
	row.RunLeaseToken = claim.RunLeaseToken
	row.LeaseExpiresAtUnixMs = expiresAt.UnixMilli()
	result, err := tx.ExecContext(ctx, `UPDATE `+tableToolInvocations+` SET owner_id = ?, fencing_token = ?, run_lease_token = ?, lease_expires_at_unix_ms = ?, updated_at = ? WHERE run_id = ? AND idempotency_key = ? AND fencing_token = ? AND status = ? AND lease_expires_at_unix_ms = ?`,
		row.OwnerID, row.FencingToken, row.RunLeaseToken, row.LeaseExpiresAtUnixMs, formatClaimTime(now), row.RunID, row.IdempotencyKey, previousFencingToken, toolInvocationStatusRunning, previousLeaseExpiresAtUnixMs)
	err = requireToolClaimUpdate(result, err, claim)
	if err == nil {
		err = tx.Commit()
	}
	return ticketFromInvocationRow(row, true, false, nil), err
}

func requireToolClaimUpdate(result sql.Result, err error, claim ToolInvocationClaim) error {
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return fmt.Errorf("%w: run %s tool %s", ErrToolInvocationInFlight, claim.RunID, claim.ToolName)
	}
	return nil
}

func (s *Store) HeartbeatToolInvocation(ctx context.Context, ticket ToolInvocationTicket, now time.Time, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("ADK tool invocation TTL must be positive")
	}
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowUnixMs := now.UnixMilli()
	result, err := s.db.ExecContext(ctx, `UPDATE `+tableToolInvocations+` SET lease_expires_at_unix_ms = ?, updated_at = ? WHERE run_id = ? AND idempotency_key = ? AND owner_id = ? AND fencing_token = ? AND status = ? AND lease_expires_at_unix_ms > ? AND EXISTS (SELECT 1 FROM `+tableRunLeases+` WHERE run_id = ? AND owner_id = ? AND fencing_token = ? AND expires_at_unix_ms > ?)`,
		now.Add(ttl).UnixMilli(), formatClaimTime(now), ticket.RunID, ticket.IdempotencyKey,
		ticket.OwnerID, ticket.FencingToken, toolInvocationStatusRunning,
		nowUnixMs, ticket.RunID, ticket.OwnerID, ticket.RunLeaseToken, nowUnixMs)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return ErrToolInvocationLost
	}
	return nil
}

func (s *Store) CompleteToolInvocation(ctx context.Context, ticket ToolInvocationTicket, output map[string]any, now time.Time) error {
	outputJSON, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("encode ADK tool invocation output: %w", err)
	}
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowUnixMs := now.UnixMilli()
	result, err := s.db.ExecContext(ctx, `UPDATE `+tableToolInvocations+` SET status = ?, output_json = ?, lease_expires_at_unix_ms = ?, updated_at = ? WHERE run_id = ? AND idempotency_key = ? AND owner_id = ? AND fencing_token = ? AND status = ? AND lease_expires_at_unix_ms > ? AND EXISTS (SELECT 1 FROM `+tableRunLeases+` WHERE run_id = ? AND owner_id = ? AND fencing_token = ? AND expires_at_unix_ms > ?)`,
		toolInvocationStatusCompleted, string(outputJSON), nowUnixMs, formatClaimTime(now),
		ticket.RunID, ticket.IdempotencyKey, ticket.OwnerID, ticket.FencingToken, toolInvocationStatusRunning,
		nowUnixMs, ticket.RunID, ticket.OwnerID, ticket.RunLeaseToken, nowUnixMs)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return ErrToolInvocationLost
	}
	return nil
}

func (s *Store) MarkToolInvocationIndeterminate(ctx context.Context, ticket ToolInvocationTicket, now time.Time) error {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowUnixMs := now.UnixMilli()
	result, err := s.db.ExecContext(ctx, `UPDATE `+tableToolInvocations+` SET status = ?, lease_expires_at_unix_ms = ?, updated_at = ? WHERE run_id = ? AND idempotency_key = ? AND owner_id = ? AND fencing_token = ? AND status = ? AND lease_expires_at_unix_ms > ? AND EXISTS (SELECT 1 FROM `+tableRunLeases+` WHERE run_id = ? AND owner_id = ? AND fencing_token = ? AND expires_at_unix_ms > ?)`,
		toolInvocationStatusIndeterminate, nowUnixMs, formatClaimTime(now),
		ticket.RunID, ticket.IdempotencyKey, ticket.OwnerID, ticket.FencingToken, toolInvocationStatusRunning,
		nowUnixMs, ticket.RunID, ticket.OwnerID, ticket.RunLeaseToken, nowUnixMs)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return ErrToolInvocationLost
	}
	return nil
}

func (s *Store) AbandonToolInvocation(ctx context.Context, ticket ToolInvocationTicket) error {
	nowUnixMs := time.Now().UTC().UnixMilli()
	result, err := s.db.ExecContext(ctx, `DELETE FROM `+tableToolInvocations+` WHERE run_id = ? AND idempotency_key = ? AND owner_id = ? AND fencing_token = ? AND status = ? AND lease_expires_at_unix_ms > ? AND EXISTS (SELECT 1 FROM `+tableRunLeases+` WHERE run_id = ? AND owner_id = ? AND fencing_token = ? AND expires_at_unix_ms > ?)`,
		ticket.RunID, ticket.IdempotencyKey, ticket.OwnerID, ticket.FencingToken, toolInvocationStatusRunning,
		nowUnixMs, ticket.RunID, ticket.OwnerID, ticket.RunLeaseToken, nowUnixMs)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return ErrToolInvocationLost
	}
	return nil
}

func runLeaseFromRow(row runLeaseRow) (RunLease, error) {
	return RunLease{
		RunID:        row.RunID,
		OwnerID:      row.OwnerID,
		FencingToken: row.FencingToken,
		HeartbeatAt:  time.UnixMilli(row.HeartbeatAtUnixMs).UTC(),
		ExpiresAt:    time.UnixMilli(row.ExpiresAtUnixMs).UTC(),
	}, nil
}

func ticketFromInvocationRow(row toolInvocationRow, execute bool, replayed bool, output map[string]any) ToolInvocationTicket {
	return ToolInvocationTicket{
		RunID: row.RunID, IdempotencyKey: row.IdempotencyKey, OwnerID: row.OwnerID,
		FencingToken: row.FencingToken, RunLeaseToken: row.RunLeaseToken,
		Execute: execute, Replayed: replayed, Output: output,
	}
}

func normalizeToolIdempotencyMode(mode string, permission string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ToolIdempotencyReplaySafe:
		return ToolIdempotencyReplaySafe
	case ToolIdempotencyKeyed:
		return ToolIdempotencyKeyed
	case ToolIdempotencyFailClosed:
		return ToolIdempotencyFailClosed
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(permission)), "read") {
		return ToolIdempotencyReplaySafe
	}
	return ToolIdempotencyFailClosed
}

func formatClaimTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
