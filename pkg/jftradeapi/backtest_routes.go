package jftradeapi

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"
	"github.com/gin-gonic/gin"
	"github.com/jftrade/jftrade-main/pkg/backtest"
	"github.com/jftrade/jftrade-main/pkg/futu"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

type backtestStartRequest struct {
	DefinitionID      string  `json:"definitionId"`
	DefinitionVersion string  `json:"definitionVersion,omitempty"`
	Market            string  `json:"market"`
	Code              string  `json:"code"`
	Symbol            string  `json:"symbol"`
	Interval          string  `json:"interval"`
	StartTime         string  `json:"startTime"`
	EndTime           string  `json:"endTime"`
	InitialBalance    float64 `json:"initialBalance"`
	RehabType         string  `json:"rehabType"` // "forward" | "backward" | "none"
	UseExtendedHours  *bool   `json:"useExtendedHours,omitempty"`
}

type backtestRunState struct {
	ID        string               `json:"id"`
	Status    string               `json:"status"` // "queued", "running", "completed", "failed"
	Request   backtestStartRequest `json:"request"`
	Result    *backtest.RunResult  `json:"result,omitempty"`
	CreatedAt string               `json:"createdAt"`
	UpdatedAt string               `json:"updatedAt"`
}

// handleBacktestList godoc
// @Summary 读取回测列表
// @Tags backtest
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/backtests [get]
func (s *Server) handleBacktestList(c *gin.Context) {
	s.writeOK(c, map[string]any{"runs": s.backtestRuns.listLightweight()})
}

// handleBacktestStart godoc
// @Summary 启动回测
// @Tags backtest
// @Accept json
// @Produce json
// @Param request body backtestStartRequest true "回测请求"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/backtests [post]
func (s *Server) handleBacktestStart(c *gin.Context) {
	var req backtestStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid backtest request")
		return
	}
	run, err := s.enqueueBacktest(req)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		s.writeError(c, status, "BAD_REQUEST", err.Error())
		return
	}
	s.writeOK(c, map[string]any{
		"id":      run.ID,
		"status":  run.Status,
		"message": "backtest queued",
	})
}

func (s *Server) enqueueBacktest(req backtestStartRequest) (*backtestRunState, error) {
	if strings.TrimSpace(req.DefinitionID) == "" {
		return nil, fmt.Errorf("definitionId is required")
	}
	instrument, err := normalizeInstrumentInput(req.Market, req.Symbol, req.Code)
	if err != nil {
		return nil, err
	}
	req.Market = instrument.Market
	req.Code = instrument.Code
	req.Symbol = instrument.Symbol
	if strings.TrimSpace(req.Interval) == "" {
		req.Interval = "1m"
	}
	if req.InitialBalance <= 0 {
		req.InitialBalance = 100000
	}

	// Look up the strategy definition for the script.
	definition, ok := s.designStore.definition(req.DefinitionID)
	if !ok {
		return nil, fmt.Errorf("strategy definition not found")
	}
	if err := strategydefinition.ValidateScript(definition.SourceFormat, definition.Script); err != nil {
		return nil, err
	}
	req.DefinitionVersion = definition.Version

	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		return nil, fmt.Errorf("invalid startTime, use RFC3339 format")
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("invalid endTime, use RFC3339 format")
	}
	if !endTime.After(startTime) {
		return nil, fmt.Errorf("endTime must be after startTime")
	}

	runID := "bt-" + time.Now().UTC().Format("20060102T150405.000000000")
	dbPath := s.backtestDBPath()

	run := &backtestRunState{
		ID:        runID,
		Status:    "queued",
		Request:   req,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.backtestRuns.add(run); err != nil {
		return nil, fmt.Errorf("persist backtest run: %w", err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.backtestRuns.setCancel(runID, cancel)

	// Start the backtest in a goroutine.
	go func() {
		defer s.backtestRuns.setCancel(runID, nil)
		defer cancel()
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("backtest run %s panicked: %v\n%s", runID, recovered, string(debug.Stack()))
				s.finishBacktestRun(runID, "failed", backtestFailureResult(req, fmt.Sprintf("backtest panic: %v", recovered)))
			}
		}()
		if _, err := s.backtestRuns.update(runID, func(run *backtestRunState) {
			run.Status = "running"
			run.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}); err != nil {
			log.Printf("backtest run store update(%s running) failed: %v", runID, err)
		}

		result := backtest.Run(runCtx, backtest.RunConfig{
			DBPath:           dbPath,
			Symbol:           req.Symbol,
			Interval:         req.Interval,
			SourceFormat:     definition.SourceFormat,
			StartTime:        startTime,
			EndTime:          endTime,
			StrategyScript:   definition.Script,
			InitialBalance:   req.InitialBalance,
			RehabType:        req.RehabType,
			UseExtendedHours: req.UseExtendedHours,
		})

		if result == nil {
			result = backtestFailureResult(req, "backtest returned no result")
		}
		status := "completed"
		if runCtx.Err() == context.Canceled {
			status = "cancelled"
		} else if strings.TrimSpace(result.Error) != "" {
			status = "failed"
		}
		s.finishBacktestRun(runID, status, result)
	}()

	return run, nil
}

