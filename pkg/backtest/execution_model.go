package backtest

import (
	"fmt"
	"strings"
)

const (
	// ExecutionModelConservativeBarV1 is the project backtest fill model that
	// replaces bbgo's native whole-order matching while still reusing bbgo
	// session/account/stream infrastructure.
	ExecutionModelConservativeBarV1 = "conservative-bar-v1"

	DefaultExecutionModel = ExecutionModelConservativeBarV1
)

// NormalizeExecutionModelName resolves the user/API-facing execution model
// name. An empty value intentionally selects the current default.
func NormalizeExecutionModelName(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return DefaultExecutionModel, nil
	}
	switch normalized {
	case ExecutionModelConservativeBarV1:
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported backtest executionModel: %s", value)
	}
}
