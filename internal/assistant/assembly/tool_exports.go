package assembly

import (
	"context"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
)

// The exported registration slices let focused callers install only the tool
// families they need without reaching into the private assistant engine.
func RegisterProductToolSet(registry *jfadk.ToolRegistry, deps ToolDeps) {
	registerJFTradeProductTools(registry, deps)
}

func RegisterStrategyTools(store *jfadk.Store, registry *jfadk.ToolRegistry, deps ToolDeps) {
	registerJFTradeADKStrategyTools(store, registry, deps)
}

func RegisterStrategyResearchTools(registry *jfadk.ToolRegistry, deps ToolDeps) {
	registerADKStrategyResearchTools(registry, deps)
}

func RegisterStrategyOptimizationTools(store *jfadk.Store, registry *jfadk.ToolRegistry, deps ToolDeps) {
	registerADKStrategyOptimizationTools(store, registry, deps)
}

func RecordWorkflowAudit(ctx context.Context, deps ToolDeps, kind string, subjectID string, detail string, metadata map[string]any) {
	recordADKWorkflowAudit(ctx, deps, kind, subjectID, detail, metadata)
}
