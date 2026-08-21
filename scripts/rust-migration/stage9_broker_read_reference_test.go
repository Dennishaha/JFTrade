package rustmigration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	productfeatures "github.com/jftrade/jftrade-main/internal/api/productfeatures"
	tradingapi "github.com/jftrade/jftrade-main/internal/api/trading"
	productservice "github.com/jftrade/jftrade-main/internal/productfeatures"
	trading "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const stage9BrokerReadFixtureVersion = "stage9.broker-read.v1"

type stage9BrokerReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9BrokerReadFixture struct {
	Version string                 `json:"version"`
	Cases   []stage9BrokerReadCase `json:"cases"`
}

// TestStage9BrokerReadFixtureMatchesCurrentGoOwner freezes broker-backed GET
// projections using the Go degraded/no-provider fallback. Rust does not start
// a broker runtime or open a trading connection for this fixture.
func TestStage9BrokerReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 broker fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/broker-read.json")
	gin.SetMode(gin.TestMode)
	productSvc := productservice.NewService(broker.NewRegistry(), "", nil, nil)
	tradingSvc := trading.NewService()
	router := gin.New()
	productfeatures.RegisterRoutes(router.Group("/api/v1"), productSvc)
	tradingapi.RegisterRoutes(router.Group("/api/v1"), tradingSvc)
	cases := []struct {
		name string
		path string
	}{
		{"capabilities", "/api/v1/brokers/capabilities"},
		{"runtime", "/api/v1/brokers/fixture/runtime"},
		{"funds", "/api/v1/brokers/fixture/funds?market=US"},
		{"positions", "/api/v1/brokers/fixture/positions?market=US"},
		{"orders", "/api/v1/brokers/fixture/orders?scope=current&symbol=US.AAPL"},
		{"fills", "/api/v1/brokers/fixture/fills?scope=current&symbol=US.AAPL"},
		{"cash-flows", "/api/v1/brokers/fixture/cash-flows?clearingDate=2026-08-21"},
		{"order-fees", "/api/v1/brokers/fixture/order-fees?orderIdEx=OID-1"},
		{"margin-ratios", "/api/v1/brokers/fixture/margin-ratios?market=US&symbol=US.AAPL"},
		{"max-trade-qtys", "/api/v1/brokers/fixture/max-trade-qtys?market=US&symbol=US.AAPL&orderType=LIMIT&price=100"},
		{"quote", "/api/v1/brokers/fixture/quote?symbol=US.AAPL"},
		{"klines", "/api/v1/brokers/fixture/klines?symbol=US.AAPL&period=1d&limit=10"},
		{"securities", "/api/v1/brokers/fixture/securities?symbol=US.AAPL"},
	}
	want := stage9BrokerReadFixture{Version: stage9BrokerReadFixtureVersion, Cases: make([]stage9BrokerReadCase, 0, len(cases))}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		entry := stage9BrokerReadCase{Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path, ExpectedStatus: recorder.Code}
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v", testCase.name, err)
		}
		if envelope.Error != nil {
			entry.ErrorCode, entry.ErrorMessage = envelope.Error.Code, envelope.Error.Message
		} else {
			entry.Data = compactResearchJSON(envelope.Data)
			entry.Data = normalizeBrokerReadData(entry.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode broker fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write broker fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read broker fixture: %v", err)
	}
	var got stage9BrokerReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode broker fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactResearchJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactResearchJSON(want.Cases[index].Data)
		got.Cases[index].Data = normalizeBrokerReadData(got.Cases[index].Data)
		want.Cases[index].Data = normalizeBrokerReadData(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		for index := range want.Cases {
			if !reflect.DeepEqual(got.Cases[index], want.Cases[index]) {
				t.Fatalf("stage 9 broker case %s drifted: got=%s want=%s", want.Cases[index].Name, got.Cases[index].Data, want.Cases[index].Data)
			}
		}
		t.Fatalf("stage 9 broker read fixture drifted from the Go owner")
	}
}

func normalizeBrokerReadData(data json.RawMessage) json.RawMessage {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return data
	}
	normalizeBrokerReadTimes(value)
	return mustResearchJSON(value)
}

func normalizeBrokerReadTimes(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "checkedAt" || key == "observedAt" || key == "quoteAt" {
				if _, ok := child.(string); ok {
					typed[key] = "fixture-time"
					continue
				}
			}
			normalizeBrokerReadTimes(child)
		}
	case []any:
		for _, child := range typed {
			normalizeBrokerReadTimes(child)
		}
	}
}
