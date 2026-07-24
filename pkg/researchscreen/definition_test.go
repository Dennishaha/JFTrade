package researchscreen

import (
	"errors"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestNormalizeDefinitionPreservesParameterizedInstances(t *testing.T) {
	definition := broker.ScreenDefinitionV2{
		BrokerID: "futu", Market: "US",
		CatalogVersion: CatalogVersion, QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
		Columns: []broker.ScreenColumn{
			{ID: "ma20-column", Factor: broker.FactorRef{FactorKey: "indicator.ma", Params: broker.ResearchScreenFactorParams{Period: 11, IndicatorParams: []int64{20}}}},
			{ID: "ma60-column", Factor: broker.FactorRef{FactorKey: "indicator.ma", Params: broker.ResearchScreenFactorParams{Period: 11, IndicatorParams: []int64{60}}}},
		},
	}
	normalized, err := NormalizeDefinitionV2(definition)
	if err != nil {
		t.Fatalf("NormalizeDefinitionV2: %v", err)
	}
	if len(normalized.Columns) != 2 || normalized.Columns[0].Factor.InstanceID == "" ||
		normalized.Columns[0].Factor.InstanceID == normalized.Columns[1].Factor.InstanceID {
		t.Fatalf("normalized columns = %#v", normalized.Columns)
	}
	if got := normalized.Columns[0].Factor.Params.IndicatorParams[0]; got != 20 {
		t.Fatalf("MA(20) params = %#v", normalized.Columns[0].Factor.Params)
	}
	if got := normalized.Columns[1].Factor.Params.IndicatorParams[0]; got != 60 {
		t.Fatalf("MA(60) params = %#v", normalized.Columns[1].Factor.Params)
	}
}

func TestNormalizeDefinitionRequiresExplicitV2Versions(t *testing.T) {
	base := broker.ScreenDefinitionV2{
		Market: "US",
		Columns: []broker.ScreenColumn{{
			ID: "price", Factor: broker.FactorRef{InstanceID: "price", FactorKey: "simple.price"},
		}},
	}
	testCases := []struct {
		name       string
		definition broker.ScreenDefinitionV2
		path       string
	}{
		{name: "missing query version", definition: base, path: "querySchemaVersion"},
		{name: "V1 query version", definition: func() broker.ScreenDefinitionV2 {
			value := base
			value.QuerySchemaVersion = 1
			value.CatalogVersion = CatalogVersion
			return value
		}(), path: "querySchemaVersion"},
		{name: "missing catalog version", definition: func() broker.ScreenDefinitionV2 {
			value := base
			value.QuerySchemaVersion = broker.ScreenQuerySchemaVersionV2
			return value
		}(), path: "catalogVersion"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NormalizeDefinitionV2(testCase.definition)
			var fieldErr *FieldError
			if !errors.As(err, &fieldErr) || fieldErr.Path != testCase.path {
				t.Fatalf("version error = %#v", err)
			}
		})
	}
}

func TestNormalizeDefinitionRejectsDuplicateConfigurationWithFieldPath(t *testing.T) {
	definition := broker.ScreenDefinitionV2{
		Market: "US", CatalogVersion: CatalogVersion,
		QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
		Columns: []broker.ScreenColumn{
			{ID: "one", Factor: broker.FactorRef{InstanceID: "ma-one", FactorKey: "indicator.ma", Params: broker.ResearchScreenFactorParams{Period: 11, IndicatorParams: []int64{20}}}},
			{ID: "two", Factor: broker.FactorRef{InstanceID: "ma-two", FactorKey: "indicator.ma", Params: broker.ResearchScreenFactorParams{Period: 11, IndicatorParams: []int64{20}}}},
		},
	}
	_, err := NormalizeDefinitionV2(definition)
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Path != "columns[1].factor" || fieldErr.Code != "duplicate_factor" {
		t.Fatalf("duplicate error = %#v", err)
	}
}

func TestNormalizeDefinitionRejectsMarketIncompatibleFactor(t *testing.T) {
	definition := broker.ScreenDefinitionV2{
		Market: "US", CatalogVersion: CatalogVersion,
		QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
		Conditions: []broker.ScreenCondition{{
			ID: "broker", Factor: broker.FactorRef{InstanceID: "broker-holdings", FactorKey: "broker.holdings_ratio"},
			Operator: "between", Value: map[string]any{"min": 1.0, "max": 2.0},
		}},
	}
	_, err := NormalizeDefinitionV2(definition)
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Path != "conditions[0].factor.factorKey" {
		t.Fatalf("market error = %#v", err)
	}
}

func TestNormalizeDefinitionValidatesParameterTypesEnumsAndUnions(t *testing.T) {
	base := broker.ScreenDefinitionV2{
		Market:             "US",
		CatalogVersion:     CatalogVersion,
		QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
		Columns: []broker.ScreenColumn{{
			ID: "rsi",
			Factor: broker.FactorRef{
				InstanceID: "rsi", FactorKey: "indicator.rsi",
				Params: broker.ResearchScreenFactorParams{Period: 999},
			},
		}},
	}
	_, err := NormalizeDefinitionV2(base)
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) ||
		fieldErr.Path != "columns[0].factor.params.period" ||
		fieldErr.Code != "invalid_enum" {
		t.Fatalf("enum validation error = %#v", err)
	}

	base.Columns = []broker.ScreenColumn{{
		ID: "iv",
		Factor: broker.FactorRef{
			InstanceID: "iv", FactorKey: "option.stock_iv",
			Params: broker.ResearchScreenFactorParams{OptionParamType: 2},
		},
	}}
	_, err = NormalizeDefinitionV2(base)
	if !errors.As(err, &fieldErr) ||
		fieldErr.Path != "columns[0].factor.params.optionParam.integer" ||
		fieldErr.Code != "required" {
		t.Fatalf("union validation error = %#v", err)
	}
}

func TestNormalizeDefinitionRejectsWrongOperatorForFactorKind(t *testing.T) {
	definition := broker.ScreenDefinitionV2{
		Market:             "US",
		CatalogVersion:     CatalogVersion,
		QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
		Conditions: []broker.ScreenCondition{{
			ID: "ma",
			Factor: broker.FactorRef{
				InstanceID: "ma", FactorKey: "indicator.ma",
				Params: broker.ResearchScreenFactorParams{Period: 11},
			},
			Operator: "between",
			Value:    map[string]any{"min": 1.0},
		}},
	}
	_, err := NormalizeDefinitionV2(definition)
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) ||
		fieldErr.Path != "conditions[0].operator" ||
		fieldErr.Code != "unsupported_operator" {
		t.Fatalf("operator validation error = %#v", err)
	}
}
