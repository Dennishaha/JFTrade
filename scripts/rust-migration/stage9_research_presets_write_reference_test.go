package rustmigration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	researchapi "github.com/jftrade/jftrade-main/internal/api/research"
	domain "github.com/jftrade/jftrade-main/internal/research"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

const (
	stage9ResearchPresetsWriteFixtureVersion = "stage9.research-presets-write.v1"
	stage9ResearchPresetsWriteTimestamp      = "2026-08-22T04:00:00Z"
)

type stage9ResearchPresetsWriteFixture struct {
	Version   string                                  `json:"version"`
	Timestamp string                                  `json:"timestamp"`
	Cases     []stage9ResearchPresetsWriteFixtureCase `json:"cases"`
}

type stage9ResearchPresetsWriteFixtureCase struct {
	Name                string                                `json:"name"`
	Requests            []stage9ResearchPresetsWriteRequest   `json:"requests"`
	Expected            []stage9ResearchPresetsWriteExpected  `json:"expected"`
	ExpectedObservation stage9ResearchPresetsWriteObservation `json:"expectedObservation"`
	Concurrent          bool                                  `json:"concurrent,omitempty"`
}

type stage9ResearchPresetsWriteRequest struct {
	Method   string  `json:"method"`
	Path     string  `json:"path"`
	Body     *string `json:"body,omitempty"`
	Context  string  `json:"context,omitempty"`
	PortCall bool    `json:"portCall"`
}

type stage9ResearchPresetsWriteExpected struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	Envelope json.RawMessage   `json:"envelope"`
	PortCall bool              `json:"portCall"`
}

type stage9ResearchPresetsWriteObservation struct {
	Presets     []domain.ScreenPreset `json:"presets"`
	CreateCalls int                   `json:"createCalls"`
	GetCalls    int                   `json:"getCalls"`
	UpdateCalls int                   `json:"updateCalls"`
	DeleteCalls int                   `json:"deleteCalls"`
}

type stage9ResearchPresetsWriteCaseSpec struct {
	Name       string
	Requests   []stage9ResearchPresetsWriteRequest
	Seeds      []stage9ResearchPresetSeed
	NextIDs    []string
	Mode       string
	Concurrent bool
}

type stage9ResearchPresetSeed struct {
	ID       string
	Name     string
	Market   string
	Revision int64
}

// TestStage9ResearchPresetsWriteFixtureMatchesCurrentGoOwner freezes the
// three preset mutation routes using the real Go service and Gin handlers,
// with a deterministic in-memory repository instead of production SQLite.
func TestStage9ResearchPresetsWriteFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 research presets write fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/research-presets-write.json",
	)
	want := stage9ResearchPresetsWriteFixture{
		Version:   stage9ResearchPresetsWriteFixtureVersion,
		Timestamp: stage9ResearchPresetsWriteTimestamp,
		Cases:     make([]stage9ResearchPresetsWriteFixtureCase, 0),
	}
	for _, spec := range stage9ResearchPresetsWriteCaseSpecs() {
		want.Cases = append(want.Cases, runStage9ResearchPresetsWriteCase(t, spec))
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode research presets write fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write research presets write fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read research presets write fixture: %v", err)
	}
	var got stage9ResearchPresetsWriteFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode research presets write fixture: %v", err)
	}
	compactStage9ResearchPresetsWriteFixture(&got)
	compactStage9ResearchPresetsWriteFixture(&want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 research presets write fixture drifted from the Go owner")
	}
}

