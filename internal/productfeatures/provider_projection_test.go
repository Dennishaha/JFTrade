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

// Profile entry keys are consumed by the console's profile section builder at
// apps/web/src/components/research/useInstrumentResearchController.ts:104-132
// (fieldType "title" opens a group, fieldType "text" rows render name/value).
func TestProviderCompanyProfileProjectionMapsFrontendKeys(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	response := marketdata.CompanyProfileResponse{
		InstrumentID: "US.AAPL", Source: "yfinance-profile",
		Groups: []marketdata.CompanyProfileGroup{
			{
				Title: "公司概要",
				Fields: []marketdata.CompanyProfileField{
					{Name: "行业", Value: "消费电子"},
					{Name: "", Value: ""}, // fully empty rows are skipped
				},
			},
			{Title: "", Fields: []marketdata.CompanyProfileField{{Name: "员工数", Value: "164000"}}},
		},
	}
	result := projectProviderCompanyProfile(
		marketdata.ProviderDescriptor{BrokerID: "yfinance"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchInstrument, InstrumentID: "US.AAPL"},
		response, "US", now,
	)
	if len(result.Entries) != 3 {
		t.Fatalf("entries = %#v", result.Entries)
	}
	if result.Entries[0]["fieldType"] != "title" || result.Entries[0]["name"] != "公司概要" {
		t.Fatalf("title entry = %#v", result.Entries[0])
	}
	if _, ok := result.Entries[0]["value"]; ok {
		t.Fatalf("title entry must not carry value: %#v", result.Entries[0])
	}
	if result.Entries[1]["fieldType"] != "text" || result.Entries[1]["name"] != "行业" ||
		result.Entries[1]["value"] != "消费电子" {
		t.Fatalf("text entry = %#v", result.Entries[1])
	}
	if result.Entries[2]["name"] != "员工数" {
		t.Fatalf("title-less group fields must still project: %#v", result.Entries[2])
	}
	if result.ResolvedInstrument == nil || result.ResolvedInstrument.InstrumentID != "US.AAPL" ||
		result.ResolvedInstrument.Code != "AAPL" {
		t.Fatalf("resolved instrument = %#v", result.ResolvedInstrument)
	}
	if result.Metadata["source"] != "yfinance-profile" ||
		result.Total == nil || *result.Total != 3 {
		t.Fatalf("envelope metadata=%#v Total=%#v", result.Metadata, result.Total)
	}
}

// Statement keys are consumed by the console's financial table at
// apps/web/src/components/research/useInstrumentResearchController.ts:134-181
// (metadata.structureList columns plus periodText/itemList entries; yoy/qoq
// absent means the feed published no comparison).
func TestProviderFinancialStatementsProjectionMapsFrontendKeys(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	number := func(value string) *json.Number { n := json.Number(value); return &n }
	currency := "USD"
	response := marketdata.FinancialStatementsResponse{
		InstrumentID: "US.AAPL", Statement: marketdata.StatementIncome,
		Currency: &currency, Source: "yfinance-financials",
		Fields: []marketdata.FinancialStatementField{
			{FieldID: "total_revenue", DisplayName: "总营收"},
			{FieldID: "net_income", DisplayName: "净利润"},
		},
		Periods: []marketdata.FinancialStatementPeriod{
			{
				PeriodText: "2025财年",
				Values: map[string]marketdata.FinancialStatementValue{
					"total_revenue": {Data: number("416161000000"), YoY: number("0.02")},
					"net_income":    {Data: number("112010000000")},
				},
			},
			{PeriodText: "2024财年", Values: map[string]marketdata.FinancialStatementValue{}},
		},
	}
	result := projectProviderFinancialStatements(
		marketdata.ProviderDescriptor{BrokerID: "yfinance"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchFinancials, InstrumentID: "US.AAPL"},
		response, "US", now,
	)
	structure, ok := result.Metadata["structureList"].([]map[string]any)
	if !ok || len(structure) != 2 {
		t.Fatalf("structureList = %#v", result.Metadata["structureList"])
	}
	if structure[0]["fieldId"] != "total_revenue" || structure[0]["displayName"] != "总营收" {
		t.Fatalf("structureList[0] = %#v", structure[0])
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries = %#v", result.Entries)
	}
	first := result.Entries[0]
	if first["periodText"] != "2025财年" || first["currencyCode"] != "USD" {
		t.Fatalf("period entry = %#v", first)
	}
	items, ok := first["itemList"].([]map[string]any)
	if !ok || len(items) != 2 {
		t.Fatalf("itemList = %#v", first["itemList"])
	}
	if items[0]["fieldId"] != "total_revenue" || items[0]["data"] != json.Number("416161000000") ||
		items[0]["yoy"] != json.Number("0.02") {
		t.Fatalf("revenue cell = %#v", items[0])
	}
	if _, ok := items[0]["qoq"]; ok {
		t.Fatalf("nil qoq must be omitted: %#v", items[0])
	}
	if _, ok := items[1]["yoy"]; ok {
		t.Fatalf("nil yoy must be omitted: %#v", items[1])
	}
	second := result.Entries[1]
	if items, ok := second["itemList"].([]map[string]any); !ok || len(items) != 0 {
		t.Fatalf("empty period must project empty itemList: %#v", second)
	}
}

