package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func (s *StoreCore) SaveApproval(ctx context.Context, approval jfadkmodel.Approval) error {
	if approval.CreatedAt == "" {
		approval.CreatedAt = jfadkmodel.NowString()
	}
	approval.UpdatedAt = jfadkmodel.NowString()
	payload, err := json.Marshal(approval)
	if err != nil {
		return err
	}
	_, err = s.DB().ExecContext(ctx, `INSERT INTO `+tableApprovals+` (id, run_id, agent_id, status, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET run_id = excluded.run_id, agent_id = excluded.agent_id, status = excluded.status, payload_json = excluded.payload_json, updated_at = excluded.updated_at`, approval.ID, approval.RunID, approval.AgentID, approval.Status, string(payload), approval.CreatedAt, approval.UpdatedAt)
	return err
}

func (s *StoreCore) SaveApprovalIfConfirmationAbsent(ctx context.Context, approval jfadkmodel.Approval) (jfadkmodel.Approval, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	confirmationID := strings.TrimSpace(approval.ConfirmationCallID)
	if confirmationID != "" {
		existing, ok, err := s.approvalByConfirmationCallID(ctx, confirmationID)
		if err != nil {
			return jfadkmodel.Approval{}, false, err
		}
		if ok {
			return existing, false, nil
		}
	}
	if approval.CreatedAt == "" {
		approval.CreatedAt = jfadkmodel.NowString()
	}
	approval.UpdatedAt = jfadkmodel.NowString()
	payload, err := json.Marshal(approval)
	if err != nil {
		return jfadkmodel.Approval{}, false, err
	}
	_, err = s.DB().ExecContext(ctx, `INSERT INTO `+tableApprovals+` (id, run_id, agent_id, status, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, approval.ID, approval.RunID, approval.AgentID, approval.Status, string(payload), approval.CreatedAt, approval.UpdatedAt)
	if err != nil {
		if confirmationID != "" {
			existing, ok, lookupErr := s.approvalByConfirmationCallID(ctx, confirmationID)
			if lookupErr == nil && ok {
				return existing, false, nil
			}
		}
		return jfadkmodel.Approval{}, false, err
	}
	return approval, true, nil
}

func (s *StoreCore) ApprovalByConfirmationCallID(ctx context.Context, confirmationID string) (jfadkmodel.Approval, bool, error) {
	return s.approvalByConfirmationCallID(ctx, strings.TrimSpace(confirmationID))
}

// ApprovalByConfirmationCallIDQuery locates the oldest approval row for a
// confirmation call id.
const ApprovalByConfirmationCallIDQuery = `SELECT payload_json FROM ` + tableApprovals + `
	WHERE COALESCE(json_extract(payload_json, '$.confirmationCallId'), '') <> ''
		AND json_extract(payload_json, '$.confirmationCallId') = ?
	ORDER BY created_at ASC, id ASC
	LIMIT 1`

func (s *StoreCore) approvalByConfirmationCallID(ctx context.Context, confirmationID string) (jfadkmodel.Approval, bool, error) {
	var approval jfadkmodel.Approval
	if confirmationID == "" {
		return approval, false, nil
	}
	var payload string
	err := s.DB().QueryRowxContext(ctx, ApprovalByConfirmationCallIDQuery, confirmationID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return approval, false, nil
	}
	if err != nil {
		return approval, false, err
	}
	if err := json.Unmarshal([]byte(payload), &approval); err != nil {
		return jfadkmodel.Approval{}, false, err
	}
	return approval, true, nil
}

func (s *StoreCore) ResolvePendingApproval(ctx context.Context, id string, status string) (jfadkmodel.Approval, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var approval jfadkmodel.Approval
	ok, err := s.GetJSON(ctx, tableApprovals, id, &approval)
	if err != nil || !ok {
		return jfadkmodel.Approval{}, ok, err
	}
	if approval.Status != jfadkmodel.ApprovalStatusPending {
		return approval, false, nil
	}
	approval.Status = status
	approval.UpdatedAt = jfadkmodel.NowString()
	payload, err := json.Marshal(approval)
	if err != nil {
		return jfadkmodel.Approval{}, false, err
	}
	result, err := s.DB().ExecContext(ctx, `UPDATE `+tableApprovals+` SET status = ?, payload_json = ?, updated_at = ? WHERE id = ? AND status = ?`, approval.Status, string(payload), approval.UpdatedAt, approval.ID, jfadkmodel.ApprovalStatusPending)
	if err != nil {
		return jfadkmodel.Approval{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return jfadkmodel.Approval{}, false, err
	}
	if affected == 0 {
		current, currentOK, currentErr := s.Approval(ctx, id)
		return current, false, currentErrOrNotFound(currentErr, currentOK)
	}
	return approval, true, nil
}

// ResolveAndStageApproval atomically resolves one approval, merges all
// authoritative sibling states into the embedded run, and claims the sole
// pending-to-resuming transition.
func (s *StoreCore) ResolveAndStageApproval(ctx context.Context, approvalID, status string) (jfadkmodel.Approval, bool, *jfadkmodel.Run, bool, error) {
	if s == nil || s.DB() == nil {
		return jfadkmodel.Approval{}, false, nil, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	stage, err := s.beginApprovalStage(ctx)
	if err != nil {
		return jfadkmodel.Approval{}, false, nil, false, err
	}
	defer stage.rollback()

	approval, changed, found, err := stage.resolveApproval(ctx, approvalID, status)
	if err != nil {
		return jfadkmodel.Approval{}, false, nil, false, err
	}
	if !found {
		return jfadkmodel.Approval{}, false, nil, false, stage.commit()
	}
	if approval.Status != status {
		return approval, false, nil, false, stage.commit()
	}
	if !changed {
		if err := stage.lockPendingRun(ctx, approval.RunID); err != nil {
			return jfadkmodel.Approval{}, false, nil, false, err
		}
	}

	run, pending, err := stage.pendingRun(ctx, approval.RunID)
	if err != nil {
		return jfadkmodel.Approval{}, false, nil, false, err
	}
	if !pending {
		return approval, changed, nil, false, stage.commit()
	}

	replaced, denied, err := stage.mergeApprovalStates(ctx, &run, approval)
	if err != nil {
		return jfadkmodel.Approval{}, false, nil, false, err
	}
	if !replaced {
		return approval, changed, &run, false, stage.commit()
	}

	if denied {
		if err := stage.denyPendingSiblings(ctx, &run); err != nil {
			return jfadkmodel.Approval{}, false, nil, false, err
		}
	}

	shouldContinue := prepareApprovalContinuation(&run, denied)
	if err := stage.savePendingRun(ctx, &run); err != nil {
		return jfadkmodel.Approval{}, false, nil, false, err
	}
	return approval, changed, &run, shouldContinue, stage.commit()
}

func (s *StoreCore) Approval(ctx context.Context, id string) (jfadkmodel.Approval, bool, error) {
	var approval jfadkmodel.Approval
	ok, err := s.GetJSON(ctx, tableApprovals, id, &approval)
	return approval, ok, err
}

func (s *StoreCore) ListApprovals(ctx context.Context) ([]jfadkmodel.Approval, error) {
	var approvals []jfadkmodel.Approval
	return approvals, s.ListJSON(ctx, tableApprovals, "updated_at DESC, id ASC", &approvals)
}

func (s *StoreCore) ListApprovalsPage(ctx context.Context, status string, agentID string, limit int, offset int) ([]jfadkmodel.Approval, int, error) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if status = strings.ToUpper(strings.TrimSpace(status)); status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	if agentID = strings.TrimSpace(agentID); agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	var approvals []jfadkmodel.Approval
	total, err := s.ListJSONPage(ctx, tableApprovals, clauses, args, "updated_at DESC, id ASC", limit, offset, &approvals)
	return approvals, total, err
}
