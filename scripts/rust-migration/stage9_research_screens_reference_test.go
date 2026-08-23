package rustmigration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	productfeatures "github.com/jftrade/jftrade-main/internal/api/productfeatures"
	marketdatasrv "github.com/jftrade/jftrade-main/internal/marketdata"
	service "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const (
	stage9ResearchScreensFixtureVersion = "stage9.research-screens.v1"
	stage9ResearchScreensTimestamp      = "2026-08-23T12:00:00Z"
)

type stage9ResearchScreensFixture struct {
	Version   string                             `json:"version"`
	Timestamp string                             `json:"timestamp"`
	Cases     []stage9ResearchScreensFixtureCase `json:"cases"`
}

type stage9ResearchScreensFixtureCase struct {
	Name                string                           `json:"name"`
	Requests            []stage9ResearchScreensRequest   `json:"requests"`
	Expected            []stage9ResearchScreensExpected  `json:"expected"`
	ExpectedObservation stage9ResearchScreensObservation `json:"expectedObservation"`
	Concurrent          bool                             `json:"concurrent,omitempty"`
}

type stage9ResearchScreensRequest struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"`
}

type stage9ResearchScreensExpected struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	Envelope json.RawMessage   `json:"envelope"`
	PortCall bool              `json:"portCall"`
}

type stage9ResearchScreensObservation struct {
	CallCount int                         `json:"callCount"`
	Calls     []stage9ResearchScreensCall `json:"calls"`
}

type stage9ResearchScreensCall struct {
	BrokerID  string `json:"brokerId"`
	Market    string `json:"market"`
	Cursor    string `json:"cursor"`
	PageSize  int    `json:"pageSize"`
	Operation string `json:"operation"`
	PageFrom  int    `json:"pageFrom"`
}

type stage9ResearchScreensCaseSpec struct {
	Name       string
	Mode       string
	Requests   []stage9ResearchScreensRequest
	Concurrent bool
}

// TestStage9ResearchScreensFixtureMatchesCurrentGoOwner freezes the POST
// stock-screen contract at the HTTP boundary. The fixture broker records the
// typed service call but never opens OpenD, SQLite, a sidecar, or a network.
func TestStage9ResearchScreensFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 research screens fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/research-screens.json")
	gin.SetMode(gin.TestMode)
	want := stage9ResearchScreensFixture{
		Version:   stage9ResearchScreensFixtureVersion,
		Timestamp: stage9ResearchScreensTimestamp,
		Cases:     make([]stage9ResearchScreensFixtureCase, 0),
	}
	for _, spec := range stage9ResearchScreensCaseSpecs() {
		want.Cases = append(want.Cases, runStage9ResearchScreensCase(t, spec))
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode research screens fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write research screens fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read research screens fixture: %v", err)
	}
	var got stage9ResearchScreensFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode research screens fixture: %v", err)
	}
	for index := range got.Cases {
		for responseIndex := range got.Cases[index].Expected {
			got.Cases[index].Expected[responseIndex].Envelope = compactStage9ResearchScreensJSON(
				got.Cases[index].Expected[responseIndex].Envelope,
			)
			want.Cases[index].Expected[responseIndex].Envelope = compactStage9ResearchScreensJSON(
				want.Cases[index].Expected[responseIndex].Envelope,
			)
		}
	}
	if !reflect.DeepEqual(got, want) {
		for index := range want.Cases {
			if index >= len(got.Cases) || !reflect.DeepEqual(got.Cases[index], want.Cases[index]) {
				t.Fatalf("stage 9 research screens case %s drifted: got=%#v want=%#v", want.Cases[index].Name, got.Cases[index], want.Cases[index])
			}
		}
		t.Fatalf("stage 9 research screens fixture drifted from the Go owner")
	}
}

func compactStage9ResearchScreensJSON(data json.RawMessage) json.RawMessage {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	contents, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return contents
}

