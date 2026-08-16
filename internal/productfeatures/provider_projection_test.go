package productfeatures

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestProviderNewsProjectionMapsNullableFieldsAndAsOf(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	title := "Apple beats expectations"
	link := "https://example.com/aapl"
	published := "2026-08-15T21:30:00Z"
	older := "2026-08-14T10:00:00Z"
	descriptor := marketdata.ProviderDescriptor{BrokerID: "yfinance", ProviderID: "yahoo-finance"}
	query := &broker.FeatureQuery{FeatureID: broker.FeatureResearchNews, InstrumentID: "US.AAPL"}
	response := marketdata.NewsResponse{
		InstrumentID: "US.AAPL",
		Source:       "yfinance-news",
		Entries: []marketdata.NewsEntry{
			{Title: &title, Link: &link, PublishedAt: &published},
			{PublishedAt: &older},
			{},
		},
	}

	result := projectProviderNews(descriptor, query, response, "US", now)

	if len(result.Entries) != 3 {
		t.Fatalf("entries = %#v", result.Entries)
	}
	first := result.Entries[0]
	if first["title"] != title || first["link"] != link || first["publishedAt"] != published {
		t.Fatalf("first entry = %#v", first)
	}
	if _, ok := first["publisher"]; ok {
		t.Fatalf("nil publisher must be omitted: %#v", first)
	}
	if _, ok := first["summary"]; ok {
		t.Fatalf("nil summary must be omitted: %#v", first)
	}
	if len(result.Entries[2]) != 0 {
		t.Fatalf("fully null entry must project empty: %#v", result.Entries[2])
	}
	want := time.Date(2026, 8, 15, 21, 30, 0, 0, time.UTC)
	if !result.AsOf.Equal(want) {
		t.Fatalf("AsOf = %v, want latest publishedAt %v", result.AsOf, want)
	}
	if result.Total == nil || *result.Total != 3 {
		t.Fatalf("Total = %#v", result.Total)
	}
	if result.HasMore == nil || *result.HasMore {
		t.Fatalf("HasMore = %#v", result.HasMore)
	}
	if result.Metadata["source"] != "yfinance-news" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
	attribution := result.Provider
	if attribution.BrokerID != "yfinance" ||
		attribution.FeatureID != broker.FeatureResearchNews ||
		attribution.Capability != broker.CapabilityAvailable ||
		attribution.SelectionReason != embeddedProviderSelectionReason ||
		!attribution.AsOf.Equal(result.AsOf) || !attribution.ResolvedAt.Equal(now) {
		t.Fatalf("attribution = %#v", attribution)
	}
	if result.ResolvedInstrument == nil ||
		result.ResolvedInstrument.InstrumentID != "US.AAPL" ||
		result.ResolvedInstrument.Code != "AAPL" ||
		result.ResolvedInstrument.QuoteMarket != "US" {
		t.Fatalf("resolved instrument = %#v", result.ResolvedInstrument)
	}
}

func TestProviderNewsProjectionFallsBackToNowWithoutTimestamps(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	title := "untimed"
	result := projectProviderNews(
		marketdata.ProviderDescriptor{BrokerID: "akshare"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchNews, InstrumentID: "SH.600519"},
		marketdata.NewsResponse{
			InstrumentID: "SH.600519",
			Entries:      []marketdata.NewsEntry{{Title: &title}},
		},
		"SH",
		now,
	)
	if !result.AsOf.Equal(now) {
		t.Fatalf("AsOf = %v, want %v", result.AsOf, now)
	}
}

