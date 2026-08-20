package rustmigration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	catalog "github.com/jftrade/jftrade-main/internal/strategy/catalog"
)

const stage9PluginUninstallGuidanceVersion = "stage9.plugin-uninstall-guidance.v1"

type stage9PluginUninstallGuidanceCase struct {
	Name        string                            `json:"name"`
	RequestPath string                            `json:"requestPath"`
	Response    *stratsrv.PluginUninstallGuidance `json:"response,omitempty"`
	ErrorCode   string                            `json:"errorCode,omitempty"`
}

type stage9PluginUninstallGuidanceFixture struct {
	Version string                              `json:"version"`
	Cases   []stage9PluginUninstallGuidanceCase `json:"cases"`
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
	service, err := catalog.New(nil, nil, "plugins")
	if err != nil {
		t.Fatalf("create plugin catalog: %v", err)
	}
	for _, plugin := range []catalog.ManagedPlugin{
		{
			Descriptor: stratsrv.PluginDescriptor{ID: "pine-plan"},
			Installation: stratsrv.PluginInstallation{
				InstallPath: "plugins/pine-plan.so",
			},
		},
		{
			Descriptor: stratsrv.PluginDescriptor{ID: "quote-plugin"},
			Installation: stratsrv.PluginInstallation{
				InstallPath: "plugins/O'Brien/plugin.so",
			},
		},
	} {
		if err := service.RegisterPlugin(plugin); err != nil {
			t.Fatalf("register plugin %q: %v", plugin.Descriptor.ID, err)
		}
	}

	want := stage9PluginUninstallGuidanceFixture{
		Version: stage9PluginUninstallGuidanceVersion,
		Cases: []stage9PluginUninstallGuidanceCase{
			guidanceCase(t, service, "normal", "/api/v1/plugins/pine-plan/uninstall-guidance", "pine-plan"),
			guidanceCase(t, service, "quoted-path", "/api/v1/plugins/quote-plugin/uninstall-guidance", "quote-plugin"),
			{Name: "unknown", RequestPath: "/api/v1/plugins/missing/uninstall-guidance", ErrorCode: "NOT_FOUND"},
			{Name: "blank-encoded", RequestPath: "/api/v1/plugins/%20/uninstall-guidance", ErrorCode: "BAD_REQUEST"},
			{Name: "blank", RequestPath: "/api/v1/plugins//uninstall-guidance", ErrorCode: "NOT_FOUND"},
		},
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

func guidanceCase(
	t *testing.T,
	service *catalog.Service,
	name string,
	requestPath string,
	pluginID string,
) stage9PluginUninstallGuidanceCase {
	t.Helper()
	guidance, ok := service.PluginUninstallGuidance(pluginID)
	if !ok {
		t.Fatalf("plugin %q not found", pluginID)
	}
	return stage9PluginUninstallGuidanceCase{
		Name:        name,
		RequestPath: requestPath,
		Response:    &guidance,
	}
}
