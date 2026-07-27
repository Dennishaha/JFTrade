package catalog

import (
	"errors"
	"reflect"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

func TestCatalogConstructionPropagatesRepositoryLoadFailure(t *testing.T) {
	repository := &catalogMemoryRepository{loadErr: errCatalogRepositoryUnavailable}
	service, err := New(repository, nil, t.TempDir())
	if service != nil || !errors.Is(err, errCatalogRepositoryUnavailable) {
		t.Fatalf("New = %#v, %v", service, err)
	}
}

func TestCatalogFailedSavesLeaveDurableRepositorySnapshotUnchanged(t *testing.T) {
	baseline := Snapshot{
		Plugins: []ManagedPlugin{{
			Descriptor: stratsrv.PluginDescriptor{ID: "plugin"},
		}},
		Strategies: []stratsrv.ManagedInstance{
			catalogBusinessInstance("stopped", "mean-revert", "1.0.0", StatusStopped),
			catalogBusinessInstance("running", "trend", "1.0.0", StatusRunning),
		},
	}
	tests := []struct {
		name      string
		operation func(*Service) error
	}{
		{
			name: "create instance",
			operation: func(service *Service) error {
				_, err := service.CreateInstance(catalogBusinessDefinition("new", "1.0.0"), stratsrv.InstanceBinding{})
				return err
			},
		},
		{
			name: "update instance",
			operation: func(service *Service) error {
				_, err := service.UpdateInstance("stopped", stratsrv.InstanceBinding{Symbols: []string{"HK.00700"}})
				return err
			},
		},
		{
			name: "delete instance",
			operation: func(service *Service) error {
				_, err := service.DeleteInstance("stopped")
				return err
			},
		},
		{
			name: "transition runtime",
			operation: func(service *Service) error {
				_, err := service.TransitionInstance("stopped", StatusRunning)
				return err
			},
		},
		{
			name: "reconcile runtime failure",
			operation: func(service *Service) error {
				return service.ReconcileRuntimeFailure("running", "worker exited")
			},
		},
		{
			name: "register plugin",
			operation: func(service *Service) error {
				return service.RegisterPlugin(ManagedPlugin{
					Descriptor: stratsrv.PluginDescriptor{ID: "new.plugin"},
				})
			},
		},
		{
			name: "install plugin",
			operation: func(service *Service) error {
				_, err := service.InstallPlugin("plugin")
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &catalogMemoryRepository{
				snapshot: cloneSnapshot(baseline),
				saveErr:  errCatalogRepositoryUnavailable,
			}
			service, err := New(repository, newCatalogMemoryActivityStore(), t.TempDir())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			before := repository.durableSnapshot()
			if err := test.operation(service); !errors.Is(err, errCatalogRepositoryUnavailable) {
				t.Fatalf("operation error = %v, want repository failure", err)
			}
			after := repository.durableSnapshot()
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("failed save committed durable snapshot:\nbefore=%#v\nafter=%#v", before, after)
			}

			reloaded, err := New(repository, nil, t.TempDir())
			if err != nil {
				t.Fatalf("reload after failed save: %v", err)
			}
			if got := reloaded.GetLinkedInstanceIDs("mean-revert"); !reflect.DeepEqual(got, []string{"stopped"}) {
				t.Fatalf("reloaded linked instances = %v", got)
			}
		})
	}
}

func TestCatalogReturnsIndependentCopiesAcrossRepositoryAndCallers(t *testing.T) {
	maxQuantity := 10.0
	snapshot := Snapshot{
		Plugins: []ManagedPlugin{{
			Descriptor: stratsrv.PluginDescriptor{ID: "plugin", Keywords: []string{"alpha"}},
			Artifact: &PluginArtifact{
				Build: stratsrv.PluginBuildTuple{BuildTags: []string{"netgo"}},
			},
		}},
		Strategies: []stratsrv.ManagedInstance{{
			ID: "instance",
			Binding: stratsrv.InstanceBinding{
				Symbols: []string{"US.AAPL"},
				RuntimeRisk: stratsrv.RuntimeRiskSettings{
					MaxOrderQuantity: &maxQuantity,
				},
			},
			Params: map[string]any{
				"runtime": stratsrv.RuntimePinePlan,
				"nested":  map[string]any{"symbols": []any{"US.AAPL"}},
			},
		}},
	}
	repository := &catalogMemoryRepository{snapshot: snapshot}
	service, err := New(repository, nil, t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	instance, ok := service.GetInstance("instance")
	if !ok {
		t.Fatal("instance missing")
	}
	instance.Binding.Symbols[0] = "MUTATED"
	instance.Params["runtime"] = "mutated"
	instance.Params["nested"].(map[string]any)["symbols"].([]any)[0] = "MUTATED"
	second, _ := service.GetInstance("instance")
	if second.Binding.Symbols[0] != "US.AAPL" ||
		second.Params["runtime"] != stratsrv.RuntimePinePlan ||
		second.Params["nested"].(map[string]any)["symbols"].([]any)[0] != "US.AAPL" {
		t.Fatalf("caller mutation leaked into service state = %#v", second)
	}

	durable := repository.durableSnapshot()
	durable.Plugins[0].Descriptor.Keywords[0] = "mutated"
	durable.Plugins[0].Artifact.Build.BuildTags[0] = "mutated"
	again := repository.durableSnapshot()
	if again.Plugins[0].Descriptor.Keywords[0] != "alpha" ||
		again.Plugins[0].Artifact.Build.BuildTags[0] != "netgo" {
		t.Fatalf("repository snapshot clone leaked mutation = %#v", again.Plugins[0])
	}
}