func TestProviderCorporateActionProjectionFormatsStatements(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	amount := json.Number("0.5")
	ratio := json.Number("4")
	response := marketdata.CorporateActionsResponse{
		InstrumentID: "US.AAPL",
		Source:       "yfinance-actions",
		Events: []marketdata.CorporateActionEvent{
			{Kind: "dividend", ExDate: "2026-08-10", Amount: &amount},
			{Kind: "split", ExDate: "2026-08-01", Ratio: &ratio},
			{Kind: "dividend", ExDate: "2026-07-01"},
		},
	}
	result := projectProviderCorporateActions(
		marketdata.ProviderDescriptor{BrokerID: "yfinance"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchCorporateAction, InstrumentID: "US.AAPL"},
		response,
		"US",
		now,
	)
	if len(result.Entries) != 3 {
		t.Fatalf("entries = %#v", result.Entries)
	}
	if result.Entries[0]["statement"] != "每股派息 0.5" ||
		result.Entries[0]["exDate"] != "2026-08-10" ||
		result.Entries[0]["kind"] != "dividend" {
		t.Fatalf("dividend entry = %#v", result.Entries[0])
	}
	if result.Entries[1]["statement"] != "1 拆 4" {
		t.Fatalf("split entry = %#v", result.Entries[1])
	}
	if _, ok := result.Entries[2]["statement"]; ok {
		t.Fatalf("amount-less dividend must omit statement: %#v", result.Entries[2])
	}
	if !result.AsOf.Equal(now) {
		t.Fatalf("AsOf = %v, want %v", result.AsOf, now)
	}
	if result.Total == nil || *result.Total != 3 || result.HasMore == nil || *result.HasMore {
		t.Fatalf("Total=%#v HasMore=%#v", result.Total, result.HasMore)
	}
}

func TestEmbeddedNewsLimitPrecedenceAndClamp(t *testing.T) {
	query := &broker.FeatureQuery{}
	if got := embeddedNewsLimit(query, 5); got != 5 {
		t.Fatalf("explicit pageSize limit = %d", got)
	}
	if got := embeddedNewsLimit(query, 500); got != marketdata.MaxNewsLimit {
		t.Fatalf("clamped limit = %d", got)
	}
	withParam := &broker.FeatureQuery{Params: map[string]any{"limit": int64(7)}}
	if got := embeddedNewsLimit(withParam, 0); got != 7 {
		t.Fatalf("limit param = %d", got)
	}
	if got := embeddedNewsLimit(query, 0); got != marketdata.DefaultNewsLimit {
		t.Fatalf("default limit = %d", got)
	}
	negative := &broker.FeatureQuery{Params: map[string]any{"limit": int64(-3)}}
	if got := embeddedNewsLimit(negative, 0); got != marketdata.DefaultNewsLimit {
		t.Fatalf("negative limit param = %d", got)
	}
}

func TestEmbeddedProviderServesMirrorsActiveProviderMatching(t *testing.T) {
	yfinance := marketdata.ProviderDescriptor{BrokerID: "yfinance", ProviderID: "yahoo-finance"}
	futu := marketdata.ProviderDescriptor{BrokerID: "futu", ProviderID: "futu"}
	cases := []struct {
		descriptor marketdata.ProviderDescriptor
		requested  string
		want       bool
	}{
		{yfinance, "", true},
		{yfinance, "yfinance", true},
		{yfinance, "yahoo-finance", true},
		{yfinance, "YFINANCE", true},
		{yfinance, "akshare", false},
		{yfinance, "futu", false},
		{futu, "", false},
		{futu, "yfinance", false},
		{marketdata.ProviderDescriptor{}, "", false},
	}
	for _, tc := range cases {
		if got := embeddedProviderServes(tc.descriptor, tc.requested); got != tc.want {
			t.Fatalf("embeddedProviderServes(%#v, %q) = %v, want %v",
				tc.descriptor, tc.requested, got, tc.want)
		}
	}
}

func TestEmbeddedResearchInstrumentDerivesMarketAndSymbol(t *testing.T) {
	market, symbol, ok := embeddedResearchInstrument(&broker.FeatureQuery{
		Market: "us", InstrumentID: "us.aapl",
	})
	if !ok || market != "US" || symbol != "AAPL" {
		t.Fatalf("explicit market = %q %q %v", market, symbol, ok)
	}
	market, symbol, ok = embeddedResearchInstrument(&broker.FeatureQuery{InstrumentID: "HK.00700"})
	if !ok || market != "HK" || symbol != "00700" {
		t.Fatalf("prefix-derived market = %q %q %v", market, symbol, ok)
	}
	if _, _, ok = embeddedResearchInstrument(&broker.FeatureQuery{}); ok {
		t.Fatal("empty instrumentId must not be served")
	}
	if _, _, ok = embeddedResearchInstrument(&broker.FeatureQuery{InstrumentID: "AAPL"}); ok {
		t.Fatal("instrument without market or prefix must not be served")
	}
}

