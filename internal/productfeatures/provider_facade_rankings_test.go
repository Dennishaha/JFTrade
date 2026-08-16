package productfeatures

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func marketResearchBrokerAdapter() *featureBroker {
	return &featureBroker{
		id: "futu",
		features: []broker.FeatureID{
			broker.FeatureResearchRankings,
			broker.FeatureResearchIndustry,
		},
	}
}

func newEmbeddedMarketResearchService(
	adapter *featureBroker,
	reader *embeddedReaderStub,
	descriptor marketdata.ProviderDescriptor,
) *Service {
	registry := broker.NewRegistry()
	registry.Register(adapter)
	return NewService(registry, adapter.id, nil, nil,
		WithEmbeddedProviderResearch(
			func() EmbeddedResearchReader { return reader },
			activeProviderStub(descriptor),
		),
	)
}

func rankingsFixture() marketdata.RankingsResponse {
	changeRate := json.Number("5.42")
	price := json.Number("1680.5")
	return marketdata.RankingsResponse{
		Market: "CN", Kind: "gainers", Source: "akshare-rankings",
		Entries: []marketdata.RankingEntry{{
			InstrumentID: "SH.600519", Name: "贵州茅台", Price: &price, ChangeRate: &changeRate,
		}},
	}
}

// The operation strings below are the exact values the frontend sends:
// apps/web/src/components/research/MarketRankingsView.vue:69-91 (top_movers
// with direction up/down, hot), MarketHomeView.vue:44-63 (top_movers, hot,
// heatmap with plateType).
func TestEmbeddedProviderMapsRankingsOperationsToKinds(t *testing.T) {
	cases := []struct {
		name      string
		operation string
		direction string
		wantKind  string
	}{
		{name: "gainers via direction up", operation: "top_movers", direction: "up", wantKind: "gainers"},
		{name: "gainers without direction", operation: "top_movers", wantKind: "gainers"},
		{name: "losers via direction down", operation: "top_movers", direction: "down", wantKind: "losers"},
		{name: "hot maps to active", operation: "hot", wantKind: "active"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := marketResearchBrokerAdapter()
			reader := &embeddedReaderStub{rankingsResult: rankingsFixture()}
			svc := newEmbeddedMarketResearchService(adapter, reader,
				marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"})

			params := map[string]any{"operation": tc.operation}
			if tc.direction != "" {
				params["direction"] = tc.direction
			}
			result, err := svc.Query(t.Context(), broker.FeatureQuery{
				Market: "CN", FeatureID: broker.FeatureResearchRankings, PageSize: 30, Params: params,
			})
			if err != nil {
				t.Fatalf("rankings query: %v", err)
			}
			if reader.rankingsCalls != 1 || adapter.queryCalls != 0 {
				t.Fatalf("reader calls = %d, broker calls = %d", reader.rankingsCalls, adapter.queryCalls)
			}
			if reader.rankingsMarket != "CN" || reader.rankingsKind != tc.wantKind ||
				reader.rankingsLimit != 30 {
				t.Fatalf("rankings read = %q/%q/%d, want kind %q",
					reader.rankingsMarket, reader.rankingsKind, reader.rankingsLimit, tc.wantKind)
			}
			if len(result.Entries) != 1 || result.Entries[0]["instrumentId"] != "SH.600519" {
				t.Fatalf("entries = %#v", result.Entries)
			}
			if result.Provider.BrokerID != "akshare" ||
				result.Provider.SelectionReason != embeddedProviderSelectionReason {
				t.Fatalf("provider attribution = %#v", result.Provider)
			}
		})
	}
}

func TestEmbeddedProviderRejectsUnmappedRankingsOperations(t *testing.T) {
	// pre_market/after_hours/overnight/high_dividend_state/fund_catalog are
	// Futu-only ranking operations (MarketRankingsView.vue:37-53,
	// MarketHomeView.vue:55-58).
	for _, operation := range []string{"pre_market", "after_hours", "overnight", "high_dividend_state", "fund_catalog", ""} {
		adapter := marketResearchBrokerAdapter()
		reader := &embeddedReaderStub{}
		svc := newEmbeddedMarketResearchService(adapter, reader,
			marketdata.ProviderDescriptor{BrokerID: "yfinance", ProviderID: "yahoo-finance"})

		_, err := svc.Query(t.Context(), broker.FeatureQuery{
			Market: "US", FeatureID: broker.FeatureResearchRankings,
			Params: map[string]any{"operation": operation},
		})
		if !errors.Is(err, ErrCapabilityUnavailable) {
			t.Fatalf("operation %q error = %v", operation, err)
		}
		if reader.rankingsCalls != 0 || adapter.queryCalls != 0 {
			t.Fatalf("operation %q leaked: reader=%d broker=%d",
				operation, reader.rankingsCalls, adapter.queryCalls)
		}
	}
}

