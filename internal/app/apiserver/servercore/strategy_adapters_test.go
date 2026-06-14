package servercore

import "testing"

func TestStrategyAdapterDefinitionSyncSupportsLegacyParams(t *testing.T) {
	adapter := &strategyCatalogStoreAdapter{}
	status := adapter.buildDefinitionSyncStatus(strategyListItem{
		Definition: strategyDefinitionSummary{Version: "0.1.0"},
		Params:     map[string]any{"definitionId": "legacy-definition"},
	})
	if status == nil {
		t.Fatal("expected definition sync status")
	}
	if status.DefinitionID != "legacy-definition" {
		t.Fatalf("definitionId = %q", status.DefinitionID)
	}
	if status.AppliedVersion != "0.1.0" || !status.IsLatest {
		t.Fatalf("unexpected definition sync status: %+v", status)
	}
}
