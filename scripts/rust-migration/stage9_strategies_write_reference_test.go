package rustmigration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	strategyapi "github.com/jftrade/jftrade-main/internal/api/strategy"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/pkg/chart"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

const (
	stage9StrategiesWriteFixtureVersion = "stage9.strategies-write.v1"
	stage9StrategiesWriteTimestamp      = "2026-08-23T06:00:00Z"
)

type stage9StrategiesWriteFixture struct {
	Version string                      `json:"version"`
	Cases   []stage9StrategiesWriteCase `json:"cases"`
}

type stage9StrategiesWriteCase struct {
	Name                string            `json:"name"`
	Method              string            `json:"method"`
	RequestPaths        []string          `json:"requestPaths"`
	RequestBodies       []string          `json:"requestBodies"`
	ContextError        string            `json:"contextError,omitempty"`
	ExpectedStatuses    []int             `json:"expectedStatuses"`
	PortCalls           []bool            `json:"portCalls"`
	Responses           []json.RawMessage `json:"responses"`
	ExpectedObservation map[string]any    `json:"expectedObservation"`
}

type stage9StrategiesWriteCaseSpec struct {
	Name         string
	Method       string
	Paths        []string
	Bodies       []string
	ContextError string
	Setup        func(*testing.T) *stage9StrategiesWriteHarness
}

type stage9StrategiesWriteHarness struct {
	router  *gin.Engine
	catalog *stage9StrategiesWriteCatalog
	runtime *stage9StrategiesWriteRuntime
}

type stage9StrategiesWriteCatalog struct {
	*stage9StrategiesWriteNoopCatalog
	updateResult      stratsrv.InstanceView
	updateErr         error
	updateInstanceID  string
	updateBinding     stratsrv.InstanceBinding
	riskResult        stratsrv.InstanceView
	riskErr           error
	riskInstanceID    string
	risk              stratsrv.RuntimeRiskSettings
	deleteResult      stratsrv.InstanceView
	deleteErr         error
	deleteInstanceID  string
	transitionResult  stratsrv.InstanceView
	transitionErr     error
	transitionCalls   []stage9TransitionCall
	refreshResult     stratsrv.InstanceView
	refreshErr        error
	refreshInstanceID string
	instance          stratsrv.ManagedInstance
	instanceFound     bool
	startableErr      error
}

type stage9TransitionCall struct {
	InstanceID string `json:"instanceId"`
	Status     string `json:"status"`
}

type stage9StrategiesWriteRuntime struct {
	startErr     error
	honorContext bool
	startCalls   []string
	stopCalls    []string
}

// The route harness only exercises mutation handlers. The no-op implementation
// keeps unrelated strategy routes inert without opening a store or runtime.
type stage9StrategiesWriteNoopCatalog struct{}