func stage9ResearchPresetsWriteCaseSpecs() []stage9ResearchPresetsWriteCaseSpec {
	return []stage9ResearchPresetsWriteCaseSpec{
		{
			Name: "create-success",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPost, "/api/v1/research/screens/presets", stage9CreatePresetBody("  Value  ", "US")),
			},
			NextIDs: []string{"rsp-create-success"},
		},
		{
			Name: "create-empty-input",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPost, "/api/v1/research/screens/presets", "{}"),
			},
		},
		{
			Name: "create-malformed-unknown-field",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPost, "/api/v1/research/screens/presets", `{"name":"Value","definition":{"brokerId":"futu"},"unknown":true}`),
			},
		},
		{
			Name: "create-unavailable",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPost, "/api/v1/research/screens/presets", stage9CreatePresetBody("Value", "US")),
			},
			Mode: "create-unavailable",
		},
		{
			Name: "create-failure-rolls-back",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPost, "/api/v1/research/screens/presets", stage9CreatePresetBody("New", "US")),
			},
			Seeds: []stage9ResearchPresetSeed{{ID: "rsp-existing", Name: "Existing", Market: "US", Revision: 1}},
			Mode:  "create-failure",
		},
		{
			Name: "create-duplicate-name",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPost, "/api/v1/research/screens/presets", stage9CreatePresetBody(" Value ", "US")),
			},
			Seeds: []stage9ResearchPresetSeed{{ID: "rsp-existing", Name: "Value", Market: "US", Revision: 1}},
		},
		{
			Name: "create-concurrent-duplicate",
			Requests: stage9RepeatedResearchPresetWriteRequest(
				http.MethodPost,
				"/api/v1/research/screens/presets",
				stage9CreatePresetBody("Concurrent", "US"),
				8,
			),
			NextIDs:    []string{"rsp-concurrent"},
			Concurrent: true,
		},
		{
			Name: "patch-name-success",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPatch, "/api/v1/research/screens/presets/rsp-patch", `{"name":" Updated ","expectedRevision":1}`),
			},
			Seeds: []stage9ResearchPresetSeed{{ID: "rsp-patch", Name: "Original", Market: "US", Revision: 1}},
		},
		{
			Name: "patch-empty-change",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPatch, "/api/v1/research/screens/presets/rsp-patch", `{"expectedRevision":1}`),
			},
			Seeds: []stage9ResearchPresetSeed{{ID: "rsp-patch", Name: "Original", Market: "US", Revision: 1}},
		},
		{
			Name: "patch-blank-percent-decoded-id",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPatch, "/api/v1/research/screens/presets/%20", `{"name":"New","expectedRevision":1}`),
			},
		},
		{
			Name: "patch-not-found",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPatch, "/api/v1/research/screens/presets/missing", `{"name":"New","expectedRevision":1}`),
			},
		},
		{
			Name: "patch-revision-conflict",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPatch, "/api/v1/research/screens/presets/rsp-patch", `{"name":"Stale","expectedRevision":1}`),
			},
			Seeds: []stage9ResearchPresetSeed{{ID: "rsp-patch", Name: "Original", Market: "US", Revision: 2}},
		},
		{
			Name: "patch-unavailable",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPatch, "/api/v1/research/screens/presets/rsp-patch", `{"name":"New","expectedRevision":1}`),
			},
			Seeds: []stage9ResearchPresetSeed{{ID: "rsp-patch", Name: "Original", Market: "US", Revision: 1}},
			Mode:  "update-unavailable",
		},
		{
			Name: "patch-invalid-definition",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPatch, "/api/v1/research/screens/presets/rsp-patch", `{"definition":{"brokerId":"futu","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":1},"expectedRevision":1}`),
			},
			Seeds: []stage9ResearchPresetSeed{{ID: "rsp-patch", Name: "Original", Market: "US", Revision: 1}},
		},
		{
			Name: "patch-failure-rolls-back-and-recovers",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodPatch, "/api/v1/research/screens/presets/rsp-patch", `{"name":"Retry","expectedRevision":1}`),
				stage9ResearchPresetWriteRequest(http.MethodPatch, "/api/v1/research/screens/presets/rsp-patch", `{"name":"Retry","expectedRevision":1}`),
			},
			Seeds: []stage9ResearchPresetSeed{{ID: "rsp-patch", Name: "Original", Market: "US", Revision: 1}},
			Mode:  "update-failure-once",
		},
		{
			Name: "patch-concurrent-revision-fence",
			Requests: stage9RepeatedResearchPresetWriteRequest(
				http.MethodPatch,
				"/api/v1/research/screens/presets/rsp-patch",
				`{"name":"Concurrent","expectedRevision":1}`,
				8,
			),
			Seeds:      []stage9ResearchPresetSeed{{ID: "rsp-patch", Name: "Original", Market: "US", Revision: 1}},
			Concurrent: true,
		},
		{
			Name: "delete-success-and-repeat",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetWriteRequest(http.MethodDelete, "/api/v1/research/screens/presets/rsp-delete", "{"),
				stage9ResearchPresetWriteRequest(http.MethodDelete, "/api/v1/research/screens/presets/rsp-delete", "{"),
			},
			Seeds: []stage9ResearchPresetSeed{{ID: "rsp-delete", Name: "Delete me", Market: "US", Revision: 1}},
		},
		{
			Name: "delete-not-found",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetRequestWithoutBody(http.MethodDelete, "/api/v1/research/screens/presets/missing"),
			},
		},
		{
			Name: "delete-unavailable",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetRequestWithoutBody(http.MethodDelete, "/api/v1/research/screens/presets/rsp-delete"),
			},
			Seeds: []stage9ResearchPresetSeed{{ID: "rsp-delete", Name: "Delete me", Market: "US", Revision: 1}},
			Mode:  "delete-unavailable",
		},
		{
			Name: "delete-failure-rolls-back",
			Requests: []stage9ResearchPresetsWriteRequest{
				stage9ResearchPresetRequestWithoutBody(http.MethodDelete, "/api/v1/research/screens/presets/rsp-delete"),
			},
			Seeds: []stage9ResearchPresetSeed{{ID: "rsp-delete", Name: "Delete me", Market: "US", Revision: 1}},
			Mode:  "delete-failure",
		},
		{
			Name: "cancel-create-recovers",
			Requests: []stage9ResearchPresetsWriteRequest{
				{
					Method: http.MethodPost, Path: "/api/v1/research/screens/presets",
					Body: stage9StringPointer(stage9CreatePresetBody("Cancelled", "US")), Context: "canceled",
				},
				stage9ResearchPresetWriteRequest(http.MethodPost, "/api/v1/research/screens/presets", stage9CreatePresetBody("Recovered", "US")),
			},
			NextIDs: []string{"rsp-recovered"},
		},
		{
			Name: "timeout-update-fails-closed",
			Requests: []stage9ResearchPresetsWriteRequest{
				{
					Method: http.MethodPatch, Path: "/api/v1/research/screens/presets/rsp-timeout",
					Body: stage9StringPointer(`{"name":"Too late","expectedRevision":1}`), Context: "deadline-exceeded",
				},
			},
			Seeds: []stage9ResearchPresetSeed{{ID: "rsp-timeout", Name: "Original", Market: "US", Revision: 1}},
		},
	}
}

