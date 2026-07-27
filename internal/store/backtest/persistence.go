package backtest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/chart"
)

const runTable = "backtest_runs"

type runStateRow struct {
	ID          string `db:"id"`
	Status      string `db:"status"`
	RequestJSON string `db:"request_json"`
	ResultJSON  string `db:"result_json"`
	CreatedAt   string `db:"created_at"`
	UpdatedAt   string `db:"updated_at"`
}

func (s *Store) initializeOrValidateSchema() error {
	if s == nil || s.db == nil {
		return nil
	}
	statements := []string{
		strings.Join([]string{
			`CREATE TABLE IF NOT EXISTS ` + runTable + ` (`,
			`  id           TEXT PRIMARY KEY,`,
			`  status       TEXT NOT NULL DEFAULT '',`,
			`  request_json TEXT NOT NULL DEFAULT '',`,
			`  result_json  TEXT NOT NULL DEFAULT '',`,
			`  created_at   TEXT NOT NULL DEFAULT '',`,
			`  updated_at   TEXT NOT NULL DEFAULT ''`,
			`)`,
		}, " "),
		`CREATE INDEX IF NOT EXISTS idx_backtest_runs_updated_at ON ` + runTable + ` (updated_at DESC, id ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_backtest_runs_status ON ` + runTable + ` (status, updated_at DESC)`,
	}
	return sqliteschema.InitializeOrValidate(
		context.Background(), s.db, s.dbPath, "backtest-runs", 1, statements,
		func(ctx context.Context, db sqliteschema.Database) error {
			return sqliteschema.ValidateTable(ctx, db, runTable, []string{
				"id:TEXT:1", "status:TEXT:0", "request_json:TEXT:0", "result_json:TEXT:0",
				"created_at:TEXT:0", "updated_at:TEXT:0",
			})
		},
	)
}

func (s *Store) loadFromDB() error {
	if s == nil || s.db == nil {
		return nil
	}
	rows := []runStateRow{}
	if err := s.db.Select(&rows,
		`SELECT id, status, request_json, '' AS result_json, created_at, updated_at `+
			`FROM `+runTable+` ORDER BY updated_at DESC, id ASC`); err != nil {
		return err
	}

	recoveredAt := time.Now().UTC().Format(time.RFC3339Nano)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range rows {
		run, err := runStateFromRow(row)
		if err != nil {
			return err
		}
		if markRecoveredRun(run, recoveredAt) {
			if err := s.persistRunLocked(run); err != nil {
				return err
			}
		}
		s.runs[run.ID] = run
	}
	return nil
}

func runStateFromRow(row runStateRow) (*btsrv.RunState, error) {
	var request btsrv.StartRequest
	if err := json.Unmarshal([]byte(row.RequestJSON), &request); err != nil {
		return nil, fmt.Errorf("decode backtest request %s: %w", row.ID, err)
	}
	request.ChartType = chart.NormalizeChartType(string(request.ChartType))
	result, err := decodeResultJSON(row.ID, row.ResultJSON)
	if err != nil {
		return nil, err
	}
	return &btsrv.RunState{
		ID: row.ID, Status: row.Status, Request: request, Result: result,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

func decodeResultJSON(runID, resultJSON string) (*bt.RunResult, error) {
	if trimmed := strings.TrimSpace(resultJSON); trimmed != "" && trimmed != "null" {
		decoded := &bt.RunResult{}
		if err := json.Unmarshal([]byte(trimmed), decoded); err != nil {
			return nil, fmt.Errorf("decode backtest result %s: %w", runID, err)
		}
		decoded.ChartType = chart.NormalizeChartType(string(decoded.ChartType))
		return decoded, nil
	}
	return nil, nil
}

func markRecoveredRun(run *btsrv.RunState, recoveredAt string) bool {
	if run == nil || (run.Status != "queued" && run.Status != "running") {
		return false
	}
	run.Status = "failed"
	run.UpdatedAt = recoveredAt
	if run.Result == nil {
		run.Result = &bt.RunResult{}
	}
	if strings.TrimSpace(run.Result.Symbol) == "" {
		run.Result.Symbol = run.Request.Symbol
	}
	if strings.TrimSpace(run.Result.Interval) == "" {
		run.Result.Interval = run.Request.Interval
	}
	if strings.TrimSpace(run.Result.StartTime) == "" {
		run.Result.StartTime = run.Request.StartTime
	}
	if strings.TrimSpace(run.Result.EndTime) == "" {
		run.Result.EndTime = run.Request.EndTime
	}
	if strings.TrimSpace(string(run.Result.ChartType)) == "" {
		run.Result.ChartType = chart.NormalizeChartType(string(run.Request.ChartType))
	} else {
		run.Result.ChartType = chart.NormalizeChartType(string(run.Result.ChartType))
	}
	if strings.TrimSpace(run.Result.Error) == "" {
		run.Result.Error = RecoveredRunErrorText
	}
	return true
}

func (s *Store) persistRunLocked(run *btsrv.RunState) error {
	if s == nil || s.db == nil {
		return nil
	}
	snapshot := cloneRunState(run)
	requestJSON, err := json.Marshal(snapshot.Request)
	if err != nil {
		return err
	}
	resultJSON := ""
	if snapshot.Result != nil {
		encodedResult, err := json.Marshal(snapshot.Result)
		if err != nil {
			return err
		}
		resultJSON = string(encodedResult)
	}
	_, err = s.db.ExecContext(context.Background(),
		`INSERT INTO `+runTable+` (id, status, request_json, result_json, created_at, updated_at) `+
			`VALUES (?, ?, ?, ?, ?, ?) `+
			`ON CONFLICT(id) DO UPDATE SET `+
			`status=excluded.status, request_json=excluded.request_json, result_json=excluded.result_json, `+
			`created_at=excluded.created_at, updated_at=excluded.updated_at`,
		snapshot.ID, snapshot.Status, string(requestJSON), resultJSON, snapshot.CreatedAt, snapshot.UpdatedAt,
	)
	return err
}

func (s *Store) deleteFromDBLocked(runID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM `+runTable+` WHERE id = ?`, runID)
	return err
}

func (s *Store) loadResult(runID string) (*bt.RunResult, bool, error) {
	var resultJSON string
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT result_json FROM `+runTable+` WHERE id = ?`, runID,
	).Scan(&resultJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	result, err := decodeResultJSON(runID, resultJSON)
	return result, true, err
}
