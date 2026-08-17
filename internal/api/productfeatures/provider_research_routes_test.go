package productfeatures

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	marketdatasrv "github.com/jftrade/jftrade-main/internal/marketdata"
	service "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type apiEmbeddedReader struct {
	newsResponse marketdatasrv.NewsResponse
	newsErr      error
	actions      marketdatasrv.CorporateActionsResponse
	actionsErr   error
	rankings     marketdatasrv.RankingsResponse
	rankingsErr  error
	boards       marketdatasrv.IndustryBoardsResponse
	boardsErr    error
	members      marketdatasrv.IndustryMembersResponse
	membersErr   error
	profile      marketdatasrv.CompanyProfileResponse
	profileErr   error
	statements   marketdatasrv.FinancialStatementsResponse
	statementErr error
	consensus    marketdatasrv.AnalystConsensusResponse
	consensusErr error
	ownership    marketdatasrv.OwnershipResponse
	ownershipErr error
	earnings     marketdatasrv.EarningsCalendarResponse
	earningsErr  error
	dividends    marketdatasrv.DividendCalendarResponse
	dividendsErr error
	economic     marketdatasrv.EconomicCalendarResponse
	economicErr  error
	ipos         marketdatasrv.IpoCalendarResponse
	iposErr      error
	indicators   marketdatasrv.MacroIndicatorsResponse
	history      marketdatasrv.MacroIndicatorHistoryResponse
	historyID    string
	historyLimit int
	screen       marketdatasrv.ScreenResponse
	screenErr    error
	screenReq    marketdatasrv.ScreenRequest
}

func (r *apiEmbeddedReader) GetNews(
	context.Context, string, string, int,
) (marketdatasrv.NewsResponse, error) {
	return r.newsResponse, r.newsErr
}

func (r *apiEmbeddedReader) GetCorporateActions(
	context.Context, string, string, time.Time, time.Time,
) (marketdatasrv.CorporateActionsResponse, error) {
	return r.actions, r.actionsErr
}

func (r *apiEmbeddedReader) GetRankings(
	_ context.Context, market, kind string, _ int,
) (marketdatasrv.RankingsResponse, error) {
	r.rankings.Market, r.rankings.Kind = market, kind
	return r.rankings, r.rankingsErr
}

func (r *apiEmbeddedReader) GetIndustries(
	_ context.Context, _, kind string,
) (marketdatasrv.IndustryBoardsResponse, error) {
	r.boards.Kind = kind
	return r.boards, r.boardsErr
}

func (r *apiEmbeddedReader) GetIndustryMembers(
	_ context.Context, _, _, board string, _ int,
) (marketdatasrv.IndustryMembersResponse, error) {
	r.members.Board = board
	return r.members, r.membersErr
}

func (r *apiEmbeddedReader) GetCompanyProfile(
	_ context.Context, market, symbol string,
) (marketdatasrv.CompanyProfileResponse, error) {
	r.profile.Market, r.profile.Symbol = market, symbol
	return r.profile, r.profileErr
}

func (r *apiEmbeddedReader) GetFinancialStatements(
	_ context.Context, market, symbol, statement string,
) (marketdatasrv.FinancialStatementsResponse, error) {
	r.statements.Market, r.statements.Symbol = market, symbol
	r.statements.Statement = statement
	return r.statements, r.statementErr
}

func (r *apiEmbeddedReader) GetAnalystConsensus(
	_ context.Context, market, symbol string,
) (marketdatasrv.AnalystConsensusResponse, error) {
	r.consensus.Market, r.consensus.Symbol = market, symbol
	return r.consensus, r.consensusErr
}

func (r *apiEmbeddedReader) GetOwnership(
	_ context.Context, market, symbol string,
) (marketdatasrv.OwnershipResponse, error) {
	r.ownership.Market, r.ownership.Symbol = market, symbol
	return r.ownership, r.ownershipErr
}

func (r *apiEmbeddedReader) GetEarningsCalendar(
	_ context.Context, beginDate, endDate string,
) (marketdatasrv.EarningsCalendarResponse, error) {
	r.earnings.BeginDate, r.earnings.EndDate = beginDate, endDate
	return r.earnings, r.earningsErr
}

func (r *apiEmbeddedReader) GetDividendCalendar(
	_ context.Context, date string,
) (marketdatasrv.DividendCalendarResponse, error) {
	r.dividends.Date = date
	return r.dividends, r.dividendsErr
}

