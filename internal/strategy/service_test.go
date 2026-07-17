package strategy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

type fakeDesignStore struct {
	definitions []Definition
	saved       Definition
}

func (s *fakeDesignStore) ListDefinitions() []Definition { return s.definitions }
func (s *fakeDesignStore) GetDefinition(id string) (Definition, bool, error) {
	return Definition{ID: id}, true, nil
}
func (s *fakeDesignStore) SaveDefinition(input Definition) (Definition, error) {
	s.saved = input
	return input, nil
}
func (s *fakeDesignStore) DeleteDefinition(id string) (Definition, error) {
	return Definition{ID: id}, nil
}

type fakeCatalogStore struct {
	closed         bool
	startableErr   error
	instanceFound  *bool
	transitionErr  error
	pluginCatalog  PluginCatalog
	pluginOp       PluginOperation
	pluginOpFound  bool
	uninstallGuide PluginUninstallGuidance
	guideFound     bool
}

func (s *fakeCatalogStore) ListInstances() []InstanceView {
	return []InstanceView{{ID: "instance-a"}}
}
func (s *fakeCatalogStore) GetInstance(id string) (ManagedInstance, bool) {
	if s.instanceFound != nil && !*s.instanceFound {
		return ManagedInstance{}, false
	}
	return ManagedInstance{ID: id}, true
}
func (s *fakeCatalogStore) ValidateStartable(instance ManagedInstance) error {
	return s.startableErr
}
func (s *fakeCatalogStore) CreateInstance(def Definition, binding InstanceBinding) (InstanceView, error) {
	return InstanceView{ID: def.ID, Binding: binding}, nil
}
func (s *fakeCatalogStore) UpdateInstance(id string, binding InstanceBinding) (InstanceView, error) {
	return InstanceView{ID: id, Binding: binding}, nil
}
func (s *fakeCatalogStore) UpdateInstanceRuntimeRisk(id string, risk RuntimeRiskSettings) (InstanceView, error) {
	return InstanceView{ID: id, Binding: InstanceBinding{RuntimeRisk: risk}}, nil
}
func (s *fakeCatalogStore) DeleteInstance(id string) (InstanceView, error) {
	return InstanceView{ID: id}, nil
}
func (s *fakeCatalogStore) TransitionInstance(id string, status string) (InstanceView, error) {
	if s.transitionErr != nil {
		return InstanceView{}, s.transitionErr
	}
	return InstanceView{ID: id, Status: status}, nil
}
func (s *fakeCatalogStore) RefreshDefinition(id string, def Definition) (InstanceView, error) {
	return InstanceView{ID: id, Definition: DefinitionSummary{StrategyID: def.ID}}, nil
}
func (s *fakeCatalogStore) RefreshInstanceDefinition(instanceID string) (InstanceView, error) {
	return InstanceView{ID: instanceID}, nil
}
func (s *fakeCatalogStore) ApplyDefinitionToLinked(def Definition) (ApplyLinkedInstancesResult, error) {
	return ApplyLinkedInstancesResult{DefinitionID: def.ID}, nil
}
func (s *fakeCatalogStore) GetLinkedInstanceIDs(definitionID string) []string {
	return []string{definitionID + "-instance"}
}
func (s *fakeCatalogStore) GetLogs(id string, query LogQuery) (LogsResult, bool) {
	return LogsResult{InstanceID: id, Page: ActivityPage{Limit: query.Limit}}, true
}
func (s *fakeCatalogStore) GetAudit(id string, query AuditQuery) (AuditResult, bool) {
	return AuditResult{InstanceID: id, Page: ActivityPage{Limit: query.Limit}}, true
}
func (s *fakeCatalogStore) ReconcileOnStartup() (int, error) { return 2, nil }
func (s *fakeCatalogStore) PluginCatalog() PluginCatalog     { return s.pluginCatalog }
func (s *fakeCatalogStore) PluginOperation(string) (PluginOperation, bool) {
	return s.pluginOp, s.pluginOpFound
}
func (s *fakeCatalogStore) PluginUninstallGuidance(string) (PluginUninstallGuidance, bool) {
	return s.uninstallGuide, s.guideFound
}
func (s *fakeCatalogStore) InstallPlugin(id string) (PluginOperation, error) {
	return PluginOperation{PluginID: id, Phase: "installed"}, nil
}
func (s *fakeCatalogStore) UninstallPlugin(id string) (PluginOperation, error) {
	return PluginOperation{PluginID: id, Phase: "uninstalled"}, nil
}
func (s *fakeCatalogStore) Close() error {
	s.closed = true
	return nil
}

