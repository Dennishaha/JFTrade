package rustmigration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	strategyapi "github.com/jftrade/jftrade-main/internal/api/strategy"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	catalog "github.com/jftrade/jftrade-main/internal/strategy/catalog"
)

const stage9PluginsReadFixtureVersion = "stage9.plugins-read.v1"

type stage9PluginsReadCase struct {
	Name           string            `json:"name"`
	Method         string            `json:"method"`
	RequestPath    string            `json:"requestPath"`
	ExpectedStatus int               `json:"expectedStatus"`
	Headers        map[string]string `json:"headers,omitempty"`
	Data           json.RawMessage   `json:"data,omitempty"`
	ErrorCode      string            `json:"errorCode,omitempty"`
	ErrorMessage   string            `json:"errorMessage,omitempty"`
}

type stage9PluginsReadFixture struct {
	Version string                  `json:"version"`
	Cases   []stage9PluginsReadCase `json:"cases"`
}

type stage9PluginsReadCaseSpec struct {
	name     string
	path     string
	snapshot catalog.Snapshot
}

// TestStage9PluginsReadFixtureMatchesCurrentGoOwner freezes the catalog and
// operation projections together. The repository is an in-memory fixture;
// the test never loads plugin files, starts plugin code, or mutates a real
// catalog store.
func TestStage9PluginsReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 plugins fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/plugins-read.json",
	)
	want := stage9PluginsReadFixture{
		Version: stage9PluginsReadFixtureVersion,
		Cases:   make([]stage9PluginsReadCase, 0, len(stage9PluginsReadCaseSpecs())),
	}
	for _, testCase := range stage9PluginsReadCaseSpecs() {
		repository := &stage9PluginsReadRepository{snapshot: testCase.snapshot}
		catalogService, err := catalog.New(repository, nil, "plugins")
		if err != nil {
			t.Fatalf("create plugin catalog for %s: %v", testCase.name, err)
		}
		service := stratsrv.NewService(nil, catalogService, nil)
		router := gin.New()
		strategyapi.RegisterPluginRoutes(router.Group("/api/v1"), service)

		recorder := httptest.NewRecorder()
		request := stage9PluginsReadRequest(t.Context(), testCase.path)
		router.ServeHTTP(recorder, request)
		entry := stage9PluginsReadCase{
			Name:           testCase.name,
			Method:         http.MethodGet,
			RequestPath:    testCase.path,
			ExpectedStatus: recorder.Code,
			Headers:        pluginReadHeaders(recorder.Header()),
		}
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v (%s)", testCase.name, err, recorder.Body.String())
		}
		if envelope.Error != nil {
			entry.ErrorCode = envelope.Error.Code
			entry.ErrorMessage = envelope.Error.Message
		} else {
			entry.Data = normalizePluginReadData(testCase.path, envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode plugins fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write plugins fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read plugins fixture: %v", err)
	}
	var got stage9PluginsReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode plugins fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactPluginJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactPluginJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 plugins fixture drifted from the Go owner")
	}
}

func stage9PluginsReadRequest(ctx context.Context, path string) *http.Request {
	if _, err := url.Parse(path); err == nil {
		return httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	}
	request := &http.Request{
		Method:     http.MethodGet,
		URL:        &url.URL{Path: path, RawPath: path},
		RequestURI: path,
		Header:     make(http.Header),
		Body:       http.NoBody,
		Host:       "example.com",
	}
	return request.WithContext(ctx)
}

