package rustmigration

import (
	"context"
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
	service "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const stage9MarketDataOptionsReadFixtureVersion = "stage9.market-data-options-read.v1"

type stage9MarketDataOptionsReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9MarketDataOptionsReadFixture struct {
	Version string                            `json:"version"`
	Cases   []stage9MarketDataOptionsReadCase `json:"cases"`
}

// TestStage9MarketDataOptionsReadFixtureMatchesCurrentGoOwner freezes the
// provider-backed option projections without starting Provider/OpenD.
func TestStage9MarketDataOptionsReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve options fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/market-data-options-read.json")
	gin.SetMode(gin.TestMode)
	adapter := &stage9ResearchBroker{}
	registry := broker.NewRegistry()
	registry.Register(adapter)
	successRouter := gin.New()
	productfeatures.RegisterRoutes(successRouter.Group("/api/v1"), service.NewService(registry, adapter.ID(), nil, nil))
	failureRouter := gin.New()
	productfeatures.RegisterRoutes(failureRouter.Group("/api/v1"), service.NewService(broker.NewRegistry(), "", nil, nil))
	cases := []struct {
		name   string
		path   string
		router *gin.Engine
	}{
		{"chain", "/api/v1/market-data/options/chains/US.AAPL?brokerId=api-test&market=US&operation=chain", successRouter},
		{"expirations", "/api/v1/market-data/options/expirations/US.AAPL?brokerId=api-test&market=US&operation=expirations", successRouter},
		{"screen", "/api/v1/market-data/options/screens?brokerId=api-test&market=US&operation=screen", successRouter},
		{"analysis", "/api/v1/market-data/options/analysis/US.AAPL?brokerId=api-test&market=US", successRouter},
		{"events", "/api/v1/market-data/options/events?brokerId=api-test&market=US", successRouter},
		{"broker-capability-unavailable", "/api/v1/market-data/options/chains/US.AAPL?brokerId=missing&market=US", failureRouter},
	}
	want := stage9MarketDataOptionsReadFixture{Version: stage9MarketDataOptionsReadFixtureVersion, Cases: make([]stage9MarketDataOptionsReadCase, 0, len(cases))}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, testCase.path, nil)
		testCase.router.ServeHTTP(recorder, request)
		entry := stage9MarketDataOptionsReadCase{Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path, ExpectedStatus: recorder.Code}
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
			entry.Data = normalizeResearchReadData(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode options fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write options fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read options fixture: %v", err)
	}
	var got stage9MarketDataOptionsReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode options fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactResearchJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactResearchJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 market-data options fixture drifted from the Go owner: got=%#v want=%#v", got, want)
	}
}