func (r *apiEmbeddedReader) GetEconomicCalendar(
	_ context.Context, beginDate, endDate string,
) (marketdatasrv.EconomicCalendarResponse, error) {
	r.economic.BeginDate, r.economic.EndDate = beginDate, endDate
	return r.economic, r.economicErr
}

func (r *apiEmbeddedReader) GetIpoCalendar(context.Context) (marketdatasrv.IpoCalendarResponse, error) {
	return r.ipos, r.iposErr
}

func (r *apiEmbeddedReader) GetMacroIndicators(context.Context) (marketdatasrv.MacroIndicatorsResponse, error) {
	return r.indicators, nil
}

func (r *apiEmbeddedReader) GetMacroIndicatorHistory(
	_ context.Context, indicatorID string, limit int,
) (marketdatasrv.MacroIndicatorHistoryResponse, error) {
	r.historyID, r.historyLimit = indicatorID, limit
	return r.history, nil
}

func (r *apiEmbeddedReader) GetScreen(
	_ context.Context, req marketdatasrv.ScreenRequest,
) (marketdatasrv.ScreenResponse, error) {
	r.screenReq = req
	return r.screen, r.screenErr
}

func newEmbeddedProviderRouter(reader *apiEmbeddedReader) *gin.Engine {
	adapter := &apiFeatureBroker{}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	svc := service.NewService(registry, adapter.ID(), nil, nil,
		service.WithEmbeddedProviderResearch(
			func() service.EmbeddedResearchReader { return reader },
			func(context.Context) (marketdatasrv.ProviderDescriptor, error) {
				return marketdatasrv.ProviderDescriptor{
					BrokerID: "yfinance", ProviderID: "yahoo-finance",
				}, nil
			},
		),
	)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), svc)
	return router
}

func TestEmbeddedProviderNewsAndCorporateActionRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	title := "Apple beats expectations"
	published := "2026-08-15T21:30:00Z"
	amount := json.Number("0.5")
	reader := &apiEmbeddedReader{
		newsResponse: marketdatasrv.NewsResponse{
			InstrumentID: "US.AAPL", Source: "yfinance-news",
			Entries: []marketdatasrv.NewsEntry{{Title: &title, PublishedAt: &published}},
		},
		actions: marketdatasrv.CorporateActionsResponse{
			InstrumentID: "US.AAPL", Source: "yfinance-actions",
			Events: []marketdatasrv.CorporateActionEvent{
				{Kind: "dividend", ExDate: "2026-08-10", Amount: &amount},
			},
		},
	}
	router := newEmbeddedProviderRouter(reader)

	news := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/market-data/news?brokerId=yfinance&instrumentId=US.AAPL&pageSize=5", "",
	)
	if news.Code != http.StatusOK {
		t.Fatalf("news status=%d body=%s", news.Code, news.Body.String())
	}
	if got := responseStringField(news.Body.String(), "title"); got != title {
		t.Fatalf("news title = %q body=%s", got, news.Body.String())
	}
	if got := responseStringField(news.Body.String(), "brokerId"); got != "yfinance" {
		t.Fatalf("news brokerId = %q body=%s", got, news.Body.String())
	}
	if got := responseStringField(news.Body.String(), "selectionReason"); got != "embedded-market-data-provider" {
		t.Fatalf("news selectionReason = %q", got)
	}

	actions := performFeatureRequest(
		t, router, http.MethodGet, "/api/v1/research/corporate-actions/US.AAPL", "",
	)
	if actions.Code != http.StatusOK {
		t.Fatalf("corporate actions status=%d body=%s", actions.Code, actions.Body.String())
	}
	if got := responseStringField(actions.Body.String(), "statement"); got != "每股派息 0.5" {
		t.Fatalf("corporate action statement = %q body=%s", got, actions.Body.String())
	}
	if got := responseStringField(actions.Body.String(), "exDate"); got != "2026-08-10" {
		t.Fatalf("corporate action exDate = %q", got)
	}
}