func stage9PluginsReadCaseSpecs() []stage9PluginsReadCaseSpec {
	return []stage9PluginsReadCaseSpec{
		{name: "catalog", path: "/api/v1/plugins", snapshot: stage9PluginsReadSnapshot()},
		{
			name:     "catalog-empty",
			path:     "/api/v1/plugins",
			snapshot: catalog.Snapshot{TargetDir: "plugins", Plugins: []catalog.ManagedPlugin{}},
		},
		{
			name:     "operation",
			path:     "/api/v1/plugins/operations/op-alpha",
			snapshot: stage9PluginsReadSnapshot(),
		},
		{
			name:     "operation-missing",
			path:     "/api/v1/plugins/operations/missing",
			snapshot: stage9PluginsReadSnapshot(),
		},
		{
			name:     "operation-blank-encoded",
			path:     "/api/v1/plugins/operations/%20",
			snapshot: stage9PluginsReadSnapshot(),
		},
		{
			name:     "operation-invalid-escape",
			path:     "/api/v1/plugins/operations/%ZZ",
			snapshot: stage9PluginsReadSnapshot(),
		},
	}
}

func stage9PluginsReadSnapshot() catalog.Snapshot {
	operation := stage9PluginOperation()
	return catalog.Snapshot{
		TargetDir: "plugins",
		Plugins: []catalog.ManagedPlugin{
			{
				Descriptor: stratsrv.PluginDescriptor{
					ID:          "alpha",
					Type:        "strategy-go-plugin",
					DisplayName: "Alpha Strategy",
					Version:     "1.2.3",
					Description: "fixture strategy plugin",
					Keywords:    []string{"alpha", "trend"},
				},
				Installation: stratsrv.PluginInstallation{
					Status:        "INSTALLED",
					Installed:     true,
					TargetDir:     "plugins",
					InstallPath:   "plugins/alpha.so",
					MarkerPath:    "plugins/alpha.json",
					LastOperation: &operation,
				},
			},
			{
				Descriptor: stratsrv.PluginDescriptor{ID: "beta"},
				Installation: stratsrv.PluginInstallation{
					TargetDir:   "plugins",
					InstallPath: "plugins/beta.so",
					MarkerPath:  "plugins/beta.json",
				},
			},
		},
		Operations: []stratsrv.PluginOperation{operation},
	}
}

type stage9PluginsReadRepository struct {
	snapshot catalog.Snapshot
}

func (r *stage9PluginsReadRepository) Load(context.Context) (catalog.Snapshot, error) {
	return r.snapshot, nil
}

func (r *stage9PluginsReadRepository) Save(_ context.Context, snapshot catalog.Snapshot) error {
	r.snapshot = snapshot
	return nil
}

func stage9PluginOperation() stratsrv.PluginOperation {
	completedAt := "2026-08-21T04:00:02Z"
	return stratsrv.PluginOperation{
		OperationID: "op-alpha",
		PluginID:    "alpha",
		Status:      "SUCCEEDED",
		Phase:       "installed",
		Progress:    100,
		Message:     "plugin metadata installed",
		TargetDir:   "plugins",
		InstallPath: "plugins/alpha.so",
		StartedAt:   "2026-08-21T04:00:00Z",
		UpdatedAt:   "2026-08-21T04:00:02Z",
		CompletedAt: &completedAt,
	}
}

func pluginReadHeaders(header http.Header) map[string]string {
	contentType := header.Get("Content-Type")
	if contentType == "" {
		return nil
	}
	return map[string]string{"Content-Type": contentType}
}

func normalizePluginReadData(path string, data json.RawMessage) json.RawMessage {
	if path != "/api/v1/plugins" {
		return data
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	if plugins, ok := value["plugins"].([]any); ok {
		for _, item := range plugins {
			plugin, ok := item.(map[string]any)
			if !ok {
				continue
			}
			compatibility, ok := plugin["compatibility"].(map[string]any)
			if !ok {
				continue
			}
			compatibility["supported"] = true
			compatibility["requiresRebuild"] = false
			compatibility["host"] = map[string]any{
				"jftradeVersion": "fixture",
				"goVersion":      "go1.26.6",
				"goos":           "fixture",
				"goarch":         "fixture",
				"buildMode":      "plugin",
			}
		}
	}
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

func compactPluginJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, data); err != nil {
		return data
	}
	return compacted.Bytes()
}
