package rustmigration

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

const stage9ResearchDefinitionNormalizationFixtureVersion = "stage9.research-definition-normalization.v1"

type stage9ResearchDefinitionNormalizationFixture struct {
	Version string                                             `json:"version"`
	Cases   []stage9ResearchDefinitionNormalizationFixtureCase `json:"cases"`
}

type stage9ResearchDefinitionNormalizationFixtureCase struct {
	Name       string                              `json:"name"`
	SourceTest string                              `json:"sourceTest"`
	Input      json.RawMessage                     `json:"input"`
	Normalized json.RawMessage                     `json:"normalized,omitempty"`
	Error      *stage9ResearchDefinitionFieldError `json:"error,omitempty"`
}

type stage9ResearchDefinitionFieldError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type stage9ResearchDefinitionNormalizationSpec struct {
	Name       string
	SourceTest string
	Input      string
}

// TestStage9ResearchDefinitionNormalizationFixtureMatchesCurrentGoOwner
// freezes the public NormalizeDefinitionV2 success and FieldError contract.
// Every case is derived from the existing pkg/researchscreen tests named by
// SourceTest; this file is a migration projection, not a second truth source.
func TestStage9ResearchDefinitionNormalizationFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 research definition normalization fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/research-definition-normalization.json")
	want := stage9ResearchDefinitionNormalizationFixture{
		Version: stage9ResearchDefinitionNormalizationFixtureVersion,
		Cases:   make([]stage9ResearchDefinitionNormalizationFixtureCase, 0),
	}
	for _, spec := range stage9ResearchDefinitionNormalizationSpecs() {
		want.Cases = append(want.Cases, runStage9ResearchDefinitionNormalizationCase(t, spec))
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode research definition normalization fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write research definition normalization fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read research definition normalization fixture: %v", err)
	}
	var got stage9ResearchDefinitionNormalizationFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode research definition normalization fixture: %v", err)
	}
	compactStage9ResearchDefinitionNormalizationFixture(&got)
	compactStage9ResearchDefinitionNormalizationFixture(&want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 research definition normalization fixture drifted from the Go owner")
	}
}

func runStage9ResearchDefinitionNormalizationCase(
	t *testing.T,
	spec stage9ResearchDefinitionNormalizationSpec,
) stage9ResearchDefinitionNormalizationFixtureCase {
	t.Helper()
	input := json.RawMessage(spec.Input)
	var definition broker.ScreenDefinitionV2
	if err := json.Unmarshal(input, &definition); err != nil {
		t.Fatalf("decode %s input: %v", spec.Name, err)
	}
	entry := stage9ResearchDefinitionNormalizationFixtureCase{
		Name: spec.Name, SourceTest: spec.SourceTest, Input: input,
	}
	normalized, err := researchscreen.NormalizeDefinitionV2(definition)
	if err != nil {
		var fieldErr *researchscreen.FieldError
		if !errors.As(err, &fieldErr) {
			t.Fatalf("%s returned non-field error: %v", spec.Name, err)
		}
		entry.Error = &stage9ResearchDefinitionFieldError{
			Path: fieldErr.Path, Code: fieldErr.Code, Message: fieldErr.Message,
		}
		return entry
	}
	contents, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("encode %s normalized definition: %v", spec.Name, err)
	}
	entry.Normalized = contents
	return entry
}

func compactStage9ResearchDefinitionNormalizationFixture(
	fixture *stage9ResearchDefinitionNormalizationFixture,
) {
	for index := range fixture.Cases {
		fixture.Cases[index].Input = compactStage9ResearchDefinitionJSON(fixture.Cases[index].Input)
		fixture.Cases[index].Normalized = compactStage9ResearchDefinitionJSON(fixture.Cases[index].Normalized)
	}
}

func compactStage9ResearchDefinitionJSON(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return nil
	}
	var output bytes.Buffer
	if err := json.Compact(&output, input); err != nil {
		return input
	}
	return output.Bytes()
}

