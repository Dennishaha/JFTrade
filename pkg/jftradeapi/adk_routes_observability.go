package jftradeapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func (s *Server) handleADKAudit(w http.ResponseWriter, r *http.Request) {
	items, err := s.adkRuntime.Store().ListAuditEvents(r.Context())
	if err == nil {
		items = filterADKAudit(items, r.URL.Query().Get("kind"), r.URL.Query().Get("subjectId"))
	}
	writeADKPagedListOrError(s, w, "ADK_AUDIT_LIST_FAILED", "events", items, err, r)
}

func (s *Server) handleADKMetrics(w http.ResponseWriter, r *http.Request) {
	runs, err := s.adkRuntime.Store().ListRuns(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_METRICS_FAILED", err.Error())
		return
	}
	agents, err := s.adkRuntime.Store().ListAllAgents(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_METRICS_FAILED", err.Error())
		return
	}
	approvals, err := s.adkRuntime.Store().ListApprovals(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_METRICS_FAILED", err.Error())
		return
	}
	statuses := map[string]int{}
	byAgent := map[string]int{}
	byProvider := map[string]int{}
	agentProvider := make(map[string]string, len(agents))
	for _, agent := range agents {
		agentProvider[agent.ID] = strings.TrimSpace(agent.ProviderID)
	}
	toolCalls := 0
	successfulTools := 0
	toolsByName := map[string]int{}
	toolsByStatus := map[string]int{}
	var totalDuration int64
	var durationCount int64
	var failedRuns int
	var timedOutRuns int
	var cancelledRuns int
	var resumedRuns int
	var orphanedRuns int
	var tokensInTotal int
	var tokensOutTotal int
	var tokenSamples int
	for _, run := range runs {
		statuses[run.Status]++
		byAgent[run.AgentID]++
		providerID := strings.TrimSpace(run.ProviderID)
		if providerID == "" {
			providerID = agentProvider[run.AgentID]
		}
		if providerID == "" {
			providerID = "unbound"
		}
		byProvider[providerID]++
		switch run.Status {
		case jfadk.RunStatusFailed:
			failedRuns++
		case jfadk.RunStatusTimedOut:
			timedOutRuns++
		case jfadk.RunStatusCancelled:
			cancelledRuns++
		}
		if strings.TrimSpace(run.ResumeState) == "adk_confirmation_resolved" {
			resumedRuns++
		}
		if strings.TrimSpace(run.ErrorCode) == "RUN_ORPHANED" {
			orphanedRuns++
		}
		if run.Usage != nil && (run.Usage.TokensIn > 0 || run.Usage.TokensOut > 0) {
			tokensInTotal += run.Usage.TokensIn
			tokensOutTotal += run.Usage.TokensOut
			tokenSamples++
		}
		for _, call := range run.ToolCalls {
			toolCalls++
			toolsByName[call.ToolName]++
			toolsByStatus[call.Status]++
			if call.Status == "SUCCEEDED" {
				successfulTools++
			}
			if call.DurationMs > 0 {
				totalDuration += call.DurationMs
				durationCount++
			}
		}
	}
	pendingApprovals := 0
	approvedApprovals := 0
	deniedApprovals := 0
	recoverablePending := 0
	var pendingWaitTotal int64
	var pendingWaitMax int64
	var resolvedWaitTotal int64
	var resolvedWaitMax int64
	var resolvedWaitCount int64
	now := time.Now().UTC()
	for _, approval := range approvals {
		waitMs := approvalWaitDurationMs(approval, now)
		if approval.Status == jfadk.ApprovalStatusPending {
			pendingApprovals++
			pendingWaitTotal += waitMs
			if waitMs > pendingWaitMax {
				pendingWaitMax = waitMs
			}
			if strings.TrimSpace(approval.FunctionCallID) != "" && strings.TrimSpace(approval.ConfirmationCallID) != "" {
				recoverablePending++
			}
		}
		if approval.Status == jfadk.ApprovalStatusApproved {
			approvedApprovals++
			resolvedWaitTotal += waitMs
			resolvedWaitCount++
			if waitMs > resolvedWaitMax {
				resolvedWaitMax = waitMs
			}
		}
		if approval.Status == jfadk.ApprovalStatusDenied {
			deniedApprovals++
			resolvedWaitTotal += waitMs
			resolvedWaitCount++
			if waitMs > resolvedWaitMax {
				resolvedWaitMax = waitMs
			}
		}
	}
	averageToolDuration := int64(0)
	if durationCount > 0 {
		averageToolDuration = totalDuration / durationCount
	}
	var pendingWaitAvg int64
	if pendingApprovals > 0 {
		pendingWaitAvg = pendingWaitTotal / int64(pendingApprovals)
	}
	var resolvedWaitAvg int64
	if resolvedWaitCount > 0 {
		resolvedWaitAvg = resolvedWaitTotal / resolvedWaitCount
	}
	var tokensInAverage any
	var tokensOutAverage any
	var tokensInTotalValue any
	var tokensOutTotalValue any
	if tokenSamples > 0 {
		tokensInTotalValue = tokensInTotal
		tokensOutTotalValue = tokensOutTotal
		tokensInAverage = tokensInTotal / tokenSamples
		tokensOutAverage = tokensOutTotal / tokenSamples
	}
	s.writeOK(w, map[string]any{
		"runs": map[string]any{
			"total": len(runs), "byStatus": statuses, "byAgent": byAgent, "byProvider": byProvider,
			"lifecycle": map[string]any{
				"failed": failedRuns, "timedOut": timedOutRuns, "cancelled": cancelledRuns, "resumed": resumedRuns, "orphaned": orphanedRuns,
			},
		},
		"tools": map[string]any{
			"total": toolCalls, "successful": successfulTools, "averageDurationMs": averageToolDuration,
			"byName": toolsByName, "byStatus": toolsByStatus,
		},
		"approvals": map[string]any{
			"pending": pendingApprovals, "total": len(approvals), "approved": approvedApprovals, "denied": deniedApprovals, "recoverablePending": recoverablePending,
			"pendingWaitMs":    map[string]any{"average": pendingWaitAvg, "max": pendingWaitMax},
			"resolutionWaitMs": map[string]any{"average": resolvedWaitAvg, "max": resolvedWaitMax, "count": resolvedWaitCount},
		},
		"usage": map[string]any{
			"samples":          tokenSamples,
			"tokensInTotal":    tokensInTotalValue,
			"tokensOutTotal":   tokensOutTotalValue,
			"tokensInAverage":  tokensInAverage,
			"tokensOutAverage": tokensOutAverage,
		},
		"checkedAt": nowStringRFC3339Nano(),
	})
}