// Analyst keys are consumed by the rating dashboard at
// apps/web/src/components/research/useInstrumentResearchController.ts:412-442
// and apps/web/src/components/research/InstrumentResearchView.vue:66-99,
// 280-282, 309 (single entry at index 0; nullable buckets are omitted).
func TestProviderAnalystConsensusProjectionMapsFrontendKeys(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	number := func(value string) *json.Number { n := json.Number(value); return &n }
	updateTime := "2026-08-15"
	response := marketdata.AnalystConsensusResponse{
		InstrumentID: "HK.00700", Source: "yfinance-analyst",
		Rating: number("4"), AnalystCount: number("38"),
		TargetPrice: &marketdata.AnalystTargetPrice{
			Lowest: number("520"), Average: number("700.5"), Highest: number("860"),
		},
		Distribution: &marketdata.AnalystDistribution{
			StrongBuy: number("45"), Buy: number("30"), Hold: number("20"),
			// Underperform/Sell nil: the feed did not publish those buckets.
		},
		UpdateTime: &updateTime,
	}
	result := projectProviderAnalystConsensus(
		marketdata.ProviderDescriptor{BrokerID: "yfinance"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchAnalyst, InstrumentID: "HK.00700"},
		response, "HK", now,
	)
	if len(result.Entries) != 1 {
		t.Fatalf("entries = %#v", result.Entries)
	}
	entry := result.Entries[0]
	if entry["rating"] != json.Number("4") || entry["analystCount"] != json.Number("38") ||
		entry["lowest"] != json.Number("520") || entry["average"] != json.Number("700.5") ||
		entry["highest"] != json.Number("860") {
		t.Fatalf("rating/target keys = %#v", entry)
	}
	if entry["strongBuy"] != json.Number("45") || entry["buy"] != json.Number("30") ||
		entry["hold"] != json.Number("20") || entry["updateTimeStr"] != "2026-08-15" {
		t.Fatalf("distribution/update keys = %#v", entry)
	}
	for _, key := range []string{"underperform", "sell"} {
		if _, ok := entry[key]; ok {
			t.Fatalf("nil bucket %q must be omitted: %#v", key, entry)
		}
	}
	if result.Total == nil || *result.Total != 1 || result.HasMore == nil || *result.HasMore {
		t.Fatalf("envelope Total=%#v HasMore=%#v", result.Total, result.HasMore)
	}
}

