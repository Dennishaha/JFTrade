package system

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
		{http.MethodGet, "/api/v1/system/real-trade-approvals", []string{"realTradingEnabled", "requiredConfirmationText", "approvalWorkflowStatus", "approvalPolicy", "entries"}},
		{http.MethodGet, "/api/v1/system/real-trade-hard-stops", []string{"blockedOperations", "entries"}},
		{http.MethodGet, "/api/v1/system/real-trade-hard-stop-events", []string{"blockedOperations", "entries"}},
		{http.MethodGet, "/api/v1/system/real-trade-kill-switch", []string{"killSwitchActive", "blockedOperations", "entry"}},
		{http.MethodGet, "/api/v1/system/real-trade-kill-switch-events", []string{"killSwitchActive", "entries"}},
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

func TestRealTradeControlRoutesDelegateStateChanges(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	killSwitchActive := false
	hardStopActive := false
	hardStopID := "hs-1"
	svc := sysservice.NewService(
		sysservice.WithRealTradeRiskState(func() map[string]any {
			entries := []map[string]any{}
			if hardStopActive {
				entries = append(entries, map[string]any{
					"id":                 hardStopID,
					"brokerId":           "futu",
					"tradingEnvironment": "REAL",
					"accountId":          "ACC-1",
				})
			}
			return map[string]any{
				"realTradingEnabled":           true,
				"killSwitchActive":             killSwitchActive,
				"killSwitchControlPlaneActive": killSwitchActive,
				"killSwitchSource":             "CONTROL_PLANE",
				"hardStopEntries":              entries,
			}
		}),
		sysservice.WithRealTradeKillSwitchControls(
			func(context.Context, sysservice.RealTradeKillSwitchCommand) (map[string]any, error) {
				killSwitchActive = true
				return map[string]any{"killSwitchActive": true}, nil
			},
			func(context.Context, sysservice.RealTradeKillSwitchCommand) (map[string]any, error) {
				killSwitchActive = false
				return map[string]any{"killSwitchActive": false}, nil
			},
		),
		sysservice.WithRealTradeHardStopControls(
			func(context.Context, sysservice.RealTradeHardStopCommand) (map[string]any, error) {
				hardStopActive = true
				return map[string]any{"entries": []any{map[string]any{"id": hardStopID}}}, nil
			},
			func(_ context.Context, id string, _ sysservice.RealTradeHardStopCommand) (map[string]any, error) {
				if id == hardStopID {
					hardStopActive = false
				}
				return map[string]any{"entries": []any{}}, nil
			},
		),
	)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), svc)

	resp := performSystemRouteJSONRequest(router, http.MethodPost, "/api/v1/system/real-trade-kill-switch/activate", `{"operatorId":"tester","reason":"incident"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("activate kill switch status = %d body=%s", resp.Code, resp.Body.String())
	}
	if data := decodeSystemRouteData(t, resp); data["killSwitchActive"] != true {
		t.Fatalf("activate kill switch data = %#v", data)
	}
	if data := decodeSystemRouteData(t, performSystemRouteRequest(router, http.MethodGet, "/api/v1/system/real-trade-kill-switch")); data["killSwitchActive"] != true {
		t.Fatalf("GET kill switch data = %#v", data)
	}

	resp = performSystemRouteJSONRequest(router, http.MethodPost, "/api/v1/system/real-trade-hard-stops", `{"accountId":"ACC-1","market":"US","symbol":"AAPL"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("activate hard stop status = %d body=%s", resp.Code, resp.Body.String())
	}
	if data := decodeSystemRouteData(t, performSystemRouteRequest(router, http.MethodGet, "/api/v1/system/real-trade-hard-stops")); len(data["entries"].([]any)) != 1 {
		t.Fatalf("GET hard stops data = %#v", data)
	}

	resp = performSystemRouteJSONRequest(router, http.MethodPost, "/api/v1/system/real-trade-hard-stops/hs-1/release", `{"operatorId":"tester"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("release hard stop status = %d body=%s", resp.Code, resp.Body.String())
	}
	if data := decodeSystemRouteData(t, performSystemRouteRequest(router, http.MethodGet, "/api/v1/system/real-trade-hard-stops")); len(data["entries"].([]any)) != 0 {
		t.Fatalf("GET released hard stops data = %#v", data)
	}

	resp = performSystemRouteJSONRequest(router, http.MethodPost, "/api/v1/system/real-trade-kill-switch/release", `{"operatorId":"tester"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("release kill switch status = %d body=%s", resp.Code, resp.Body.String())
	}
	if data := decodeSystemRouteData(t, performSystemRouteRequest(router, http.MethodGet, "/api/v1/system/real-trade-kill-switch")); data["killSwitchActive"] != false {
		t.Fatalf("GET released kill switch data = %#v", data)
	}
}

func TestRealTradeControlRoutesMapValidationAndControlFailures(t *testing.T) {
	gatewayErr := errors.New("control persistence unavailable")
	svc := sysservice.NewService(
		sysservice.WithRealTradeKillSwitchControls(
			func(context.Context, sysservice.RealTradeKillSwitchCommand) (map[string]any, error) {
				return nil, gatewayErr
			},
			func(context.Context, sysservice.RealTradeKillSwitchCommand) (map[string]any, error) {
				return nil, gatewayErr
			},
		),
		sysservice.WithRealTradeHardStopControls(
			func(context.Context, sysservice.RealTradeHardStopCommand) (map[string]any, error) {
				return nil, gatewayErr
			},
			func(context.Context, string, sysservice.RealTradeHardStopCommand) (map[string]any, error) {
				return nil, gatewayErr
			},
		),
	)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), svc)

	for _, path := range []string{
		"/api/v1/system/real-trade-kill-switch/activate",
		"/api/v1/system/real-trade-hard-stops",
	} {
		response := performSystemRouteJSONRequest(router, http.MethodPost, path, `{`)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s bad JSON status = %d body=%s", path, response.Code, response.Body.String())
		}
	}
	for _, test := range []struct {
		path string
		body string
	}{
		{path: "/api/v1/system/real-trade-kill-switch/activate", body: `{}`},
		{path: "/api/v1/system/real-trade-kill-switch/release", body: `{}`},
		{path: "/api/v1/system/real-trade-hard-stops", body: `{}`},
		{path: "/api/v1/system/real-trade-hard-stops/hs-1/release", body: `{}`},
	} {
		response := performSystemRouteJSONRequest(router, http.MethodPost, test.path, test.body)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "REAL_TRADE_CONTROL_FAILED") {
			t.Fatalf("%s control error = %d body=%s", test.path, response.Code, response.Body.String())
		}
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

func performSystemRouteJSONRequest(router http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
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