func approvalWaitDurationMs(approval jfadk.Approval, now time.Time) int64 {
	createdAt := strings.TrimSpace(approval.CreatedAt)
	if createdAt == "" {
		return 0
	}
	startedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return 0
	}
	endedAt := now
	if approval.Status != jfadk.ApprovalStatusPending {
		if updatedAt := strings.TrimSpace(approval.UpdatedAt); updatedAt != "" {
			if parsed, parseErr := time.Parse(time.RFC3339Nano, updatedAt); parseErr == nil {
				endedAt = parsed
			}
		}
	}
	if endedAt.Before(startedAt) {
		return 0
	}
	return endedAt.Sub(startedAt).Milliseconds()
}

func (s *Server) handleADKOptimizationTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.adkRuntime.Store().ListOptimizationTasks(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_OPTIMIZATION_LIST_FAILED", err.Error())
		return
	}
	items := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, s.adkOptimizationTaskResponse(r.Context(), task))
	}
	writeADKPagedListOrError(s, w, "ADK_OPTIMIZATION_LIST_FAILED", "tasks", items, nil, r)
}

func (s *Server) handleADKOptimizationTask(w http.ResponseWriter, r *http.Request) {
	id, err := decodePathSegment(strings.TrimPrefix(r.URL.Path, "/api/v1/adk/optimization-tasks/"))
	if err != nil || strings.TrimSpace(id) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "taskId is invalid")
		return
	}
	task, ok, err := s.adkRuntime.Store().OptimizationTask(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_OPTIMIZATION_GET_FAILED", err.Error())
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "optimization task not found")
		return
	}
	s.writeOK(w, s.adkOptimizationTaskResponse(r.Context(), task))
}

func (s *Server) handleADKOptimizationTaskCancel(w http.ResponseWriter, r *http.Request) {
	id, err := decodePathSegment(pathMiddle(r.URL.Path, "/api/v1/adk/optimization-tasks/", "/cancel"))
	if err != nil || strings.TrimSpace(id) == "" {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "taskId is invalid")
		return
	}
	task, ok, err := s.adkRuntime.Store().OptimizationTask(r.Context(), id)
	if err != nil || !ok {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "optimization task not found")
		return
	}
	for _, ref := range task.Runs {
		s.backtestRuns.cancel(ref.RunID)
	}
	task.Status = "cancelled"
	task, err = s.adkRuntime.Store().SaveOptimizationTask(r.Context(), task)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "ADK_OPTIMIZATION_CANCEL_FAILED", err.Error())
		return
	}
	s.adkRuntime.RecordAudit(r.Context(), "optimization.cancelled", task.ID, "Optimization task cancelled.", nil)
	s.writeOK(w, s.adkOptimizationTaskResponse(r.Context(), task))
}

func (s *Server) adkOptimizationTaskResponse(ctx context.Context, task jfadk.OptimizationTask) map[string]any {
	runs := make([]map[string]any, 0, len(task.Runs))
	running := 0
	completed := 0
	failed := 0
	cancelled := 0
	for _, ref := range task.Runs {
		status := "missing"
		var result any
		if run, ok := s.backtestRuns.get(ref.RunID); ok {
			status = run.Status
			result = run.Result
		}
		switch status {
		case "queued", "running":
			running++
		case "completed":
			completed++
		case "cancelled":
			cancelled++
		case "failed", "missing":
			failed++
		}
		runs = append(runs, map[string]any{
			"definitionId": ref.DefinitionID, "runId": ref.RunID, "status": status, "result": result,
		})
	}
	status := task.Status
	if status != "cancelled" {
		switch {
		case running > 0:
			status = "running"
		case failed > 0:
			status = "failed"
		case completed == len(task.Runs) && len(task.Runs) > 0:
			status = "completed"
		case cancelled == len(task.Runs) && len(task.Runs) > 0:
			status = "cancelled"
		default:
			status = "queued"
		}
	}
	if task.Status != status {
		task.Status = status
		task, _ = s.adkRuntime.Store().SaveOptimizationTask(ctx, task)
	}
	return map[string]any{
		"id": task.ID, "status": status, "objective": task.Objective, "runs": runs,
		"progress":  map[string]any{"total": len(task.Runs), "running": running, "completed": completed, "failed": failed, "cancelled": cancelled},
		"createdAt": task.CreatedAt, "updatedAt": task.UpdatedAt,
	}
}

func queryIntDefault(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
