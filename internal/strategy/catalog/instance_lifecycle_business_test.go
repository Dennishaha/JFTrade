package catalog

import (
	"errors"
	"strings"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

func TestCatalogInstanceCreateUpdateAndDeleteRespectStoppedBoundary(t *testing.T) {
	service, repository, _ := newCatalogBusinessService(t, Snapshot{})
	definition := catalogBusinessDefinition("mean-revert", "1.0.0")

	created, err := service.CreateInstance(definition, stratsrv.InstanceBinding{
		Symbols:       []string{" us:aapl ", "US:AAPL"},
		Interval:      " 15m ",
		ExecutionMode: ExecutionModeNotifyOnly,
	})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if created.Status != StatusStopped || !created.Startable {
		t.Fatalf("created instance = %#v", created)
	}
	if len(created.Binding.Symbols) != 1 || created.Binding.Symbols[0] != "US.AAPL" {
		t.Fatalf("created binding = %#v", created.Binding)
	}
	if created.Params["definitionId"] != definition.ID || created.Params["runtime"] != stratsrv.RuntimePinePlan {
		t.Fatalf("created params = %#v", created.Params)
	}
	if repository.saveCount() != 1 {
		t.Fatalf("create save count = %d, want 1", repository.saveCount())
	}

	running, err := service.TransitionInstance(created.ID, StatusRunning)
	if err != nil {
		t.Fatalf("TransitionInstance running: %v", err)
	}
	if running.Status != StatusRunning {
		t.Fatalf("running status = %q", running.Status)
	}
	if _, err := service.UpdateInstance(created.ID, stratsrv.InstanceBinding{}); !errors.Is(err, stratsrv.ErrBusy) {
		t.Fatalf("UpdateInstance running error = %v, want busy", err)
	}
	if _, err := service.DeleteInstance(created.ID); !errors.Is(err, stratsrv.ErrBusy) {
		t.Fatalf("DeleteInstance running error = %v, want busy", err)
	}

	risk, err := service.UpdateInstanceRuntimeRisk(created.ID, stratsrv.RuntimeRiskSettings{
		Mode:          "monitor",
		CloseOnly:     true,
		PauseOnReject: true,
	})
	if err != nil {
		t.Fatalf("UpdateInstanceRuntimeRisk while running: %v", err)
	}
	if risk.Binding.RuntimeRisk.Mode != "monitor" || !risk.Binding.RuntimeRisk.CloseOnly {
		t.Fatalf("runtime risk = %#v", risk.Binding.RuntimeRisk)
	}

	if _, err := service.TransitionInstance(created.ID, StatusStopped); err != nil {
		t.Fatalf("TransitionInstance stopped: %v", err)
	}
	updated, err := service.UpdateInstance(created.ID, stratsrv.InstanceBinding{
		Symbols:       []string{"hk:00700"},
		Interval:      "5m",
		ExecutionMode: ExecutionModeLive,
	})
	if err != nil {
		t.Fatalf("UpdateInstance stopped: %v", err)
	}
	if len(updated.Binding.Symbols) != 1 || updated.Binding.Symbols[0] != "HK.00700" {
		t.Fatalf("updated binding = %#v", updated.Binding)
	}
	if symbols, ok := updated.Params["symbols"].([]string); !ok || len(symbols) != 1 || symbols[0] != "HK.00700" {
		t.Fatalf("updated params symbols = %#v", updated.Params["symbols"])
	}

	removed, err := service.DeleteInstance(created.ID)
	if err != nil {
		t.Fatalf("DeleteInstance stopped: %v", err)
	}
	if removed.ID != created.ID || len(service.ListInstances()) != 0 {
		t.Fatalf("removed instance = %#v, remaining = %#v", removed, service.ListInstances())
	}
}

func TestCatalogInstanceOperationsClassifyInvalidAndMissingResources(t *testing.T) {
	service, _, _ := newCatalogBusinessService(t, Snapshot{})
	invalid := catalogBusinessDefinition("invalid", "1.0.0")
	invalid.SourceFormat = "legacy"
	if _, err := service.CreateInstance(invalid, stratsrv.InstanceBinding{}); err == nil {
		t.Fatal("CreateInstance invalid source format error = nil")
	}
	if err := service.ValidateStartable(stratsrv.ManagedInstance{
		Params: map[string]any{"runtime": "legacy-runtime", "sourceFormat": "legacy"},
	}); !errors.Is(err, stratsrv.ErrBadRequest) {
		t.Fatalf("ValidateStartable legacy error = %v, want bad request", err)
	}

	for name, operation := range map[string]func() error{
		"update binding": func() error {
			_, err := service.UpdateInstance("missing", stratsrv.InstanceBinding{})
			return err
		},
		"update risk": func() error {
			_, err := service.UpdateInstanceRuntimeRisk("missing", stratsrv.RuntimeRiskSettings{})
			return err
		},
		"delete": func() error {
			_, err := service.DeleteInstance("missing")
			return err
		},
		"transition": func() error {
			_, err := service.TransitionInstance("missing", StatusRunning)
			return err
		},
		"runtime event": func() error {
			return service.AppendRuntimeEvent("missing", "event", "runtime", "")
		},
		"runtime failure": func() error {
			return service.ReconcileRuntimeFailure("missing", "failure")
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, stratsrv.ErrNotFound) {
				t.Fatalf("error = %v, want not found", err)
			}
		})
	}
	if _, ok := service.GetInstance("missing"); ok {
		t.Fatal("GetInstance missing returned found")
	}
	if _, ok := service.GetLogs("missing", stratsrv.LogQuery{}); ok {
		t.Fatal("GetLogs missing returned found")
	}
	if _, ok := service.GetAudit("missing", stratsrv.AuditQuery{}); ok {
		t.Fatal("GetAudit missing returned found")
	}
}

