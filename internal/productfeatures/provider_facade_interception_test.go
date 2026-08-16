package productfeatures

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type embeddedReaderStub struct {
	newsResponse   marketdata.NewsResponse
	newsErr        error
	newsCalls      int
	newsMarket     string
	newsSymbol     string
	newsLimit      int
	actionsResult  marketdata.CorporateActionsResponse
	actionsErr     error
	actionsCalls   int
	actionsFrom    time.Time
	actionsTo      time.Time
	rankingsResult marketdata.RankingsResponse
	rankingsErr    error
	rankingsCalls  int
	rankingsMarket string
	rankingsKind   string
	rankingsLimit  int
	boardsResult   marketdata.IndustryBoardsResponse
	boardsErr      error
	boardsCalls    int
	boardsMarket   string
	boardsKind     string
	membersResult  marketdata.IndustryMembersResponse
	membersErr     error
	membersCalls   int
	membersMarket  string
	membersKind    string
	membersBoard   string
	membersLimit   int

	profileResult    marketdata.CompanyProfileResponse
	profileErr       error
	profileCalls     int
	profileMarket    string
	profileSymbol    string
	statementsResult marketdata.FinancialStatementsResponse
	statementsErr    error
	statementsCalls  int
	statementsMarket string
	statementsSymbol string
	statementsKind   string
	analystResult    marketdata.AnalystConsensusResponse
	analystErr       error
	analystCalls     int
	analystMarket    string
	analystSymbol    string
	ownershipResult  marketdata.OwnershipResponse
	ownershipErr     error
	ownershipCalls   int
	ownershipMarket  string
	ownershipSymbol  string
}

func (s *embeddedReaderStub) GetNews(
	_ context.Context,
	market, symbol string,
	limit int,
) (marketdata.NewsResponse, error) {
	s.newsCalls++
	s.newsMarket, s.newsSymbol, s.newsLimit = market, symbol, limit
	return s.newsResponse, s.newsErr
}

func (s *embeddedReaderStub) GetCorporateActions(
	_ context.Context,
	_, _ string,
	from, to time.Time,
) (marketdata.CorporateActionsResponse, error) {
	s.actionsCalls++
	s.actionsFrom, s.actionsTo = from, to
	return s.actionsResult, s.actionsErr
}

func (s *embeddedReaderStub) GetRankings(
	_ context.Context,
	market, kind string,
	limit int,
) (marketdata.RankingsResponse, error) {
	s.rankingsCalls++
	s.rankingsMarket, s.rankingsKind, s.rankingsLimit = market, kind, limit
	return s.rankingsResult, s.rankingsErr
}

func (s *embeddedReaderStub) GetIndustries(
	_ context.Context,
	market, kind string,
) (marketdata.IndustryBoardsResponse, error) {
	s.boardsCalls++
	s.boardsMarket, s.boardsKind = market, kind
	return s.boardsResult, s.boardsErr
}

func (s *embeddedReaderStub) GetIndustryMembers(
	_ context.Context,
	market, kind, board string,
	limit int,
) (marketdata.IndustryMembersResponse, error) {
	s.membersCalls++
	s.membersMarket, s.membersKind, s.membersBoard, s.membersLimit = market, kind, board, limit
	return s.membersResult, s.membersErr
}

func activeProviderStub(descriptor marketdata.ProviderDescriptor) func(context.Context) (marketdata.ProviderDescriptor, error) {
	return func(context.Context) (marketdata.ProviderDescriptor, error) {
		return descriptor, nil
	}
}

func newEmbeddedResearchService(
	brokerAdapter *featureBroker,
	reader *embeddedReaderStub,
	descriptor marketdata.ProviderDescriptor,
) *Service {
	registry := broker.NewRegistry()
	registry.Register(brokerAdapter)
	return NewService(registry, brokerAdapter.id, nil, nil,
		WithEmbeddedProviderResearch(
			func() EmbeddedResearchReader { return reader },
			activeProviderStub(descriptor),
		),
	)
}

func researchBrokerAdapter() *featureBroker {
	return &featureBroker{
		id: "futu",
		features: []broker.FeatureID{
			broker.FeatureResearchNews,
			broker.FeatureResearchCorporateAction,
		},
	}
}

