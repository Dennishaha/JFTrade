package rustmigration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
				frozenSections := 0
				for _, section := range sections {
					if section["id"] != "support-matrix" {
						frozenSections++
					}
				}
				if frozenSections != item.Expected.SectionCount {
					t.Fatalf("frozen section count = %d, want %d", frozenSections, item.Expected.SectionCount)
				}
				externalEngine, ok := payload["externalEngine"].(map[string]any)
				if !ok || externalEngine["engine"] != "pinets-shadow" || externalEngine["enabled"] != false || externalEngine["license"] != "AGPL-3.0-only" {
					t.Fatalf("externalEngine = %#v", payload["externalEngine"])
				}
				if _, ok := payload["goldenScripts"].([]map[string]any); !ok {
					t.Fatalf("goldenScripts = %T, want []map[string]any", payload["goldenScripts"])
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
				externalEngine, ok := validation["externalEngine"].(map[string]any)
				if !ok || externalEngine["engine"] != "pinets-shadow" || externalEngine["enabled"] != false {
					t.Fatalf("validation externalEngine = %#v", validation["externalEngine"])
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
	for _, section := range strategypinespec.AllowedSections() {
		if section == value || section == "" {
			return section
		}
	}
	if value == " examples " {
		return "examples"
	}
	return value
}
