package researchscreen

import (
	"strings"
	"testing"
)

func TestCatalogHelperContractsCoverEditorVariants(t *testing.T) {
	expected, ok := Lookup("simple.price")
	if !ok {
		t.Fatal("Lookup(simple.price) failed")
	}
	if factor, found := LookupProvider(expected.Category, expected.ProviderID); !found || factor.Key != expected.Key {
		t.Fatalf("LookupProvider(%s, %d) = %#v, %v", expected.Category, expected.ProviderID, factor, found)
	}
	if _, ok := LookupProvider("missing", -1); ok {
		t.Fatal("LookupProvider accepted an unknown provider key")
	}

	cases := []struct {
		factor FactorDescriptor
		editor string
		enum   string
	}{
		{factor: FactorDescriptor{Filter: false}, editor: ""},
		{factor: FactorDescriptor{Filter: true, FilterKind: "enum"}, editor: "integer"},
		{factor: FactorDescriptor{Key: "field.market", Filter: true, FilterKind: "enum"}, editor: "singleSelect", enum: "market"},
		{factor: FactorDescriptor{Filter: true, FilterKind: "set"}, editor: "integerSet"},
		{factor: FactorDescriptor{Key: "kline_shape.shape_type", Category: "kline_shape", Filter: true, FilterKind: "set"}, editor: "multiSelect", enum: "kline_shape_type"},
		{factor: FactorDescriptor{Filter: true, FilterKind: "interval"}, editor: "range"},
		{factor: FactorDescriptor{Key: "featured.cash_flow_net_in", Filter: true, FilterKind: "interval_or_set"}, editor: "rangeOrSet", enum: "cash_flow_period"},
		{factor: FactorDescriptor{Filter: true, FilterKind: "position"}, editor: "indicatorCompare"},
		{factor: FactorDescriptor{Filter: true, FilterKind: "pattern"}, editor: "pattern"},
		{factor: FactorDescriptor{Filter: true, FilterKind: "unknown"}, editor: "range"},
	}
	for _, testCase := range cases {
		editor, enum, _ := conditionContract(testCase.factor)
		if editor != testCase.editor || enum != testCase.enum {
			t.Fatalf("conditionContract(%#v) = %q, %q", testCase.factor, editor, enum)
		}
	}

	editorCases := []struct {
		parameter ParameterDescriptor
		want      string
	}{
		{parameter: ParameterDescriptor{EditorType: "custom"}, want: "custom"},
		{parameter: ParameterDescriptor{Enum: "period"}, want: "select"},
		{parameter: ParameterDescriptor{Type: "integer_array"}, want: "multiNumber"},
		{parameter: ParameterDescriptor{Type: "number_array"}, want: "multiNumber"},
		{parameter: ParameterDescriptor{Type: "union"}, want: "union"},
		{parameter: ParameterDescriptor{Type: "string"}, want: "text"},
		{parameter: ParameterDescriptor{Type: "date"}, want: "date"},
		{parameter: ParameterDescriptor{Type: "timestamp"}, want: "date"},
		{parameter: ParameterDescriptor{Type: "integer"}, want: "number"},
	}
	for _, testCase := range editorCases {
		if got := parameterEditorType(testCase.parameter); got != testCase.want {
			t.Fatalf("parameterEditorType(%#v) = %q", testCase.parameter, got)
		}
	}

	helpCases := map[string]string{
		"days":            "统计窗口",
		"period":          "K 线周期",
		"term":            "财务数据周期",
		"duration":        "累计周期",
		"indicatorParams": "指标专用参数",
		"optionParam":     "期权参数联合类型",
		"other":           "可选参数",
	}
	for name, fragment := range helpCases {
		if help := parameterHelp(ParameterDescriptor{Name: name}); !strings.Contains(help, fragment) {
			t.Fatalf("parameterHelp(%q) = %q", name, help)
		}
	}
	if help := parameterHelp(ParameterDescriptor{Enum: "period"}); help != "从目录枚举中选择" {
		t.Fatalf("enum help = %q", help)
	}
}

