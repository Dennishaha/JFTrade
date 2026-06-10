package dslspec

import (
	"strings"
	"testing"

	strategydsl "github.com/jftrade/jftrade-main/pkg/strategy/dsl"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestExamplesParseAndPlan(t *testing.T) {
	for _, example := range Examples() {
		example := example
		t.Run(example.ID, func(t *testing.T) {
			program, err := strategydsl.ParseScript(example.Script)
			if err != nil {
				t.Fatalf("ParseScript(%s): %v", example.ID, err)
			}
			if _, err := strategyir.PlanRequirements(program); err != nil {
				t.Fatalf("PlanRequirements(%s): %v", example.ID, err)
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

	payload, err = BuildToolPayload("examples", true)
	if err != nil {
		t.Fatalf("BuildToolPayload examples: %v", err)
	}
	examples, ok = payload["examples"].([]map[string]any)
	if !ok || len(examples) == 0 {
		t.Fatalf("examples payload = %#v, want non-empty examples", payload["examples"])
	}
}

func TestSkillResourcesContainSpecAndExamples(t *testing.T) {
	files := SkillResourceFiles()
	spec := files["references/dsl-v1-spec.md"]
	if !strings.Contains(spec, "# JFTrade DSL v1 规范") {
		t.Fatalf("spec resource missing heading: %q", spec)
	}
	examples := files["references/dsl-v1-examples.md"]
	if !strings.Contains(examples, "## 最小可保存草稿") {
		t.Fatalf("examples resource missing expected example heading: %q", examples)
	}
}
