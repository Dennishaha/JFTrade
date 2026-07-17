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
	if !strings.Contains(ResearchSkillInstructions(), "load_skill(jftrade-strategy-publish)") {
		t.Fatal("research skill metadata missing publish handoff guidance")
	}
	if PublishSkillDescription() == "" || !strings.Contains(PublishSkillInstructions(), "strategy.save_definition") {
		t.Fatalf("publish skill metadata missing save guidance")
	}
	if !strings.Contains(PublishSkillInstructions(), "load_skill(jftrade-strategy-research)") {
		t.Fatal("publish skill metadata missing research handoff guidance")
	}
	if researchWorkflow := BuildResearchWorkflowMarkdown(); !strings.Contains(researchWorkflow, "load_skill(jftrade-strategy-publish)") ||
		!strings.Contains(researchWorkflow, "未完成时只报告进度") {
		t.Fatalf("research workflow markdown missing handoff/reporting guidance: %s", researchWorkflow)
	}
	if publishChecklist := BuildPublishChecklistMarkdown(); !strings.Contains(publishChecklist, "load_skill(jftrade-strategy-research)") ||
		!strings.Contains(publishChecklist, "实际写入/优化对象") {
		t.Fatalf("publish checklist markdown missing research handoff/reporting guidance: %s", publishChecklist)
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

func TestPineSpecRejectsUnknownSectionsAndKeepsFallbackFormattingStable(t *testing.T) {
	if _, err := BuildToolPayload("unknown-section", false); err == nil || !strings.Contains(err.Error(), "不支持 section") {
		t.Fatalf("unknown section error = %v", err)
	}
	if isKnownSection("unknown-section") {
		t.Fatal("unknown section was accepted")
	}
	if title := sectionTitle("unknown-section"); title != "unknown-section" {
		t.Fatalf("unknown section title = %q", title)
	}
	if summary := sectionSummary("unknown-section"); summary != "" {
		t.Fatalf("unknown section summary = %q", summary)
	}
	if details := sectionDetails("unknown-section"); details != nil {
		t.Fatalf("unknown section details = %#v", details)
	}

	items := []map[string]any{
		{"name": "plain", "notes": "from notes"},
		{"name": "bare"},
	}
	flattened := flattenNamedItems(items)
	if len(flattened) != 2 || flattened[0] != "plain: from notes" || flattened[1] != "bare" {
		t.Fatalf("flattened named items = %#v", flattened)
	}
}
