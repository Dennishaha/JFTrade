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
	"testing"

	"github.com/gin-gonic/gin"
	strategyapi "github.com/jftrade/jftrade-main/internal/api/strategy"
	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineengine"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

const stage9StrategyPineFixtureVersion = "stage9.strategy-pine.v1"

type stage9StrategyPineCase struct {
	Name           string            `json:"name"`
	Method         string            `json:"method"`
	Body           string            `json:"body,omitempty"`
	ExpectedStatus int               `json:"expectedStatus"`
	Headers        map[string]string `json:"headers,omitempty"`
	Data           json.RawMessage   `json:"data,omitempty"`
	ErrorCode      string            `json:"errorCode,omitempty"`
	ErrorMessage   string            `json:"errorMessage,omitempty"`
	WorkerMode     string            `json:"workerMode,omitempty"`
}

type stage9StrategyPineFixture struct {
	Version string                   `json:"version"`
	Cases   []stage9StrategyPineCase `json:"cases"`
}

// TestStage9StrategyPineFixtureMatchesCurrentGoOwner freezes the complete
// strategy-pine request/response boundary. Worker modes use local deterministic
// scripts and only exercise the existing externalEngine shadow projection.
func TestStage9StrategyPineFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve strategy-pine fixture source")
	}
	fixturePath := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/strategy-pine.json",
	)
	cases := stage9StrategyPineCases()
	want := stage9StrategyPineFixture{
		Version: stage9StrategyPineFixtureVersion,
		Cases:   make([]stage9StrategyPineCase, 0, len(cases)),
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			configureStage9StrategyPineWorker(t, testCase.workerMode)
			response := stage9StrategyPineRequest(t, testCase.method, testCase.body)
			entry := stage9StrategyPineCase{
				Name:           testCase.name,
				Method:         testCase.method,
				Body:           testCase.body,
				ExpectedStatus: response.Status,
				Headers:        response.Headers,
				WorkerMode:     testCase.workerMode,
			}
			if response.Envelope.Error != nil {
				entry.ErrorCode = response.Envelope.Error.Code
				entry.ErrorMessage = response.Envelope.Error.Message
			} else {
				entry.Data = response.Envelope.Data
			}
			want.Cases = append(want.Cases, entry)
		})
	}

	if os.Getenv("JFTRADE_UPDATE_RUST_MIGRATION_FIXTURES") == "1" {
		contents, err := json.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatalf("encode strategy-pine fixture: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(contents, '\n'), 0o644); err != nil {
			t.Fatalf("write strategy-pine fixture: %v", err)
		}
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read strategy-pine fixture: %v", err)
	}
	var got stage9StrategyPineFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode strategy-pine fixture: %v", err)
	}
	for index := range got.Cases {
		got.Cases[index].Data = compactJSON(got.Cases[index].Data)
		want.Cases[index].Data = compactJSON(want.Cases[index].Data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("strategy-pine fixture drifted from current Go owner")
	}
}

type stage9StrategyPineInput struct {
	name       string
	method     string
	body       string
	workerMode string
}

func stage9StrategyPineCases() []stage9StrategyPineInput {
	return []stage9StrategyPineInput{
		{
			name:   "success-default-source-format",
			method: http.MethodPost,
			body:   `{"script":"//@version=6\nstrategy(\"fixture\", overlay=true)\nfast = ta.sma(close, 3)\nplot(fast)"}`,
		},
		{
			name:   "success-include-ast-and-semantic",
			method: http.MethodPost,
			body:   `{"sourceFormat":" PINE-V6 ","includeAst":true,"script":"//@version=6\nstrategy(\"fixture ast\", overlay=true, pyramiding=2)\nentry = close > close[1]\nif entry\n    strategy.entry(\"Long\", strategy.long)"}`,
		},
		{
			name:   "empty-script-projection",
			method: http.MethodPost,
			body:   `{"script":""}`,
		},
		{
			name:   "null-body-is-zero-value-input",
			method: http.MethodPost,
			body:   "null",
		},
		{
			name:   "unsupported-syntax-projection",
			method: http.MethodPost,
			body:   `{"script":"//@version=6\nstrategy(\"fixture\")\nimport TradingView/ta/7"}`,
		},
		{
			name:   "malformed-json",
			method: http.MethodPost,
			body:   "{",
		},
		{
			name:   "wrong-script-type",
			method: http.MethodPost,
			body:   `{"script":123}`,
		},
		{
			name:   "unsupported-source-format",
			method: http.MethodPost,
			body:   `{"sourceFormat":"legacy","script":"//@version=6\nstrategy(\"fixture\")"}`,
		},
		{
			name:       "worker-unavailable-projection",
			method:     http.MethodPost,
			body:       `{"script":"//@version=6\nindicator(\"worker unavailable\")\nplot(close)"}`,
			workerMode: "unavailable",
		},
		{
			name:       "worker-timeout-projection",
			method:     http.MethodPost,
			body:       `{"script":"//@version=6\nindicator(\"worker timeout\")\nplot(close)"}`,
			workerMode: "timeout",
		},
		{
			name:       "worker-cancel-projection",
			method:     http.MethodPost,
			body:       `{"script":"//@version=6\nindicator(\"worker cancel\")\nplot(close)"}`,
			workerMode: "cancel",
		},
		{
			name:       "worker-crash-projection",
			method:     http.MethodPost,
			body:       `{"script":"//@version=6\nindicator(\"worker crash\")\nplot(close)"}`,
			workerMode: "crash",
		},
	}
}

