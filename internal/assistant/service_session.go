package assistant

import (
	"context"
	"fmt"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

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
	return s.runtime.Store().CreateSessionWithSource(ctx, req.AgentID, req.Title, req.WorkflowID, req.WorkflowName)
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
		return jfadk.SessionsResponse{}, wrapSessionTimelineError(err)
	}
	if timeline == nil {
		timeline = []jfadk.TimelineEntry{}
	}
	runs, err := s.runtime.Store().SessionRuns(ctx, sessionID)
	if err != nil {
		return jfadk.SessionsResponse{}, err
	}
	composerState, _, err := s.runtime.Store().SessionComposerState(ctx, sessionID)
	if err != nil {
		return jfadk.SessionsResponse{}, err
	}
	return jfadk.NormalizeSessionsResponse(jfadk.SessionsResponse{
		Session:       session,
		Timeline:      timeline,
		Runs:          runs,
		ComposerState: composerState,
	}), nil
}

// RenameSession 重命名会话。
func (s *Service) RenameSession(ctx context.Context, sessionID string, title string) (jfadk.Session, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.Session{}, fmt.Errorf("adk runtime is unavailable")
	}
	return s.runtime.Store().RenameSession(ctx, sessionID, title)
}

func (s *Service) UpdateSessionComposerState(ctx context.Context, sessionID string, patch jfadk.SessionComposerStatePatch) (jfadk.SessionComposerState, error) {
	if s.runtime == nil || s.runtime.Store() == nil {
		return jfadk.SessionComposerState{}, fmt.Errorf("adk runtime is unavailable")
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
