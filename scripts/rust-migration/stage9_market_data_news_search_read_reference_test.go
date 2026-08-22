package rustmigration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	productfeaturesapi "github.com/jftrade/jftrade-main/internal/api/productfeatures"
	marketdatasrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productfeatures "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const stage9MarketDataNewsSearchReadFixtureVersion = "stage9.market-data-news-search-read.v1"

type stage9MarketDataNewsSearchReadCase struct {
	Name           string                                  `json:"name"`
	Method         string                                  `json:"method"`
	RequestPath    string                                  `json:"requestPath"`
	ExpectedStatus int                                     `json:"expectedStatus"`
	Headers        map[string]string                       `json:"headers,omitempty"`
	Data           json.RawMessage                         `json:"data,omitempty"`
	ErrorCode      string                                  `json:"errorCode,omitempty"`
	ErrorMessage   string                                  `json:"errorMessage,omitempty"`
	ProviderCall   *stage9MarketDataNewsSearchProviderCall `json:"providerCall,omitempty"`
}

type stage9MarketDataNewsSearchReadFixture struct {
	Version string                               `json:"version"`
	Cases   []stage9MarketDataNewsSearchReadCase `json:"cases"`
}

// ProviderCall is fixture evidence for the facade boundary. It is not part of
// the public HTTP response and exists to freeze query normalization and
// provider fallback decisions alongside the response wire.
type stage9MarketDataNewsSearchProviderCall struct {
	Source       string         `json:"source"`
	Market       string         `json:"market,omitempty"`
	Symbol       string         `json:"symbol,omitempty"`
	Limit        int            `json:"limit,omitempty"`
	BrokerID     string         `json:"brokerId,omitempty"`
	AccountID    string         `json:"accountId,omitempty"`
	MarketQuery  string         `json:"marketQuery,omitempty"`
	InstrumentID string         `json:"instrumentId,omitempty"`
	PageSize     int            `json:"pageSize,omitempty"`
	Cursor       string         `json:"cursor,omitempty"`
	Params       map[string]any `json:"params,omitempty"`
}

type stage9MarketDataNewsSearchInput struct {
	name          string
	path          string
	brokerID      string
	descriptor    marketdatasrv.ProviderDescriptor
	descriptorErr error
	newsEntries   []marketdatasrv.NewsEntry
	newsSource    string
	newsErr       error
	brokerErr     error
}

type stage9MarketDataNewsSearchReader struct {
	entries []marketdatasrv.NewsEntry
	source  string
	err     error
	call    *stage9MarketDataNewsSearchProviderCall
}

func (r *stage9MarketDataNewsSearchReader) GetNews(
	_ context.Context,
	market string,
	symbol string,
	limit int,
) (marketdatasrv.NewsResponse, error) {
	r.call = &stage9MarketDataNewsSearchProviderCall{
		Source: "embedded",
		Market: market,
		Symbol: symbol,
		Limit:  limit,
	}
	return marketdatasrv.NewsResponse{
		Market:       market,
		Symbol:       symbol,
		InstrumentID: market + "." + symbol,
		Entries:      r.entries,
		Source:       r.source,
	}, r.err
}

