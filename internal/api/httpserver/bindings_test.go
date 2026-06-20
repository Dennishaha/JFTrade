package httpserver

import (
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