type fakeRuntimeManager struct {
	started    ManagedInstance
	startCount int
	stopped    string
	startErr   error
}

func (m *fakeRuntimeManager) Start(ctx context.Context, instance ManagedInstance) error {
	m.started = instance
	m.startCount++
	return m.startErr
}
func (m *fakeRuntimeManager) Stop(instanceID string) { m.stopped = instanceID }
func (m *fakeRuntimeManager) GetObservation(id string) (RuntimeObservation, bool) {
	return RuntimeObservation{ActualStatus: id}, true
}
func (m *fakeRuntimeManager) RuntimeSummary() RuntimeSummary {
	return RuntimeSummary{Status: "active", ActiveStrategies: 1}
}
func (m *fakeRuntimeManager) ActiveInstrumentIDs() []string {
	return []string{"US.AAPL"}
}

func TestServiceDelegatesStoresAndRuntime(t *testing.T) {
	design := &fakeDesignStore{definitions: []Definition{{ID: "definition-a"}}}
	catalog := &fakeCatalogStore{}
	runtime := &fakeRuntimeManager{}
	service := NewService(design, catalog, runtime)

	if got := service.ListDefinitions(); len(got) != 1 {
		t.Fatalf("ListDefinitions() = %#v", got)
	}
	if got, ok, err := service.GetDefinition("d1"); err != nil || !ok || got.ID != "d1" {
		t.Fatalf("GetDefinition() = %#v, %v, %v", got, ok, err)
	}
	if _, err := service.SaveDefinition(Definition{ID: "draft"}); err != nil || design.saved.ID != "draft" {
		t.Fatalf("SaveDefinition() saved %#v err %v", design.saved, err)
	}
	if err := service.Start(t.Context(), ManagedInstance{ID: "instance-a"}); err != nil || runtime.started.ID != "instance-a" {
		t.Fatalf("Start() started %#v err %v", runtime.started, err)
	}
	service.Stop("instance-a")
	if runtime.stopped != "instance-a" {
		t.Fatalf("Stop() stopped %q", runtime.stopped)
	}
	if got := service.ActiveInstrumentIDs(); len(got) != 1 || got[0] != "US.AAPL" {
		t.Fatalf("ActiveInstrumentIDs() = %#v", got)
	}
	if err := service.Close(); err != nil || !catalog.closed {
		t.Fatalf("Close() closed %v err %v", catalog.closed, err)
	}
}

func TestServicePineAnalyzerOption(t *testing.T) {
	wantErr := errors.New("bad pine")
	service := NewService(&fakeDesignStore{}, &fakeCatalogStore{}, &fakeRuntimeManager{})

	if _, err := service.AnalyzePine(PineAnalyzeInput{Script: "x"}); err == nil {
		t.Fatal("AnalyzePine() error = nil, want missing analyzer")
	}

	service = NewService(&fakeDesignStore{}, &fakeCatalogStore{}, &fakeRuntimeManager{},
		WithPineAnalyzer(func(input PineAnalyzeInput) (PineAnalysisResult, error) {
			if input.Script != "x" {
				t.Fatalf("script = %q, want x", input.Script)
			}
			if input.SourceFormat != "pine-v6" {
				t.Fatalf("sourceFormat = %q, want pine-v6", input.SourceFormat)
			}
			return PineAnalysisResult{"result": "analysis"}, wantErr
		}),
	)
	if got, err := service.AnalyzePine(PineAnalyzeInput{Script: "x"}); got["result"] != "analysis" || !errors.Is(err, wantErr) {
		t.Fatalf("AnalyzePine() = %#v, %v", got, err)
	}
}

func TestServiceAnalyzePineRejectsUnsupportedSourceFormat(t *testing.T) {
	service := NewService(&fakeDesignStore{}, &fakeCatalogStore{}, &fakeRuntimeManager{},
		WithPineAnalyzer(func(input PineAnalyzeInput) (PineAnalysisResult, error) {
			t.Fatal("analyzer should not be called for unsupported source format")
			return nil, nil
		}),
	)
	if _, err := service.AnalyzePine(PineAnalyzeInput{Script: "x", SourceFormat: "legacy"}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("AnalyzePine() error = %v, want ErrBadRequest", err)
	}
}

func TestServiceStartInstanceRejectsNotStartableBeforeRuntimeStart(t *testing.T) {
	catalog := &fakeCatalogStore{startableErr: BadRequestError("strategy runtime legacy is not startable yet")}
	runtime := &fakeRuntimeManager{}
	service := NewService(&fakeDesignStore{}, catalog, runtime)

	if _, err := service.StartInstance(t.Context(), "instance-a"); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("StartInstance() error = %v, want ErrBadRequest", err)
	}
	if runtime.startCount != 0 {
		t.Fatalf("runtime start count = %d, want 0", runtime.startCount)
	}
}