func (*stage9StrategiesWriteNoopCatalog) ListInstances() []stratsrv.InstanceView {
	return []stratsrv.InstanceView{}
}
func (*stage9StrategiesWriteNoopCatalog) GetInstance(string) (stratsrv.ManagedInstance, bool) {
	return stratsrv.ManagedInstance{}, false
}
func (*stage9StrategiesWriteNoopCatalog) ValidateStartable(stratsrv.ManagedInstance) error {
	return nil
}
func (*stage9StrategiesWriteNoopCatalog) CreateInstance(stratsrv.Definition, stratsrv.InstanceBinding) (stratsrv.InstanceView, error) {
	return stratsrv.InstanceView{}, nil
}
func (*stage9StrategiesWriteNoopCatalog) UpdateInstance(string, stratsrv.InstanceBinding) (stratsrv.InstanceView, error) {
	return stratsrv.InstanceView{}, nil
}
func (*stage9StrategiesWriteNoopCatalog) UpdateInstanceRuntimeRisk(string, stratsrv.RuntimeRiskSettings) (stratsrv.InstanceView, error) {
	return stratsrv.InstanceView{}, nil
}
func (*stage9StrategiesWriteNoopCatalog) DeleteInstance(string) (stratsrv.InstanceView, error) {
	return stratsrv.InstanceView{}, nil
}
func (*stage9StrategiesWriteNoopCatalog) TransitionInstance(string, string) (stratsrv.InstanceView, error) {
	return stratsrv.InstanceView{}, nil
}
func (*stage9StrategiesWriteNoopCatalog) RefreshDefinition(string, stratsrv.Definition) (stratsrv.InstanceView, error) {
	return stratsrv.InstanceView{}, nil
}
func (*stage9StrategiesWriteNoopCatalog) RefreshInstanceDefinition(string) (stratsrv.InstanceView, error) {
	return stratsrv.InstanceView{}, nil
}
func (*stage9StrategiesWriteNoopCatalog) ApplyDefinitionToLinked(stratsrv.Definition) (stratsrv.ApplyLinkedInstancesResult, error) {
	return stratsrv.ApplyLinkedInstancesResult{}, nil
}
func (*stage9StrategiesWriteNoopCatalog) GetLinkedInstanceIDs(string) []string { return nil }
func (*stage9StrategiesWriteNoopCatalog) GetLogs(string, stratsrv.LogQuery) (stratsrv.LogsResult, bool) {
	return stratsrv.LogsResult{}, false
}
func (*stage9StrategiesWriteNoopCatalog) GetAudit(string, stratsrv.AuditQuery) (stratsrv.AuditResult, bool) {
	return stratsrv.AuditResult{}, false
}
func (*stage9StrategiesWriteNoopCatalog) ReconcileOnStartup() (int, error) { return 0, nil }
func (*stage9StrategiesWriteNoopCatalog) PluginCatalog() stratsrv.PluginCatalog {
	return stratsrv.PluginCatalog{}
}
func (*stage9StrategiesWriteNoopCatalog) PluginOperation(string) (stratsrv.PluginOperation, bool) {
	return stratsrv.PluginOperation{}, false
}
func (*stage9StrategiesWriteNoopCatalog) PluginUninstallGuidance(string) (stratsrv.PluginUninstallGuidance, bool) {
	return stratsrv.PluginUninstallGuidance{}, false
}
func (*stage9StrategiesWriteNoopCatalog) InstallPlugin(string) (stratsrv.PluginOperation, error) {
	return stratsrv.PluginOperation{}, nil
}
func (*stage9StrategiesWriteNoopCatalog) UninstallPlugin(string) (stratsrv.PluginOperation, error) {
	return stratsrv.PluginOperation{}, nil
}

type stage9StrategiesWriteDesignStore struct{}

func (*stage9StrategiesWriteDesignStore) ListDefinitions() ([]stratsrv.Definition, error) {
	return []stratsrv.Definition{}, nil
}
func (*stage9StrategiesWriteDesignStore) GetDefinition(string) (stratsrv.Definition, bool, error) {
	return stratsrv.Definition{}, false, nil
}
func (*stage9StrategiesWriteDesignStore) SaveDefinition(stratsrv.Definition) (stratsrv.Definition, error) {
	return stratsrv.Definition{}, nil
}
func (*stage9StrategiesWriteDesignStore) DeleteDefinition(string) (stratsrv.Definition, error) {
	return stratsrv.Definition{}, nil
}
func (*stage9StrategiesWriteDesignStore) ListDefinitionVersions(string) ([]stratsrv.DefinitionVersionSummary, bool, error) {
	return nil, false, nil
}
func (*stage9StrategiesWriteDesignStore) GetDefinitionVersion(string, string) (stratsrv.DefinitionVersion, bool, error) {
	return stratsrv.DefinitionVersion{}, false, nil
}

func (s *stage9StrategiesWriteCatalog) GetInstance(string) (stratsrv.ManagedInstance, bool) {
	return s.instance, s.instanceFound
}
func (s *stage9StrategiesWriteCatalog) ValidateStartable(stratsrv.ManagedInstance) error {
	return s.startableErr
}
func (s *stage9StrategiesWriteCatalog) UpdateInstance(id string, binding stratsrv.InstanceBinding) (stratsrv.InstanceView, error) {
	s.updateInstanceID, s.updateBinding = id, binding
	return s.updateResult, s.updateErr
}
func (s *stage9StrategiesWriteCatalog) UpdateInstanceRuntimeRisk(id string, risk stratsrv.RuntimeRiskSettings) (stratsrv.InstanceView, error) {
	s.riskInstanceID, s.risk = id, risk
	return s.riskResult, s.riskErr
}
func (s *stage9StrategiesWriteCatalog) DeleteInstance(id string) (stratsrv.InstanceView, error) {
	s.deleteInstanceID = id
	return s.deleteResult, s.deleteErr
}
func (s *stage9StrategiesWriteCatalog) TransitionInstance(id, status string) (stratsrv.InstanceView, error) {
	s.transitionCalls = append(s.transitionCalls, stage9TransitionCall{InstanceID: id, Status: status})
	if s.transitionErr != nil {
		return stratsrv.InstanceView{}, s.transitionErr
	}
	result := s.transitionResult
	result.Status = status
	return result, nil
}
func (s *stage9StrategiesWriteCatalog) RefreshInstanceDefinition(id string) (stratsrv.InstanceView, error) {
	s.refreshInstanceID = id
	return s.refreshResult, s.refreshErr
}

