package servercoretest

import (
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	strategystore "github.com/jftrade/jftrade-main/internal/store/strategy"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategycatalog "github.com/jftrade/jftrade-main/internal/strategy/catalog"
	"github.com/jftrade/jftrade-main/internal/strategy/instancebinding"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

// openSeededStrategyCatalog opens the same catalog the sidecar opens at
// startup. Callers seed instances, append runtime events, close the catalog,
// and only then create the HTTP test server.
func openSeededStrategyCatalog(t *testing.T, store *servercore.SettingsStore) (strategystore.CatalogResource, error) {
	t.Helper()
	return strategystore.NewCatalog(
		strategystore.DeriveCatalogPath(store.Path()),
		strategystore.DerivePluginTargetDir(store.Path()),
	)
}

func seedStrategyCatalogInstance(t *testing.T, catalog strategystore.CatalogResource, input stratsrv.ManagedInstance) string {
	t.Helper()
	definition := stratsrv.Definition{
		ID:           input.Definition.StrategyID,
		Name:         input.Definition.Name,
		Version:      input.Definition.Version,
		Runtime:      stratsrv.RuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       strategystore.DefaultPine(input.Definition.Name),
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
		binding.ExecutionMode = instancebinding.ExecutionModeNotifyOnly
	}
	created, err := catalog.CreateInstance(definition, binding)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	status := input.Status
	if status != "" && status != strategycatalog.StatusStopped {
		if _, err := catalog.TransitionRuntime(created.ID, status, "test.seeded", "test setup"); err != nil {
			t.Fatalf("TransitionRuntime(%s): %v", status, err)
		}
	}
	return created.ID
}
