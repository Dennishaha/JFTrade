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

const stage9MarketDataDerivativesReadFixtureVersion = "stage9.market-data-derivatives-read.v1"

type stage9MarketDataDerivativesReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9MarketDataDerivativesReadFixture struct {
	Version string                                `json:"version"`
	Cases   []stage9MarketDataDerivativesReadCase `json:"cases"`
}

// TestStage9MarketDataDerivativesReadFixtureMatchesCurrentGoOwner freezes
// warrant and future catalog GET projections without starting Provider/OpenD.
func TestStage9MarketDataDerivativesReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve derivative fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/market-data-derivatives-read.json")
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
		{"warrants-list", "/api/v1/market-data/warrants?brokerId=api-test&market=US&operation=list&pageSize=20", successRouter},
		{"warrants-related", "/api/v1/market-data/warrants?brokerId=api-test&market=US&operation=related&cursor=next", successRouter},
		{"warrants-screen", "/api/v1/market-data/warrants?brokerId=api-test&market=US&operation=screen&pageSize=50", successRouter},
		{"futures-contracts", "/api/v1/market-data/futures?brokerId=api-test&market=US&pageSize=25", successRouter},
		{"broker-capability-unavailable", "/api/v1/market-data/futures?brokerId=missing&market=US", failureRouter},
	}
	want := stage9MarketDataDerivativesReadFixture{
		Version: stage9MarketDataDerivativesReadFixtureVersion,
		Cases:   make([]stage9MarketDataDerivativesReadCase, 0, len(cases)),
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, testCase.path, nil)
		testCase.router.ServeHTTP(recorder, request)
		entry := stage9MarketDataDerivativesReadCase{
			Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path, ExpectedStatus: recorder.Code,
		}
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
			t.Fatalf("encode derivative fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write derivative fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read derivative fixture: %v", err)
	}
	var got stage9MarketDataDerivativesReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode derivative fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactResearchJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactResearchJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		for index := range want.Cases {
			if !reflect.DeepEqual(got.Cases[index], want.Cases[index]) {
				t.Fatalf("stage 9 derivative case %s drifted: got=%s want=%s", want.Cases[index].Name, got.Cases[index].Data, want.Cases[index].Data)
			}
		}
		t.Fatalf("stage 9 market-data derivatives fixture drifted from the Go owner")
	}
}
