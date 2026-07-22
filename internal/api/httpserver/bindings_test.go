package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestParseQueryTimeNormalizesToUTC(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Time
	}{
		{
			name:  "timezone-less timestamp is UTC",
			value: "2026-06-20 09:30:00",
			want:  time.Date(2026, time.June, 20, 9, 30, 0, 0, time.UTC),
		},
		{
			name:  "timezone-less date is UTC",
			value: "2026-06-20",
			want:  time.Date(2026, time.June, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "explicit offset is normalized",
			value: "2026-06-20T09:30:00+08:00",
			want:  time.Date(2026, time.June, 20, 1, 30, 0, 0, time.UTC),
		},
	}

	originalLocal := time.Local
	time.Local = time.FixedZone("host-local", -7*60*60)
	t.Cleanup(func() {
		time.Local = originalLocal
	})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseQueryTime(tc.value, time.Time{})
			if !got.Equal(tc.want) {
				t.Fatalf("ParseQueryTime(%q) = %s, want %s", tc.value, got, tc.want)
			}
			if got.Location() != time.UTC {
				t.Fatalf("ParseQueryTime(%q) location = %s, want UTC", tc.value, got.Location())
			}
		})
	}
}

func TestBindURIRejectsMalformedEscapeInRequestURI(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{
		Method:     http.MethodGet,
		RequestURI: "/items/bad%ZZ",
		URL:        &url.URL{Path: "/items/bad%ZZ"},
	}
	c.Params = gin.Params{{Key: "id", Value: "bad%ZZ"}}

	var uri struct {
		ID string `uri:"id" binding:"required"`
	}

	if err := BindURI(c, &uri); err == nil {
		t.Fatal("BindURI error = nil, want malformed URL escape error")
	}
}

