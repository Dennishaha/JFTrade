package researchscreen

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestDefinitionHelperContractsCoverSupportedValueShapes(t *testing.T) {
	if (*FieldError)(nil).Error() != "research screen validation failed" {
		t.Fatal("nil FieldError text changed")
	}
	if (&FieldError{Message: "message"}).Error() != "message" {
		t.Fatal("pathless FieldError text changed")
	}
	if !reflect.DeepEqual(cleanIDs([]string{" a ", "", "a", "b"}), []string{"a", "b"}) {
		t.Fatal("cleanIDs did not trim and deduplicate")
	}
	if operator := inferOperator(map[string]any{"min": 1}); operator != "between" {
		t.Fatalf("map operator = %q", operator)
	}
	if operator := inferOperator([]any{1}); operator != "in" {
		t.Fatalf("slice operator = %q", operator)
	}
	if operator := inferOperator("value"); operator != "eq" {
		t.Fatalf("scalar operator = %q", operator)
	}

	typed, ok := numericSlice([]int64{1, 2})
	if !ok || !reflect.DeepEqual(typed, []int64{1, 2}) {
		t.Fatalf("typed numeric slice = %#v, %v", typed, ok)
	}
	decoded, ok := numericSlice([]any{float64(3), json.Number("4")})
	if !ok || !reflect.DeepEqual(decoded, []int64{3, 4}) {
		t.Fatalf("decoded numeric slice = %#v, %v", decoded, ok)
	}
	for _, invalid := range []any{"not-a-slice", []any{"bad"}} {
		if _, ok := numericSlice(invalid); ok {
			t.Fatalf("numericSlice accepted %#v", invalid)
		}
	}
	for _, value := range []any{float64(1), float32(1), int(1), int32(1), int64(1), json.Number("1")} {
		if number, ok := numericJSONValue(value); !ok || number != 1 {
			t.Fatalf("numericJSONValue(%T) = %v, %v", value, number, ok)
		}
	}
	if _, ok := numericJSONValue("1"); ok {
		t.Fatal("numericJSONValue accepted a string")
	}
}

func TestParameterValidationRejectsTypeRangeStepAndEnumErrors(t *testing.T) {
	minimum, maximum, step := int64(2), int64(10), float64(2)
	base := ParameterDescriptor{
		Type: "integer", Minimum: &minimum, Maximum: &maximum, Step: &step,
	}
	cases := []struct {
		name      string
		parameter ParameterDescriptor
		value     any
		code      string
	}{
		{name: "string type", parameter: ParameterDescriptor{Type: "string"}, value: 1, code: "invalid_type"},
		{name: "array type", parameter: ParameterDescriptor{Type: "integer_array"}, value: 1, code: "invalid_type"},
		{name: "non numeric", parameter: ParameterDescriptor{Type: "number_array"}, value: []any{"bad"}, code: "invalid_type"},
		{name: "nan", parameter: ParameterDescriptor{Type: "number"}, value: math.NaN(), code: "invalid_type"},
		{name: "fractional integer", parameter: base, value: 2.5, code: "invalid_type"},
		{name: "minimum", parameter: base, value: 0, code: "minimum"},
		{name: "maximum", parameter: base, value: 12, code: "maximum"},
		{name: "step", parameter: base, value: 3, code: "step"},
		{name: "enum", parameter: ParameterDescriptor{Type: "integer", Enum: "period"}, value: 999, code: "invalid_enum"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var fieldErr *FieldError
			err := validateParameterValue("params.value", testCase.parameter, testCase.value)
			if !errors.As(err, &fieldErr) || fieldErr.Code != testCase.code {
				t.Fatalf("error = %#v", err)
			}
		})
	}
	if err := validateParameterValue("params.value", ParameterDescriptor{Type: "string"}, "ok"); err != nil {
		t.Fatalf("valid string: %v", err)
	}
	if err := validateParameterValue("params.value", base, 4); err != nil {
		t.Fatalf("valid integer: %v", err)
	}
}