func TestEmbeddedProviderServesNewsForExplicitBrokerID(t *testing.T) {
	adapter := researchBrokerAdapter()
	title := "results"
	published := "2026-08-15T21:30:00Z"
	reader := &embeddedReaderStub{newsResponse: marketdata.NewsResponse{
		InstrumentID: "US.AAPL", Source: "yfinance-news",
		Entries: []marketdata.NewsEntry{{Title: &title, PublishedAt: &published}},
	}}
	svc := newEmbeddedResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "yfinance", ProviderID: "yahoo-finance"})

	result, err := svc.Query(t.Context(), broker.FeatureQuery{
		BrokerID: "yfinance", Market: "US", InstrumentID: "US.AAPL",
		FeatureID: broker.FeatureResearchNews, PageSize: 5,
	})
	if err != nil {
		t.Fatalf("embedded news query: %v", err)
	}
	if reader.newsCalls != 1 || adapter.queryCalls != 0 {
		t.Fatalf("reader calls = %d, broker calls = %d", reader.newsCalls, adapter.queryCalls)
	}
	if reader.newsMarket != "US" || reader.newsSymbol != "AAPL" || reader.newsLimit != 5 {
		t.Fatalf("news read = %q %q limit %d", reader.newsMarket, reader.newsSymbol, reader.newsLimit)
	}
	if result.Provider.BrokerID != "yfinance" ||
		result.Provider.SelectionReason != embeddedProviderSelectionReason {
		t.Fatalf("provider attribution = %#v", result.Provider)
	}
	if len(result.Entries) != 1 || result.Entries[0]["title"] != title {
		t.Fatalf("entries = %#v", result.Entries)
	}
}

func TestEmbeddedProviderServesCorporateActionsForActiveProvider(t *testing.T) {
	adapter := researchBrokerAdapter()
	amount := mustJSONNumber(t, "1.2")
	reader := &embeddedReaderStub{actionsResult: marketdata.CorporateActionsResponse{
		InstrumentID: "SH.600519", Source: "akshare-actions",
		Events: []marketdata.CorporateActionEvent{
			{Kind: "dividend", ExDate: "2026-06-30", Amount: amount},
		},
	}}
	svc := newEmbeddedResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"})

	result, err := svc.QueryInstrumentResearch(t.Context(), InstrumentResearchRequest{
		ReadContext: ReadContext{Market: "SH"}, Family: InstrumentCorporateAction,
		InstrumentID: "SH.600519",
	})
	if err != nil {
		t.Fatalf("embedded corporate actions query: %v", err)
	}
	if reader.actionsCalls != 1 || adapter.queryCalls != 0 {
		t.Fatalf("reader calls = %d, broker calls = %d", reader.actionsCalls, adapter.queryCalls)
	}
	if reader.actionsTo.Sub(reader.actionsFrom) < 700*24*time.Hour ||
		reader.actionsTo.Sub(reader.actionsFrom) > 740*24*time.Hour {
		t.Fatalf("corporate actions window = %v .. %v", reader.actionsFrom, reader.actionsTo)
	}
	documents := result.Entries
	if len(documents) != 1 {
		t.Fatalf("documents = %#v", documents)
	}
	if result.Provider.BrokerID != "akshare" {
		t.Fatalf("provider attribution = %#v", result.Provider)
	}
}