type stage9StrategyPineResponse struct {
	Status   int
	Headers  map[string]string
	Envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"envelope"`
}

func stage9StrategyPineRequest(t *testing.T, method string, body string) stage9StrategyPineResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	analyzer := func(input stratsrv.PineAnalyzeInput) (stratsrv.PineAnalysisResult, error) {
		analysis := strategypine.AnalyzeScript(input.Script, strategypine.AnalysisOptions{IncludeAST: input.IncludeAST})
		response := map[string]any{
			"ok":               analysis.OK,
			"sourceFormat":     strategypinespec.SourceFormat,
			"runtime":          strategypinespec.Runtime,
			"normalizedScript": analysis.NormalizedScript,
			"diagnostics":      analysis.Diagnostics,
			"warnings":         analysis.Warnings,
			"externalEngine":   pineengine.PayloadMap(pineengine.ShadowPayloadForScript(input.Script)),
			"metadata":         assistantassembly.StrategyMetadataPayload(analysis.Program),
			"hooks":            assistantassembly.BuildCompiledHookKinds(analysis.Program),
			"requirements":     assistantassembly.BuildCompiledRequirementsPayload(analysis.Requirements),
			"features":         analysis.Features,
		}
		if len(analysis.Visuals) > 0 {
			response["visuals"] = analysis.Visuals
		}
		if len(analysis.Declarations) > 0 {
			response["declarations"] = analysis.Declarations
		}
		if len(analysis.CollectionOperations) > 0 {
			response["collectionOperations"] = analysis.CollectionOperations
		}
		if len(analysis.ObjectOperations) > 0 {
			response["objectOperations"] = analysis.ObjectOperations
		}
		if input.IncludeAST {
			response["ast"] = analysis.AST
			response["semantic"] = analysis.Semantic
		}
		return response, nil
	}
	service := stratsrv.NewService(nil, nil, nil, stratsrv.WithPineAnalyzer(analyzer))
	router := gin.New()
	strategyapi.RegisterRoutes(router.Group("/api/v1"), service)
	request := httptest.NewRequestWithContext(t.Context(), method, "/api/v1/strategy-pine/analyze", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s response: %v (%s)", method, err, recorder.Body.String())
	}
	return stage9StrategyPineResponse{
		Status: recorder.Code,
		Headers: map[string]string{
			"Content-Type": recorder.Header().Get("Content-Type"),
		},
		Envelope: struct {
			Data  json.RawMessage `json:"data"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}{Data: envelope.Data, Error: envelope.Error},
	}
}

func configureStage9StrategyPineWorker(t *testing.T, mode string) {
	t.Helper()
	t.Setenv("JFTRADE_PINETS_MODE", pineengine.ModeOff)
	t.Setenv("JFTRADE_PINETS_WORKER_PATH", "")
	if mode == "" {
		return
	}
	t.Setenv("JFTRADE_PINETS_MODE", pineengine.ModeShadow)
	switch mode {
	case "unavailable":
		t.Setenv("JFTRADE_PINETS_WORKER_PATH", "/definitely-not-a-real-jftrade-worker.mjs")
	case "crash":
		t.Setenv("JFTRADE_PINETS_WORKER_PATH", writeStage9WorkerScript(t, "process.exit(17);"))
	case "timeout":
		t.Setenv("JFTRADE_PINETS_WORKER_PATH", writeStage9WorkerScript(t, `process.stdin.on("data", (line) => {
  const request = JSON.parse(line);
  process.stdout.write(JSON.stringify({id: request.id, ok: false, error: {
    code: "TimeoutError", message: "pinets worker timed out after 1ms"
  }}) + "\n");
});`))
	case "cancel":
		t.Setenv("JFTRADE_PINETS_WORKER_PATH", writeStage9WorkerScript(t, `process.stdin.on("data", (line) => {
  const request = JSON.parse(line);
  process.stdout.write(JSON.stringify({id: request.id, ok: false, error: {
    code: "AbortError", message: "request canceled"
  }}) + "\n");
});`))
	default:
		t.Fatalf("unknown strategy-pine worker fixture mode %q", mode)
	}
}

func writeStage9WorkerScript(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture-worker.mjs")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write fixture worker: %v", err)
	}
	return path
}

func compactJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return value
	}
	return compact.Bytes()
}
