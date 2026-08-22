package rustmigration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	strategyapi "github.com/jftrade/jftrade-main/internal/api/strategy"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	catalog "github.com/jftrade/jftrade-main/internal/strategy/catalog"
)

const stage9PluginsWriteFixtureVersion = "stage9.plugins-write.v1"

type stage9PluginsWriteFixture struct {
	Version string                          `json:"version"`
	Cases   []stage9PluginsWriteFixtureCase `json:"cases"`
}

type stage9PluginsWriteFixtureCase struct {
	Name                 string                        `json:"name"`
	Method               string                        `json:"method"`
	RequestPaths         []string                      `json:"requestPaths"`
	RequestBodies        []string                      `json:"requestBodies,omitempty"`
	InitialPluginPresent bool                          `json:"initialPluginPresent"`
	InitialInstalled     bool                          `json:"initialInstalled"`
	PersistFailure       bool                          `json:"persistFailure,omitempty"`
	Concurrent           bool                          `json:"concurrent,omitempty"`
	ExpectedStatuses     []int                         `json:"expectedStatuses"`
	Responses            []json.RawMessage             `json:"responses"`
	ExpectedObservation  stage9PluginsWriteObservation `json:"expectedObservation"`
}

type stage9PluginsWriteObservation struct {
	DurablePluginPresent  bool     `json:"durablePluginPresent"`
	DurableInstalled      bool     `json:"durableInstalled"`
	DurableStatus         string   `json:"durableStatus"`
	DurableOperationCount int      `json:"durableOperationCount"`
	MemoryPluginPresent   bool     `json:"memoryPluginPresent"`
	MemoryInstalled       bool     `json:"memoryInstalled"`
	MemoryStatus          string   `json:"memoryStatus"`
	SaveCount             int      `json:"saveCount"`
	ResourceEvents        []string `json:"resourceEvents"`
}

type stage9PluginsWriteCaseSpec struct {
	Name                 string
	Method               string
	RequestPaths         []string
	RequestBodies        []string
	InitialPluginPresent bool
	InitialInstalled     bool
	PersistFailure       bool
	Concurrent           bool
}

// TestStage9PluginsWriteFixtureMatchesCurrentGoOwner freezes the Go handler
// projection and catalog mutation observations without loading plugin files,
// starting plugin code, or using the production catalog store.
func TestStage9PluginsWriteFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 plugins-write fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/plugins-write.json",
	)
	want := stage9PluginsWriteFixture{
		Version: stage9PluginsWriteFixtureVersion,
		Cases:   make([]stage9PluginsWriteFixtureCase, 0),
	}
	for _, spec := range stage9PluginsWriteCaseSpecs() {
		want.Cases = append(want.Cases, runStage9PluginsWriteCase(t, spec))
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode plugins-write fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write plugins-write fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read plugins-write fixture: %v", err)
	}
	var got stage9PluginsWriteFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode plugins-write fixture: %v", err)
	}
	compactStage9PluginsWriteFixture(&got)
	compactStage9PluginsWriteFixture(&want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 plugins-write fixture drifted from the Go owner")
	}
}

func stage9PluginsWriteCaseSpecs() []stage9PluginsWriteCaseSpec {
	return []stage9PluginsWriteCaseSpec{
		{
			Name:                 "install-success",
			Method:               http.MethodPost,
			RequestPaths:         []string{"/api/v1/plugins/alpha/install"},
			RequestBodies:        []string{""},
			InitialPluginPresent: true,
		},
		{
			Name:                 "uninstall-success",
			Method:               http.MethodPost,
			RequestPaths:         []string{"/api/v1/plugins/alpha/uninstall"},
			RequestBodies:        []string{""},
			InitialPluginPresent: true,
			InitialInstalled:     true,
		},
		{
			Name:   "install-repeat",
			Method: http.MethodPost,
			RequestPaths: []string{
				"/api/v1/plugins/alpha/install",
				"/api/v1/plugins/alpha/install",
			},
			RequestBodies:        []string{"", ""},
			InitialPluginPresent: true,
			InitialInstalled:     true,
		},
		{
			Name:   "uninstall-repeat",
			Method: http.MethodPost,
			RequestPaths: []string{
				"/api/v1/plugins/alpha/uninstall",
				"/api/v1/plugins/alpha/uninstall",
			},
			RequestBodies:        []string{"", ""},
			InitialPluginPresent: true,
		},
		{
			Name:   "mixed-state",
			Method: http.MethodPost,
			RequestPaths: []string{
				"/api/v1/plugins/alpha/install",
				"/api/v1/plugins/alpha/uninstall",
				"/api/v1/plugins/alpha/install",
			},
			RequestBodies:        []string{"", "", ""},
			InitialPluginPresent: true,
		},
		{
			Name:                 "body-ignored",
			Method:               http.MethodPost,
			RequestPaths:         []string{"/api/v1/plugins/alpha/install"},
			RequestBodies:        []string{"not-json"},
			InitialPluginPresent: true,
		},
		{
			Name:                 "blank-encoded",
			Method:               http.MethodPost,
			RequestPaths:         []string{"/api/v1/plugins/%20/install"},
			RequestBodies:        []string{""},
			InitialPluginPresent: true,
		},
		{
			Name:          "missing-catalog-install",
			Method:        http.MethodPost,
			RequestPaths:  []string{"/api/v1/plugins/missing/install"},
			RequestBodies: []string{""},
		},
		{
			Name:          "missing-catalog-uninstall",
			Method:        http.MethodPost,
			RequestPaths:  []string{"/api/v1/plugins/missing/uninstall"},
			RequestBodies: []string{""},
		},
		{
			Name:                 "persist-failure-install",
			Method:               http.MethodPost,
			RequestPaths:         []string{"/api/v1/plugins/alpha/install"},
			RequestBodies:        []string{""},
			InitialPluginPresent: true,
			PersistFailure:       true,
		},
		{
			Name:                 "persist-failure-uninstall",
			Method:               http.MethodPost,
			RequestPaths:         []string{"/api/v1/plugins/alpha/uninstall"},
			RequestBodies:        []string{""},
			InitialPluginPresent: true,
			InitialInstalled:     true,
			PersistFailure:       true,
		},
		{
			Name:                 "concurrent-install",
			Method:               http.MethodPost,
			RequestPaths:         repeatedStage9PluginPath("/api/v1/plugins/alpha/install", 8),
			RequestBodies:        repeatedStage9PluginPath("", 8),
			InitialPluginPresent: true,
			Concurrent:           true,
		},
	}
}