func TestEmbeddedProviderRouteErrorsKeepHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &apiEmbeddedReader{
		newsErr: fmt.Errorf("%w: active provider %q does not support instrument news",
			marketdatasrv.ErrCapabilityUnsupported, "akshare"),
	}
	router := newEmbeddedProviderRouter(reader)

	unsupported := performFeatureRequest(
		t, router, http.MethodGet, "/api/v1/market-data/news?instrumentId=US.AAPL", "",
	)
	if unsupported.Code != http.StatusConflict {
		t.Fatalf("unsupported status=%d body=%s", unsupported.Code, unsupported.Body.String())
	}

	reader.newsErr = marketdatasrv.ErrProviderWarming
	warming := performFeatureRequest(
		t, router, http.MethodGet, "/api/v1/market-data/news?instrumentId=US.AAPL", "",
	)
	if warming.Code != http.StatusServiceUnavailable ||
		warming.Header().Get("Retry-After") != "1" {
		t.Fatalf("warming status=%d retry=%q", warming.Code, warming.Header().Get("Retry-After"))
	}

	reader.newsErr = nil
	reader.actionsErr = marketdatasrv.ErrProviderBusy
	busy := performFeatureRequest(
		t, router, http.MethodGet, "/api/v1/research/corporate-actions/SH.600519", "",
	)
	if busy.Code != http.StatusServiceUnavailable ||
		busy.Header().Get("Retry-After") != "2" {
		t.Fatalf("busy status=%d retry=%q", busy.Code, busy.Header().Get("Retry-After"))
	}
}

func TestEmbeddedProviderRankingsAndIndustryRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	price := json.Number("232.1")
	changeRate := json.Number("1.25")
	reader := &apiEmbeddedReader{
		rankings: marketdatasrv.RankingsResponse{
			Source: "yfinance-rankings",
			Entries: []marketdatasrv.RankingEntry{{
				InstrumentID: "US.AAPL", Name: "Apple Inc.", Price: &price, ChangeRate: &changeRate,
			}},
		},
		boards: marketdatasrv.IndustryBoardsResponse{
			Market: "CN", Source: "akshare-industries",
			Boards: []marketdatasrv.IndustryBoard{{Name: "人工智能", ChangeRate: &changeRate}},
		},
		members: marketdatasrv.IndustryMembersResponse{
			Market: "CN", Source: "akshare-industries",
			Entries: []marketdatasrv.RankingEntry{{InstrumentID: "SH.688981", Name: "中芯国际"}},
		},
	}
	router := newEmbeddedProviderRouter(reader)

	rankings := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/rankings?brokerId=yfinance&market=US&operation=top_movers&direction=up&pageSize=10", "",
	)
	if rankings.Code != http.StatusOK {
		t.Fatalf("rankings status=%d body=%s", rankings.Code, rankings.Body.String())
	}
	if got := responseStringField(rankings.Body.String(), "instrumentId"); got != "US.AAPL" {
		t.Fatalf("rankings instrumentId = %q body=%s", got, rankings.Body.String())
	}
	if reader.rankings.Kind != "gainers" {
		t.Fatalf("rankings kind forwarded = %q", reader.rankings.Kind)
	}
	if got := responseStringField(rankings.Body.String(), "selectionReason"); got != "embedded-market-data-provider" {
		t.Fatalf("rankings selectionReason = %q", got)
	}

	boards := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/industries?brokerId=yfinance&market=CN&operation=plate_list&plateType=concept", "",
	)
	if boards.Code != http.StatusOK {
		t.Fatalf("industries status=%d body=%s", boards.Code, boards.Body.String())
	}
	if reader.boards.Kind != "concept" {
		t.Fatalf("industries kind forwarded = %q", reader.boards.Kind)
	}
	if got := responseStringField(boards.Body.String(), "instrumentId"); got != "CN.人工智能" {
		t.Fatalf("board instrumentId = %q body=%s", got, boards.Body.String())
	}

	members := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/industries?brokerId=yfinance&market=CN&operation=plate_members&instrumentId=CN.半导体&pageSize=20", "",
	)
	if members.Code != http.StatusOK {
		t.Fatalf("members status=%d body=%s", members.Code, members.Body.String())
	}
	if reader.members.Board != "半导体" {
		t.Fatalf("members board forwarded = %q", reader.members.Board)
	}
	if got := responseStringField(members.Body.String(), "instrumentId"); got != "CN.半导体" {
		t.Fatalf("resolved plate instrumentId = %q body=%s", got, members.Body.String())
	}
	if !strings.Contains(members.Body.String(), `"SH.688981"`) {
		t.Fatalf("member entry missing from body=%s", members.Body.String())
	}
}