func runStage9ResearchPresetsWriteCase(
	t *testing.T,
	spec stage9ResearchPresetsWriteCaseSpec,
) stage9ResearchPresetsWriteFixtureCase {
	t.Helper()
	repository := newStage9ResearchPresetsWriteRepository(spec)
	router := gin.New()
	researchapi.RegisterRoutes(router.Group("/api/v1"), domain.NewService(repository))

	responses := make([]stage9ResearchPresetsWriteExpected, len(spec.Requests))
	for index := range spec.Requests {
		spec.Requests[index].PortCall = stage9ResearchPresetsWriteServiceDispatchable(spec.Requests[index])
	}
	if spec.Concurrent {
		var waitGroup sync.WaitGroup
		for index := range spec.Requests {
			waitGroup.Add(1)
			go func(index int) {
				defer waitGroup.Done()
				responses[index] = serveStage9ResearchPresetsWriteRequest(t, router, spec.Requests[index])
			}(index)
		}
		waitGroup.Wait()
		sort.SliceStable(responses, func(left, right int) bool {
			return stage9ResearchPresetsWriteResponseSortKey(responses[left]) <
				stage9ResearchPresetsWriteResponseSortKey(responses[right])
		})
	} else {
		for index, request := range spec.Requests {
			responses[index] = serveStage9ResearchPresetsWriteRequest(t, router, request)
		}
	}

	return stage9ResearchPresetsWriteFixtureCase{
		Name:                spec.Name,
		Requests:            spec.Requests,
		Expected:            responses,
		ExpectedObservation: repository.observation(),
		Concurrent:          spec.Concurrent,
	}
}

