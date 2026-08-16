package skillsruntime

import (
	"slices"
	"testing"
)

func TestMarketIndexConstituentsSchemaStaysStrict(t *testing.T) {
	schema := DefaultToolInputSchema("market.index_constituents")
	if schema["additionalProperties"] != false {
		t.Fatalf("market.index_constituents schema = %#v", schema)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("market.index_constituents properties = %#v", schema["properties"])
	}
	limit, ok := properties["limit"].(map[string]any)
	if !ok || limit["minimum"] != 1 || limit["maximum"] != 1000 || limit["default"] != 200 {
		t.Fatalf("market.index_constituents limit schema = %#v", properties["limit"])
	}
	required, ok := schema["required"].([]string)
	if !ok || !slices.Equal(required, []string{"market", "symbol"}) {
		t.Fatalf("market.index_constituents required = %#v", schema["required"])
	}
}

func TestMarketSkillDocumentsIndexConstituentsTool(t *testing.T) {
	for _, spec := range BuiltinSkillSpecs {
		if spec.Name != "jftrade-market" {
			continue
		}
		skill, err := BuiltinSkillMetadata(spec)
		if err != nil {
			t.Fatalf("BuiltinSkillMetadata: %v", err)
		}
		if !slices.Contains(skill.Tools, "market.index_constituents") {
			t.Fatalf("jftrade-market tools = %#v", skill.Tools)
		}
		if skill.Version != "9" {
			t.Fatalf("jftrade-market version = %q", skill.Version)
		}
		return
	}
	t.Fatal("jftrade-market builtin skill not registered")
}
