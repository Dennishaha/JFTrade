package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func (s *StoreCore) ResolveRunInput(ctx context.Context, runID string, payload jfadkmodel.InputResponseRequest) (jfadkmodel.Run, bool, error) {
	if s == nil || s.DB() == nil {
		return jfadkmodel.Run{}, false, fmt.Errorf("store is unavailable")
	}
	runID = strings.TrimSpace(runID)
	requestID := strings.TrimSpace(payload.RequestID)
	if runID == "" || requestID == "" {
		return jfadkmodel.Run{}, false, fmt.Errorf("%w: runId and requestId are required", jfadkmodel.ErrInputRequestInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.DB().BeginWrite(ctx, nil)
	if err != nil {
		return jfadkmodel.Run{}, false, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	var raw string
	if err := tx.QueryRowxContext(ctx, `SELECT payload_json FROM `+tableRuns+` WHERE id = ?`, runID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return jfadkmodel.Run{}, false, fmt.Errorf("%w: %s", jfadkmodel.ErrInputRequestNotFound, runID)
		}
		return jfadkmodel.Run{}, false, err
	}
	run, err := decodeRun([]byte(raw))
	if err != nil {
		return jfadkmodel.Run{}, false, err
	}
	run, changed, err := s.resolveRunInputState(s.normalizeRun(run), requestID, payload.Answers)
	if err != nil || !changed {
		return run, changed, err
	}
	encoded, err := encodeRun(run)
	if err != nil {
		return jfadkmodel.Run{}, false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE `+tableRuns+` SET status = ?, payload_json = ?, updated_at = ? WHERE id = ? AND status = ?`,
		run.Status, string(encoded), run.UpdatedAt, run.ID, jfadkmodel.RunStatusPendingInput)
	if err != nil {
		return jfadkmodel.Run{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return jfadkmodel.Run{}, false, err
	}
	if rows != 1 {
		return jfadkmodel.Run{}, false, fmt.Errorf("%w: request was resolved concurrently", jfadkmodel.ErrInputRequestConflict)
	}
	if err := tx.Commit(); err != nil {
		return jfadkmodel.Run{}, false, err
	}
	tx = nil
	return run, true, nil
}

func (s *StoreCore) resolveRunInputState(run jfadkmodel.Run, requestID string, submitted []jfadkmodel.InputAnswer) (jfadkmodel.Run, bool, error) {
	requestIndex := -1
	for index := range run.InputRequests {
		if run.InputRequests[index].ID == requestID {
			requestIndex = index
			break
		}
	}
	if requestIndex < 0 {
		return jfadkmodel.Run{}, false, fmt.Errorf("%w: request does not match run", jfadkmodel.ErrInputRequestConflict)
	}
	request := run.InputRequests[requestIndex]
	answers, err := jfadkmodel.ValidateInputAnswers(request, submitted)
	if err != nil {
		return jfadkmodel.Run{}, false, err
	}
	if request.Status == jfadkmodel.InputRequestStatusAnswered {
		if jfadkmodel.InputAnswersEqual(request.Answers, answers) {
			return run, false, nil
		}
		return jfadkmodel.Run{}, false, fmt.Errorf("%w: run already has a different answer", jfadkmodel.ErrInputRequestAlreadyAnswered)
	}
	if request.Status != jfadkmodel.InputRequestStatusPending || run.Status != jfadkmodel.RunStatusPendingInput || run.InputRequest == nil || run.InputRequest.ID != request.ID {
		return jfadkmodel.Run{}, false, fmt.Errorf("%w: request is no longer pending", jfadkmodel.ErrInputRequestConflict)
	}
	now := jfadkmodel.NowString()
	request.Status = jfadkmodel.InputRequestStatusAnswered
	request.Answers = answers
	request.AnsweredAt = &now
	request.UpdatedAt = now
	run.InputRequests[requestIndex] = request
	run.InputRequest = jfadkmodel.NormalizeInputRequest(&request)
	run.Status = jfadkmodel.RunStatusRunning
	run.ResumeState = "input_resuming"
	run.Message = "正在根据用户回答继续执行。"
	run.UpdatedAt = now
	run = s.normalizeRun(run)
	return run, true, nil
}
