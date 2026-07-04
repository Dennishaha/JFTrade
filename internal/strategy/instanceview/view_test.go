package instanceview

import (
	"strings"
	"testing"
	"time"

	strategy "github.com/jftrade/jftrade-main/internal/strategy"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestRuntimeAndSourceFormatFromParams(t *testing.T) {
	if got := RuntimeFromParams(map[string]any{"runtime": " "}); got != pineworker.RuntimeID {
		t.Fatalf("blank runtime = %q, want %q", got, pineworker.RuntimeID)
	}
	if got := RuntimeFromParams(map[string]any{"runtime": "removed-script-runtime"}); got != "removed-script-runtime" {
		t.Fatalf("legacy runtime = %q", got)
	}
	if got := SourceFormatFromParams(map[string]any{"sourceFormat": "legacy-source"}); got != "legacy-source" {
		t.Fatalf("legacy source format = %q", got)
	}
	if got := SourceFormatFromParams(nil); got != strategydefinition.SourceFormatPineV6 {
		t.Fatalf("default source format = %q", got)
	}
}

func TestStartableRequiresPineV6AndPineRuntime(t *testing.T) {
	startable := strategy.ManagedInstance{Params: map[string]any{
		"runtime":      pineworker.RuntimeID,
		"sourceFormat": strategydefinition.SourceFormatPineV6,
	}}
	if !Startable(startable) {
		t.Fatalf("expected Pine v6 PineTS instance to be startable")
	}
	if Startable(strategy.ManagedInstance{Params: map[string]any{"runtime": "legacy-runtime", "sourceFormat": strategydefinition.SourceFormatPineV6}}) {
		t.Fatalf("legacy runtime should not be startable")
	}
	if Startable(strategy.ManagedInstance{Params: map[string]any{"runtime": pineworker.RuntimeID, "sourceFormat": "legacy-source"}}) {
		t.Fatalf("legacy source format should not be startable")
	}
}

func TestToInstanceViewNormalizesBindingAndCopiesParams(t *testing.T) {
	instance := strategy.ManagedInstance{
		ID:       "instance-1",
		PluginID: pineworker.RuntimeID,
		Definition: strategy.DefinitionSummary{
			StrategyID: "definition-1",
			Name:       "Definition",
			Version:    "0.1.0",
		},
		Binding: strategy.InstanceBinding{
			Symbols:  []string{"us:aapl"},
			Interval: "1m",
		},
		Params: map[string]any{"runtime": pineworker.RuntimeID, "sourceFormat": strategydefinition.SourceFormatPineV6},
		Status: "STOPPED",
	}

	view := ToInstanceView(instance)
	if view.Runtime != pineworker.RuntimeID || view.SourceFormat != strategydefinition.SourceFormatPineV6 || !view.Startable {
		t.Fatalf("unexpected view runtime fields: %+v", view)
	}
	if len(view.Binding.Symbols) != 1 || view.Binding.Symbols[0] != "US.AAPL" {
		t.Fatalf("binding symbols = %+v", view.Binding.Symbols)
	}
	view.Params["runtime"] = "mutated"
	if instance.Params["runtime"] != pineworker.RuntimeID {
		t.Fatalf("view params mutated source params")
	}
}

func TestBuildInstanceIDUsesDefinitionOrDefaultPrefix(t *testing.T) {
	at := time.Date(2026, time.January, 2, 3, 4, 5, 6, time.UTC)
	if got, want := BuildInstanceID("definition-1", at), "definition-1-20260102030405.000000006"; got != want {
		t.Fatalf("instance id = %q, want %q", got, want)
	}
	if got, want := BuildInstanceID(" ", at), pineworker.RuntimeID+"-20260102030405.000000006"; got != want {
		t.Fatalf("default instance id = %q, want %q", got, want)
	}
	if got := BuildInstanceID("definition-2", time.Time{}); !strings.HasPrefix(got, "definition-2-") {
		t.Fatalf("zero-time instance id = %q", got)
	}
	if PluginIDForDefinition(strategy.Definition{}) != pineworker.RuntimeID {
		t.Fatal("definition plugin ID mismatch")
	}
	if copied := copyMap(nil); copied == nil || len(copied) != 0 {
		t.Fatalf("copyMap(nil) = %#v", copied)
	}
}
