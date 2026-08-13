package servercoretest

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
)

func TestSystemStatusEndpointReturnsStatus(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/system/status")
	if err != nil {
		t.Fatalf("GET system status: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET system status = %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode system status: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected ok=true")
	}
	if got := envelope.Data["name"]; got != "JFTrade" {
		t.Fatalf("system name = %v", got)
	}
	if _, ok := envelope.Data["broker"]; !ok {
		t.Fatal("expected broker in system status response")
	}
	build, ok := envelope.Data["build"].(map[string]any)
	if !ok {
		t.Fatalf("expected build metadata, got %+v", envelope.Data["build"])
	}
	if build["version"] == "" || build["commit"] == "" {
		t.Fatalf("expected build version and commit, got %+v", build)
	}
	strategyRuntime, ok := envelope.Data["strategyRuntime"].(map[string]any)
	if !ok {
		t.Fatalf("expected strategyRuntime summary, got %+v", envelope.Data["strategyRuntime"])
	}
	if got := int(jftradeCheckedTypeAssertion[float64](strategyRuntime["activeStrategies"])); got != 0 {
		t.Fatalf("activeStrategies = %d", got)
	}
	activeInstances, ok := strategyRuntime["activeInstances"].([]any)
	if !ok {
		t.Fatalf("expected activeInstances list, got %+v", strategyRuntime["activeInstances"])
	}
	if len(activeInstances) != 0 {
		t.Fatalf("expected no active runtime instances, got %+v", activeInstances)
	}
	observability, ok := envelope.Data["observability"].(map[string]any)
	if !ok {
		t.Fatalf("expected observability summary, got %+v", envelope.Data["observability"])
	}
	api, ok := observability["api"].(map[string]any)
	if !ok || api["startedAt"] == "" {
		t.Fatalf("expected api observability, got %+v", observability["api"])
	}
	live, ok := observability["live"].(map[string]any)
	if !ok {
		t.Fatalf("expected live observability, got %+v", observability["live"])
	}
	if got := int(jftradeCheckedTypeAssertion[float64](live["connected"])); got != 0 {
		t.Fatalf("live connected = %d, want 0", got)
	}
	marketdata, ok := observability["marketdata"].(map[string]any)
	if !ok {
		t.Fatalf("expected marketdata observability, got %+v", observability["marketdata"])
	}
	if got := marketdata["status"]; got == "" {
		t.Fatalf("expected marketdata status, got %+v", marketdata)
	}
}

func TestRequestObservabilityMiddlewarePropagatesRequestID(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	const requestIDHeader = "X-Request-ID"
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/system/status", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set(requestIDHeader, "test-request-id")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET system status: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if got := resp.Header.Get(requestIDHeader); got != "test-request-id" {
		t.Fatalf("%s = %q, want propagated request id", requestIDHeader, got)
	}
}

func TestSystemStatusReflectsUpdatedAPIPort(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	handler, srv := newHTTPTestServerWithHandler(t, store)
	handler.SetAPIPort(38401)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/system/status")
	if err != nil {
		t.Fatalf("GET system status: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET system status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode system status: %v", err)
	}
	if got := int(jftradeCheckedTypeAssertion[float64](envelope.Data["apiPort"])); got != 38401 {
		t.Fatalf("apiPort = %d, want 38401", got)
	}
}
