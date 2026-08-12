package persistence

import (
	"context"
	"encoding/json"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jmoiron/sqlx"
)

// persistedRun keeps provider wire details in the store without exposing them
// through the public Run JSON contract.
type persistedRun struct {
	jfadkmodel.Run
	ReasoningEffortField string `json:"reasoningEffortField,omitempty"`
	ReasoningEffortValue string `json:"reasoningEffortValue,omitempty"`
}

func encodeRun(run jfadkmodel.Run) ([]byte, error) {
	return json.Marshal(persistedRun{
		Run: run, ReasoningEffortField: run.ReasoningEffortField,
		ReasoningEffortValue: run.ReasoningEffortValue,
	})
}

func decodeRun(payload []byte) (jfadkmodel.Run, error) {
	var stored persistedRun
	if err := json.Unmarshal(payload, &stored); err != nil {
		return jfadkmodel.Run{}, err
	}
	run := stored.Run
	run.ReasoningEffortField = stored.ReasoningEffortField
	run.ReasoningEffortValue = stored.ReasoningEffortValue
	return run, nil
}

func (s *StoreCore) SaveRun(ctx context.Context, run jfadkmodel.Run) error {
	run, err := s.PrepareRunForSave(ctx, run)
	if err != nil {
		return err
	}
	return s.SavePreparedRun(ctx, run)
}

// PrepareRunForSave normalizes a run and preserves user goal pause lifecycle
// before persisting.
func (s *StoreCore) PrepareRunForSave(ctx context.Context, run jfadkmodel.Run) (jfadkmodel.Run, error) {
	if run.CreatedAt == "" {
		run.CreatedAt = jfadkmodel.NowString()
	}
	if s.isRootLoopGoalRun(run) {
		latest, ok, err := s.Run(ctx, run.ID)
		if err != nil {
			return jfadkmodel.Run{}, err
		}
		if ok {
			run = s.preserveUserGoalPauseLifecycle(latest, run)
		}
	}
	run = s.normalizeRun(run)
	run.UpdatedAt = jfadkmodel.NowString()
	return run, nil
}