func serveStage9ResearchPresetsWriteRequest(
	t *testing.T,
	router http.Handler,
	request stage9ResearchPresetsWriteRequest,
) stage9ResearchPresetsWriteExpected {
	t.Helper()
	var body io.Reader
	if request.Body != nil {
		body = bytes.NewBufferString(*request.Body)
	}
	ctx := t.Context()
	var cancel context.CancelFunc
	switch request.Context {
	case "canceled":
		ctx, cancel = context.WithCancel(ctx)
		cancel()
	case "deadline-exceeded":
		ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
		cancel()
	}
	if cancel != nil {
		defer cancel()
	}
	httpRequest := httptest.NewRequestWithContext(ctx, request.Method, request.Path, body)
	if request.Body != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httpRequest)

	var envelope map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s response: %v; body=%s", request.Path, err, recorder.Body.String())
	}
	runtimeTimestamp, ok := envelope["timestamp"].(string)
	if !ok {
		t.Fatalf("%s response has no timestamp: %s", request.Path, recorder.Body.String())
	}
	if _, err := time.Parse(time.RFC3339Nano, runtimeTimestamp); err != nil {
		t.Fatalf("%s response timestamp %q is not RFC3339Nano: %v", request.Path, runtimeTimestamp, err)
	}
	envelope["timestamp"] = stage9ResearchPresetsWriteTimestamp
	contents, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode %s response: %v", request.Path, err)
	}
	headers := make(map[string]string)
	for key, values := range recorder.Header() {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return stage9ResearchPresetsWriteExpected{
		Status:   recorder.Code,
		Headers:  headers,
		Envelope: contents,
		PortCall: stage9ResearchPresetsWriteServiceDispatchable(request),
	}
}

func stage9ResearchPresetsWriteResponseSortKey(response stage9ResearchPresetsWriteExpected) string {
	return fmt.Sprintf("%03d:%s", response.Status, string(response.Envelope))
}

func stage9ResearchPresetsWriteServiceDispatchable(request stage9ResearchPresetsWriteRequest) bool {
	path := request.Path
	if index := strings.IndexByte(path, '?'); index >= 0 {
		path = path[:index]
	}
	switch request.Method {
	case http.MethodPost:
		if path != "/api/v1/research/screens/presets" {
			return false
		}
		var input domain.CreateScreenPresetInput
		return stage9StrictPresetBodyBinds(request.Body, &input)
	case http.MethodPatch:
		if !stage9ResearchPresetIDPath(path) {
			return false
		}
		var input domain.UpdateScreenPresetInput
		return stage9StrictPresetBodyBinds(request.Body, &input)
	case http.MethodDelete:
		return stage9ResearchPresetIDPath(path)
	default:
		return false
	}
}

func stage9ResearchPresetIDPath(path string) bool {
	const prefix = "/api/v1/research/screens/presets/"
	id := strings.TrimPrefix(path, prefix)
	return id != path && id != "" && !strings.Contains(id, "/")
}

