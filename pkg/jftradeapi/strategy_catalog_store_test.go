package jftradeapi

import "testing"

func TestNormalizeStrategyDoesNotShareReferenceFields(t *testing.T) {
	store := &strategyCatalogStore{}
	input := managedStrategyInstance{
		ID:       "instance-1",
		PluginID: IDQuickJSPlugin(),
		Definition: strategyDefinitionSummary{
			StrategyID: "mean-revert",
			Name:       "Mean Revert",
			Version:    "0.1.0",
		},
		Params: map[string]any{
			"definitionId": "mean-revert",
		},
		Logs: []string{"started"},
		AuditEntries: []strategyAuditEntry{{
			InstanceID: "instance-1",
			Kind:       "started",
			Detail:     "mean-revert",
			At:         "2026-05-23T17:54:38Z",
		}},
	}

	normalized := store.normalizeStrategy(input)

	if _, ok := input.Params["runtime"]; ok {
		t.Fatal("normalizeStrategy mutated input params")
	}
	if got, ok := normalized.Params["runtime"].(string); !ok || got == "" {
		t.Fatalf("normalized runtime = %#v, want non-empty string", normalized.Params["runtime"])
	}

	normalized.Params["definitionId"] = "changed"
	normalized.Logs[0] = "mutated"
	normalized.AuditEntries[0].Kind = "mutated"

	if got := input.Params["definitionId"]; got != "mean-revert" {
		t.Fatalf("input params shared with normalized copy: %#v", got)
	}
	if got := input.Logs[0]; got != "started" {
		t.Fatalf("input logs shared with normalized copy: %q", got)
	}
	if got := input.AuditEntries[0].Kind; got != "started" {
		t.Fatalf("input audit entries shared with normalized copy: %q", got)
	}
}
