package rustmigration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

type stage9PineMCPFixture struct {
	Version string              `json:"version"`
	Cases   []stage9PineMCPCase `json:"cases"`
}
type stage9PineMCPCase struct {
	Name      string                `json:"name"`
	Tool      string                `json:"tool"`
	Arguments map[string]any        `json:"arguments"`
	Expected  stage9PineMCPExpected `json:"expected"`
}
type stage9PineMCPExpected struct {
	SourceFormat        string `json:"sourceFormat"`
	Runtime             string `json:"runtime"`
	SelectedSection     string `json:"selectedSection"`
	SectionCount        int    `json:"sectionCount"`
	ExamplesIncluded    bool   `json:"examplesIncluded"`
	Ok                  bool   `json:"ok"`
	RequirementsPresent bool   `json:"requirementsPresent"`
	EmaKey              string `json:"emaKey"`
	ErrorCode           string `json:"errorCode"`
}

func TestStage9PineMCPFixtureMatchesCurrentGoOwner(t *testing.T) {
	t.Setenv("JFTRADE_PINETS_MODE", "off")
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve pine MCP fixture")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/pine-mcp/cases.json")
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture stage9PineMCPFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != "stage9.pine-mcp.v1" {
		t.Fatalf("fixture version = %q", fixture.Version)
	}
	for _, item := range fixture.Cases {
		t.Run(item.Name, func(t *testing.T) {
			switch item.Tool {
			case strategypinespec.ToolName:
				section, _ := item.Arguments["section"].(string)
				include, _ := item.Arguments["includeExamples"].(bool)
				payload, err := strategypinespec.BuildToolPayload(section, include)
				if err != nil {
					t.Fatal(err)
				}
				if got := payload["sourceFormat"]; got != item.Expected.SourceFormat {
					t.Fatalf("sourceFormat = %#v", got)
				}
				if got := payload["runtime"]; got != item.Expected.Runtime {
					t.Fatalf("runtime = %#v", got)
				}
				if got := payload["selectedSection"]; got != normalizeStage9PineSection(section) {
					t.Fatalf("selectedSection = %#v", got)
				}
				sections, ok := payload["sections"].([]map[string]any)
				if !ok {
					t.Fatalf("sections = %#v", payload["sections"])
				}
				if len(sections) != item.Expected.SectionCount {
					t.Fatalf("section count = %d, want %d", len(sections), item.Expected.SectionCount)
				}
				allowedSections := strategypinespec.AllowedSections()
				if len(sections) != len(allowedSections) {
					t.Fatalf("section count = %d, allowed sections = %d", len(sections), len(allowedSections))
				}
				for index, section := range sections {
					if section["id"] != allowedSections[index] {
						t.Fatalf("section[%d] id = %#v, want %q", index, section["id"], allowedSections[index])
					}
				}
				if payload["compatibilityScore"] != strategypine.CompatibilityScore().Score {
					t.Fatalf("compatibilityScore = %#v, want %#v", payload["compatibilityScore"], strategypine.CompatibilityScore().Score)
				}
				if payload["scoreModelVersion"] != strategypine.CompatibilityScore().ScoreModelVersion {
					t.Fatalf("scoreModelVersion = %#v, want %#v", payload["scoreModelVersion"], strategypine.CompatibilityScore().ScoreModelVersion)
				}
				if dimensions, ok := payload["compatibilityDimensions"].([]strategypine.CompatibilityDimension); !ok || len(dimensions) != len(strategypine.CompatibilityScore().Dimensions) {
					t.Fatalf("compatibilityDimensions = %#v", payload["compatibilityDimensions"])
				}
				capabilities, ok := payload["capabilities"].([]strategypine.Capability)
				if !ok || !stage9PineCapabilityContains(capabilities, "strategy.v40_broker_boundary_decision", "orders") || !stage9PineCapabilityContains(capabilities, "tooling.v34_generated_support_snapshot", "tooling") {
					t.Fatalf("capabilities missing v4.0 evidence entries: %#v", payload["capabilities"])
				}
				externalEngine, ok := payload["externalEngine"].(map[string]any)
				if !ok || externalEngine["engine"] != "pinets-shadow" || externalEngine["enabled"] != false || externalEngine["license"] != "AGPL-3.0-only" {
					t.Fatalf("externalEngine = %#v", payload["externalEngine"])
				}
				assertStage9PineKeys(t, externalEngine, []string{"authority", "compliance", "differenceSummary", "enabled", "engine", "license", "mode", "package", "repository", "scope", "status", "strategyMetrics", "worker"})
				if externalEngine["package"] != "pinets@0.9.31" || externalEngine["worker"] != "scripts/pinets-worker.mjs" {
					t.Fatalf("spec externalEngine package/worker = %#v", externalEngine)
				}
				supportMatrix, ok := payload["supportMatrix"].([]map[string]any)
				if !ok || !stage9PineMatrixContains(supportMatrix, "JFTrade Pine v6 main path", "sourceFormat=pine-v6") {
					t.Fatalf("supportMatrix missing main path: %#v", payload["supportMatrix"])
				}
				if !stage9PineMatrixContains(supportMatrix, "v4.0 broker emulator boundary decision", "brokerBoundary payload") {
					t.Fatalf("supportMatrix missing v4.0 broker boundary: %#v", payload["supportMatrix"])
				}
				brokerBoundary, ok := payload["brokerBoundary"].([]map[string]any)
				if !ok || !stage9PineBrokerBoundaryContains(brokerBoundary, "OCA and partial fill", "excluded from executable Pine v6 score") {
					t.Fatalf("brokerBoundary missing v4.0 score treatment: %#v", payload["brokerBoundary"])
				}
				if _, ok := payload["goldenScripts"].([]map[string]any); !ok {
					t.Fatalf("goldenScripts = %T, want []map[string]any", payload["goldenScripts"])
				}
				goldenScripts := payload["goldenScripts"].([]map[string]any)
				if len(goldenScripts) != len(strategypinespec.GoldenExamples()) {
					t.Fatalf("goldenScripts len = %d, want %d", len(goldenScripts), len(strategypinespec.GoldenExamples()))
				}
				if goldenScripts[0]["id"] != "golden-ma-cross" || goldenScripts[len(goldenScripts)-1]["id"] != "golden-v17-semantic-transition" {
					t.Fatalf("goldenScripts order = %#v ... %#v", goldenScripts[0], goldenScripts[len(goldenScripts)-1])
				}
			case "strategy.validate_pine":
				script, _ := item.Arguments["script"].(string)
				analysis := strategypine.AnalyzeScript(script, strategypine.AnalysisOptions{})
				if analysis.OK != item.Expected.Ok {
					t.Fatalf("ok = %v", analysis.OK)
				}
				if item.Expected.EmaKey != "" {
					found := false
					for _, requirement := range analysis.Requirements.Indicators {
						if requirement.Key == item.Expected.EmaKey {
							found = true
						}
					}
					if !found {
						t.Fatalf("missing requirement %q", item.Expected.EmaKey)
					}
				}
				if item.Expected.ErrorCode != "" && len(analysis.Diagnostics) == 0 {
					t.Fatalf("expected diagnostic %q", item.Expected.ErrorCode)
				}
				validation := assistantassembly.StrategyValidatePineToolPayload(item.Arguments)
				if got := validation["ok"]; got != item.Expected.Ok {
					t.Fatalf("validation ok = %#v, want %v", got, item.Expected.Ok)
				}
				assertStage9PineKeys(t, validation, []string{"errors", "externalEngine", "hooks", "metadata", "normalizedScript", "ok", "requirements", "runtime", "saveHint", "sourceFormat", "warnings"})
				externalEngine, ok := validation["externalEngine"].(map[string]any)
				if !ok || externalEngine["engine"] != "pinets-shadow" || externalEngine["enabled"] != false {
					t.Fatalf("validation externalEngine = %#v", validation["externalEngine"])
				}
				assertStage9PineKeys(t, externalEngine, []string{"compliance", "diagnostics", "differenceSummary", "enabled", "engine", "engineVersion", "license", "mode", "ok", "repository", "status"})
				if externalEngine["license"] != "" || externalEngine["repository"] != "" {
					t.Fatalf("validation externalEngine should use disabled PayloadMap shape: %#v", externalEngine)
				}
				if _, ok := externalEngine["package"]; ok {
					t.Fatalf("validation externalEngine leaked spec-only package field: %#v", externalEngine)
				}
				if item.Expected.Ok {
					if validation["saveHint"] != nil || validation["requirements"] == nil {
						t.Fatalf("valid validation payload = %#v", validation)
					}
				} else if validation["saveHint"] == nil {
					t.Fatalf("invalid validation payload lacks saveHint = %#v", validation)
				}
			default:
				t.Fatalf("unknown tool %q", item.Tool)
			}
		})
	}
}