func TestProviderAnalystConsensusProjectionOmitsAbsentSections(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	result := projectProviderAnalystConsensus(
		marketdata.ProviderDescriptor{BrokerID: "yfinance"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchAnalyst, InstrumentID: "US.AAPL"},
		marketdata.AnalystConsensusResponse{InstrumentID: "US.AAPL", Source: "yfinance-analyst"},
		"US", now,
	)
	if len(result.Entries) != 1 || len(result.Entries[0]) != 0 {
		t.Fatalf("fully null consensus must project one empty entry: %#v", result.Entries)
	}
}

// Ownership keys are consumed by the holder panels at
// apps/web/src/components/research/useInstrumentResearchController.ts:443-514
// (metadata.mainHolderInfoList / metadata.holderTypeInfoList groups with
// itemList rows; the entry stream itself stays empty).
func TestProviderOwnershipProjectionMapsFrontendKeys(t *testing.T) {
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	number := func(value string) *json.Number { n := json.Number(value); return &n }
	staticDate := "2026-06-30"
	response := marketdata.OwnershipResponse{
		InstrumentID: "SH.600519", Source: "akshare-ownership",
		Groups: []marketdata.OwnershipGroup{
			{
				Kind: marketdata.OwnershipGroupMajorHolders, StaticDate: &staticDate,
				Items: []marketdata.OwnershipItem{
					{Name: "中国贵州茅台酒厂(集团)", HolderPct: number("54.07")},
					{Name: "香港中央结算有限公司"},
				},
			},
			{
				Kind: marketdata.OwnershipGroupHolderTypes,
				Items: []marketdata.OwnershipItem{
					{Name: "国有法人", HolderPct: number("60.1")},
					{Name: "流通A股", HolderPct: number("39.9")},
				},
			},
		},
	}
	result := projectProviderOwnership(
		marketdata.ProviderDescriptor{BrokerID: "akshare"},
		&broker.FeatureQuery{FeatureID: broker.FeatureResearchOwnership, InstrumentID: "SH.600519"},
		response, "SH", now,
	)
	if len(result.Entries) != 0 {
		t.Fatalf("ownership entry stream must stay empty: %#v", result.Entries)
	}
	if result.Total == nil || *result.Total != 0 {
		t.Fatalf("Total = %#v, want 0", result.Total)
	}
	mainHolders, ok := result.Metadata["mainHolderInfoList"].([]map[string]any)
	if !ok || len(mainHolders) != 1 {
		t.Fatalf("mainHolderInfoList = %#v", result.Metadata["mainHolderInfoList"])
	}
	if mainHolders[0]["staticDateStr"] != "2026-06-30" {
		t.Fatalf("major holder group = %#v", mainHolders[0])
	}
	mainItems, ok := mainHolders[0]["itemList"].([]map[string]any)
	if !ok || len(mainItems) != 2 {
		t.Fatalf("major holder itemList = %#v", mainHolders[0])
	}
	if mainItems[0]["name"] != "中国贵州茅台酒厂(集团)" || mainItems[0]["holderPct"] != json.Number("54.07") {
		t.Fatalf("major holder item = %#v", mainItems[0])
	}
	if _, ok := mainItems[1]["holderPct"]; ok {
		t.Fatalf("nil holderPct must be omitted: %#v", mainItems[1])
	}
	holderTypes, ok := result.Metadata["holderTypeInfoList"].([]map[string]any)
	if !ok || len(holderTypes) != 1 {
		t.Fatalf("holderTypeInfoList = %#v", result.Metadata["holderTypeInfoList"])
	}
	if _, ok := holderTypes[0]["staticDateStr"]; ok {
		t.Fatalf("nil staticDate must be omitted: %#v", holderTypes[0])
	}
	typeItems, ok := holderTypes[0]["itemList"].([]map[string]any)
	if !ok || len(typeItems) != 2 || typeItems[1]["name"] != "流通A股" {
		t.Fatalf("holder type itemList = %#v", holderTypes[0])
	}
}