func TestMapEmbeddedProviderErrorKeepsSentinels(t *testing.T) {
	unsupported := fmt.Errorf("%w: active provider %q does not support instrument news",
		marketdata.ErrCapabilityUnsupported, "akshare")
	mapped := mapEmbeddedProviderError(unsupported, "US")
	if !errors.Is(mapped, ErrCapabilityUnavailable) {
		t.Fatalf("unsupported error = %v", mapped)
	}
	if warming := mapEmbeddedProviderError(marketdata.ErrProviderWarming, "US"); !errors.Is(warming, marketdata.ErrProviderWarming) {
		t.Fatalf("warming error = %v", warming)
	}
	if busy := mapEmbeddedProviderError(marketdata.ErrProviderBusy, "HK"); !errors.Is(busy, marketdata.ErrProviderBusy) {
		t.Fatalf("busy error = %v", busy)
	}
}

// Rankings entry keys are consumed by:
// apps/web/src/components/research/RankListPanel.vue:65-79 (instrumentId,
// symbol, name, price, changeRate), apps/web/src/components/research/
// ConceptSectorView.vue:123-129 SORT_FIELDS (price/changeAmount/changeRate/
// volume/turnover), and apps/web/src/composables/research/
// useResearchFeature.ts:317-324 (changeRate merge sorting).
func TestProviderRankingsProjectionMapsFrontendKeys(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	number := func(value string) *json.Number { n := json.Number(value); return &n }
	response := marketdata.RankingsResponse{
		Market: "CN", Kind: "gainers", Source: "akshare-rankings",
		Entries: []marketdata.RankingEntry{
			{
				InstrumentID: "sh.600519", Name: "贵州茅台",
				Price: number("1680.5"), ChangeRate: number("5.42"), ChangeAmount: number("86.4"),
				Volume: number("123456"), Turnover: number("207000000"), TurnoverRatio: number("0.98"),
				PETTM: number("24.6"), MarketCap: number("2110000000000"),
			},
			{InstrumentID: "SZ.000001", Name: "平安银行"},
		},
	}
	result := projectProviderRankings(
		marketdata.ProviderDescriptor{BrokerID: "akshare"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchRankings},
		response, "CN", now,
	)
	if len(result.Entries) != 2 {
		t.Fatalf("entries = %#v", result.Entries)
	}
	first := result.Entries[0]
	wantKeys := []string{
		"instrumentId", "market", "symbol", "name", "price", "changeRate",
		"changeAmount", "volume", "turnover", "turnoverRatio", "peTTM", "marketCap",
	}
	for _, key := range wantKeys {
		if _, ok := first[key]; !ok {
			t.Fatalf("first entry missing key %q: %#v", key, first)
		}
	}
	if first["instrumentId"] != "SH.600519" || first["market"] != "SH" ||
		first["symbol"] != "600519" || first["name"] != "贵州茅台" {
		t.Fatalf("first entry = %#v", first)
	}
	if first["changeRate"] != json.Number("5.42") || first["peTTM"] != json.Number("24.6") {
		t.Fatalf("numeric values = %#v", first)
	}
	second := result.Entries[1]
	for _, key := range []string{"price", "changeRate", "turnover", "marketCap"} {
		if _, ok := second[key]; ok {
			t.Fatalf("nil field %q must be omitted: %#v", key, second)
		}
	}
	if result.ResolvedInstrument != nil {
		t.Fatalf("market-scoped result must not resolve an instrument: %#v", result.ResolvedInstrument)
	}
	if !result.AsOf.Equal(now) || result.Total == nil || *result.Total != 2 ||
		result.HasMore == nil || *result.HasMore {
		t.Fatalf("envelope AsOf=%v Total=%#v HasMore=%#v", result.AsOf, result.Total, result.HasMore)
	}
	if result.Metadata["source"] != "akshare-rankings" ||
		result.Provider.SelectionReason != embeddedProviderSelectionReason {
		t.Fatalf("metadata=%#v provider=%#v", result.Metadata, result.Provider)
	}
}