func repeatedStage9PluginPath(path string, count int) []string {
	paths := make([]string, count)
	for index := range paths {
		paths[index] = path
	}
	return paths
}

func runStage9PluginsWriteCase(
	t *testing.T,
	spec stage9PluginsWriteCaseSpec,
) stage9PluginsWriteFixtureCase {
	t.Helper()
	repository := &stage9PluginsWriteRepository{
		snapshot: stage9PluginsWriteSeedSnapshot(spec),
	}
	if spec.PersistFailure {
		repository.saveErr = errors.New("catalog repository unavailable")
	}
	catalogService, err := catalog.New(repository, nil, "plugins")
	if err != nil {
		t.Fatalf("create catalog for %s: %v", spec.Name, err)
	}
	service := stratsrv.NewService(nil, catalogService, nil)
	router := gin.New()
	strategyapi.RegisterPluginRoutes(router.Group("/api/v1"), service)

	rawResponses := make([]stage9RawHTTPResponse, len(spec.RequestPaths))
	if spec.Concurrent {
		var waitGroup sync.WaitGroup
		for index := range spec.RequestPaths {
			waitGroup.Add(1)
			go func(index int) {
				defer waitGroup.Done()
				rawResponses[index] = stage9ServePluginWriteRequest(
					router,
					spec.Method,
					spec.RequestPaths[index],
					spec.RequestBodies[index],
				)
			}(index)
		}
		waitGroup.Wait()
		sort.Slice(rawResponses, func(left, right int) bool {
			return stage9RawOperationID(rawResponses[left].body) <
				stage9RawOperationID(rawResponses[right].body)
		})
	} else {
		for index := range spec.RequestPaths {
			rawResponses[index] = stage9ServePluginWriteRequest(
				router,
				spec.Method,
				spec.RequestPaths[index],
				spec.RequestBodies[index],
			)
		}
	}

	fixtureCase := stage9PluginsWriteFixtureCase{
		Name:                 spec.Name,
		Method:               spec.Method,
		RequestPaths:         append([]string(nil), spec.RequestPaths...),
		RequestBodies:        append([]string(nil), spec.RequestBodies...),
		InitialPluginPresent: spec.InitialPluginPresent,
		InitialInstalled:     spec.InitialInstalled,
		PersistFailure:       spec.PersistFailure,
		Concurrent:           spec.Concurrent,
		ExpectedStatuses:     make([]int, len(rawResponses)),
		Responses:            make([]json.RawMessage, 0, len(rawResponses)),
	}
	for index, response := range rawResponses {
		fixtureCase.ExpectedStatuses[index] = response.status
		label := fmt.Sprintf("%s-%d", spec.Name, index+1)
		fixtureCase.Responses = append(
			fixtureCase.Responses,
			normalizeStage9PluginWriteResponse(t, response.body, label),
		)
	}
	fixtureCase.ExpectedObservation = stage9PluginWriteObservation(repository, catalogService)
	return fixtureCase
}

type stage9RawHTTPResponse struct {
	status int
	body   []byte
}

func stage9ServePluginWriteRequest(
	handler http.Handler,
	method string,
	path string,
	body string,
) stage9RawHTTPResponse {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		context.Background(),
		method,
		path,
		bytes.NewBufferString(body),
	)
	handler.ServeHTTP(recorder, request)
	return stage9RawHTTPResponse{status: recorder.Code, body: recorder.Body.Bytes()}
}

