package rustmigration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	strategyapi "github.com/jftrade/jftrade-main/internal/api/strategy"
	strategystore "github.com/jftrade/jftrade-main/internal/store/strategy"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategycatalog "github.com/jftrade/jftrade-main/internal/strategy/catalog"
	_ "modernc.org/sqlite"
)

const (
	stage9StrategyDefinitionsWriteFixtureVersion = "stage9.strategy-definitions-write.v1"
	stage9StrategyDefinitionsWriteTimestamp      = "2026-08-22T06:00:00Z"
)

type stage9StrategyDefinitionsWriteFixture struct {
	Version string                               `json:"version"`
	Cases   []stage9StrategyDefinitionsWriteCase `json:"cases"`
}

type stage9StrategyDefinitionsWriteCase struct {
	Name                string            `json:"name"`
	Method              string            `json:"method"`
	RequestPaths        []string          `json:"requestPaths"`
	RequestBodies       []string          `json:"requestBodies,omitempty"`
	Concurrent          bool              `json:"concurrent,omitempty"`
	Repeat              int               `json:"repeat,omitempty"`
	ExpectedStatuses    []int             `json:"expectedStatuses"`
	Responses           []json.RawMessage `json:"responses"`
	ExpectedObservation map[string]any    `json:"expectedObservation"`
}

type stage9StrategyDefinitionsWriteCaseSpec struct {
	Name       string
	Method     string
	Paths      []string
	Bodies     []string
	Concurrent bool
	Repeat     int
	Setup      func(*testing.T) *stage9StrategyDefinitionsWriteHarness
}

type stage9StrategyDefinitionsWriteHarness struct {
	router       *gin.Engine
	design       *stage9RecordingDesignStore
	catalog      *stage9RecordingCatalogStore
	baseCatalog  *strategycatalog.Service
	resource     strategystore.Resource
	dbPath       string
	idAliases    map[string]string
	definitionID map[string]string
}

type stage9RecordingDesignStore struct {
	stratsrv.DesignStore
	mu         sync.Mutex
	getIDs     []string
	saveInputs []stratsrv.Definition
	deleteIDs  []string
}

func (s *stage9RecordingDesignStore) GetDefinition(id string) (stratsrv.Definition, bool, error) {
	s.mu.Lock()
	s.getIDs = append(s.getIDs, id)
	s.mu.Unlock()
	return s.DesignStore.GetDefinition(id)
}

func (s *stage9RecordingDesignStore) SaveDefinition(input stratsrv.Definition) (stratsrv.Definition, error) {
	s.mu.Lock()
	s.saveInputs = append(s.saveInputs, input)
	s.mu.Unlock()
	return s.DesignStore.SaveDefinition(input)
}

func (s *stage9RecordingDesignStore) DeleteDefinition(id string) (stratsrv.Definition, error) {
	s.mu.Lock()
	s.deleteIDs = append(s.deleteIDs, id)
	s.mu.Unlock()
	return s.DesignStore.DeleteDefinition(id)
}

type stage9RecordingCatalogStore struct {
	*strategycatalog.Service
	mu             sync.Mutex
	linkedSequence [][]string
	linkedReads    []string
	applyInputs    []stratsrv.Definition
	applyErr       error
	createInputs   []stage9CreateCall
	createErr      error
	createResults  []stratsrv.InstanceView
}

type stage9CreateCall struct {
	DefinitionID  string
	Symbols       []string
	Interval      string
	ExecutionMode string
}

func (s *stage9RecordingCatalogStore) GetLinkedInstanceIDs(definitionID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.linkedReads = append(s.linkedReads, definitionID)
	if len(s.linkedSequence) > 0 {
		index := len(s.linkedReads) - 1
		if index >= len(s.linkedSequence) {
			index = len(s.linkedSequence) - 1
		}
		return append([]string(nil), s.linkedSequence[index]...)
	}
	return s.Service.GetLinkedInstanceIDs(definitionID)
}

func (s *stage9RecordingCatalogStore) ApplyDefinitionToLinked(definition stratsrv.Definition) (stratsrv.ApplyLinkedInstancesResult, error) {
	s.mu.Lock()
	s.applyInputs = append(s.applyInputs, definition)
	err := s.applyErr
	s.mu.Unlock()
	if err != nil {
		return stratsrv.ApplyLinkedInstancesResult{}, err
	}
	return s.Service.ApplyDefinitionToLinked(definition)
}

