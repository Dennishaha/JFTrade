package assembly

import (
	"slices"
	"testing"

	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	jfadkruntime "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestDefaultBuiltinAgentToolsExistInAssembledRegistry(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	runtime := jfadkruntime.NewRuntime(nil, registry)
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("runtime.Close: %v", err)
		}
	})
	RegisterJFTradeADKTools(nil, registry, ToolDeps{})
	for _, toolName := range jfadkruntime.DefaultBuiltinToolNames() {
		if _, ok := registry.Get(toolName); !ok {
			t.Errorf("default builtin agent references unregistered tool %q", toolName)
		}
	}
}

func TestCapabilityCatalogSurfacesAreRegisteredAndMCPBounded(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	RegisterJFTradeADKTools(nil, registry, ToolDeps{})
	mcpNames := append([]string(nil), assistant.LocalMCPReadOnlyToolNames...)
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

func TestProductToolRegistryAndOperationSchemasAreCatalogBacked(t *testing.T) {
	productTools := map[string]struct{}{}
	for _, definition := range ProductReadToolDefinitions() {
		productTools[definition.Name] = struct{}{}
	}
	for _, definition := range ProductTradeToolDefinitions() {
		productTools[definition.Name] = struct{}{}
	}
	for _, definition := range ProductWriteToolDefinitions() {
		productTools[definition.Name] = struct{}{}
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
	for tool, schemaOperations := range ProductToolOperations() {
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

	registry := assistanttestkit.NewToolRegistry()
	RegisterJFTradeADKTools(nil, registry, ToolDeps{})
	for _, tool := range assistant.LocalMCPReadOnlyToolNames {
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

func TestResearchMacroMCPDescriptorRequiresIndicatorIDForHistory(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	RegisterJFTradeADKTools(nil, registry, ToolDeps{})
	registered, ok := registry.Get("research.macro")
	if !ok {
		t.Fatal("research.macro is not registered")
	}
	schema := registered.Descriptor.InputSchema
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("research.macro properties = %#v", schema["properties"])
	}
	indicatorID, ok := properties["indicatorId"].(map[string]any)
	if !ok || indicatorID["type"] != "string" || indicatorID["minLength"] != 1 || indicatorID["maxLength"] != 120 {
		t.Fatalf("research.macro indicatorId schema = %#v", properties["indicatorId"])
	}
	condition, ok := schema["if"].(map[string]any)
	if !ok {
		t.Fatalf("research.macro if condition = %#v", schema["if"])
	}
	conditionProperties, ok := condition["properties"].(map[string]any)
	if !ok {
		t.Fatalf("research.macro if properties = %#v", condition["properties"])
	}
	operation, ok := conditionProperties["operation"].(map[string]any)
	if !ok || operation["const"] != "indicator_history" {
		t.Fatalf("research.macro if operation = %#v", conditionProperties["operation"])
	}
	if required, ok := condition["required"].([]string); !ok || !slices.Equal(required, []string{"operation"}) {
		t.Fatalf("research.macro if required = %#v", condition["required"])
	}
	then, ok := schema["then"].(map[string]any)
	if !ok {
		t.Fatalf("research.macro then condition = %#v", schema["then"])
	}
	if required, ok := then["required"].([]string); !ok || !slices.Equal(required, []string{"indicatorId"}) {
		t.Fatalf("research.macro then required = %#v", then["required"])
	}
}

func TestProductReadSchemasRejectInvalidRoutingAndFreeTextFields(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	RegisterJFTradeADKTools(nil, registry, ToolDeps{})

	capabilities, _ := registry.Get("market.capabilities")
	capabilityProperties := capabilities.Descriptor.InputSchema["properties"].(map[string]any)
	if _, ok := capabilityProperties["tradingEnvironment"]; !ok {
		t.Fatal("market.capabilities schema lost structured tradingEnvironment")
	}
	if _, ok := capabilityProperties["query"]; ok {
		t.Fatal("market.capabilities schema exposed free-text query")
	}

	for _, name := range []string{"market.snapshot", "research.news", "research.calendar"} {
		tool, _ := registry.Get(name)
		properties := tool.Descriptor.InputSchema["properties"].(map[string]any)
		if _, ok := properties["tradingEnvironment"]; ok {
			t.Errorf("%s schema exposed invalid tradingEnvironment", name)
		}
	}

	orders, _ := registry.Get("account.orders")
	orderProperties := orders.Descriptor.InputSchema["properties"].(map[string]any)
	if _, ok := orderProperties["activeOnly"]; !ok {
		t.Fatal("account.orders schema missing activeOnly")
	}
	if _, ok := orderProperties["query"]; ok {
		t.Fatal("account.orders schema exposed ignored query")
	}
}
