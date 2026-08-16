package productfeatures

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