// SavePreparedRun persists an already normalized run, honoring the runtime's
// execution lease when one is attached to the context.
func (s *StoreCore) SavePreparedRun(ctx context.Context, run jfadkmodel.Run) error {
	if _, leased := s.runLeaseFromContext(ctx); !leased {
		return savePreparedRunWithExecutor(ctx, s.DB(), run)
	}
	tx, err := s.DB().BeginWrite(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.lockRunLeaseFromContext(ctx, tx, run.ID); err != nil {
		return err
	}
	if err := savePreparedRunWithExecutor(ctx, tx, run); err != nil {
		return err
	}
	return tx.Commit()
}

func savePreparedRunWithExecutor(ctx context.Context, executor sqlx.ExtContext, run jfadkmodel.Run) error {
	payload, err := encodeRun(run)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO `+tableRuns+` (id, session_id, agent_id, status, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET session_id = excluded.session_id, agent_id = excluded.agent_id, status = excluded.status, payload_json = excluded.payload_json, updated_at = excluded.updated_at WHERE (`+tableRuns+`.status NOT IN (?, ?, ?, ?, ?) OR (`+tableRuns+`.status = excluded.status AND `+tableRuns+`.status <> ?) OR (`+tableRuns+`.status = ? AND COALESCE(json_extract(`+tableRuns+`.payload_json, '$.finalMessageId'), '') = '' AND COALESCE(json_extract(excluded.payload_json, '$.finalMessageId'), '') <> '') OR (`+tableRuns+`.status = ? AND json_extract(`+tableRuns+`.payload_json, '$.workflowStatus') = ? AND excluded.status IN (?, ?, ?, ?, ?)) OR (`+tableRuns+`.status = ? AND excluded.status = ? AND json_array_length(json_extract(excluded.payload_json, '$.pendingApprovals')) > 0)) AND NOT (`+tableRuns+`.status = ? AND COALESCE(json_extract(`+tableRuns+`.payload_json, '$.resumeState'), '') = ? AND excluded.status = ? AND NOT EXISTS (SELECT 1 FROM json_each(excluded.payload_json, '$.pendingApprovals') AS next_approval WHERE UPPER(TRIM(COALESCE(json_extract(next_approval.value, '$.status'), ''))) = ? AND TRIM(COALESCE(json_extract(next_approval.value, '$.id'), '')) <> '' AND EXISTS (SELECT 1 FROM `+tableApprovals+` AS durable_approval WHERE durable_approval.id = TRIM(json_extract(next_approval.value, '$.id')) AND durable_approval.run_id = `+tableRuns+`.id AND durable_approval.status = ?) AND (CASE WHEN TRIM(COALESCE(json_extract(next_approval.value, '$.id'), '')) <> '' THEN 'id:' || TRIM(json_extract(next_approval.value, '$.id')) WHEN TRIM(COALESCE(json_extract(next_approval.value, '$.confirmationCallId'), '')) <> '' THEN 'confirmation:' || TRIM(json_extract(next_approval.value, '$.confirmationCallId')) WHEN TRIM(COALESCE(json_extract(next_approval.value, '$.functionCallId'), '')) <> '' THEN 'function:' || TRIM(json_extract(next_approval.value, '$.functionCallId')) ELSE '' END) <> '' AND NOT EXISTS (SELECT 1 FROM json_each(`+tableRuns+`.payload_json, '$.pendingApprovals') AS current_approval WHERE (CASE WHEN TRIM(COALESCE(json_extract(current_approval.value, '$.id'), '')) <> '' THEN 'id:' || TRIM(json_extract(current_approval.value, '$.id')) WHEN TRIM(COALESCE(json_extract(current_approval.value, '$.confirmationCallId'), '')) <> '' THEN 'confirmation:' || TRIM(json_extract(current_approval.value, '$.confirmationCallId')) WHEN TRIM(COALESCE(json_extract(current_approval.value, '$.functionCallId'), '')) <> '' THEN 'function:' || TRIM(json_extract(current_approval.value, '$.functionCallId')) ELSE '' END) = (CASE WHEN TRIM(COALESCE(json_extract(next_approval.value, '$.id'), '')) <> '' THEN 'id:' || TRIM(json_extract(next_approval.value, '$.id')) WHEN TRIM(COALESCE(json_extract(next_approval.value, '$.confirmationCallId'), '')) <> '' THEN 'confirmation:' || TRIM(json_extract(next_approval.value, '$.confirmationCallId')) WHEN TRIM(COALESCE(json_extract(next_approval.value, '$.functionCallId'), '')) <> '' THEN 'function:' || TRIM(json_extract(next_approval.value, '$.functionCallId')) ELSE '' END))))`,
		run.ID, run.SessionID, run.AgentID, run.Status, string(payload), run.CreatedAt, run.UpdatedAt,
		jfadkmodel.RunStatusCompleted, jfadkmodel.RunStatusFailed, jfadkmodel.RunStatusDenied, jfadkmodel.RunStatusCancelled, jfadkmodel.RunStatusTimedOut,
		jfadkmodel.RunStatusCancelled, jfadkmodel.RunStatusCancelled, jfadkmodel.RunStatusCompleted, jfadkmodel.WorkflowStatusRunning,
		jfadkmodel.RunStatusCompleted, jfadkmodel.RunStatusFailed, jfadkmodel.RunStatusDenied, jfadkmodel.RunStatusCancelled, jfadkmodel.RunStatusTimedOut,
		jfadkmodel.RunStatusCompleted, jfadkmodel.RunStatusPending,
		jfadkmodel.RunStatusRunning, "approval_resuming", jfadkmodel.RunStatusPending, jfadkmodel.ApprovalStatusPending, jfadkmodel.ApprovalStatusPending,
	)
	return err
}

func (s *StoreCore) SaveRunAndDenyPendingApprovals(ctx context.Context, run jfadkmodel.Run) error {
	run, err := s.PrepareRunForSave(ctx, run)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.DB().BeginWrite(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			jftradeErr := tx.Rollback()
			besteffort.LogError(jftradeErr)
		}
	}()
	if err := s.lockRunLeaseFromContext(ctx, tx, run.ID); err != nil {
		return err
	}
	rows := []struct {
		PayloadJSON string `db:"payload_json"`
	}{}
	if err := tx.SelectContext(ctx, &rows, `SELECT payload_json FROM `+tableApprovals+` WHERE run_id = ? AND status = ?`, run.ID, jfadkmodel.ApprovalStatusPending); err != nil {
		return err
	}
	for _, row := range rows {
		var approval jfadkmodel.Approval
		if err := json.Unmarshal([]byte(row.PayloadJSON), &approval); err != nil {
			return err
		}
		approval.Status = jfadkmodel.ApprovalStatusDenied
		approval.UpdatedAt = jfadkmodel.NowString()
		payload, err := json.Marshal(approval)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+tableApprovals+` SET status = ?, payload_json = ?, updated_at = ? WHERE id = ? AND status = ?`,
			approval.Status, string(payload), approval.UpdatedAt, approval.ID, jfadkmodel.ApprovalStatusPending,
		); err != nil {
			return err
		}
	}
	if err := savePreparedRunWithExecutor(ctx, tx, run); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (s *StoreCore) Run(ctx context.Context, id string) (jfadkmodel.Run, bool, error) {
	var stored persistedRun
	ok, err := s.GetJSON(ctx, tableRuns, id, &stored)
	if err != nil || !ok {
		return jfadkmodel.Run{}, ok, err
	}
	run := stored.Run
	run.ReasoningEffortField = stored.ReasoningEffortField
	run.ReasoningEffortValue = stored.ReasoningEffortValue
	return s.normalizeRun(run), true, nil
}

func (s *StoreCore) ListRuns(ctx context.Context) ([]jfadkmodel.Run, error) {
	var stored []persistedRun
	if err := s.ListJSON(ctx, tableRuns, "created_at DESC, id ASC", &stored); err != nil {
		return nil, err
	}
	runs := make([]jfadkmodel.Run, 0, len(stored))
	for _, item := range stored {
		item.Run.ReasoningEffortField = item.ReasoningEffortField
		item.Run.ReasoningEffortValue = item.ReasoningEffortValue
		runs = append(runs, s.normalizeRun(item.Run))
	}
	return runs, nil
}

func (s *StoreCore) ListRunsPage(ctx context.Context, status string, agentID string, sessionID string, limit int, offset int) ([]jfadkmodel.Run, int, error) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if status = strings.ToUpper(strings.TrimSpace(status)); status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	if agentID = strings.TrimSpace(agentID); agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, sessionID)
	}
	var stored []persistedRun
	total, err := s.ListJSONPage(ctx, tableRuns, clauses, args, "created_at DESC, id ASC", limit, offset, &stored)
	runs := make([]jfadkmodel.Run, 0, len(stored))
	for _, item := range stored {
		item.Run.ReasoningEffortField = item.ReasoningEffortField
		item.Run.ReasoningEffortValue = item.ReasoningEffortValue
		runs = append(runs, s.normalizeRun(item.Run))
	}
	return runs, total, err
}
