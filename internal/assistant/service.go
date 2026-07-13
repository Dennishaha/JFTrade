// Package assistant 提供 ADK/Assistant 业务门面层。将 ADK Runtime 的
// 会话、聊天、工具、审批、任务、记忆、Provider、Agent、Skill、审计、指标等
// 能力封装为统一 Service，Handler 层仅负责参数绑定与响应写入。
//
// Service 直接持有 *jfadk.Runtime——Runtime 聚合了 Store、ToolRegistry、
// SkillRegistry，所有 CRUD 和运行时操作均通过 Runtime 或其子组件委托。
// 业务输入输出使用明确类型；仅运行时设置、工具结果等动态数据保留 any。
package assistant

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

var ErrSessionTimelineFailed = errors.New("adk session timeline failed")

func wrapSessionTimelineError(err error) error {
	return fmt.Errorf("%w: %w", ErrSessionTimelineFailed, err)
}

// ──────────────────────────────────────────────────────────────────────────────
// Service
// ──────────────────────────────────────────────────────────────────────────────

// Service ADK 助手业务门面。持有 Runtime 及其聚合的 Store/ToolRegistry/SkillRegistry。
type Service struct {
	runtime           *jfadk.Runtime
	runtimeSettings   func() any
	streamIdleTimeout func() int
	optimizationRuns  OptimizationRuns
	marketSnapshot    WorkflowMarketSnapshot
	workflowInterval  time.Duration
	workflowScheduler *WorkflowScheduler
}

// OptimizationRun is the assistant-facing projection of a backtest run.
type OptimizationRun struct {
	Status string
	Result any
}

// OptimizationRuns isolates optimization task orchestration from the concrete
// backtest store owned by the application assembly layer.
type OptimizationRuns interface {
	Get(runID string) (OptimizationRun, bool)
	Cancel(runID string)
}

// Option configures optional application-owned assistant ports.
type Option func(*Service)

// WithRuntimeSettings exposes the current public runtime settings snapshot.
func WithRuntimeSettings(settings func() any) Option {
	return func(service *Service) {
		service.runtimeSettings = settings
	}
}

// WithStreamIdleTimeout exposes the configured SSE idle timeout in milliseconds.
func WithStreamIdleTimeout(timeout func() int) Option {
	return func(service *Service) {
		service.streamIdleTimeout = timeout
	}
}

// WithOptimizationRuns connects optimization tasks to application-owned runs.
func WithOptimizationRuns(runs OptimizationRuns) Option {
	return func(service *Service) {
		service.optimizationRuns = runs
	}
}

// WithWorkflowMarketSnapshot connects workflow market-threshold triggers to
// the application-owned market data service.
func WithWorkflowMarketSnapshot(fn WorkflowMarketSnapshot) Option {
	return func(service *Service) {
		service.marketSnapshot = fn
	}
}

// WithWorkflowSchedulerInterval overrides the production polling interval.
func WithWorkflowSchedulerInterval(interval time.Duration) Option {
	return func(service *Service) {
		service.workflowInterval = interval
	}
}

// NewService 创建助手服务。
func NewService(runtime *jfadk.Runtime, options ...Option) *Service {
	service := &Service{runtime: runtime}
	for _, option := range options {
		option(service)
	}
	return service
}

// Available 检查 ADK 运行时是否可用。
func (s *Service) Available() bool {
	return s.runtime != nil && s.runtime.Store() != nil
}

// StreamIdleTimeoutMillis returns the configured stream idle timeout.
func (s *Service) StreamIdleTimeoutMillis() int {
	if s.streamIdleTimeout == nil {
		return 0
	}
	return s.streamIdleTimeout()
}

// ──────────────────────────────────────────────────────────────────────────────
// Snapshot & Tools
// ──────────────────────────────────────────────────────────────────────────────

// Snapshot 返回 ADK 运行时快照（Providers + Agents + Skills + Tools）。

