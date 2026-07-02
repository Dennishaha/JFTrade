package strategy

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/strategy"
)

type routeLifecycleDesignStore struct {
	list       []srv.Definition
	definition srv.Definition
	found      bool
	err        error
	saveInput  srv.Definition
	saveErr    error
	deleteID   string
	deleteErr  error
}

func (s *routeLifecycleDesignStore) ListDefinitions() []srv.Definition { return s.list }
func (s *routeLifecycleDesignStore) GetDefinition(string) (srv.Definition, bool, error) {
	return s.definition, s.found, s.err
}
func (s *routeLifecycleDesignStore) SaveDefinition(input srv.Definition) (srv.Definition, error) {
	s.saveInput = input
	if s.saveErr != nil {
		return srv.Definition{}, s.saveErr
	}
	if input.ID == "" {
		input.ID = "generated-id"
	}
	return input, nil
}
func (s *routeLifecycleDesignStore) DeleteDefinition(id string) (srv.Definition, error) {
	s.deleteID = id
	return s.definition, s.deleteErr
}

type routeLifecycleCatalogStore struct {
	linkedIDs               []string
	listInstances           []srv.InstanceView
	validateStartableErr    error
	createDefinition        srv.Definition
	createBinding           srv.InstanceBinding
	createResult            srv.InstanceView
	createErr               error
	updateInstanceID        string
	updateBinding           srv.InstanceBinding
	updateResult            srv.InstanceView
	updateErr               error
	updateRuntimeRiskID     string
	updateRuntimeRisk       srv.RuntimeRiskSettings
	updateRuntimeRiskResult srv.InstanceView
	updateRuntimeRiskErr    error
	deleteInstanceID        string
	deleteInstanceResult    srv.InstanceView
	deleteInstanceErr       error
	transitionInstanceID    string
	transitionStatus        string
	transitionResult        srv.InstanceView
	transitionErr           error
	refreshInstanceID       string
	refreshInstanceResult   srv.InstanceView
	refreshInstanceErr      error
	applyDefinition         srv.Definition
	applyResult             srv.ApplyLinkedInstancesResult
	applyErr                error
	logsQuery               srv.LogQuery
	logsResult              srv.LogsResult
	logsExists              bool
	auditQuery              srv.AuditQuery
	auditResult             srv.AuditResult
	auditExists             bool
	instance                srv.ManagedInstance
	instanceFound           bool
	pluginCatalog           srv.PluginCatalog
	pluginOperation         srv.PluginOperation
	pluginOperationExists   bool
	uninstallGuidance       srv.PluginUninstallGuidance
	uninstallGuidanceExists bool
	installPluginID         string
	installPluginResult     srv.PluginOperation
	installPluginErr        error
	uninstallPluginID       string
	uninstallPluginResult   srv.PluginOperation
	uninstallPluginErr      error
}

