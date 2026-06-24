package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func (s *Store) SessionComposerState(ctx context.Context, sessionID string) (SessionComposerState, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionComposerState{}, false, os.ErrNotExist
	}
	var state SessionComposerState
	ok, err := s.getJSON(ctx, tableSessionComposer, sessionID, &state)
	if err != nil {
		return SessionComposerState{}, false, err
	}
	if !ok {
		return defaultSessionComposerState(sessionID), false, nil
	}
	state = normalizeSessionComposerState(sessionID, state)
	return state, true, nil
}

func (s *Store) SaveSessionComposerState(ctx context.Context, sessionID string, patch SessionComposerStatePatch) (SessionComposerState, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionComposerState{}, os.ErrNotExist
	}
	if _, ok, err := s.Session(ctx, sessionID); err != nil {
		return SessionComposerState{}, err
	} else if !ok {
		return SessionComposerState{}, os.ErrNotExist
	}
	state, ok, err := s.SessionComposerState(ctx, sessionID)
	if err != nil {
		return SessionComposerState{}, err
	}
	if !ok {
		state = defaultSessionComposerState(sessionID)
	}
	if patch.ChatDraft != nil {
		state.ChatDraft = limitComposerText(*patch.ChatDraft)
	}
	if patch.ProviderIDOverride != nil {
		state.ProviderIDOverride = strings.TrimSpace(*patch.ProviderIDOverride)
	}
	if patch.ModelOverride != nil {
		state.ModelOverride = strings.TrimSpace(*patch.ModelOverride)
	}
	if patch.WorkModeOverride != nil {
		mode, err := normalizeSessionComposerWorkMode(*patch.WorkModeOverride)
		if err != nil {
			return SessionComposerState{}, err
		}
		state.WorkModeOverride = mode
	}
	if patch.PermissionModeOverride != nil {
		mode, err := normalizeSessionComposerPermissionMode(*patch.PermissionModeOverride)
		if err != nil {
			return SessionComposerState{}, err
		}
		state.PermissionModeOverride = mode
	}
	if patch.GoalObjectiveDraft != nil {
		state.GoalObjectiveDraft = limitComposerText(*patch.GoalObjectiveDraft)
	}
	if patch.GoalObjectiveTouched != nil {
		state.GoalObjectiveTouched = *patch.GoalObjectiveTouched
	}
	state = normalizeSessionComposerState(sessionID, state)
	state.UpdatedAt = nowString()
	payload, err := json.Marshal(state)
	if err != nil {
		return SessionComposerState{}, err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO `+tableSessionComposer+` (id, session_id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET session_id = excluded.session_id, payload_json = excluded.payload_json, updated_at = excluded.updated_at`,
		sessionID, sessionID, string(payload), state.UpdatedAt, state.UpdatedAt,
	)
	return state, err
}

func defaultSessionComposerState(sessionID string) SessionComposerState {
	return SessionComposerState{SessionID: strings.TrimSpace(sessionID)}
}

func normalizeSessionComposerState(sessionID string, state SessionComposerState) SessionComposerState {
	state.SessionID = strings.TrimSpace(defaultString(state.SessionID, sessionID))
	state.ChatDraft = limitComposerText(state.ChatDraft)
	mode, err := normalizeSessionComposerWorkMode(state.WorkModeOverride)
	if err != nil {
		mode = ""
	}
	state.WorkModeOverride = mode
	state.ProviderIDOverride = strings.TrimSpace(state.ProviderIDOverride)
	state.ModelOverride = strings.TrimSpace(state.ModelOverride)
	permissionMode, err := normalizeSessionComposerPermissionMode(state.PermissionModeOverride)
	if err != nil {
		permissionMode = ""
	}
	state.PermissionModeOverride = permissionMode
	state.GoalObjectiveDraft = limitComposerText(state.GoalObjectiveDraft)
	return state
}

func normalizeSessionComposerPermissionMode(mode string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return "", nil
	}
	if !validPermissionMode(mode) {
		return "", fmt.Errorf("invalid composer permission mode %q", mode)
	}
	return normalizePermissionMode(mode), nil
}

func normalizeSessionComposerWorkMode(mode string) (string, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return "", nil
	}
	if !validWorkMode(mode) {
		return "", fmt.Errorf("invalid composer work mode %q", mode)
	}
	return normalizeWorkMode(mode), nil
}

func limitComposerText(value string) string {
	if len([]rune(value)) <= MaxMessageLength {
		return value
	}
	return string([]rune(value)[:MaxMessageLength])
}
