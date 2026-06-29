package pinespec

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

func TestExamplesParseAndPlan(t *testing.T) {
	for _, example := range Examples() {
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
	if ProductVersion != "v4.0" {
		t.Fatalf("ProductVersion = %q, want v4.0", ProductVersion)
	}
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
	externalEngine, ok := payload["externalEngine"].(map[string]any)
	if !ok {
		t.Fatalf("externalEngine = %T, want map[string]any", payload["externalEngine"])
	}
	if externalEngine["engine"] != "pinets-shadow" || externalEngine["enabled"] != false || externalEngine["license"] != "AGPL-3.0-only" {
		t.Fatalf("externalEngine = %#v, want disabled AGPL pinets shadow metadata", externalEngine)
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
	if score, ok := payload["compatibilityScore"].(float64); !ok || score < 98 || score > 100 {
		t.Fatalf("compatibilityScore = %#v, want v4.0 score", payload["compatibilityScore"])
	}
	if payload["scoreModelVersion"] != "closed-bar-strategy-v4.0" {
		t.Fatalf("scoreModelVersion = %#v", payload["scoreModelVersion"])
	}
	if capabilities, ok := payload["capabilities"].([]strategypine.Capability); !ok || len(capabilities) == 0 {
		t.Fatalf("capabilities = %#v, want registry entries", payload["capabilities"])
	}
	foundMainPathGate := false
	foundCollectionTypeDiagnostics := false
	foundV22RuntimeSet := false
	foundV23ExpansionSet := false
	foundV24ExpansionSet := false
	foundV25ExpansionSet := false
	foundV26ExpansionSet := false
	foundV27ExpansionSet := false
	foundV28ExpansionSet := false
	foundV29ExpansionSet := false
	foundV30ExpansionSet := false
	foundV31PublicSurfaceSet := false
	foundV32MTFPreflightSet := false
	foundV33LanguageBoundarySet := false
	foundV34SupportSnapshotSet := false
	foundV40BrokerBoundarySet := false
	for _, item := range matrix {
		if item["capability"] == "JFTrade Pine v6 main path" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "sourceFormat=pine-v6") && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "runtime=pine-pinets") {
			foundMainPathGate = true
		}
		if item["capability"] == "v2.0 language foundation" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "collection namespace/type argument compatibility") {
			foundCollectionTypeDiagnostics = true
		}
		if item["capability"] == "v2.2 structured loops, tuple and pure object subset" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "动态 for/while") {
			foundV22RuntimeSet = true
		}
		if item["capability"] == "v2.3 collection, pure object and MTF expression expansion" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "纯 collection/object") {
			foundV23ExpansionSet = true
		}
		if item["capability"] == "v2.4 collection/map, MTF stoch and persistent object expansion" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "MTF ta.stoch") {
			foundV24ExpansionSet = true
		}
		if item["capability"] == "v2.5 array stats, string and timeframe helpers" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "timeframe.change") {
			foundV25ExpansionSet = true
		}
		if item["capability"] == "v2.6 collection iteration, history and object fields" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "array for-in") {
			foundV26ExpansionSet = true
		}
		if item["capability"] == "v2.7 collection/timeframe and MTF helper expansion" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "timeframe.in_seconds") {
			foundV27ExpansionSet = true
		}
		if item["capability"] == "v2.8 object history, method chain and export metadata" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "method chain") {
			foundV28ExpansionSet = true
		}
		if item["capability"] == "v2.9 object history method receiver and MTF diagnostics" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "box[1].score") {
			foundV29ExpansionSet = true
		}
		if item["capability"] == "v3.0 stable semantic declarations and varip policy" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "unsupportedReason") {
			foundV30ExpansionSet = true
		}
		if item["capability"] == "v3.1 native public surface diagnostics" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "AnalyzeScript") && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "Monaco") {
			foundV31PublicSurfaceSet = true
		}
		if item["capability"] == "v3.2 MTF diagnostics and lower-timeframe preflight" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "backtest replay") && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "tuple assignment") {
			foundV32MTFPreflightSet = true
		}
		if item["capability"] == "v3.3 advanced language boundary diagnostics" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "递归 UDF") && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "循环变量只读") {
			foundV33LanguageBoundarySet = true
		}
		if item["capability"] == "v3.4 generated support snapshot" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "pine-v6-support.md") && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "过期快照") {
			foundV34SupportSnapshotSet = true
		}
		if item["capability"] == "v4.0 broker emulator boundary decision" && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "trading-runtime parity track") && strings.Contains(jftradeCheckedTypeAssertion[string](item["notes"]), "brokerBoundary") {
			foundV40BrokerBoundarySet = true
		}
	}
	if !foundMainPathGate {
		t.Fatalf("support matrix missing v1.0 Pine main path gate: %#v", matrix)
	}
	if !foundCollectionTypeDiagnostics {
		t.Fatalf("support matrix missing v2.0 collection type diagnostics: %#v", matrix)
	}
	if !foundV22RuntimeSet {
		t.Fatalf("support matrix missing v2.2 runtime set: %#v", matrix)
	}
	if !foundV23ExpansionSet {
		t.Fatalf("support matrix missing v2.3 expansion set: %#v", matrix)
	}
	if !foundV24ExpansionSet {
		t.Fatalf("support matrix missing v2.4 expansion set: %#v", matrix)
	}
	if !foundV25ExpansionSet {
		t.Fatalf("support matrix missing v2.5 expansion set: %#v", matrix)
	}
	if !foundV26ExpansionSet {
		t.Fatalf("support matrix missing v2.6 expansion set: %#v", matrix)
	}
	if !foundV27ExpansionSet {
		t.Fatalf("support matrix missing v2.7 expansion set: %#v", matrix)
	}
	if !foundV28ExpansionSet {
		t.Fatalf("support matrix missing v2.8 expansion set: %#v", matrix)
	}
	if !foundV29ExpansionSet {
		t.Fatalf("support matrix missing v2.9 expansion set: %#v", matrix)
	}
	if !foundV30ExpansionSet {
		t.Fatalf("support matrix missing v3.0 expansion set: %#v", matrix)
	}
	if !foundV31PublicSurfaceSet {
		t.Fatalf("support matrix missing v3.1 public surface set: %#v", matrix)
	}
	if !foundV32MTFPreflightSet {
		t.Fatalf("support matrix missing v3.2 MTF preflight set: %#v", matrix)
	}
	if !foundV33LanguageBoundarySet {
		t.Fatalf("support matrix missing v3.3 language boundary set: %#v", matrix)
	}
	if !foundV34SupportSnapshotSet {
		t.Fatalf("support matrix missing v3.4 support snapshot set: %#v", matrix)
	}
	if !foundV40BrokerBoundarySet {
		t.Fatalf("support matrix missing v4.0 broker boundary set: %#v", matrix)
	}
	if _, ok := payload["compatibilityLayers"]; ok {
		t.Fatalf("compatibilityLayers should not be present in v1.0 payload: %#v", payload["compatibilityLayers"])
	}
}

