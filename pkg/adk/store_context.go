package adk

import (
	"context"
	"os"
	"strings"
)

func (s *Store) SessionContext(ctx context.Context, sessionID string) (SessionContextState, bool, error) {
	var state SessionContextState
	ok, err := s.getJSON(ctx, tableSessionContextLive, sessionID, &state)
	if ok {
		state.SessionID = strings.TrimSpace(sessionID)
		state = ensureSessionContextRevision(state, sessionID)
	}
	return state, ok, err
}

func (s *Store) SaveSessionContext(ctx context.Context, state SessionContextState) (SessionContextState, error) {
	state.SessionID = strings.TrimSpace(state.SessionID)
	if state.SessionID == "" {
		return SessionContextState{}, os.ErrNotExist
	}
	state = ensureSessionContextRevision(state, state.SessionID)
	now := nowString()
	existing, ok, err := s.SessionContext(ctx, state.SessionID)
	if err != nil {
		return SessionContextState{}, err
	}
	if state.CreatedAt == "" {
		if ok {
			state.CreatedAt = existing.CreatedAt
		} else {
			state.CreatedAt = now
		}
	}
	if state.UpdatedAt == "" {
		state.UpdatedAt = now
	}
	return state, s.saveJSON(ctx, tableSessionContextLive, state.SessionID, state.CreatedAt, state.UpdatedAt, state)
}

func (s *Store) DeleteSessionContext(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return os.ErrNotExist
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM `+tableSessionContextLive+` WHERE id = ?`, sessionID)
	return err
}