func stage9ResearchScreensCaseSpecs() []stage9ResearchScreensCaseSpec {
	valid := stage9ResearchScreensValidBody(`"offset":50,"limit":25`)
	return []stage9ResearchScreensCaseSpec{
		{
			Name: "valid-page-result", Mode: "result",
			Requests: []stage9ResearchScreensRequest{{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid}},
		},
		{
			Name: "default-page-and-empty-result", Mode: "empty",
			Requests: []stage9ResearchScreensRequest{{
				Method: http.MethodPost, Path: "/api/v1/research/screens",
				Body: stage9ResearchScreensValidBody(""),
			}},
		},
		{
			Name: "null-json-body", Mode: "result",
			Requests: []stage9ResearchScreensRequest{{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: "null"}},
		},
		{
			Name: "empty-json-body", Mode: "result",
			Requests: []stage9ResearchScreensRequest{{Method: http.MethodPost, Path: "/api/v1/research/screens"}},
		},
		{
			Name: "trailing-json-value", Mode: "result",
			Requests: []stage9ResearchScreensRequest{{
				Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid + " {}",
			}},
		},
		{
			Name: "unknown-json-field", Mode: "result",
			Requests: []stage9ResearchScreensRequest{{
				Method: http.MethodPost, Path: "/api/v1/research/screens",
				Body: strings.TrimSuffix(valid, "}") + `,"unknown":true}`,
			}},
		},
		{
			Name: "wrong-json-field-type", Mode: "result",
			Requests: []stage9ResearchScreensRequest{{
				Method: http.MethodPost, Path: "/api/v1/research/screens",
				Body: `{"brokerId":7,"market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}`,
			}},
		},
		{
			Name: "unsupported-query-schema", Mode: "result",
			Requests: []stage9ResearchScreensRequest{{
				Method: http.MethodPost, Path: "/api/v1/research/screens",
				Body: strings.Replace(valid, `"querySchemaVersion":2`, `"querySchemaVersion":1`, 1),
			}},
		},
		{
			Name: "negative-page-offset", Mode: "result",
			Requests: []stage9ResearchScreensRequest{{
				Method: http.MethodPost, Path: "/api/v1/research/screens",
				Body: stage9ResearchScreensValidBody(`"offset":-1,"limit":25`),
			}},
		},
		{
			Name: "page-limit-overflow", Mode: "result",
			Requests: []stage9ResearchScreensRequest{{
				Method: http.MethodPost, Path: "/api/v1/research/screens",
				Body: stage9ResearchScreensValidBody(`"offset":0,"limit":101`),
			}},
		},
		{
			Name: "unknown-factor-definition", Mode: "result",
			Requests: []stage9ResearchScreensRequest{{
				Method: http.MethodPost, Path: "/api/v1/research/screens",
				Body: strings.Replace(valid, "simple.price", "simple.unknown", 1),
			}},
		},
		{
			Name: "query-string-does-not-override-body", Mode: "result",
			Requests: []stage9ResearchScreensRequest{{
				Method: http.MethodPost, Path: "/api/v1/research/screens?brokerId=other&market=HK", Body: valid,
			}},
		},
		{
			Name: "rate-limit-error", Mode: "rate-limit",
			Requests: []stage9ResearchScreensRequest{{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid}},
		},
		{
			Name: "capability-error", Mode: "capability",
			Requests: []stage9ResearchScreensRequest{{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid}},
		},
		{
			Name: "provider-warming-error", Mode: "warming",
			Requests: []stage9ResearchScreensRequest{{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid}},
		},
		{
			Name: "provider-busy-error", Mode: "busy",
			Requests: []stage9ResearchScreensRequest{{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid}},
		},
		{
			Name: "broker-failure-error", Mode: "failed",
			Requests: []stage9ResearchScreensRequest{{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid}},
		},
		{
			Name: "invalid-result-row", Mode: "invalid-row",
			Requests: []stage9ResearchScreensRequest{{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid}},
		},
		{
			Name: "invalid-result-cursor", Mode: "invalid-cursor",
			Requests: []stage9ResearchScreensRequest{{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid}},
		},
		{
			Name: "failure-then-recovery", Mode: "fail-then-result",
			Requests: []stage9ResearchScreensRequest{
				{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid},
				{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid},
			},
		},
		{
			Name: "repeated-identical-request", Mode: "result",
			Requests: []stage9ResearchScreensRequest{
				{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid},
				{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: valid},
			},
		},
		{
			Name: "concurrent-distinct-pages", Mode: "result", Concurrent: true,
			Requests: []stage9ResearchScreensRequest{
				{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: stage9ResearchScreensValidBody(`"offset":0,"limit":25`)},
				{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: stage9ResearchScreensValidBody(`"offset":25,"limit":25`)},
				{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: stage9ResearchScreensValidBody(`"offset":50,"limit":25`)},
				{Method: http.MethodPost, Path: "/api/v1/research/screens", Body: stage9ResearchScreensValidBody(`"offset":75,"limit":25`)},
			},
		},
	}
}

