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