func (s *stage9RecordingCatalogStore) CreateInstance(definition stratsrv.Definition, binding stratsrv.InstanceBinding) (stratsrv.InstanceView, error) {
	s.mu.Lock()
	s.createInputs = append(s.createInputs, stage9CreateCall{
		DefinitionID:  definition.ID,
		Symbols:       append([]string(nil), binding.Symbols...),
		Interval:      binding.Interval,
		ExecutionMode: binding.ExecutionMode,
	})
	err := s.createErr
	s.mu.Unlock()
	if err != nil {
		return stratsrv.InstanceView{}, err
	}
	result, err := s.Service.CreateInstance(definition, binding)
	if err == nil {
		s.mu.Lock()
		s.createResults = append(s.createResults, result)
		s.mu.Unlock()
	}
	return result, err
}

type stage9StaticDesignStore struct {
	definition stratsrv.Definition
	found      bool
	getErr     error
	saveErr    error
	deleteErr  error
}

func (s *stage9StaticDesignStore) ListDefinitions() ([]stratsrv.Definition, error) {
	if !s.found {
		return []stratsrv.Definition{}, nil
	}
	return []stratsrv.Definition{s.definition}, nil
}

func (s *stage9StaticDesignStore) GetDefinition(string) (stratsrv.Definition, bool, error) {
	return s.definition, s.found, s.getErr
}

func (s *stage9StaticDesignStore) SaveDefinition(input stratsrv.Definition) (stratsrv.Definition, error) {
	if s.saveErr != nil {
		return stratsrv.Definition{}, s.saveErr
	}
	return input, nil
}

func (s *stage9StaticDesignStore) DeleteDefinition(string) (stratsrv.Definition, error) {
	if s.deleteErr != nil {
		return stratsrv.Definition{}, s.deleteErr
	}
	return s.definition, nil
}

func (s *stage9StaticDesignStore) ListDefinitionVersions(string) ([]stratsrv.DefinitionVersionSummary, bool, error) {
	return nil, false, nil
}

func (s *stage9StaticDesignStore) GetDefinitionVersion(string, string) (stratsrv.DefinitionVersion, bool, error) {
	return stratsrv.DefinitionVersion{}, false, nil
}

func stage9NewStrategyDefinitionsWriteHarness(
	t *testing.T,
	design stratsrv.DesignStore,
	baseCatalog *strategycatalog.Service,
) *stage9StrategyDefinitionsWriteHarness {
	t.Helper()
	recordingDesign := &stage9RecordingDesignStore{DesignStore: design}
	recordingCatalog := &stage9RecordingCatalogStore{Service: baseCatalog}
	router := gin.New()
	strategyapi.RegisterRoutes(
		router.Group("/api/v1"),
		stratsrv.NewService(recordingDesign, recordingCatalog, nil),
	)
	return &stage9StrategyDefinitionsWriteHarness{
		router:       router,
		design:       recordingDesign,
		catalog:      recordingCatalog,
		baseCatalog:  baseCatalog,
		resource:     nil,
		idAliases:    map[string]string{},
		definitionID: map[string]string{},
	}
}

func stage9ActualStrategyDefinitionsWriteHarness(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
	t.Helper()
	path := filepath.Join(t.TempDir(), "strategy-definitions.json")
	resource, err := strategystore.New(path)
	if err != nil {
		t.Fatalf("open strategy definition fixture store: %v", err)
	}
	t.Cleanup(func() { _ = resource.Close() })
	baseCatalog, err := strategycatalog.New(nil, nil, filepath.Join(t.TempDir(), "plugins"))
	if err != nil {
		t.Fatalf("open strategy catalog fixture: %v", err)
	}
	harness := stage9NewStrategyDefinitionsWriteHarness(t, resource, baseCatalog)
	harness.resource = resource
	harness.dbPath = strategystore.DeriveDBPath(path)
	return harness
}

func stage9StaticStrategyDefinitionsWriteHarness(t *testing.T, store stratsrv.DesignStore) *stage9StrategyDefinitionsWriteHarness {
	t.Helper()
	baseCatalog, err := strategycatalog.New(nil, nil, filepath.Join(t.TempDir(), "plugins"))
	if err != nil {
		t.Fatalf("open strategy catalog fixture: %v", err)
	}
	return stage9NewStrategyDefinitionsWriteHarness(t, store, baseCatalog)
}