func normalizeStage9PineSection(value string) string {
	return strategypinespec.NormalizeSection(value)
}

func assertStage9PineKeys(t *testing.T, value map[string]any, expected []string) {
	t.Helper()
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sort.Strings(expected)
	if !equalStage9PineStringSlices(keys, expected) {
		t.Fatalf("keys = %#v, want %#v", keys, expected)
	}
}

func equalStage9PineStringSlices(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func stage9PineMatrixContains(matrix []map[string]any, capability string, notesSubstring string) bool {
	for _, item := range matrix {
		if item["capability"] == capability {
			notes, _ := item["notes"].(string)
			return notesSubstring == "" || strings.Contains(notes, notesSubstring)
		}
	}
	return false
}

func stage9PineBrokerBoundaryContains(boundary []map[string]any, area string, scoreTreatmentSubstring string) bool {
	for _, item := range boundary {
		if item["area"] == area {
			scoreTreatment, _ := item["scoreTreatment"].(string)
			return strings.Contains(scoreTreatment, scoreTreatmentSubstring)
		}
	}
	return false
}

func stage9PineCapabilityContains(capabilities []strategypine.Capability, id string, dimension string) bool {
	for _, capability := range capabilities {
		if capability.ID == id && capability.Dimension == dimension {
			return true
		}
	}
	return false
}