func TestServiceStartInstanceMapsPineWorkerCapacityToBusyError(t *testing.T) {
	runtime := &fakeRuntimeManager{startErr: pineworker.CapacityExceededError{Workers: 10}}
	service := NewService(&fakeDesignStore{}, &fakeCatalogStore{}, runtime)

	_, err := service.StartInstance(t.Context(), "instance-a")
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("StartInstance() error = %v, want ErrBusy", err)
	}
	if !strings.Contains(err.Error(), "运行实例 Worker 最大值") {
		t.Fatalf("StartInstance() error = %q, want settings guidance", err.Error())
	}
}

func TestServiceStartInstancePreservesLookupRuntimeAndTransitionFailures(t *testing.T) {
	notFound := false
	if _, err := NewService(&fakeDesignStore{}, &fakeCatalogStore{instanceFound: &notFound}, &fakeRuntimeManager{}).StartInstance(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing StartInstance error = %v, want ErrNotFound", err)
	}

	runtimeFailure := errors.New("runtime unavailable")
	runtime := &fakeRuntimeManager{startErr: runtimeFailure}
	if _, err := NewService(&fakeDesignStore{}, &fakeCatalogStore{}, runtime).StartInstance(t.Context(), "instance-a"); !errors.Is(err, runtimeFailure) || runtime.stopped != "" {
		t.Fatalf("runtime StartInstance error=%v stopped=%q", err, runtime.stopped)
	}

	transitionFailure := errors.New("state persistence failed")
	runtime = &fakeRuntimeManager{}
	if _, err := NewService(&fakeDesignStore{}, &fakeCatalogStore{transitionErr: transitionFailure}, runtime).StartInstance(t.Context(), "instance-a"); !errors.Is(err, transitionFailure) || runtime.stopped != "instance-a" {
		t.Fatalf("transition StartInstance error=%v stopped=%q", err, runtime.stopped)
	}
}

func TestServiceStartInstanceRefreshesLiveMarketStreamAfterSuccess(t *testing.T) {
	catalog := &fakeCatalogStore{}
	runtime := &fakeRuntimeManager{}
	refreshCount := 0
	service := NewService(&fakeDesignStore{}, catalog, runtime, WithLiveMarketStreamRefresher(func(ctx context.Context) {
		refreshCount++
	}))

	got, err := service.StartInstance(t.Context(), "instance-a")
	if err != nil {
		t.Fatalf("StartInstance() error = %v", err)
	}
	if runtime.started.ID != "instance-a" {
		t.Fatalf("runtime started = %#v", runtime.started)
	}
	if refreshCount != 1 {
		t.Fatalf("refresh count = %d, want 1", refreshCount)
	}
	if got.ID != "instance-a" || got.Status != "RUNNING" {
		t.Fatalf("StartInstance() result = %#v", got)
	}
}

