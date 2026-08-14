package assembly

import (
	"context"
	"errors"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

type adapterDesignStore struct {
	stratsrv.DesignStore
	definition stratsrv.Definition
	getErr     error
}

func (s adapterDesignStore) GetDefinition(id string) (stratsrv.Definition, bool, error) {
	if s.getErr != nil {
		return stratsrv.Definition{}, false, s.getErr
	}
	return s.definition, s.definition.ID != "" && s.definition.ID == id, nil
}

type adapterCatalogStore struct {
	stratsrv.CatalogStore
	instance   stratsrv.ManagedInstance
	view       stratsrv.InstanceView
	logs       stratsrv.LogsResult
	audit      stratsrv.AuditResult
	stopped    []string
	activityOK bool
}

func (s *adapterCatalogStore) GetInstance(string) (stratsrv.ManagedInstance, bool) {
	return s.instance, s.instance.ID != ""
}

func (s *adapterCatalogStore) ValidateStartable(stratsrv.ManagedInstance) error { return nil }

func (s *adapterCatalogStore) CreateInstance(_ stratsrv.Definition, _ stratsrv.InstanceBinding) (stratsrv.InstanceView, error) {
	return s.view, nil
}

func (s *adapterCatalogStore) UpdateInstanceRuntimeRisk(string, stratsrv.RuntimeRiskSettings) (stratsrv.InstanceView, error) {
	return s.view, nil
}

func (s *adapterCatalogStore) TransitionInstance(id, status string) (stratsrv.InstanceView, error) {
	s.stopped = append(s.stopped, id+":"+status)
	return s.view, nil
}

func (s *adapterCatalogStore) RefreshInstanceDefinition(string) (stratsrv.InstanceView, error) {
	return s.view, nil
}

func (s *adapterCatalogStore) GetLogs(id string, query stratsrv.LogQuery) (stratsrv.LogsResult, bool) {
	s.logs.InstanceID = id
	s.logs.Page.Limit = query.Limit
	s.logs.Page.Offset = query.Offset
	return s.logs, s.activityOK
}

func (s *adapterCatalogStore) GetAudit(id string, query stratsrv.AuditQuery) (stratsrv.AuditResult, bool) {
	s.audit.InstanceID = id
	s.audit.Page.Limit = query.Limit
	s.audit.Page.Offset = query.Offset
	return s.audit, s.activityOK
}

type adapterRuntimeManager struct {
	stratsrv.RuntimeManager
	started []string
	stopped []string
}

func (r *adapterRuntimeManager) Start(_ context.Context, instance stratsrv.ManagedInstance) error {
	r.started = append(r.started, instance.ID)
	return nil
}

func (r *adapterRuntimeManager) Stop(instanceID string) { r.stopped = append(r.stopped, instanceID) }

func TestApplicationAdapterStrategyInstanceLifecyclePorts(t *testing.T) {
	definition := stratsrv.Definition{ID: "definition-1", Name: "Fixture", Version: "1.0.0"}
	instance := stratsrv.ManagedInstance{ID: "instance-1", Definition: stratsrv.DefinitionSummary{StrategyID: definition.ID}}
	view := stratsrv.InstanceView{ID: instance.ID, Status: "STOPPED", Definition: instance.Definition}
	catalog := &adapterCatalogStore{instance: instance, view: view, activityOK: true, logs: stratsrv.LogsResult{Logs: []string{"started"}}, audit: stratsrv.AuditResult{Entries: []stratsrv.AuditEntry{{Kind: "pause"}}}}
	runtime := &adapterRuntimeManager{}
	service := stratsrv.NewService(adapterDesignStore{definition: definition}, catalog, runtime)
	deps := NewApplicationAdapter(ApplicationPorts{Strategy: func() *stratsrv.Service { return service }}).ToolDeps()

	if got, err := deps.InstantiateStrategy(" definition-1 ", stratsrv.InstanceBinding{}); err != nil || got.(stratsrv.InstanceView).ID != instance.ID {
		t.Fatalf("InstantiateStrategy = %#v, %v", got, err)
	}
	if got, err := deps.StartStrategyInstance(t.Context(), " instance-1 "); err != nil || got.(stratsrv.InstanceView).ID != instance.ID {
		t.Fatalf("StartStrategyInstance = %#v, %v", got, err)
	}
	if got, err := deps.StopStrategyInstance("instance-1", "pause"); err != nil || got.(stratsrv.InstanceView).ID != instance.ID {
		t.Fatalf("PauseStrategyInstance = %#v, %v", got, err)
	}
	if got, err := deps.StopStrategyInstance("instance-1", "stop"); err != nil || got.(stratsrv.InstanceView).ID != instance.ID {
		t.Fatalf("StopStrategyInstance = %#v, %v", got, err)
	}
	if got, err := deps.RefreshStrategyInstance("instance-1"); err != nil || got.(stratsrv.InstanceView).ID != instance.ID {
		t.Fatalf("RefreshStrategyInstance = %#v, %v", got, err)
	}
	if got, err := deps.UpdateStrategyInstanceRisk("instance-1", stratsrv.RuntimeRiskSettings{Mode: "close_only"}); err != nil || got.(stratsrv.InstanceView).ID != instance.ID {
		t.Fatalf("UpdateStrategyInstanceRisk = %#v, %v", got, err)
	}
	if got, err := deps.StrategyInstanceActivity("instance-1", "logs", 0, -1); err != nil || got.(stratsrv.LogsResult).Page.Limit != 1 || got.(stratsrv.LogsResult).Page.Offset != 0 {
		t.Fatalf("logs activity = %#v, %v", got, err)
	}
	if got, err := deps.StrategyInstanceActivity("instance-1", "audit", 10, 2); err != nil || got.(stratsrv.AuditResult).Page.Offset != 2 {
		t.Fatalf("audit activity = %#v, %v", got, err)
	}
	if len(runtime.started) != 1 || len(runtime.stopped) != 2 || len(catalog.stopped) != 3 {
		t.Fatalf("lifecycle calls started=%v runtimeStopped=%v transitions=%v", runtime.started, runtime.stopped, catalog.stopped)
	}
}

func TestApplicationAdapterStrategyInstanceLifecycleRejectsInvalidInputs(t *testing.T) {
	deps := NewApplicationAdapter(ApplicationPorts{}).ToolDeps()
	for name, call := range map[string]func() error{
		"instantiate": func() error { _, err := deps.InstantiateStrategy("", stratsrv.InstanceBinding{}); return err },
		"start":       func() error { _, err := deps.StartStrategyInstance(context.Background(), ""); return err },
		"stop":        func() error { _, err := deps.StopStrategyInstance("", "pause"); return err },
		"refresh":     func() error { _, err := deps.RefreshStrategyInstance(""); return err },
		"risk": func() error {
			_, err := deps.UpdateStrategyInstanceRisk("", stratsrv.RuntimeRiskSettings{})
			return err
		},
		"activity": func() error { _, err := deps.StrategyInstanceActivity("", "logs", 1, 0); return err },
	} {
		if err := call(); err == nil {
			t.Fatalf("%s invalid input error = nil", name)
		}
	}
}

func TestApplicationAdapterStrategyInstanceLifecycleRejectsDefinitionAndActivityBoundaries(t *testing.T) {
	definition := stratsrv.Definition{ID: "definition-1"}
	service := stratsrv.NewService(adapterDesignStore{definition: definition}, &adapterCatalogStore{}, &adapterRuntimeManager{})
	deps := NewApplicationAdapter(ApplicationPorts{Strategy: func() *stratsrv.Service { return service }}).ToolDeps()
	if _, err := deps.InstantiateStrategy("", stratsrv.InstanceBinding{}); err == nil {
		t.Fatal("blank definition id error = nil")
	}
	if _, err := deps.InstantiateStrategy("missing", stratsrv.InstanceBinding{}); err == nil {
		t.Fatal("missing definition error = nil")
	}
	if _, err := deps.StopStrategyInstance("instance-1", "restart"); err == nil {
		t.Fatal("invalid stop action error = nil")
	}
	if _, err := deps.StrategyInstanceActivity("missing", "audit", 10, 0); err == nil {
		t.Fatal("missing audit instance error = nil")
	}
	if _, err := deps.StrategyInstanceActivity("missing", "logs", 10, 0); err == nil {
		t.Fatal("missing logs instance error = nil")
	}

	wantErr := errors.New("definition lookup failed")
	service = stratsrv.NewService(adapterDesignStore{getErr: wantErr}, &adapterCatalogStore{}, &adapterRuntimeManager{})
	deps = NewApplicationAdapter(ApplicationPorts{Strategy: func() *stratsrv.Service { return service }}).ToolDeps()
	if _, err := deps.InstantiateStrategy("definition-1", stratsrv.InstanceBinding{}); !errors.Is(err, wantErr) {
		t.Fatalf("definition lookup error = %v", err)
	}
}