// plate_list/plate_type values come from ConceptSectorView.vue:29-41 and the
// heatmap request in MarketHomeView.vue:59-63.
func TestEmbeddedProviderMapsIndustryBoardOperations(t *testing.T) {
	boards := marketdata.IndustryBoardsResponse{
		Market: "CN", Kind: "concept", Source: "akshare-industries",
		Boards: []marketdata.IndustryBoard{{Name: "人工智能"}},
	}
	cases := []struct {
		name      string
		feature   broker.FeatureID
		operation string
		plateType string
		wantKind  string
	}{
		{name: "plate_list concept", feature: broker.FeatureResearchIndustry, operation: "plate_list", plateType: "concept", wantKind: "concept"},
		{name: "plate_list defaults to industry", feature: broker.FeatureResearchIndustry, operation: "plate_list", wantKind: "industry"},
		{name: "heatmap industry", feature: broker.FeatureResearchRankings, operation: "heatmap", plateType: "industry", wantKind: "industry"},
		{name: "heatmap concept", feature: broker.FeatureResearchRankings, operation: "heatmap", plateType: "concept", wantKind: "concept"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := marketResearchBrokerAdapter()
			reader := &embeddedReaderStub{boardsResult: boards}
			svc := newEmbeddedMarketResearchService(adapter, reader,
				marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"})

			params := map[string]any{"operation": tc.operation}
			if tc.plateType != "" {
				params["plateType"] = tc.plateType
			}
			result, err := svc.Query(t.Context(), broker.FeatureQuery{
				Market: "CN", FeatureID: tc.feature, Params: params,
			})
			if err != nil {
				t.Fatalf("boards query: %v", err)
			}
			if reader.boardsCalls != 1 || reader.boardsKind != tc.wantKind || adapter.queryCalls != 0 {
				t.Fatalf("boards read = %d/%q, broker calls = %d",
					reader.boardsCalls, reader.boardsKind, adapter.queryCalls)
			}
			if len(result.Entries) != 1 || result.Entries[0]["instrumentId"] != "CN.人工智能" ||
				result.Entries[0]["productClass"] != "plate" {
				t.Fatalf("entries = %#v", result.Entries)
			}
		})
	}
}

func TestEmbeddedProviderServesPlateMembersFromInstrumentID(t *testing.T) {
	adapter := marketResearchBrokerAdapter()
	reader := &embeddedReaderStub{membersResult: marketdata.IndustryMembersResponse{
		Market: "CN", Board: "半导体", Source: "akshare-industries",
		Entries: []marketdata.RankingEntry{{InstrumentID: "SH.688981", Name: "中芯国际"}},
	}}
	svc := newEmbeddedMarketResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"})

	result, err := svc.Query(t.Context(), broker.FeatureQuery{
		Market: "CN", InstrumentID: "CN.半导体", FeatureID: broker.FeatureResearchIndustry,
		PageSize: 50, Params: map[string]any{"operation": "plate_members"},
	})
	if err != nil {
		t.Fatalf("plate members query: %v", err)
	}
	if reader.membersCalls != 1 || reader.membersBoard != "半导体" || reader.membersMarket != "CN" ||
		reader.membersKind != "" || reader.membersLimit != 50 {
		t.Fatalf("members read = %q/%q/%q/%d",
			reader.membersMarket, reader.membersKind, reader.membersBoard, reader.membersLimit)
	}
	if len(result.Entries) != 1 || result.Entries[0]["symbol"] != "688981" {
		t.Fatalf("entries = %#v", result.Entries)
	}
}