func stage9DefinitionFixtureValue(id, name, description, script string) stratsrv.Definition {
	return stratsrv.Definition{
		ID:           id,
		Name:         name,
		Description:  description,
		Runtime:      stratsrv.RuntimePinePlan,
		SourceFormat: "pine-v6",
		Symbol:       "US.AAPL",
		Interval:     "5m",
		Script:       script,
	}
}

func stage9SeedDefinition(t *testing.T, resource strategystore.Resource, definition stratsrv.Definition) stratsrv.Definition {
	t.Helper()
	created, err := resource.SaveDefinition(definition)
	if err != nil {
		t.Fatalf("seed strategy definition %s: %v", definition.ID, err)
	}
	return created
}

func stage9DefaultScript(name string) string {
	return fmt.Sprintf("//@version=6\nstrategy(\"%s\")\nslow = ta.sma(close, 20)", name)
}

func stage9JSONBody(value map[string]any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("encode strategy definition request body: %v", err))
	}
	return string(encoded)
}

func stage9Request(t *testing.T, router *gin.Engine, method, path, body string) (int, map[string]any) {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	request := httptest.NewRequestWithContext(context.Background(), method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var envelope map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s %s response: %v (%s)", method, path, err, recorder.Body.String())
	}
	return recorder.Code, envelope
}

func stage9RunStrategyDefinitionsWriteCase(
	t *testing.T,
	spec stage9StrategyDefinitionsWriteCaseSpec,
) stage9StrategyDefinitionsWriteCase {
	t.Helper()
	harness := spec.Setup(t)
	paths := append([]string(nil), spec.Paths...)
	bodies := append([]string(nil), spec.Bodies...)
	if spec.Concurrent {
		if spec.Repeat < 1 {
			t.Fatalf("case %s has invalid repeat %d", spec.Name, spec.Repeat)
		}
		paths = make([]string, spec.Repeat)
		bodies = make([]string, spec.Repeat)
		for index := range paths {
			paths[index] = spec.Paths[0]
			bodies[index] = spec.Bodies[0]
		}
	}
	if len(paths) != len(bodies) {
		t.Fatalf("case %s paths/bodies length mismatch", spec.Name)
	}
	statuses := make([]int, len(paths))
	responses := make([]json.RawMessage, len(paths))
	if spec.Concurrent {
		var wait sync.WaitGroup
		var mu sync.Mutex
		for index := range paths {
			index := index
			wait.Add(1)
			go func() {
				defer wait.Done()
				status, envelope := stage9Request(t, harness.router, spec.Method, paths[index], bodies[index])
				harness.normalizeDynamicIDs(envelope, spec.Name)
				stage9NormalizeStrategyWriteValue(envelope, harness.idAliases)
				encoded, err := json.Marshal(envelope)
				if err != nil {
					t.Errorf("encode case %s response: %v", spec.Name, err)
					return
				}
				mu.Lock()
				statuses[index] = status
				responses[index] = encoded
				mu.Unlock()
			}()
		}
		wait.Wait()
		sort.SliceStable(responses, func(left, right int) bool {
			return string(responses[left]) < string(responses[right])
		})
		sort.Ints(statuses)
	} else {
		for index := range paths {
			status, envelope := stage9Request(t, harness.router, spec.Method, paths[index], bodies[index])
			harness.normalizeDynamicIDs(envelope, spec.Name)
			stage9NormalizeStrategyWriteValue(envelope, harness.idAliases)
			encoded, err := json.Marshal(envelope)
			if err != nil {
				t.Fatalf("encode case %s response: %v", spec.Name, err)
			}
			statuses[index] = status
			responses[index] = encoded
		}
	}
	return stage9StrategyDefinitionsWriteCase{
		Name:                spec.Name,
		Method:              spec.Method,
		RequestPaths:        paths,
		RequestBodies:       bodies,
		Concurrent:          spec.Concurrent,
		Repeat:              spec.Repeat,
		ExpectedStatuses:    statuses,
		Responses:           responses,
		ExpectedObservation: harness.observation(),
	}
}