func TestEmbeddedProviderRankingsRouteMapsUnsupportedOperations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &apiEmbeddedReader{}
	router := newEmbeddedProviderRouter(reader)

	// pre_market has no embedded feed: the facade must answer 409 instead of a
	// broker-registry failure.
	unsupported := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/rankings?brokerId=yfinance&market=US&operation=pre_market", "",
	)
	if unsupported.Code != http.StatusConflict {
		t.Fatalf("unsupported operation status=%d body=%s", unsupported.Code, unsupported.Body.String())
	}

	reader.rankingsErr = fmt.Errorf("%w: active provider %q does not support market rankings",
		marketdatasrv.ErrCapabilityUnsupported, "yfinance")
	unavailable := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/rankings?brokerId=yfinance&market=HK&operation=hot", "",
	)
	if unavailable.Code != http.StatusConflict {
		t.Fatalf("capability error status=%d body=%s", unavailable.Code, unavailable.Body.String())
	}

	reader.rankingsErr = marketdatasrv.ErrProviderBusy
	busy := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/rankings?brokerId=yfinance&market=US&operation=hot", "",
	)
	if busy.Code != http.StatusServiceUnavailable || busy.Header().Get("Retry-After") != "2" {
		t.Fatalf("busy status=%d retry=%q", busy.Code, busy.Header().Get("Retry-After"))
	}
}

func TestEmbeddedProviderCompanyResearchRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	revenue := json.Number("416161000000")
	rating := json.Number("4")
	holderPct := json.Number("54.07")
	reader := &apiEmbeddedReader{
		profile: marketdatasrv.CompanyProfileResponse{
			InstrumentID: "US.AAPL", Source: "yfinance-profile",
			Groups: []marketdatasrv.CompanyProfileGroup{{
				Title:  "公司概要",
				Fields: []marketdatasrv.CompanyProfileField{{Name: "行业", Value: "消费电子"}},
			}},
		},
		statements: marketdatasrv.FinancialStatementsResponse{
			InstrumentID: "SH.600519", Source: "akshare-financials",
			Fields: []marketdatasrv.FinancialStatementField{{FieldID: "total_revenue", DisplayName: "总营收"}},
			Periods: []marketdatasrv.FinancialStatementPeriod{{
				PeriodText: "2025财年",
				Values:     map[string]marketdatasrv.FinancialStatementValue{"total_revenue": {Data: &revenue}},
			}},
		},
		consensus: marketdatasrv.AnalystConsensusResponse{
			InstrumentID: "US.AAPL", Source: "yfinance-analyst", Rating: &rating,
		},
		ownership: marketdatasrv.OwnershipResponse{
			InstrumentID: "SH.600519", Source: "akshare-ownership",
			Groups: []marketdatasrv.OwnershipGroup{{
				Kind:  marketdatasrv.OwnershipGroupMajorHolders,
				Items: []marketdatasrv.OwnershipItem{{Name: "茅台集团", HolderPct: &holderPct}},
			}},
		},
	}
	router := newEmbeddedProviderRouter(reader)

	profile := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/instruments/US.AAPL?brokerId=yfinance&operation=profile", "",
	)
	if profile.Code != http.StatusOK {
		t.Fatalf("profile status=%d body=%s", profile.Code, profile.Body.String())
	}
	if !strings.Contains(profile.Body.String(), `"fieldType":"title"`) ||
		!strings.Contains(profile.Body.String(), `"fieldType":"text"`) {
		t.Fatalf("profile entries missing from body=%s", profile.Body.String())
	}
	if reader.profile.Market != "US" || reader.profile.Symbol != "AAPL" {
		t.Fatalf("profile instrument forwarded = %q %q", reader.profile.Market, reader.profile.Symbol)
	}

	financials := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/financials/SH.600519?brokerId=yfinance&operation=statements&statement=cashflow", "",
	)
	if financials.Code != http.StatusOK {
		t.Fatalf("financials status=%d body=%s", financials.Code, financials.Body.String())
	}
	if !strings.Contains(financials.Body.String(), `"structureList"`) ||
		!strings.Contains(financials.Body.String(), `"total_revenue"`) {
		t.Fatalf("financials projection missing from body=%s", financials.Body.String())
	}
	if reader.statements.Statement != "cashflow" {
		t.Fatalf("statement param forwarded = %q", reader.statements.Statement)
	}

	analyst := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/analyst/US.AAPL?brokerId=yfinance&operation=consensus", "",
	)
	if analyst.Code != http.StatusOK {
		t.Fatalf("analyst status=%d body=%s", analyst.Code, analyst.Body.String())
	}
	if !strings.Contains(analyst.Body.String(), `"rating":4`) {
		t.Fatalf("analyst rating missing from body=%s", analyst.Body.String())
	}

	ownership := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/ownership/SH.600519?brokerId=yfinance&operation=overview", "",
	)
	if ownership.Code != http.StatusOK {
		t.Fatalf("ownership status=%d body=%s", ownership.Code, ownership.Body.String())
	}
	if !strings.Contains(ownership.Body.String(), `"mainHolderInfoList"`) ||
		!strings.Contains(ownership.Body.String(), `"holderTypeInfoList"`) {
		t.Fatalf("ownership metadata missing from body=%s", ownership.Body.String())
	}
}

