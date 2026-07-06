package assistant

import (
	"context"
	"fmt"
	"strings"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

// GetMetrics 聚合 ADK 运行指标（runs/tools/approvals/usage）。
func (s *Service) GetMetrics(ctx context.Context) (any, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	runs, agentProvider, approvals, err := s.loadMetricsInputs(ctx)
	if err != nil {
		return nil, err
	}
	runMetrics, toolMetrics, usageMetrics := aggregateRunMetrics(runs, agentProvider)
	approvalMetrics := aggregateApprovalMetrics(approvals, time.Now().UTC())
	return buildMetricsPayload(runs, approvals, runMetrics, toolMetrics, approvalMetrics, usageMetrics), nil
}

type runMetricsSummary struct {
	statuses   map[string]int
	byAgent    map[string]int
	byProvider map[string]int
	failed     int
	timedOut   int
	cancelled  int
	resumed    int
	orphaned   int
}

type toolMetricsSummary struct {
	total             int
	successful        int
	averageDurationMs int64
	byName            map[string]int
	byStatus          map[string]int
}

type usageMetricsSummary struct {
	samples        int
	tokensInTotal  any
	tokensOutTotal any
	tokensInAvg    any
	tokensOutAvg   any
}

type approvalMetricsSummary struct {
	pending           int
	approved          int
	denied            int
	recoverable       int
	pendingWaitAvg    int64
	pendingWaitMax    int64
	resolutionWaitAvg int64
	resolutionWaitMax int64
	resolutionCount   int64
}

func (s *Service) loadMetricsInputs(ctx context.Context) ([]jfadk.Run, map[string]string, []jfadk.Approval, error) {
	store := s.runtime.Store()
	runs, err := store.ListRuns(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	agents, err := store.ListAllAgents(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	approvals, err := store.ListApprovals(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	return runs, metricsAgentProviders(agents), approvals, nil
}

func metricsAgentProviders(agents []jfadk.Agent) map[string]string {
	agentProvider := make(map[string]string, len(agents))
	for _, agent := range agents {
		agentProvider[agent.ID] = strings.TrimSpace(agent.ProviderID)
	}
	return agentProvider
}

func aggregateRunMetrics(runs []jfadk.Run, agentProvider map[string]string) (runMetricsSummary, toolMetricsSummary, usageMetricsSummary) {
	runMetrics := runMetricsSummary{
		statuses:   map[string]int{},
		byAgent:    map[string]int{},
		byProvider: map[string]int{},
	}
	toolMetrics := toolMetricsSummary{
		byName:   map[string]int{},
		byStatus: map[string]int{},
	}
	var totalDuration int64
	var durationCount int64
	var tokensInTotal int
	var tokensOutTotal int
	tokenSamples := 0

	for _, run := range runs {
		runMetrics.statuses[run.Status]++
		runMetrics.byAgent[run.AgentID]++
		runMetrics.byProvider[metricsProviderID(run, agentProvider)]++
		accumulateRunLifecycle(&runMetrics, run)
		if run.Usage != nil && (run.Usage.TokensIn > 0 || run.Usage.TokensOut > 0) {
			tokensInTotal += run.Usage.TokensIn
			tokensOutTotal += run.Usage.TokensOut
			tokenSamples++
		}
		for _, call := range run.ToolCalls {
			toolMetrics.total++
			toolMetrics.byName[call.ToolName]++
			toolMetrics.byStatus[call.Status]++
			if call.Status == "SUCCEEDED" {
				toolMetrics.successful++
			}
			if call.DurationMs > 0 {
				totalDuration += call.DurationMs
				durationCount++
			}
		}
	}
	if durationCount > 0 {
		toolMetrics.averageDurationMs = totalDuration / durationCount
	}
	return runMetrics, toolMetrics, finalizeUsageMetrics(tokensInTotal, tokensOutTotal, tokenSamples)
}

func metricsProviderID(run jfadk.Run, agentProvider map[string]string) string {
	providerID := strings.TrimSpace(run.ProviderID)
	if providerID == "" {
		providerID = agentProvider[run.AgentID]
	}
	if providerID == "" {
		return "unbound"
	}
	return providerID
}

func accumulateRunLifecycle(metrics *runMetricsSummary, run jfadk.Run) {
	switch run.Status {
	case jfadk.RunStatusFailed:
		metrics.failed++
	case jfadk.RunStatusTimedOut:
		metrics.timedOut++
	case jfadk.RunStatusCancelled:
		metrics.cancelled++
	}
	if strings.TrimSpace(run.ResumeState) == "adk_confirmation_resolved" {
		metrics.resumed++
	}
	if strings.TrimSpace(run.ErrorCode) == "RUN_ORPHANED" {
		metrics.orphaned++
	}
}

func finalizeUsageMetrics(tokensInTotal int, tokensOutTotal int, tokenSamples int) usageMetricsSummary {
	usage := usageMetricsSummary{samples: tokenSamples}
	if tokenSamples == 0 {
		return usage
	}
	usage.tokensInTotal = tokensInTotal
	usage.tokensOutTotal = tokensOutTotal
	usage.tokensInAvg = tokensInTotal / tokenSamples
	usage.tokensOutAvg = tokensOutTotal / tokenSamples
	return usage
}

func aggregateApprovalMetrics(approvals []jfadk.Approval, now time.Time) approvalMetricsSummary {
	var metrics approvalMetricsSummary
	var pendingWaitTotal int64
	var resolvedWaitTotal int64

	for _, approval := range approvals {
		waitMs := approvalWaitDurationMs(approval, now)
		switch approval.Status {
		case jfadk.ApprovalStatusPending:
			metrics.pending++
			pendingWaitTotal += waitMs
			if waitMs > metrics.pendingWaitMax {
				metrics.pendingWaitMax = waitMs
			}
			if strings.TrimSpace(approval.FunctionCallID) != "" && strings.TrimSpace(approval.ConfirmationCallID) != "" {
				metrics.recoverable++
			}
		case jfadk.ApprovalStatusApproved:
			metrics.approved++
			resolvedWaitTotal += waitMs
			metrics.resolutionCount++
			if waitMs > metrics.resolutionWaitMax {
				metrics.resolutionWaitMax = waitMs
			}
		case jfadk.ApprovalStatusDenied:
			metrics.denied++
			resolvedWaitTotal += waitMs
			metrics.resolutionCount++
			if waitMs > metrics.resolutionWaitMax {
				metrics.resolutionWaitMax = waitMs
			}
		}
	}
	if metrics.pending > 0 {
		metrics.pendingWaitAvg = pendingWaitTotal / int64(metrics.pending)
	}
	if metrics.resolutionCount > 0 {
		metrics.resolutionWaitAvg = resolvedWaitTotal / metrics.resolutionCount
	}
	return metrics
}

func buildMetricsPayload(
	runs []jfadk.Run,
	approvals []jfadk.Approval,
	runMetrics runMetricsSummary,
	toolMetrics toolMetricsSummary,
	approvalMetrics approvalMetricsSummary,
	usageMetrics usageMetricsSummary,
) map[string]any {
	return map[string]any{
		"runs": map[string]any{
			"total":      len(runs),
			"byStatus":   runMetrics.statuses,
			"byAgent":    runMetrics.byAgent,
			"byProvider": runMetrics.byProvider,
			"lifecycle": map[string]any{
				"failed":    runMetrics.failed,
				"timedOut":  runMetrics.timedOut,
				"cancelled": runMetrics.cancelled,
				"resumed":   runMetrics.resumed,
				"orphaned":  runMetrics.orphaned,
			},
		},
		"tools": map[string]any{
			"total":             toolMetrics.total,
			"successful":        toolMetrics.successful,
			"averageDurationMs": toolMetrics.averageDurationMs,
			"byName":            toolMetrics.byName,
			"byStatus":          toolMetrics.byStatus,
		},
		"approvals": map[string]any{
			"pending":            approvalMetrics.pending,
			"total":              len(approvals),
			"approved":           approvalMetrics.approved,
			"denied":             approvalMetrics.denied,
			"recoverablePending": approvalMetrics.recoverable,
			"pendingWaitMs": map[string]any{
				"average": approvalMetrics.pendingWaitAvg,
				"max":     approvalMetrics.pendingWaitMax,
			},
			"resolutionWaitMs": map[string]any{
				"average": approvalMetrics.resolutionWaitAvg,
				"max":     approvalMetrics.resolutionWaitMax,
				"count":   approvalMetrics.resolutionCount,
			},
		},
		"usage": map[string]any{
			"samples":          usageMetrics.samples,
			"tokensInTotal":    usageMetrics.tokensInTotal,
			"tokensOutTotal":   usageMetrics.tokensOutTotal,
			"tokensInAverage":  usageMetrics.tokensInAvg,
			"tokensOutAverage": usageMetrics.tokensOutAvg,
		},
		"checkedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
}