func (s *routeLifecycleCatalogStore) ListInstances() []srv.InstanceView { return s.listInstances }
func (s *routeLifecycleCatalogStore) GetInstance(string) (srv.ManagedInstance, bool) {
	return s.instance, s.instanceFound
}
func (s *routeLifecycleCatalogStore) ValidateStartable(srv.ManagedInstance) error {
	return s.validateStartableErr
}
func (s *routeLifecycleCatalogStore) CreateInstance(def srv.Definition, binding srv.InstanceBinding) (srv.InstanceView, error) {
	s.createDefinition = def
	s.createBinding = binding
	return s.createResult, s.createErr
}
func (s *routeLifecycleCatalogStore) UpdateInstance(id string, binding srv.InstanceBinding) (srv.InstanceView, error) {
	s.updateInstanceID = id
	s.updateBinding = binding
	return s.updateResult, s.updateErr
}
func (s *routeLifecycleCatalogStore) UpdateInstanceRuntimeRisk(id string, risk srv.RuntimeRiskSettings) (srv.InstanceView, error) {
	s.updateRuntimeRiskID = id
	s.updateRuntimeRisk = risk
	return s.updateRuntimeRiskResult, s.updateRuntimeRiskErr
}
func (s *routeLifecycleCatalogStore) DeleteInstance(id string) (srv.InstanceView, error) {
	s.deleteInstanceID = id
	return s.deleteInstanceResult, s.deleteInstanceErr
}
func (s *routeLifecycleCatalogStore) TransitionInstance(id string, status string) (srv.InstanceView, error) {
	s.transitionInstanceID = id
	s.transitionStatus = status
	result := s.transitionResult
	result.Status = status
	return result, s.transitionErr
}
func (s *routeLifecycleCatalogStore) RefreshDefinition(string, srv.Definition) (srv.InstanceView, error) {
	return srv.InstanceView{}, nil
}
func (s *routeLifecycleCatalogStore) RefreshInstanceDefinition(id string) (srv.InstanceView, error) {
	s.refreshInstanceID = id
	return s.refreshInstanceResult, s.refreshInstanceErr
}
func (s *routeLifecycleCatalogStore) ApplyDefinitionToLinked(def srv.Definition) (srv.ApplyLinkedInstancesResult, error) {
	s.applyDefinition = def
	return s.applyResult, s.applyErr
}
func (s *routeLifecycleCatalogStore) GetLinkedInstanceIDs(string) []string { return s.linkedIDs }
func (s *routeLifecycleCatalogStore) GetLogs(_ string, query srv.LogQuery) (srv.LogsResult, bool) {
	s.logsQuery = query
	return s.logsResult, s.logsExists
}
func (s *routeLifecycleCatalogStore) GetAudit(_ string, query srv.AuditQuery) (srv.AuditResult, bool) {
	s.auditQuery = query
	return s.auditResult, s.auditExists
}
func (s *routeLifecycleCatalogStore) ReconcileOnStartup() (int, error) { return 0, nil }
func (s *routeLifecycleCatalogStore) PluginCatalog() srv.PluginCatalog { return s.pluginCatalog }
func (s *routeLifecycleCatalogStore) PluginOperation(string) (srv.PluginOperation, bool) {
	return s.pluginOperation, s.pluginOperationExists
}
func (s *routeLifecycleCatalogStore) PluginUninstallGuidance(string) (srv.PluginUninstallGuidance, bool) {
	return s.uninstallGuidance, s.uninstallGuidanceExists
}
func (s *routeLifecycleCatalogStore) InstallPlugin(id string) (srv.PluginOperation, error) {
	s.installPluginID = id
	return s.installPluginResult, s.installPluginErr
}
func (s *routeLifecycleCatalogStore) UninstallPlugin(id string) (srv.PluginOperation, error) {
	s.uninstallPluginID = id
	return s.uninstallPluginResult, s.uninstallPluginErr
}
func (s *routeLifecycleCatalogStore) Close() error { return nil }

type routeLifecycleRuntime struct {
	startErr error
	stopIDs  []string
}

func (r *routeLifecycleRuntime) Start(context.Context, srv.ManagedInstance) error { return r.startErr }
func (r *routeLifecycleRuntime) Stop(instanceID string)                           { r.stopIDs = append(r.stopIDs, instanceID) }
func (r *routeLifecycleRuntime) GetObservation(string) (srv.RuntimeObservation, bool) {
	return srv.RuntimeObservation{}, false
}
func (r *routeLifecycleRuntime) RuntimeSummary() srv.RuntimeSummary { return srv.RuntimeSummary{} }
func (r *routeLifecycleRuntime) ActiveInstrumentIDs() []string      { return nil }