func (r *stage9StrategiesWriteRuntime) Start(ctx context.Context, instance stratsrv.ManagedInstance) error {
	r.startCalls = append(r.startCalls, instance.ID)
	if r.honorContext && ctx.Err() != nil {
		return ctx.Err()
	}
	return r.startErr
}
func (r *stage9StrategiesWriteRuntime) Stop(instanceID string) {
	r.stopCalls = append(r.stopCalls, instanceID)
}
func (*stage9StrategiesWriteRuntime) GetObservation(string) (stratsrv.RuntimeObservation, bool) {
	return stratsrv.RuntimeObservation{}, false
}
func (*stage9StrategiesWriteRuntime) RuntimeSummary() stratsrv.RuntimeSummary {
	return stratsrv.RuntimeSummary{}
}
func (*stage9StrategiesWriteRuntime) ActiveInstrumentIDs() []string { return nil }

func newStage9StrategiesWriteHarness(
	catalog *stage9StrategiesWriteCatalog,
	runtime *stage9StrategiesWriteRuntime,
) *stage9StrategiesWriteHarness {
	router := gin.New()
	strategyapi.RegisterRoutes(
		router.Group("/api/v1"),
		stratsrv.NewService(&stage9StrategiesWriteDesignStore{}, catalog, runtime),
	)
	return &stage9StrategiesWriteHarness{router: router, catalog: catalog, runtime: runtime}
}

func stage9StrategiesWriteView(id, status string) stratsrv.InstanceView {
	return stratsrv.InstanceView{
		ID: id,
		Definition: stratsrv.DefinitionSummary{
			StrategyID: "fixture-strategy",
			Name:       "Fixture Strategy",
			Version:    "1.2.0",
		},
		Runtime:      stratsrv.RuntimePinePlan,
		SourceFormat: "pine-v6",
		Startable:    status == "STOPPED",
		Binding: stratsrv.InstanceBinding{
			Symbols:       []string{"US.AAPL"},
			Interval:      "5m",
			ChartType:     chart.ChartTypeStandard,
			ExecutionMode: "paper",
			RuntimeRisk: stratsrv.RuntimeRiskSettings{
				Mode:          "observe",
				PauseOnReject: true,
			},
		},
		Params:    map[string]any{"symbol": "US.AAPL"},
		Status:    status,
		CreatedAt: "2026-08-22T05:00:00Z",
		Logs:      []string{"fixture runtime"},
	}
}

func stage9StrategiesWriteBase(t *testing.T) *stage9StrategiesWriteHarness {
	t.Helper()
	view := stage9StrategiesWriteView("fixture-instance", "STOPPED")
	catalog := &stage9StrategiesWriteCatalog{
		stage9StrategiesWriteNoopCatalog: &stage9StrategiesWriteNoopCatalog{},
		updateResult:                     view,
		riskResult:                       view,
		deleteResult:                     view,
		transitionResult:                 view,
		refreshResult:                    view,
	}
	return newStage9StrategiesWriteHarness(catalog, &stage9StrategiesWriteRuntime{})
}

func stage9StrategiesWriteStartable(t *testing.T) *stage9StrategiesWriteHarness {
	harness := stage9StrategiesWriteBase(t)
	harness.catalog.instance = stratsrv.ManagedInstance{ID: "fixture-instance", Status: "STOPPED"}
	harness.catalog.instanceFound = true
	return harness
}

func stage9StrategiesWriteRequest(
	t *testing.T,
	harness *stage9StrategiesWriteHarness,
	method, path, body, contextError string,
) (int, map[string]any) {
	t.Helper()
	ctx := context.Background()
	var cancel context.CancelFunc
	switch contextError {
	case "canceled":
		ctx, cancel = context.WithCancel(ctx)
		cancel()
	case "deadline":
		ctx, cancel = context.WithDeadline(ctx, time.Unix(0, 0))
		cancel()
	}
	request := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, request)
	var envelope map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s %s response: %v (%s)", method, path, err, recorder.Body.String())
	}
	return recorder.Code, envelope
}