func TestCatalogDefinitionRefreshPreservesPlacementAndClassifiesLinkedInstances(t *testing.T) {
	snapshot := Snapshot{Strategies: []stratsrv.ManagedInstance{
		catalogBusinessInstance("stale-stopped", "mean-revert", "1.0.0", StatusStopped),
		catalogBusinessInstance("already-latest", "mean-revert", "2.0.0", StatusStopped),
		catalogBusinessInstance("busy-running", "mean-revert", "1.0.0", StatusRunning),
		catalogBusinessInstance("unrelated", "trend", "1.0.0", StatusStopped),
	}}
	service, _, _ := newCatalogBusinessService(t, snapshot)
	latest := catalogBusinessDefinition("mean-revert", "2.0.0")
	latest.Name = "Mean Revert v2"
	latest.Script += "\nfast = ta.sma(close, 10)"

	result, err := service.ApplyDefinitionToLinked(latest)
	if err != nil {
		t.Fatalf("ApplyDefinitionToLinked: %v", err)
	}
	if result.TotalLinked != 3 || result.DefinitionID != latest.ID || result.LatestVersion != latest.Version {
		t.Fatalf("apply summary = %#v", result)
	}
	assertCatalogStringSet(t, result.Applied, []string{"stale-stopped"})
	assertCatalogStringSet(t, result.AlreadyLatest, []string{"already-latest"})
	assertCatalogStringSet(t, result.SkippedBusy, []string{"busy-running"})

	refreshed, ok := service.GetInstance("stale-stopped")
	if !ok {
		t.Fatal("refreshed instance missing")
	}
	if refreshed.Definition.Version != "2.0.0" || refreshed.Definition.Name != latest.Name {
		t.Fatalf("refreshed definition = %#v", refreshed.Definition)
	}
	if refreshed.Binding.Interval != "1m" || refreshed.Binding.ExecutionMode != ExecutionModeNotifyOnly {
		t.Fatalf("placement changed = %#v", refreshed.Binding)
	}
	script, _ := refreshed.Params["script"].(string)
	if !strings.Contains(script, "fast = ta.sma") {
		t.Fatalf("refreshed script = %q", script)
	}

	if _, err := service.RefreshDefinition("busy-running", latest); !errors.Is(err, stratsrv.ErrBusy) {
		t.Fatalf("RefreshDefinition busy error = %v, want busy", err)
	}
	if _, err := service.RefreshDefinition("missing", latest); !errors.Is(err, stratsrv.ErrNotFound) {
		t.Fatalf("RefreshDefinition missing error = %v, want not found", err)
	}
}

func TestCatalogRefreshInstanceDefinitionUsesConfiguredDefinitionStore(t *testing.T) {
	service, _, _ := newCatalogBusinessService(t, Snapshot{Strategies: []stratsrv.ManagedInstance{
		catalogBusinessInstance("instance", "mean-revert", "1.0.0", StatusStopped),
	}})
	if _, err := service.RefreshInstanceDefinition("instance"); !errors.Is(err, stratsrv.ErrNotFound) {
		t.Fatalf("RefreshInstanceDefinition without store error = %v", err)
	}

	latest := catalogBusinessDefinition("mean-revert", "2.0.0")
	service.SetDefinitionStore(catalogDefinitionStore{definition: latest, found: true})
	refreshed, err := service.RefreshInstanceDefinition("instance")
	if err != nil {
		t.Fatalf("RefreshInstanceDefinition: %v", err)
	}
	if refreshed.Definition.Version != latest.Version {
		t.Fatalf("refreshed version = %q", refreshed.Definition.Version)
	}

	service.SetDefinitionStore(catalogDefinitionStore{err: errCatalogRepositoryUnavailable})
	if _, err := service.RefreshInstanceDefinition("instance"); !errors.Is(err, errCatalogRepositoryUnavailable) {
		t.Fatalf("definition store error = %v", err)
	}
}
