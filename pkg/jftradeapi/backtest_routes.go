package jftradeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/futu/backtest"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func (s *Server) serveBacktestRoutes(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.URL.Path == "/api/v1/backtests" && r.Method == http.MethodGet:
		s.handleBacktestList(w, r)
	case r.URL.Path == "/api/v1/backtests" && r.Method == http.MethodPost:
		s.handleBacktestStart(w, r)
	case r.URL.Path == "/api/v1/backtests/sync" && r.Method == http.MethodPost:
		s.handleBacktestSync(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/backtests/sync/") && r.Method == http.MethodGet:
		s.handleBacktestSyncProgress(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/backtests/sync/") && r.Method == http.MethodDelete:
		s.handleBacktestSyncCancel(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/backtests/") && strings.HasSuffix(r.URL.Path, "/status") && r.Method == http.MethodGet:
		s.handleBacktestStatus(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/backtests/") && r.Method == http.MethodGet:
		s.handleBacktestResult(w, r)
	default:
		return false
	}
	return true
}

type backtestStartRequest struct {
	DefinitionID   string  `json:"definitionId"`
	Symbol         string  `json:"symbol"`
	Interval       string  `json:"interval"`
	StartTime      string  `json:"startTime"`
	EndTime        string  `json:"endTime"`
	InitialBalance float64 `json:"initialBalance"`
	WarmupCandles  int     `json:"warmupCandles"`
	RehabType      string  `json:"rehabType"` // "forward" | "backward" | "none"
}

type backtestRunState struct {
	ID        string               `json:"id"`
	Status    string               `json:"status"` // "queued", "running", "completed", "failed"
	Request   backtestStartRequest `json:"request"`
	Result    *backtest.RunResult  `json:"result,omitempty"`
	CreatedAt string               `json:"createdAt"`
	UpdatedAt string               `json:"updatedAt"`
}

func (s *Server) handleBacktestList(w http.ResponseWriter, r *http.Request) {
	s.writeOK(w, map[string]any{"runs": s.backtestRuns.list()})
}

func (s *Server) handleBacktestStart(w http.ResponseWriter, r *http.Request) {
	var req backtestStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid backtest request")
		return
	}
	if strings.TrimSpace(req.DefinitionID) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "definitionId is required")
		return
	}
	if strings.TrimSpace(req.Symbol) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "symbol is required")
		return
	}
	if strings.TrimSpace(req.Interval) == "" {
		req.Interval = "1m"
	}
	if req.InitialBalance <= 0 {
		req.InitialBalance = 100000
	}
	if req.WarmupCandles <= 0 {
		req.WarmupCandles = 365
	}

	// Look up the strategy definition for the script.
	definition, ok := s.designStore.definition(req.DefinitionID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "strategy definition not found")
		return
	}
	if err := strategydefinition.ValidateScript(definition.SourceFormat, definition.Script); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid startTime, use RFC3339 format")
		return
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid endTime, use RFC3339 format")
		return
	}

	runID := "bt-" + time.Now().UTC().Format("20060102T150405")
	dbPath := s.backtestDBPath()

	run := &backtestRunState{
		ID:        runID,
		Status:    "queued",
		Request:   req,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	s.backtestRuns.add(run)

	// Start the backtest in a goroutine.
	go func() {
		s.backtestRuns.update(runID, func(run *backtestRunState) {
			run.Status = "running"
			run.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		})

		result := backtest.Run(context.Background(), backtest.RunConfig{
			DBPath:         dbPath,
			Symbol:         req.Symbol,
			Interval:       req.Interval,
			SourceFormat:   definition.SourceFormat,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: definition.Script,
			InitialBalance: req.InitialBalance,
			WarmupCandles:  req.WarmupCandles,
			RehabType:      req.RehabType,
		})

		s.backtestRuns.update(runID, func(run *backtestRunState) {
			run.Result = result
			if result.Error != "" {
				run.Status = "failed"
			} else {
				run.Status = "completed"
			}
			run.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		})
	}()

	s.writeOK(w, map[string]any{
		"id":      run.ID,
		"status":  run.Status,
		"message": "backtest queued",
	})
}

func (s *Server) handleBacktestStatus(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimPrefix(r.URL.Path, "/api/v1/backtests/")
	runID = strings.TrimSuffix(runID, "/status")
	runID = strings.TrimSpace(runID)

	run, ok := s.backtestRuns.get(runID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "backtest run not found")
		return
	}

	s.writeOK(w, map[string]any{
		"id":     run.ID,
		"status": run.Status,
	})
}

