package researchscreen

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogIsCompleteStableAndDoesNotExposeProviderEnums(t *testing.T) {
	catalog := FullCatalog()
	if catalog.Version != CatalogVersion || catalog.ProviderVersion != ProviderVersion {
		t.Fatalf("catalog versions = %q %q", catalog.Version, catalog.ProviderVersion)
	}
	if catalog.SchemaVersion != CatalogSchemaVersion || catalog.QuerySchemaVersion != QuerySchemaVersion {
		t.Fatalf("catalog schema versions = %d %d", catalog.SchemaVersion, catalog.QuerySchemaVersion)
	}
	if got := len(catalog.Factors); got != 402 {
		t.Fatalf("factor count = %d, want 402", got)
	}
	if len(catalog.Categories) != 11 {
		t.Fatalf("category count = %d, want 11", len(catalog.Categories))
	}
	if len(catalog.Enums["period"]) != 10 || len(catalog.Enums["term"]) != 14 {
		t.Fatalf("enum catalog is incomplete: %#v", catalog.Enums)
	}
	for _, key := range []string{
		"basic.code", "simple.price", "cumulative.price_change_pct",
		"financial.net_profit", "indicator.macd_dif", "pattern.macd_gold_cross",
		"featured.chips_profit_ratio", "broker.holdings_ratio",
		"option.stock_iv", "kline_shape.shape_type",
	} {
		if factor, ok := Lookup(key); !ok || factor.ProviderID == 0 {
			t.Fatalf("Lookup(%q) = %#v, %v", key, factor, ok)
		}
	}
	content, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "providerId") || strings.Contains(string(content), "ProviderID") {
		t.Fatalf("catalog leaked provider ids: %s", content)
	}
}

func TestCatalogParametersExposeEditorContract(t *testing.T) {
	for _, key := range []string{"cumulative.price_change_pct", "financial.roe", "indicator.ma", "option.stock_iv"} {
		factor, ok := Lookup(key)
		if !ok {
			t.Fatalf("factor %q not found", key)
		}
		for _, parameter := range factor.Parameters {
			if parameter.EditorType == "" || parameter.Default == nil || parameter.Step == nil || parameter.Minimum == nil {
				t.Fatalf("%s parameter contract incomplete: %#v", key, parameter)
			}
		}
		if len(factor.Roles) == 0 || factor.Help == "" || len(factor.SearchKeywords) == 0 {
			t.Fatalf("%s semantic contract incomplete: %#v", key, factor)
		}
	}
	if err := ValidateCatalog(); err != nil {
		t.Fatal(err)
	}
	catalog := FullCatalog()
	for _, factor := range catalog.Factors {
		if !factor.Filter {
			continue
		}
		if factor.ConditionEditor == "" || len(factor.Operators) == 0 {
			t.Fatalf("%s condition contract incomplete: %#v", factor.Key, factor)
		}
		if factor.ValueEnum != "" && len(catalog.Enums[factor.ValueEnum]) == 0 {
			t.Fatalf("%s value enum %q is missing", factor.Key, factor.ValueEnum)
		}
	}
}

func TestValidateFactorUseRejectsUnsupportedPurposes(t *testing.T) {
	if _, err := ValidateFactorUse("field.market", false, true, false); err == nil {
		t.Fatal("field.market unexpectedly supports retrieve")
	}
	if _, err := ValidateFactorUse("pattern.macd_gold_cross", false, false, true); err == nil {
		t.Fatal("pattern unexpectedly supports sort")
	}
	if _, err := ValidateFactorUse("missing.factor", true, false, false); err == nil {
		t.Fatal("unknown factor was accepted")
	}
	if _, err := ValidateFactorForMarket("broker.holdings_ratio", "US", true, false, false); err == nil {
		t.Fatal("HK-only broker factor was accepted in US")
	}
	if factor, ok := Lookup("broker.broker_rank"); !ok {
		t.Fatal("complete catalog omitted an unsupported documented factor")
	} else if decorated := factorAvailability(factor, "HK"); decorated.Availability != "unsupported" {
		t.Fatalf("broker rank availability = %#v", decorated)
	}
}

func TestFactorDisplaySemanticsAreExplicitAndCorrected(t *testing.T) {
	testCases := []struct {
		key           string
		unit          string
		currencyBasis string
		displayFormat string
	}{
		{key: "simple.price", unit: "currency", currencyBasis: "quote", displayFormat: "price"},
		{key: "simple.market_cap", unit: "currency", currencyBasis: "quote", displayFormat: "compact_amount"},
		{key: "financial.net_profit", unit: "currency", currencyBasis: "reporting", displayFormat: "compact_amount"},
		{key: "financial.float_market_cap", unit: "currency", currencyBasis: "quote", displayFormat: "compact_amount"},
		{key: "financial.equity_multiplier", displayFormat: "number"},
		{key: "financial.money_turnover_cycle", unit: "days", displayFormat: "integer"},
		{key: "financial.stockholder_profit_cagr", unit: "percent", displayFormat: "percent"},
		{key: "financial.surprise_revenue_date", unit: "timestamp", displayFormat: "timestamp"},
		{key: "featured.cash_flow_net_in_count", unit: "count", displayFormat: "integer"},
		{key: "indicator.ma", unit: "currency", currencyBasis: "quote", displayFormat: "price"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.key, func(t *testing.T) {
			factor, ok := Lookup(testCase.key)
			if !ok {
				t.Fatalf("factor %q not found", testCase.key)
			}
			if factor.Unit != testCase.unit ||
				factor.CurrencyBasis != testCase.currencyBasis ||
				factor.DisplayFormat != testCase.displayFormat {
				t.Fatalf(
					"factor semantics = unit %q basis %q format %q",
					factor.Unit,
					factor.CurrencyBasis,
					factor.DisplayFormat,
				)
			}
		})
	}
}
