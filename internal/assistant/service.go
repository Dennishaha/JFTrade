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
	"strings"
	"time"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

var ErrSessionTimelineFailed = errors.New("adk session timeline failed")

// ──────────────────────────────────────────────────────────────────────────────
// Service
// ──────────────────────────────────────────────────────────────────────────────

// Service ADK 助手业务门面。持有 Runtime 及其聚合的 Store/ToolRegistry/SkillRegistry。
type Service struct {
	runtime           *jfadk.Runtime
	runtimeSettings   func() any
	streamIdleTimeout func() int
	optimizationRuns  OptimizationRuns
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
func (s *Service) Snapshot(ctx context.Context) (any, error) {
	if s.runtime == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	snapshot, err := s.runtime.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	var runtimeSettings any
	if s.runtimeSettings != nil {
		runtimeSettings = s.runtimeSettings()
	}
	return map[string]any{
		"providers":       snapshot.Providers,
		"agents":          snapshot.Agents,
		"skills":          snapshot.Skills,
		"tools":           snapshot.Tools,
		"runtimeSettings": runtimeSettings,
	}, nil
}

// Tools 返回所有注册工具的列表。
func (s *Service) Tools(ctx context.Context) (any, error) {
	if s.runtime == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.Tools().List(), nil
}

// AgentTemplates 返回内置 agent 模板。
func (s *Service) AgentTemplates(ctx context.Context) (any, error) {
	return jfadk.BuiltinAgentTemplates(), nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Tasks
// ──────────────────────────────────────────────────────────────────────────────

// ListTasks 分页列出任务，支持 status/agentId/runId 过滤。
func (s *Service) ListTasks(ctx context.Context, query TaskQuery) (Page[jfadk.Task], error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return Page[jfadk.Task]{}, fmt.Errorf("adk runtime is unavailable")
	}
	tasks, total, err := s.runtime.Store().ListTasksPage(ctx, query.Status, query.AgentID, query.RunID, query.Limit, query.Offset)
	if err != nil {
		return Page[jfadk.Task]{}, err
	}
	return Page[jfadk.Task]{Items: tasks, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

// GetTask 按 ID 获取单个任务。
func (s *Service) GetTask(ctx context.Context, taskID string) (jfadk.Task, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.Task{}, fmt.Errorf("adk runtime is unavailable")
	}
	task, ok, err := s.runtime.Store().Task(ctx, taskID)
	if err != nil {
		return jfadk.Task{}, err
	}
	if !ok {
		return jfadk.Task{}, fmt.Errorf("task not found")
	}
	return task, nil
}

// SaveTask 创建或更新任务。
func (s *Service) SaveTask(ctx context.Context, req jfadk.TaskWriteRequest) (jfadk.Task, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.Task{}, fmt.Errorf("adk runtime is unavailable")
	}
	saved, err := s.runtime.Store().SaveTask(ctx, req)
	if err != nil {
		return jfadk.Task{}, err
	}
	s.runtime.RecordAudit(ctx, "task.saved", saved.ID, "ADK task saved.", map[string]any{"status": saved.Status})
	return saved, nil
}

// UpdateTask 局部更新任务（PATCH 语义）。
func (s *Service) UpdateTask(ctx context.Context, taskID string, req jfadk.TaskPatchRequest) (jfadk.Task, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.Task{}, fmt.Errorf("adk runtime is unavailable")
	}
	updated, err := s.runtime.Store().UpdateTask(ctx, taskID, req)
	if err != nil {
		return jfadk.Task{}, err
	}
	s.runtime.RecordAudit(ctx, "task.updated", updated.ID, "ADK task updated.", map[string]any{"status": updated.Status})
	return updated, nil
}

