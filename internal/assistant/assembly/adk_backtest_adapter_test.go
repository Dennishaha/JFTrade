package assembly

import (
	"strings"
	"testing"
)

func TestADKStrategyValidationAndVisualModelBoundaries(t *testing.T) {
	validation, err := ValidateADKStrategyScript("strategy.save_draft", `//@version=6
strategy("Adapter Save Helper", overlay=true)
log.info("ok")`)
	if err != nil || validation.NormalizedScript == "" || validation.Program == nil {
		t.Fatalf("ValidateADKStrategyScript = %#v, %v", validation, err)
	}
	if got := SourceFormatPineV6(); got != "pine-v6" {
		t.Fatalf("SourceFormatPineV6 = %q", got)
	}

	model, err := strategyVisualModelFromInput(map[string]any{
		"nodes": []map[string]any{{"id": "n1", "type": "note"}},
	})
	if err != nil || model == nil || model.Engine != "logic-flow" || model.Version != 1 || model.Nodes[0].Properties == nil {
		t.Fatalf("strategyVisualModelFromInput(valid) = %#v, %v", model, err)
	}
	if _, err := strategyVisualModelFromInput("not-an-object"); err == nil || !strings.Contains(err.Error(), "visualModel") {
		t.Fatalf("strategyVisualModelFromInput(string) = %v, want validation error", err)
	}
	if _, err := strategyVisualModelFromInput(map[string]any{
		"nodes": []map[string]any{{
			"id": "n1", "type": "note",
			"properties": map[string]any{"blockKind": "codeBlock"},
		}},
	}); err == nil {
		t.Fatal("strategyVisualModelFromInput(legacy block) error = nil")
	}
}
