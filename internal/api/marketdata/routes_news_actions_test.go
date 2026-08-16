package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	srv "github.com/jftrade/jftrade-main/internal/marketdata"
)

type routeNewsProvider struct {
	routeTestProvider
	newsMarket    string
	newsSymbol    string
	newsLimit     int
	newsErr       error
	actionsMarket string
	actionsSymbol string
	actionsFrom   time.Time
	actionsTo     time.Time
	actionsErr    error
}

func (p *routeNewsProvider) News(_ context.Context, market, symbol string, limit int) (srv.NewsResponse, error) {
	p.newsMarket, p.newsSymbol, p.newsLimit = market, symbol, limit
	title := "fixture headline"
	return srv.NewsResponse{
		Market: market, Symbol: symbol, InstrumentID: market + "." + symbol,
		Entries: []srv.NewsEntry{{Title: &title}}, Source: "yfinance-news",
	}, p.newsErr
}

func (p *routeNewsProvider) CorporateActions(
	_ context.Context,
	market string,
	symbol string,
	from time.Time,
	to time.Time,
) (srv.CorporateActionsResponse, error) {
	p.actionsMarket, p.actionsSymbol, p.actionsFrom, p.actionsTo = market, symbol, from, to
	return srv.CorporateActionsResponse{
		Market: market, Symbol: symbol, InstrumentID: market + "." + symbol,
		Events: []srv.CorporateActionEvent{{Kind: "dividend", ExDate: "2026-05-11"}},
		Source: "yfinance-actions",
	}, p.actionsErr
}

func TestNewsAndCorporateActionsRoutesRequireInstrumentURI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := srv.NewService(&routeNewsProvider{})
	for name, handler := range map[string]gin.HandlerFunc{
		"news":              handleNews(service),
		"corporate actions": handleCorporateActions(service),
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ginContext, _ := gin.CreateTestContext(response)
			ginContext.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)

			handler(ginContext)

			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"BAD_REQUEST"`) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestNewsRouteValidatesLimitAndForwardsToService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeNewsProvider{}
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), srv.NewService(provider))

	for _, path := range []string{
		"/api/v1/market-data/news/US/AAPL?limit=abc",
		"/api/v1/market-data/news/US/AAPL?limit=0",
		"/api/v1/market-data/news/US/AAPL?limit=51",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s response = %d %s", path, response.Code, response.Body.String())
		}
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/news/US/AAPL?limit=5", nil),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("news response = %d %s", response.Code, response.Body.String())
	}
	if provider.newsMarket != "US" || provider.newsSymbol != "AAPL" || provider.newsLimit != 5 {
		t.Fatalf("news service args = %s/%s/%d", provider.newsMarket, provider.newsSymbol, provider.newsLimit)
	}
	var envelope struct {
		Data srv.NewsResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil ||
		envelope.Data.InstrumentID != "US.AAPL" || envelope.Data.Source != "yfinance-news" ||
		len(envelope.Data.Entries) != 1 {
		t.Fatalf("news payload = %s, err=%v", response.Body.String(), err)
	}
}

func TestCorporateActionsRouteValidatesRangeAndForwardsToService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &routeNewsProvider{}
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), srv.NewService(provider))

	for _, path := range []string{
		"/api/v1/market-data/corporate-actions/US/AAPL?from=not-a-time",
		"/api/v1/market-data/corporate-actions/US/AAPL?to=not-a-time",
		"/api/v1/market-data/corporate-actions/US/AAPL?from=2026-01-01T00:00:00Z&to=2025-01-01T00:00:00Z",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s response = %d %s", path, response.Code, response.Body.String())
		}
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequestWithContext(
			t.Context(), http.MethodGet,
			"/api/v1/market-data/corporate-actions/US/AAPL?from=2025-01-01T00:00:00Z&to=2026-01-01T00:00:00Z", nil,
		),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("corporate actions response = %d %s", response.Code, response.Body.String())
	}
	wantFrom := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if provider.actionsMarket != "US" || provider.actionsSymbol != "AAPL" ||
		!provider.actionsFrom.Equal(wantFrom) || !provider.actionsTo.Equal(wantTo) {
		t.Fatalf("actions service args = %s/%s/%v/%v",
			provider.actionsMarket, provider.actionsSymbol, provider.actionsFrom, provider.actionsTo)
	}
	var envelope struct {
		Data srv.CorporateActionsResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil ||
		len(envelope.Data.Events) != 1 || envelope.Data.Events[0].Kind != "dividend" {
		t.Fatalf("corporate actions payload = %s, err=%v", response.Body.String(), err)
	}
}

func TestNewsAndCorporateActionsRoutesMapCapabilityAndProviderFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	unsupported := gin.New()
	RegisterRoutes(unsupported.Group("/api/v1"), srv.NewService(&routeTestProvider{}))
	for _, path := range []string{
		"/api/v1/market-data/news/US/AAPL",
		"/api/v1/market-data/corporate-actions/US/AAPL",
	} {
		response := httptest.NewRecorder()
		unsupported.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		if response.Code != http.StatusConflict ||
			!strings.Contains(response.Body.String(), `"code":"MARKET_DATA_CAPABILITY_UNSUPPORTED"`) {
			t.Fatalf("%s response = %d %s", path, response.Code, response.Body.String())
		}
	}

	provider := &routeNewsProvider{
		newsErr:    errors.New("news feed unavailable"),
		actionsErr: srv.ErrProviderBusy,
	}
	failing := gin.New()
	RegisterRoutes(failing.Group("/api/v1"), srv.NewService(provider))

	response := httptest.NewRecorder()
	failing.ServeHTTP(
		response,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/news/US/AAPL", nil),
	)
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), `"code":"MARKET_NEWS_FAILED"`) {
		t.Fatalf("news failure response = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	failing.ServeHTTP(
		response,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/market-data/corporate-actions/US/AAPL", nil),
	)
	if response.Code != http.StatusServiceUnavailable ||
		!strings.Contains(response.Body.String(), `"code":"MARKET_DATA_PROVIDER_BUSY"`) ||
		response.Header().Get("Retry-After") == "" {
		t.Fatalf("busy actions response = %d %s", response.Code, response.Body.String())
	}
}
