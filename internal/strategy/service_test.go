package strategy

import (
	"context"
	"errors"
	"testing"
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
	closed       bool
	startableErr error
}

func (s *fakeCatalogStore) ListInstances() []InstanceView {
	return []InstanceView{{ID: "instance-a"}}
}
func (s *fakeCatalogStore) GetInstance(id string) (ManagedInstance, bool) {
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
func (s *fakeCatalogStore) DeleteInstance(id string) (InstanceView, error) {
	return InstanceView{ID: id}, nil
}
func (s *fakeCatalogStore) TransitionInstance(id string, status string) (InstanceView, error) {
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
func (s *fakeCatalogStore) PluginCatalog() PluginCatalog     { return PluginCatalog{} }
func (s *fakeCatalogStore) PluginOperation(string) (PluginOperation, bool) {
	return PluginOperation{}, false
}
func (s *fakeCatalogStore) PluginUninstallGuidance(string) (PluginUninstallGuidance, bool) {
	return PluginUninstallGuidance{}, false
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
}

func (m *fakeRuntimeManager) Start(ctx context.Context, instance ManagedInstance) error {
	m.started = instance
	m.startCount++
	return nil
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
