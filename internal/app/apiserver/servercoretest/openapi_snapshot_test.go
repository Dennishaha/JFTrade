package servercoretest

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
)

const openapiBaselinePath = "../../../../tests/fixtures/openapi-baseline.json"

// TestOpenAPISpecStable 在存在本地基线快照时校验 OpenAPI 规范是否稳定。
// 当有意修改 API 时，运行 UPDATE_OPENAPI_SNAPSHOT=1 go test -run TestOpenAPISpecStable 更新本地快照。
func TestOpenAPISpecStable(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/swagger/doc.json")
	if err != nil {
		t.Fatalf("GET /swagger/doc.json: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/swagger/doc.json status = %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /swagger/doc.json body: %v", err)
	}

	// 格式化 JSON 以便可读对比
	var spec any
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("parse /swagger/doc.json: %v", err)
	}
	formatted, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		t.Fatalf("format /swagger/doc.json: %v", err)
	}
	formatted = append(formatted, '\n')

	baselinePath := filepath.Clean(os.Getenv("JFTRADE_OPENAPI_BASELINE"))
	if baselinePath == "." || baselinePath == "" {
		baselinePath = filepath.Clean(openapiBaselinePath)
	}

	// UPDATE_OPENAPI_SNAPSHOT=1：写入快照
	if update := os.Getenv("UPDATE_OPENAPI_SNAPSHOT"); update == "1" || update == "true" {
		if err := os.MkdirAll(filepath.Dir(baselinePath), 0o755); err != nil {
			t.Fatalf("create fixture dir: %v", err)
		}
		if err := os.WriteFile(baselinePath, formatted, 0o644); err != nil {
			t.Fatalf("write baseline: %v", err)
		}
		t.Logf("OpenAPI baseline written to %s", baselinePath)
		return
	}

	// 对比快照
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("OpenAPI baseline not found at %s — run UPDATE_OPENAPI_SNAPSHOT=1 go test -run TestOpenAPISpecStable to create", baselinePath)
		}
		t.Fatalf("read baseline: %v", err)
	}

	if string(formatted) != string(baseline) {
		t.Errorf("OpenAPI spec changed from baseline at %s", baselinePath)
		t.Errorf("Run UPDATE_OPENAPI_SNAPSHOT=1 go test -run TestOpenAPISpecStable to update the baseline after intentional API changes.")
		// 输出 diff（简单行数对比）
		formattedLines := len(strings.Split(string(formatted), "\n"))
		baselineLines := len(strings.Split(string(baseline), "\n"))
		t.Logf("Current spec: %d lines, Baseline: %d lines", formattedLines, baselineLines)
	}
}

