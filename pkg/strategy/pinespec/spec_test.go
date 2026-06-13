package pinespec

import (
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

func TestExamplesParseAndPlan(t *testing.T) {
	for _, example := range Examples() {
		example := example
		t.Run(example.ID, func(t *testing.T) {
			program, err := strategypine.ParseScript(example.Script)
			if err != nil {
				t.Fatalf("ParseScript(%s): %v", example.ID, err)
			}
			if _, err := strategyir.PlanRequirements(program); err != nil {
				t.Fatalf("PlanRequirements(%s): %v", example.ID, err)
			}
		})
	}
}

func TestGoldenExamplesAnalyzeAndPlan(t *testing.T) {
	for _, example := range GoldenExamples() {
		example := example
		t.Run(example.ID, func(t *testing.T) {
			analysis := strategypine.AnalyzeScript(example.Script, strategypine.AnalysisOptions{})
			if !analysis.OK {
				t.Fatalf("AnalyzeScript(%s).OK = false, diagnostics = %#v", example.ID, analysis.Diagnostics)
			}
			program, err := strategypine.ParseScript(example.Script)
			if err != nil {
				t.Fatalf("ParseScript(%s): %v", example.ID, err)
			}
			requirements, err := strategyir.PlanRequirements(program)
			if err != nil {
				t.Fatalf("PlanRequirements(%s): %v", example.ID, err)
			}
			if len(example.RequirementKeys) == 0 {
				return
			}
			gotKeys := map[string]bool{}
			for _, indicator := range requirements.Indicators {
				gotKeys[indicator.Key] = true
			}
			for _, key := range example.RequirementKeys {
				if !gotKeys[key] {
					t.Fatalf("requirements for %s missing key %q; got %#v", example.ID, key, requirements.Indicators)
				}
			}
		})
	}
}

func TestBuildToolPayloadSectionsAndExamples(t *testing.T) {
	payload, err := BuildToolPayload("orders", false)
	if err != nil {
		t.Fatalf("BuildToolPayload: %v", err)
	}
	if got := payload["sourceFormat"]; got != SourceFormat {
		t.Fatalf("sourceFormat = %#v, want %q", got, SourceFormat)
	}
	if got := payload["runtime"]; got != Runtime {
		t.Fatalf("runtime = %#v, want %q", got, Runtime)
	}
	if got := payload["selectedSection"]; got != "orders" {
		t.Fatalf("selectedSection = %#v, want orders", got)
	}
	examples, ok := payload["examples"].([]map[string]any)
	if !ok {
		t.Fatalf("examples = %T, want []map[string]any", payload["examples"])
	}
	if len(examples) != 0 {
		t.Fatalf("examples len = %d, want 0", len(examples))
	}
	goldenScripts, ok := payload["goldenScripts"].([]map[string]any)
	if !ok || len(goldenScripts) == 0 {
		t.Fatalf("goldenScripts payload = %#v, want non-empty golden script table", payload["goldenScripts"])
	}

	payload, err = BuildToolPayload("examples", true)
	if err != nil {
		t.Fatalf("BuildToolPayload examples: %v", err)
	}
	examples, ok = payload["examples"].([]map[string]any)
	if !ok || len(examples) == 0 {
		t.Fatalf("examples payload = %#v, want non-empty examples", payload["examples"])
	}
}

func TestBuildToolPayloadIncludesSupportMatrixAndCompatibilityLayers(t *testing.T) {
	payload, err := BuildToolPayload("support-matrix", false)
	if err != nil {
		t.Fatalf("BuildToolPayload support-matrix: %v", err)
	}
	matrix, ok := payload["supportMatrix"].([]map[string]any)
	if !ok || len(matrix) == 0 {
		t.Fatalf("supportMatrix = %#v, want non-empty matrix", payload["supportMatrix"])
	}
	if got := payload["selectedSection"]; got != "support-matrix" {
		t.Fatalf("selectedSection = %#v, want support-matrix", got)
	}

	payload, err = BuildToolPayload("compatibility", false)
	if err != nil {
		t.Fatalf("BuildToolPayload compatibility: %v", err)
	}
	layers, ok := payload["compatibilityLayers"].([]map[string]any)
	if !ok || len(layers) == 0 {
		t.Fatalf("compatibilityLayers = %#v, want non-empty layers", payload["compatibilityLayers"])
	}
	foundPineSnippetMigration := false
	for _, layer := range layers {
		if layer["name"] == "legacy visual codeBlock" && strings.Contains(layer["notes"].(string), "pineSnippet") {
			foundPineSnippetMigration = true
		}
	}
	if !foundPineSnippetMigration {
		t.Fatalf("compatibility layers missing codeBlock -> pineSnippet migration note: %#v", layers)
	}
}

func TestSkillResourcesContainSpecAndExamples(t *testing.T) {
	files := SkillResourceFiles()
	spec := files["references/pine-v6-spec.md"]
	if !strings.Contains(spec, "# JFTrade Pine Script v6 规范") {
		t.Fatalf("spec resource missing heading: %q", spec)
	}
	if !strings.Contains(spec, "## 支持矩阵") || !strings.Contains(spec, "## 兼容迁移") {
		t.Fatalf("spec resource missing support matrix or compatibility sections: %q", spec)
	}
	examples := files["references/pine-v6-examples.md"]
	if !strings.Contains(examples, "### 最小可保存草稿") {
		t.Fatalf("examples resource missing expected example heading: %q", examples)
	}
	if !strings.Contains(examples, "## v0.8 黄金脚本") || !strings.Contains(examples, "### UDF 与静态 for") {
		t.Fatalf("examples resource missing golden scripts: %q", examples)
	}
}
