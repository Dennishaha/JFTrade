package strategy

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	srv "github.com/jftrade/jftrade-main/internal/strategy"
)

type routeTestDesignStore struct {
	definition srv.Definition
	found      bool
	err        error
}

func (s *routeTestDesignStore) ListDefinitions() []srv.Definition { return nil }
func (s *routeTestDesignStore) GetDefinition(string) (srv.Definition, bool, error) {
	return s.definition, s.found, s.err
}
func (s *routeTestDesignStore) SaveDefinition(input srv.Definition) (srv.Definition, error) {
	return input, nil
}
func (s *routeTestDesignStore) DeleteDefinition(string) (srv.Definition, error) {
	return srv.Definition{}, nil
}

func TestWriteStrategyErrorMapsBusinessErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name       string
		err        error
		status     int
		code       string
		message    string
		fallback   string
		statusCode int
	}{
		{
			name:       "not found",
			err:        srv.NotFoundError("definition missing"),
			status:     http.StatusNotFound,
			code:       "NOT_FOUND",
			message:    "definition missing",
			statusCode: http.StatusInternalServerError,
		},
		{
			name:       "busy falls back to supplied message when empty",
			err:        srv.BusyError(""),
			status:     http.StatusBadRequest,
			code:       "BAD_REQUEST",
			message:    "instance is busy",
			fallback:   "instance is busy",
			statusCode: http.StatusInternalServerError,
		},
		{
			name:       "upstream uses dedicated gateway code",
			err:        srv.UpstreamError("runtime start failed"),
			status:     http.StatusBadGateway,
			code:       "STRATEGY_RUNTIME_START_FAILED",
			message:    "runtime start failed",
			fallback:   "fallback",
			statusCode: http.StatusInternalServerError,
		},
		{
			name:       "plain errors keep fallback envelope",
			err:        errors.New("unexpected"),
			status:     http.StatusConflict,
			code:       "STRATEGY_CONFLICT",
			message:    "fallback conflict",
			fallback:   "fallback conflict",
			statusCode: http.StatusConflict,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)

			writeStrategyError(ctx, tc.err, tc.statusCode, tc.code, tc.fallback)

			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, tc.status, rec.Body.String())
			}
			var envelope httpserver.Envelope
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("unmarshal envelope: %v", err)
			}
			if envelope.Error == nil {
				t.Fatalf("error envelope missing: %#v", envelope)
			}
			if envelope.Error.Code != tc.code || envelope.Error.Message != tc.message {
				t.Fatalf("error = %#v, want code=%q message=%q", envelope.Error, tc.code, tc.message)
			}
		})
	}
}

func TestEnrichDefinitionResponseDefaultsAndQueryOverride(t *testing.T) {
	definition := srv.Definition{
		ID:           "def-1",
		Name:         "Breakout",
		SourceFormat: "pine_v6",
		Symbol:       "HK.00700",
		Script:       "not valid pine",
	}

	defaultView := enrichDefinitionResponse(definition, definitionPreviewQuery{})
	if defaultView.DerivedWarmupInterval != "5m" {
		t.Fatalf("default interval = %q, want 5m", defaultView.DerivedWarmupInterval)
	}
	if defaultView.Symbol != definition.Symbol {
		t.Fatalf("symbol = %q, want %q", defaultView.Symbol, definition.Symbol)
	}
	if defaultView.DerivedWarmupBars != 0 {
		t.Fatalf("warmup bars = %d, want 0 for invalid script", defaultView.DerivedWarmupBars)
	}

	overrideView := enrichDefinitionResponse(definition, definitionPreviewQuery{
		Interval: "15m",
		Symbol:   "US.AAPL",
	})
	if overrideView.DerivedWarmupInterval != "15m" {
		t.Fatalf("override interval = %q, want 15m", overrideView.DerivedWarmupInterval)
	}
	if overrideView.Symbol != definition.Symbol {
		t.Fatalf("definition symbol should be preserved in payload = %q", overrideView.Symbol)
	}
}

func TestHandleGetDefinitionReturnsNotFoundAndBadQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("not found", func(t *testing.T) {
		router := gin.New()
		service := srv.NewService(&routeTestDesignStore{found: false}, nil, nil)
		RegisterRoutes(router.Group("/api/v1"), service)

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/strategy-definitions/missing", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("bad query", func(t *testing.T) {
		router := gin.New()
		service := srv.NewService(&routeTestDesignStore{
			found:      true,
			definition: srv.Definition{ID: "def-1", Script: "strategy('x')", SourceFormat: "pine-v6"},
		}, nil, nil)
		RegisterRoutes(router.Group("/api/v1"), service)

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/strategy-definitions/def-1?useExtendedHours=not-bool", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestHandleAnalyzePineMapsValidationErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	service := srv.NewService(nil, nil, nil, srv.WithPineAnalyzer(func(srv.PineAnalyzeInput) (srv.PineAnalysisResult, error) {
		return srv.PineAnalysisResult{}, srv.BadRequestError("unsupported strategy input")
	}))
	RegisterRoutes(router.Group("/api/v1"), service)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/strategy-pine/analyze", bytes.NewBufferString(`{"script":"x","sourceFormat":"legacy"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	var envelope httpserver.Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.Error == nil || envelope.Error.Code != "BAD_REQUEST" {
		t.Fatalf("error envelope = %#v", envelope.Error)
	}
}