func TestOpenAPIDocumentsExplicitErrorResponses(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/swagger/doc.json")
	if err != nil {
		t.Fatalf("GET /swagger/doc.json: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	var spec openAPIContractSpec
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		t.Fatalf("parse /swagger/doc.json: %v", err)
	}

	const errorEnvelopeRef = "#/definitions/httpserver.ErrorEnvelope"
	var operationCount int
	missing := make([]string, 0)
	for path, methods := range spec.Paths {
		for method, operation := range methods {
			operationCount++
			hasError := false
			for status, response := range operation.Responses {
				if len(status) != 3 || (status[0] != '4' && status[0] != '5') {
					continue
				}
				hasError = true
				if response.Schema.Ref != errorEnvelopeRef {
					t.Fatalf("%s %s error response %s schema = %q, want %q", strings.ToUpper(method), path, status, response.Schema.Ref, errorEnvelopeRef)
				}
			}
			if !hasError {
				missing = append(missing, strings.ToUpper(method)+" "+path)
			}
		}
	}
	if operationCount != 274 {
		t.Fatalf("OpenAPI operation count = %d, want 274", operationCount)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("OpenAPI operations missing explicit error responses:\n%s", strings.Join(missing, "\n"))
	}
}

func TestOpenAPIDocumentsWritableRequestBodies(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/swagger/doc.json")
	if err != nil {
		t.Fatalf("GET /swagger/doc.json: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /swagger/doc.json body: %v", err)
	}
	var spec openAPIContractSpec
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatalf("parse /swagger/doc.json: %v", err)
	}

	cases := []struct {
		path       string
		method     string
		refSuffix  string
		properties []string
		forbidden  []string
	}{
		{path: "/api/v1/strategy-definitions", method: "post", refSuffix: "strategy.StrategyDesignDefinition", properties: []string{"script", "visualModel"}},
		{path: "/api/v1/strategy-definitions/{definitionId}", method: "put", refSuffix: "strategy.StrategyDesignDefinition", properties: []string{"script", "visualModel"}},
		{path: "/api/v1/market-data/subscriptions", method: "post", refSuffix: "marketdata.SubscriptionRequest", properties: []string{"consumerId", "instruments"}, forbidden: []string{"channel", "market", "symbol", "interval"}},
		{path: "/api/v1/market-data/subscriptions/release", method: "post", refSuffix: "marketdata.SubscriptionRequest", properties: []string{"consumerId", "instruments"}, forbidden: []string{"channel", "market", "symbol", "interval"}},
		{path: "/api/v1/market-data/subscriptions/heartbeat", method: "post", refSuffix: "marketdata.SubscriptionHeartbeatRequest", properties: []string{"consumerId"}},
		{path: "/api/v1/settings/adk", method: "put", refSuffix: "settings.ADKRuntimeSettingsWriteRequest", properties: []string{"runTimeoutMs", "streamIdleTimeoutMs"}},
		{path: "/api/v1/settings/execution", method: "put", refSuffix: "settings.ExecutionSettingsWriteRequest", properties: []string{"defaultTradingEnvironment", "brokerOrderHistoryLookbackDays", "seenFillRetentionDays"}},
		{path: "/api/v1/settings/pine-worker", method: "put", refSuffix: "settings.PineWorkerSettingsWriteRequest", properties: []string{"backtestWorkerLimit", "instanceWorkerLimit", "nodeBinaryPath"}},
		{path: "/api/v1/settings/system-notifications", method: "put", refSuffix: "settings.SystemNotificationSettingsWriteRequest", properties: []string{"enabled", "mode", "levels", "categories", "soundEnabled"}},
		{path: "/api/v1/settings/security", method: "put", refSuffix: "jftsettings.SecuritySettingsUpdate", properties: []string{"newPassword", "publicAccessEnabled", "webAccessEnabled", "webPort"}, forbidden: []string{"passwordConfigured", "passwordHash"}},
		{path: "/api/v1/settings/brokers/{brokerId}/integration", method: "put", refSuffix: "settings.BrokerIntegrationSaveRequest", properties: []string{"enabled", "config"}, forbidden: []string{"brokerId", "createdAt", "updatedAt"}},
		{path: "/api/v1/settings/broker-accounts", method: "post", refSuffix: "settings.ManagedBrokerAccountWriteRequest", properties: []string{"brokerId", "accountId", "enabled"}, forbidden: []string{"id", "createdAt", "updatedAt"}},
		{path: "/api/v1/settings/broker-accounts/{accountRecordId}", method: "put", refSuffix: "settings.ManagedBrokerAccountWriteRequest", properties: []string{"brokerId", "accountId", "enabled"}, forbidden: []string{"id", "createdAt", "updatedAt"}},
		{path: "/api/v1/brokers/{brokerId}/orders", method: "post", refSuffix: "trading.PlaceOrderRequest", properties: []string{"symbol", "side", "orderType", "quantity"}, forbidden: []string{"brokerId", "tradingEnvironment", "accountId", "market"}},
		{path: "/api/v1/brokers/{brokerId}/orders", method: "delete", refSuffix: "trading.CancelOrdersRequest", properties: []string{"orders"}},
		{path: "/api/v1/brokers/{brokerId}/unlock", method: "post", refSuffix: "trading.UnlockTradeRequest", properties: []string{"unlock"}},
	}
	for _, tc := range cases {
		ref := openAPIRequestBodyRef(t, spec, tc.path, tc.method)
		if !strings.HasSuffix(ref, tc.refSuffix) {
			t.Fatalf("%s %s body ref = %q, want suffix %q", strings.ToUpper(tc.method), tc.path, ref, tc.refSuffix)
		}
		defName := strings.TrimPrefix(ref, "#/definitions/")
		definition, ok := spec.Definitions[defName]
		if !ok {
			t.Fatalf("definition %q not found for %s %s", defName, strings.ToUpper(tc.method), tc.path)
		}
		for _, property := range tc.properties {
			if _, ok := definition.Properties[property]; !ok {
				t.Fatalf("definition %q missing property %q", defName, property)
			}
		}
		for _, property := range tc.forbidden {
			if _, ok := definition.Properties[property]; ok {
				t.Fatalf("definition %q should not expose server-managed property %q", defName, property)
			}
		}
	}
}