func TestEmbeddedProviderCompanyResearchRejectsUnsupportedOperations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &apiEmbeddedReader{}
	router := newEmbeddedProviderRouter(reader)

	// Each embedded research feature serves exactly one operation; anything
	// else must answer 409 instead of leaking to the broker registry.
	for _, path := range []string{
		"/api/v1/research/instruments/US.AAPL?brokerId=yfinance&operation=deep_dive",
		"/api/v1/research/financials/US.AAPL?brokerId=yfinance&operation=guidance",
		"/api/v1/research/analyst/US.AAPL?brokerId=yfinance&operation=estimate_trend",
		"/api/v1/research/ownership/US.AAPL?brokerId=yfinance&operation=holder_changes",
	} {
		response := performFeatureRequest(t, router, http.MethodGet, path, "")
		if response.Code != http.StatusConflict {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}

	reader.profileErr = fmt.Errorf("%w: active provider %q does not support company profile",
		marketdatasrv.ErrCapabilityUnsupported, "yfinance")
	unavailable := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/instruments/US.AAPL?brokerId=yfinance", "",
	)
	if unavailable.Code != http.StatusConflict {
		t.Fatalf("capability error status=%d body=%s", unavailable.Code, unavailable.Body.String())
	}

	reader.profileErr = marketdatasrv.ErrProviderWarming
	warming := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/instruments/US.AAPL?brokerId=yfinance&operation=profile", "",
	)
	if warming.Code != http.StatusServiceUnavailable ||
		warming.Header().Get("Retry-After") != "1" {
		t.Fatalf("warming status=%d retry=%q", warming.Code, warming.Header().Get("Retry-After"))
	}
}

