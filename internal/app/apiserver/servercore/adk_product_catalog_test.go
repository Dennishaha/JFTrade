package servercore

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type capabilityUISurfaceManifest map[string]map[string]string

func loadCapabilityUISurfaceManifest(t *testing.T) capabilityUISurfaceManifest {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve capability catalog test source path")
	}
	path := filepath.Join(
		filepath.Dir(sourceFile),
		"../../../../apps/web/src/features/capability-surfaces.json",
	)
	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read UI capability surface manifest: %v", err)
	}
	var manifest capabilityUISurfaceManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("parse UI capability surface manifest: %v", err)
	}
	return manifest
}

func TestCapabilityCatalogSurfacesAreRegisteredAndMCPBounded(t *testing.T) {
	registry := jfadk.NewToolRegistry()
	RegisterJFTradeADKTools(nil, registry, ToolDeps{})
	mcpNames := append([]string(nil), jfadk.LocalMCPReadOnlyToolNames...)
	slices.Sort(mcpNames)

	for _, capability := range broker.BuiltinCapabilityCatalog.Features {
		toolName := capability.Surface.Tool
		if toolName == "" {
			t.Errorf("%s has no tool mapping", capability.ID)
			continue
		}
		registered, ok := registry.Get(toolName)
		if !ok {
			t.Errorf("%s maps to unregistered tool %q", capability.ID, toolName)
			continue
		}
		switch capability.Access {
		case broker.FeatureAccessRead:
			if registered.Descriptor.Permission != "read_internal" {
				t.Errorf("%s permission = %q, want read_internal", toolName, registered.Descriptor.Permission)
			}
			if !capability.Surface.ReadOnlyMCP || !slices.Contains(mcpNames, toolName) {
				t.Errorf("%s is a reviewed read capability but is absent from local read-only MCP", toolName)
			}
		case broker.FeatureAccessWrite:
			if registered.Descriptor.Permission != "write_external" ||
				registered.Descriptor.RiskLevel != "high" {
				t.Errorf("%s descriptor = %#v, want write_external + high", toolName, registered.Descriptor)
			}
			if slices.Contains(mcpNames, toolName) {
				t.Errorf("%s external write leaked into local read-only MCP", toolName)
			}
		case broker.FeatureAccessTrade:
			if registered.Descriptor.Permission != "live_trading" ||
				registered.Descriptor.RiskLevel != "critical" {
				t.Errorf("%s descriptor = %#v, want live_trading + critical", toolName, registered.Descriptor)
			}
			if len(registered.Descriptor.RequiresApprovalIn) != 3 {
				t.Errorf("%s approval modes = %v, want approval in every mode", toolName, registered.Descriptor.RequiresApprovalIn)
			}
			if slices.Contains(mcpNames, toolName) {
				t.Errorf("%s trading tool leaked into local read-only MCP", toolName)
			}
		}
	}
}

func TestCustomizationToolsMapToOpenDOperations(t *testing.T) {
	want := map[string]string{
		"alerts.price.set":        "set",
		"alerts.option_event.set": "set",
		"watchlist.remote.modify": "modify",
	}
	if !maps.Equal(customizationToolActions, want) {
		t.Fatalf("customization tool actions = %v, want %v", customizationToolActions, want)
	}
}

