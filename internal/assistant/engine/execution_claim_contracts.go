package adk

import enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"

const (
	ToolIdempotencyFailClosed = enginepersistence.ToolIdempotencyFailClosed
	ToolIdempotencyReplaySafe = enginepersistence.ToolIdempotencyReplaySafe
	ToolIdempotencyKeyed      = enginepersistence.ToolIdempotencyKeyed
)

func normalizeToolIdempotencyMode(mode string, permission string) string {
	return enginepersistence.NormalizeToolIdempotencyMode(mode, permission)
}