func TestBuildToolPayloadIncludesBrokerBoundary(t *testing.T) {
	payload, err := BuildToolPayload("support-matrix", false)
	if err != nil {
		t.Fatalf("BuildToolPayload support-matrix: %v", err)
	}
	boundaries, ok := payload["brokerBoundary"].([]map[string]any)
	if !ok || len(boundaries) == 0 {
		t.Fatalf("brokerBoundary = %#v, want non-empty structured boundary", payload["brokerBoundary"])
	}
	foundOutOfScope := false
	foundDiagnostic := false
	for _, boundary := range boundaries {
		if boundary["status"] == "out_of_scope" && strings.Contains(jftradeCheckedTypeAssertion[string](boundary["scoreTreatment"]), "excluded") {
			foundOutOfScope = true
		}
		for _, code := range jftradeCheckedTypeAssertion[[]string](boundary["diagnosticCodes"]) {
			if code == "PINE_ORDER_OCA_UNSUPPORTED" || code == "PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED" {
				foundDiagnostic = true
			}
		}
	}
	if !foundOutOfScope || !foundDiagnostic {
		t.Fatalf("brokerBoundary = %#v, want out-of-scope score treatment and order diagnostics", boundaries)
	}
}

func TestGeneratedPineSupportSnapshotIsCurrent(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "reference", "generated", "pine-v6-support.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated Pine support snapshot: %v", err)
	}
	if got, want := string(raw), BuildSupportSnapshotMarkdown(); got != want {
		t.Fatalf("%s is stale; run npm run generate:reference", path)
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
	if !strings.Contains(examples, "## v1.7 黄金脚本") || !strings.Contains(examples, "### UDF 与静态 for") || !strings.Contains(examples, "### v1.4 MTF 纯表达式") || !strings.Contains(examples, "### v1.5 MTF common TA") || !strings.Contains(examples, "### v1.6 MTF tuple 白名单") || !strings.Contains(examples, "### v1.7 Semantic 过渡") {
		t.Fatalf("examples resource missing golden scripts: %q", examples)
	}
	if cheatsheet := files["references/pine-v6-cheatsheet.md"]; !strings.Contains(cheatsheet, "# JFTrade Pine v6 快速参考") {
		t.Fatalf("cheatsheet resource missing heading: %q", cheatsheet)
	}
	researchFiles := ResearchSkillResourceFiles()
	if workflow := researchFiles["references/strategy-research-workflow.md"]; !strings.Contains(workflow, "strategy.research_backtest") || !strings.Contains(workflow, "backtest.result_view") || !strings.Contains(workflow, "backtest.kline_sync_status") {
		t.Fatalf("research workflow missing research tool routing: %q", workflow)
	}
	publishFiles := PublishSkillResourceFiles()
	if checklist := publishFiles["references/strategy-publish-checklist.md"]; !strings.Contains(checklist, "strategy.save_definition") || !strings.Contains(checklist, "strategy.validate_pine") {
		t.Fatalf("publish checklist missing save validation flow: %q", checklist)
	}
	if containsString(ResearchSkillAllowedTools(), "strategy.save_definition") {
		t.Fatalf("research skill should not expose save_definition")
	}
	if containsString(PublishSkillAllowedTools(), "strategy.research_backtest") {
		t.Fatalf("publish skill should not expose research_backtest")
	}
	if !containsString(ResearchSkillAllowedTools(), "backtest.kline_sync_status") || !containsString(PublishSkillAllowedTools(), "backtest.kline_sync_status") {
		t.Fatal("research and publish skills must expose K-line sync status")
	}
}

func containsString(values []string, target string) bool {
	return slices.Contains(values, target)
}
