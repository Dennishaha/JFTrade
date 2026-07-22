package adk

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

type approvalStage struct {
	tx *sqliteconn.Tx
}

func (s *Store) beginApprovalStage(ctx context.Context) (*approvalStage, error) {
	tx, err := s.db.BeginWrite(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &approvalStage{tx: tx}, nil
}

func (s *approvalStage) rollback() {
	if s.tx != nil {
		besteffort.LogError(s.tx.Rollback())
	}
}

func (s *approvalStage) commit() error {
	if err := s.tx.Commit(); err != nil {
		return err
	}
	s.tx = nil
	return nil
}

func (s *approvalStage) resolveApproval(ctx context.Context, approvalID, status string) (Approval, bool, bool, error) {
	resolvedAt := nowString()
	var payload string
	changed := true
	err := s.tx.QueryRowxContext(ctx, `UPDATE `+tableApprovals+` SET status = ?, payload_json = json_set(payload_json, '$.status', ?, '$.updatedAt', ?), updated_at = ? WHERE id = ? AND status = ? RETURNING payload_json`,
		status, status, resolvedAt, resolvedAt, approvalID, ApprovalStatusPending,
	).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		changed = false
		err = s.tx.QueryRowxContext(ctx, `SELECT payload_json FROM `+tableApprovals+` WHERE id = ?`, approvalID).Scan(&payload)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return Approval{}, false, false, nil
	}
	if err != nil {
		return Approval{}, false, false, err
	}
	var approval Approval
	if err := json.Unmarshal([]byte(payload), &approval); err != nil {
		return Approval{}, false, false, err
	}
	return approval, changed, true, nil
}

func (s *approvalStage) lockPendingRun(ctx context.Context, runID string) error {
	// A retry may be repairing an approval row that committed before its run
	// snapshot. Acquire the SQLite write lock before reading that snapshot.
	_, err := s.tx.ExecContext(ctx, `UPDATE `+tableRuns+` SET updated_at = updated_at WHERE id = ? AND status = ?`, runID, RunStatusPending)
	return err
}

func (s *approvalStage) pendingRun(ctx context.Context, runID string) (Run, bool, error) {
	var payload string
	if err := s.tx.QueryRowxContext(ctx, `SELECT payload_json FROM `+tableRuns+` WHERE id = ?`, runID).Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, false, nil
		}
		return Run{}, false, err
	}
	var run Run
	if err := json.Unmarshal([]byte(payload), &run); err != nil {
		return Run{}, false, err
	}
	run = NormalizeRun(run)
	return run, run.Status == RunStatusPending, nil
}

func (s *approvalStage) mergeApprovalStates(ctx context.Context, run *Run, resolved Approval) (bool, bool, error) {
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := s.tx.SelectContext(ctx, &rows, `SELECT payload_json FROM `+tableApprovals+` WHERE run_id = ?`, resolved.RunID); err != nil {
		return false, false, err
	}
	authoritative := make(map[string]Approval, len(rows))
	for _, row := range rows {
		var item Approval
		if err := json.Unmarshal([]byte(row.PayloadJSON), &item); err != nil {
			return false, false, err
		}
		authoritative[item.ID] = item
	}
	replaced := false
	denied := false
	for index := range run.PendingApprovals {
		item := &run.PendingApprovals[index]
		if stored, ok := authoritative[item.ID]; ok {
			*item = stored
		}
		replaced = replaced || item.ID == resolved.ID
		denied = denied || item.Status == ApprovalStatusDenied
	}
	return replaced, denied, nil
}

func (s *approvalStage) denyPendingSiblings(ctx context.Context, run *Run) error {
	for index := range run.PendingApprovals {
		item := &run.PendingApprovals[index]
		if item.Status != ApprovalStatusPending {
			continue
		}
		item.Status = ApprovalStatusDenied
		item.UpdatedAt = nowString()
		payload, err := json.Marshal(*item)
		if err != nil {
			return err
		}
		if _, err := s.tx.ExecContext(ctx, `UPDATE `+tableApprovals+` SET status = ?, payload_json = ?, updated_at = ? WHERE id = ? AND status = ?`,
			item.Status, string(payload), item.UpdatedAt, item.ID, ApprovalStatusPending,
		); err != nil {
			return err
		}
	}
	for index := range run.ToolCalls {
		call := &run.ToolCalls[index]
		if call.Status == "PENDING_APPROVAL" {
			call.Status = "DENIED"
			call.RequiresUser = false
			finishToolCall(call)
		}
	}
	return nil
}

func prepareApprovalContinuation(run *Run, denied bool) bool {
	shouldContinue := !runHasPendingApproval(run.PendingApprovals)
	if !shouldContinue {
		return false
	}
	// Approval wait time must not consume the continuation's execution timeout
	// window. Reconciliation uses StartedAt for running runs.
	run.StartedAt = nowString()
	run.ResumeState = "approval_resuming"
	run.Status = RunStatusRunning
	if denied {
		run.Message = "审批已拒绝，正在后台结束运行。"
		return true
	}
	for index := range run.ToolCalls {
		call := &run.ToolCalls[index]
		if call.Status != "PENDING_APPROVAL" {
			continue
		}
		call.Status = "RUNNING"
		call.RequiresUser = false
		call.UpdatedAt = nowString()
	}
	run.Message = "审批已通过，正在后台继续执行。"
	return true
}

func (s *approvalStage) savePendingRun(ctx context.Context, run *Run) error {
	*run = NormalizeRun(*run)
	run.UpdatedAt = nowString()
	payload, err := json.Marshal(*run)
	if err != nil {
		return err
	}
	result, err := s.tx.ExecContext(ctx, `UPDATE `+tableRuns+` SET status = ?, payload_json = ?, updated_at = ? WHERE id = ? AND status = ?`,
		run.Status, string(payload), run.UpdatedAt, run.ID, RunStatusPending,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("claim approval continuation for run %s: updated %d rows", run.ID, affected)
	}
	return nil
}
