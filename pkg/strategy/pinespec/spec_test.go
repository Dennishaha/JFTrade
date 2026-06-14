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
	if got := payload["productVersion"]; got != ProductVersion {
		t.Fatalf("productVersion = %#v, want %q", got, ProductVersion)
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

func TestBuildToolPayloadIncludesSupportMatrix(t *testing.T) {
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
	if score, ok := payload["compatibilityScore"].(float64); !ok || score < 86.5 || score > 88.5 {
		t.Fatalf("compatibilityScore = %#v, want about 87", payload["compatibilityScore"])
	}
	if payload["scoreModelVersion"] != "closed-bar-strategy-v1.5" {
		t.Fatalf("scoreModelVersion = %#v", payload["scoreModelVersion"])
	}
	if capabilities, ok := payload["capabilities"].([]strategypine.Capability); !ok || len(capabilities) == 0 {
		t.Fatalf("capabilities = %#v, want registry entries", payload["capabilities"])
	}
	foundMainPathGate := false
	for _, item := range matrix {
		if item["capability"] == "JFTrade Pine v6 main path" && strings.Contains(item["notes"].(string), "sourceFormat=pine-v6") && strings.Contains(item["notes"].(string), "runtime=pine-go-plan") {
			foundMainPathGate = true
		}
	}
	if !foundMainPathGate {
		t.Fatalf("support matrix missing v1.0 Pine main path gate: %#v", matrix)
	}
	if _, ok := payload["compatibilityLayers"]; ok {
		t.Fatalf("compatibilityLayers should not be present in v1.0 payload: %#v", payload["compatibilityLayers"])
	}
}

func TestSkillResourcesContainSpecAndExamples(t *testing.T) {
	files := SkillResourceFiles()
	spec := files["references/pine-v6-spec.md"]
	if !strings.Contains(spec, "# JFTrade Pine Script v6 规范") {
		t.Fatalf("spec resource missing heading: %q", spec)
	}
	if !strings.Contains(spec, "## 支持矩阵") || strings.Contains(spec, "## 兼容迁移") {
		t.Fatalf("spec resource missing support matrix or still exposing compatibility section: %q", spec)
	}
	examples := files["references/pine-v6-examples.md"]
	if !strings.Contains(examples, "### 最小可保存草稿") {
		t.Fatalf("examples resource missing expected example heading: %q", examples)
	}
	if !strings.Contains(examples, "## v1.5 黄金脚本") || !strings.Contains(examples, "### UDF 与静态 for") || !strings.Contains(examples, "### v1.4 MTF 纯表达式") || !strings.Contains(examples, "### v1.5 MTF common TA") {
		t.Fatalf("examples resource missing golden scripts: %q", examples)
	}
}
