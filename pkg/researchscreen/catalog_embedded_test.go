package researchscreen

import (
	"errors"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func embeddedScreenDefinition() broker.ScreenDefinitionV2 {
	return broker.ScreenDefinitionV2{
		Market:             "SH",
		CatalogVersion:     EmbeddedCatalogVersion,
		QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
		Conditions: []broker.ScreenCondition{{
			Factor:   broker.FactorRef{FactorKey: "simple.pe_ttm"},
			Operator: "between",
			Value:    map[string]any{"min": 0.0, "max": 30.0},
		}},
		Columns: []broker.ScreenColumn{
			{Factor: broker.FactorRef{FactorKey: "simple.price"}},
			{Factor: broker.FactorRef{FactorKey: "basic.name"}},
		},
		Sorts: []broker.ScreenSort{{
			Factor: broker.FactorRef{FactorKey: "simple.change_pct"}, Direction: "desc",
		}},
	}
}

func TestEmbeddedCatalogShapeAndSemantics(t *testing.T) {
	catalog := EmbeddedCatalog("akshare", "")
	if catalog.Version != EmbeddedCatalogVersion || catalog.Provider != "akshare" ||
		catalog.QuerySchemaVersion != QuerySchemaVersion {
		t.Fatalf("catalog header = %#v", catalog)
	}
	if len(catalog.Factors) != 9 {
		t.Fatalf("embedded factors = %d, want 9", len(catalog.Factors))
	}
	wantMarkets := []string{"SH", "SZ", "CN", "HK", "US"}
	if strings.Join(catalog.Markets, ",") != strings.Join(wantMarkets, ",") {
		t.Fatalf("akshare markets = %v", catalog.Markets)
	}
	yfinance := EmbeddedCatalog("yfinance", "US")
	if yfinance.Market != "US" || len(yfinance.Markets) != 1 || yfinance.Markets[0] != "US" {
		t.Fatalf("yfinance catalog = %#v", yfinance.Markets)
	}

	price, ok := LookupEmbedded("simple.price")
	if !ok || !price.Filter || !price.Retrieve || !price.Sort ||
		price.FilterKind != "interval" || price.Unit != "currency" ||
		price.DisplayFormat != "price" || len(price.Operators) != 1 || price.Operators[0] != "between" {
		t.Fatalf("price factor = %#v", price)
	}
	changePct, ok := LookupEmbedded("SIMPLE.CHANGE_PCT")
	if !ok || changePct.Unit != "percent" || changePct.DisplayFormat != "percent" {
		t.Fatalf("change_pct factor = %#v", changePct)
	}
	volume, ok := LookupEmbedded("simple.volume")
	if !ok || volume.Unit != "shares" || volume.DisplayFormat != "integer" || volume.ValueType != "integer" {
		t.Fatalf("volume factor = %#v", volume)
	}
	code, ok := LookupEmbedded("basic.code")
	if !ok || code.Filter || !code.Retrieve || !code.Sort {
		t.Fatalf("basic.code roles = %#v", code)
	}
	industry, ok := LookupEmbedded("basic.industry")
	if !ok || industry.Filter || industry.Sort || !industry.Retrieve {
		t.Fatalf("basic.industry roles = %#v", industry)
	}
	if _, ok := LookupEmbedded("indicator.ma"); ok {
		t.Fatal("generated-only factor must not appear in the embedded catalog")
	}
}

func TestEmbeddedCatalogValidationUseFlags(t *testing.T) {
	if _, err := ValidateEmbeddedFactorUse("basic.code", true, false, false); err == nil {
		t.Fatal("basic.code must reject filtering")
	}
	if _, err := ValidateEmbeddedFactorUse("basic.industry", false, false, true); err == nil {
		t.Fatal("basic.industry must reject sorting")
	}
	if _, err := ValidateEmbeddedFactorUse("simple.market_cap", true, true, true); err != nil {
		t.Fatalf("simple.market_cap full roles: %v", err)
	}
	if _, err := ValidateEmbeddedFactorUse("cumulative.change_5d", true, false, false); err == nil ||
		!strings.Contains(err.Error(), "unknown embedded") {
		t.Fatalf("futu-only factor error = %v", err)
	}
}

func TestNormalizeDefinitionV2AcceptsEmbeddedCatalog(t *testing.T) {
	normalized, err := NormalizeDefinitionV2(embeddedScreenDefinition())
	if err != nil {
		t.Fatalf("embedded definition: %v", err)
	}
	if normalized.CatalogVersion != EmbeddedCatalogVersion ||
		normalized.Conditions[0].Operator != "between" {
		t.Fatalf("normalized = %#v", normalized)
	}

	cn := embeddedScreenDefinition()
	cn.Market = "cn"
	if _, err := NormalizeDefinitionV2(cn); err != nil {
		t.Fatalf("CN market must be accepted for the embedded catalog: %v", err)
	}
	if cn.Market = "MO"; true {
		if _, err := NormalizeDefinitionV2(cn); err == nil {
			t.Fatal("unknown market must be rejected for the embedded catalog")
		}
	}
}

func TestNormalizeDefinitionV2EmbeddedRejectsOutOfCatalogFactors(t *testing.T) {
	definition := embeddedScreenDefinition()
	definition.Conditions[0].Factor.FactorKey = "indicator.ma"
	var fieldErr *FieldError
	if _, err := NormalizeDefinitionV2(definition); !errors.As(err, &fieldErr) ||
		fieldErr.Code != "unsupported_factor" {
		t.Fatalf("generated-only condition factor error = %v", err)
	}

	definition = embeddedScreenDefinition()
	definition.Columns = append(definition.Columns, broker.ScreenColumn{
		Factor: broker.FactorRef{FactorKey: "cumulative.change_5d"},
	})
	if _, err := NormalizeDefinitionV2(definition); err == nil {
		t.Fatal("generated-only column factor must be rejected")
	}

	definition = embeddedScreenDefinition()
	definition.Conditions[0].Operator = "gt"
	var operatorErr *FieldError
	if _, err := NormalizeDefinitionV2(definition); !errors.As(err, &operatorErr) ||
		operatorErr.Code != "unsupported_operator" {
		t.Fatalf("non-interval operator error = %v", err)
	}
}

// The Futu catalog contract must stay byte-identical: unknown versions, CN
// market, and generated factors behave exactly as before this change.
func TestNormalizeDefinitionV2FutuContractUnchanged(t *testing.T) {
	definition := embeddedScreenDefinition()
	definition.CatalogVersion = CatalogVersion
	definition.Conditions[0].Factor.FactorKey = "simple.pe_ttm"
	definition.Sorts[0].Factor.FactorKey = "simple.price"
	if _, err := NormalizeDefinitionV2(definition); err != nil {
		t.Fatalf("futu definition: %v", err)
	}

	cn := definition
	cn.Market = "CN"
	var marketErr *FieldError
	if _, err := NormalizeDefinitionV2(cn); !errors.As(err, &marketErr) ||
		marketErr.Code != "unsupported_market" {
		t.Fatalf("futu CN market error = %v", marketErr)
	}

	definition.CatalogVersion = "embedded-stock-screen-v0"
	var catalogErr *FieldError
	if _, err := NormalizeDefinitionV2(definition); !errors.As(err, &catalogErr) ||
		catalogErr.Code != "unsupported_catalog" {
		t.Fatalf("unknown catalog error = %v", catalogErr)
	}
}
