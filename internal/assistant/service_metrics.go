package assistant

import (
	"context"
	"fmt"
	"strings"
	"time"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
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
	now := time.Now().UTC()
	approvalMetrics := aggregateApprovalMetrics(approvals, now)
	activityMetrics, err := s.loadActivityMetrics(ctx, runs, approvals, now)
	if err != nil {
		return nil, err
	}
	return buildMetricsPayload(runs, approvals, runMetrics, toolMetrics, approvalMetrics, usageMetrics, activityMetrics, now), nil
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

type activityMetricsSummary struct {
	windowSince             time.Time
	runsRecent              int
	approvalsRecent         int
	sessionsTotal           int
	sessionsRecent          int
	workflowDefinitions     int
	workflowDefinitionsLive int
	workflowTriggers        int
	workflowTriggersLive    int
	workflowInvocations     int
	workflowRecent          int
	workflowByStatus        map[string]int
	workflowByTriggerType   map[string]int
}

const activityMeasurementWindow = 7 * 24 * time.Hour

func (s *Service) loadActivityMetrics(
	ctx context.Context,
	runs []jfadk.Run,
	approvals []jfadk.Approval,
	now time.Time,
) (activityMetricsSummary, error) {
	store := s.runtime.Store()
	sessions, err := store.ListSessions(ctx)
	if err != nil {
		return activityMetricsSummary{}, err
	}
	workflows, _, err := store.ListWorkflowDefinitionsPage(ctx, "", 100_000, 0)
	if err != nil {
		return activityMetricsSummary{}, err
	}
	triggers, err := store.ListWorkflowTriggers(ctx, "")
	if err != nil {
		return activityMetricsSummary{}, err
	}
	logs, totalLogs, err := store.ListWorkflowTriggerLogsPage(ctx, "", "", "", 100_000, 0)
	if err != nil {
		return activityMetricsSummary{}, err
	}
	return aggregateActivityMetrics(runs, approvals, sessions, workflows, triggers, logs, totalLogs, now), nil
}

func aggregateActivityMetrics(
	runs []jfadk.Run,
	approvals []jfadk.Approval,
	sessions []jfadk.Session,
	workflows []jfadk.WorkflowDefinition,
	triggers []jfadk.WorkflowTrigger,
	logs []jfadk.WorkflowTriggerLog,
	totalLogs int,
	now time.Time,
) activityMetricsSummary {
	since := now.Add(-activityMeasurementWindow)
	metrics := activityMetricsSummary{
		windowSince:           since,
		sessionsTotal:         len(sessions),
		workflowDefinitions:   len(workflows),
		workflowTriggers:      len(triggers),
		workflowInvocations:   totalLogs,
		workflowByStatus:      map[string]int{},
		workflowByTriggerType: map[string]int{},
	}
	metrics.runsRecent = countRecentRuns(runs, since)
	metrics.approvalsRecent = countRecentApprovals(approvals, since)
	metrics.sessionsRecent = countRecentSessions(sessions, since)
	for _, workflow := range workflows {
		if workflow.Status == jfadk.WorkflowStatusEnabled {
			metrics.workflowDefinitionsLive++
		}
	}
	for _, trigger := range triggers {
		if trigger.Status == jfadk.WorkflowTriggerStatusEnabled {
			metrics.workflowTriggersLive++
		}
	}
	for _, log := range logs {
		metrics.workflowByStatus[log.Status]++
		metrics.workflowByTriggerType[log.TriggerType]++
		if timestampInWindow(log.CreatedAt, since) {
			metrics.workflowRecent++
		}
	}
	return metrics
}

func countRecentRuns(runs []jfadk.Run, since time.Time) int {
	count := 0
	for _, run := range runs {
		if timestampInWindow(run.CreatedAt, since) {
			count++
		}
	}
	return count
}

func countRecentApprovals(approvals []jfadk.Approval, since time.Time) int {
	count := 0
	for _, approval := range approvals {
		if timestampInWindow(approval.CreatedAt, since) {
			count++
		}
	}
	return count
}

func countRecentSessions(sessions []jfadk.Session, since time.Time) int {
	count := 0
	for _, session := range sessions {
		if timestampInWindow(session.CreatedAt, since) {
			count++
		}
	}
	return count
}

func timestampInWindow(value string, since time.Time) bool {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	return err == nil && !parsed.Before(since)
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
	activityMetrics activityMetricsSummary,
	now time.Time,
) map[string]any {
	return map[string]any{
		"runs": map[string]any{
			"total":      len(runs),
			"last7Days":  activityMetrics.runsRecent,
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
			"last7Days":          activityMetrics.approvalsRecent,
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
		"sessions": map[string]any{
			"total":     activityMetrics.sessionsTotal,
			"last7Days": activityMetrics.sessionsRecent,
		},
		"workflows": map[string]any{
			"definitions":          activityMetrics.workflowDefinitions,
			"enabledDefinitions":   activityMetrics.workflowDefinitionsLive,
			"triggers":             activityMetrics.workflowTriggers,
			"enabledTriggers":      activityMetrics.workflowTriggersLive,
			"invocations":          activityMetrics.workflowInvocations,
			"invocationsLast7Days": activityMetrics.workflowRecent,
			"byStatus":             activityMetrics.workflowByStatus,
			"byTriggerType":        activityMetrics.workflowByTriggerType,
		},
		"measurementWindow": map[string]any{
			"days":  7,
			"since": activityMetrics.windowSince.Format(time.RFC3339Nano),
		},
		"checkedAt": now.Format(time.RFC3339Nano),
	}
}
