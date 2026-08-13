package persistence

import (
	"errors"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"path/filepath"
	"testing"
)

func TestProviderReasoningPersistenceAllowsMappingChanges(t *testing.T) {
	store := newReasoningTestStore(t)
	ctx := t.Context()
	provider, err := store.SaveProvider(ctx, assistantmodel.ProviderWriteRequest{
		ID: "reasoning", APIKey: "secret", Enabled: true,
		ReasoningConfig: &assistantmodel.ProviderReasoningConfig{RequestField: "reasoning.level", Mappings: []assistantmodel.ProviderReasoningMapping{
			{Effort: assistantmodel.ReasoningEffortLow, Value: "LOW"},
			{Effort: assistantmodel.ReasoningEffortHigh, Value: "HIGH"},
		}},
	})
	if err != nil {
		t.Fatalf("SaveProvider: %v", err)
	}
	if _, err := store.SaveAgent(ctx, assistantmodel.AgentWriteRequest{
		ID: "bound-agent", Name: "Bound agent", ProviderID: provider.ID,
		ReasoningEffort: assistantmodel.ReasoningEffortHigh, Status: assistantmodel.AgentStatusEnabled,
	}); err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	updated, err := store.SaveProvider(ctx, assistantmodel.ProviderWriteRequest{
		ID: provider.ID,
		ReasoningConfig: &assistantmodel.ProviderReasoningConfig{RequestField: "reasoning.level", Mappings: []assistantmodel.ProviderReasoningMapping{
			{Effort: assistantmodel.ReasoningEffortLow, Value: "FAST"},
		}},
	})
	if err != nil || len(updated.ReasoningConfig.Mappings) != 1 {
		t.Fatalf("remove referenced mapping = %+v err=%v", updated.ReasoningConfig, err)
	}
	if _, err := store.SaveAgent(ctx, assistantmodel.AgentWriteRequest{
		ID: "invalid-agent", Name: "Invalid agent", ProviderID: provider.ID,
		ReasoningEffort: assistantmodel.ReasoningEffortHigh, Status: assistantmodel.AgentStatusEnabled,
	}); !errors.Is(err, assistantmodel.ErrProviderReasoningUnsupported) {
		t.Fatalf("unsupported agent effort error = %v", err)
	}
}

func TestProviderReasoningPersistenceDefaultsToEmptyMappings(t *testing.T) {
	store := newReasoningTestStore(t)
	provider, err := store.SaveProvider(t.Context(), assistantmodel.ProviderWriteRequest{
		ID: "empty-reasoning", Enabled: true,
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