//nolint:funlen
func TestServiceDelegatesCatalogRuntimeAndLifecycleEntryPoints(t *testing.T) {
	catalog := &fakeCatalogStore{
		pluginCatalog: PluginCatalog{
			Plugins: []PluginCatalogItem{{Descriptor: PluginDescriptor{ID: "plugin-a"}}},
		},
		pluginOp:      PluginOperation{PluginID: "plugin-a", Phase: "running"},
		pluginOpFound: true,
		uninstallGuide: PluginUninstallGuidance{
			PluginID: "plugin-a",
			Path:     "/tmp/plugin-a",
			Exists:   true,
		},
		guideFound: true,
	}
	runtime := &fakeRuntimeManager{}
	service := NewService(&fakeDesignStore{}, catalog, runtime)

	if got, err := service.DeleteDefinition("def-1"); err != nil || got.ID != "def-1" {
		t.Fatalf("DeleteDefinition() = %#v, %v", got, err)
	}
	if got := service.ListInstances(); len(got) != 1 || got[0].ID != "instance-a" {
		t.Fatalf("ListInstances() = %#v", got)
	}
	if got, ok := service.GetInstance("instance-a"); !ok || got.ID != "instance-a" {
		t.Fatalf("GetInstance() = %#v, %v", got, ok)
	}
	if err := service.ValidateStartable(ManagedInstance{ID: "instance-a"}); err != nil {
		t.Fatalf("ValidateStartable() error = %v", err)
	}

	created, err := service.CreateInstance(Definition{ID: "def-1"}, InstanceBinding{Symbols: []string{"US.AAPL"}})
	if err != nil || created.ID != "def-1" || len(created.Binding.Symbols) != 1 {
		t.Fatalf("CreateInstance() = %#v, %v", created, err)
	}
	updated, err := service.UpdateInstance("instance-a", InstanceBinding{Interval: "1m"})
	if err != nil || updated.ID != "instance-a" || updated.Binding.Interval != "1m" {
		t.Fatalf("UpdateInstance() = %#v, %v", updated, err)
	}
	risk := RuntimeRiskSettings{Mode: "strict", CloseOnly: true}
	updatedRisk, err := service.UpdateInstanceRuntimeRisk("instance-a", risk)
	if err != nil || updatedRisk.Binding.RuntimeRisk.Mode != "strict" || !updatedRisk.Binding.RuntimeRisk.CloseOnly {
		t.Fatalf("UpdateInstanceRuntimeRisk() = %#v, %v", updatedRisk, err)
	}
	deleted, err := service.DeleteInstance("instance-a")
	if err != nil || deleted.ID != "instance-a" {
		t.Fatalf("DeleteInstance() = %#v, %v", deleted, err)
	}

	transitioned, err := service.TransitionInstance("instance-a", "PAUSED")
	if err != nil || transitioned.Status != "PAUSED" {
		t.Fatalf("TransitionInstance() = %#v, %v", transitioned, err)
	}
	refreshed, err := service.RefreshDefinition("instance-a", Definition{ID: "def-2"})
	if err != nil || refreshed.Definition.StrategyID != "def-2" {
		t.Fatalf("RefreshDefinition() = %#v, %v", refreshed, err)
	}
	applyResult, err := service.ApplyDefinitionToLinked(Definition{ID: "def-3"})
	if err != nil || applyResult.DefinitionID != "def-3" {
		t.Fatalf("ApplyDefinitionToLinked() = %#v, %v", applyResult, err)
	}
	if got := service.GetLinkedInstanceIDs("def-3"); len(got) != 1 || got[0] != "def-3-instance" {
		t.Fatalf("GetLinkedInstanceIDs() = %#v", got)
	}
	if got, err := service.RefreshInstanceDefinition("instance-a"); err != nil || got.ID != "instance-a" {
		t.Fatalf("RefreshInstanceDefinition() = %#v, %v", got, err)
	}

	if got, ok := service.GetObservation("running"); !ok || got.ActualStatus != "running" {
		t.Fatalf("GetObservation() = %#v, %v", got, ok)
	}
	if got := service.RuntimeSummary(); got.Status != "active" || got.ActiveStrategies != 1 {
		t.Fatalf("RuntimeSummary() = %#v", got)
	}

	logs, ok := service.GetLogs("instance-a", LogQuery{Limit: 20})
	if !ok || logs.InstanceID != "instance-a" || logs.Page.Limit != 20 {
		t.Fatalf("GetLogs() = %#v, %v", logs, ok)
	}
	audit, ok := service.GetAudit("instance-a", AuditQuery{Limit: 15})
	if !ok || audit.InstanceID != "instance-a" || audit.Page.Limit != 15 {
		t.Fatalf("GetAudit() = %#v, %v", audit, ok)
	}

	if got, err := service.ReconcileOnStartup(); err != nil || got != 2 {
		t.Fatalf("ReconcileOnStartup() = %d, %v", got, err)
	}
	if got := service.PluginCatalog(); len(got.Plugins) != 1 || got.Plugins[0].Descriptor.ID != "plugin-a" {
		t.Fatalf("PluginCatalog() = %#v", got)
	}
	if got, ok := service.PluginOperation("plugin-a"); !ok || got.PluginID != "plugin-a" || got.Phase != "running" {
		t.Fatalf("PluginOperation() = %#v, %v", got, ok)
	}
	if got, ok := service.PluginUninstallGuidance("plugin-a"); !ok || got.PluginID != "plugin-a" || !got.Exists {
		t.Fatalf("PluginUninstallGuidance() = %#v, %v", got, ok)
	}
	if got, err := service.InstallPlugin("plugin-a"); err != nil || got.Phase != "installed" {
		t.Fatalf("InstallPlugin() = %#v, %v", got, err)
	}
	if got, err := service.UninstallPlugin("plugin-a"); err != nil || got.Phase != "uninstalled" {
		t.Fatalf("UninstallPlugin() = %#v, %v", got, err)
	}
}

func TestServiceCloseWithoutCatalogIsSafe(t *testing.T) {
	service := &Service{}
	if err := service.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
