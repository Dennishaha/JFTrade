package assistant

import (
	"context"
	"fmt"
	"strings"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

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

// SetDefaultProvider 将指定 Provider 设为默认模型。
func (s *Service) SetDefaultProvider(ctx context.Context, providerID string) (jfadk.Provider, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.Provider{}, fmt.Errorf("adk runtime is unavailable")
	}
	provider, err := s.runtime.Store().SetDefaultProvider(ctx, providerID)
	if err != nil {
		return jfadk.Provider{}, err
	}
	s.runtime.RecordAudit(ctx, "provider.default_set", provider.ID, "ADK default provider changed.", map[string]any{"providerId": provider.ID})
	return provider, nil
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
	if jfadk.IsPrimaryBuiltinAgentID(strings.TrimSpace(req.ID)) {
		return jfadk.Agent{}, fmt.Errorf("%w: primary builtin agent cannot be edited", jfadk.ErrBuiltinAgentProtected)
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
	if strings.TrimSpace(payload.WorkMode) != "" {
		switch strings.ToLower(strings.TrimSpace(payload.WorkMode)) {
		case jfadk.WorkModeChat, jfadk.WorkModeLoop:
		default:
			return fmt.Errorf("invalid agent work mode")
		}
	}
	if payload.LoopMaxIterations < 0 || payload.LoopMaxIterations > jfadk.MaxLoopIterations {
		return fmt.Errorf("loop max iterations must be between 1 and %d", jfadk.MaxLoopIterations)
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
