package productfeatures

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func companyResearchBrokerAdapter() *featureBroker {
	return &featureBroker{
		id: "futu",
		features: []broker.FeatureID{
			broker.FeatureResearchInstrument,
			broker.FeatureResearchFinancials,
			broker.FeatureResearchAnalyst,
			broker.FeatureResearchOwnership,
		},
	}
}

func newEmbeddedCompanyResearchService(
	adapter *featureBroker,
	reader *embeddedReaderStub,
) *Service {
	registry := broker.NewRegistry()
	registry.Register(adapter)
	return NewService(registry, adapter.id, nil, nil,
		WithEmbeddedProviderResearch(
			func() EmbeddedResearchReader { return reader },
			activeProviderStub(marketdata.ProviderDescriptor{BrokerID: "yfinance", ProviderID: "yahoo-finance"}),
		),
	)
}

func companyQuery(feature broker.FeatureID, operation string) broker.FeatureQuery {
	params := map[string]any{}
	if operation != "" {
		params["operation"] = operation
	}
	return broker.FeatureQuery{
		Market: "US", InstrumentID: "US.AAPL", FeatureID: feature, Params: params,
	}
}

// The operation strings are the exact values the frontend sends:
// apps/web/src/components/research/useInstrumentResearchController.ts:36-47.
func TestEmbeddedProviderServesCompanyResearchDefaultOperations(t *testing.T) {
	rating := json.Number("4")
	holderPct := json.Number("8.6")
	reader := &embeddedReaderStub{
		profileResult: marketdata.CompanyProfileResponse{
			InstrumentID: "US.AAPL", Source: "yfinance-profile",
			Groups: []marketdata.CompanyProfileGroup{{
				Title:  "Company",
				Fields: []marketdata.CompanyProfileField{{Name: "Sector", Value: "Technology"}},
			}},
		},
		statementsResult: marketdata.FinancialStatementsResponse{
			InstrumentID: "US.AAPL", Statement: "income", Source: "yfinance-financials",
			Fields: []marketdata.FinancialStatementField{{FieldID: "revenue", DisplayName: "Revenue"}},
		},
		analystResult: marketdata.AnalystConsensusResponse{
			InstrumentID: "US.AAPL", Source: "yfinance-analyst", Rating: &rating,
		},
		ownershipResult: marketdata.OwnershipResponse{
			InstrumentID: "US.AAPL", Source: "yfinance-ownership",
			Groups: []marketdata.OwnershipGroup{{
				Kind:  "major_holders",
				Items: []marketdata.OwnershipItem{{Name: "Vanguard", HolderPct: &holderPct}},
			}},
		},
	}
	adapter := companyResearchBrokerAdapter()
	svc := newEmbeddedCompanyResearchService(adapter, reader)

	cases := []struct {
		name      string
		feature   broker.FeatureID
		operation string
		calls     func() int
		check     func(t *testing.T, result *broker.FeatureResult)
	}{
		{
			name: "profile", feature: broker.FeatureResearchInstrument, operation: "profile",
			calls: func() int { return reader.profileCalls },
			check: func(t *testing.T, result *broker.FeatureResult) {
				if len(result.Entries) != 2 || result.Entries[0]["fieldType"] != "title" ||
					result.Entries[1]["name"] != "Sector" {
					t.Fatalf("profile entries = %#v", result.Entries)
				}
			},
		},
		{
			name: "financials", feature: broker.FeatureResearchFinancials, operation: "statements",
			calls: func() int { return reader.statementsCalls },
			check: func(t *testing.T, result *broker.FeatureResult) {
				structure, ok := result.Metadata["structureList"].([]map[string]any)
				if !ok || len(structure) != 1 || structure[0]["fieldId"] != "revenue" {
					t.Fatalf("structureList = %#v", result.Metadata)
				}
			},
		},
		{
			name: "analyst", feature: broker.FeatureResearchAnalyst, operation: "consensus",
			calls: func() int { return reader.analystCalls },
			check: func(t *testing.T, result *broker.FeatureResult) {
				if len(result.Entries) != 1 || result.Entries[0]["rating"] != rating {
					t.Fatalf("analyst entries = %#v", result.Entries)
				}
			},
		},
		{
			name: "ownership", feature: broker.FeatureResearchOwnership, operation: "overview",
			calls: func() int { return reader.ownershipCalls },
			check: func(t *testing.T, result *broker.FeatureResult) {
				main, ok := result.Metadata["mainHolderInfoList"].([]map[string]any)
				if !ok || len(main) != 1 {
					t.Fatalf("mainHolderInfoList = %#v", result.Metadata)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := svc.Query(t.Context(), companyQuery(tc.feature, tc.operation))
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if tc.calls() != 1 || adapter.queryCalls != 0 {
				t.Fatalf("reader calls = %d, broker calls = %d", tc.calls(), adapter.queryCalls)
			}
			if result.Provider.BrokerID != "yfinance" ||
				result.Provider.SelectionReason != embeddedProviderSelectionReason {
				t.Fatalf("provider attribution = %#v", result.Provider)
			}
			if result.ResolvedInstrument == nil || result.ResolvedInstrument.InstrumentID != "US.AAPL" {
				t.Fatalf("resolved instrument = %#v", result.ResolvedInstrument)
			}
			tc.check(t, result)
		})
	}
}

func TestEmbeddedProviderCompanyResearchForwardsMarketSymbolAndStatement(t *testing.T) {
	reader := &embeddedReaderStub{
		statementsResult: marketdata.FinancialStatementsResponse{
			InstrumentID: "SH.600519", Statement: "cashflow", Source: "akshare-financials",
		},
	}
	svc := newEmbeddedCompanyResearchService(companyResearchBrokerAdapter(), reader)

	query := companyQuery(broker.FeatureResearchFinancials, "statements")
	query.Market = "SH"
	query.InstrumentID = "SH.600519"
	query.Params["statement"] = "cashflow"
	if _, err := svc.Query(t.Context(), query); err != nil {
		t.Fatalf("financials query: %v", err)
	}
	if reader.statementsMarket != "SH" || reader.statementsSymbol != "600519" ||
		reader.statementsKind != "cashflow" {
		t.Fatalf("statements read = %q/%q/%q",
			reader.statementsMarket, reader.statementsSymbol, reader.statementsKind)
	}
}

func TestEmbeddedProviderCompanyResearchAcceptsOmittedOperation(t *testing.T) {
	reader := &embeddedReaderStub{
		profileResult: marketdata.CompanyProfileResponse{InstrumentID: "US.AAPL", Source: "yfinance-profile"},
	}
	svc := newEmbeddedCompanyResearchService(companyResearchBrokerAdapter(), reader)
	if _, err := svc.Query(t.Context(), companyQuery(broker.FeatureResearchInstrument, "")); err != nil {
		t.Fatalf("profile query without operation: %v", err)
	}
	if reader.profileCalls != 1 {
		t.Fatalf("profile calls = %d", reader.profileCalls)
	}
}

func TestEmbeddedProviderRejectsNonDefaultCompanyOperations(t *testing.T) {
	cases := []struct {
		feature   broker.FeatureID
		operation string
	}{
		{broker.FeatureResearchInstrument, "deep_dive"},
		{broker.FeatureResearchFinancials, "ratios"},
		{broker.FeatureResearchAnalyst, "estimate_trend"},
		{broker.FeatureResearchOwnership, "history"},
	}
	for _, tc := range cases {
		adapter := companyResearchBrokerAdapter()
		reader := &embeddedReaderStub{}
		svc := newEmbeddedCompanyResearchService(adapter, reader)
		_, err := svc.Query(t.Context(), companyQuery(tc.feature, tc.operation))
		if !errors.Is(err, ErrCapabilityUnavailable) {
			t.Fatalf("%s operation %q error = %v", tc.feature, tc.operation, err)
		}
		if adapter.queryCalls != 0 {
			t.Fatalf("%s operation %q leaked to the broker", tc.feature, tc.operation)
		}
	}
}

func TestEmbeddedProviderPropagatesCompanyResearchCapabilityErrors(t *testing.T) {
	adapter := companyResearchBrokerAdapter()
	reader := &embeddedReaderStub{
		analystErr: fmt.Errorf("%w: active provider %q does not support analyst consensus",
			marketdata.ErrCapabilityUnsupported, "akshare"),
	}
	svc := newEmbeddedCompanyResearchService(adapter, reader)
	_, err := svc.Query(t.Context(), companyQuery(broker.FeatureResearchAnalyst, "consensus"))
	if !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("unsupported analyst error = %v", err)
	}

	reader = &embeddedReaderStub{profileErr: marketdata.ErrProviderWarming}
	svc = newEmbeddedCompanyResearchService(adapter, reader)
	if _, err = svc.Query(t.Context(), companyQuery(broker.FeatureResearchInstrument, "profile")); !errors.Is(err, marketdata.ErrProviderWarming) {
		t.Fatalf("warming error = %v", err)
	}
}

func TestEmbeddedProviderCompanyResearchStaysOnBrokerPathForFutu(t *testing.T) {
	adapter := companyResearchBrokerAdapter()
	reader := &embeddedReaderStub{}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	svc := NewService(registry, adapter.id, nil, nil,
		WithEmbeddedProviderResearch(
			func() EmbeddedResearchReader { return reader },
			activeProviderStub(marketdata.ProviderDescriptor{BrokerID: "futu", ProviderID: "futu"}),
		),
	)
	if _, err := svc.Query(t.Context(), companyQuery(broker.FeatureResearchAnalyst, "consensus")); err != nil {
		t.Fatalf("futu-active analyst query: %v", err)
	}
	if adapter.queryCalls != 1 || reader.analystCalls != 0 {
		t.Fatalf("broker calls = %d, reader calls = %d", adapter.queryCalls, reader.analystCalls)
	}
}