func stage9RunStrategiesWriteCase(t *testing.T, spec stage9StrategiesWriteCaseSpec) stage9StrategiesWriteCase {
	t.Helper()
	harness := spec.Setup(t)
	if len(spec.Paths) != len(spec.Bodies) {
		t.Fatalf("case %s paths/bodies length mismatch", spec.Name)
	}
	statuses := make([]int, len(spec.Paths))
	portCalls := make([]bool, len(spec.Paths))
	responses := make([]json.RawMessage, len(spec.Paths))
	for index := range spec.Paths {
		status, envelope := stage9StrategiesWriteRequest(
			t, harness, spec.Method, spec.Paths[index], spec.Bodies[index], spec.ContextError,
		)
		stage9NormalizeStrategiesWriteValue(envelope)
		encoded, err := json.Marshal(envelope)
		if err != nil {
			t.Fatalf("encode case %s response: %v", spec.Name, err)
		}
		statuses[index] = status
		portCalls[index] = stage9StrategiesWritePortCall(spec.Method, spec.Paths[index], spec.Bodies[index])
		responses[index] = encoded
	}
	return stage9StrategiesWriteCase{
		Name:                spec.Name,
		Method:              spec.Method,
		RequestPaths:        append([]string(nil), spec.Paths...),
		RequestBodies:       append([]string(nil), spec.Bodies...),
		ContextError:        spec.ContextError,
		ExpectedStatuses:    statuses,
		PortCalls:           portCalls,
		Responses:           responses,
		ExpectedObservation: harness.observation(),
	}
}

func stage9StrategiesWritePortCall(method, path, body string) bool {
	if !strings.HasPrefix(path, "/api/v1/strategies/") {
		return false
	}
	if method != http.MethodPut {
		return true
	}
	if body == "" {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(body))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	if value == nil {
		return true
	}
	_, ok := value.(map[string]any)
	return ok
}

func stage9NormalizeStrategiesWriteValue(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "timestamp" {
				current[key] = stage9StrategiesWriteTimestamp
				continue
			}
			stage9NormalizeStrategiesWriteValue(child)
		}
	case []any:
		for _, child := range current {
			stage9NormalizeStrategiesWriteValue(child)
		}
	}
}

func (h *stage9StrategiesWriteHarness) observation() map[string]any {
	updateCalls := make([]map[string]any, 0)
	if h.catalog.updateInstanceID != "" {
		updateCalls = append(updateCalls, map[string]any{
			"instanceId": h.catalog.updateInstanceID,
			"binding":    stage9JSONValue(h.catalog.updateBinding),
		})
	}
	riskCalls := make([]map[string]any, 0)
	if h.catalog.riskInstanceID != "" {
		riskCalls = append(riskCalls, map[string]any{
			"instanceId": h.catalog.riskInstanceID,
			"risk":       stage9JSONValue(h.catalog.risk),
		})
	}
	deleteCalls := make([]string, 0)
	if h.catalog.deleteInstanceID != "" {
		deleteCalls = append(deleteCalls, h.catalog.deleteInstanceID)
	}
	refreshCalls := make([]string, 0)
	if h.catalog.refreshInstanceID != "" {
		refreshCalls = append(refreshCalls, h.catalog.refreshInstanceID)
	}
	return map[string]any{
		"updateCalls":      updateCalls,
		"runtimeRiskCalls": riskCalls,
		"deleteCalls":      deleteCalls,
		"transitionCalls":  append([]stage9TransitionCall{}, h.catalog.transitionCalls...),
		"refreshCalls":     refreshCalls,
		"startCalls":       append([]string{}, h.runtime.startCalls...),
		"stopCalls":        append([]string{}, h.runtime.stopCalls...),
	}
}

func stage9JSONValue(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		panic(err)
	}
	return decoded
}

