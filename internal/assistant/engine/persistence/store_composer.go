package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func (s *StoreCore) SessionComposerState(ctx context.Context, sessionID string) (jfadkmodel.SessionComposerState, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return jfadkmodel.SessionComposerState{}, false, os.ErrNotExist
	}
	var state jfadkmodel.SessionComposerState
	ok, err := s.GetJSON(ctx, tableSessionComposer, sessionID, &state)
	if err != nil {
		return jfadkmodel.SessionComposerState{}, false, err
	}
	if !ok {
		return defaultSessionComposerState(sessionID), false, nil
	}
	state = NormalizeSessionComposerState(sessionID, state)
	return state, true, nil
}

func (s *StoreCore) SaveSessionComposerState(ctx context.Context, sessionID string, patch jfadkmodel.SessionComposerStatePatch) (jfadkmodel.SessionComposerState, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return jfadkmodel.SessionComposerState{}, os.ErrNotExist
	}
	if _, ok, err := s.Session(ctx, sessionID); err != nil {
		return jfadkmodel.SessionComposerState{}, err
	} else if !ok {
		return jfadkmodel.SessionComposerState{}, os.ErrNotExist
	}
	state, ok, err := s.SessionComposerState(ctx, sessionID)
	if err != nil {
		return jfadkmodel.SessionComposerState{}, err
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
			return jfadkmodel.SessionComposerState{}, err
		}
		state.WorkModeOverride = mode
	}
	if patch.PermissionModeOverride != nil {
		mode, err := normalizeSessionComposerPermissionMode(*patch.PermissionModeOverride)
		if err != nil {
			return jfadkmodel.SessionComposerState{}, err
		}
		state.PermissionModeOverride = mode
	}
	if patch.GoalObjectiveDraft != nil {
		state.GoalObjectiveDraft = limitComposerText(*patch.GoalObjectiveDraft)
	}
	if patch.GoalObjectiveTouched != nil {
		state.GoalObjectiveTouched = *patch.GoalObjectiveTouched
	}
	state = NormalizeSessionComposerState(sessionID, state)
	state.UpdatedAt = jfadkmodel.NowString()
	payload, err := json.Marshal(state)
	if err != nil {
		return jfadkmodel.SessionComposerState{}, err
	}
	_, err = s.DB().ExecContext(
		ctx,
		`INSERT INTO `+tableSessionComposer+` (id, session_id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET session_id = excluded.session_id, payload_json = excluded.payload_json, updated_at = excluded.updated_at`,
		sessionID, sessionID, string(payload), state.UpdatedAt, state.UpdatedAt,
	)
	return state, err
}

func defaultSessionComposerState(sessionID string) jfadkmodel.SessionComposerState {
	return jfadkmodel.SessionComposerState{SessionID: strings.TrimSpace(sessionID)}
}

// NormalizeSessionComposerState applies shared composer state normalization.
func NormalizeSessionComposerState(sessionID string, state jfadkmodel.SessionComposerState) jfadkmodel.SessionComposerState {
	state.SessionID = strings.TrimSpace(jfadkmodel.DefaultString(state.SessionID, sessionID))
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
	if !jfadkmodel.ValidPermissionMode(mode) {
		return "", fmt.Errorf("invalid composer permission mode %q", mode)
	}
	return jfadkmodel.NormalizePermissionMode(mode), nil
}

func normalizeSessionComposerWorkMode(mode string) (string, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return "", nil
	}
	if !jfadkmodel.ValidWorkMode(mode) {
		return "", fmt.Errorf("invalid composer work mode %q", mode)
	}
	return jfadkmodel.NormalizeWorkMode(mode), nil
}

func limitComposerText(value string) string {
	if len([]rune(value)) <= jfadkmodel.MaxMessageLength {
		return value
	}
	return string([]rune(value)[:jfadkmodel.MaxMessageLength])
}