func (h *stage9StrategyDefinitionsWriteHarness) normalizeDynamicIDs(envelope map[string]any, name string) {
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		return
	}
	id, ok := data["id"].(string)
	if !ok || id == "" {
		return
	}
	switch {
	case strings.HasPrefix(name, "create-"):
		h.idAliases[id] = "<generated-definition-id>"
	case strings.HasPrefix(name, "instantiate-"):
		h.idAliases[id] = "<generated-instance-id>"
	}
}

func stage9NormalizeStrategyWriteValue(value any, aliases map[string]string) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if stringValue, ok := child.(string); ok {
				if replacement, exists := aliases[stringValue]; exists {
					current[key] = replacement
					continue
				}
				if key == "timestamp" || strings.HasSuffix(key, "At") || key == "compiledAt" {
					current[key] = stage9StrategyDefinitionsWriteTimestamp
				}
				continue
			}
			stage9NormalizeStrategyWriteValue(child, aliases)
		}
	case []any:
		for index, child := range current {
			if stringValue, ok := child.(string); ok {
				if replacement, exists := aliases[stringValue]; exists {
					current[index] = replacement
				}
				continue
			}
			stage9NormalizeStrategyWriteValue(child, aliases)
		}
	}
}

func (h *stage9StrategyDefinitionsWriteHarness) observation() map[string]any {
	h.design.mu.Lock()
	getIDs := append([]string(nil), h.design.getIDs...)
	saves := append([]stratsrv.Definition(nil), h.design.saveInputs...)
	deleteIDs := append([]string(nil), h.design.deleteIDs...)
	h.design.mu.Unlock()
	h.catalog.mu.Lock()
	linkedReads := append([]string(nil), h.catalog.linkedReads...)
	applyInputs := append([]stratsrv.Definition(nil), h.catalog.applyInputs...)
	createInputs := append([]stage9CreateCall(nil), h.catalog.createInputs...)
	h.catalog.mu.Unlock()
	if len(saves) > 1 {
		sort.SliceStable(saves, func(left, right int) bool {
			return saves[left].Name < saves[right].Name
		})
	}
	definitionSaves := make([]map[string]any, 0, len(saves))
	for _, definition := range saves {
		definitionSaves = append(definitionSaves, stage9DefinitionCall(definition))
	}
	applies := make([]map[string]any, 0, len(applyInputs))
	for _, definition := range applyInputs {
		applies = append(applies, map[string]any{
			"definitionId": definition.ID,
			"version":      definition.Version,
		})
	}
	creates := make([]map[string]any, 0, len(createInputs))
	for _, call := range createInputs {
		creates = append(creates, map[string]any{
			"definitionId":  call.DefinitionID,
			"symbols":       call.Symbols,
			"interval":      call.Interval,
			"executionMode": call.ExecutionMode,
		})
	}
	return map[string]any{
		"definitionReads":   getIDs,
		"definitionSaves":   definitionSaves,
		"definitionDeletes": deleteIDs,
		"linkedReads":       linkedReads,
		"applies":           applies,
		"creates":           creates,
	}
}

func stage9DefinitionCall(definition stratsrv.Definition) map[string]any {
	return map[string]any{
		"id":           definition.ID,
		"name":         definition.Name,
		"version":      definition.Version,
		"description":  definition.Description,
		"runtime":      definition.Runtime,
		"sourceFormat": definition.SourceFormat,
		"symbol":       definition.Symbol,
		"interval":     definition.Interval,
		"script":       definition.Script,
	}
}