func TestUnionValidationCoversEveryProviderShape(t *testing.T) {
	valid := []map[string]any{
		{},
		{"optionParamType": float64(1), "optionParamString": "call"},
		{"optionParamType": float64(2), "optionParamInteger": float64(7)},
		{"optionParamType": float64(3), "optionParamIntegers": []any{float64(7), float64(8)}},
	}
	for _, values := range valid {
		if err := validateUnionParameter("option", "optionParam", values); err != nil {
			t.Fatalf("valid union %#v: %v", values, err)
		}
	}
	invalid := []struct {
		name   string
		values map[string]any
	}{
		{name: "unsupported", values: map[string]any{"name": "other"}},
		{name: "type required", values: map[string]any{"optionParamString": "call"}},
		{name: "type integer", values: map[string]any{"optionParamType": 1.5}},
		{name: "string required", values: map[string]any{"optionParamType": 1}},
		{name: "integer required", values: map[string]any{"optionParamType": 2}},
		{name: "integers required", values: map[string]any{"optionParamType": 3}},
		{name: "integers typed", values: map[string]any{"optionParamType": 3, "optionParamIntegers": []any{"bad"}}},
		{name: "type enum", values: map[string]any{"optionParamType": 4}},
	}
	for _, testCase := range invalid {
		t.Run(testCase.name, func(t *testing.T) {
			name := "optionParam"
			if testCase.name == "unsupported" {
				name = "other"
			}
			if err := validateUnionParameter("option", name, testCase.values); err == nil {
				t.Fatalf("union %#v was accepted", testCase.values)
			}
		})
	}
}

func TestConditionValueValidationCoversSetRangePositionAndPattern(t *testing.T) {
	if err := validateSetConditionValue("condition", []int64{1}); err != nil {
		t.Fatalf("valid set: %v", err)
	}
	for _, value := range []any{[]any{}, "bad"} {
		if err := validateSetConditionValue("condition", value); err == nil {
			t.Fatalf("invalid set %#v accepted", value)
		}
	}

	validRanges := []any{
		map[string]any{"min": float64(1)},
		map[string]any{"max": float64(2)},
		map[string]any{"intervals": []any{map[string]any{"min": float64(1), "max": float64(2)}}},
	}
	for _, value := range validRanges {
		if err := validateBetweenConditionValue("condition", value); err != nil {
			t.Fatalf("valid range %#v: %v", value, err)
		}
	}
	invalidRanges := []any{
		"bad",
		map[string]any{},
		map[string]any{"min": float64(3), "max": float64(2)},
		map[string]any{"intervals": "bad"},
		map[string]any{"intervals": []any{"bad"}},
		map[string]any{"intervals": []any{map[string]any{}}},
	}
	for _, value := range invalidRanges {
		if err := validateBetweenConditionValue("condition", value); err == nil {
			t.Fatalf("invalid range %#v accepted", value)
		}
	}

	position := broker.ScreenCondition{Value: map[string]any{"position": float64(1), "secondValue": float64(2)}}
	if err := validatePositionConditionValue("condition", &position); err != nil {
		t.Fatalf("valid position: %v", err)
	}
	second := broker.FactorRef{FactorKey: "indicator.ma"}
	position.SecondFactor = &second
	delete(position.Value.(map[string]any), "secondValue")
	if err := validatePositionConditionValue("condition", &position); err != nil {
		t.Fatalf("factor position: %v", err)
	}
	for _, value := range []any{
		"bad",
		map[string]any{"position": float64(5)},
		map[string]any{"position": float64(1)},
	} {
		position := broker.ScreenCondition{Value: value}
		if err := validatePositionConditionValue("condition", &position); err == nil {
			t.Fatalf("invalid position %#v accepted", value)
		}
	}

	for _, value := range []any{map[string]any{}, map[string]any{"match": true}} {
		if err := validatePatternConditionValue("condition", value); err != nil {
			t.Fatalf("valid pattern %#v: %v", value, err)
		}
	}
	for _, value := range []any{"bad", map[string]any{"match": "yes"}} {
		if err := validatePatternConditionValue("condition", value); err == nil {
			t.Fatalf("invalid pattern %#v accepted", value)
		}
	}
}

