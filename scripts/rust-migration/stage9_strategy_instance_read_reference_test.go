package rustmigration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	strategyapi "github.com/jftrade/jftrade-main/internal/api/strategy"
	srv "github.com/jftrade/jftrade-main/internal/strategy"
)

const stage9StrategyInstanceReadFixtureVersion = "stage9.strategy-instance-read.v1"

type stage9StrategyInstanceReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9StrategyInstanceReadFixture struct {
	Version string                           `json:"version"`
	Cases   []stage9StrategyInstanceReadCase `json:"cases"`
}

// TestStage9StrategyInstanceReadFixtureMatchesCurrentGoOwner freezes the
// list and activity projections without starting a strategy or activity store.
func TestStage9StrategyInstanceReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 strategy fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/strategy-instance-read.json")
	cases := []struct {
		name string
		path string
	}{
		{name: "list", path: "/api/v1/strategies"},
		{name: "logs-default", path: "/api/v1/strategies/fixture-running/logs"},
		{name: "logs-filtered", path: "/api/v1/strategies/fixture-running/logs?limit=1&offset=1&level=%20WARN%20&fromTime=2026-08-15T20:00:00Z&toTime=2026-08-15T20:02:00Z"},
		{name: "logs-degraded", path: "/api/v1/strategies/fixture-degraded/logs"},
		{name: "audit-filtered", path: "/api/v1/strategies/fixture-running/audit?limit=2&offset=0&kind=%20execution%20"},
		{name: "logs-missing", path: "/api/v1/strategies/missing/logs"},
		{name: "audit-missing", path: "/api/v1/strategies/missing/audit"},
		{name: "logs-invalid", path: "/api/v1/strategies/fixture-running/logs?limit=bad"},
		{name: "audit-invalid", path: "/api/v1/strategies/fixture-running/audit?toTime=not-a-time"},
	}
	want := stage9StrategyInstanceReadFixture{
		Version: stage9StrategyInstanceReadFixtureVersion,
		Cases:   make([]stage9StrategyInstanceReadCase, 0, len(cases)),
	}
	for _, testCase := range cases {
		gin.SetMode(gin.TestMode)
		catalog := &stage9StrategyCatalogStore{}
		router := gin.New()
		strategyapi.RegisterRoutes(router.Group("/api/v1"), srv.NewService(
			&stage9StrategyDesignStore{}, catalog, &stage9StrategyRuntimeManager{},
		))
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		entry := stage9StrategyInstanceReadCase{
			Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path,
			ExpectedStatus: recorder.Code,
		}
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v", testCase.name, err)
		}
		if envelope.Error != nil {
			entry.ErrorCode, entry.ErrorMessage = envelope.Error.Code, envelope.Error.Message
		} else {
			entry.Data = compactStrategyReadJSON(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode strategy fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write strategy fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read strategy fixture: %v", err)
	}
	var got stage9StrategyInstanceReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode strategy fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactStrategyReadJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactStrategyReadJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 strategy instance read fixture drifted from the Go owner: got=%#v want=%#v", got, want)
	}
}

func compactStrategyReadJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

type stage9StrategyDesignStore struct{}

func (stage9StrategyDesignStore) ListDefinitions() ([]srv.Definition, error) { return nil, nil }
func (stage9StrategyDesignStore) GetDefinition(string) (srv.Definition, bool, error) {
	return srv.Definition{}, false, nil
}
func (stage9StrategyDesignStore) SaveDefinition(srv.Definition) (srv.Definition, error) {
	return srv.Definition{}, nil
}
func (stage9StrategyDesignStore) DeleteDefinition(string) (srv.Definition, error) {
	return srv.Definition{}, nil
}
func (stage9StrategyDesignStore) ListDefinitionVersions(string) ([]srv.DefinitionVersionSummary, bool, error) {
	return nil, false, nil
}
func (stage9StrategyDesignStore) GetDefinitionVersion(string, string) (srv.DefinitionVersion, bool, error) {
	return srv.DefinitionVersion{}, false, nil
}

type stage9StrategyRuntimeManager struct{}

func (stage9StrategyRuntimeManager) Start(context.Context, srv.ManagedInstance) error { return nil }
func (stage9StrategyRuntimeManager) Stop(string)                                      {}
func (stage9StrategyRuntimeManager) GetObservation(string) (srv.RuntimeObservation, bool) {
	return srv.RuntimeObservation{}, false
}
func (stage9StrategyRuntimeManager) RuntimeSummary() srv.RuntimeSummary { return srv.RuntimeSummary{} }
func (stage9StrategyRuntimeManager) ActiveInstrumentIDs() []string      { return nil }

type stage9StrategyCatalogStore struct{}