func stage9StrategiesWriteCaseSpecs() []stage9StrategiesWriteCaseSpec {
	view := func(status string) stratsrv.InstanceView {
		return stage9StrategiesWriteView("fixture-instance", status)
	}
	base := func(t *testing.T) *stage9StrategiesWriteHarness { return stage9StrategiesWriteBase(t) }
	return []stage9StrategiesWriteCaseSpec{
		{
			Name:   "update-success-unknown-field-ignored",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/fixture-instance"},
			Bodies: []string{`{"symbols":["US.AAPL"],"interval":" 1m ","chartType":"heikinashi","executionMode":"paper","runtimeRisk":{"mode":"close_only","closeOnly":true,"maxOrderQuantity":1.5,"dailyMaxOrders":2,"pauseOnReject":true},"unknownField":"ignored"}`},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.updateResult = view("STOPPED")
				return harness
			},
		},
		{
			Name:   "update-null-body-zero-value",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/fixture-instance"},
			Bodies: []string{"null"},
			Setup:  base,
		},
		{
			Name:   "update-trailing-json-first-value-wins",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/fixture-instance"},
			Bodies: []string{`{"symbols":["US.AAPL"]}{"ignored":true}`},
			Setup:  base,
		},
		{
			Name:   "update-empty-body-rejected-before-port",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/fixture-instance"},
			Bodies: []string{""},
			Setup:  base,
		},
		{
			Name:   "update-malformed-body-rejected-before-port",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/fixture-instance"},
			Bodies: []string{"{"},
			Setup:  base,
		},
		{
			Name:   "update-not-found",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/missing"},
			Bodies: []string{`{"symbols":["US.AAPL"]}`},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.updateErr = stratsrv.NotFoundError("strategy instance not found")
				return harness
			},
		},
		{
			Name:   "update-busy",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/fixture-instance"},
			Bodies: []string{`{"symbols":["US.AAPL"]}`},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.updateErr = stratsrv.BusyError("strategy instance must be stopped before modification")
				return harness
			},
		},
		{
			Name:   "update-store-failure",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/fixture-instance"},
			Bodies: []string{`{"symbols":["US.AAPL"]}`},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.updateErr = errors.New("catalog repository unavailable")
				return harness
			},
		},
		{
			Name:   "runtime-risk-success-unknown-field-ignored",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/fixture-instance/runtime-risk"},
			Bodies: []string{`{"mode":"close_only","closeOnly":true,"maxOrderNotional":1000.25,"dailyMaxOrders":5,"pauseOnReject":true,"unknownField":"ignored"}`},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.riskResult = view("RUNNING")
				return harness
			},
		},
		{
			Name:   "runtime-risk-null-body-zero-value",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/fixture-instance/runtime-risk"},
			Bodies: []string{"null"},
			Setup:  base,
		},
		{
			Name:   "runtime-risk-trailing-json-first-value-wins",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/fixture-instance/runtime-risk"},
			Bodies: []string{`{"mode":"observe"}{"ignored":true}`},
			Setup:  base,
		},
		{
			Name:   "runtime-risk-malformed-body-rejected-before-port",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/fixture-instance/runtime-risk"},
			Bodies: []string{"{"},
			Setup:  base,
		},
		{
			Name:   "runtime-risk-not-found",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/missing/runtime-risk"},
			Bodies: []string{`{"mode":"observe"}`},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.riskErr = stratsrv.NotFoundError("strategy instance not found")
				return harness
			},
		},
		{
			Name:   "runtime-risk-store-failure",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategies/fixture-instance/runtime-risk"},
			Bodies: []string{`{"mode":"observe"}`},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.riskErr = errors.New("risk persistence unavailable")
				return harness
			},
		},
		{
			Name:   "pause-success",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/fixture-instance/pause"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.transitionResult = view("PAUSED")
				return harness
			},
		},
		{
			Name:   "pause-repeat-replays-side-effects",
			Method: http.MethodPost,
			Paths: []string{
				"/api/v1/strategies/fixture-instance/pause",
				"/api/v1/strategies/fixture-instance/pause",
			},
			Bodies: []string{"", ""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.transitionResult = view("PAUSED")
				return harness
			},
		},
		{
			Name:   "pause-not-found",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/missing/pause"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.transitionErr = stratsrv.NotFoundError("strategy instance not found")
				return harness
			},
		},
		{
			Name:   "pause-upstream-uses-start-failure-code",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/fixture-instance/pause"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.transitionErr = stratsrv.UpstreamError("runtime cleanup failed")
				return harness
			},
		},
		{
			Name:   "stop-success",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/fixture-instance/stop"},
			Bodies: []string{"ignored body is not bound"},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.transitionResult = view("STOPPED")
				return harness
			},
		},
		{
			Name:   "stop-busy",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/fixture-instance/stop"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.transitionErr = stratsrv.BusyError("strategy transition is busy")
				return harness
			},
		},
		{
			Name:   "start-success",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/fixture-instance/start"},
			Bodies: []string{"malformed body is ignored by start"},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteStartable(t)
				harness.catalog.transitionResult = view("RUNNING")
				return harness
			},
		},
		{
			Name:   "start-missing",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/missing/start"},
			Bodies: []string{""},
			Setup:  base,
		},
		{
			Name:   "start-not-startable",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/fixture-instance/start"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteStartable(t)
				harness.catalog.startableErr = stratsrv.BusyError("strategy instance is not startable")
				return harness
			},
		},
		{
			Name:   "start-runtime-capacity-maps-busy",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/fixture-instance/start"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteStartable(t)
				harness.runtime.startErr = pineworker.CapacityExceededError{Workers: 3}
				return harness
			},
		},
		{
			Name:         "start-context-cancelled-maps-gateway",
			Method:       http.MethodPost,
			Paths:        []string{"/api/v1/strategies/fixture-instance/start"},
			Bodies:       []string{""},
			ContextError: "canceled",
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteStartable(t)
				harness.runtime.honorContext = true
				return harness
			},
		},
		{
			Name:         "start-timeout-maps-gateway",
			Method:       http.MethodPost,
			Paths:        []string{"/api/v1/strategies/fixture-instance/start"},
			Bodies:       []string{""},
			ContextError: "deadline",
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteStartable(t)
				harness.runtime.honorContext = true
				return harness
			},
		},
		{
			Name:   "start-transition-failure-stops-runtime",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/fixture-instance/start"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteStartable(t)
				harness.catalog.transitionErr = errors.New("catalog transition unavailable")
				return harness
			},
		},
		{
			Name:   "start-transition-busy-stops-runtime",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/fixture-instance/start"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteStartable(t)
				harness.catalog.transitionErr = stratsrv.BusyError("strategy transition is busy")
				return harness
			},
		},
		{
			Name:   "refresh-definition-success",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/fixture-instance/refresh-definition"},
			Bodies: []string{"malformed body is ignored by refresh"},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.refreshResult = view("STOPPED")
				return harness
			},
		},
		{
			Name:   "refresh-definition-not-found",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/missing/refresh-definition"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.refreshErr = stratsrv.NotFoundError("strategy definition not found")
				return harness
			},
		},
		{
			Name:   "refresh-definition-failure",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategies/fixture-instance/refresh-definition"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.refreshErr = errors.New("definition refresh unavailable")
				return harness
			},
		},
		{
			Name:   "delete-success",
			Method: http.MethodDelete,
			Paths:  []string{"/api/v1/strategies/fixture-instance"},
			Bodies: []string{"ignored body is not bound"},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.deleteResult = view("STOPPED")
				return harness
			},
		},
		{
			Name:   "delete-not-found",
			Method: http.MethodDelete,
			Paths:  []string{"/api/v1/strategies/missing"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.deleteErr = stratsrv.NotFoundError("strategy instance not found")
				return harness
			},
		},
		{
			Name:   "delete-busy",
			Method: http.MethodDelete,
			Paths:  []string{"/api/v1/strategies/fixture-instance"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.deleteErr = stratsrv.BusyError("strategy instance is running")
				return harness
			},
		},
		{
			Name:   "delete-store-failure",
			Method: http.MethodDelete,
			Paths:  []string{"/api/v1/strategies/fixture-instance"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategiesWriteHarness {
				harness := stage9StrategiesWriteBase(t)
				harness.catalog.deleteErr = errors.New("catalog repository unavailable")
				return harness
			},
		},
	}
}