func TestCapabilityOperationContracts(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	routes := make(map[string]struct{}, len(server.router.Routes()))
	for _, route := range server.router.Routes() {
		routes[strings.ToUpper(route.Method)+" "+route.Path] = struct{}{}
	}
	placeholder := regexp.MustCompile(`\{([^{}]+)\}`)
	uiSurfaces := loadCapabilityUISurfaceManifest(t)
	registry := jfadk.NewToolRegistry()
	RegisterJFTradeADKTools(nil, registry, ToolDeps{})
	mcpNames := append([]string(nil), jfadk.LocalMCPReadOnlyToolNames...)
	slices.Sort(mcpNames)

	for _, feature := range broker.BuiltinCapabilityCatalog.Features {
		t.Run(string(feature.ID), func(t *testing.T) {
			for _, operation := range feature.Operations {
				t.Run(operation.ID, func(t *testing.T) {
					if operation.TestID != t.Name() {
						t.Fatalf("test mapping = %q, want current behavior test %q", operation.TestID, t.Name())
					}
					path := strings.SplitN(operation.API, "?", 2)[0]
					path = placeholder.ReplaceAllString(path, `:$1`)
					routeKey := strings.ToUpper(operation.HTTPMethod) + " " + path
					if _, ok := routes[routeKey]; !ok {
						t.Errorf("API operation is not registered: %s", routeKey)
					}
					if operation.UISurfaceID != "" {
						if _, ok := uiSurfaces[operation.UISurfaceID]; !ok {
							t.Errorf("UI surface %q is absent from shared frontend manifest", operation.UISurfaceID)
						}
					}
					if operation.Tool == "" {
						return
					}
					registered, ok := registry.Get(operation.Tool)
					if !ok {
						t.Fatalf("tool %q is not registered", operation.Tool)
					}
					schema := registered.Descriptor.InputSchema
					if schema["type"] != "object" || schema["additionalProperties"] != false {
						t.Errorf("tool %q does not have a closed business JSON schema: %#v", operation.Tool, schema)
					}
					inMCP := slices.Contains(mcpNames, operation.Tool)
					if feature.Access == broker.FeatureAccessRead && !inMCP {
						t.Errorf("reviewed read tool %q is absent from local read-only MCP", operation.Tool)
					}
					if feature.Access != broker.FeatureAccessRead && inMCP {
						t.Errorf("write-capable tool %q leaked into local read-only MCP", operation.Tool)
					}
					for _, protocol := range operation.Protocols {
						if protocol.BrokerID == "" || protocol.Key == "" || protocol.ID == 0 ||
							(protocol.Kind != "request" && protocol.Kind != "push") {
							t.Errorf("invalid OpenD protocol mapping: %#v", protocol)
						}
					}
				})
			}
		})
	}
}

func TestProductToolRegistryAndOperationSchemasAreCatalogBacked(t *testing.T) {
	productTools := map[string]struct{}{}
	for _, definition := range productReadTools {
		productTools[definition.name] = struct{}{}
	}
	for _, definition := range productTradeTools {
		productTools[definition.name] = struct{}{}
	}
	for _, definition := range productWriteTools {
		productTools[definition.name] = struct{}{}
	}

	catalogOperations := map[string][]string{}
	for _, feature := range broker.BuiltinCapabilityCatalog.Features {
		for _, operation := range feature.Operations {
			if operation.Tool == "" {
				continue
			}
			catalogOperations[operation.Tool] = append(catalogOperations[operation.Tool], operation.ID)
		}
	}
	for tool := range catalogOperations {
		slices.Sort(catalogOperations[tool])
		catalogOperations[tool] = slices.Compact(catalogOperations[tool])
	}

	for tool := range productTools {
		if tool == "market.capabilities" {
			continue
		}
		if len(catalogOperations[tool]) == 0 {
			t.Errorf("registered product tool %q has no CapabilityCatalog operation", tool)
		}
	}
	for tool, schemaOperations := range productToolOperations {
		if _, ok := productTools[tool]; !ok {
			t.Errorf("operation schema exists for unregistered product tool %q", tool)
			continue
		}
		want := append([]string(nil), schemaOperations...)
		slices.Sort(want)
		got := catalogOperations[tool]
		if !slices.Equal(got, want) {
			t.Errorf("tool %q operation schema = %v, catalog operations = %v", tool, want, got)
		}
	}

	registry := jfadk.NewToolRegistry()
	RegisterJFTradeADKTools(nil, registry, ToolDeps{})
	for _, tool := range jfadk.LocalMCPReadOnlyToolNames {
		registered, ok := registry.Get(tool)
		if !ok {
			t.Errorf("local MCP tool %q is not registered", tool)
			continue
		}
		if registered.Descriptor.Permission != "read_internal" {
			t.Errorf("local MCP tool %q permission = %q, want read_internal", tool, registered.Descriptor.Permission)
		}
	}
}
