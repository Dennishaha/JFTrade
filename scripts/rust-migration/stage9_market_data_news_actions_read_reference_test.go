package rustmigration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	marketdataapi "github.com/jftrade/jftrade-main/internal/api/marketdata"
	srv "github.com/jftrade/jftrade-main/internal/marketdata"
)

const stage9MarketDataNewsActionsReadFixtureVersion = "stage9.market-data-news-actions-read.v1"

type stage9MarketDataNewsActionsReadCase struct {
	Name           string                                   `json:"name"`
	Method         string                                   `json:"method"`
	RequestPath    string                                   `json:"requestPath"`
	ExpectedStatus int                                      `json:"expectedStatus"`
	Headers        map[string]string                        `json:"headers,omitempty"`
	Data           json.RawMessage                          `json:"data,omitempty"`
	ErrorCode      string                                   `json:"errorCode,omitempty"`
	ErrorMessage   string                                   `json:"errorMessage,omitempty"`
	ProviderCall   *stage9MarketDataNewsActionsProviderCall `json:"providerCall,omitempty"`
}

type stage9MarketDataNewsActionsReadFixture struct {
	Version string                                `json:"version"`
	Cases   []stage9MarketDataNewsActionsReadCase `json:"cases"`
}

// stage9MarketDataNewsActionsProviderCall is fixture evidence only. It records
// the Go service call made after route validation; it is not a public field.
type stage9MarketDataNewsActionsProviderCall struct {
	Operation string `json:"operation"`
	Market    string `json:"market"`
	Symbol    string `json:"symbol"`
	Limit     *int   `json:"limit,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
}

type stage9MarketDataNewsActionsRouteProvider interface {
	srv.Provider
	providerCall() *stage9MarketDataNewsActionsProviderCall
}

type stage9MarketDataNewsActionsReadInput struct {
	name             string
	path             string
	provider         stage9MarketDataNewsActionsRouteProvider
	wantProviderCall *stage9MarketDataNewsActionsProviderCall
}

// TestStage9MarketDataNewsActionsReadFixtureMatchesCurrentGoOwner freezes the
// provider-backed news and corporate-actions GET wire without starting a
// sidecar, Provider, or OpenD connection.
func TestStage9MarketDataNewsActionsReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 market-data news/actions fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/market-data-news-actions-read.json",
	)
	gin.SetMode(gin.TestMode)
	want := stage9MarketDataNewsActionsReadFixture{
		Version: stage9MarketDataNewsActionsReadFixtureVersion,
		Cases:   make([]stage9MarketDataNewsActionsReadCase, 0),
	}
	for _, testCase := range stage9MarketDataNewsActionsReadInputs() {
		router := gin.New()
		marketdataapi.RegisterRoutes(
			router.Group("/api/v1"),
			srv.NewService(testCase.provider),
		)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		if got := testCase.provider.providerCall(); !reflect.DeepEqual(got, testCase.wantProviderCall) {
			t.Fatalf("%s provider call = %#v, want %#v", testCase.name, got, testCase.wantProviderCall)
		}
		entry := stage9MarketDataNewsActionsReadCase{
			Name:           testCase.name,
			Method:         http.MethodGet,
			RequestPath:    testCase.path,
			ExpectedStatus: recorder.Code,
			ProviderCall:   testCase.provider.providerCall(),
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
			t.Fatalf("decode %s response: %v", testCase.name, err)
		}
		if envelope.Error != nil {
			entry.ErrorCode = envelope.Error.Code
			entry.ErrorMessage = envelope.Error.Message
		} else {
			entry.Data = compactStage9MarketDataNewsActionsJSON(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode market-data news/actions fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write market-data news/actions fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read market-data news/actions fixture: %v", err)
	}
	var got stage9MarketDataNewsActionsReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode market-data news/actions fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactStage9MarketDataNewsActionsJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactStage9MarketDataNewsActionsJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		for index := range want.Cases {
			if !reflect.DeepEqual(got.Cases[index], want.Cases[index]) {
				t.Fatalf(
					"stage 9 market-data news/actions case %s drifted: got=%#v want=%#v",
					want.Cases[index].Name,
					got.Cases[index],
					want.Cases[index],
				)
			}
		}
		t.Fatal("stage 9 market-data news/actions fixture drifted from the Go owner")
	}
}

func compactStage9MarketDataNewsActionsJSON(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return data
	}
	return compact.Bytes()
}

func stage9MarketDataNewsActionsReadInputs() []stage9MarketDataNewsActionsReadInput {
	defaultLimit := 10
	limitTwo := 2
	limitThree := 3
	limitFour := 4
	limitFive := 5
	limitSix := 6
	from := "2025-01-01T00:00:00Z"
	to := "2026-01-01T00:00:00Z"
	return []stage9MarketDataNewsActionsReadInput{
		{
			name:     "news-omitted-limit-nullable-entries",
			path:     "/api/v1/market-data/news/US/AAPL",
			provider: stage9MarketDataNewsActionsProviderReady(),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "news", Market: "US", Symbol: "AAPL", Limit: &defaultLimit,
			},
		},
		{
			name:             "news-explicit-zero-rejected",
			path:             "/api/v1/market-data/news/US/AAPL?limit=0",
			provider:         stage9MarketDataNewsActionsProviderReady(),
			wantProviderCall: nil,
		},
		{
			name:             "news-malformed-limit-rejected",
			path:             "/api/v1/market-data/news/US/AAPL?limit=abc",
			provider:         stage9MarketDataNewsActionsProviderReady(),
			wantProviderCall: nil,
		},
		{
			name:             "news-over-limit-rejected",
			path:             "/api/v1/market-data/news/US/AAPL?limit=51",
			provider:         stage9MarketDataNewsActionsProviderReady(),
			wantProviderCall: nil,
		},
		{
			name: "news-empty-entries",
			path: "/api/v1/market-data/news/HK/00700?fixture=empty&limit=2",
			provider: stage9MarketDataNewsActionsProviderWith(func(p *stage9MarketDataNewsActionsProvider) {
				p.newsEntries = []srv.NewsEntry{}
				p.newsSource = "fixture-news-empty"
			}),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "news", Market: "HK", Symbol: "00700", Limit: &limitTwo,
			},
		},
		{
			name: "news-null-entries",
			path: "/api/v1/market-data/news/US/NIL?fixture=null&limit=3",
			provider: stage9MarketDataNewsActionsProviderWith(func(p *stage9MarketDataNewsActionsProvider) {
				p.newsEntries = nil
				p.newsSource = "fixture-news-null"
			}),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "news", Market: "US", Symbol: "NIL", Limit: &limitThree,
			},
		},
		{
			name:     "news-cn-aggregate-normalizes-to-sh",
			path:     "/api/v1/market-data/news/CN/SH.600519?fixture=cn&limit=3",
			provider: stage9MarketDataNewsActionsProviderReady(),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "news", Market: "SH", Symbol: "600519", Limit: &limitThree,
			},
		},
		{
			name:     "news-capability-unsupported",
			path:     "/api/v1/market-data/news/US/AAPL?fixture=capability",
			provider: stage9MarketDataNewsActionsNoCapabilityProvider(),
		},
		{
			name: "news-provider-fallback",
			path: "/api/v1/market-data/news/US/MSFT?fixture=fallback&limit=4",
			provider: stage9MarketDataNewsActionsProviderWith(func(p *stage9MarketDataNewsActionsProvider) {
				p.newsErr = errors.New("fixture news fallback unavailable")
			}),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "news", Market: "US", Symbol: "MSFT", Limit: &limitFour,
			},
		},
		{
			name: "news-provider-warming-retry",
			path: "/api/v1/market-data/news/SH/600519?fixture=warming&limit=5",
			provider: stage9MarketDataNewsActionsProviderWith(func(p *stage9MarketDataNewsActionsProvider) {
				p.newsErr = srv.ErrProviderWarming
			}),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "news", Market: "SH", Symbol: "600519", Limit: &limitFive,
			},
		},
		{
			name: "news-provider-busy-retry",
			path: "/api/v1/market-data/news/SZ/000001?fixture=busy&limit=6",
			provider: stage9MarketDataNewsActionsProviderWith(func(p *stage9MarketDataNewsActionsProvider) {
				p.newsErr = srv.ErrProviderBusy
			}),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "news", Market: "SZ", Symbol: "000001", Limit: &limitSix,
			},
		},
		{
			name: "news-provider-changed",
			path: "/api/v1/market-data/news/US/AAPL?fixture=provider-changed&limit=4",
			provider: stage9MarketDataNewsActionsProviderWith(func(p *stage9MarketDataNewsActionsProvider) {
				p.newsErr = srv.ErrProviderChanged
			}),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "news", Market: "US", Symbol: "AAPL", Limit: &limitFour,
			},
		},
		{
			name:     "corporate-actions-range-nullable-numbers",
			path:     "/api/v1/market-data/corporate-actions/US/AAPL?from=2025-01-01T08:00:00%2B08:00&to=2026-01-01T00:00:00Z",
			provider: stage9MarketDataNewsActionsProviderReady(),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "corporate-actions", Market: "US", Symbol: "AAPL", From: from, To: to,
			},
		},
		{
			name:             "corporate-actions-invalid-from-rejected",
			path:             "/api/v1/market-data/corporate-actions/US/AAPL?from=not-a-time",
			provider:         stage9MarketDataNewsActionsProviderReady(),
			wantProviderCall: nil,
		},
		{
			name:             "corporate-actions-invalid-to-rejected",
			path:             "/api/v1/market-data/corporate-actions/US/AAPL?to=not-a-time",
			provider:         stage9MarketDataNewsActionsProviderReady(),
			wantProviderCall: nil,
		},
		{
			name: "corporate-actions-empty-events",
			path: "/api/v1/market-data/corporate-actions/HK/00700?fixture=empty",
			provider: stage9MarketDataNewsActionsProviderWith(func(p *stage9MarketDataNewsActionsProvider) {
				p.actionEvents = []srv.CorporateActionEvent{}
				p.actionSource = "fixture-actions-empty"
			}),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "corporate-actions", Market: "HK", Symbol: "00700",
			},
		},
		{
			name: "corporate-actions-null-events",
			path: "/api/v1/market-data/corporate-actions/US/NIL?fixture=null",
			provider: stage9MarketDataNewsActionsProviderWith(func(p *stage9MarketDataNewsActionsProvider) {
				p.actionEvents = nil
				p.actionSource = "fixture-actions-null"
			}),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "corporate-actions", Market: "US", Symbol: "NIL",
			},
		},
		{
			name:     "corporate-actions-cn-aggregate-normalizes-to-sz",
			path:     "/api/v1/market-data/corporate-actions/CN/SZ.000001?fixture=cn",
			provider: stage9MarketDataNewsActionsProviderReady(),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "corporate-actions", Market: "SZ", Symbol: "000001",
			},
		},
		{
			name:             "corporate-actions-invalid-range-rejected",
			path:             "/api/v1/market-data/corporate-actions/US/AAPL?fixture=invalid-range&from=2026-01-01T00:00:00Z&to=2025-01-01T00:00:00Z",
			provider:         stage9MarketDataNewsActionsProviderReady(),
			wantProviderCall: nil,
		},
		{
			name:     "corporate-actions-capability-unsupported",
			path:     "/api/v1/market-data/corporate-actions/US/AAPL?fixture=capability",
			provider: stage9MarketDataNewsActionsNoCapabilityProvider(),
		},
		{
			name: "corporate-actions-provider-fallback",
			path: "/api/v1/market-data/corporate-actions/US/MSFT?fixture=fallback",
			provider: stage9MarketDataNewsActionsProviderWith(func(p *stage9MarketDataNewsActionsProvider) {
				p.actionsErr = errors.New("fixture corporate-actions fallback unavailable")
			}),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "corporate-actions", Market: "US", Symbol: "MSFT",
			},
		},
		{
			name: "corporate-actions-provider-warming-retry",
			path: "/api/v1/market-data/corporate-actions/SH/600519?fixture=warming",
			provider: stage9MarketDataNewsActionsProviderWith(func(p *stage9MarketDataNewsActionsProvider) {
				p.actionsErr = srv.ErrProviderWarming
			}),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "corporate-actions", Market: "SH", Symbol: "600519",
			},
		},
		{
			name: "corporate-actions-provider-busy-retry",
			path: "/api/v1/market-data/corporate-actions/SZ/000001?fixture=busy",
			provider: stage9MarketDataNewsActionsProviderWith(func(p *stage9MarketDataNewsActionsProvider) {
				p.actionsErr = srv.ErrProviderBusy
			}),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "corporate-actions", Market: "SZ", Symbol: "000001",
			},
		},
		{
			name: "corporate-actions-provider-changed",
			path: "/api/v1/market-data/corporate-actions/US/AAPL?fixture=provider-changed",
			provider: stage9MarketDataNewsActionsProviderWith(func(p *stage9MarketDataNewsActionsProvider) {
				p.actionsErr = srv.ErrProviderChanged
			}),
			wantProviderCall: &stage9MarketDataNewsActionsProviderCall{
				Operation: "corporate-actions", Market: "US", Symbol: "AAPL",
			},
		},
	}
}

type stage9MarketDataNewsActionsBaseProvider struct {
	providerID string
}

func stage9MarketDataNewsActionsNoCapabilityProvider() *stage9MarketDataNewsActionsBaseProvider {
	return &stage9MarketDataNewsActionsBaseProvider{providerID: "fixture-no-capability"}
}

func (p *stage9MarketDataNewsActionsBaseProvider) Descriptor(context.Context) (srv.ProviderDescriptor, error) {
	return srv.ProviderDescriptor{ProviderID: p.providerID}, nil
}

func (*stage9MarketDataNewsActionsBaseProvider) GetMarkets(context.Context) ([]srv.MarketProfile, error) {
	return nil, nil
}

func (*stage9MarketDataNewsActionsBaseProvider) GetSecurityDetails(context.Context, string, string) (srv.SecurityDetails, error) {
	return nil, nil
}

func (*stage9MarketDataNewsActionsBaseProvider) LookupInstrument(context.Context, string, string) ([]srv.InstrumentCandidate, error) {
	return nil, nil
}

func (*stage9MarketDataNewsActionsBaseProvider) SearchInstruments(context.Context, string, int) ([]srv.InstrumentCandidate, error) {
	return nil, nil
}

func (*stage9MarketDataNewsActionsBaseProvider) QuerySnapshot(context.Context, string) (*srv.Tick, error) {
	return nil, nil
}

func (*stage9MarketDataNewsActionsBaseProvider) QueryTicker(context.Context, string) (*srv.Tick, error) {
	return nil, nil
}

func (*stage9MarketDataNewsActionsBaseProvider) GetHistoricalCandles(context.Context, srv.HistoricalCandlesQuery) (srv.CandlesResponse, error) {
	return nil, nil
}

func (*stage9MarketDataNewsActionsBaseProvider) GetDepth(context.Context, string, string, int) (srv.DepthResponse, error) {
	return nil, nil
}

func (*stage9MarketDataNewsActionsBaseProvider) NormalizeInstrument(context.Context, map[string]any) (map[string]any, error) {
	return nil, nil
}

func (*stage9MarketDataNewsActionsBaseProvider) Health(context.Context) (srv.HealthStatus, error) {
	return srv.HealthStatus{Connected: true, Readiness: srv.ProviderReadinessReady}, nil
}

func (*stage9MarketDataNewsActionsBaseProvider) providerCall() *stage9MarketDataNewsActionsProviderCall {
	return nil
}

type stage9MarketDataNewsActionsProvider struct {
	stage9MarketDataNewsActionsBaseProvider
	newsEntries  []srv.NewsEntry
	newsSource   string
	newsErr      error
	actionEvents []srv.CorporateActionEvent
	actionSource string
	actionsErr   error
	call         *stage9MarketDataNewsActionsProviderCall
}

func stage9MarketDataNewsActionsProviderReady() *stage9MarketDataNewsActionsProvider {
	return stage9MarketDataNewsActionsProviderWith(nil)
}

func stage9MarketDataNewsActionsProviderWith(
	configure func(*stage9MarketDataNewsActionsProvider),
) *stage9MarketDataNewsActionsProvider {
	provider := &stage9MarketDataNewsActionsProvider{
		stage9MarketDataNewsActionsBaseProvider: stage9MarketDataNewsActionsBaseProvider{
			providerID: "fixture-news-actions",
		},
		newsEntries: []srv.NewsEntry{
			{
				Title:       stage9MarketDataNewsActionsText("Fixture headline"),
				Link:        stage9MarketDataNewsActionsText("https://example.test/news"),
				Publisher:   nil,
				PublishedAt: stage9MarketDataNewsActionsText("2026-08-10T13:30:00Z"),
				Summary:     nil,
			},
			{},
		},
		newsSource: "fixture-news",
		actionEvents: []srv.CorporateActionEvent{
			{
				Kind: "dividend", ExDate: "2026-05-11",
				Amount: stage9MarketDataNewsActionsNumber("0.25"), Ratio: nil,
			},
			{
				Kind: "split", ExDate: "2026-06-09",
				Amount: nil, Ratio: stage9MarketDataNewsActionsNumber("4"),
			},
		},
		actionSource: "fixture-actions",
	}
	if configure != nil {
		configure(provider)
	}
	return provider
}

func (p *stage9MarketDataNewsActionsProvider) News(
	_ context.Context,
	market string,
	symbol string,
	limit int,
) (srv.NewsResponse, error) {
	limitCopy := limit
	p.call = &stage9MarketDataNewsActionsProviderCall{
		Operation: "news", Market: market, Symbol: symbol, Limit: &limitCopy,
	}
	return srv.NewsResponse{
		Market: market, Symbol: symbol, InstrumentID: market + "." + symbol,
		Entries: p.newsEntries, Source: p.newsSource,
	}, p.newsErr
}

func (p *stage9MarketDataNewsActionsProvider) CorporateActions(
	_ context.Context,
	market string,
	symbol string,
	from time.Time,
	to time.Time,
) (srv.CorporateActionsResponse, error) {
	call := &stage9MarketDataNewsActionsProviderCall{
		Operation: "corporate-actions", Market: market, Symbol: symbol,
	}
	if !from.IsZero() {
		call.From = from.UTC().Format(time.RFC3339Nano)
	}
	if !to.IsZero() {
		call.To = to.UTC().Format(time.RFC3339Nano)
	}
	p.call = call
	return srv.CorporateActionsResponse{
		Market: market, Symbol: symbol, InstrumentID: market + "." + symbol,
		Events: p.actionEvents, Source: p.actionSource,
	}, p.actionsErr
}

func (p *stage9MarketDataNewsActionsProvider) providerCall() *stage9MarketDataNewsActionsProviderCall {
	if p.call == nil {
		return nil
	}
	result := *p.call
	if p.call.Limit != nil {
		limit := *p.call.Limit
		result.Limit = &limit
	}
	return &result
}

func stage9MarketDataNewsActionsText(value string) *string {
	return &value
}

func stage9MarketDataNewsActionsNumber(value string) *json.Number {
	number := json.Number(value)
	return &number
}