func normalizeStage9PluginWriteResponse(
	t *testing.T,
	body []byte,
	operationLabel string,
) json.RawMessage {
	t.Helper()
	var envelope struct {
		OK    bool            `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode plugin mutation response: %v (%s)", err, body)
	}
	if envelope.Error != nil {
		result, err := json.Marshal(map[string]any{
			"ok": false,
			"error": map[string]string{
				"code":    envelope.Error.Code,
				"message": envelope.Error.Message,
			},
		})
		if err != nil {
			t.Fatalf("encode plugin mutation error: %v", err)
		}
		return result
	}
	var data struct {
		Operation stratsrv.PluginOperation `json:"operation"`
	}
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode plugin mutation operation: %v", err)
	}
	completedAt := "2026-08-22T00:00:00Z"
	data.Operation.OperationID = operationLabel
	data.Operation.StartedAt = completedAt
	data.Operation.UpdatedAt = completedAt
	data.Operation.CompletedAt = &completedAt
	result, err := json.Marshal(map[string]any{
		"ok": true,
		"data": map[string]any{
			"operation": data.Operation,
		},
	})
	if err != nil {
		t.Fatalf("encode plugin mutation response: %v", err)
	}
	return result
}

func stage9RawOperationID(body []byte) string {
	var envelope struct {
		Data struct {
			Operation struct {
				OperationID string `json:"operationId"`
			} `json:"operation"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return envelope.Data.Operation.OperationID
}

func stage9PluginsWriteSeedSnapshot(spec stage9PluginsWriteCaseSpec) catalog.Snapshot {
	snapshot := catalog.Snapshot{
		TargetDir:  "plugins",
		Plugins:    []catalog.ManagedPlugin{},
		Operations: []stratsrv.PluginOperation{},
	}
	if !spec.InitialPluginPresent {
		return snapshot
	}
	status := "NOT_INSTALLED"
	if spec.InitialInstalled {
		status = "INSTALLED"
	}
	snapshot.Plugins = []catalog.ManagedPlugin{{
		Descriptor: stratsrv.PluginDescriptor{
			ID:          "alpha",
			Type:        "strategy-go-plugin",
			DisplayName: "Alpha Strategy",
			Version:     "1.2.3",
		},
		Installation: stratsrv.PluginInstallation{
			Status:      status,
			Installed:   spec.InitialInstalled,
			TargetDir:   "plugins",
			InstallPath: "plugins/alpha.so",
			MarkerPath:  "plugins/alpha.json",
		},
	}}
	return snapshot
}

func stage9PluginWriteObservation(
	repository *stage9PluginsWriteRepository,
	service *catalog.Service,
) stage9PluginsWriteObservation {
	durable := repository.durableSnapshot()
	memory := service.PluginCatalog()
	observation := stage9PluginsWriteObservation{
		DurableOperationCount: len(durable.Operations),
		SaveCount:             repository.saveCount(),
		ResourceEvents:        []string{},
	}
	if plugin, ok := stage9FindManagedPlugin(durable, "alpha"); ok {
		observation.DurablePluginPresent = true
		observation.DurableInstalled = plugin.Installation.Installed
		observation.DurableStatus = plugin.Installation.Status
	}
	for _, plugin := range memory.Plugins {
		if plugin.Descriptor.ID != "alpha" {
			continue
		}
		observation.MemoryPluginPresent = true
		observation.MemoryInstalled = plugin.Installation.Installed
		observation.MemoryStatus = plugin.Installation.Status
		break
	}
	return observation
}

func stage9FindManagedPlugin(
	snapshot catalog.Snapshot,
	pluginID string,
) (catalog.ManagedPlugin, bool) {
	for _, plugin := range snapshot.Plugins {
		if plugin.Descriptor.ID == pluginID {
			return plugin, true
		}
	}
	return catalog.ManagedPlugin{}, false
}

func compactStage9PluginsWriteFixture(fixture *stage9PluginsWriteFixture) {
	for index := range fixture.Cases {
		for responseIndex := range fixture.Cases[index].Responses {
			var compacted bytes.Buffer
			if json.Compact(&compacted, fixture.Cases[index].Responses[responseIndex]) == nil {
				fixture.Cases[index].Responses[responseIndex] = compacted.Bytes()
			}
		}
	}
}

type stage9PluginsWriteRepository struct {
	mu        sync.Mutex
	snapshot  catalog.Snapshot
	saveErr   error
	saveCalls int
}

func (r *stage9PluginsWriteRepository) Load(context.Context) (catalog.Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneStage9PluginSnapshot(r.snapshot), nil
}

func (r *stage9PluginsWriteRepository) Save(_ context.Context, snapshot catalog.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saveCalls++
	if r.saveErr != nil {
		return r.saveErr
	}
	r.snapshot = cloneStage9PluginSnapshot(snapshot)
	return nil
}

func (r *stage9PluginsWriteRepository) durableSnapshot() catalog.Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneStage9PluginSnapshot(r.snapshot)
}

func (r *stage9PluginsWriteRepository) saveCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveCalls
}

func cloneStage9PluginSnapshot(snapshot catalog.Snapshot) catalog.Snapshot {
	contents, err := json.Marshal(snapshot)
	if err != nil {
		panic(err)
	}
	var clone catalog.Snapshot
	if err := json.Unmarshal(contents, &clone); err != nil {
		panic(err)
	}
	return clone
}
