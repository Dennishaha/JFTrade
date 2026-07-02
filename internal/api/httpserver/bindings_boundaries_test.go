package httpserver

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestOptionalBoolRecognizesFalseAliases(t *testing.T) {
	for _, input := range []string{"0", "false", "no", "n", "off", ""} {
		t.Run(input, func(t *testing.T) {
			value := OptionalBoolValue{Value: true}
			if err := value.UnmarshalText([]byte(input)); err != nil {
				t.Fatalf("UnmarshalText(%q): %v", input, err)
			}
			if !value.Set || value.Bool() {
				t.Fatalf("UnmarshalText(%q) = %#v", input, value)
			}
		})
	}
}

func TestCandlePeriodValueHandlesEmptyAndUnsupportedInputs(t *testing.T) {
	value := CandlePeriodValue("5m")
	if err := value.UnmarshalText([]byte(" ")); err != nil || value != "" {
		t.Fatalf("empty period = %q, %v", value, err)
	}
	if err := value.UnmarshalText([]byte("2h")); err == nil {
		t.Fatal("unsupported period error = nil")
	}
}

func TestNormalizeCandlePeriodSupportsEveryDocumentedFamily(t *testing.T) {
	tests := map[string]string{
		"ticker":  "tick",
		"1min":    "1m",
		"k_3m":    "3m",
		"5min":    "5m",
		"k_10m":   "10m",
		"15min":   "15m",
		"k_30m":   "30m",
		"k_60m":   "1h",
		"day":     "1d",
		"k_week":  "1w",
		"k_month": "1mo",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := NormalizeCandlePeriod(input)
			if err != nil || got != want {
				t.Fatalf("NormalizeCandlePeriod(%q) = %q, %v; want %q", input, got, err, want)
			}
		})
	}
}

func TestParseQueryTimeReturnsCallerFallback(t *testing.T) {
	fallback := time.Date(2026, time.July, 2, 9, 30, 0, 0, time.UTC)
	for _, input := range []string{"", "not-a-time"} {
		if got := ParseQueryTime(input, fallback); !got.Equal(fallback) {
			t.Fatalf("ParseQueryTime(%q) = %s, want %s", input, got, fallback)
		}
	}
}

func TestBindURIHandlesBindingAndFallbackEscapeValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("required parameter missing", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/items", nil)
		var target struct {
			ID string `uri:"id" binding:"required"`
		}
		if err := BindURI(context, &target); err == nil {
			t.Fatal("BindURI missing required parameter error = nil")
		}
	})

	t.Run("params fallback rejects malformed escape", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = &http.Request{URL: &url.URL{}}
		context.Params = gin.Params{{Key: "id", Value: "bad%2"}}
		var target struct {
			ID string `uri:"id" binding:"required"`
		}
		if err := BindURI(context, &target); err == nil {
			t.Fatal("BindURI malformed params escape error = nil")
		}
	})

	t.Run("params fallback accepts valid escape", func(t *testing.T) {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = &http.Request{URL: &url.URL{}}
		context.Params = gin.Params{{Key: "id", Value: "valid%20value"}}
		var target struct {
			ID string `uri:"id" binding:"required"`
		}
		if err := BindURI(context, &target); err != nil {
			t.Fatalf("BindURI valid params escape: %v", err)
		}
	})
}

func TestRequestEscapedPathFallsBackToURLRawPath(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := requestEscapedPath(nil); got != "" {
		t.Fatalf("requestEscapedPath(nil) = %q", got)
	}
	context.Request = &http.Request{URL: &url.URL{Path: "/items/value%", RawPath: "/items/value%25"}}
	if got := requestEscapedPath(context); got != "/items/value%25" {
		t.Fatalf("requestEscapedPath(RawPath) = %q", got)
	}
	context.Request.URL.RawPath = ""
	if got := requestEscapedPath(context); got != "" {
		t.Fatalf("requestEscapedPath(no escaped path) = %q", got)
	}
}