func stage9ResearchScreensValidBody(page string) string {
	pageJSON := ""
	if page != "" {
		pageJSON = `,"page":{` + page + "}"
	}
	return `{"brokerId":" API-TEST ","market":"us","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"conditions":[{"id":" price-filter ","factor":{"instanceId":"price-filter","factorKey":" SIMPLE.PRICE "},"operator":" BETWEEN ","value":{"min":10}}],"columns":[{"columnId":"code-column","factor":{"instanceId":"code-column","factorKey":"basic.code"}},{"columnId":"price-column","factor":{"instanceId":"price-column","factorKey":"simple.price"},"label":"最新价"}],"sorts":[{"sortId":"cap-sort","factor":{"instanceId":"cap-sort","factorKey":"simple.market_cap"},"direction":" DESC "}]` + pageJSON + `}`
}

func runStage9ResearchScreensCase(
	t *testing.T,
	spec stage9ResearchScreensCaseSpec,
) stage9ResearchScreensFixtureCase {
	t.Helper()
	adapter := &stage9ResearchScreensBroker{mode: spec.Mode}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	svc := service.NewService(registry, adapter.ID(), nil, nil)
	router := gin.New()
	productfeatures.RegisterRoutes(router.Group("/api/v1"), svc)
	responses := make([]stage9ResearchScreensExpected, len(spec.Requests))
	if spec.Concurrent {
		var waitGroup sync.WaitGroup
		for index := range spec.Requests {
			waitGroup.Add(1)
			go func(index int) {
				defer waitGroup.Done()
				responses[index] = serveStage9ResearchScreensRequest(t, router, spec.Requests[index])
			}(index)
		}
		waitGroup.Wait()
	} else {
		for index, request := range spec.Requests {
			responses[index] = serveStage9ResearchScreensRequest(t, router, request)
		}
	}
	observation := adapter.observation()
	return stage9ResearchScreensFixtureCase{
		Name:                spec.Name,
		Requests:            spec.Requests,
		Expected:            responses,
		ExpectedObservation: observation,
		Concurrent:          spec.Concurrent,
	}
}

func serveStage9ResearchScreensRequest(
	t *testing.T,
	router http.Handler,
	request stage9ResearchScreensRequest,
) stage9ResearchScreensExpected {
	t.Helper()
	var body *bytes.Reader
	if request.Body == "" {
		body = bytes.NewReader(nil)
	} else {
		body = bytes.NewReader([]byte(request.Body))
	}
	httpRequest := httptest.NewRequestWithContext(
		t.Context(), request.Method, request.Path, body,
	)
	if request.Body != "" {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httpRequest)
	var envelope map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s response: %v; body=%s", request.Path, err, recorder.Body.String())
	}
	normalizeStage9ResearchScreensTimes(envelope)
	envelope["timestamp"] = stage9ResearchScreensTimestamp
	contents, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode %s response: %v", request.Path, err)
	}
	headers := make(map[string]string)
	for key, values := range recorder.Header() {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return stage9ResearchScreensExpected{
		Status:   recorder.Code,
		Headers:  headers,
		Envelope: contents,
		PortCall: stage9ResearchScreensServiceDispatchable(request),
	}
}

func stage9ResearchScreensServiceDispatchable(request stage9ResearchScreensRequest) bool {
	if request.Method != http.MethodPost {
		return false
	}
	path := request.Path
	if index := strings.IndexByte(path, '?'); index >= 0 {
		path = path[:index]
	}
	if path != "/api/v1/research/screens" || request.Body == "" {
		return false
	}
	return !strings.Contains(request.Body, `"unknown"`) &&
		!strings.Contains(request.Body, `"querySchemaVersion":1`) &&
		!strings.Contains(request.Body, `"offset":-1`) &&
		!strings.Contains(request.Body, `"limit":101`) &&
		!strings.Contains(request.Body, "simple.unknown") &&
		request.Body != "null" && !strings.Contains(request.Body, " {}") &&
		!strings.Contains(request.Body, `"brokerId":7`)
}

func normalizeStage9ResearchScreensTimes(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "timestamp" || key == "asOf" || key == "resolvedAt" {
				if _, ok := child.(string); ok {
					typed[key] = stage9ResearchScreensTimestamp
					continue
				}
			}
			normalizeStage9ResearchScreensTimes(child)
		}
	case []any:
		for _, child := range typed {
			normalizeStage9ResearchScreensTimes(child)
		}
	}
}

type stage9ResearchScreensBroker struct {
	mu    sync.Mutex
	mode  string
	calls []stage9ResearchScreensCall
}

func (b *stage9ResearchScreensBroker) ID() string { return "api-test" }

