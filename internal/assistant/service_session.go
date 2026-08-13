package assistant

import (
	"context"
	"fmt"

	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func (s *Service) ListSessions(ctx context.Context, query SessionQuery) (Page[assistantmodel.Session], error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return Page[assistantmodel.Session]{}, fmt.Errorf("adk runtime is unavailable")
	}
	sessions, total, err := s.runtime.Store().ListSessionsPage(ctx, query.AgentID, query.Query, query.Limit, query.Offset)
	if err != nil {
		return Page[assistantmodel.Session]{}, err
	}
	return Page[assistantmodel.Session]{Items: sessions, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

// CreateSession 为指定 agent 创建会话。
func (s *Service) CreateSession(ctx context.Context, req CreateSessionRequest) (assistantmodel.Session, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return assistantmodel.Session{}, fmt.Errorf("adk runtime is unavailable")
	}
	agent, ok, err := s.runtime.Store().Agent(ctx, req.AgentID)
	if err != nil || !ok || agent.Status != assistantmodel.AgentStatusEnabled || agent.DeletedAt != nil {
		return assistantmodel.Session{}, fmt.Errorf("enabled agent is required")
	}
	return s.runtime.Store().CreateSessionWithSource(ctx, req.AgentID, req.Title, req.WorkflowID, req.WorkflowName)
}

// GetSession 按 ID 获取会话。
func (s *Service) GetSession(ctx context.Context, sessionID string) (assistantmodel.Session, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return assistantmodel.Session{}, fmt.Errorf("adk runtime is unavailable")
	}
	session, ok, err := s.runtime.Store().Session(ctx, sessionID)
	if err != nil {
		return assistantmodel.Session{}, err
	}
	if !ok {
		return assistantmodel.Session{}, fmt.Errorf("session not found")
	}
	return session, nil
}

// GetSessionDetail returns the normalized session and timeline contract.
func (s *Service) GetSessionDetail(ctx context.Context, sessionID string) (assistantmodel.SessionsResponse, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return assistantmodel.SessionsResponse{}, fmt.Errorf("adk runtime is unavailable")
	}
	session, ok, err := s.runtime.Store().Session(ctx, sessionID)
	if err != nil {
		return assistantmodel.SessionsResponse{}, err
	}
	if !ok {
		return assistantmodel.SessionsResponse{}, fmt.Errorf("session not found")
	}
	timeline, _, err := s.runtime.Store().SessionTimeline(ctx, sessionID)
	if err != nil {
		return assistantmodel.SessionsResponse{}, wrapSessionTimelineError(err)
	}
	if timeline == nil {
		timeline = []assistantmodel.TimelineEntry{}
	}
	runs, err := s.runtime.Store().SessionRuns(ctx, sessionID)
	if err != nil {
		return assistantmodel.SessionsResponse{}, err
	}
	composerState, _, err := s.runtime.Store().SessionComposerState(ctx, sessionID)
	if err != nil {
		return assistantmodel.SessionsResponse{}, err
	}
	return assistantmodel.NormalizeSessionsResponse(assistantmodel.SessionsResponse{
		Session:       session,
		Timeline:      timeline,
		Runs:          runs,
		ComposerState: composerState,
	}), nil
}

// RenameSession 重命名会话。
func (s *Service) RenameSession(ctx context.Context, sessionID string, title string) (assistantmodel.Session, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return assistantmodel.Session{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.Store().RenameSession(ctx, sessionID, title)
}

func (s *Service) UpdateSessionComposerState(ctx context.Context, sessionID string, patch assistantmodel.SessionComposerStatePatch) (assistantmodel.SessionComposerState, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return assistantmodel.SessionComposerState{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.Store().SaveSessionComposerState(ctx, sessionID, patch)
}

// DeleteSession 删除会话及其关联的 runs、approvals、context。
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	if s.runtime == nil {
		return fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.DeleteSession(ctx, sessionID)
}

// GetSessionContext 获取会话上下文快照。
func (s *Service) GetSessionContext(ctx context.Context, sessionID string) (assistantmodel.SessionContextSnapshot, error) {
	if s.runtime == nil {
		return assistantmodel.SessionContextSnapshot{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.SessionContext(ctx, sessionID)
}

// CompactSessionContext 压缩会话上下文。
func (s *Service) CompactSessionContext(ctx context.Context, sessionID string, mode string, trigger string, reason string) (assistantmodel.SessionContextSnapshot, error) {
	if s.runtime == nil {
		return assistantmodel.SessionContextSnapshot{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.CompactSessionContext(ctx, sessionID, mode, trigger, reason)
}

// ──────────────────────────────────────────────────────────────────────────────
// Chat
// ──────────────────────────────────────────────────────────────────────────────
