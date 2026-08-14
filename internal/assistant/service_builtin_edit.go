package assistant

import (
	"context"
	"fmt"
	"slices"
	"strings"

	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

// validatePrimaryBuiltinAgentUpdate keeps the built-in agent's identity and
// behavior immutable while allowing its provider-specific reasoning settings
// to be changed through the normal Agent save contract.
func (s *Service) validatePrimaryBuiltinAgentUpdate(ctx context.Context, req assistantmodel.AgentWriteRequest) error {
	current, ok, err := s.runtime.Store().Agent(ctx, assistantmodel.DefaultBuiltinAgentID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: primary builtin agent not found", assistantmodel.ErrBuiltinAgentProtected)
	}
	if primaryBuiltinProtectedFieldsMatch(req, current) {
		return nil
	}
	return fmt.Errorf("%w: only provider, model, and reasoning effort can be edited", assistantmodel.ErrBuiltinAgentProtected)
}

func primaryBuiltinProtectedFieldsMatch(req assistantmodel.AgentWriteRequest, current assistantmodel.Agent) bool {
	status := strings.ToUpper(strings.TrimSpace(req.Status))
	if status == "" {
		status = assistantmodel.AgentStatusEnabled
	}
	return strings.TrimSpace(req.Name) == current.Name &&
		strings.TrimSpace(req.Instruction) == current.Instruction &&
		slices.Equal(assistantmodel.NormalizeStringSlice(req.Tools), assistantmodel.NormalizeStringSlice(current.Tools)) &&
		assistantmodel.NormalizeToolAccessMode(req.ToolAccessMode, req.Tools) == assistantmodel.NormalizeToolAccessMode(current.ToolAccessMode, current.Tools) &&
		slices.Equal(assistantmodel.NormalizeStringSlice(req.Skills), assistantmodel.NormalizeStringSlice(current.Skills)) &&
		assistantmodel.NormalizePermissionMode(req.PermissionMode) == current.PermissionMode &&
		req.MemoryEnabled == current.MemoryEnabled &&
		assistantmodel.NormalizeRecentUserWindow(req.RecentUserWindow) == current.RecentUserWindow &&
		assistantmodel.NormalizeAgentDefaultWorkMode(req.WorkMode) == current.WorkMode &&
		assistantmodel.NormalizeLoopMaxIterations(req.LoopMaxIterations) == current.LoopMaxIterations &&
		status == current.Status
}