func stage9ResearchDefinitionNormalizationSpecs() []stage9ResearchDefinitionNormalizationSpec {
	const base = `{"market":"US","pool":{},"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`
	return []stage9ResearchDefinitionNormalizationSpec{
		{Name: "empty-valid-definition", SourceTest: "TestNormalizeDefinitionV2FutuContractUnchanged", Input: base},
		{Name: "canonical-header-pool-sort-and-second-factor", SourceTest: "TestDefinitionNormalizationCoversPoolsSortsAndSecondFactors", Input: `{"brokerId":" FUTU ","market":" hk ","pool":{"watchlistStockIds":[" 123 "],"plates":[{"parentPlateId":" parent ","plateIds":[" plate ","plate",""]}]},"conditions":[{"factor":{"factorKey":"indicator.ma","params":{"period":11,"indicatorParams":[10]}},"operator":" POSITION ","value":{"position":1},"secondFactor":{"factorKey":"indicator.ema","params":{"period":11,"indicatorParams":[20]}}}],"sorts":[{"factor":{"factorKey":"simple.price","params":{}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "parameterized-instance-identities", SourceTest: "TestNormalizeDefinitionPreservesParameterizedInstances", Input: `{"market":"US","pool":{},"columns":[{"columnId":"ma20-column","factor":{"factorKey":"indicator.ma","params":{"period":11,"indicatorParams":[20]}}},{"columnId":"ma60-column","factor":{"factorKey":"indicator.ma","params":{"period":11,"indicatorParams":[60]}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "embedded-catalog", SourceTest: "TestNormalizeDefinitionV2AcceptsEmbeddedCatalog", Input: `{"brokerId":" AKSHARE ","market":"cn","pool":{},"conditions":[{"factor":{"factorKey":"simple.pe_ttm","params":{}},"operator":"between","value":{"min":0,"max":30}}],"columns":[{"factor":{"factorKey":"simple.price","params":{}}},{"factor":{"factorKey":"basic.name","params":{}}}],"sorts":[{"factor":{"factorKey":"simple.change_pct","params":{}},"direction":"DESC"}],"catalogVersion":" embedded-stock-screen-v1 ","querySchemaVersion":2}`},
		{Name: "catalog-defaults", SourceTest: "TestDefinitionNormalizationCoversPoolsSortsAndSecondFactors", Input: `{"market":"US","pool":{},"columns":[{"factor":{"factorKey":"cumulative.price_change","params":{}}},{"factor":{"factorKey":"financial.net_profit","params":{}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "inferred-set-operator", SourceTest: "TestDefinitionHelperContractsCoverSupportedValueShapes", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"field.market","params":{}},"value":[2]}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "inferred-scalar-operator", SourceTest: "TestDefinitionHelperContractsCoverSupportedValueShapes", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"field.has_option","params":{}},"value":1}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "between-intervals", SourceTest: "TestConditionValueValidationCoversSetRangePositionAndPattern", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"simple.price","params":{}},"operator":"between","value":{"intervals":[{"min":1,"max":2},{"min":5}]}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "pattern-value", SourceTest: "TestConditionValueValidationCoversSetRangePositionAndPattern", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"pattern.ma_long","params":{}},"operator":"pattern","value":{"match":true}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "missing-query-schema", SourceTest: "TestNormalizeDefinitionRequiresExplicitV2Versions", Input: `{"market":"US","pool":{},"catalogVersion":"futu-stock-screen-v1"}`},
		{Name: "v1-query-schema", SourceTest: "TestNormalizeDefinitionRequiresExplicitV2Versions", Input: `{"market":"US","pool":{},"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":1}`},
		{Name: "missing-catalog", SourceTest: "TestNormalizeDefinitionRequiresExplicitV2Versions", Input: `{"market":"US","pool":{},"querySchemaVersion":2}`},
		{Name: "unsupported-catalog", SourceTest: "TestNormalizeDefinitionV2FutuContractUnchanged", Input: `{"market":"US","pool":{},"catalogVersion":"embedded-stock-screen-v0","querySchemaVersion":2}`},
		{Name: "unsupported-futu-market", SourceTest: "TestDefinitionNormalizationRejectsPoolSortAndIdentityErrors", Input: `{"market":"SG","pool":{},"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "unsupported-embedded-market", SourceTest: "TestNormalizeDefinitionV2AcceptsEmbeddedCatalog", Input: `{"market":"MO","pool":{},"catalogVersion":"embedded-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "plate-ids-required", SourceTest: "TestDefinitionNormalizationRejectsPoolSortAndIdentityErrors", Input: `{"market":"US","pool":{"plates":[{"plateIds":[]}]},"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "watchlist-id-required", SourceTest: "TestDefinitionNormalizationRejectsPoolSortAndIdentityErrors", Input: `{"market":"US","pool":{"watchlistStockIds":[""]},"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "watchlist-id-invalid", SourceTest: "TestDefinitionNormalizationRejectsPoolSortAndIdentityErrors", Input: `{"market":"US","pool":{"watchlistStockIds":["not-number"]},"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "factor-key-required", SourceTest: "TestDefinitionNormalizationRejectsPoolSortAndIdentityErrors", Input: `{"market":"US","pool":{},"columns":[{"factor":{"params":{}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "factor-unknown", SourceTest: "TestNormalizeDefinitionV2EmbeddedRejectsOutOfCatalogFactors", Input: `{"market":"US","pool":{},"columns":[{"factor":{"factorKey":"unknown.factor","params":{}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "factor-role-invalid", SourceTest: "TestEmbeddedCatalogValidationUseFlags", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"basic.name","params":{}},"operator":"eq","value":"A"}],"catalogVersion":"embedded-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "factor-market-invalid", SourceTest: "TestNormalizeDefinitionRejectsMarketIncompatibleFactor", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"broker.holdings_ratio","params":{}},"operator":"between","value":{"min":1,"max":2}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "parameter-invalid-enum", SourceTest: "TestNormalizeDefinitionValidatesParameterTypesEnumsAndUnions", Input: `{"market":"US","pool":{},"columns":[{"factor":{"factorKey":"indicator.rsi","params":{"period":999}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "parameter-union-member-required", SourceTest: "TestNormalizeDefinitionValidatesParameterTypesEnumsAndUnions", Input: `{"market":"US","pool":{},"columns":[{"factor":{"factorKey":"option.stock_iv","params":{"optionParamType":2}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "parameter-minimum", SourceTest: "TestParameterValidationRejectsTypeRangeStepAndEnumErrors", Input: `{"market":"US","pool":{},"columns":[{"factor":{"factorKey":"cumulative.price_change","params":{"days":-1}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "parameter-maximum", SourceTest: "TestParameterValidationRejectsTypeRangeStepAndEnumErrors", Input: `{"market":"US","pool":{},"columns":[{"factor":{"factorKey":"cumulative.price_change","params":{"days":3651}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "duplicate-condition-id", SourceTest: "TestDefinitionNormalizationRejectsPoolSortAndIdentityErrors", Input: `{"market":"US","pool":{},"conditions":[{"id":"same","factor":{"factorKey":"simple.price","params":{}},"operator":"between","value":{"min":1}},{"id":"same","factor":{"factorKey":"simple.market_cap","params":{}},"operator":"between","value":{"min":1}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "duplicate-condition-factor", SourceTest: "TestNormalizeDefinitionRejectsDuplicateConfigurationWithFieldPath", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"simple.price","params":{}},"operator":"between","value":{"min":1}},{"factor":{"factorKey":"SIMPLE.PRICE","params":{}},"operator":"between","value":{"max":2}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "duplicate-column-id", SourceTest: "TestDefinitionNormalizationRejectsPoolSortAndIdentityErrors", Input: `{"market":"US","pool":{},"columns":[{"columnId":"same","factor":{"factorKey":"simple.price","params":{}}},{"columnId":"same","factor":{"factorKey":"simple.market_cap","params":{}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "duplicate-column-factor", SourceTest: "TestNormalizeDefinitionRejectsDuplicateConfigurationWithFieldPath", Input: `{"market":"US","pool":{},"columns":[{"factor":{"factorKey":"indicator.ma","params":{"period":11,"indicatorParams":[20]}}},{"factor":{"factorKey":"indicator.ma","params":{"period":11,"indicatorParams":[20]}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "sort-direction-invalid", SourceTest: "TestDefinitionNormalizationRejectsPoolSortAndIdentityErrors", Input: `{"market":"US","pool":{},"sorts":[{"factor":{"factorKey":"simple.price","params":{}},"direction":"sideways"}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "sort-factor-role-invalid", SourceTest: "TestDefinitionNormalizationRejectsPoolSortAndIdentityErrors", Input: `{"market":"US","pool":{},"sorts":[{"factor":{"factorKey":"field.market","params":{}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "condition-operator-unknown", SourceTest: "TestNormalizeDefinitionRejectsWrongOperatorForFactorKind", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"simple.price","params":{}},"operator":"sideways","value":1}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "condition-operator-wrong-kind", SourceTest: "TestNormalizeDefinitionRejectsWrongOperatorForFactorKind", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"indicator.ma","params":{"period":11}},"operator":"between","value":{"min":1}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "condition-value-required", SourceTest: "TestConditionValueValidationCoversSetRangePositionAndPattern", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"simple.price","params":{}},"operator":"between"}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "condition-set-invalid", SourceTest: "TestConditionValueValidationCoversSetRangePositionAndPattern", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"field.market","params":{}},"operator":"in","value":[]}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "condition-range-invalid", SourceTest: "TestConditionValueValidationCoversSetRangePositionAndPattern", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"simple.price","params":{}},"operator":"between","value":{"min":3,"max":2}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "condition-interval-invalid", SourceTest: "TestConditionValueValidationCoversSetRangePositionAndPattern", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"simple.price","params":{}},"operator":"between","value":{"intervals":["bad"]}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "condition-position-invalid", SourceTest: "TestConditionValueValidationCoversSetRangePositionAndPattern", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"indicator.ma","params":{"period":11}},"operator":"position","value":{"position":5,"secondValue":1}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "condition-position-second-required", SourceTest: "TestConditionValueValidationCoversSetRangePositionAndPattern", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"indicator.ma","params":{"period":11}},"operator":"position","value":{"position":1}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "condition-pattern-invalid", SourceTest: "TestConditionValueValidationCoversSetRangePositionAndPattern", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"pattern.ma_long","params":{}},"operator":"pattern","value":{"match":"yes"}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
		{Name: "second-factor-must-be-indicator", SourceTest: "TestDefinitionNormalizationCoversPoolsSortsAndSecondFactors", Input: `{"market":"US","pool":{},"conditions":[{"factor":{"factorKey":"indicator.ma","params":{"period":11}},"operator":"position","value":{"position":1},"secondFactor":{"factorKey":"simple.price","params":{}}}],"catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`},
	}
}