func stage9StrictPresetBodyBinds(body *string, target any) bool {
	if body == nil {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(*body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	var extra any
	return decoder.Decode(&extra) == io.EOF
}

type stage9ResearchPresetsWriteRepository struct {
	mu                 sync.Mutex
	presets            map[string]domain.ScreenPreset
	nextIDs            []string
	mode               string
	failureRemaining   int
	createCalls        int
	getCalls           int
	updateCalls        int
	deleteCalls        int
	concurrentGetCount int
	concurrentGetTotal int
	concurrentGetReady chan struct{}
}

func newStage9ResearchPresetsWriteRepository(spec stage9ResearchPresetsWriteCaseSpec) *stage9ResearchPresetsWriteRepository {
	repository := &stage9ResearchPresetsWriteRepository{
		presets: make(map[string]domain.ScreenPreset),
		nextIDs: append([]string(nil), spec.NextIDs...),
		mode:    spec.Mode,
	}
	for _, seed := range spec.Seeds {
		repository.presets[seed.ID] = stage9ResearchPresetForWrite(
			seed.ID, seed.Name, seed.Market, seed.Revision,
		)
	}
	if spec.Concurrent && spec.Mode == "" && len(spec.Requests) > 1 &&
		len(spec.Seeds) > 0 && strings.HasPrefix(spec.Requests[0].Method, http.MethodPatch) {
		repository.concurrentGetTotal = len(spec.Requests)
		repository.concurrentGetReady = make(chan struct{})
	}
	return repository
}

func (r *stage9ResearchPresetsWriteRepository) ListScreenPresets(context.Context) ([]domain.ScreenPreset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sortedPresetsLocked(), nil
}

func (r *stage9ResearchPresetsWriteRepository) GetScreenPreset(
	ctx context.Context,
	id string,
) (domain.ScreenPreset, error) {
	if err := ctx.Err(); err != nil {
		return domain.ScreenPreset{}, err
	}
	r.mu.Lock()
	r.getCalls++
	preset, ok := r.presets[strings.TrimSpace(id)]
	ready := r.concurrentGetReady
	if ready != nil {
		r.concurrentGetCount++
		if r.concurrentGetCount == r.concurrentGetTotal {
			close(ready)
		}
	}
	r.mu.Unlock()
	if ready != nil {
		<-ready
	}
	if err := ctx.Err(); err != nil {
		return domain.ScreenPreset{}, err
	}
	if !ok {
		return domain.ScreenPreset{}, domain.ErrNotFound
	}
	if r.mode == "update-unavailable" {
		return domain.ScreenPreset{}, domain.ErrUnavailable
	}
	return preset, nil
}

func (r *stage9ResearchPresetsWriteRepository) CreateScreenPreset(
	ctx context.Context,
	name string,
	definition broker.ScreenDefinitionV2,
	schemaVersion int,
) (domain.ScreenPreset, error) {
	if err := ctx.Err(); err != nil {
		return domain.ScreenPreset{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createCalls++
	if r.mode == "create-unavailable" {
		return domain.ScreenPreset{}, domain.ErrUnavailable
	}
	if r.mode == "create-failure" {
		return domain.ScreenPreset{}, errors.New("database write failed")
	}
	nameKey := stage9ResearchPresetNameKey(name)
	for _, preset := range r.presets {
		if stage9ResearchPresetNameKey(preset.Name) == nameKey {
			return domain.ScreenPreset{}, fmt.Errorf(
				"%w: UNIQUE constraint failed: research_screen_presets.name_key",
				domain.ErrConflict,
			)
		}
	}
	id := fmt.Sprintf("rsp-created-%d", r.createCalls)
	if len(r.nextIDs) > 0 {
		id, r.nextIDs = r.nextIDs[0], r.nextIDs[1:]
	}
	createdAt := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	preset := domain.ScreenPreset{
		ID: id, Name: name, QuerySchemaVersion: schemaVersion, Definition: definition,
		Revision: 1, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	r.presets[id] = preset
	return preset, nil
}

func (r *stage9ResearchPresetsWriteRepository) UpdateScreenPreset(
	ctx context.Context,
	id string,
	name string,
	definition broker.ScreenDefinitionV2,
	schemaVersion int,
	expectedRevision int64,
) (domain.ScreenPreset, error) {
	if err := ctx.Err(); err != nil {
		return domain.ScreenPreset{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updateCalls++
	if r.mode == "update-failure-once" && r.failureRemaining == 0 {
		r.failureRemaining++
		return domain.ScreenPreset{}, errors.New("database write failed")
	}
	if r.mode == "update-unavailable" {
		return domain.ScreenPreset{}, domain.ErrUnavailable
	}
	id = strings.TrimSpace(id)
	preset, ok := r.presets[id]
	if !ok {
		return domain.ScreenPreset{}, domain.ErrNotFound
	}
	if preset.Revision != expectedRevision {
		return domain.ScreenPreset{}, domain.ErrConflict
	}
	preset.Name = name
	preset.Definition = definition
	preset.QuerySchemaVersion = schemaVersion
	preset.Revision++
	preset.UpdatedAt = time.Date(2026, 8, 22, 0, 1, 0, 0, time.UTC)
	r.presets[id] = preset
	return preset, nil
}

func (r *stage9ResearchPresetsWriteRepository) DeleteScreenPreset(
	ctx context.Context,
	id string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleteCalls++
	if r.mode == "delete-unavailable" {
		return domain.ErrUnavailable
	}
	if r.mode == "delete-failure" {
		return errors.New("database delete failed")
	}
	id = strings.TrimSpace(id)
	if _, ok := r.presets[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.presets, id)
	return nil
}

func (r *stage9ResearchPresetsWriteRepository) observation() stage9ResearchPresetsWriteObservation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return stage9ResearchPresetsWriteObservation{
		Presets:     r.sortedPresetsLocked(),
		CreateCalls: r.createCalls,
		GetCalls:    r.getCalls,
		UpdateCalls: r.updateCalls,
		DeleteCalls: r.deleteCalls,
	}
}

func (r *stage9ResearchPresetsWriteRepository) sortedPresetsLocked() []domain.ScreenPreset {
	items := make([]domain.ScreenPreset, 0, len(r.presets))
	for _, preset := range r.presets {
		items = append(items, preset)
	}
	sort.Slice(items, func(left, right int) bool { return items[left].ID < items[right].ID })
	return items
}

func stage9ResearchPresetWriteRequest(method, path, body string) stage9ResearchPresetsWriteRequest {
	return stage9ResearchPresetsWriteRequest{
		Method: method,
		Path:   path,
		Body:   stage9StringPointer(body),
	}
}

func stage9ResearchPresetRequestWithoutBody(method, path string) stage9ResearchPresetsWriteRequest {
	return stage9ResearchPresetsWriteRequest{Method: method, Path: path}
}

func stage9RepeatedResearchPresetWriteRequest(
	method, path, body string,
	count int,
) []stage9ResearchPresetsWriteRequest {
	requests := make([]stage9ResearchPresetsWriteRequest, count)
	for index := range requests {
		requests[index] = stage9ResearchPresetWriteRequest(method, path, body)
	}
	return requests
}

func stage9StringPointer(value string) *string {
	return &value
}

func stage9CreatePresetBody(name, market string) string {
	return fmt.Sprintf(
		`{"name":%q,"definition":{"brokerId":"futu","market":%q,"catalogVersion":"%s","querySchemaVersion":2,"columns":[{"columnId":"price","factor":{"instanceId":"price","factorKey":"simple.price"}}]}}`,
		name, market, researchscreen.CatalogVersion,
	)
}

func stage9ResearchPresetForWrite(id, name, market string, revision int64) domain.ScreenPreset {
	createdAt := time.Date(2026, 8, 15, 20, 0, 0, 0, time.UTC)
	return domain.ScreenPreset{
		ID: id, Name: name, QuerySchemaVersion: domain.QuerySchemaVersion,
		Definition: stage9ResearchPresetDefinition(market), Revision: revision,
		CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Minute),
	}
}

func stage9ResearchPresetDefinition(market string) broker.ScreenDefinitionV2 {
	return broker.ScreenDefinitionV2{
		BrokerID: "futu", Market: market, Pool: broker.ResearchScreenPool{},
		Columns: []broker.ScreenColumn{{
			ID: "price", Factor: broker.FactorRef{InstanceID: "price", FactorKey: "simple.price"},
		}},
		CatalogVersion:     researchscreen.CatalogVersion,
		QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
	}
}

func stage9ResearchPresetNameKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func compactStage9ResearchPresetsWriteFixture(fixture *stage9ResearchPresetsWriteFixture) {
	for caseIndex := range fixture.Cases {
		for expectedIndex := range fixture.Cases[caseIndex].Expected {
			raw := fixture.Cases[caseIndex].Expected[expectedIndex].Envelope
			var value any
			if err := json.Unmarshal(raw, &value); err == nil {
				if compact, err := json.Marshal(value); err == nil {
					fixture.Cases[caseIndex].Expected[expectedIndex].Envelope = compact
				}
			}
		}
	}
}
