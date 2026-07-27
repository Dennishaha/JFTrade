package catalog

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
)

type catalogQueryFailureActivityStore struct {
	*catalogMemoryActivityStore
	listLogsErr  error
	listAuditErr error
}

func (s catalogQueryFailureActivityStore) ListLogs(
	context.Context,
	runtimeactivity.LogQuery,
) ([]runtimeactivity.LogEvent, error) {
	return nil, s.listLogsErr
}

func (s catalogQueryFailureActivityStore) ListAudit(
	context.Context,
	runtimeactivity.AuditQuery,
) ([]runtimeactivity.AuditEvent, error) {
	return nil, s.listAuditErr
}

func TestCatalogActivityQueryFailureReturnsKnownEmptyPage(t *testing.T) {
	repository := &catalogMemoryRepository{snapshot: Snapshot{Strategies: []stratsrv.ManagedInstance{
		catalogBusinessInstance("activity", "definition", "1.0.0", StatusStopped),
	}}}
	queryErr := errors.New("activity query unavailable")
	service, err := New(repository, catalogQueryFailureActivityStore{
		catalogMemoryActivityStore: newCatalogMemoryActivityStore(),
		listLogsErr:                queryErr,
		listAuditErr:               queryErr,
	}, t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	logs, ok := service.GetLogs("activity", stratsrv.LogQuery{Limit: 10})
	if !ok || len(logs.Logs) != 0 || logs.Page.Total != 0 {
		t.Fatalf("degraded logs = %#v, found=%v", logs, ok)
	}
	audit, ok := service.GetAudit("activity", stratsrv.AuditQuery{Limit: 10})
	if !ok || len(audit.Entries) != 0 || audit.Page.Total != 0 {
		t.Fatalf("degraded audit = %#v, found=%v", audit, ok)
	}
}

func TestCatalogActivityWriteFailureDoesNotBlockControlState(t *testing.T) {
	repository := &catalogMemoryRepository{}
	service, err := New(
		repository,
		degradedActivityStore{err: errors.New("activity write unavailable")},
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	created, err := service.CreateInstance(catalogBusinessDefinition("activity-write", "1.0.0"), stratsrv.InstanceBinding{})
	if err != nil {
		t.Fatalf("CreateInstance should tolerate activity failure: %v", err)
	}
	if _, err := service.TransitionRuntime(created.ID, StatusRunning, "", ""); err != nil {
		t.Fatalf("TransitionRuntime should tolerate activity failure: %v", err)
	}
	instance, ok := service.GetInstance(created.ID)
	if !ok || instance.Status != StatusRunning {
		t.Fatalf("control state after activity failure = %#v, found=%v", instance, ok)
	}

	service.recordEventsLocked(nil, time.Now(), "ignored", "info", "runtime", "ignored", "")
	service.recordEventsLocked(&instance, time.Now(), " ", "info", "runtime", "", "")
}

func TestCatalogDefinitionSyncExplainsLatestRefreshableAndBusyStates(t *testing.T) {
	staleDefinition := catalogBusinessDefinition("definition", "2.0.0")
	definitions := catalogDefinitionStore{definition: staleDefinition, found: true}

	if status := buildDefinitionSyncStatus(stratsrv.InstanceView{}, definitions); status != nil {
		t.Fatalf("unlinked definition sync = %#v", status)
	}
	unversioned := stratsrv.InstanceView{
		Status: StatusStopped,
		Params: map[string]any{"definitionId": "definition"},
	}
	if status := buildDefinitionSyncStatus(unversioned, nil); status == nil || !status.IsLatest {
		t.Fatalf("sync without definition store = %#v", status)
	}
	if status := buildDefinitionSyncStatus(unversioned, catalogDefinitionStore{found: false}); status == nil || !status.IsLatest {
		t.Fatalf("sync with missing definition = %#v", status)
	}

	staleStopped := stratsrv.InstanceView{
		Status:     StatusStopped,
		Definition: stratsrv.DefinitionSummary{StrategyID: "definition", Version: "1.0.0"},
	}
	status := buildDefinitionSyncStatus(staleStopped, definitions)
	if status == nil || status.IsLatest || !status.CanApplyLatest || status.LatestVersion != "2.0.0" {
		t.Fatalf("stale stopped sync = %#v", status)
	}
	staleStopped.Status = StatusRunning
	status = buildDefinitionSyncStatus(staleStopped, definitions)
	if status == nil || status.CanApplyLatest || status.BlockedReason == nil {
		t.Fatalf("stale busy sync = %#v", status)
	}

	latest := staleStopped
	latest.Definition.Version = "2.0.0"
	if status := buildDefinitionSyncStatus(latest, definitions); status == nil || !status.IsLatest {
		t.Fatalf("latest sync = %#v", status)
	}
}

func TestCatalogNormalizationAndClonePreserveCallerIsolation(t *testing.T) {
	service, err := New(nil, nil, " ")
	if err != nil {
		t.Fatalf("New in-memory catalog: %v", err)
	}
	if service.PluginCatalog().TargetDir != defaultPluginDir {
		t.Fatalf("default target dir = %q", service.PluginCatalog().TargetDir)
	}
	if err := service.persistLocked(); err != nil {
		t.Fatalf("in-memory persist: %v", err)
	}

	normalized := service.normalizeSnapshot(Snapshot{})
	if normalized.Plugins == nil || normalized.Strategies == nil || normalized.Operations == nil {
		t.Fatalf("normalized empty snapshot = %#v", normalized)
	}
	instance := service.normalizeStrategy(stratsrv.ManagedInstance{})
	if instance.ID == "" || instance.PluginID == "" || instance.CreatedAt == "" {
		t.Fatalf("normalized empty instance = %#v", instance)
	}
	if err := service.ValidateStartable(instance); err != nil {
		t.Fatalf("normalized Pine instance should be startable: %v", err)
	}
	if !instanceUsesDefinition(stratsrv.ManagedInstance{
		Definition: stratsrv.DefinitionSummary{StrategyID: "definition"},
	}, " definition ") {
		t.Fatal("definition summary should link instance")
	}
	if instanceUsesDefinition(instance, " ") {
		t.Fatal("blank definition should not link instance")
	}

	currentOperation := &stratsrv.PluginOperation{OperationID: "current"}
	lastOperation := &stratsrv.PluginOperation{OperationID: "last"}
	maxQuantity := 5.0
	maxNotional := 500.0
	dailyOrders := 2
	account := &stratsrv.BrokerAccountBinding{BrokerID: "futu", AccountID: "1"}
	snapshot := Snapshot{
		Plugins: []ManagedPlugin{{
			Installation: stratsrv.PluginInstallation{
				CurrentOperation: currentOperation,
				LastOperation:    lastOperation,
			},
		}},
		Strategies: []stratsrv.ManagedInstance{{
			Binding: stratsrv.InstanceBinding{
				BrokerAccount: account,
				RuntimeRisk: stratsrv.RuntimeRiskSettings{
					MaxOrderQuantity: &maxQuantity,
					MaxOrderNotional: &maxNotional,
					DailyMaxOrders:   &dailyOrders,
				},
			},
		}},
	}
	cloned := cloneSnapshot(snapshot)
	if cloned.Plugins[0].Installation.CurrentOperation == currentOperation ||
		cloned.Plugins[0].Installation.LastOperation == lastOperation ||
		cloned.Strategies[0].Binding.BrokerAccount == account ||
		cloned.Strategies[0].Binding.RuntimeRisk.MaxOrderQuantity == &maxQuantity {
		t.Fatal("clone retained mutable pointers")
	}
	if cloned.Plugins[0].Installation.CurrentOperation.OperationID != "current" ||
		cloned.Plugins[0].Installation.LastOperation.OperationID != "last" {
		t.Fatalf("clone lost plugin operations = %#v", cloned.Plugins[0].Installation)
	}
}

func TestCatalogPrivateBusinessHelpersHandleEmptyAndUnknownInputs(t *testing.T) {
	if buildRuntimeLogEntry(time.Now(), " ") != "" {
		t.Fatal("blank runtime log should be omitted")
	}
	if logLevelForKind("risk_rejected", "") != "warning" ||
		logLevelForKind("custom", "worker failed") != "error" ||
		logLevelForKind("custom", "healthy") != "info" {
		t.Fatal("runtime log level classification mismatch")
	}
	if hooks := compiledHookKinds(nil); len(hooks) != 0 {
		t.Fatalf("nil program hooks = %#v", hooks)
	}
	invalid := catalogBusinessDefinition("invalid-script", "1.0.0")
	invalid.Script = "//@version=6\nindicator(\"Not executable\")"
	if _, err := buildInstanceParams(invalid, time.Now().Format(time.RFC3339Nano)); err == nil {
		t.Fatal("invalid Pine script compile error = nil")
	}
	withoutInterval := catalogBusinessDefinition("default-interval", "1.0.0")
	withoutInterval.Interval = ""
	params, err := buildInstanceParams(withoutInterval, time.Now().Format(time.RFC3339Nano))
	if err != nil || params["interval"] != "5m" {
		t.Fatalf("default interval params = %#v, %v", params, err)
	}
	if statusKind("CUSTOM") != "CUSTOM" || statusDetail("CUSTOM") != "status transition" {
		t.Fatal("custom status fallback mismatch")
	}
	if _, ok := ObservationSourceFunc(nil).GetObservation("instance"); ok {
		t.Fatal("nil observation source returned found")
	}

	host := currentPluginBuildTuple()
	if compatibility := buildPluginCompatibility(nil); compatibility.Artifact != nil {
		t.Fatalf("nil artifact compatibility = %#v", compatibility)
	}
	partial := host
	partial.BuildTags = []string{"netgo"}
	if samePluginBuildTuple(host, partial) {
		t.Fatal("different build tag counts should not match")
	}

	plugin := ManagedPlugin{
		Descriptor: stratsrv.PluginDescriptor{ID: "installed"},
		Installation: stratsrv.PluginInstallation{
			Installed: true,
		},
	}
	normalizedPlugin := (&Service{targetDir: filepath.Join("tmp", "plugins")}).normalizePlugin(plugin)
	if normalizedPlugin.Installation.Status != "INSTALLED" ||
		normalizedPlugin.Descriptor.DisplayName != "installed" ||
		normalizedPlugin.Installation.InstallPath != filepath.Join("tmp", "plugins", "installed.so") {
		t.Fatalf("normalized plugin = %#v", normalizedPlugin)
	}
}
