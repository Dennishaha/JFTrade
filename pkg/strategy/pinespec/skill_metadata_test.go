package pinespec

import (
	"strings"
	"testing"
)

func TestPineSpecSkillMetadataAndResources(t *testing.T) {
	sections := Sections()
	if len(sections) == 0 || sections[0].ID == "" {
		t.Fatalf("Sections = %#v", sections)
	}
	sections[0].ID = "mutated"
	if Sections()[0].ID == "mutated" {
		t.Fatal("Sections should return a defensive copy")
	}

	allowed := AllowedSections()
	if len(allowed) != len(Sections()) {
		t.Fatalf("AllowedSections = %#v", allowed)
	}
	if ResearchSkillDescription() == "" || !strings.Contains(ResearchSkillInstructions(), "strategy.research_backtest") {
		t.Fatalf("research skill metadata missing backtest guidance")
	}
	if PublishSkillDescription() == "" || !strings.Contains(PublishSkillInstructions(), "strategy.save_definition") {
		t.Fatalf("publish skill metadata missing save guidance")
	}
	if tools := ResearchSkillAllowedTools(); len(tools) == 0 || tools[0] != ToolName {
		t.Fatalf("research allowed tools = %#v", tools)
	}
	if tools := PublishSkillAllowedTools(); len(tools) == 0 || tools[0] != "strategy.validate_pine" {
		t.Fatalf("publish allowed tools = %#v", tools)
	}
	resources := SkillResourceFiles()
	for _, key := range []string{
		"references/pine-v6-spec.md",
		"references/pine-v6-examples.md",
		"references/pine-v6-cheatsheet.md",
	} {
		if strings.TrimSpace(resources[key]) == "" {
			t.Fatalf("missing skill resource %s in %#v", key, resources)
		}
	}
	if strings.TrimSpace(ResearchSkillResourceFiles()["references/strategy-research-workflow.md"]) == "" {
		t.Fatal("research skill resources should include workflow guide")
	}
	if strings.TrimSpace(PublishSkillResourceFiles()["references/strategy-publish-checklist.md"]) == "" {
		t.Fatal("publish skill resources should include publish checklist")
	}
	if !strings.Contains(SaveDraftUsageHint(), Skeleton()) {
		t.Fatal("SaveDraftUsageHint should include the Pine skeleton")
	}
}