func (b *stage9ResearchScreensBroker) Descriptor() broker.Descriptor {
	features := make([]broker.FeatureCapability, 0, len(broker.BuiltinCapabilityCatalog.Features))
	for _, definition := range broker.BuiltinCapabilityCatalog.Features {
		features = append(features, broker.FeatureCapability{
			ID: definition.ID, Markets: []string{"HK", "US", "SH", "SZ"},
			Access: definition.Access, State: broker.CapabilityAvailable,
		})
	}
	return broker.Descriptor{
		ID: "api-test", SecurityFirm: "Fixture",
		Capabilities: []broker.MarketCapability{{Market: "US", Features: features}},
	}
}

func (*stage9ResearchScreensBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}

func (*stage9ResearchScreensBroker) Trading() broker.TradingService      { return nil }
func (*stage9ResearchScreensBroker) MarketData() broker.MarketDataReader { return nil }

func (b *stage9ResearchScreensBroker) QueryMarketResearch(
	_ context.Context,
	query broker.FeatureQuery,
) (*broker.FeatureResult, error) {
	b.mu.Lock()
	callNumber := len(b.calls) + 1
	b.calls = append(b.calls, stage9ResearchScreensCall{
		BrokerID:  query.BrokerID,
		Market:    query.Market,
		Cursor:    query.Cursor,
		PageSize:  query.PageSize,
		Operation: fmt.Sprint(query.Params["operation"]),
		PageFrom:  intParam(query.Params["pageFrom"]),
	})
	mode := b.mode
	b.mu.Unlock()
	switch mode {
	case "rate-limit":
		return nil, broker.NewResearchScreenRateLimitError(2500 * time.Millisecond)
	case "capability":
		return nil, fmt.Errorf("%w: fixture broker unavailable", service.ErrCapabilityUnavailable)
	case "warming":
		return nil, marketdatasrv.ErrProviderWarming
	case "busy":
		return nil, marketdatasrv.ErrProviderBusy
	case "failed":
		return nil, errors.New("fixture broker failed")
	case "fail-then-result":
		if callNumber == 1 {
			return nil, errors.New("fixture broker failed before recovery")
		}
	case "invalid-row":
		return stage9ResearchScreensInvalidRowResult(), nil
	case "invalid-cursor":
		return stage9ResearchScreensResult(intParam(query.Params["pageFrom"]), true, "not-an-offset"), nil
	case "empty":
		return stage9ResearchScreensEmptyResult(), nil
	}
	return stage9ResearchScreensResult(intParam(query.Params["pageFrom"]), true, ""), nil
}

func intParam(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func (b *stage9ResearchScreensBroker) observation() stage9ResearchScreensObservation {
	b.mu.Lock()
	defer b.mu.Unlock()
	calls := make([]stage9ResearchScreensCall, len(b.calls))
	copy(calls, b.calls)
	sort.Slice(calls, func(left, right int) bool {
		if calls[left].Cursor != calls[right].Cursor {
			return calls[left].Cursor < calls[right].Cursor
		}
		return calls[left].Operation < calls[right].Operation
	})
	return stage9ResearchScreensObservation{CallCount: len(calls), Calls: calls}
}

func stage9ResearchScreensResult(offset int, hasMore bool, nextCursor string) *broker.FeatureResult {
	if nextCursor == "" {
		nextCursor = strconv.Itoa(offset + 1)
	}
	more := hasMore
	total := 7
	return &broker.FeatureResult{
		AsOf: time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		Entries: []map[string]any{{
			"stockId":       "AAPL",
			"instrumentId":  "US.AAPL",
			"market":        "US",
			"symbol":        "AAPL",
			"name":          "Apple",
			"quoteCurrency": "USD",
			"productClass":  broker.ProductClassEquity,
			"cells": map[string]any{
				"code-column": map[string]any{
					"columnId": "code-column", "instanceId": "code-column", "factorKey": "basic.code",
					"value": map[string]any{"type": "string", "string": "AAPL"},
				},
				"price-column": map[string]any{
					"columnId": "price-column", "instanceId": "price-column", "factorKey": "simple.price",
					"value": map[string]any{"type": "number", "number": 189.25, "unit": "currency"},
				},
			},
		}},
		NextCursor: nextCursor,
		HasMore:    &more,
		Total:      &total,
	}
}

func stage9ResearchScreensEmptyResult() *broker.FeatureResult {
	more := false
	return &broker.FeatureResult{AsOf: time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC), HasMore: &more}
}

func stage9ResearchScreensInvalidRowResult() *broker.FeatureResult {
	more := false
	return &broker.FeatureResult{
		AsOf:    time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC),
		HasMore: &more,
		Entries: []map[string]any{{"stockId": "AAPL", "cells": "invalid"}},
	}
}
