package rustmigration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"

	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
)

const stage9MCPToolSchemaFixtureVersion = "stage9.mcp-tool-schemas.v1"

type stage9MCPToolSchemaFixture struct {
	Version   string                     `json:"version"`
	ToolCount int                        `json:"toolCount"`
	Schemas   map[string]json.RawMessage `json:"schemas"`
}

// TestStage9MCPToolSchemasMatchGoReference keeps the complete inputSchema
// object for every reviewed local MCP tool anchored to the Go registry. The
// fixture is only regenerated explicitly; normal runs are read-only.
func TestStage9MCPToolSchemasMatchGoReference(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 MCP schema reference source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/mcp-tool-schemas.json",
	)

	registry := assistanttestkit.NewToolRegistry()
	assistantassembly.RegisterJFTradeADKTools(nil, registry, assistantassembly.ToolDeps{
		// A non-nil product port selects the production Go descriptors for the
		// three legacy market tools instead of their test-only fallback schemas.
		ProductTool: func(context.Context, string, map[string]any) (any, error) {
			return nil, nil
		},
		ExecutionTool: func(context.Context, string, map[string]any) (any, error) {
			return nil, nil
		},
	})
	want := stage9MCPToolSchemaFixture{
		Version:   stage9MCPToolSchemaFixtureVersion,
		ToolCount: len(assistant.LocalMCPReadOnlyToolNames),
		Schemas:   make(map[string]json.RawMessage, len(assistant.LocalMCPReadOnlyToolNames)),
	}
	for _, name := range assistant.LocalMCPReadOnlyToolNames {
		if _, duplicate := want.Schemas[name]; duplicate {
			t.Fatalf("local MCP tool list contains duplicate %q", name)
		}
		registered, found := registry.Get(name)
		if !found {
			t.Fatalf("Go registry is missing reviewed MCP tool %q", name)
		}
		if registered.Descriptor.Permission != "read_internal" {
			t.Fatalf("Go registry permission for %q = %q, want read_internal", name, registered.Descriptor.Permission)
		}
		if registered.Descriptor.InputSchema == nil {
			t.Fatalf("Go registry has nil input schema for %q", name)
		}
		raw, err := json.Marshal(registered.Descriptor.InputSchema)
		if err != nil {
			t.Fatalf("marshal Go input schema %q: %v", name, err)
		}
		want.Schemas[name] = raw
	}
	if len(want.Schemas) != 69 {
		t.Fatalf("Go reviewed MCP schema count = %d, want 69", len(want.Schemas))
	}
	if want.ToolCount != len(want.Schemas) {
		t.Fatalf("Go reviewed MCP tool count = %d, schema count = %d", want.ToolCount, len(want.Schemas))
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode MCP schema fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write MCP schema fixture: %v", err)
		}
	}

	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read MCP schema fixture: %v", err)
	}
	var got stage9MCPToolSchemaFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode MCP schema fixture: %v", err)
	}
	if got.Version != stage9MCPToolSchemaFixtureVersion {
		t.Fatalf("MCP schema fixture version = %q, want %q", got.Version, stage9MCPToolSchemaFixtureVersion)
	}
	if got.ToolCount != len(got.Schemas) {
		t.Fatalf("MCP schema fixture toolCount = %d, schema count = %d", got.ToolCount, len(got.Schemas))
	}
	if got.ToolCount != want.ToolCount {
		t.Fatalf("MCP schema fixture toolCount = %d, Go registry count = %d", got.ToolCount, want.ToolCount)
	}
	if len(got.Schemas) != len(want.Schemas) {
		t.Fatalf("MCP schema fixture count = %d, Go registry count = %d", len(got.Schemas), len(want.Schemas))
	}
	gotNames := make([]string, 0, len(got.Schemas))
	for name := range got.Schemas {
		gotNames = append(gotNames, name)
	}
	sort.Strings(gotNames)
	wantNames := append([]string(nil), assistant.LocalMCPReadOnlyToolNames...)
	sort.Strings(wantNames)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("MCP schema fixture names differ:\nfixture: %v\nGo: %v", gotNames, wantNames)
	}

	for _, name := range wantNames {
		t.Run(name, func(t *testing.T) {
			actual := canonicalStage9MCPJSON(t, want.Schemas[name], "Go schema")
			expected := canonicalStage9MCPJSON(t, got.Schemas[name], "fixture schema")
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("Go input schema differs from fixture:\nactual: %s\nfixture: %s", actual, expected)
			}
		})
	}
}

func canonicalStage9MCPJSON(t *testing.T, raw json.RawMessage, label string) any {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", label, err)
	}
	return value
}