func TestBindURIAllowsEscapedLiteralPercent(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/items/:id", func(c *gin.Context) {
		var uri struct {
			ID string `uri:"id" binding:"required"`
		}
		if err := BindURI(c, &uri); err != nil {
			t.Fatalf("BindURI: %v", err)
		}
		if uri.ID != "value%" {
			t.Fatalf("uri.ID = %q, want %q", uri.ID, "value%")
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/items/value%25", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestOptionalQueryValueParsingSemantics(t *testing.T) {
	t.Run("optional int treats empty as valid zero and invalid text as non-fatal", func(t *testing.T) {
		var empty OptionalIntValue
		if err := empty.UnmarshalText([]byte("   ")); err != nil {
			t.Fatalf("OptionalIntValue.UnmarshalText(empty) error = %v", err)
		}
		if !empty.Set || !empty.Valid || empty.Int() != 0 {
			t.Fatalf("OptionalIntValue(empty) = %#v", empty)
		}

		var invalid OptionalIntValue
		if err := invalid.UnmarshalText([]byte("abc")); err != nil {
			t.Fatalf("OptionalIntValue.UnmarshalText(invalid) error = %v", err)
		}
		if !invalid.Set || invalid.Valid || invalid.Int() != 0 {
			t.Fatalf("OptionalIntValue(invalid) = %#v", invalid)
		}

		var valid OptionalIntValue
		if err := valid.UnmarshalText([]byte("42")); err != nil {
			t.Fatalf("OptionalIntValue.UnmarshalText(valid) error = %v", err)
		}
		if !valid.Set || !valid.Valid || valid.Int() != 42 {
			t.Fatalf("OptionalIntValue(valid) = %#v", valid)
		}
	})

	t.Run("optional bool accepts common truthy and falsy forms", func(t *testing.T) {
		var truthy OptionalBoolValue
		if err := truthy.UnmarshalText([]byte(" YeS ")); err != nil {
			t.Fatalf("OptionalBoolValue.UnmarshalText(truthy) error = %v", err)
		}
		if !truthy.Set || !truthy.Bool() {
			t.Fatalf("OptionalBoolValue(truthy) = %#v", truthy)
		}

		var falsy OptionalBoolValue
		if err := falsy.UnmarshalText([]byte("maybe")); err != nil {
			t.Fatalf("OptionalBoolValue.UnmarshalText(falsy) error = %v", err)
		}
		if !falsy.Set || falsy.Bool() {
			t.Fatalf("OptionalBoolValue(falsy) = %#v", falsy)
		}
	})

	t.Run("optional time normalizes parsed values to UTC pointers and strings", func(t *testing.T) {
		var value OptionalTimeValue
		if err := value.UnmarshalText([]byte("2026-06-22T09:30:00+08:00")); err != nil {
			t.Fatalf("OptionalTimeValue.UnmarshalText error = %v", err)
		}
		ptr := value.PtrUTC()
		if ptr == nil {
			t.Fatal("OptionalTimeValue.PtrUTC() = nil")
			return
		}
		want := time.Date(2026, time.June, 22, 1, 30, 0, 0, time.UTC)
		if !ptr.Equal(want) {
			t.Fatalf("OptionalTimeValue.PtrUTC() = %s, want %s", ptr, want)
		}
		if value.StringValue() != "2026-06-22T01:30:00Z" {
			t.Fatalf("OptionalTimeValue.StringValue() = %q", value.StringValue())
		}

		var zero OptionalTimeValue
		if zero.PtrUTC() != nil || zero.StringValue() != "" {
			t.Fatalf("zero OptionalTimeValue = ptr:%v string:%q", zero.PtrUTC(), zero.StringValue())
		}
	})
}

func TestCandlePeriodAndPaginationNormalization(t *testing.T) {
	t.Run("candle period aliases normalize to canonical form", func(t *testing.T) {
		var value CandlePeriodValue
		if err := value.UnmarshalText([]byte(" 60m ")); err != nil {
			t.Fatalf("CandlePeriodValue.UnmarshalText(alias) error = %v", err)
		}
		if value.String() != "1h" {
			t.Fatalf("CandlePeriodValue(alias) = %q", value.String())
		}

		normalized, err := NormalizeCandlePeriod("k_day")
		if err != nil || normalized != "1d" {
			t.Fatalf("NormalizeCandlePeriod(k_day) = %q, %v", normalized, err)
		}

		if _, err := NormalizeCandlePeriod("2h"); err == nil {
			t.Fatal("NormalizeCandlePeriod(2h) error = nil")
		}
	})

	t.Run("pagination bounds clamp invalid or oversized requests", func(t *testing.T) {
		limit, offset := NormalizeBoundPage(0, -5, 50, 200)
		if limit != 50 || offset != 0 {
			t.Fatalf("NormalizeBoundPage(default) = %d, %d", limit, offset)
		}

		limit, offset = NormalizeBoundPage(500, 7, 50, 200)
		if limit != 200 || offset != 7 {
			t.Fatalf("NormalizeBoundPage(max) = %d, %d", limit, offset)
		}

		limit, offset = NormalizeBoundPage(-3, 2, 50, 200)
		if limit != 1 || offset != 2 {
			t.Fatalf("NormalizeBoundPage(min) = %d, %d", limit, offset)
		}
	})
}

func TestResponseEnvelopeWriters(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)

	t.Run("WriteOK wraps payload with timestamp", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)

		WriteOK(c, gin.H{"status": "ready"})

		if recorder.Code != http.StatusOK {
			t.Fatalf("WriteOK status = %d", recorder.Code)
		}

		var envelope Envelope
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("unmarshal WriteOK body: %v", err)
		}
		if !envelope.OK || envelope.Error != nil || envelope.Timestamp == "" {
			t.Fatalf("WriteOK envelope = %#v", envelope)
		}
		if _, err := time.Parse(time.RFC3339Nano, envelope.Timestamp); err != nil {
			t.Fatalf("WriteOK timestamp parse error = %v", err)
		}
		data, ok := envelope.Data.(map[string]any)
		if !ok || data["status"] != "ready" {
			t.Fatalf("WriteOK data = %#v", envelope.Data)
		}
	})

	t.Run("WriteError and WriteNotFound abort with machine-readable envelope", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)

		WriteError(c, http.StatusBadRequest, "BAD_INPUT", "invalid symbol")

		if recorder.Code != http.StatusBadRequest || !c.IsAborted() {
			t.Fatalf("WriteError status/abort = %d/%v", recorder.Code, c.IsAborted())
		}

		var envelope Envelope
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("unmarshal WriteError body: %v", err)
		}
		if envelope.OK || envelope.Error == nil || envelope.Error.Code != "BAD_INPUT" || envelope.Error.Message != "invalid symbol" {
			t.Fatalf("WriteError envelope = %#v", envelope)
		}

		recorder = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(recorder)
		WriteNotFound(c)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("WriteNotFound status = %d", recorder.Code)
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("unmarshal WriteNotFound body: %v", err)
		}
		if envelope.Error == nil || envelope.Error.Code != "NOT_FOUND" || envelope.Error.Message != "resource not found" {
			t.Fatalf("WriteNotFound envelope = %#v", envelope)
		}
	})
}