func TestDefinitionRoutesNormalizeCreateUpdateAndDeleteGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)

	design := &routeLifecycleDesignStore{
		list:       []srv.Definition{{ID: "def-1", Name: "One"}},
		definition: srv.Definition{ID: "def-1", Name: "Breakout"},
		found:      true,
	}
	catalog := &routeLifecycleCatalogStore{linkedIDs: []string{"inst-1"}}
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), srv.NewService(design, catalog, &routeLifecycleRuntime{}))

	createReq := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategy-definitions", bytes.NewBufferString(`{"id":"client-id","name":"Draft"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK || design.saveInput.ID != "" || design.saveInput.Name != "Draft" {
		t.Fatalf("create status=%d saveInput=%#v body=%s", createRec.Code, design.saveInput, createRec.Body.String())
	}

	updateReq := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/strategy-definitions/def-9", bytes.NewBufferString(`{"name":"Updated"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK || design.saveInput.ID != "def-9" || design.saveInput.Name != "Updated" {
		t.Fatalf("update status=%d saveInput=%#v body=%s", updateRec.Code, design.saveInput, updateRec.Body.String())
	}

	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/strategy-definitions", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}

	deleteReq := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/api/v1/strategy-definitions/def-1", nil)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusBadRequest {
		t.Fatalf("delete guard status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	catalog.linkedIDs = nil
	deleteRec = httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK || design.deleteID != "def-1" {
		t.Fatalf("delete status=%d deleteID=%q body=%s", deleteRec.Code, design.deleteID, deleteRec.Body.String())
	}
}