// Board keys are consumed by:
// apps/web/src/components/research/ConceptSectorView.vue:80-98 (instrumentId
// must contain "." for plate_members market derivation), :222-231 (name,
// price, changeRate), and apps/web/src/components/research/SectorHeatmap.vue:
// 8,72-89 (name, changeRate, turnover as weight fallback).
func TestProviderIndustryBoardsProjectionMapsFrontendKeys(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	changeRate := json.Number("2.31")
	turnover := json.Number("1500000000")
	leading := json.Number("7.02")
	response := marketdata.IndustryBoardsResponse{
		Market: "CN", Kind: "concept", Source: "akshare-industries",
		Boards: []marketdata.IndustryBoard{{
			Name: "人工智能", ChangeRate: &changeRate, Turnover: &turnover,
			LeadingStockName: "宁德时代", LeadingStockChangeRate: &leading,
		}},
	}
	result := projectProviderIndustryBoards(
		marketdata.ProviderDescriptor{BrokerID: "akshare"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchIndustry},
		response, "CN", now,
	)
	if len(result.Entries) != 1 {
		t.Fatalf("entries = %#v", result.Entries)
	}
	entry := result.Entries[0]
	if entry["instrumentId"] != "CN.人工智能" || entry["market"] != "CN" ||
		entry["name"] != "人工智能" || entry["productClass"] != "plate" {
		t.Fatalf("board identity = %#v", entry)
	}
	if entry["changeRate"] != changeRate || entry["turnover"] != turnover ||
		entry["leadingStockName"] != "宁德时代" || entry["leadingStockChangeRate"] != leading {
		t.Fatalf("board metrics = %#v", entry)
	}
	if result.Total == nil || *result.Total != 1 || result.HasMore == nil || *result.HasMore {
		t.Fatalf("envelope Total=%#v HasMore=%#v", result.Total, result.HasMore)
	}
}

// Member entries reuse the ranking keys read by the member table at
// apps/web/src/components/research/ConceptSectorView.vue:275-306.
func TestProviderIndustryMembersProjectionUsesRankingKeys(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	price := json.Number("92.4")
	response := marketdata.IndustryMembersResponse{
		Market: "CN", Kind: "industry", Board: "半导体", Source: "akshare-industries",
		Entries: []marketdata.RankingEntry{{InstrumentID: "SH.688981", Name: "中芯国际", Price: &price}},
	}
	result := projectProviderIndustryMembers(
		marketdata.ProviderDescriptor{BrokerID: "akshare"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchIndustry, InstrumentID: "CN.半导体"},
		response, "CN", now,
	)
	if len(result.Entries) != 1 {
		t.Fatalf("entries = %#v", result.Entries)
	}
	entry := result.Entries[0]
	if entry["instrumentId"] != "SH.688981" || entry["symbol"] != "688981" ||
		entry["name"] != "中芯国际" || entry["price"] != price {
		t.Fatalf("member entry = %#v", entry)
	}
	if result.Metadata["source"] != "akshare-industries" {
		t.Fatalf("metadata = %#v", result.Metadata)
	}
}

func TestEmbeddedRankingsLimitPrecedenceAndClamp(t *testing.T) {
	query := &broker.FeatureQuery{}
	if got := embeddedRankingsLimit(query, 12); got != 12 {
		t.Fatalf("explicit pageSize limit = %d", got)
	}
	if got := embeddedRankingsLimit(query, 500); got != marketdata.MaxRankingsLimit {
		t.Fatalf("clamped limit = %d", got)
	}
	withParam := &broker.FeatureQuery{Params: map[string]any{"limit": int64(9)}}
	if got := embeddedRankingsLimit(withParam, 0); got != 9 {
		t.Fatalf("limit param = %d", got)
	}
	if got := embeddedRankingsLimit(query, 0); got != marketdata.DefaultRankingsLimit {
		t.Fatalf("default limit = %d", got)
	}
}