func TestStage9StrategiesWriteFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve strategies-write fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/strategies-write.json",
	)
	want := stage9StrategiesWriteFixture{
		Version: stage9StrategiesWriteFixtureVersion,
		Cases:   make([]stage9StrategiesWriteCase, 0),
	}
	for _, spec := range stage9StrategiesWriteCaseSpecs() {
		want.Cases = append(want.Cases, stage9RunStrategiesWriteCase(t, spec))
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode strategies-write fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write strategies-write fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read strategies-write fixture: %v", err)
	}
	var got stage9StrategiesWriteFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode strategies-write fixture: %v", err)
	}
	compactStage9StrategiesWriteFixture(&got)
	compactStage9StrategiesWriteFixture(&want)
	wantBytes, _ := json.Marshal(want)
	gotBytes, _ := json.Marshal(got)
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Fatalf("strategies-write fixture drifted: want=%s got=%s", wantBytes, gotBytes)
	}
}

func compactStage9StrategiesWriteFixture(fixture *stage9StrategiesWriteFixture) {
	for caseIndex := range fixture.Cases {
		for responseIndex, response := range fixture.Cases[caseIndex].Responses {
			var compacted bytes.Buffer
			if err := json.Compact(&compacted, response); err == nil {
				fixture.Cases[caseIndex].Responses[responseIndex] = append(
					json.RawMessage(nil), compacted.Bytes()...,
				)
			}
		}
	}
}
