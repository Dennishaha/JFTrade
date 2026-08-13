package assembly

import (
	"reflect"
	"testing"

	"github.com/jftrade/jftrade-main/internal/productfeatures"
)

func TestTypedProductCapabilitiesDriveFeatureAndAssistantSchemas(t *testing.T) {
	for _, description := range productfeatures.TypedCapabilityDescriptions() {
		description := description
		t.Run(description.ToolName, func(t *testing.T) {
			if got := productToolFeatureIDs[description.ToolName]; got != description.FeatureID {
				t.Fatalf("feature ID = %q, want %q", got, description.FeatureID)
			}
			schema := productToolInputSchema(description.ToolName)
			if schema["additionalProperties"] != false {
				t.Fatalf("schema allows unknown properties: %#v", schema)
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("properties = %#v", schema["properties"])
			}
			if len(description.Operations) > 0 {
				operation, ok := properties["operation"].(map[string]any)
				if !ok || !reflect.DeepEqual(operation["enum"], description.Operations) {
					t.Fatalf("operation schema = %#v, want %#v", operation, description.Operations)
				}
			}
			switch description.SchemaKind {
			case productfeatures.ToolSchemaInstrument:
				assertRequiredField(t, schema, "instrumentId")
			case productfeatures.ToolSchemaPredictionDiscovery:
				assertRequiredField(t, schema, "operation")
			case productfeatures.ToolSchemaPredictionQuote:
				for _, name := range []string{"accountId", "mvc", "legs"} {
					assertRequiredField(t, schema, name)
				}
			}
		})
	}
}

func assertRequiredField(t *testing.T, schema map[string]any, field string) {
	t.Helper()
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required = %#v", schema["required"])
	}
	for _, candidate := range required {
		if candidate == field {
			return
		}
	}
	t.Fatalf("required = %#v, missing %q", required, field)
}