// DeleteTask 删除任务。
func (s *Service) DeleteTask(ctx context.Context, taskID string) error {
	if s.runtime == nil || s.runtime.Store() == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	if err := s.runtime.Store().DeleteTask(ctx, taskID); err != nil {
		return err
	}
	s.runtime.RecordAudit(ctx, "task.deleted", taskID, "ADK task deleted.", nil)
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Memory
// ──────────────────────────────────────────────────────────────────────────────

// ListMemory 列出记忆条目，支持 scope/agentId/key 过滤。
func (s *Service) ListMemory(ctx context.Context, query MemoryQuery) ([]jfadk.MemoryEntry, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	entries, err := s.runtime.Store().ListMemoryFiltered(ctx, query.Scope, query.AgentID, query.Key)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// SaveMemory 创建或覆写记忆条目。
func (s *Service) SaveMemory(ctx context.Context, req jfadk.MemoryWriteRequest) (jfadk.MemoryEntry, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.MemoryEntry{}, fmt.Errorf("adk runtime is unavailable")
	}
	saved, err := s.runtime.Store().SaveMemory(ctx, req)
	if err != nil {
		return jfadk.MemoryEntry{}, err
	}
	s.runtime.RecordAudit(ctx, "memory.saved", saved.ID, "ADK memory saved.", map[string]any{"scope": saved.Scope, "key": saved.Key})
	return saved, nil
}

// DeleteMemory 删除记忆条目。
func (s *Service) DeleteMemory(ctx context.Context, memoryID string) error {
	if s.runtime == nil || s.runtime.Store() == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	if err := s.runtime.Store().DeleteMemory(ctx, memoryID); err != nil {
		return err
	}
	s.runtime.RecordAudit(ctx, "memory.deleted", memoryID, "ADK memory deleted.", nil)
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Providers
// ──────────────────────────────────────────────────────────────────────────────

// ListProviders 列出所有 AI Provider。
func (s *Service) ListProviders(ctx context.Context) ([]jfadk.Provider, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	items, err := s.runtime.Store().ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// SaveProvider 创建或更新 Provider。
func (s *Service) SaveProvider(ctx context.Context, req jfadk.ProviderWriteRequest) (jfadk.Provider, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.Provider{}, fmt.Errorf("adk runtime is unavailable")
	}
	saved, err := s.runtime.Store().SaveProvider(ctx, req)
	if err != nil {
		return jfadk.Provider{}, err
	}
	s.runtime.RecordAudit(ctx, "provider.saved", saved.ID, "ADK provider saved.", map[string]any{"enabled": saved.Enabled})
	return saved, nil
}

// DeleteProvider 删除 Provider。
func (s *Service) DeleteProvider(ctx context.Context, providerID string) error {
	if s.runtime == nil || s.runtime.Store() == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.Store().DeleteProvider(ctx, providerID)
}

// TestProvider 测试 Provider 连通性与工具能力。
func (s *Service) TestProvider(ctx context.Context, providerID string) (any, error) {
	if s.runtime == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.TestProvider(ctx, providerID)
}

// ──────────────────────────────────────────────────────────────────────────────
// Agents
// ──────────────────────────────────────────────────────────────────────────────

// ListAgents 列出所有 Agent，支持按 status 过滤。
func (s *Service) ListAgents(ctx context.Context, query AgentQuery) ([]jfadk.Agent, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	items, err := s.runtime.Store().ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	if query.Status != "" {
		filtered := make([]jfadk.Agent, 0, len(items))
		for _, a := range items {
			if strings.EqualFold(a.Status, query.Status) {
				filtered = append(filtered, a)
			}
		}
		items = filtered
	}
	return items, nil
}

// SaveAgent 创建或更新 Agent。
func (s *Service) SaveAgent(ctx context.Context, req jfadk.AgentWriteRequest) (jfadk.Agent, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.Agent{}, fmt.Errorf("adk runtime is unavailable")
	}
	if err := s.validateAgent(ctx, req); err != nil {
		return jfadk.Agent{}, err
	}
	saved, err := s.runtime.Store().SaveAgent(ctx, req)
	if err != nil {
		return jfadk.Agent{}, err
	}
	s.runtime.RecordAudit(ctx, "agent.saved", saved.ID, "ADK agent saved.", map[string]any{"status": saved.Status, "permissionMode": saved.PermissionMode})
	return saved, nil
}

// DeleteAgent 删除 Agent。
func (s *Service) DeleteAgent(ctx context.Context, agentID string) error {
	if s.runtime == nil || s.runtime.Store() == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.Store().DeleteAgent(ctx, agentID)
}

func (s *Service) validateAgent(ctx context.Context, payload jfadk.AgentWriteRequest) error {
	status := strings.ToUpper(strings.TrimSpace(payload.Status))
	if status != "" && status != jfadk.AgentStatusEnabled && status != jfadk.AgentStatusDisabled {
		return fmt.Errorf("invalid agent status")
	}
	if strings.TrimSpace(payload.ProviderID) != "" {
		provider, ok, err := s.runtime.Store().Provider(ctx, payload.ProviderID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("provider not found")
		}
		if status != jfadk.AgentStatusDisabled {
			if !provider.Enabled {
				return fmt.Errorf("provider is disabled")
			}
			if _, hasKey, keyErr := s.runtime.Store().ProviderAPIKey(provider.ID); keyErr != nil {
				return keyErr
			} else if !hasKey {
				return fmt.Errorf("provider API key is not configured")
			}
		}
	}
	for _, name := range payload.Tools {
		if _, ok := s.runtime.Tools().CanonicalName(name); !ok {
			return fmt.Errorf("unknown ADK tool: %s", strings.TrimSpace(name))
		}
	}
	for _, id := range payload.Skills {
		if _, ok, err := s.runtime.Skills().Get(ctx, id); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("unknown ADK skill: %s", strings.TrimSpace(id))
		}
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Sessions
// ──────────────────────────────────────────────────────────────────────────────

// ListSessions 分页列出会话，支持 agentId/query 过滤。
func (s *Service) ListSessions(ctx context.Context, query SessionQuery) (Page[jfadk.Session], error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return Page[jfadk.Session]{}, fmt.Errorf("adk runtime is unavailable")
	}
	sessions, total, err := s.runtime.Store().ListSessionsPage(ctx, query.AgentID, query.Query, query.Limit, query.Offset)
	if err != nil {
		return Page[jfadk.Session]{}, err
	}
	return Page[jfadk.Session]{Items: sessions, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

// CreateSession 为指定 agent 创建会话。
func (s *Service) CreateSession(ctx context.Context, req CreateSessionRequest) (jfadk.Session, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.Session{}, fmt.Errorf("adk runtime is unavailable")
	}
	agent, ok, err := s.runtime.Store().Agent(ctx, req.AgentID)
	if err != nil || !ok || agent.Status != jfadk.AgentStatusEnabled || agent.DeletedAt != nil {
		return jfadk.Session{}, fmt.Errorf("enabled agent is required")
	}
	return s.runtime.Store().CreateSession(ctx, req.AgentID, req.Title)
}

// GetSession 按 ID 获取会话。
func (s *Service) GetSession(ctx context.Context, sessionID string) (jfadk.Session, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.Session{}, fmt.Errorf("adk runtime is unavailable")
	}
	session, ok, err := s.runtime.Store().Session(ctx, sessionID)
	if err != nil {
		return jfadk.Session{}, err
	}
	if !ok {
		return jfadk.Session{}, fmt.Errorf("session not found")
	}
	return session, nil
}

// GetSessionDetail returns the normalized session and timeline contract.
func (s *Service) GetSessionDetail(ctx context.Context, sessionID string) (jfadk.SessionsResponse, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.SessionsResponse{}, fmt.Errorf("adk runtime is unavailable")
	}
	session, ok, err := s.runtime.Store().Session(ctx, sessionID)
	if err != nil {
		return jfadk.SessionsResponse{}, err
	}
	if !ok {
		return jfadk.SessionsResponse{}, fmt.Errorf("session not found")
	}
	timeline, _, err := s.runtime.Store().SessionTimeline(ctx, sessionID)
	if err != nil {
		return jfadk.SessionsResponse{}, fmt.Errorf("%w: %v", ErrSessionTimelineFailed, err)
	}
	if timeline == nil {
		timeline = []jfadk.TimelineEntry{}
	}
	return jfadk.NormalizeSessionsResponse(jfadk.SessionsResponse{
		Session:  session,
		Timeline: timeline,
	}), nil
}

