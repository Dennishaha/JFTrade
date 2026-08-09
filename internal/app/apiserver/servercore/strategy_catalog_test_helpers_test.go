package servercore

import (
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategycatalog "github.com/jftrade/jftrade-main/internal/strategy/catalog"
	instancebinding "github.com/jftrade/jftrade-main/internal/strategy/instancebinding"
	instanceview "github.com/jftrade/jftrade-main/internal/strategy/instanceview"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

const (
	strategyStatusRunning           = strategycatalog.StatusRunning
	strategyStatusPaused            = strategycatalog.StatusPaused
	strategyStatusStopped           = strategycatalog.StatusStopped
	strategyExecutionModeLive       = strategycatalog.ExecutionModeLive
	strategyExecutionModeNotifyOnly = strategycatalog.ExecutionModeNotifyOnly
)

func IDPinePlanPlugin() string {
	return instanceview.DefaultPluginID
}

func normalizeStrategyInstanceBinding(input stratsrv.InstanceBinding, params map[string]any) stratsrv.InstanceBinding {
	return instancebinding.NormalizeBinding(input, params)
}

func applyStrategyBindingParams(input *stratsrv.ManagedInstance) {
	instancebinding.ApplyParams(input)
}

func createCatalogInstanceForTest(t *testing.T, server *Server, input stratsrv.ManagedInstance) string {
	t.Helper()
	definition := stratsrv.Definition{
		ID:           input.Definition.StrategyID,
		Name:         input.Definition.Name,
		Version:      input.Definition.Version,
		Runtime:      stratsrv.RuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       stratsrv.DefaultPine(input.Definition.Name),
	}
	if definition.ID == "" {
		definition.ID = input.PluginID
	}
	if definition.ID == "" {
		definition.ID = "catalog-test"
	}
	if definition.Name == "" {
		definition.Name = definition.ID
	}
	if definition.Version == "" {
		definition.Version = stratsrv.DefaultVersion
	}
	binding := input.Binding
	if len(binding.Symbols) == 0 {
		binding.Symbols = []string{"US.AAPL"}
	}
	if binding.Interval == "" {
		binding.Interval = "1m"
	}
	if binding.ExecutionMode == "" {
		binding.ExecutionMode = strategyExecutionModeNotifyOnly
	}
	created, err := server.stores.StrategyCatalog.CreateInstance(definition, binding)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	status := input.Status
	if status != "" && status != strategyStatusStopped {
		if _, err := server.stores.StrategyCatalog.TransitionRuntime(created.ID, status, "test.seeded", "test setup"); err != nil {
			t.Fatalf("TransitionRuntime(%s): %v", status, err)
		}
	}
	return created.ID
}