func TestDefinitionNormalizationCoversPoolsSortsAndSecondFactors(t *testing.T) {
	second := broker.FactorRef{
		FactorKey: "indicator.ema",
		Params: broker.ResearchScreenFactorParams{
			Period: 11, IndicatorParams: []int64{20},
		},
	}
	definition := broker.ScreenDefinitionV2{
		Market:             " hk ",
		CatalogVersion:     CatalogVersion,
		QuerySchemaVersion: QuerySchemaVersion,
		Pool: broker.ResearchScreenPool{
			WatchlistStockIDs: []string{" 123 "},
			Plates: []broker.ResearchScreenPlate{{
				ParentPlateID: " parent ", PlateIDs: []string{" plate ", "plate"},
			}},
		},
		Conditions: []broker.ScreenCondition{{
			Factor: broker.FactorRef{
				FactorKey: "indicator.ma",
				Params: broker.ResearchScreenFactorParams{
					Period: 11, IndicatorParams: []int64{10},
				},
			},
			Operator:     "position",
			Value:        map[string]any{"position": float64(1)},
			SecondFactor: &second,
		}},
		Sorts: []broker.ScreenSort{{
			Factor: broker.FactorRef{FactorKey: "simple.price"},
		}},
	}
	normalized, err := NormalizeDefinitionV2(definition)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.BrokerID != defaultBroker || normalized.Market != "HK" ||
		normalized.Pool.WatchlistStockIDs[0] != "123" ||
		normalized.Pool.Plates[0].ParentPlateID != "parent" ||
		len(normalized.Pool.Plates[0].PlateIDs) != 1 ||
		normalized.Sorts[0].Direction != "desc" ||
		normalized.Sorts[0].ID != "sort-1" ||
		normalized.Conditions[0].SecondFactor.InstanceID == "" {
		t.Fatalf("normalized definition = %#v", normalized)
	}
	if err := ValidateDefinitionV2(normalized); err != nil {
		t.Fatalf("ValidateDefinitionV2: %v", err)
	}
}

func TestDefinitionNormalizationRejectsPoolSortAndIdentityErrors(t *testing.T) {
	base := broker.ScreenDefinitionV2{
		Market: "US", CatalogVersion: CatalogVersion, QuerySchemaVersion: QuerySchemaVersion,
	}
	cases := []broker.ScreenDefinitionV2{
		func() broker.ScreenDefinitionV2 {
			value := base
			value.Market = "SG"
			return value
		}(),
		func() broker.ScreenDefinitionV2 {
			value := base
			value.Pool.Plates = []broker.ResearchScreenPlate{{}}
			return value
		}(),
		func() broker.ScreenDefinitionV2 {
			value := base
			value.Pool.WatchlistStockIDs = []string{""}
			return value
		}(),
		func() broker.ScreenDefinitionV2 {
			value := base
			value.Pool.WatchlistStockIDs = []string{"not-number"}
			return value
		}(),
		func() broker.ScreenDefinitionV2 {
			value := base
			value.Sorts = []broker.ScreenSort{{
				Direction: "sideways",
				Factor:    broker.FactorRef{FactorKey: "simple.price"},
			}}
			return value
		}(),
		func() broker.ScreenDefinitionV2 {
			value := base
			value.Sorts = []broker.ScreenSort{{
				Factor: broker.FactorRef{FactorKey: "field.market"},
			}}
			return value
		}(),
		func() broker.ScreenDefinitionV2 {
			value := base
			value.Columns = []broker.ScreenColumn{
				{ID: "same", Factor: broker.FactorRef{FactorKey: "simple.price"}},
				{ID: "same", Factor: broker.FactorRef{FactorKey: "simple.market_cap"}},
			}
			return value
		}(),
		func() broker.ScreenDefinitionV2 {
			value := base
			value.Conditions = []broker.ScreenCondition{
				{ID: "same", Factor: broker.FactorRef{FactorKey: "simple.price"}, Operator: "between", Value: map[string]any{"min": 1}},
				{ID: "same", Factor: broker.FactorRef{FactorKey: "simple.market_cap"}, Operator: "between", Value: map[string]any{"min": 1}},
			}
			return value
		}(),
	}
	for index, definition := range cases {
		if _, err := NormalizeDefinitionV2(definition); err == nil {
			t.Fatalf("case %d was accepted", index)
		}
	}
}