// Chat 同步对话。
func (s *Service) Chat(ctx context.Context, req jfadk.ChatRequest) (jfadk.ChatResponse, error) {
	if s.runtime == nil {
		return jfadk.ChatResponse{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.Chat(ctx, req)
}

// ChatStream 流式对话。onDelta 接收每个 delta 事件。
func (s *Service) ChatStream(ctx context.Context, req jfadk.ChatRequest, onDelta func(jfadk.ChatDelta) error) (jfadk.ChatResponse, error) {
	if s.runtime == nil {
		return jfadk.ChatResponse{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.ChatStream(ctx, req, onDelta)
}

// PreviewSession returns the existing or prospective session emitted at stream start.
func (s *Service) PreviewSession(ctx context.Context, payload jfadk.ChatRequest) (jfadk.Session, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.Session{}, fmt.Errorf("adk runtime is unavailable")
	}
	agent, err := s.runtime.Store().DefaultAgent(ctx)
	if strings.TrimSpace(payload.AgentID) != "" {
		var ok bool
		agent, ok, err = s.runtime.Store().Agent(ctx, payload.AgentID)
		if err != nil {
			return jfadk.Session{}, err
		}
		if !ok {
			return jfadk.Session{}, fmt.Errorf("agent not found")
		}
	}
	if err != nil {
		return jfadk.Session{}, err
	}
	if strings.TrimSpace(payload.SessionID) != "" {
		session, ok, sessionErr := s.runtime.Store().Session(ctx, payload.SessionID)
		if sessionErr != nil {
			return jfadk.Session{}, sessionErr
		}
		if ok {
			return session, nil
		}
	}
	title := strings.TrimSpace(payload.Message)
	if len([]rune(title)) > 28 {
		title = string([]rune(title)[:28])
	}
	return jfadk.Session{AgentID: agent.ID, Title: title}, nil
}

// RecoverTerminalChatResponse rebuilds a final stream response after a terminal
// run was persisted but the final session message append failed.
func (s *Service) RecoverTerminalChatResponse(ctx context.Context, runID string) (*jfadk.ChatResponse, error) {
	if s.runtime == nil || s.runtime.Store() == nil || strings.TrimSpace(runID) == "" {
		return nil, nil
	}
	run, ok, err := s.runtime.Store().Run(ctx, strings.TrimSpace(runID))
	if err != nil || !ok || !isTerminalRunStatus(run.Status) {
		return nil, err
	}
	session, ok, err := s.runtime.Store().Session(ctx, run.SessionID)
	if err != nil || !ok {
		return nil, err
	}
	timeline, _, err := s.runtime.Store().SessionTimeline(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	if timeline == nil {
		timeline = []jfadk.TimelineEntry{}
	}
	reply, reasoning := s.recoverAssistantReply(ctx, run)
	response := jfadk.ChatResponse{
		Reply:            reply,
		ReasoningContent: reasoning,
		Session:          session,
		Run:              run,
		PendingApprovals: run.PendingApprovals,
		Timeline:         timeline,
	}
	if snapshot, snapshotErr := s.runtime.SessionContext(ctx, session.ID); snapshotErr == nil {
		response.Context = &snapshot
	}
	return new(jfadk.NormalizeChatResponse(response)), nil
}

func (s *Service) recoverAssistantReply(ctx context.Context, run jfadk.Run) (string, string) {
	projection, ok, err := s.runtime.Store().SessionProjection(ctx, run.SessionID)
	if err != nil || !ok {
		return "", ""
	}
	if finalMessageID := strings.TrimSpace(run.FinalMessageID); finalMessageID != "" {
		for _, message := range projection.Messages {
			if message.ID == finalMessageID && strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
				return message.Content, message.ReasoningContent
			}
		}
	}
	if projection.LatestAssistant != nil {
		return projection.LatestAssistant.Content, projection.LatestAssistant.ReasoningContent
	}
	return "", ""
}

func isTerminalRunStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case jfadk.RunStatusCompleted, jfadk.RunStatusFailed, jfadk.RunStatusTimedOut, jfadk.RunStatusCancelled, jfadk.RunStatusDenied:
		return true
	default:
		return false
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Runs
// ──────────────────────────────────────────────────────────────────────────────

// ListRuns 分页列出运行记录，支持 status/agentId/sessionId 过滤。
func (s *Service) ListRuns(ctx context.Context, query RunQuery) (Page[jfadk.Run], error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return Page[jfadk.Run]{}, fmt.Errorf("adk runtime is unavailable")
	}
	runs, total, err := s.runtime.Store().ListRunsPage(ctx, query.Status, query.AgentID, query.SessionID, query.Limit, query.Offset)
	if err != nil {
		return Page[jfadk.Run]{}, err
	}
	return Page[jfadk.Run]{Items: runs, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

// GetRun 按 ID 获取单个运行记录。
func (s *Service) GetRun(ctx context.Context, runID string) (jfadk.Run, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.Run{}, fmt.Errorf("adk runtime is unavailable")
	}
	run, ok, err := s.runtime.Store().Run(ctx, runID)
	if err != nil {
		return jfadk.Run{}, err
	}
	if !ok {
		return jfadk.Run{}, fmt.Errorf("run not found")
	}
	return run, nil
}

// CancelRun 取消运行中的 run。
func (s *Service) CancelRun(ctx context.Context, runID string) (jfadk.Run, error) {
	if s.runtime == nil {
		return jfadk.Run{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.CancelRun(ctx, runID)
}

func (s *Service) PauseGoalRun(ctx context.Context, runID string) (jfadk.Run, error) {
	if s.runtime == nil {
		return jfadk.Run{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.PauseGoalRun(ctx, runID)
}

func (s *Service) ResumeGoalRun(ctx context.Context, runID string) (jfadk.Run, error) {
	if s.runtime == nil {
		return jfadk.Run{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.ResumeGoalRun(ctx, runID)
}

func (s *Service) UpdateRunObjective(ctx context.Context, runID string, objective string) (jfadk.Run, error) {
	if s.runtime == nil {
		return jfadk.Run{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.UpdateRunObjective(ctx, runID, objective)
}

// ReconcileExpiredRuns 清理超时 run。
func (s *Service) ReconcileExpiredRuns(ctx context.Context) {
	if s.runtime != nil {
		s.runtime.ReconcileExpiredRuns(ctx)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Approvals
// ──────────────────────────────────────────────────────────────────────────────

// ListApprovals 分页列出审批，支持 status/agentId 过滤。
func (s *Service) ListApprovals(ctx context.Context, query ApprovalQuery) (Page[jfadk.Approval], error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return Page[jfadk.Approval]{}, fmt.Errorf("adk runtime is unavailable")
	}
	approvals, total, err := s.runtime.Store().ListApprovalsPage(ctx, query.Status, query.AgentID, query.Limit, query.Offset)
	if err != nil {
		return Page[jfadk.Approval]{}, err
	}
	return Page[jfadk.Approval]{Items: approvals, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

// ResolveApproval 同步审批（批准或拒绝），等待后续 run 完成。
func (s *Service) ResolveApproval(ctx context.Context, approvalID string, approved bool) (jfadk.ApprovalResolution, error) {
	if s.runtime == nil {
		return jfadk.ApprovalResolution{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.ResolveApproval(ctx, approvalID, approved)
}

// ResolveApprovalAsync 异步审批，立即返回（后续 run 在后台继续）。
func (s *Service) ResolveApprovalAsync(ctx context.Context, approvalID string, approved bool) (jfadk.ApprovalResolution, error) {
	if s.runtime == nil {
		return jfadk.ApprovalResolution{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.ResolveApprovalAsync(ctx, approvalID, approved)
}

func (s *Service) ResolveInputAsync(ctx context.Context, runID string, payload jfadk.InputResponseRequest) (jfadk.InputResolution, error) {
	if s == nil || s.runtime == nil {
		return jfadk.InputResolution{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.ResolveInputAsync(ctx, runID, payload)
}

// ReconcileResolvedApprovals 核对已解决的审批并触发后续 run。
func (s *Service) ReconcileResolvedApprovals(ctx context.Context) {
	if s.runtime != nil {
		s.runtime.ReconcileResolvedApprovals(ctx)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Skills
// ──────────────────────────────────────────────────────────────────────────────

// ListSkills 列出所有已安装的 Skill。
func (s *Service) ListSkills(ctx context.Context) ([]jfadk.Skill, error) {
	if s.runtime == nil || s.runtime.Skills() == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	items, err := s.runtime.Skills().List(ctx)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// InstallSkill 通过 URL 安装 Skill。
func (s *Service) InstallSkill(ctx context.Context, url string) (jfadk.Skill, error) {
	if s.runtime == nil || s.runtime.Skills() == nil {
		return jfadk.Skill{}, fmt.Errorf("adk runtime is unavailable")
	}
	skill, err := s.runtime.Skills().InstallURL(ctx, url)
	if err != nil {
		return jfadk.Skill{}, err
	}
	s.runtime.RecordAudit(ctx, "skill.installed", skill.ID, "ADK skill installed.", map[string]any{"source": skill.Source})
	return skill, nil
}

// DeleteSkill 卸载 Skill。
func (s *Service) DeleteSkill(ctx context.Context, skillID string) error {
	if s.runtime == nil || s.runtime.Skills() == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	if err := s.runtime.Skills().Uninstall(ctx, skillID); err != nil {
		return err
	}
	s.runtime.RecordAudit(ctx, "skill.uninstalled", skillID, "ADK skill uninstalled.", nil)
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Audit & Metrics
// ──────────────────────────────────────────────────────────────────────────────

// GetAudit 列出审计事件，支持 kind/subjectId 过滤。
func (s *Service) GetAudit(ctx context.Context, query AuditQuery) ([]jfadk.AuditEvent, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	events, err := s.runtime.Store().ListAuditEvents(ctx)
	if err != nil {
		return nil, err
	}
	if query.Kind != "" || query.SubjectID != "" {
		filtered := make([]jfadk.AuditEvent, 0, len(events))
		for _, e := range events {
			if query.Kind != "" && e.Kind != query.Kind {
				continue
			}
			if query.SubjectID != "" && e.SubjectID != query.SubjectID {
				continue
			}
			filtered = append(filtered, e)
		}
		events = filtered
	}
	return events, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Optimization Tasks
// ──────────────────────────────────────────────────────────────────────────────

// ListOptimizationTasks 列出所有优化任务。
func (s *Service) ListOptimizationTasks(ctx context.Context) (OptimizationTasks, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return OptimizationTasks{}, fmt.Errorf("adk runtime is unavailable")
	}
	tasks, err := s.runtime.Store().ListOptimizationTasks(ctx)
	if err != nil {
		return OptimizationTasks{}, err
	}
	items := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, s.optimizationTaskResponse(ctx, task))
	}
	return OptimizationTasks{Tasks: items}, nil
}

// GetOptimizationTask 按 ID 获取单个优化任务。
func (s *Service) GetOptimizationTask(ctx context.Context, taskID string) (any, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	task, ok, err := s.runtime.Store().OptimizationTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("optimization task not found")
	}
	return s.optimizationTaskResponse(ctx, task), nil
}

// CancelOptimizationTask 取消优化任务（将状态设为 CANCELLED）。
func (s *Service) CancelOptimizationTask(ctx context.Context, taskID string) (any, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	store := s.runtime.Store()
	task, ok, err := store.OptimizationTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("optimization task not found")
	}
	if s.optimizationRuns != nil {
		for _, ref := range task.Runs {
			s.optimizationRuns.Cancel(ref.RunID)
		}
	}
	task.Status = "cancelled"
	saved, err := store.SaveOptimizationTask(ctx, task)
	if err != nil {
		return nil, err
	}
	s.runtime.RecordAudit(ctx, "optimization.cancelled", saved.ID, "Optimization task cancelled.", nil)
	return s.optimizationTaskResponse(ctx, saved), nil
}

func (s *Service) optimizationTaskResponse(ctx context.Context, task jfadk.OptimizationTask) map[string]any {
	runs := make([]map[string]any, 0, len(task.Runs))
	running := 0
	completed := 0
	failed := 0
	cancelled := 0
	for _, ref := range task.Runs {
		status := "missing"
		var result any
		if s.optimizationRuns != nil {
			if run, ok := s.optimizationRuns.Get(ref.RunID); ok {
				status = run.Status
				result = run.Result
			}
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
			"definitionId": ref.DefinitionID,
			"runId":        ref.RunID,
			"status":       status,
			"result":       result,
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
		updatedTask, jftradeErr1 := s.runtime.Store().SaveOptimizationTask(ctx, task)
		jftradeLogError(jftradeErr1)
		task = updatedTask
	}
	return map[string]any{
		"id": task.ID, "status": status, "objective": task.Objective, "runs": runs,
		"progress": map[string]any{
			"total": len(task.Runs), "running": running, "completed": completed,
			"failed": failed, "cancelled": cancelled,
		},
		"createdAt": task.CreatedAt, "updatedAt": task.UpdatedAt,
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Lifecycle
// ──────────────────────────────────────────────────────────────────────────────

// Close 关闭 ADK 运行时，释放资源。
func (s *Service) Close() error {
	if s.workflowScheduler != nil {
		s.workflowScheduler.Stop()
		s.workflowScheduler = nil
	}
	if s.runtime != nil {
		return s.runtime.Close()
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

// approvalWaitDurationMs 计算审批等待时长（毫秒）。
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

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
