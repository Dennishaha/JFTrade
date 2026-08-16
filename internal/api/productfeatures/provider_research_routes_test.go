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
