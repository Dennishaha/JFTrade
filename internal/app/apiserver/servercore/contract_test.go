package servercore

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

// TestContractSystemStatus 验证 /api/v1/system/status 响应信封与关键字段。
func TestContractSystemStatus(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/system/status")
	if err != nil {
		t.Fatalf("GET /api/v1/system/status: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var env struct {
		OK        bool           `json:"ok"`
		Timestamp string         `json:"timestamp"`
		Data      map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !env.OK {
		t.Fatal("ok != true")
	}
	if env.Timestamp == "" {
		t.Fatal("timestamp is empty")
	}
	if env.Data == nil {
		t.Fatal("data is nil")
	}

	// 验证关键字段存在
	requiredFields := []string{"name", "apiPort", "defaultBroker", "build", "persistence", "runtimeResources"}
	for _, field := range requiredFields {
		if _, ok := env.Data[field]; !ok {
			t.Errorf("missing field: %s", field)
		}
	}

	// 验证 build 子对象含有 version 字段
	if build, ok := env.Data["build"].(map[string]any); ok {
		if _, ok := build["version"]; !ok {
			t.Error("build.version missing")
		}
	} else {
		t.Error("build is not a map")
	}
	runtimeResources, ok := env.Data["runtimeResources"].(map[string]any)
	if !ok {
		t.Fatalf("runtimeResources is not a map: %#v", env.Data["runtimeResources"])
	}
	items, ok := runtimeResources["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("runtimeResources.items = %#v", runtimeResources["items"])
	}
	foundExecution := false
	for _, item := range items {
		resource, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if resource["id"] == "execution-orders-db" && resource["owner"] == "trading" {
			foundExecution = true
			break
		}
	}
	if !foundExecution {
		t.Fatalf("execution-orders-db trading resource missing: %#v", items)
	}
}

// TestContractSettings 验证 settings UI 响应信封。
func TestContractSettings(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/settings/ui")
	if err != nil {
		t.Fatalf("GET /api/v1/settings/ui: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var env struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.OK {
		t.Fatal("ok != true")
	}
	if _, ok := env.Data["appearance"]; !ok {
		t.Error("data.appearance missing")
	}
}

// TestContractMarketDataMarkets 验证市场列表响应结构。
func TestContractMarketDataMarkets(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/market-data/markets")
	if err != nil {
		t.Fatalf("GET /api/v1/market-data/markets: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var env struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.OK {
		t.Fatal("ok != true")
	}

	// data 应包含 defaultMarket 和 markets 字段
	if _, ok := env.Data["defaultMarket"]; !ok {
		t.Error("data.defaultMarket missing")
	}
	markets, ok := env.Data["markets"].([]any)
	if !ok {
		t.Fatal("data.markets is not an array")
	}
	if len(markets) == 0 {
		t.Error("data.markets is empty, expected at least one market profile")
	}
}

// TestContractBrokerRuntime 验证 broker runtime 响应（即使未集成也应有结构）。
func TestContractBrokerRuntime(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/brokers/futu/runtime")
	if err != nil {
		t.Fatalf("GET /api/v1/brokers/futu/runtime: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	// 即使未集成也应返回有效响应
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var env struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.OK {
		t.Fatal("ok != true")
	}

	// broker runtime 响应应包含 descriptor、session、accounts
	requiredFields := []string{"descriptor", "session", "accounts"}
	for _, field := range requiredFields {
		if _, ok := env.Data[field]; !ok {
			t.Errorf("data.%s missing", field)
		}
	}
}

// TestContractStrategyDefinitions 验证策略定义列表响应。
// 注意：实际 API 返回 data 为直接数组，而非 {definitions: [...]}。
func TestContractStrategyDefinitions(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/strategy-definitions")
	if err != nil {
		t.Fatalf("GET /api/v1/strategy-definitions: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var env struct {
		OK   bool `json:"ok"`
		Data any  `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.OK {
		t.Fatal("ok != true")
	}

	// data 是直接数组（策略定义列表）
	if _, ok := env.Data.([]any); !ok {
		t.Errorf("data is not an array, got %T", env.Data)
	}
}

// TestContractBacktests 验证回测列表响应。
func TestContractBacktests(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/backtests")
	if err != nil {
		t.Fatalf("GET /api/v1/backtests: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	var env struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.OK {
		t.Fatal("ok != true")
	}

	// data 应包含 runs 数组
	runs, ok := env.Data["runs"].([]any)
	if !ok {
		t.Fatalf("data.runs is not an array, got %T", env.Data["runs"])
	}
	// 空回测列表 runs 可为空数组
	_ = runs
}
