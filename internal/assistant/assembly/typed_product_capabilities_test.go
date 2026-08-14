package assembly

import (
	"reflect"
	"slices"
	"testing"

	"github.com/jftrade/jftrade-main/internal/productfeatures"
)

func TestTypedProductCapabilitiesDriveFeatureAndAssistantSchemas(t *testing.T) {
	for _, description := range productfeatures.TypedCapabilityDescriptions() {
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

func TestAssistantSchemasCoverProviderAndResearchExtensions(t *testing.T) {
	for _, name := range []string{
		"research.screen_catalog", "market.candles", "market.depth", "research.calendar",
	} {
		schema := productToolInputSchema(name)
		if schema["additionalProperties"] != false {
			t.Fatalf("%s allows unknown properties: %#v", name, schema)
		}
	}
	candles := productToolInputSchema("market.candles")["properties"].(map[string]any)
	for _, field := range []string{"sessions", "beforeTime", "adjustment", "startTime", "endTime"} {
		if _, ok := candles[field]; !ok {
			t.Fatalf("market.candles missing %q: %#v", field, candles)
		}
	}
	calendar := productToolInputSchema("research.calendar")["properties"].(map[string]any)
	for _, field := range []string{"sort", "stockScope", "marketCapMin", "optionVolumeMax", "ivMin", "ivRankMax", "ivPercentileMin"} {
		if _, ok := calendar[field]; !ok {
			t.Fatalf("research.calendar missing %q: %#v", field, calendar)
		}
	}
	if got := calendar["sort"].(map[string]any)["enum"]; !reflect.DeepEqual(got, []string{"hot", "market_cap", "option_volume", "iv", "iv_rank", "iv_percentile"}) {
		t.Fatalf("research.calendar sort = %#v", got)
	}
	if got := calendar["stockScope"].(map[string]any)["enum"]; !reflect.DeepEqual(got, []string{"all", "watchlist", "position", "special"}) {
		t.Fatalf("research.calendar stockScope = %#v", got)
	}
	marketCapNumber := calendar["marketCapMin"].(map[string]any)["anyOf"].([]any)[0].(map[string]any)
	if marketCapNumber["minimum"] != 0 || marketCapNumber["maximum"] != nil {
		t.Fatalf("research.calendar market cap bounds = %#v", marketCapNumber)
	}
	ivNumber := calendar["ivMax"].(map[string]any)["anyOf"].([]any)[0].(map[string]any)
	if ivNumber["minimum"] != 0 || ivNumber["maximum"] != float64(100) {
		t.Fatalf("research.calendar IV bounds = %#v", ivNumber)
	}
	screen := productToolInputSchema("research.screen")["properties"].(map[string]any)
	if _, ok := screen["conditions"]; !ok {
		t.Fatalf("research.screen missing conditions: %#v", screen)
	}
	operation := screen["operation"].(map[string]any)
	if !reflect.DeepEqual(operation["enum"], []string{"stock_v2"}) {
		t.Fatalf("research.screen operation schema = %#v, want V2 only", operation)
	}
}

func assertRequiredField(t *testing.T, schema map[string]any, field string) {
	t.Helper()
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required = %#v", schema["required"])
	}
	if slices.Contains(required, field) {
		return
	}
	t.Fatalf("required = %#v, missing %q", required, field)
}