func backtestFailureResult(req backtestStartRequest, message string) *backtest.RunResult {
	return &backtest.RunResult{
		Symbol:       req.Symbol,
		Interval:     req.Interval,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		FinalBalance: req.InitialBalance,
		Error:        message,
	}
}

func (s *Server) finishBacktestRun(runID string, status string, result *backtest.RunResult) {
	mutate := func(run *backtestRunState) {
		run.Result = result
		run.Status = status
		run.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if _, err := s.backtestRuns.update(runID, mutate); err != nil {
		log.Printf("backtest run store update(%s %s) failed: %v", runID, status, err)
		s.backtestRuns.updateMemoryOnly(runID, mutate)
	}
}

// handleBacktestStatus godoc
// @Summary 读取回测状态
// @Tags backtest
// @Produce json
// @Param runId path string true "回测运行 ID"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/backtests/{runId}/status [get]
func (s *Server) handleBacktestStatus(c *gin.Context) {
	var uri backtestRunURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "backtest run id is invalid")
		return
	}
	runID := strings.TrimSpace(uri.RunID)

	run, ok := s.backtestRuns.get(runID)
	if !ok {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "backtest run not found")
		return
	}

	s.writeOK(c, map[string]any{
		"id":     run.ID,
		"status": run.Status,
	})
}

// handleBacktestResult godoc
// @Summary 读取回测结果
// @Tags backtest
// @Produce json
// @Param runId path string true "回测运行 ID"
// @Success 200 {object} envelope{data=backtestRunState}
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/backtests/{runId} [get]
func (s *Server) handleBacktestResult(c *gin.Context) {
	var uri backtestRunURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "backtest run id is invalid")
		return
	}
	runID := strings.TrimSpace(uri.RunID)

	run, ok, err := s.backtestRuns.getFull(runID)
	if err != nil {
		s.writeError(c, http.StatusInternalServerError, "BACKTEST_RUN_STORE_FAILED", "load backtest result failed")
		return
	}
	if !ok {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "backtest run not found")
		return
	}

	s.writeOK(c, run)
}

// handleBacktestDelete godoc
// @Summary 删除回测记录
// @Tags backtest
// @Produce json
// @Param runId path string true "回测运行 ID"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/backtests/{runId} [delete]
func (s *Server) handleBacktestDelete(c *gin.Context) {
	var uri backtestRunURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "backtest run id is invalid")
		return
	}
	runID := strings.TrimSpace(uri.RunID)
	if runID == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "backtest run id is required")
		return
	}

	run, ok := s.backtestRuns.get(runID)
	if !ok {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "backtest run not found")
		return
	}
	if run.Status != "completed" && run.Status != "failed" && run.Status != "cancelled" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "only completed, failed or cancelled backtest runs can be deleted")
		return
	}

	if _, deleted, err := s.backtestRuns.delete(runID); err != nil {
		s.writeError(c, http.StatusInternalServerError, "BACKTEST_RUN_STORE_FAILED", "delete backtest run failed")
		return
	} else if !deleted {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "backtest run not found")
		return
	}

	s.writeOK(c, map[string]any{"deleted": true, "id": runID})
}

func (s *Server) backtestDBPath() string {
	return deriveBacktestDBPath()
}

type backtestSyncRequest struct {
	Market       string   `json:"market"`
	Code         string   `json:"code"`
	Symbol       string   `json:"symbol"`
	Intervals    []string `json:"intervals"`
	Since        string   `json:"since"`
	Until        string   `json:"until"`
	RehabType    string   `json:"rehabType"` // "none" | "forward" | "backward"
	SessionScope string   `json:"sessionScope,omitempty"`
}