// RenameSession 重命名会话。
func (s *Service) RenameSession(ctx context.Context, sessionID string, title string) (jfadk.Session, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.Session{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.Store().RenameSession(ctx, sessionID, title)
}

// DeleteSession 删除会话及其关联的 runs、approvals、context。
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	if s.runtime == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.DeleteSession(ctx, sessionID)
}

// GetSessionContext 获取会话上下文快照。
func (s *Service) GetSessionContext(ctx context.Context, sessionID string) (jfadk.SessionContextSnapshot, error) {
	if s.runtime == nil {
		return jfadk.SessionContextSnapshot{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.SessionContext(ctx, sessionID)
}

// CompactSessionContext 压缩会话上下文。
func (s *Service) CompactSessionContext(ctx context.Context, sessionID string, mode string, trigger string, reason string) (jfadk.SessionContextSnapshot, error) {
	if s.runtime == nil {
		return jfadk.SessionContextSnapshot{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.CompactSessionContext(ctx, sessionID, mode, trigger, reason)
}

// ──────────────────────────────────────────────────────────────────────────────
// Chat
// ──────────────────────────────────────────────────────────────────────────────

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
	normalized := jfadk.NormalizeChatResponse(response)
	return &normalized, nil
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

// GetMetrics 聚合 ADK 运行指标（runs/tools/approvals/usage）。
func (s *Service) GetMetrics(ctx context.Context) (any, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	store := s.runtime.Store()
	runs, err := store.ListRuns(ctx)
	if err != nil {
		return nil, err
	}
	agents, err := store.ListAllAgents(ctx)
	if err != nil {
		return nil, err
	}
	approvals, err := store.ListApprovals(ctx)
	if err != nil {
		return nil, err
	}

	// Build agent→provider lookup
	agentProvider := make(map[string]string, len(agents))
	for _, a := range agents {
		agentProvider[a.ID] = strings.TrimSpace(a.ProviderID)
	}

	// Aggregate run statistics
	statuses := map[string]int{}
	byAgent := map[string]int{}
	byProvider := map[string]int{}
	toolCalls := 0
	successfulTools := 0
	toolsByName := map[string]int{}
	toolsByStatus := map[string]int{}
	var totalDuration int64
	var durationCount int64
	failedRuns := 0
	timedOutRuns := 0
	cancelledRuns := 0
	resumedRuns := 0
	orphanedRuns := 0
	var tokensInTotal int
	var tokensOutTotal int
	tokenSamples := 0

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

	// Aggregate approval statistics
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
		switch approval.Status {
		case jfadk.ApprovalStatusPending:
			pendingApprovals++
			pendingWaitTotal += waitMs
			if waitMs > pendingWaitMax {
				pendingWaitMax = waitMs
			}
			if strings.TrimSpace(approval.FunctionCallID) != "" && strings.TrimSpace(approval.ConfirmationCallID) != "" {
				recoverablePending++
			}
		case jfadk.ApprovalStatusApproved:
			approvedApprovals++
			resolvedWaitTotal += waitMs
			resolvedWaitCount++
			if waitMs > resolvedWaitMax {
				resolvedWaitMax = waitMs
			}
		case jfadk.ApprovalStatusDenied:
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

	return map[string]any{
		"runs": map[string]any{
			"total": len(runs), "byStatus": statuses, "byAgent": byAgent, "byProvider": byProvider,
			"lifecycle": map[string]any{
				"failed": failedRuns, "timedOut": timedOutRuns, "cancelled": cancelledRuns,
				"resumed": resumedRuns, "orphaned": orphanedRuns,
			},
		},
		"tools": map[string]any{
			"total": toolCalls, "successful": successfulTools, "averageDurationMs": averageToolDuration,
			"byName": toolsByName, "byStatus": toolsByStatus,
		},
		"approvals": map[string]any{
			"pending": pendingApprovals, "total": len(approvals),
			"approved": approvedApprovals, "denied": deniedApprovals, "recoverablePending": recoverablePending,
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
		"checkedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
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
		task, _ = s.runtime.Store().SaveOptimizationTask(ctx, task)
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