func TestOpenAPIDocumentsTypedBrokerRuntimeResponse(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/swagger/doc.json")
	if err != nil {
		t.Fatalf("GET /swagger/doc.json: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	var spec openAPIContractSpec
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		t.Fatalf("parse /swagger/doc.json: %v", err)
	}

	operation := spec.Paths["/api/v1/brokers/{brokerId}/runtime"]["get"]
	success, ok := operation.Responses["200"]
	if !ok {
		t.Fatal("broker runtime OpenAPI response 200 is missing")
	}
	dataRef := ""
	for _, schema := range success.Schema.AllOf {
		if data, found := schema.Properties["data"]; found {
			dataRef = data.Ref
			break
		}
	}
	if !strings.HasSuffix(dataRef, "trading.BrokerRuntimeResponse") {
		t.Fatalf("broker runtime response data ref = %q", dataRef)
	}
	definition := spec.Definitions[strings.TrimPrefix(dataRef, "#/definitions/")]
	for _, property := range []string{"descriptor", "session", "accounts"} {
		if _, ok := definition.Properties[property]; !ok {
			t.Fatalf("broker runtime response definition missing property %q", property)
		}
	}
}

type openAPIContractSpec struct {
	Paths       map[string]map[string]openAPIOperation `json:"paths"`
	Definitions map[string]openAPIContractDefinition   `json:"definitions"`
}

type openAPIOperation struct {
	Parameters []openAPIParameter         `json:"parameters"`
	Responses  map[string]openAPIResponse `json:"responses"`
}

type openAPIResponse struct {
	Schema openAPIResponseSchema `json:"schema"`
}

type openAPIResponseSchema struct {
	Ref        string                           `json:"$ref"`
	AllOf      []openAPIResponseSchema          `json:"allOf"`
	Properties map[string]openAPIResponseSchema `json:"properties"`
}

type openAPIParameter struct {
	Name   string             `json:"name"`
	In     string             `json:"in"`
	Schema openAPIParamSchema `json:"schema"`
}

type openAPIParamSchema struct {
	Ref string `json:"$ref"`
}

type openAPIContractDefinition struct {
	Properties map[string]json.RawMessage `json:"properties"`
}

func openAPIRequestBodyRef(t *testing.T, spec openAPIContractSpec, path string, method string) string {
	t.Helper()
	methods, ok := spec.Paths[path]
	if !ok {
		t.Fatalf("path %q missing from OpenAPI", path)
	}
	operation, ok := methods[strings.ToLower(method)]
	if !ok {
		t.Fatalf("method %s missing from OpenAPI path %q", method, path)
	}
	for _, parameter := range operation.Parameters {
		if parameter.In == "body" && parameter.Name == "request" {
			if parameter.Schema.Ref == "" {
				t.Fatalf("%s %s body parameter is missing schema ref", strings.ToUpper(method), path)
			}
			return parameter.Schema.Ref
		}
	}
	t.Fatalf("%s %s missing request body parameter", strings.ToUpper(method), path)
	return ""
}
