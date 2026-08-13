package settings_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apisettings "github.com/jftrade/jftrade-main/internal/api/settings"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/marketdata"
	srvsettings "github.com/jftrade/jftrade-main/internal/settings"
)

func TestMarketDataSettingsRoutesReadSaveAndApplyProvider(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	store := &routeStore{}
	var applied []jfsettings.ActiveMarketDataProvider
	service := srvsettings.NewService(store, srvsettings.WithSideEffects(srvsettings.SideEffects{
		OnProviderChanged: func(provider jfsettings.ActiveMarketDataProvider) error {
			applied = append(applied, provider)
			return nil
		},
	}))
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), service)

	active := performSettingsRequest(t, router, http.MethodGet, "/api/v1/settings/market-data-provider", "")
	if active.Code != http.StatusOK || !strings.Contains(active.Body.String(), `"activeProvider":"yfinance"`) {
		t.Fatalf("default provider response = %d %s", active.Code, active.Body.String())
	}

	switched := performSettingsRequest(
		t,
		router,
		http.MethodPut,
		"/api/v1/settings/market-data-provider",
		`{"activeProvider":" futu "}`,
	)
	if switched.Code != http.StatusOK ||
		!strings.Contains(switched.Body.String(), `"activeProvider":"futu"`) ||
		len(applied) != 1 || applied[0] != jfsettings.MarketDataProviderFutu {
		t.Fatalf("provider switch to futu = %d %s, callbacks=%#v", switched.Code, switched.Body.String(), applied)
	}

	switched = performSettingsRequest(
		t,
		router,
		http.MethodPut,
		"/api/v1/settings/market-data-provider",
		`{"activeProvider":"yfinance"}`,
	)
	if switched.Code != http.StatusOK ||
		!strings.Contains(switched.Body.String(), `"activeProvider":"yfinance"`) ||
		len(applied) != 2 || applied[1] != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("provider switch to yfinance = %d %s, callbacks=%#v", switched.Code, switched.Body.String(), applied)
	}
}

func TestBacktestMarketDataSettingsRoutesExposeCatalogAndRollbackPreparationFailure(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	store := &routeStore{activeProvider: jfsettings.MarketDataProviderFutu}
	prepareErr := errors.New("akshare helper unavailable")
	service := srvsettings.NewService(
		store,
		srvsettings.WithMarketDataProviderCatalog(func(context.Context) ([]marketdata.ProviderDescriptor, error) {
			return []marketdata.ProviderDescriptor{{
				ProviderID: "yfinance", SelectionID: "yfinance",
				Capabilities: marketdata.ProviderCapabilities{
					HistoricalCandles: true, PriceAdjustments: []string{"none"},
				},
			}}, nil
		}),
		srvsettings.WithSideEffects(srvsettings.SideEffects{
			OnBacktestProviderChanged: func(provider jfsettings.ActiveMarketDataProvider) error {
				if provider == jfsettings.MarketDataProviderAKShare {
					return prepareErr
				}
				return nil
			},
		}),
	)
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), service)

	read := performSettingsRequest(t, router, http.MethodGet, "/api/v1/settings/backtest-market-data-provider", "")
	if read.Code != http.StatusOK ||
		!strings.Contains(read.Body.String(), `"activeProvider":"futu"`) ||
		!strings.Contains(read.Body.String(), `"priceAdjustments":["none"]`) {
		t.Fatalf("backtest provider read = %d %s", read.Code, read.Body.String())
	}

	failed := performSettingsRequest(t, router, http.MethodPut,
		"/api/v1/settings/backtest-market-data-provider", `{"activeProvider":"akshare"}`)
	if failed.Code != http.StatusConflict ||
		!strings.Contains(failed.Body.String(), `"code":"MARKET_DATA_PROVIDER_UPDATE_FAILED"`) ||
		store.BacktestMarketDataProvider() != jfsettings.MarketDataProviderFutu {
		t.Fatalf("failed backtest switch = %d %s, stored=%q",
			failed.Code, failed.Body.String(), store.BacktestMarketDataProvider())
	}

	switched := performSettingsRequest(t, router, http.MethodPut,
		"/api/v1/settings/backtest-market-data-provider", `{"activeProvider":"yfinance"}`)
	if switched.Code != http.StatusOK ||
		!strings.Contains(switched.Body.String(), `"activeProvider":"yfinance"`) ||
		store.BacktestMarketDataProvider() != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("successful backtest switch = %d %s, stored=%q",
			switched.Code, switched.Body.String(), store.BacktestMarketDataProvider())
	}
}

func TestLegacyYFinanceConnectionRoutesAreRemoved(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	apisettings.RegisterRoutes(router.Group("/api/v1"), srvsettings.NewService(&routeStore{}))

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		response := performSettingsRequest(t, router, method, "/api/v1/settings/yfinance", "{}")
		if response.Code != http.StatusNotFound {
			t.Fatalf("legacy %s route status = %d, body = %s", method, response.Code, response.Body.String())
		}
	}
}

func TestMarketDataSettingsRoutesMapValidationPersistenceAndRuntimeErrors(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	tests := []struct {
		name       string
		body       string
		store      *routeStore
		sideEffect func(jfsettings.ActiveMarketDataProvider) error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "malformed provider payload",
			body:       `{`,
			store:      &routeStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "invalid active provider",
			body:       `{"activeProvider":"other"}`,
			store:      &routeStore{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MARKET_DATA_PROVIDER_INVALID",
		},
		{
			name:       "provider persistence failure",
			body:       `{"activeProvider":"futu"}`,
			store:      &routeStore{saveErr: errors.New("disk full")},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "SETTINGS_SAVE_FAILED",
		},
		{
			name:  "provider runtime failure",
			body:  `{"activeProvider":"futu"}`,
			store: &routeStore{},
			sideEffect: func(jfsettings.ActiveMarketDataProvider) error {
				return errors.New("provider unavailable")
			},
			wantStatus: http.StatusConflict,
			wantCode:   "MARKET_DATA_PROVIDER_UPDATE_FAILED",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := srvsettings.NewService(test.store, srvsettings.WithSideEffects(srvsettings.SideEffects{
				OnProviderChanged: test.sideEffect,
			}))
			router := gin.New()
			apisettings.RegisterRoutes(router.Group("/api/v1"), service)
			response := performSettingsRequest(
				t,
				router,
				http.MethodPut,
				"/api/v1/settings/market-data-provider",
				test.body,
			)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if test.name == "provider runtime failure" &&
				test.store.ActiveMarketDataProvider() != jfsettings.MarketDataProviderYFinance {
				t.Fatalf("runtime failure did not roll back provider: %q", test.store.ActiveMarketDataProvider())
			}
		})
	}
}