func TestCatalogParameterBoundsAndAvailability(t *testing.T) {
	low, high := int64(0), int64(5000)
	if got := int64PointerAtLeast(&low, 1); *got != 1 {
		t.Fatalf("minimum = %d", *got)
	}
	if got := int64PointerAtLeast(&high, 1); got != &high {
		t.Fatal("valid minimum pointer was replaced")
	}
	if got := int64PointerAtMost(&high, 3650); *got != 3650 {
		t.Fatalf("maximum = %d", *got)
	}
	if got := int64PointerAtMost(&low, 3650); got != &low {
		t.Fatal("valid maximum pointer was replaced")
	}
	if got := int64PointerAtLeast(nil, 2); *got != 2 {
		t.Fatalf("nil minimum = %d", *got)
	}
	if got := int64PointerAtMost(nil, 2); *got != 2 {
		t.Fatalf("nil maximum = %d", *got)
	}

	for _, name := range []string{
		"days", "period", "term", "optionHvPeriod", "futureDuration",
		"rangePeriod", "periodAverage", "duration", "year",
		"firstCustomParam", "indicatorParams", "brokerParam", "optionParam", "other",
	} {
		required, defaultValue, minimum, maximum, step := parameterContract(ParameterDescriptor{Name: name})
		if minimum == nil || step == nil {
			t.Fatalf("%s missing contract bounds", name)
		}
		if name == "days" && (!required || maximum == nil || defaultValue == nil) {
			t.Fatalf("days contract = %v %#v %#v", required, defaultValue, maximum)
		}
	}

	hkBroker := factorAvailability(FactorDescriptor{Category: "broker", ProviderID: 6102}, "HK")
	if hkBroker.Availability != "unsupported" || hkBroker.Reason == "" {
		t.Fatalf("unsupported broker factor = %#v", hkBroker)
	}
	usOption := factorAvailability(FactorDescriptor{Category: "option"}, "US")
	if usOption.Availability != "available" {
		t.Fatalf("US option availability = %#v", usOption)
	}
	shOption := factorAvailability(FactorDescriptor{Category: "option"}, "SH")
	if shOption.Availability != "unsupported" || !strings.Contains(shOption.Reason, "SH") {
		t.Fatalf("SH option availability = %#v", shOption)
	}

	if _, err := ValidateFactorUse("simple.price", true, true, true); err != nil {
		t.Fatalf("valid factor use: %v", err)
	}
	if _, err := ValidateFactorUse("basic.name", true, false, false); err == nil {
		t.Fatal("non-filter factor accepted as a condition")
	}
}

func TestValidateCatalogRejectsIncompleteSemanticRows(t *testing.T) {
	originalFactors := generatedFactors
	originalEnums := generatedEnums
	t.Cleanup(func() {
		generatedFactors = originalFactors
		generatedEnums = originalEnums
	})

	valid := FactorDescriptor{
		Key: "test.factor", Category: "test", Label: "Test",
		Filter: true, Roles: []string{"condition"},
		ConditionEditor: "range", Operators: []string{"between"},
	}
	cases := []struct {
		name    string
		factors []FactorDescriptor
		enums   map[string][]EnumOption
	}{
		{name: "empty", factors: nil, enums: map[string][]EnumOption{}},
		{name: "identity", factors: []FactorDescriptor{{Filter: true}}, enums: map[string][]EnumOption{}},
		{name: "roles", factors: []FactorDescriptor{{Key: "x", Category: "x", Label: "x"}}, enums: map[string][]EnumOption{}},
		{name: "semantic roles", factors: []FactorDescriptor{{Key: "x", Category: "x", Label: "x", Filter: true}}, enums: map[string][]EnumOption{}},
		{name: "editor", factors: []FactorDescriptor{{Key: "x", Category: "x", Label: "x", Filter: true, Roles: []string{"condition"}}}, enums: map[string][]EnumOption{}},
		{name: "factor enum", factors: []FactorDescriptor{func() FactorDescriptor {
			value := valid
			value.ValueEnum = "missing"
			return value
		}()}, enums: map[string][]EnumOption{}},
		{name: "parameter identity", factors: []FactorDescriptor{func() FactorDescriptor {
			value := valid
			value.Parameters = []ParameterDescriptor{{}}
			return value
		}()}, enums: map[string][]EnumOption{}},
		{name: "parameter enum", factors: []FactorDescriptor{func() FactorDescriptor {
			value := valid
			value.Parameters = []ParameterDescriptor{{
				Name: "period", Type: "integer", EditorType: "select", Enum: "missing",
			}}
			return value
		}()}, enums: map[string][]EnumOption{}},
		{name: "required default", factors: []FactorDescriptor{func() FactorDescriptor {
			value := valid
			value.Parameters = []ParameterDescriptor{{
				Name: "days", Type: "integer", EditorType: "number", Required: true,
			}}
			return value
		}()}, enums: map[string][]EnumOption{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			generatedFactors = testCase.factors
			generatedEnums = testCase.enums
			if err := ValidateCatalog(); err == nil {
				t.Fatal("invalid catalog was accepted")
			}
		})
	}
}
