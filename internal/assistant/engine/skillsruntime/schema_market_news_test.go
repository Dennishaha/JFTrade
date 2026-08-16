package skillsruntime

import (
	"slices"
	"testing"
)

func TestMarketNewsAndCorporateActionsSchemasStayStrict(t *testing.T) {
	news := DefaultToolInputSchema("market.news")
	if news["additionalProperties"] != false {
		t.Fatalf("market.news schema = %#v", news)
	}
	properties, ok := news["properties"].(map[string]any)
	if !ok {
		t.Fatalf("market.news properties = %#v", news["properties"])
	}
	limit, ok := properties["limit"].(map[string]any)
	if !ok || limit["minimum"] != 1 || limit["maximum"] != 50 || limit["default"] != 10 {
		t.Fatalf("market.news limit schema = %#v", properties["limit"])
	}
	required, ok := news["required"].([]string)
	if !ok || !slices.Equal(required, []string{"market", "symbol"}) {
		t.Fatalf("market.news required = %#v", news["required"])
	}

	actions := DefaultToolInputSchema("market.corporate_actions")
	if actions["additionalProperties"] != false {
		t.Fatalf("market.corporate_actions schema = %#v", actions)
	}
	properties, ok = actions["properties"].(map[string]any)
	if !ok {
		t.Fatalf("market.corporate_actions properties = %#v", actions["properties"])
	}
	for _, key := range []string{"from", "to"} {
		field, ok := properties[key].(map[string]any)
		if !ok || field["type"] != "string" {
			t.Fatalf("market.corporate_actions %s schema = %#v", key, properties[key])
		}
	}
	if _, ok := properties["limit"]; ok {
		t.Fatalf("market.corporate_actions unexpectedly accepts limit: %#v", properties)
	}
	required, ok = actions["required"].([]string)
	if !ok || !slices.Equal(required, []string{"market", "symbol"}) {
		t.Fatalf("market.corporate_actions required = %#v", actions["required"])
	}
}

func TestMarketSkillDocumentsNewsAndCorporateActionsTools(t *testing.T) {
	for _, spec := range BuiltinSkillSpecs {
		if spec.Name != "jftrade-market" {
			continue
		}
		bundle, err := spec.BuildBundle()
		if err != nil {
			t.Fatalf("BuildBundle: %v", err)
		}
		skill, err := BuiltinSkillMetadata(spec)
		if err != nil {
			t.Fatalf("BuiltinSkillMetadata: %v", err)
		}
		_ = bundle
		if !slices.Contains(skill.Tools, "market.news") || !slices.Contains(skill.Tools, "market.corporate_actions") {
			t.Fatalf("jftrade-market tools = %#v", skill.Tools)
		}
		if skill.Version != "9" {
			t.Fatalf("jftrade-market version = %q", skill.Version)
		}
		return
	}
	t.Fatal("jftrade-market builtin skill not registered")
}