func (stage9StrategyCatalogStore) ListInstances() []srv.InstanceView {
	var instances []srv.InstanceView
	if err := json.Unmarshal([]byte(stage9StrategyInstancesJSON), &instances); err != nil {
		panic(err)
	}
	return instances
}
func (stage9StrategyCatalogStore) GetInstance(id string) (srv.ManagedInstance, bool) {
	if id == "fixture-running" || id == "fixture-degraded" {
		return srv.ManagedInstance{ID: id}, true
	}
	return srv.ManagedInstance{}, false
}
func (stage9StrategyCatalogStore) ValidateStartable(srv.ManagedInstance) error { return nil }
func (stage9StrategyCatalogStore) CreateInstance(srv.Definition, srv.InstanceBinding) (srv.InstanceView, error) {
	return srv.InstanceView{}, nil
}
func (stage9StrategyCatalogStore) UpdateInstance(string, srv.InstanceBinding) (srv.InstanceView, error) {
	return srv.InstanceView{}, nil
}
func (stage9StrategyCatalogStore) UpdateInstanceRuntimeRisk(string, srv.RuntimeRiskSettings) (srv.InstanceView, error) {
	return srv.InstanceView{}, nil
}
func (stage9StrategyCatalogStore) DeleteInstance(string) (srv.InstanceView, error) {
	return srv.InstanceView{}, nil
}
func (stage9StrategyCatalogStore) TransitionInstance(string, string) (srv.InstanceView, error) {
	return srv.InstanceView{}, nil
}
func (stage9StrategyCatalogStore) RefreshDefinition(string, srv.Definition) (srv.InstanceView, error) {
	return srv.InstanceView{}, nil
}
func (stage9StrategyCatalogStore) RefreshInstanceDefinition(string) (srv.InstanceView, error) {
	return srv.InstanceView{}, nil
}
func (stage9StrategyCatalogStore) ApplyDefinitionToLinked(srv.Definition) (srv.ApplyLinkedInstancesResult, error) {
	return srv.ApplyLinkedInstancesResult{}, nil
}
func (stage9StrategyCatalogStore) GetLinkedInstanceIDs(string) []string { return nil }
func (stage9StrategyCatalogStore) GetLogs(id string, query srv.LogQuery) (srv.LogsResult, bool) {
	if id != "fixture-running" && id != "fixture-degraded" {
		return srv.LogsResult{}, false
	}
	if id == "fixture-degraded" {
		return srv.LogsResult{InstanceID: id, Logs: []string{}, Page: srv.ActivityPage{Limit: query.Limit, Offset: query.Offset}}, true
	}
	if query.Limit == 1 {
		return srv.LogsResult{
			InstanceID: id,
			Logs:       []string{"2026-08-15T20:01:00Z [WARN] delayed tick"},
			Page:       srv.ActivityPage{Limit: 1, Offset: 1, Total: 3, Returned: 1, HasMore: true},
		}, true
	}
	return srv.LogsResult{
		InstanceID: id,
		Logs:       []string{"2026-08-15T20:01:00Z [WARN] delayed tick", "2026-08-15T20:00:00Z [INFO] started"},
		Page:       srv.ActivityPage{Limit: query.Limit, Offset: query.Offset, Total: 2, Returned: 2},
	}, true
}
func (stage9StrategyCatalogStore) GetAudit(id string, query srv.AuditQuery) (srv.AuditResult, bool) {
	if id != "fixture-running" {
		return srv.AuditResult{}, false
	}
	return srv.AuditResult{
		InstanceID: id,
		Entries:    []srv.AuditEntry{{InstanceID: id, Kind: "execution", Detail: "submitted order", At: "2026-08-15T20:01:00Z"}},
		Page:       srv.ActivityPage{Limit: query.Limit, Offset: query.Offset, Total: 1, Returned: 1},
	}, true
}
func (stage9StrategyCatalogStore) ReconcileOnStartup() (int, error) { return 0, nil }
func (stage9StrategyCatalogStore) PluginCatalog() srv.PluginCatalog { return srv.PluginCatalog{} }
func (stage9StrategyCatalogStore) PluginOperation(string) (srv.PluginOperation, bool) {
	return srv.PluginOperation{}, false
}
func (stage9StrategyCatalogStore) PluginUninstallGuidance(string) (srv.PluginUninstallGuidance, bool) {
	return srv.PluginUninstallGuidance{}, false
}
func (stage9StrategyCatalogStore) InstallPlugin(string) (srv.PluginOperation, error) {
	return srv.PluginOperation{}, nil
}
func (stage9StrategyCatalogStore) UninstallPlugin(string) (srv.PluginOperation, error) {
	return srv.PluginOperation{}, nil
}

const stage9StrategyInstancesJSON = `[
  {
    "id": "fixture-old",
    "definition": {"strategyId": "fixture-strategy", "name": "Fixture Strategy", "version": "1.1.0"},
    "runtime": "pinets",
    "sourceFormat": "pine-v6",
    "startable": false,
    "binding": {"symbols": ["HK.00700"], "interval": "5m", "chartType": "standard", "executionMode": "paper", "runtimeRisk": {"mode": "observe", "closeOnly": false, "pauseOnReject": false}},
    "params": {"symbol": "HK.00700"},
    "status": "STOPPED",
    "createdAt": "2026-08-15T19:00:00Z",
    "logs": [],
    "definitionSync": {"definitionId": "fixture-strategy", "appliedVersion": "1.1.0", "latestVersion": "1.1.0", "isLatest": true, "canApplyLatest": false}
  },
  {
    "id": "fixture-running",
    "pluginId": "fixture-plugin",
    "definition": {"strategyId": "fixture-strategy", "name": "Fixture Strategy", "version": "1.2.0"},
    "runtime": "pinets",
    "sourceFormat": "pine-v6",
    "startable": true,
    "binding": {"symbols": ["US.AAPL"], "interval": "1d", "chartType": "standard", "executionMode": "paper", "runtimeRisk": {"mode": "observe", "closeOnly": false, "pauseOnReject": true}},
    "params": {"symbol": "US.AAPL", "warmupBars": 50},
    "status": "RUNNING",
    "createdAt": "2026-08-15T20:00:00Z",
    "logs": ["runtime started"],
    "definitionSync": {"definitionId": "fixture-strategy", "appliedVersion": "1.2.0", "latestVersion": "1.3.0", "isLatest": false, "canApplyLatest": false, "blockedReason": "当前实例不是 STOPPED，先停止后才能刷新到最新策略。"},
    "runtimeObservation": {"actualStatus": "RUNNING", "activeSymbols": ["US.AAPL"], "lastSignalAt": "2026-08-15T20:01:00Z", "updatedAt": "2026-08-15T20:01:00Z"}
  }
]`
