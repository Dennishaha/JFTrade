package persistence

import (
	"errors"
	"path/filepath"
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestProviderReasoningPersistenceAllowsMappingChanges(t *testing.T) {
	store := newReasoningTestStore(t)
	ctx := t.Context()
	provider, err := store.SaveProvider(ctx, jfadkmodel.ProviderWriteRequest{
		ID: "reasoning", APIKey: "secret", Enabled: true,
		ReasoningConfig: &jfadkmodel.ProviderReasoningConfig{RequestField: "reasoning.level", Mappings: []jfadkmodel.ProviderReasoningMapping{
			{Effort: jfadkmodel.ReasoningEffortLow, Value: "LOW"},
			{Effort: jfadkmodel.ReasoningEffortHigh, Value: "HIGH"},
		}},
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if _, err := store.SaveAgent(ctx, jfadkmodel.AgentWriteRequest{
		ID: "bound-agent", Name: "Bound agent", ProviderID: provider.ID,
		ReasoningEffort: jfadkmodel.ReasoningEffortHigh, Status: jfadkmodel.AgentStatusEnabled,
	}); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	updated, err := store.SaveProvider(ctx, jfadkmodel.ProviderWriteRequest{
		ID: provider.ID,
		ReasoningConfig: &jfadkmodel.ProviderReasoningConfig{RequestField: "reasoning.level", Mappings: []jfadkmodel.ProviderReasoningMapping{
			{Effort: jfadkmodel.ReasoningEffortLow, Value: "FAST"},
		}},
	})
	if err != nil || len(updated.ReasoningConfig.Mappings) != 1 {
		t.Fatalf("remove referenced mapping = %+v err=%v", updated.ReasoningConfig, err)
	}
	if _, err := store.SaveAgent(ctx, jfadkmodel.AgentWriteRequest{
		ID: "invalid-agent", Name: "Invalid agent", ProviderID: provider.ID,
		ReasoningEffort: jfadkmodel.ReasoningEffortHigh, Status: jfadkmodel.AgentStatusEnabled,
	}); !errors.Is(err, jfadkmodel.ErrProviderReasoningUnsupported) {
		t.Fatalf("unsupported agent effort error = %v", err)
	}
}

func TestProviderReasoningPersistenceDefaultsToEmptyMappings(t *testing.T) {
	store := newReasoningTestStore(t)
	provider, err := store.SaveProvider(t.Context(), jfadkmodel.ProviderWriteRequest{
		ID: "empty-reasoning", APIProtocol: jfadkmodel.ProviderAPIProtocolResponses, Enabled: true,
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if provider.ReasoningConfig.RequestField != "reasoning.effort" || len(provider.ReasoningConfig.Mappings) != 0 {
		t.Fatalf("default reasoning config = %+v", provider.ReasoningConfig)
	}
}

func newReasoningTestStore(t *testing.T) *StoreCore {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStoreCore(
		filepath.Join(dir, "adk.db"),
		filepath.Join(dir, "secrets", "adk.json"),
		filepath.Join(dir, "skills"),
	)
	if err != nil {
		t.Fatalf("NewStoreCore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
