package rustmigration

import (
	"bytes"
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
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	catalog "github.com/jftrade/jftrade-main/internal/strategy/catalog"
)

const stage9PluginsReadFixtureVersion = "stage9.plugins-read.v1"

type stage9PluginsReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9PluginsReadFixture struct {
	Version string                  `json:"version"`
	Cases   []stage9PluginsReadCase `json:"cases"`
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
	operation := stage9PluginOperation()
	repository := &stage9PluginsReadRepository{snapshot: catalog.Snapshot{
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
	}}
	catalogService, err := catalog.New(repository, nil, "plugins")
	if err != nil {
		t.Fatalf("create plugin catalog: %v", err)
	}
	service := stratsrv.NewService(nil, catalogService, nil)
	router := gin.New()
	strategyapi.RegisterPluginRoutes(router.Group("/api/v1"), service)

	cases := []struct {
		name string
		path string
	}{
		{name: "catalog", path: "/api/v1/plugins"},
		{name: "operation", path: "/api/v1/plugins/operations/op-alpha"},
		{name: "operation-missing", path: "/api/v1/plugins/operations/missing"},
		{name: "operation-blank-encoded", path: "/api/v1/plugins/operations/%20"},
	}
	want := stage9PluginsReadFixture{
		Version: stage9PluginsReadFixtureVersion,
		Cases:   make([]stage9PluginsReadCase, 0, len(cases)),
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		entry := stage9PluginsReadCase{
			Name:           testCase.name,
			Method:         http.MethodGet,
			RequestPath:    testCase.path,
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
