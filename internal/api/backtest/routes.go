// Package backtest 提供回测 HTTP 路由注册与瘦 handler 工厂函数。
// 每个 handler 是纯工厂函数 func(svc *srv.Service) gin.HandlerFunc，
// 只负责参数绑定与响应写入，不持有任何内部状态。
package backtest

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/backtest"
)

// RegisterRoutes 注册所有 /api/v1/backtests 路由。
func RegisterRoutes(api *gin.RouterGroup, svc *srv.Service) {
	bt := api.Group("/backtests")
	bt.GET("", handleList(svc))
	bt.POST("", handleStart(svc))
	bt.POST("/sync", handleSync(svc))
	bt.GET("/sync/:taskId", handleSyncProgress(svc))
	bt.DELETE("/sync/:taskId", handleSyncCancel(svc))
	bt.GET("/:runId/status", handleStatus(svc))
	bt.GET("/:runId", handleResult(svc))
	bt.DELETE("/:runId", handleDelete(svc))
}

// handleList godoc
// @Summary 读取回测列表
// @Tags backtest
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=BacktestRunsData}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Failure 503 {object} httpserver.ErrorEnvelope
// @Router /api/v1/backtests [get]
func handleList(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		runs := svc.List()
		httpserver.WriteOK(c, map[string]any{"runs": runs})
	}
}

// handleStart godoc
// @Summary 启动回测
// @Tags backtest
// @Accept json
// @Produce json
// @Param request body srv.StartRequest true "回测启动请求"
// @Success 200 {object} httpserver.Envelope{data=BacktestQueuedData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/backtests [post]
func handleStart(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req srv.StartRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid backtest request")
			return
		}
		result, err := svc.Start(c.Request.Context(), req)
		if err != nil {
			status := http.StatusInternalServerError
			code := "BACKTEST_START_FAILED"
			message := "start backtest failed"
			if srv.IsRequestError(err) {
				status = http.StatusBadRequest
				code = "BAD_REQUEST"
				message = err.Error()
			}
			if errors.Is(err, srv.ErrStrategyDefinitionNotFound) {
				status = http.StatusNotFound
				code = "NOT_FOUND"
				message = err.Error()
			}
			httpserver.WriteError(c, status, code, message)
			return
		}
		httpserver.WriteOK(c, map[string]any{
			"id":      result.ID,
			"status":  result.Status,
			"message": "backtest queued",
		})
	}
}

// handleSync godoc
// @Summary 启动历史数据同步
// @Tags backtest
// @Accept json
// @Produce json
// @Param request body srv.SyncRequest true "历史数据同步请求"
// @Success 200 {object} httpserver.Envelope{data=srv.SyncStarted}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/backtests/sync [post]
func handleSync(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req srv.SyncRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid sync request")
			return
		}
		result, err := svc.Sync(c.Request.Context(), req)
		if err != nil {
			if srv.IsRequestError(err) {
				httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
				return
			}
			httpserver.WriteError(c, http.StatusInternalServerError, "SYNC_FAILED", err.Error())
			return
		}
		httpserver.WriteOK(c, result)
	}
}

// handleSyncProgress godoc
// @Summary 查询历史数据同步进度
// @Tags backtest
// @Produce json
// @Param taskId path string true "同步任务 ID"
// @Success 200 {object} httpserver.Envelope{data=BacktestSyncProgress}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/backtests/sync/{taskId} [get]
func handleSyncProgress(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			TaskID string `uri:"taskId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "taskId is invalid")
			return
		}
		taskID := strings.TrimSpace(uri.TaskID)
		if taskID == "" {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "taskId is required")
			return
		}
		progress, ok := svc.GetSyncProgress(taskID)
		if !ok {
			httpserver.WriteError(c, http.StatusNotFound, "NOT_FOUND", "sync task not found")
			return
		}
		httpserver.WriteOK(c, progress)
	}
}

// handleSyncCancel godoc
// @Summary 取消历史数据同步任务
// @Tags backtest
// @Produce json
// @Param taskId path string true "同步任务 ID"
// @Success 200 {object} httpserver.Envelope{data=BacktestSyncCancelData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/backtests/sync/{taskId} [delete]
func handleSyncCancel(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			TaskID string `uri:"taskId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "taskId is invalid")
			return
		}
		taskID := strings.TrimSpace(uri.TaskID)
		_, ok := svc.CancelSync(taskID)
		if !ok {
			httpserver.WriteError(c, http.StatusNotFound, "NOT_FOUND", "sync task not found or already completed")
			return
		}
		httpserver.WriteOK(c, map[string]any{"taskId": taskID, "status": "cancelled"})
	}
}

// handleStatus godoc
// @Summary 读取回测状态
// @Tags backtest
// @Produce json
// @Param runId path string true "回测运行 ID"
// @Success 200 {object} httpserver.Envelope{data=BacktestStatusData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/backtests/{runId}/status [get]
func handleStatus(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			RunID string `uri:"runId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "backtest run id is invalid")
			return
		}
		runID := strings.TrimSpace(uri.RunID)
		run, ok := svc.GetStatus(runID)
		if !ok {
			httpserver.WriteError(c, http.StatusNotFound, "NOT_FOUND", "backtest run not found")
			return
		}
		httpserver.WriteOK(c, map[string]any{
			"id":     run.ID,
			"status": run.Status,
		})
	}
}

// handleResult godoc
// @Summary 读取回测结果
// @Tags backtest
// @Produce json
// @Param runId path string true "回测运行 ID"
// @Success 200 {object} httpserver.Envelope{data=srv.RunState}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/backtests/{runId} [get]
func handleResult(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			RunID string `uri:"runId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "backtest run id is invalid")
			return
		}
		runID := strings.TrimSpace(uri.RunID)
		run, ok, err := svc.GetResult(runID)
		if err != nil {
			httpserver.WriteError(c, http.StatusInternalServerError, "BACKTEST_RUN_STORE_FAILED", "load backtest result failed")
			return
		}
		if !ok {
			httpserver.WriteError(c, http.StatusNotFound, "NOT_FOUND", "backtest run not found")
			return
		}
		httpserver.WriteOK(c, run)
	}
}

// handleDelete godoc
// @Summary 删除回测记录
// @Tags backtest
// @Produce json
// @Param runId path string true "回测运行 ID"
// @Success 200 {object} httpserver.Envelope{data=BacktestDeleteData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/backtests/{runId} [delete]
func handleDelete(svc *srv.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			RunID string `uri:"runId" binding:"required"`
		}
		if err := httpserver.BindURI(c, &uri); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "backtest run id is invalid")
			return
		}
		runID := strings.TrimSpace(uri.RunID)
		if runID == "" {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST", "backtest run id is required")
			return
		}

		// 仅允许删除终态回测
		run, ok := svc.GetStatus(runID)
		if !ok {
			httpserver.WriteError(c, http.StatusNotFound, "NOT_FOUND", "backtest run not found")
			return
		}
		if run.Status != "completed" && run.Status != "failed" && run.Status != "cancelled" {
			httpserver.WriteError(c, http.StatusBadRequest, "BAD_REQUEST",
				"only completed, failed or cancelled backtest runs can be deleted")
			return
		}

		if _, deleted, err := svc.Delete(runID); err != nil {
			httpserver.WriteError(c, http.StatusInternalServerError, "BACKTEST_RUN_STORE_FAILED", "delete backtest run failed")
			return
		} else if !deleted {
			httpserver.WriteError(c, http.StatusNotFound, "NOT_FOUND", "backtest run not found")
			return
		}

		httpserver.WriteOK(c, map[string]any{"deleted": true, "id": runID})
	}
}