func (s *Server) handleBacktestResult(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimPrefix(r.URL.Path, "/api/v1/backtests/")
	runID = strings.TrimSpace(runID)

	run, ok := s.backtestRuns.get(runID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "backtest run not found")
		return
	}

	s.writeOK(w, run)
}

func (s *Server) backtestDBPath() string {
	path := os.Getenv("JFTRADE_BACKTEST_DB")
	if path == "" {
		path = "var/jftrade-api/backtest.db"
	}
	return path
}

type backtestSyncRequest struct {
	Symbol    string   `json:"symbol"`
	Intervals []string `json:"intervals"`
	Since     string   `json:"since"`
	Until     string   `json:"until"`
	RehabType string   `json:"rehabType"` // "none" | "forward" | "backward"
}

func (s *Server) handleBacktestSync(w http.ResponseWriter, r *http.Request) {
	var req backtestSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid sync request")
		return
	}
	if strings.TrimSpace(req.Symbol) == "" {
		req.Symbol = "HK.00700"
	}
	if len(req.Intervals) == 0 {
		req.Intervals = []string{"1m", "5m", "15m", "30m", "1h", "1d", "1w"}
	}

	sinceTime := time.Now().AddDate(0, 0, -30)
	if req.Since != "" {
		var err error
		sinceTime, err = time.Parse(time.RFC3339, req.Since)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid since time, use RFC3339")
			return
		}
	}
	untilTime := time.Now()
	if req.Until != "" {
		var err error
		untilTime, err = time.Parse(time.RFC3339, req.Until)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid until time, use RFC3339")
			return
		}
	}

	var intervals []bbgotypes.Interval
	for _, iv := range req.Intervals {
		iv = strings.TrimSpace(iv)
		if iv != "" {
			intervals = append(intervals, bbgotypes.Interval(iv))
		}
	}
	if len(intervals) == 0 {
		intervals = []bbgotypes.Interval{"1m", "5m", "1h", "1d"}
	}

	// Parse rehab type from request, default to forward (前复权).
	rehabType := qotcommonpb.RehabType_RehabType_Forward
	switch strings.ToLower(strings.TrimSpace(req.RehabType)) {
	case "none":
		rehabType = qotcommonpb.RehabType_RehabType_None
	case "backward":
		rehabType = qotcommonpb.RehabType_RehabType_Backward
	case "forward", "":
		rehabType = qotcommonpb.RehabType_RehabType_Forward
	}

	dbPath := s.backtestDBPath()
	store, err := backtest.NewFutuKLineStore(dbPath)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "STORE_ERROR", fmt.Sprintf("open store: %v", err))
		return
	}
	// Do not defer store.Close() here — the goroutine below owns the store lifetime.
	exchange := futu.NewExchange(futu.DefaultOpenDAddr)

	taskID := fmt.Sprintf("sync-%s", time.Now().UTC().Format("20060102T150405"))
	syncCtx, syncCancel := context.WithCancel(context.Background())
	progress := backtest.NewSyncProgress(taskID, req.Symbol, time.Now().UTC())
	s.backtestSyncTasks.add(taskID, progress, syncCancel)

	go func() {
		defer store.Close()
		defer s.backtestSyncTasks.finish(taskID)
		if err := store.SyncKLines(syncCtx, exchange, req.Symbol, intervals, sinceTime, untilTime, rehabType, progress); err != nil {
			snapshot := progress.Snapshot()
			if snapshot != nil && snapshot.Status != "cancelled" {
				log.Printf("backtest sync failed %s: %v", req.Symbol, err)
			}
		}
		snapshot := progress.Snapshot()
		if snapshot != nil {
			log.Printf("backtest sync %s: status=%s retries=%d", req.Symbol, snapshot.Status, snapshot.Retries)
		}
	}()

	s.writeOK(w, map[string]any{
		"taskId":    taskID,
		"symbol":    req.Symbol,
		"intervals": req.Intervals,
		"since":     sinceTime.Format(time.RFC3339),
		"until":     untilTime.Format(time.RFC3339),
		"message":   "sync started",
	})
}

func (s *Server) handleBacktestSyncCancel(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimPrefix(r.URL.Path, "/api/v1/backtests/sync/")
	taskID = strings.TrimSpace(taskID)
	_, ok := s.backtestSyncTasks.cancel(taskID, time.Now().UTC())
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "sync task not found or already completed")
		return
	}
	s.writeOK(w, map[string]any{"taskId": taskID, "status": "cancelled"})
}

func (s *Server) handleBacktestSyncProgress(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimPrefix(r.URL.Path, "/api/v1/backtests/sync/")
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "taskId is required")
		return
	}
	progress, ok := s.backtestSyncTasks.get(taskID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "sync task not found")
		return
	}
	s.writeOK(w, progress)
}