func TestInstantiateApplyAndLifecycleRoutesFollowBusinessStateTransitions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	design := &routeLifecycleDesignStore{
		definition: srv.Definition{ID: "def-1", Name: "Breakout", SourceFormat: "pine-v6"},
		found:      true,
	}
	catalog := &routeLifecycleCatalogStore{
		createResult:            srv.InstanceView{ID: "inst-1"},
		updateResult:            srv.InstanceView{ID: "inst-1"},
		updateRuntimeRiskResult: srv.InstanceView{ID: "inst-1"},
		deleteInstanceResult:    srv.InstanceView{ID: "inst-1"},
		transitionResult:        srv.InstanceView{ID: "inst-1"},
		refreshInstanceResult:   srv.InstanceView{ID: "inst-1"},
		applyResult:             srv.ApplyLinkedInstancesResult{DefinitionID: "def-1", Applied: []string{"inst-1"}},
		instance:                srv.ManagedInstance{ID: "inst-1"},
		instanceFound:           true,
		listInstances:           []srv.InstanceView{{ID: "inst-1"}},
	}
	runtime := &routeLifecycleRuntime{}
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), srv.NewService(design, catalog, runtime))

	instantiateRec := httptest.NewRecorder()
	router.ServeHTTP(instantiateRec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategy-definitions/def-1/instantiate", nil))
	if instantiateRec.Code != http.StatusOK || catalog.createDefinition.ID != "def-1" || len(catalog.createBinding.Symbols) != 0 {
		t.Fatalf("instantiate status=%d def=%#v binding=%#v body=%s", instantiateRec.Code, catalog.createDefinition, catalog.createBinding, instantiateRec.Body.String())
	}

	applyRec := httptest.NewRecorder()
	router.ServeHTTP(applyRec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategy-definitions/def-1/apply-linked-instances", nil))
	if applyRec.Code != http.StatusOK || catalog.applyDefinition.ID != "def-1" {
		t.Fatalf("apply status=%d def=%#v body=%s", applyRec.Code, catalog.applyDefinition, applyRec.Body.String())
	}

	updateReq := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/strategies/inst-1", bytes.NewBufferString(`{"symbols":["US.AAPL"],"interval":"1m"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK || catalog.updateInstanceID != "inst-1" || len(catalog.updateBinding.Symbols) != 1 {
		t.Fatalf("update status=%d id=%q binding=%#v body=%s", updateRec.Code, catalog.updateInstanceID, catalog.updateBinding, updateRec.Body.String())
	}

	riskReq := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/strategies/inst-1/runtime-risk", bytes.NewBufferString(`{"mode":"close_only","closeOnly":true}`))
	riskReq.Header.Set("Content-Type", "application/json")
	riskRec := httptest.NewRecorder()
	router.ServeHTTP(riskRec, riskReq)
	if riskRec.Code != http.StatusOK || catalog.updateRuntimeRiskID != "inst-1" || !catalog.updateRuntimeRisk.CloseOnly {
		t.Fatalf("risk status=%d risk=%#v body=%s", riskRec.Code, catalog.updateRuntimeRisk, riskRec.Body.String())
	}

	pauseRec := httptest.NewRecorder()
	router.ServeHTTP(pauseRec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategies/inst-1/pause", nil))
	if pauseRec.Code != http.StatusOK || catalog.transitionStatus != "PAUSED" || len(runtime.stopIDs) != 1 || runtime.stopIDs[0] != "inst-1" {
		t.Fatalf("pause status=%d transition=%q stopIDs=%#v body=%s", pauseRec.Code, catalog.transitionStatus, runtime.stopIDs, pauseRec.Body.String())
	}

	stopRec := httptest.NewRecorder()
	router.ServeHTTP(stopRec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategies/inst-1/stop", nil))
	if stopRec.Code != http.StatusOK || catalog.transitionStatus != "STOPPED" || len(runtime.stopIDs) != 2 || runtime.stopIDs[1] != "inst-1" {
		t.Fatalf("stop status=%d transition=%q stopIDs=%#v body=%s", stopRec.Code, catalog.transitionStatus, runtime.stopIDs, stopRec.Body.String())
	}

	startRec := httptest.NewRecorder()
	router.ServeHTTP(startRec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategies/inst-1/start", nil))
	if startRec.Code != http.StatusOK || catalog.transitionStatus != "RUNNING" {
		t.Fatalf("start status=%d transition=%q body=%s", startRec.Code, catalog.transitionStatus, startRec.Body.String())
	}

	refreshRec := httptest.NewRecorder()
	router.ServeHTTP(refreshRec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategies/inst-1/refresh-definition", nil))
	if refreshRec.Code != http.StatusOK || catalog.refreshInstanceID != "inst-1" {
		t.Fatalf("refresh status=%d id=%q body=%s", refreshRec.Code, catalog.refreshInstanceID, refreshRec.Body.String())
	}

	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/strategies", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list instances status=%d body=%s", listRec.Code, listRec.Body.String())
	}
}

func TestDeleteInstanceRouteMapsBusinessOutcomes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("success deletes requested instance", func(t *testing.T) {
		router := gin.New()
		catalog := &routeLifecycleCatalogStore{
			deleteInstanceResult: srv.InstanceView{ID: "inst-1"},
		}
		RegisterRoutes(router.Group("/api/v1"), srv.NewService(&routeLifecycleDesignStore{}, catalog, &routeLifecycleRuntime{}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/api/v1/strategies/inst-1", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
		}
		if catalog.deleteInstanceID != "inst-1" {
			t.Fatalf("deleteInstanceID = %q, want inst-1", catalog.deleteInstanceID)
		}
	})

	t.Run("not found becomes 404 envelope", func(t *testing.T) {
		router := gin.New()
		catalog := &routeLifecycleCatalogStore{
			deleteInstanceErr: srv.NotFoundError("instance missing"),
		}
		RegisterRoutes(router.Group("/api/v1"), srv.NewService(&routeLifecycleDesignStore{}, catalog, &routeLifecycleRuntime{}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/api/v1/strategies/missing", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"code":"NOT_FOUND"`) {
			t.Fatalf("body = %s, want NOT_FOUND code", rec.Body.String())
		}
	})

	t.Run("busy delete becomes bad request", func(t *testing.T) {
		router := gin.New()
		catalog := &routeLifecycleCatalogStore{
			deleteInstanceErr: srv.BusyError("instance is running"),
		}
		RegisterRoutes(router.Group("/api/v1"), srv.NewService(&routeLifecycleDesignStore{}, catalog, &routeLifecycleRuntime{}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/api/v1/strategies/inst-1", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"code":"BAD_REQUEST"`) {
			t.Fatalf("body = %s, want BAD_REQUEST code", rec.Body.String())
		}
	})
}

func TestStrategyRoutesMapValidationAndBusinessErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newRouter := func(design *routeLifecycleDesignStore, catalog *routeLifecycleCatalogStore, runtime *routeLifecycleRuntime) *gin.Engine {
		router := gin.New()
		RegisterRoutes(router.Group("/api/v1"), srv.NewService(design, catalog, runtime))
		return router
	}

	t.Run("instantiate rejects malformed payload", func(t *testing.T) {
		router := newRouter(
			&routeLifecycleDesignStore{definition: srv.Definition{ID: "def-1"}, found: true},
			&routeLifecycleCatalogStore{},
			&routeLifecycleRuntime{},
		)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategy-definitions/def-1/instantiate", bytes.NewBufferString(`{"symbols":`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("instantiate create error becomes bad request", func(t *testing.T) {
		catalog := &routeLifecycleCatalogStore{createErr: srv.BadRequestError("symbol is required")}
		router := newRouter(
			&routeLifecycleDesignStore{definition: srv.Definition{ID: "def-1"}, found: true},
			catalog,
			&routeLifecycleRuntime{},
		)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategy-definitions/def-1/instantiate", bytes.NewBufferString(`{"symbols":["US.AAPL"],"interval":"1m"}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
		if catalog.createDefinition.ID != "def-1" || len(catalog.createBinding.Symbols) != 1 {
			t.Fatalf("create args = def:%#v binding:%#v", catalog.createDefinition, catalog.createBinding)
		}
	})

	t.Run("apply linked returns not found when definition is missing", func(t *testing.T) {
		router := newRouter(&routeLifecycleDesignStore{found: false}, &routeLifecycleCatalogStore{}, &routeLifecycleRuntime{})
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategy-definitions/missing/apply-linked-instances", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("apply linked maps definition read error to bad request", func(t *testing.T) {
		router := newRouter(&routeLifecycleDesignStore{err: errors.New("load failed")}, &routeLifecycleCatalogStore{}, &routeLifecycleRuntime{})
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategy-definitions/def-1/apply-linked-instances", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "load failed") {
			t.Fatalf("body = %s, want load failed error", rec.Body.String())
		}
	})

	t.Run("update instance rejects malformed payload", func(t *testing.T) {
		router := newRouter(&routeLifecycleDesignStore{}, &routeLifecycleCatalogStore{}, &routeLifecycleRuntime{})
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/strategies/inst-1", bytes.NewBufferString(`{"symbols":`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("runtime risk rejects malformed payload", func(t *testing.T) {
		router := newRouter(&routeLifecycleDesignStore{}, &routeLifecycleCatalogStore{}, &routeLifecycleRuntime{})
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/strategies/inst-1/runtime-risk", bytes.NewBufferString(`{"mode":`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("refresh definition maps not found", func(t *testing.T) {
		router := newRouter(
			&routeLifecycleDesignStore{},
			&routeLifecycleCatalogStore{refreshInstanceErr: srv.NotFoundError("instance missing")},
			&routeLifecycleRuntime{},
		)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategies/missing/refresh-definition", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("start instance maps runtime failure to gateway response", func(t *testing.T) {
		router := newRouter(
			&routeLifecycleDesignStore{},
			&routeLifecycleCatalogStore{
				instance:      srv.ManagedInstance{ID: "inst-1"},
				instanceFound: true,
			},
			&routeLifecycleRuntime{startErr: srv.UpstreamError("runtime start failed")},
		)
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategies/inst-1/start", nil)
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502, body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"code":"STRATEGY_RUNTIME_START_FAILED"`) {
			t.Fatalf("body = %s, want gateway code", rec.Body.String())
		}
	})

	t.Run("logs and audit return not found when activity is missing", func(t *testing.T) {
		router := newRouter(&routeLifecycleDesignStore{}, &routeLifecycleCatalogStore{}, &routeLifecycleRuntime{})

		logsMissingRec := httptest.NewRecorder()
		logsMissingReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/strategies/inst-1/logs?fromTime=bad-time", nil)
		router.ServeHTTP(logsMissingRec, logsMissingReq)
		if logsMissingRec.Code != http.StatusNotFound {
			t.Fatalf("logs missing status = %d, want 404, body=%s", logsMissingRec.Code, logsMissingRec.Body.String())
		}

		auditMissingRec := httptest.NewRecorder()
		auditMissingReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/strategies/inst-1/audit?toTime=bad-time", nil)
		router.ServeHTTP(auditMissingRec, auditMissingReq)
		if auditMissingRec.Code != http.StatusNotFound {
			t.Fatalf("audit missing status = %d, want 404, body=%s", auditMissingRec.Code, auditMissingRec.Body.String())
		}
	})
}

func TestActivityRoutesNormalizePaginationAndTimeFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	catalog := &routeLifecycleCatalogStore{
		logsExists:  true,
		logsResult:  srv.LogsResult{InstanceID: "inst-1"},
		auditExists: true,
		auditResult: srv.AuditResult{InstanceID: "inst-1"},
	}
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), srv.NewService(&routeLifecycleDesignStore{}, catalog, &routeLifecycleRuntime{}))

	logsRec := httptest.NewRecorder()
	router.ServeHTTP(logsRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/strategies/inst-1/logs?limit=-5&offset=-1&level=%20warn%20&fromTime=2026-06-22T01:02:03Z&toTime=2026-06-22T04:05:06Z", nil))
	if logsRec.Code != http.StatusOK {
		t.Fatalf("logs status=%d body=%s", logsRec.Code, logsRec.Body.String())
	}
	if catalog.logsQuery.Limit != 1 || catalog.logsQuery.Offset != 0 || catalog.logsQuery.Level != "warn" {
		t.Fatalf("logs query=%#v", catalog.logsQuery)
	}
	if catalog.logsQuery.FromAt == nil || catalog.logsQuery.ToAt == nil || !catalog.logsQuery.FromAt.Equal(mustParseTime("2026-06-22T01:02:03Z")) || !catalog.logsQuery.ToAt.Equal(mustParseTime("2026-06-22T04:05:06Z")) {
		t.Fatalf("logs time filters=%#v", catalog.logsQuery)
	}

	auditRec := httptest.NewRecorder()
	router.ServeHTTP(auditRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/strategies/inst-1/audit?kind=%20execution%20", nil))
	if auditRec.Code != http.StatusOK || catalog.auditQuery.Kind != "execution" {
		t.Fatalf("audit status=%d query=%#v body=%s", auditRec.Code, catalog.auditQuery, auditRec.Body.String())
	}
}

func TestPluginRoutesCoverCatalogOperationMutationAndGuidance(t *testing.T) {
	gin.SetMode(gin.TestMode)

	catalog := &routeLifecycleCatalogStore{
		pluginCatalog:           srv.PluginCatalog{TargetDir: "/plugins", Plugins: []srv.PluginCatalogItem{{Descriptor: srv.PluginDescriptor{ID: "plugin-a"}}}},
		pluginOperation:         srv.PluginOperation{OperationID: "op-1", PluginID: "plugin-a"},
		pluginOperationExists:   true,
		uninstallGuidance:       srv.PluginUninstallGuidance{PluginID: "plugin-a", Path: "/plugins/plugin-a", Exists: true},
		uninstallGuidanceExists: true,
		installPluginResult:     srv.PluginOperation{OperationID: "install-1", PluginID: "plugin-a"},
		uninstallPluginResult:   srv.PluginOperation{OperationID: "uninstall-1", PluginID: "plugin-a"},
	}
	router := gin.New()
	service := srv.NewService(&routeLifecycleDesignStore{}, catalog, &routeLifecycleRuntime{})
	RegisterPluginRoutes(router.Group("/api/v1"), service)

	pluginsRec := httptest.NewRecorder()
	router.ServeHTTP(pluginsRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/plugins", nil))
	if pluginsRec.Code != http.StatusOK {
		t.Fatalf("plugins status=%d body=%s", pluginsRec.Code, pluginsRec.Body.String())
	}

	opRec := httptest.NewRecorder()
	router.ServeHTTP(opRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/plugins/operations/op-1", nil))
	if opRec.Code != http.StatusOK {
		t.Fatalf("plugin op status=%d body=%s", opRec.Code, opRec.Body.String())
	}

	installRec := httptest.NewRecorder()
	router.ServeHTTP(installRec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/plugins/plugin-a/install", nil))
	if installRec.Code != http.StatusOK || catalog.installPluginID != "plugin-a" {
		t.Fatalf("install status=%d id=%q body=%s", installRec.Code, catalog.installPluginID, installRec.Body.String())
	}

	uninstallRec := httptest.NewRecorder()
	router.ServeHTTP(uninstallRec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/plugins/plugin-a/uninstall", nil))
	if uninstallRec.Code != http.StatusOK || catalog.uninstallPluginID != "plugin-a" {
		t.Fatalf("uninstall status=%d id=%q body=%s", uninstallRec.Code, catalog.uninstallPluginID, uninstallRec.Body.String())
	}

	guidanceRec := httptest.NewRecorder()
	router.ServeHTTP(guidanceRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/plugins/plugin-a/uninstall-guidance", nil))
	if guidanceRec.Code != http.StatusOK {
		t.Fatalf("guidance status=%d body=%s", guidanceRec.Code, guidanceRec.Body.String())
	}
}

func TestPluginMutationRoutesMapNotFoundAndInternalFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)

	catalog := &routeLifecycleCatalogStore{
		installPluginErr:        os.ErrNotExist,
		uninstallPluginErr:      errors.New("unlink failed"),
		pluginOperationExists:   false,
		uninstallGuidanceExists: false,
	}
	router := gin.New()
	service := srv.NewService(&routeLifecycleDesignStore{}, catalog, &routeLifecycleRuntime{})
	RegisterPluginRoutes(router.Group("/api/v1"), service)

	installRec := httptest.NewRecorder()
	router.ServeHTTP(installRec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/plugins/missing/install", nil))
	if installRec.Code != http.StatusNotFound {
		t.Fatalf("install not found status=%d body=%s", installRec.Code, installRec.Body.String())
	}

	uninstallRec := httptest.NewRecorder()
	router.ServeHTTP(uninstallRec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/plugins/plugin-a/uninstall", nil))
	if uninstallRec.Code != http.StatusInternalServerError {
		t.Fatalf("uninstall error status=%d body=%s", uninstallRec.Code, uninstallRec.Body.String())
	}

	opRec := httptest.NewRecorder()
	router.ServeHTTP(opRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/plugins/operations/missing", nil))
	if opRec.Code != http.StatusNotFound {
		t.Fatalf("operation missing status=%d body=%s", opRec.Code, opRec.Body.String())
	}

	guidanceRec := httptest.NewRecorder()
	router.ServeHTTP(guidanceRec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/plugins/missing/uninstall-guidance", nil))
	if guidanceRec.Code != http.StatusNotFound {
		t.Fatalf("guidance missing status=%d body=%s", guidanceRec.Code, guidanceRec.Body.String())
	}
}

func mustParseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}