func TestEmbeddedProviderCalendarAndMacroRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	price := json.Number("1680.5")
	value := json.Number("0.5")
	reader := &apiEmbeddedReader{
		earnings: marketdatasrv.EarningsCalendarResponse{
			Source: "akshare-calendar",
			Entries: []marketdatasrv.EarningsEvent{{
				InstrumentID: "SH.600519", Name: "贵州茅台", Symbol: "600519",
				EventDate: "2026-08-20", Price: &price,
			}},
		},
		dividends: marketdatasrv.DividendCalendarResponse{
			Source: "akshare-calendar",
			Entries: []marketdatasrv.DividendEvent{{
				InstrumentID: "SZ.000001", Statement: "10派2元", ExDate: "2026-08-15",
			}},
		},
		economic: marketdatasrv.EconomicCalendarResponse{
			Source: "akshare-calendar",
			Entries: []marketdatasrv.EconomicEvent{{
				EventID: "econ-1", Title: "CPI同比", Region: "中国", EventTimestamp: 1787004000,
			}},
		},
		ipos: marketdatasrv.IpoCalendarResponse{
			Source: "akshare-calendar",
			Entries: []marketdatasrv.IpoEntry{{
				InstrumentID: "SZ.301999", Name: "新股示例", Status: "pending",
			}},
		},
		indicators: marketdatasrv.MacroIndicatorsResponse{
			Source: "akshare-macro",
			Categories: []marketdatasrv.MacroIndicatorCategory{{
				CategoryName: "价格",
				Indicators:   []marketdatasrv.MacroIndicator{{IndicatorID: "cpi_yoy", Name: "CPI同比"}},
			}},
		},
		history: marketdatasrv.MacroIndicatorHistoryResponse{
			IndicatorID: "cpi_yoy", Source: "akshare-macro",
			Entries: []marketdatasrv.MacroIndicatorPoint{{DataTime: "2026-07", Value: &value}},
		},
	}
	router := newEmbeddedProviderRouter(reader)

	earnings := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/calendars?brokerId=yfinance&market=CN&operation=earnings&beginDate=2026-08-01&endDate=2026-08-31", "",
	)
	if earnings.Code != http.StatusOK {
		t.Fatalf("earnings status=%d body=%s", earnings.Code, earnings.Body.String())
	}
	if !strings.Contains(earnings.Body.String(), `"eventDate":"2026-08-20"`) ||
		!strings.Contains(earnings.Body.String(), `"market":"SH"`) {
		t.Fatalf("earnings body=%s", earnings.Body.String())
	}
	if reader.earnings.BeginDate != "2026-08-01" || reader.earnings.EndDate != "2026-08-31" {
		t.Fatalf("earnings range forwarded = %q/%q", reader.earnings.BeginDate, reader.earnings.EndDate)
	}

	dividends := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/calendars?brokerId=yfinance&market=SH&operation=dividends&date=2026-08-15", "",
	)
	if dividends.Code != http.StatusOK || reader.dividends.Date != "2026-08-15" {
		t.Fatalf("dividends status=%d forwarded=%q body=%s",
			dividends.Code, reader.dividends.Date, dividends.Body.String())
	}

	economic := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/calendars?brokerId=yfinance&market=SH&operation=economic&beginDate=2026-08-01&endDate=2026-08-07", "",
	)
	if economic.Code != http.StatusOK ||
		!strings.Contains(economic.Body.String(), `"eventId":"econ-1"`) ||
		!strings.Contains(economic.Body.String(), `"hasMore":false`) {
		t.Fatalf("economic status=%d body=%s", economic.Code, economic.Body.String())
	}

	ipos := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/calendars?brokerId=yfinance&market=CN&operation=ipos", "",
	)
	if ipos.Code != http.StatusOK || !strings.Contains(ipos.Body.String(), `"status":"pending"`) {
		t.Fatalf("ipos status=%d body=%s", ipos.Code, ipos.Body.String())
	}

	indicators := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/macro?brokerId=yfinance&market=US&operation=indicators", "",
	)
	if indicators.Code != http.StatusOK ||
		!strings.Contains(indicators.Body.String(), `"categoryName":"价格"`) ||
		!strings.Contains(indicators.Body.String(), `"indicatorList"`) {
		t.Fatalf("indicators status=%d body=%s", indicators.Code, indicators.Body.String())
	}

	history := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/macro?brokerId=yfinance&market=US&operation=indicator_history&indicatorId=cpi_yoy&pageSize=60", "",
	)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), `"dataTime":"2026-07"`) {
		t.Fatalf("history status=%d body=%s", history.Code, history.Body.String())
	}
	if reader.historyID != "cpi_yoy" || reader.historyLimit != 60 {
		t.Fatalf("history forwarded = %q/%d", reader.historyID, reader.historyLimit)
	}
}

func TestEmbeddedProviderCalendarMacroRoutesRejectUnsupportedOperations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &apiEmbeddedReader{}
	router := newEmbeddedProviderRouter(reader)

	for _, path := range []string{
		"/api/v1/research/calendars?brokerId=yfinance&market=CN&operation=trade_dates",
		"/api/v1/research/macro?brokerId=yfinance&market=US&operation=fed_target_rate",
		"/api/v1/research/macro?brokerId=yfinance&market=US&operation=fed_dot_plot",
	} {
		response := performFeatureRequest(t, router, http.MethodGet, path, "")
		if response.Code != http.StatusConflict {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}

	reader.economicErr = marketdatasrv.ErrProviderWarming
	warming := performFeatureRequest(
		t, router, http.MethodGet,
		"/api/v1/research/calendars?brokerId=yfinance&market=SH&operation=economic", "",
	)
	if warming.Code != http.StatusServiceUnavailable ||
		warming.Header().Get("Retry-After") != "1" {
		t.Fatalf("warming status=%d retry=%q", warming.Code, warming.Header().Get("Retry-After"))
	}
}