func stage9StrategyDefinitionsWriteCaseSpecs() []stage9StrategyDefinitionsWriteCaseSpec {
	valid := stage9DefaultScript("Fixture")
	base := func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
		return stage9ActualStrategyDefinitionsWriteHarness(t)
	}
	return []stage9StrategyDefinitionsWriteCaseSpec{
		{
			Name:   "create-success-client-id-ignored",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions"},
			Bodies: []string{stage9JSONBody(map[string]any{
				"id": "client-id", "name": "Created", "description": "created",
				"symbol": " us.aapl ", "interval": " 5m ", "script": valid,
			})},
			Setup: base,
		},
		{
			Name:   "create-malformed-body",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions"},
			Bodies: []string{"{"},
			Setup:  base,
		},
		{
			Name:   "create-invalid-script-400",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions"},
			Bodies: []string{stage9JSONBody(map[string]any{
				"name": "Invalid", "script": "strategy(",
			})},
			Setup: base,
		},
		{
			Name:   "create-malformed-body-before-unavailable",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions"},
			Bodies: []string{"{"},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				baseCatalog, err := strategycatalog.New(nil, nil, filepath.Join(t.TempDir(), "plugins"))
				if err != nil {
					t.Fatalf("open strategy catalog fixture: %v", err)
				}
				return stage9NewStrategyDefinitionsWriteHarness(
					t,
					strategystore.NewUnavailable(filepath.Join(t.TempDir(), "unavailable.json")),
					baseCatalog,
				)
			},
		},
		{
			Name:   "update-version-and-duplicate",
			Method: http.MethodPut,
			Paths: []string{
				"/api/v1/strategy-definitions/fixture-current",
				"/api/v1/strategy-definitions/fixture-current",
			},
			Bodies: []string{
				stage9JSONBody(map[string]any{
					"id": "body-id", "name": "Updated", "description": "second",
					"symbol": "US.MSFT", "interval": "1m", "script": stage9DefaultScript("Updated"),
				}),
				stage9JSONBody(map[string]any{
					"id": "body-id", "name": "Updated", "description": "second",
					"symbol": "US.MSFT", "interval": "1m", "script": stage9DefaultScript("Updated"),
				}),
			},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				harness := stage9ActualStrategyDefinitionsWriteHarness(t)
				stage9SeedDefinition(t, harness.resource, stage9DefinitionFixtureValue(
					"fixture-current", "Current", "first", stage9DefaultScript("Current"),
				))
				return harness
			},
		},
		{
			Name:   "update-missing-upserts",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategy-definitions/missing-upsert"},
			Bodies: []string{stage9JSONBody(map[string]any{
				"id": "body-id", "name": "Upserted", "script": valid,
			})},
			Setup: base,
		},
		{
			Name:   "update-malformed-body",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategy-definitions/fixture-current"},
			Bodies: []string{"{"},
			Setup:  base,
		},
		{
			Name:   "update-rollback-on-snapshot-failure",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategy-definitions/fixture-rollback"},
			Bodies: []string{stage9JSONBody(map[string]any{
				"name": "Changed", "description": "must rollback", "script": stage9DefaultScript("Rollback"),
			})},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				harness := stage9ActualStrategyDefinitionsWriteHarness(t)
				stage9SeedDefinition(t, harness.resource, stage9DefinitionFixtureValue(
					"fixture-rollback", "Rollback", "before", stage9DefaultScript("Rollback"),
				))
				db, err := sql.Open("sqlite", harness.dbPath)
				if err != nil {
					t.Fatalf("open rollback database: %v", err)
				}
				_, err = db.Exec(`CREATE TRIGGER reject_stage9_strategy_write_snapshot
					BEFORE INSERT ON strategy_definition_versions
					WHEN NEW.version = '0.1.1'
					BEGIN
						SELECT RAISE(ABORT, 'stage9 snapshot insert rejected');
					END`)
				if closeErr := db.Close(); err != nil || closeErr != nil {
					t.Fatalf("install rollback trigger: exec=%v close=%v", err, closeErr)
				}
				return harness
			},
		},
		{
			Name:   "delete-linked-guard-then-soft-delete",
			Method: http.MethodDelete,
			Paths:  []string{"/api/v1/strategy-definitions/fixture-delete", "/api/v1/strategy-definitions/fixture-delete"},
			Bodies: []string{"", ""},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				harness := stage9ActualStrategyDefinitionsWriteHarness(t)
				stage9SeedDefinition(t, harness.resource, stage9DefinitionFixtureValue(
					"fixture-delete", "Delete", "soft delete", stage9DefaultScript("Delete"),
				))
				harness.catalog.linkedSequence = [][]string{{"inst-1"}, {}}
				return harness
			},
		},
		{
			Name:   "delete-missing-404",
			Method: http.MethodDelete,
			Paths:  []string{"/api/v1/strategy-definitions/missing"},
			Bodies: []string{""},
			Setup:  base,
		},
		{
			Name:   "apply-linked-success-lifecycle",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions/fixture-apply/apply-linked-instances"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				harness := stage9ActualStrategyDefinitionsWriteHarness(t)
				old := stage9SeedDefinition(t, harness.resource, stage9DefinitionFixtureValue(
					"fixture-apply", "Apply", "old", stage9DefaultScript("Apply"),
				))
				latest := old
				latest.Description = "latest"
				latest.Script = stage9DefaultScript("Apply Latest")
				latest, err := harness.resource.SaveDefinition(latest)
				if err != nil {
					t.Fatalf("seed latest definition: %v", err)
				}
				stale, err := harness.baseCatalog.CreateInstance(old, stratsrv.InstanceBinding{Symbols: []string{"US.AAPL"}, Interval: "1m", ExecutionMode: strategycatalog.ExecutionModeNotifyOnly})
				if err != nil {
					t.Fatalf("seed stale linked instance: %v", err)
				}
				harness.idAliases[stale.ID] = "stale-stopped"
				current, err := harness.baseCatalog.CreateInstance(latest, stratsrv.InstanceBinding{Symbols: []string{"US.MSFT"}, Interval: "5m", ExecutionMode: strategycatalog.ExecutionModeNotifyOnly})
				if err != nil {
					t.Fatalf("seed latest linked instance: %v", err)
				}
				harness.idAliases[current.ID] = "already-latest"
				busy, err := harness.baseCatalog.CreateInstance(old, stratsrv.InstanceBinding{Symbols: []string{"US.TSLA"}, Interval: "15m", ExecutionMode: strategycatalog.ExecutionModeNotifyOnly})
				if err != nil {
					t.Fatalf("seed busy linked instance: %v", err)
				}
				harness.idAliases[busy.ID] = "busy-running"
				if _, err := harness.baseCatalog.TransitionInstance(busy.ID, strategycatalog.StatusRunning); err != nil {
					t.Fatalf("mark busy linked instance running: %v", err)
				}
				other := stage9DefinitionFixtureValue("fixture-other", "Other", "other", stage9DefaultScript("Other"))
				if _, err := harness.baseCatalog.CreateInstance(other, stratsrv.InstanceBinding{}); err != nil {
					t.Fatalf("seed unrelated instance: %v", err)
				}
				_ = latest
				return harness
			},
		},
		{
			Name:   "apply-definition-read-unavailable-400",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions/fixture-apply/apply-linked-instances"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				baseCatalog, err := strategycatalog.New(nil, nil, filepath.Join(t.TempDir(), "plugins"))
				if err != nil {
					t.Fatalf("open strategy catalog fixture: %v", err)
				}
				return stage9NewStrategyDefinitionsWriteHarness(t, &stage9StaticDesignStore{getErr: errors.New("definition store unavailable")}, baseCatalog)
			},
		},
		{
			Name:   "apply-linked-busy-maps-400",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions/fixture-apply/apply-linked-instances"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				harness := stage9StaticStrategyDefinitionsWriteHarness(t, &stage9StaticDesignStore{
					definition: stage9DefinitionFixtureValue("fixture-apply", "Apply", "latest", valid),
					found:      true,
				})
				harness.catalog.applyErr = stratsrv.BusyError("strategy instance must be stopped before modification")
				return harness
			},
		},
		{
			Name:   "apply-linked-store-failure-500",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions/fixture-apply/apply-linked-instances"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				harness := stage9StaticStrategyDefinitionsWriteHarness(t, &stage9StaticDesignStore{
					definition: stage9DefinitionFixtureValue("fixture-apply", "Apply", "latest", valid),
					found:      true,
				})
				harness.catalog.applyErr = errors.New("catalog repository unavailable")
				return harness
			},
		},
		{
			Name:   "instantiate-empty-body-success",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions/fixture-instantiate/instantiate"},
			Bodies: []string{""},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				harness := stage9ActualStrategyDefinitionsWriteHarness(t)
				stage9SeedDefinition(t, harness.resource, stage9DefinitionFixtureValue(
					"fixture-instantiate", "Instantiate", "instance", stage9DefaultScript("Instantiate"),
				))
				return harness
			},
		},
		{
			Name:   "instantiate-custom-binding-success",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions/fixture-instantiate/instantiate"},
			Bodies: []string{`{"symbols":["us:aapl","US:AAPL"],"interval":" 1m ","executionMode":"notify_only"}`},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				harness := stage9ActualStrategyDefinitionsWriteHarness(t)
				stage9SeedDefinition(t, harness.resource, stage9DefinitionFixtureValue(
					"fixture-instantiate", "Instantiate", "instance", stage9DefaultScript("Instantiate"),
				))
				return harness
			},
		},
		{
			Name:   "instantiate-definition-missing-precedes-malformed-body",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions/missing/instantiate"},
			Bodies: []string{"{"},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				return stage9StaticStrategyDefinitionsWriteHarness(t, &stage9StaticDesignStore{found: false})
			},
		},
		{
			Name:   "instantiate-malformed-body-400",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions/fixture-instantiate/instantiate"},
			Bodies: []string{"{"},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				return stage9StaticStrategyDefinitionsWriteHarness(t, &stage9StaticDesignStore{
					definition: stage9DefinitionFixtureValue("fixture-instantiate", "Instantiate", "instance", valid),
					found:      true,
				})
			},
		},
		{
			Name:   "instantiate-catalog-failure-500",
			Method: http.MethodPost,
			Paths:  []string{"/api/v1/strategy-definitions/fixture-instantiate/instantiate"},
			Bodies: []string{`{"symbols":["US.AAPL"]}`},
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				harness := stage9StaticStrategyDefinitionsWriteHarness(t, &stage9StaticDesignStore{
					definition: stage9DefinitionFixtureValue("fixture-instantiate", "Instantiate", "instance", valid),
					found:      true,
				})
				harness.catalog.createErr = errors.New("catalog repository unavailable")
				return harness
			},
		},
		{
			Name:   "concurrent-update-no-lost-version",
			Method: http.MethodPut,
			Paths:  []string{"/api/v1/strategy-definitions/fixture-concurrent"},
			Bodies: []string{stage9JSONBody(map[string]any{
				"name": "Concurrent", "description": "same update", "script": valid,
			})},
			Concurrent: true,
			Repeat:     8,
			Setup: func(t *testing.T) *stage9StrategyDefinitionsWriteHarness {
				harness := stage9ActualStrategyDefinitionsWriteHarness(t)
				stage9SeedDefinition(t, harness.resource, stage9DefinitionFixtureValue(
					"fixture-concurrent", "Concurrent", "before", stage9DefaultScript("Concurrent"),
				))
				return harness
			},
		},
	}
}

