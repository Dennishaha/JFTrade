package rustmigration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	strategyapi "github.com/jftrade/jftrade-main/internal/api/strategy"
	strategystore "github.com/jftrade/jftrade-main/internal/store/strategy"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

const stage9StrategyDefinitionsVersion = "stage9.strategy-definitions.v1"

type stage9StrategyDefinitionCase struct {
	Name           string          `json:"name"`
	Method         string          `json:"method"`
	Path           string          `json:"path"`
	ExpectedStatus int             `json:"expectedStatus"`
	Data           json.RawMessage `json:"data,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

type stage9StrategyDefinitionsFixture struct {
	Version string                         `json:"version"`
	Cases   []stage9StrategyDefinitionCase `json:"cases"`
}

// TestStage9StrategyDefinitionsFixtureMatchesCurrentGoOwner freezes all four
// read-only strategy-definition projections in one group fixture. Timestamps
// are deliberately normalized because the current Go SQLite owner assigns
// them at save time; field shape, ordering, versions and preview values remain
// exact.
func TestStage9StrategyDefinitionsFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve strategy definition fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/strategy-definitions.json",
	)
	resource, err := strategystore.New(filepath.Join(t.TempDir(), "strategy-definitions.json"))
	if err != nil {
		t.Fatalf("open strategy definition store: %v", err)
	}
	t.Cleanup(func() {
		if err := resource.Close(); err != nil {
			t.Errorf("close strategy definition store: %v", err)
		}
	})
	seedStage9StrategyDefinitions(t, resource)

	cases := []struct {
		name string
		path string
	}{
		{name: "list-current-only", path: "/api/v1/strategy-definitions"},
		{name: "detail-current-default-preview", path: "/api/v1/strategy-definitions/fixture-current"},
		{name: "detail-current-query-preview", path: "/api/v1/strategy-definitions/fixture-current?interval=1m&symbol=US.MSFT&useExtendedHours=true"},
		{name: "detail-missing", path: "/api/v1/strategy-definitions/missing-definition"},
		{name: "versions-current", path: "/api/v1/strategy-definitions/fixture-current/versions"},
		{name: "versions-soft-deleted", path: "/api/v1/strategy-definitions/fixture-deleted/versions"},
		{name: "versions-missing", path: "/api/v1/strategy-definitions/missing-definition/versions"},
		{name: "version-current-history", path: "/api/v1/strategy-definitions/fixture-current/versions/0.1.0"},
		{name: "version-missing", path: "/api/v1/strategy-definitions/fixture-current/versions/9.9.9"},
	}
	want := stage9StrategyDefinitionsFixture{
		Version: stage9StrategyDefinitionsVersion,
		Cases:   make([]stage9StrategyDefinitionCase, 0, len(cases)),
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := strategyDefinitionRequest(t, resource, http.MethodGet, testCase.path)
			entry := stage9StrategyDefinitionCase{
				Name:           testCase.name,
				Method:         http.MethodGet,
				Path:           testCase.path,
				ExpectedStatus: response.Status,
			}
			if response.Envelope.Error != nil {
				entry.ErrorCode = response.Envelope.Error.Code
				entry.ErrorMessage = response.Envelope.Error.Message
			} else {
				entry.Data = normalizeStrategyDefinitionTimestamps(response.Envelope.Data)
			}
			want.Cases = append(want.Cases, entry)
		})
	}
	// Subtests run sequentially above, but sort by the declared order if the
	// test runner is ever changed to parallel execution.
	want.Cases = append([]stage9StrategyDefinitionCase(nil), want.Cases...)

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode strategy definition fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write strategy definition fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read strategy definition fixture: %v", err)
	}
	var got stage9StrategyDefinitionsFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode strategy definition fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactStrategyDefinitionJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactStrategyDefinitionJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("strategy definition fixture drifted from current Go owner")
	}
}

type strategyDefinitionResponse struct {
	Status   int
	Envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"envelope"`
}

func strategyDefinitionRequest(
	t *testing.T,
	resource strategystore.Resource,
	method string,
	path string,
) strategyDefinitionResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := stratsrv.NewService(resource, nil, nil)
	router := gin.New()
	strategyapi.RegisterRoutes(router.Group("/api/v1"), service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	router.ServeHTTP(recorder, request)
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s %s response: %v (%s)", method, path, err, recorder.Body.String())
	}
	return strategyDefinitionResponse{Status: recorder.Code, Envelope: struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Data: envelope.Data, Error: envelope.Error}}
}

func seedStage9StrategyDefinitions(t *testing.T, resource strategystore.Resource) {
	t.Helper()
	current := stratsrv.Definition{
		ID:           "fixture-current",
		Name:         "Fixture Current",
		Description:  "first saved description",
		Runtime:      stratsrv.RuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Symbol:       "US.AAPL",
		Interval:     "5m",
		Script:       "//@version=6\nstrategy(\"Fixture Current\", overlay=true)\nslow = ta.sma(close, 20)",
	}
	created, err := resource.SaveDefinition(current)
	if err != nil {
		t.Fatalf("save current strategy definition: %v", err)
	}
	created.Description = "second saved description"
	if _, err := resource.SaveDefinition(created); err != nil {
		t.Fatalf("save current strategy definition version: %v", err)
	}
	archived := current
	archived.ID = "fixture-deleted"
	archived.Name = "Fixture Deleted"
	archived.Script = "//@version=6\nstrategy(\"Fixture Deleted\")"
	deleted, err := resource.SaveDefinition(archived)
	if err != nil {
		t.Fatalf("save deleted strategy definition: %v", err)
	}
	if _, err := resource.DeleteDefinition(deleted.ID); err != nil {
		t.Fatalf("soft-delete strategy definition: %v", err)
	}
}

func normalizeStrategyDefinitionTimestamps(contents json.RawMessage) json.RawMessage {
	var value any
	if err := json.Unmarshal(contents, &value); err != nil {
		return contents
	}
	normalizeStrategyDefinitionTimestampValue(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return contents
	}
	return compactStrategyDefinitionJSON(encoded)
}

func compactStrategyDefinitionJSON(contents json.RawMessage) json.RawMessage {
	if len(contents) == 0 {
		return contents
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, contents); err != nil {
		return contents
	}
	return json.RawMessage(compacted.String())
}

func normalizeStrategyDefinitionTimestampValue(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if strings.HasSuffix(key, "At") && key != "derivedWarmupAt" {
				if _, ok := child.(string); ok {
					value[key] = "<timestamp>"
					continue
				}
			}
			normalizeStrategyDefinitionTimestampValue(child)
		}
	case []any:
		for _, child := range value {
			normalizeStrategyDefinitionTimestampValue(child)
		}
	}
}