func TestEmbeddedProviderRejectsUnsupportedIndustryOperationsAndPlateTypes(t *testing.T) {
	// chains/chain_detail/chains_by_plate/plate/plate_stocks are Futu-only
	// (IndustryChainView.vue:70-97); region/theme plate types have no embedded
	// feed (ConceptSectorView.vue:29, MarketHomeView.vue:59).
	operationCases := []string{"chains", "chain_detail", "chains_by_plate", "plate", "plate_stocks"}
	for _, operation := range operationCases {
		adapter := marketResearchBrokerAdapter()
		reader := &embeddedReaderStub{}
		svc := newEmbeddedMarketResearchService(adapter, reader,
			marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"})
		_, err := svc.Query(t.Context(), broker.FeatureQuery{
			Market: "CN", FeatureID: broker.FeatureResearchIndustry,
			Params: map[string]any{"operation": operation},
		})
		if !errors.Is(err, ErrCapabilityUnavailable) {
			t.Fatalf("operation %q error = %v", operation, err)
		}
		if reader.boardsCalls != 0 || reader.membersCalls != 0 {
			t.Fatalf("operation %q reached the reader", operation)
		}
	}
	for _, plateType := range []string{"region", "theme"} {
		adapter := marketResearchBrokerAdapter()
		reader := &embeddedReaderStub{}
		svc := newEmbeddedMarketResearchService(adapter, reader,
			marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"})
		_, err := svc.Query(t.Context(), broker.FeatureQuery{
			Market: "CN", FeatureID: broker.FeatureResearchIndustry,
			Params: map[string]any{"operation": "plate_list", "plateType": plateType},
		})
		if !errors.Is(err, ErrCapabilityUnavailable) {
			t.Fatalf("plateType %q error = %v", plateType, err)
		}
	}
}

func TestEmbeddedProviderPropagatesRankingsCapabilityErrors(t *testing.T) {
	adapter := marketResearchBrokerAdapter()
	reader := &embeddedReaderStub{
		rankingsErr: fmt.Errorf("%w: active provider %q does not support market rankings",
			marketdata.ErrCapabilityUnsupported, "akshare"),
	}
	svc := newEmbeddedMarketResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"})
	_, err := svc.Query(t.Context(), broker.FeatureQuery{
		Market: "HK", FeatureID: broker.FeatureResearchRankings,
		Params: map[string]any{"operation": "hot"},
	})
	if !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("unsupported rankings error = %v", err)
	}

	reader = &embeddedReaderStub{boardsErr: marketdata.ErrProviderWarming}
	svc = newEmbeddedMarketResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"})
	if _, err = svc.Query(t.Context(), broker.FeatureQuery{
		Market: "CN", FeatureID: broker.FeatureResearchIndustry,
		Params: map[string]any{"operation": "plate_list"},
	}); !errors.Is(err, marketdata.ErrProviderWarming) {
		t.Fatalf("warming error = %v", err)
	}
}

func TestEmbeddedProviderRankingsStayOnBrokerPathForFutu(t *testing.T) {
	adapter := marketResearchBrokerAdapter()
	reader := &embeddedReaderStub{}
	svc := newEmbeddedMarketResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "futu", ProviderID: "futu"})
	if _, err := svc.Query(t.Context(), broker.FeatureQuery{
		Market: "US", FeatureID: broker.FeatureResearchRankings,
		Params: map[string]any{"operation": "top_movers"},
	}); err != nil {
		t.Fatalf("futu-active rankings query: %v", err)
	}
	if adapter.queryCalls != 1 || reader.rankingsCalls != 0 {
		t.Fatalf("broker calls = %d, reader calls = %d", adapter.queryCalls, reader.rankingsCalls)
	}
}

func TestEmbeddedProviderDefaultsEmptyMarketToProviderDefault(t *testing.T) {
	adapter := marketResearchBrokerAdapter()
	reader := &embeddedReaderStub{rankingsResult: rankingsFixture()}
	svc := newEmbeddedMarketResearchService(adapter, reader,
		marketdata.ProviderDescriptor{
			BrokerID: "yfinance", ProviderID: "yahoo-finance", DefaultMarket: "US",
		})
	if _, err := svc.Query(t.Context(), broker.FeatureQuery{
		FeatureID: broker.FeatureResearchRankings,
		Params:    map[string]any{"operation": "hot"},
	}); err != nil {
		t.Fatalf("rankings query: %v", err)
	}
	if reader.rankingsMarket != "US" {
		t.Fatalf("rankings market = %q", reader.rankingsMarket)
	}
}
