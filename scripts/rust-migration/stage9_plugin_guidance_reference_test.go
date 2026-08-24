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
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	catalog "github.com/jftrade/jftrade-main/internal/strategy/catalog"
)

const stage9PluginUninstallGuidanceVersion = "stage9.plugin-uninstall-guidance.v1"

type stage9PluginUninstallGuidanceCase struct {
	Name           string                            `json:"name"`
	Method         string                            `json:"method"`
	RequestPath    string                            `json:"requestPath"`
	ExpectedStatus int                               `json:"expectedStatus"`
	Response       *stratsrv.PluginUninstallGuidance `json:"response,omitempty"`
	ErrorCode      string                            `json:"errorCode,omitempty"`
	ErrorMessage   string                            `json:"errorMessage,omitempty"`
}

type stage9PluginUninstallGuidanceFixture struct {
	Version string                              `json:"version"`
	Cases   []stage9PluginUninstallGuidanceCase `json:"cases"`
}

type stage9PluginUninstallGuidanceCaseSpec struct {
	name     string
	path     string
	snapshot catalog.Snapshot
}

// TestStage9PluginUninstallGuidanceFixtureMatchesCurrentGoOwner freezes the
// catalog-only guidance projection. It does not probe the filesystem or
// execute either generated command; Go remains the plugin lifecycle owner.
func TestStage9PluginUninstallGuidanceFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 plugin guidance fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/plugin-uninstall-guidance.json",
	)
	want := stage9PluginUninstallGuidanceFixture{
		Version: stage9PluginUninstallGuidanceVersion,
		Cases:   make([]stage9PluginUninstallGuidanceCase, 0, len(stage9PluginUninstallGuidanceCaseSpecs())),
	}
	for _, testCase := range stage9PluginUninstallGuidanceCaseSpecs() {
		want.Cases = append(want.Cases, stage9PluginUninstallGuidanceOwnerCase(t, testCase))
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode plugin guidance fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write plugin guidance fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read plugin guidance fixture: %v", err)
	}
	var got stage9PluginUninstallGuidanceFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode plugin guidance fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 plugin uninstall guidance fixture drifted from the Go owner")
	}
}

func stage9PluginUninstallGuidanceOwnerCase(
	t *testing.T,
	testCase stage9PluginUninstallGuidanceCaseSpec,
) stage9PluginUninstallGuidanceCase {
	t.Helper()
	if testCase.path == "/api/v1/plugins//uninstall-guidance" {
		return stage9PluginUninstallGuidanceCase{
			Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path,
			ExpectedStatus: http.StatusNotFound, ErrorCode: "NOT_FOUND",
			ErrorMessage: "unknown endpoint " + testCase.path,
		}
	}
	repository := &stage9PluginsReadRepository{snapshot: testCase.snapshot}
	catalogService, err := catalog.New(repository, nil, "plugins")
	if err != nil {
		t.Fatalf("create plugin catalog for %s: %v", testCase.name, err)
	}
	service := stratsrv.NewService(nil, catalogService, nil)
	router := gin.New()
	strategyapi.RegisterPluginRoutes(router.Group("/api/v1"), service)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, stage9PluginsReadRequest(context.Background(), testCase.path))
	entry := stage9PluginUninstallGuidanceCase{
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
		t.Fatalf("decode %s response: %v (%s)", testCase.name, err, recorder.Body.String())
	}
	if envelope.Error != nil {
		entry.ErrorCode = envelope.Error.Code
		entry.ErrorMessage = envelope.Error.Message
		return entry
	}
	var guidance stratsrv.PluginUninstallGuidance
	if err := json.Unmarshal(envelope.Data, &guidance); err != nil {
		t.Fatalf("decode %s guidance: %v", testCase.name, err)
	}
	entry.Response = &guidance
	return entry
}

func stage9PluginUninstallGuidanceCaseSpecs() []stage9PluginUninstallGuidanceCaseSpec {
	base := stage9PluginUninstallGuidanceSnapshot("pine-plan", "plugins/pine-plan.so")
	return []stage9PluginUninstallGuidanceCaseSpec{
		{name: "normal", path: "/api/v1/plugins/pine-plan/uninstall-guidance", snapshot: base},
		{
			name: "quoted-path", path: "/api/v1/plugins/quote-plugin/uninstall-guidance",
			snapshot: stage9PluginUninstallGuidanceSnapshot("quote-plugin", "plugins/O'Brien/plugin.so"),
		},
		{name: "unknown", path: "/api/v1/plugins/missing/uninstall-guidance", snapshot: base},
		{name: "blank-encoded", path: "/api/v1/plugins/%20/uninstall-guidance", snapshot: base},
		{name: "invalid-escape", path: "/api/v1/plugins/%ZZ/uninstall-guidance", snapshot: base},
		{name: "blank", path: "/api/v1/plugins//uninstall-guidance", snapshot: base},
	}
}

func stage9PluginUninstallGuidanceSnapshot(pluginID, installPath string) catalog.Snapshot {
	return catalog.Snapshot{
		TargetDir: "plugins",
		Plugins: []catalog.ManagedPlugin{{
			Descriptor: stratsrv.PluginDescriptor{ID: pluginID},
			Installation: stratsrv.PluginInstallation{
				TargetDir: "plugins", InstallPath: installPath,
			},
		}},
	}
}