// TestStage9StrategyDefinitionsWriteFixtureMatchesCurrentGoOwner freezes the
// five mutation handlers through Gin. The fixture uses temporary Go stores and
// catalog services only; it never opens a production database or runtime.
func TestStage9StrategyDefinitionsWriteFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve strategy-definitions-write fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/strategy-definitions-write.json",
	)
	want := stage9StrategyDefinitionsWriteFixture{
		Version: stage9StrategyDefinitionsWriteFixtureVersion,
		Cases:   make([]stage9StrategyDefinitionsWriteCase, 0),
	}
	for _, spec := range stage9StrategyDefinitionsWriteCaseSpecs() {
		want.Cases = append(want.Cases, stage9RunStrategyDefinitionsWriteCase(t, spec))
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode strategy-definitions-write fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write strategy-definitions-write fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read strategy-definitions-write fixture: %v", err)
	}
	var got stage9StrategyDefinitionsWriteFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode strategy-definitions-write fixture: %v", err)
	}
	compactStage9StrategyDefinitionsWriteFixture(&got)
	compactStage9StrategyDefinitionsWriteFixture(&want)
	wantBytes, _ := json.Marshal(want)
	gotBytes, _ := json.Marshal(got)
	if !bytes.Equal(gotBytes, wantBytes) {
		limit := len(gotBytes)
		if len(wantBytes) < limit {
			limit = len(wantBytes)
		}
		firstDifference := limit
		for index := 0; index < limit; index++ {
			if gotBytes[index] != wantBytes[index] {
				firstDifference = index
				break
			}
		}
		start := firstDifference - 40
		if start < 0 {
			start = 0
		}
		end := firstDifference + 80
		if end > limit {
			end = limit
		}
		t.Fatalf("strategy-definitions-write fixture drifted at %d: want=%q got=%q", firstDifference, wantBytes[start:end], gotBytes[start:end])
	}
}

func compactStage9StrategyDefinitionsWriteFixture(fixture *stage9StrategyDefinitionsWriteFixture) {
	for caseIndex := range fixture.Cases {
		for responseIndex, response := range fixture.Cases[caseIndex].Responses {
			var compacted bytes.Buffer
			if err := json.Compact(&compacted, response); err == nil {
				fixture.Cases[caseIndex].Responses[responseIndex] = append(
					json.RawMessage(nil),
					compacted.Bytes()...,
				)
			}
		}
	}
}