// handleBacktestSync godoc
// @Summary 启动历史数据同步
// @Tags backtest
// @Accept json
// @Produce json
// @Param request body backtestSyncRequest true "同步请求"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/backtests/sync [post]
func (s *Server) handleBacktestSync(c *gin.Context) {
	var req backtestSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid sync request")
		return
	}
	if strings.TrimSpace(req.Symbol) == "" && strings.TrimSpace(req.Code) == "" {
		req.Market = "HK"
		req.Code = "00700"
	}
	instrument, err := normalizeInstrumentInput(req.Market, req.Symbol, req.Code)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	req.Market = instrument.Market
	req.Code = instrument.Code
	req.Symbol = instrument.Symbol
	if len(req.Intervals) == 0 {
		req.Intervals = []string{"1m", "5m", "15m", "30m", "1h", "1d", "1w"}
	}

	sinceTime := time.Now().AddDate(0, 0, -30)
	if req.Since != "" {
		var err error
		sinceTime, err = time.Parse(time.RFC3339, req.Since)
		if err != nil {
			s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid since time, use RFC3339")
			return
		}
	}
	untilTime := time.Now()
	if req.Until != "" {
		var err error
		untilTime, err = time.Parse(time.RFC3339, req.Until)
		if err != nil {
			s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid until time, use RFC3339")
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

	req.SessionScope = normalizeBacktestSyncSessionScope(req.SessionScope)
	intervals = planBacktestSyncIntervals(req.Symbol, intervals, req.SessionScope)

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
		s.writeError(c, http.StatusInternalServerError, "STORE_ERROR", fmt.Sprintf("open store: %v", err))
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
		if err := store.SyncKLines(syncCtx, exchange, req.Symbol, intervals, sinceTime, untilTime, rehabType, req.SessionScope, progress); err != nil {
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

	s.writeOK(c, map[string]any{
		"taskId":       taskID,
		"symbol":       req.Symbol,
		"intervals":    intervals,
		"since":        sinceTime.Format(time.RFC3339),
		"until":        untilTime.Format(time.RFC3339),
		"sessionScope": req.SessionScope,
		"message":      "sync started",
	})
}

func normalizeBacktestSyncSessionScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "regular":
		return "regular"
	case "extended":
		return "extended"
	default:
		return "legacy"
	}
}

func planBacktestSyncIntervals(symbol string, requested []bbgotypes.Interval, sessionScope string) []bbgotypes.Interval {
	planned := make([]bbgotypes.Interval, 0, len(requested))
	seen := make(map[bbgotypes.Interval]struct{}, len(requested))
	for _, interval := range requested {
		plannedInterval := planBacktestSyncInterval(symbol, interval, sessionScope)
		if _, ok := seen[plannedInterval]; ok {
			continue
		}
		seen[plannedInterval] = struct{}{}
		planned = append(planned, plannedInterval)
	}
	return planned
}

func planBacktestSyncInterval(symbol string, interval bbgotypes.Interval, sessionScope string) bbgotypes.Interval {
	duration := interval.Duration()
	if interval == bbgotypes.Interval("3d") || interval == bbgotypes.Interval("2w") {
		return bbgotypes.Interval1d
	}
	if duration > time.Hour && duration < 24*time.Hour {
		return bbgotypes.Interval1h
	}
	if normalizeBacktestSyncSessionScope(sessionScope) == "extended" && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(symbol)), "US.") && duration >= 24*time.Hour {
		return bbgotypes.Interval1h
	}
	return interval
}

func (s *Server) handleBacktestSyncCancel(c *gin.Context) {
	var uri taskURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "taskId is invalid")
		return
	}
	taskID := strings.TrimSpace(uri.TaskID)
	_, ok := s.backtestSyncTasks.cancel(taskID, time.Now().UTC())
	if !ok {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "sync task not found or already completed")
		return
	}
	s.writeOK(c, map[string]any{"taskId": taskID, "status": "cancelled"})
}

func (s *Server) handleBacktestSyncProgress(c *gin.Context) {
	var uri taskURI
	if err := bindURI(c, &uri); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "taskId is invalid")
		return
	}
	taskID := strings.TrimSpace(uri.TaskID)
	if taskID == "" {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "taskId is required")
		return
	}
	progress, ok := s.backtestSyncTasks.get(taskID)
	if !ok {
		s.writeError(c, http.StatusNotFound, "NOT_FOUND", "sync task not found")
		return
	}
	s.writeOK(c, progress)
}
