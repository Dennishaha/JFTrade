package system

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	sysservice "github.com/jftrade/jftrade-main/internal/system"
)

func TestSystemRoutesReturnEnvelopes(t *testing.T) {
	router, _ := newSystemRouteTestRouter()

	tests := []struct {
		method   string
		path     string
		wantKeys []string
	}{
		{http.MethodGet, "/api/v1/system/status", []string{"apiPort"}},
		{http.MethodGet, "/api/v1/system/exchange-calendars/status", []string{"autoRefreshEnabled"}},
		{http.MethodGet, "/api/v1/system/exchange-calendars/sources", []string{"sources"}},
		{http.MethodPost, "/api/v1/system/exchange-calendars/probe", []string{"accepted", "healthy"}},
		{http.MethodGet, "/api/v1/system/futu-opend", []string{"status"}},
		{http.MethodGet, "/api/v1/system/futu-opend/install-guide", []string{"downloadUrl"}},
		{http.MethodGet, "/api/v1/system/runtime-dependencies", []string{"checkedAt", "allRequiredSatisfied", "dependencies"}},
		{http.MethodGet, "/api/v1/system/storage/overview", []string{"pendingOutbox"}},
		{http.MethodGet, "/api/v1/system/real-trade-approvals", []string{"realTradingEnabled", "requiredConfirmationText", "entries"}},
		{http.MethodGet, "/api/v1/system/real-trade-hard-stops", []string{"blockedOperations", "entries"}},
		{http.MethodGet, "/api/v1/system/real-trade-kill-switch", []string{"killSwitchActive", "blockedOperations", "entry"}},
		{http.MethodGet, "/api/v1/system/real-trade-risk-limits", []string{"riskEnabled", "effectiveMaxOrderQuantity", "entry"}},
		{http.MethodGet, "/api/v1/system/real-trade-risk-events", []string{"riskEnabled", "maxOrderQuantity", "entries"}},
		{http.MethodGet, "/api/v1/system/worker/broker-order-updates", []string{"running"}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp := performSystemRouteRequest(router, tt.method, tt.path)
			if resp.Code != http.StatusOK {
				t.Fatalf("status = %d", resp.Code)
			}
			data := decodeSystemRouteData(t, resp)
			for _, key := range tt.wantKeys {
				if _, ok := data[key]; !ok {
					t.Fatalf("data missing %q: %#v", key, data)
				}
			}
		})
	}
}

func TestSystemManualRetryRouteCallsReset(t *testing.T) {
	router, resetCalled := newSystemRouteTestRouter()

	resp := performSystemRouteRequest(router, http.MethodPost, "/api/v1/system/futu-opend/manual-retry")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	if !*resetCalled {
		t.Fatal("expected reset callback to be called")
	}
	data := decodeSystemRouteData(t, resp)
	if data["accepted"] != true {
		t.Fatalf("accepted = %#v", data["accepted"])
	}
}

func TestExchangeCalendarRefreshRouteCallsRefresh(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	refreshedMarket := ""
	svc := sysservice.NewService(
		sysservice.WithRefreshExchangeCalendars(func(_ context.Context, market string) map[string]any {
			refreshedMarket = market
			return map[string]any{"accepted": true, "market": market}
		}),
	)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), svc)

	resp := performSystemRouteRequest(router, http.MethodPost, "/api/v1/system/exchange-calendars/refresh/US")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	if refreshedMarket != "US" {
		t.Fatalf("refreshedMarket = %q", refreshedMarket)
	}
}

func TestExchangeCalendarProbeRouteCallsProbe(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	probedMarket := ""
	svc := sysservice.NewService(
		sysservice.WithProbeExchangeCalendars(func(_ context.Context, market string) map[string]any {
			probedMarket = market
			return map[string]any{"accepted": true, "market": market, "healthy": 1}
		}),
	)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), svc)

	resp := performSystemRouteRequest(router, http.MethodPost, "/api/v1/system/exchange-calendars/probe/HK")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	if probedMarket != "HK" {
		t.Fatalf("probedMarket = %q", probedMarket)
	}
}

func newSystemRouteTestRouter() (*gin.Engine, *bool) {
	gin.SetMode(gin.ReleaseMode)
	resetCalled := false
	refreshCalled := ""
	svc := sysservice.NewService(
		sysservice.WithAPIPort(6699),
		sysservice.WithExchangeCalendarStatus(func() map[string]any {
			return map[string]any{"autoRefreshEnabled": true}
		}),
		sysservice.WithExchangeCalendarSources(func() []map[string]any {
			return []map[string]any{{"id": "builtin_rules"}}
		}),
		sysservice.WithRefreshExchangeCalendars(func(context.Context, string) map[string]any {
			refreshCalled = "all"
			return map[string]any{"accepted": true}
		}),
		sysservice.WithProbeExchangeCalendars(func(context.Context, string) map[string]any {
			return map[string]any{"accepted": true, "healthy": 1}
		}),
		sysservice.WithFutuOpenDHealth(func(context.Context) map[string]any {
			return map[string]any{"status": "ok"}
		}),
		sysservice.WithFutuOpenDInstallGuide(func() map[string]any {
			return map[string]any{"downloadUrl": "https://example.test/opend"}
		}),
		sysservice.WithResetFutuRuntime(func() {
			resetCalled = true
		}),
		sysservice.WithBrokerOrderSnapshot(func() map[string]any {
			return map[string]any{"running": false}
		}),
		sysservice.WithRuntimeDependencies(func(context.Context) map[string]any {
			return map[string]any{
				"checkedAt":            "2026-06-29T00:00:00Z",
				"allRequiredSatisfied": true,
				"dependencies":         []any{},
			}
		}),
	)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), svc)
	_ = refreshCalled
	return router, &resetCalled
}

func performSystemRouteRequest(router http.Handler, method string, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func decodeSystemRouteData(t *testing.T, resp *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("ok = false")
	}
	return envelope.Data
}
