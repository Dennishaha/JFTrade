package model

import (
	"errors"
	"testing"
)

func TestProviderReasoningPresetsAndExplicitEmptyMappings(t *testing.T) {
	responses := DefaultProviderReasoningConfig()
	if responses.RequestField != "reasoning.effort" || len(responses.Mappings) != 0 {
		t.Fatalf("responses preset = %+v, want reasoning.effort and no assumed mappings", responses)
	}

	missing := NormalizeProviderReasoningConfig(ProviderReasoningConfig{})
	if missing.RequestField != responses.RequestField || len(missing.Mappings) != 0 {
		t.Fatalf("missing provider config = %+v, want protocol field with empty support", missing)
	}
	empty := NormalizeProviderReasoningConfig(ProviderReasoningConfig{RequestField: "provider.reasoning", Mappings: []ProviderReasoningMapping{}})
	if empty.RequestField != "provider.reasoning" || empty.Mappings == nil || len(empty.Mappings) != 0 {
		t.Fatalf("explicit empty mappings = %+v, want empty support set", empty)
	}
	if _, _, err := ResolveProviderReasoning(Provider{ReasoningConfig: empty}, ReasoningEffortHigh); !errors.Is(err, ErrProviderReasoningUnsupported) {
		t.Fatalf("empty mapping resolution error = %v, want unsupported", err)
	}
}

func TestProviderReasoningValidationAndCustomMapping(t *testing.T) {
	valid := ProviderReasoningConfig{
		RequestField: "reasoning.level",
		Mappings: []ProviderReasoningMapping{
			{Effort: ReasoningEffortLow, Value: "LOW"},
			{Effort: ReasoningEffortHigh, Value: "balanced"},
			{Effort: ReasoningEffortMax, Value: "balanced"},
		},
	}
	if err := ValidateProviderReasoningConfig(valid); err != nil {
		t.Fatalf("valid custom reasoning config: %v", err)
	}
	field, value, err := ResolveProviderReasoning(Provider{ReasoningConfig: valid}, ReasoningEffortLow)
	if err != nil || field != "reasoning.level" || value != "LOW" {
		t.Fatalf("custom mapping = field %q value %q err %v, want exact case-preserving value", field, value, err)
	}
	if _, _, err := ResolveProviderReasoning(Provider{ReasoningConfig: valid}, ReasoningEffortMedium); !errors.Is(err, ErrProviderReasoningUnsupported) {
		t.Fatalf("unsupported custom effort error = %v", err)
	}
	if _, _, err := ResolveProviderReasoning(Provider{ReasoningConfig: valid}, ReasoningEffort("extreme")); err == nil {
		t.Fatal("unknown reasoning effort was silently treated as model default")
	}

	invalid := []ProviderReasoningConfig{
		{RequestField: "model.reasoning", Mappings: valid.Mappings},
		{RequestField: "reasoning[0]", Mappings: valid.Mappings},
		{RequestField: "reasoning..level", Mappings: valid.Mappings},
		{RequestField: "reasoning.level", Mappings: []ProviderReasoningMapping{{Effort: ReasoningEffortLow, Value: "x"}, {Effort: ReasoningEffortLow, Value: "y"}}},
		{RequestField: "reasoning.level", Mappings: []ProviderReasoningMapping{{Effort: ReasoningEffortLow, Value: "  "}}},
		{RequestField: "reasoning.level", Mappings: []ProviderReasoningMapping{{Effort: ReasoningEffort("default"), Value: "x"}}},
	}
	for index, config := range invalid {
		if err := ValidateProviderReasoningConfig(config); err == nil {
			t.Fatalf("invalid config %d was accepted: %+v", index, config)
		}
	}
}

func TestOptionalReasoningEffortRejectsDefault(t *testing.T) {
	if err := ValidateOptionalReasoningEffort(ReasoningEffort("default")); err == nil {
		t.Fatal("default reasoning effort was accepted")
	}
	if _, _, err := ResolveProviderReasoning(Provider{}, ReasoningEffort("default")); err == nil {
		t.Fatal("default provider reasoning effort was accepted")
	}
}
