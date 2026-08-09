package persistence

import (
	"context"
	"os"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func (s *StoreCore) SessionContext(ctx context.Context, sessionID string) (jfadkmodel.SessionContextState, bool, error) {
	var state jfadkmodel.SessionContextState
	ok, err := s.GetJSON(ctx, tableSessionContextLive, sessionID, &state)
	if ok {
		state.SessionID = strings.TrimSpace(sessionID)
		state = EnsureSessionContextRevision(state, sessionID)
	}
	return state, ok, err
}

func (s *StoreCore) SaveSessionContext(ctx context.Context, state jfadkmodel.SessionContextState) (jfadkmodel.SessionContextState, error) {
	state.SessionID = strings.TrimSpace(state.SessionID)
	if state.SessionID == "" {
		return jfadkmodel.SessionContextState{}, os.ErrNotExist
	}
	state = EnsureSessionContextRevision(state, state.SessionID)
	now := jfadkmodel.NowString()
	existing, ok, err := s.SessionContext(ctx, state.SessionID)
	if err != nil {
		return jfadkmodel.SessionContextState{}, err
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
	return state, s.SaveJSON(ctx, tableSessionContextLive, state.SessionID, state.CreatedAt, state.UpdatedAt, state)
}

func (s *StoreCore) DeleteSessionContext(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return os.ErrNotExist
	}
	_, err := s.DB().ExecContext(ctx, `DELETE FROM `+tableSessionContextLive+` WHERE id = ?`, sessionID)
	return err
}

// EnsureSessionContextRevision fills missing context revision metadata.
func EnsureSessionContextRevision(state jfadkmodel.SessionContextState, sessionID string) jfadkmodel.SessionContextState {
	state.SessionID = strings.TrimSpace(jfadkmodel.DefaultString(state.SessionID, sessionID))
	if strings.TrimSpace(state.ContextRevisionID) == "" {
		state.ContextRevisionID = jfadkmodel.NewContextRevisionID()
	}
	if strings.TrimSpace(state.ContextRevisionCreatedAt) == "" {
		state.ContextRevisionCreatedAt = jfadkmodel.DefaultString(state.CreatedAt, jfadkmodel.NowString())
	}
	return state
}
