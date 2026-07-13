package adk

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func (s *Store) ResolveRunInput(ctx context.Context, runID string, payload InputResponseRequest) (Run, bool, error) {
	if s == nil {
		return Run{}, false, fmt.Errorf("store is unavailable")
	}
	runID = strings.TrimSpace(runID)
	requestID := strings.TrimSpace(payload.RequestID)
	if runID == "" || requestID == "" {
		return Run{}, false, fmt.Errorf("%w: runId and requestId are required", errInputRequestInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginWrite(ctx, nil)
	if err != nil {
		return Run{}, false, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	var raw string
	if err := tx.QueryRowxContext(ctx, `SELECT payload_json FROM `+tableRuns+` WHERE id = ?`, runID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, false, fmt.Errorf("%w: %s", errInputRequestNotFound, runID)
		}
		return Run{}, false, err
	}
	var run Run
	if err := json.Unmarshal([]byte(raw), &run); err != nil {
		return Run{}, false, err
	}
	run, changed, err := resolveRunInputState(NormalizeRun(run), requestID, payload.Answers)
	if err != nil || !changed {
		return run, changed, err
	}
	encoded, err := json.Marshal(run)
	if err != nil {
		return Run{}, false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE `+tableRuns+` SET status = ?, payload_json = ?, updated_at = ? WHERE id = ? AND status = ?`,
		run.Status, string(encoded), run.UpdatedAt, run.ID, RunStatusPendingInput)
	if err != nil {
		return Run{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Run{}, false, err
	}
	if rows != 1 {
		return Run{}, false, fmt.Errorf("%w: request was resolved concurrently", errInputRequestConflict)
	}
	if err := tx.Commit(); err != nil {
		return Run{}, false, err
	}
	tx = nil
	return run, true, nil
}

func resolveRunInputState(run Run, requestID string, submitted []InputAnswer) (Run, bool, error) {
	requestIndex := -1
	for index := range run.InputRequests {
		if run.InputRequests[index].ID == requestID {
			requestIndex = index
			break
		}
	}
	if requestIndex < 0 {
		return Run{}, false, fmt.Errorf("%w: request does not match run", errInputRequestConflict)
	}
	request := run.InputRequests[requestIndex]
	answers, err := validateInputAnswers(request, submitted)
	if err != nil {
		return Run{}, false, err
	}
	if request.Status == InputRequestStatusAnswered {
		if inputAnswersEqual(request.Answers, answers) {
			return run, false, nil
		}
		return Run{}, false, fmt.Errorf("%w: run already has a different answer", errInputRequestAlreadyAnswered)
	}
	if request.Status != InputRequestStatusPending || run.Status != RunStatusPendingInput || run.InputRequest == nil || run.InputRequest.ID != request.ID {
		return Run{}, false, fmt.Errorf("%w: request is no longer pending", errInputRequestConflict)
	}
	now := nowString()
	request.Status = InputRequestStatusAnswered
	request.Answers = answers
	request.AnsweredAt = &now
	request.UpdatedAt = now
	run.InputRequests[requestIndex] = request
	run.InputRequest = normalizeInputRequest(&request)
	run.Status = RunStatusRunning
	run.ResumeState = "input_resuming"
	run.Message = "正在根据用户回答继续执行。"
	run.UpdatedAt = now
	run = NormalizeRun(run)
	return run, true, nil
}