func (*stage9MarketDataNewsSearchReader) GetCorporateActions(
	context.Context,
	string,
	string,
	time.Time,
	time.Time,
) (marketdatasrv.CorporateActionsResponse, error) {
	return marketdatasrv.CorporateActionsResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetRankings(
	context.Context,
	string,
	string,
	int,
) (marketdatasrv.RankingsResponse, error) {
	return marketdatasrv.RankingsResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetIndustries(
	context.Context,
	string,
	string,
) (marketdatasrv.IndustryBoardsResponse, error) {
	return marketdatasrv.IndustryBoardsResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetIndustryMembers(
	context.Context,
	string,
	string,
	string,
	int,
) (marketdatasrv.IndustryMembersResponse, error) {
	return marketdatasrv.IndustryMembersResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetCompanyProfile(
	context.Context,
	string,
	string,
) (marketdatasrv.CompanyProfileResponse, error) {
	return marketdatasrv.CompanyProfileResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetFinancialStatements(
	context.Context,
	string,
	string,
	string,
) (marketdatasrv.FinancialStatementsResponse, error) {
	return marketdatasrv.FinancialStatementsResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetAnalystConsensus(
	context.Context,
	string,
	string,
) (marketdatasrv.AnalystConsensusResponse, error) {
	return marketdatasrv.AnalystConsensusResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetOwnership(
	context.Context,
	string,
	string,
) (marketdatasrv.OwnershipResponse, error) {
	return marketdatasrv.OwnershipResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetEarningsCalendar(
	context.Context,
	string,
	string,
) (marketdatasrv.EarningsCalendarResponse, error) {
	return marketdatasrv.EarningsCalendarResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetDividendCalendar(
	context.Context,
	string,
) (marketdatasrv.DividendCalendarResponse, error) {
	return marketdatasrv.DividendCalendarResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetEconomicCalendar(
	context.Context,
	string,
	string,
) (marketdatasrv.EconomicCalendarResponse, error) {
	return marketdatasrv.EconomicCalendarResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetIpoCalendar(
	context.Context,
) (marketdatasrv.IpoCalendarResponse, error) {
	return marketdatasrv.IpoCalendarResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetMacroIndicators(
	context.Context,
) (marketdatasrv.MacroIndicatorsResponse, error) {
	return marketdatasrv.MacroIndicatorsResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetMacroIndicatorHistory(
	context.Context,
	string,
	int,
) (marketdatasrv.MacroIndicatorHistoryResponse, error) {
	return marketdatasrv.MacroIndicatorHistoryResponse{}, nil
}

func (*stage9MarketDataNewsSearchReader) GetScreen(
	context.Context,
	marketdatasrv.ScreenRequest,
) (marketdatasrv.ScreenResponse, error) {
	return marketdatasrv.ScreenResponse{}, nil
}

type stage9MarketDataNewsSearchBroker struct {
	id   string
	err  error
	call *stage9MarketDataNewsSearchProviderCall
}

func (b *stage9MarketDataNewsSearchBroker) ID() string {
	if b.id == "" {
		return "api-test"
	}
	return b.id
}

func (b *stage9MarketDataNewsSearchBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{
		ID:                b.ID(),
		SecurityFirm:      "Fixture",
		CapabilityVersion: broker.BuiltinCapabilityCatalog.Version,
		Capabilities: []broker.MarketCapability{{
			Market: "US",
			Features: []broker.FeatureCapability{{
				ID:      broker.FeatureResearchNews,
				Markets: []string{"US", "HK", "SH", "SZ"},
				Access:  broker.FeatureAccessRead,
				State:   broker.CapabilityAvailable,
			}},
		}},
	}
}

func (*stage9MarketDataNewsSearchBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}

func (*stage9MarketDataNewsSearchBroker) Trading() broker.TradingService { return nil }

func (*stage9MarketDataNewsSearchBroker) MarketData() broker.MarketDataReader { return nil }

func (b *stage9MarketDataNewsSearchBroker) QueryInstrumentResearch(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	b.call = &stage9MarketDataNewsSearchProviderCall{
		Source:       "broker",
		BrokerID:     query.BrokerID,
		AccountID:    query.AccountID,
		MarketQuery:  query.Market,
		InstrumentID: query.InstrumentID,
		PageSize:     query.PageSize,
		Cursor:       query.Cursor,
		Params:       query.Params,
	}
	if b.err != nil {
		return nil, b.err
	}
	return &broker.FeatureResult{
		AsOf: time.Now().UTC(),
		Entries: []map[string]any{{
			"source":       "fixture-broker",
			"market":       query.Market,
			"instrumentId": query.InstrumentID,
			"pageSize":     query.PageSize,
		}},
	}, nil
}

func stage9MarketDataNewsSearchDescriptor() marketdatasrv.ProviderDescriptor {
	return marketdatasrv.ProviderDescriptor{
		SelectionID:      "fixture-news-search",
		ProviderID:       "yahoo-finance",
		DisplayName:      "Fixture News Provider",
		BrokerID:         "yfinance",
		Source:           "fixture",
		DefaultMarket:    "US",
		SupportedMarkets: []string{"US", "HK", "SH", "SZ"},
		Capabilities:     marketdatasrv.ProviderCapabilities{InstrumentSearch: true},
	}
}

func stage9MarketDataNewsSearchInputCases() []stage9MarketDataNewsSearchInput {
	readyEntries := []marketdatasrv.NewsEntry{{
		Title:       stage9MarketDataNewsSearchString("Fixture headline"),
		Link:        stage9MarketDataNewsSearchString("https://example.test/news"),
		PublishedAt: stage9MarketDataNewsSearchString("2026-08-10T13:30:00Z"),
	}}
	return []stage9MarketDataNewsSearchInput{
		{
			name:        "embedded-ready-page-size-precedes-limit",
			path:        "/api/v1/market-data/news?brokerId=YFINANCE&accountId=%20acct%20&tradingEnvironment=simulate&instrumentId=us.aapl&market=us&pageSize=5&limit=2&refresh=true&cursor=page-1&query=earnings&query=dividend",
			descriptor:  stage9MarketDataNewsSearchDescriptor(),
			newsEntries: readyEntries,
			newsSource:  "fixture-news",
		},
		{
			name:        "embedded-default-limit-and-instrument-market",
			path:        "/api/v1/market-data/news?brokerId=yahoo-finance&instrumentId=sh.600519",
			descriptor:  stage9MarketDataNewsSearchDescriptor(),
			newsEntries: readyEntries,
			newsSource:  "fixture-news",
		},
		{
			name:        "embedded-explicit-market-overrides-prefix-and-clamps",
			path:        "/api/v1/market-data/news?brokerId=yfinance&instrumentId=US.AAPL&market=hk&pageSize=500&limit=1",
			descriptor:  stage9MarketDataNewsSearchDescriptor(),
			newsEntries: readyEntries,
			newsSource:  "fixture-news",
		},
		{
			name:        "embedded-limit-used-without-page-size",
			path:        "/api/v1/market-data/news?brokerId=yfinance&instrumentId=US.AAPL&limit=7",
			descriptor:  stage9MarketDataNewsSearchDescriptor(),
			newsEntries: readyEntries,
			newsSource:  "fixture-news",
		},
		{
			name:        "embedded-empty-entries",
			path:        "/api/v1/market-data/news?brokerId=yfinance&instrumentId=HK.00700&pageSize=2&fixture=empty",
			descriptor:  stage9MarketDataNewsSearchDescriptor(),
			newsEntries: []marketdatasrv.NewsEntry{},
			newsSource:  "fixture-news-empty",
		},
		{
			name:        "embedded-null-entries-project-to-empty-array",
			path:        "/api/v1/market-data/news?brokerId=yfinance&instrumentId=US.NIL&pageSize=3&fixture=null",
			descriptor:  stage9MarketDataNewsSearchDescriptor(),
			newsEntries: nil,
			newsSource:  "fixture-news-null",
		},
		{
			name:        "embedded-capability-unsupported",
			path:        "/api/v1/market-data/news?brokerId=yfinance&instrumentId=US.AAPL&fixture=capability",
			descriptor:  stage9MarketDataNewsSearchDescriptor(),
			newsEntries: readyEntries,
			newsSource:  "fixture-news",
			newsErr: fmt.Errorf(
				"%w: active provider %q does not support instrument news",
				marketdatasrv.ErrCapabilityUnsupported,
				"fixture-no-news",
			),
		},
		{
			name:       "embedded-provider-fallback-failure",
			path:       "/api/v1/market-data/news?brokerId=yfinance&instrumentId=US.MSFT&limit=4&fixture=fallback",
			descriptor: stage9MarketDataNewsSearchDescriptor(),
			newsSource: "fixture-news",
			newsErr:    errors.New("fixture news fallback unavailable"),
		},
		{
			name:       "embedded-provider-warming",
			path:       "/api/v1/market-data/news?brokerId=yfinance&instrumentId=US.AAPL&limit=5&fixture=warming",
			descriptor: stage9MarketDataNewsSearchDescriptor(),
			newsSource: "fixture-news",
			newsErr:    marketdatasrv.ErrProviderWarming,
		},
		{
			name:       "embedded-provider-busy",
			path:       "/api/v1/market-data/news?brokerId=yfinance&instrumentId=US.AAPL&limit=6&fixture=busy",
			descriptor: stage9MarketDataNewsSearchDescriptor(),
			newsSource: "fixture-news",
			newsErr:    marketdatasrv.ErrProviderBusy,
		},
		{
			name:       "embedded-provider-changed",
			path:       "/api/v1/market-data/news?brokerId=yfinance&instrumentId=US.AAPL&limit=4&fixture=provider-changed",
			descriptor: stage9MarketDataNewsSearchDescriptor(),
			newsSource: "fixture-news",
			newsErr:    marketdatasrv.ErrProviderChanged,
		},
		{
			name:        "explicit-futu-falls-back-to-broker",
			path:        "/api/v1/market-data/news?brokerId=futu&instrumentId=US.AAPL&market=us&pageSize=4&limit=2&refresh=true",
			brokerID:    "futu",
			descriptor:  stage9MarketDataNewsSearchDescriptor(),
			newsEntries: readyEntries,
			newsSource:  "fixture-news",
		},
		{
			name:          "descriptor-error-falls-back-to-broker",
			path:          "/api/v1/market-data/news?instrumentId=US.AAPL&market=US",
			descriptorErr: errors.New("provider descriptor unavailable"),
			newsEntries:   readyEntries,
			newsSource:    "fixture-news",
		},
		{
			name:        "missing-instrument-falls-to-broker",
			path:        "/api/v1/market-data/news?brokerId=api-test&market=US&limit=4",
			descriptor:  stage9MarketDataNewsSearchDescriptor(),
			newsEntries: readyEntries,
			newsSource:  "fixture-news",
		},
		{
			name:        "malformed-page-size-is-accepted-as-default",
			path:        "/api/v1/market-data/news?brokerId=yfinance&instrumentId=US.AAPL&pageSize=abc&limit=9",
			descriptor:  stage9MarketDataNewsSearchDescriptor(),
			newsEntries: readyEntries,
			newsSource:  "fixture-news",
		},
		{
			name:        "operation-query-overrides-route-default",
			path:        "/api/v1/market-data/news?brokerId=yfinance&instrumentId=US.AAPL&operation=custom-search&refresh=false",
			descriptor:  stage9MarketDataNewsSearchDescriptor(),
			newsEntries: readyEntries,
			newsSource:  "fixture-news",
		},
	}
}

// TestStage9MarketDataNewsSearchReadFixtureMatchesCurrentGoOwner freezes the
// product-feature news search projection and provider-facade decisions. It
// never starts a market-data helper, Provider, OpenD, cache, or subscription.
func TestStage9MarketDataNewsSearchReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 market-data news-search fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/market-data-news-search-read.json",
	)
	gin.SetMode(gin.TestMode)
	want := stage9MarketDataNewsSearchReadFixture{
		Version: stage9MarketDataNewsSearchReadFixtureVersion,
		Cases:   make([]stage9MarketDataNewsSearchReadCase, 0),
	}
	for _, input := range stage9MarketDataNewsSearchInputCases() {
		reader := &stage9MarketDataNewsSearchReader{
			entries: input.newsEntries,
			source:  input.newsSource,
			err:     input.newsErr,
		}
		adapter := &stage9MarketDataNewsSearchBroker{id: input.brokerID, err: input.brokerErr}
		registry := broker.NewRegistry()
		registry.Register(adapter)
		svc := productfeatures.NewService(
			registry,
			adapter.ID(),
			nil,
			nil,
			productfeatures.WithEmbeddedProviderResearch(
				func() productfeatures.EmbeddedResearchReader { return reader },
				func(context.Context) (marketdatasrv.ProviderDescriptor, error) {
					if input.descriptorErr != nil {
						return marketdatasrv.ProviderDescriptor{}, input.descriptorErr
					}
					return input.descriptor, nil
				},
			),
		)
		router := gin.New()
		productfeaturesapi.RegisterRoutes(router.Group("/api/v1"), svc)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, input.path, nil)
		router.ServeHTTP(recorder, request)

		entry := stage9MarketDataNewsSearchReadCase{
			Name:           input.name,
			Method:         http.MethodGet,
			RequestPath:    input.path,
			ExpectedStatus: recorder.Code,
		}
		if retryAfter := recorder.Header().Get("Retry-After"); retryAfter != "" {
			entry.Headers = map[string]string{"Retry-After": retryAfter}
		}
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v", input.name, err)
		}
		if envelope.Error != nil {
			entry.ErrorCode = envelope.Error.Code
			entry.ErrorMessage = envelope.Error.Message
		} else {
			entry.Data = normalizeStage9MarketDataNewsSearchData(envelope.Data)
		}
		if reader.call != nil {
			entry.ProviderCall = normalizeStage9MarketDataNewsSearchProviderCall(reader.call)
		} else {
			entry.ProviderCall = normalizeStage9MarketDataNewsSearchProviderCall(adapter.call)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode market-data news-search fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write market-data news-search fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read market-data news-search fixture: %v", err)
	}
	var got stage9MarketDataNewsSearchReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode market-data news-search fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactStage9MarketDataNewsSearchJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactStage9MarketDataNewsSearchJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		for index := range want.Cases {
			if !reflect.DeepEqual(got.Cases[index], want.Cases[index]) {
				t.Fatalf(
					"stage 9 market-data news-search case %s drifted: got=%#v want=%#v",
					want.Cases[index].Name,
					got.Cases[index],
					want.Cases[index],
				)
			}
		}
		t.Fatal("stage 9 market-data news-search fixture drifted from the Go owner")
	}
}

func normalizeStage9MarketDataNewsSearchData(data json.RawMessage) json.RawMessage {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	normalizeStage9MarketDataNewsSearchTimes(value)
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

func normalizeStage9MarketDataNewsSearchTimes(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "asOf" || key == "resolvedAt" {
				if _, ok := child.(string); ok {
					typed[key] = "fixture-time"
					continue
				}
			}
			normalizeStage9MarketDataNewsSearchTimes(child)
		}
	case []any:
		for _, child := range typed {
			normalizeStage9MarketDataNewsSearchTimes(child)
		}
	}
}

func compactStage9MarketDataNewsSearchJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return data
	}
	return compact.Bytes()
}

func normalizeStage9MarketDataNewsSearchProviderCall(
	call *stage9MarketDataNewsSearchProviderCall,
) *stage9MarketDataNewsSearchProviderCall {
	if call == nil {
		return nil
	}
	contents, err := json.Marshal(call)
	if err != nil {
		return call
	}
	var normalized stage9MarketDataNewsSearchProviderCall
	if err := json.Unmarshal(contents, &normalized); err != nil {
		return call
	}
	return &normalized
}

func stage9MarketDataNewsSearchString(value string) *string { return &value }
