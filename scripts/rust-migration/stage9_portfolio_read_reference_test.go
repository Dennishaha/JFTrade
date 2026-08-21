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
	tradingapi "github.com/jftrade/jftrade-main/internal/api/trading"
	trading "github.com/jftrade/jftrade-main/internal/trading"
)

const stage9PortfolioReadFixtureVersion = "stage9.portfolio-read.v1"

type stage9PortfolioReadCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	RequestPath    string          `json:"requestPath"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9PortfolioReadFixture struct {
	Version string                    `json:"version"`
	Cases   []stage9PortfolioReadCase `json:"cases"`
}

// TestStage9PortfolioReadFixtureMatchesCurrentGoOwner freezes both broker
// portfolio projections with the Go degraded/no-provider fallback. No broker,
// OpenD connection, or portfolio store is activated by this fixture.
func TestStage9PortfolioReadFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 portfolio fixture source")
	}
	fixturePath := filepath.Join(filepath.Dir(source), "../../tests/fixtures/rust-migration/stage9/portfolio-read.json")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	tradingapi.RegisterPortfolioRoutes(router.Group("/api/v1"), trading.NewService())
	cases := []struct {
		name string
		path string
	}{
		{name: "cash-balances", path: "/api/v1/portfolio/fixture/cash-balances"},
		{name: "positions", path: "/api/v1/portfolio/fixture/positions"},
	}
	want := stage9PortfolioReadFixture{Version: stage9PortfolioReadFixtureVersion, Cases: make([]stage9PortfolioReadCase, 0, len(cases))}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, testCase.path, nil)
		router.ServeHTTP(recorder, request)
		entry := stage9PortfolioReadCase{Name: testCase.name, Method: http.MethodGet, RequestPath: testCase.path, ExpectedStatus: recorder.Code}
		var envelope struct {
			Data  json.RawMessage `json:"data"`
			Error *struct { Code string `json:"code"`; Message string `json:"message"` } `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode %s response: %v", testCase.name, err)
		}
		if envelope.Error != nil {
			entry.ErrorCode, entry.ErrorMessage = envelope.Error.Code, envelope.Error.Message
		} else {
			entry.Data = normalizePortfolioReadData(envelope.Data)
		}
		want.Cases = append(want.Cases, entry)
	}
	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode portfolio fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write portfolio fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read portfolio fixture: %v", err)
	}
	var got stage9PortfolioReadFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode portfolio fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactPluginJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactPluginJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage 9 portfolio read fixture drifted from the Go owner")
	}
}

func normalizePortfolioReadData(data json.RawMessage) json.RawMessage {
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	value["checkedAt"] = "fixture-time"
	return mustJSON(value)
}

func mustJSON(value any) json.RawMessage {
	contents, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return contents
}