func TestEmbeddedProviderLeavesFutuQueriesOnBrokerPath(t *testing.T) {
	yfinanceDescriptor := marketdata.ProviderDescriptor{BrokerID: "yfinance", ProviderID: "yahoo-finance"}

	// Explicit futu brokerId keeps the broker path even while yfinance is active.
	adapter := researchBrokerAdapter()
	reader := &embeddedReaderStub{}
	svc := newEmbeddedResearchService(adapter, reader, yfinanceDescriptor)
	if _, err := svc.Query(t.Context(), broker.FeatureQuery{
		BrokerID: "futu", Market: "US", InstrumentID: "US.AAPL",
		FeatureID: broker.FeatureResearchNews,
	}); err != nil {
		t.Fatalf("explicit futu news query: %v", err)
	}
	if adapter.queryCalls != 1 || reader.newsCalls != 0 {
		t.Fatalf("broker calls = %d, reader calls = %d", adapter.queryCalls, reader.newsCalls)
	}

	// Empty brokerId with futu active keeps the broker path.
	adapter = researchBrokerAdapter()
	svc = newEmbeddedResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "futu", ProviderID: "futu"})
	if _, err := svc.Query(t.Context(), broker.FeatureQuery{
		Market: "US", InstrumentID: "US.AAPL", FeatureID: broker.FeatureResearchNews,
	}); err != nil {
		t.Fatalf("futu-active news query: %v", err)
	}
	if adapter.queryCalls != 1 || reader.newsCalls != 0 {
		t.Fatalf("broker calls = %d, reader calls = %d", adapter.queryCalls, reader.newsCalls)
	}

	// Explicit yfinance with futu active is not intercepted and stays a broker
	// resolution failure.
	adapter = researchBrokerAdapter()
	svc = newEmbeddedResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "futu", ProviderID: "futu"})
	if _, err := svc.Query(t.Context(), broker.FeatureQuery{
		BrokerID: "yfinance", Market: "US", InstrumentID: "US.AAPL",
		FeatureID: broker.FeatureResearchNews,
	}); !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("unregistered provider error = %v", err)
	}
	if reader.newsCalls != 0 {
		t.Fatalf("reader calls = %d", reader.newsCalls)
	}
}

func TestEmbeddedProviderPropagatesCapabilityAndLifecycleErrors(t *testing.T) {
	adapter := researchBrokerAdapter()
	reader := &embeddedReaderStub{
		newsErr: fmt.Errorf("%w: active provider %q does not support instrument news",
			marketdata.ErrCapabilityUnsupported, "akshare"),
	}
	svc := newEmbeddedResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"})
	_, err := svc.Query(t.Context(), broker.FeatureQuery{
		Market: "US", InstrumentID: "US.AAPL", FeatureID: broker.FeatureResearchNews,
	})
	if !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("unsupported market error = %v", err)
	}

	reader = &embeddedReaderStub{newsErr: marketdata.ErrProviderWarming}
	svc = newEmbeddedResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"})
	if _, err = svc.Query(t.Context(), broker.FeatureQuery{
		Market: "US", InstrumentID: "US.AAPL", FeatureID: broker.FeatureResearchNews,
	}); !errors.Is(err, marketdata.ErrProviderWarming) {
		t.Fatalf("warming error = %v", err)
	}

	reader = &embeddedReaderStub{actionsErr: marketdata.ErrProviderBusy}
	svc = newEmbeddedResearchService(adapter, reader,
		marketdata.ProviderDescriptor{BrokerID: "akshare", ProviderID: "akshare"})
	if _, err = svc.Query(t.Context(), broker.FeatureQuery{
		Market: "SH", InstrumentID: "SH.600519", FeatureID: broker.FeatureResearchCorporateAction,
	}); !errors.Is(err, marketdata.ErrProviderBusy) {
		t.Fatalf("busy error = %v", err)
	}
}

func mustJSONNumber(t *testing.T, value string) *json.Number {
	t.Helper()
	number := json.Number(value)
	if _, err := number.Float64(); err != nil {
		t.Fatalf("invalid number %q: %v", value, err)
	}
	return &number
}

func (s *embeddedReaderStub) GetCompanyProfile(
	_ context.Context,
	market, symbol string,
) (marketdata.CompanyProfileResponse, error) {
	s.profileCalls++
	s.profileMarket, s.profileSymbol = market, symbol
	return s.profileResult, s.profileErr
}

func (s *embeddedReaderStub) GetFinancialStatements(
	_ context.Context,
	market, symbol, statement string,
) (marketdata.FinancialStatementsResponse, error) {
	s.statementsCalls++
	s.statementsMarket, s.statementsSymbol, s.statementsKind = market, symbol, statement
	return s.statementsResult, s.statementsErr
}

func (s *embeddedReaderStub) GetAnalystConsensus(
	_ context.Context,
	market, symbol string,
) (marketdata.AnalystConsensusResponse, error) {
	s.analystCalls++
	s.analystMarket, s.analystSymbol = market, symbol
	return s.analystResult, s.analystErr
}

func (s *embeddedReaderStub) GetOwnership(
	_ context.Context,
	market, symbol string,
) (marketdata.OwnershipResponse, error) {
	s.ownershipCalls++
	s.ownershipMarket, s.ownershipSymbol = market, symbol
	return s.ownershipResult, s.ownershipErr
}
