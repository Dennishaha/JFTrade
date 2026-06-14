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
		path     string
		wantKeys []string
	}{
		{"/api/v1/system/status", []string{"apiPort"}},
		{"/api/v1/system/futu-opend", []string{"status"}},
		{"/api/v1/system/futu-opend/install-guide", []string{"downloadUrl"}},
		{"/api/v1/system/storage/overview", []string{"pendingOutbox"}},
		{"/api/v1/system/real-trade-approvals", []string{"realTradingEnabled", "requiredConfirmationText", "entries"}},
		{"/api/v1/system/real-trade-hard-stops", []string{"blockedOperations", "entries"}},
		{"/api/v1/system/real-trade-kill-switch", []string{"killSwitchActive", "blockedOperations", "entry"}},
		{"/api/v1/system/real-trade-risk-limits", []string{"riskEnabled", "effectiveMaxOrderQuantity", "entry"}},
		{"/api/v1/system/real-trade-risk-events", []string{"riskEnabled", "maxOrderQuantity", "entries"}},
		{"/api/v1/system/worker/broker-order-updates", []string{"running"}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp := performSystemRouteRequest(router, http.MethodGet, tt.path)
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

func newSystemRouteTestRouter() (*gin.Engine, *bool) {
	gin.SetMode(gin.ReleaseMode)
	resetCalled := false
	svc := sysservice.NewService(
		sysservice.WithAPIPort(6699),
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
	)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"), svc)
	return router, &resetCalled
}

func performSystemRouteRequest(router http.Handler, method string, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
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
