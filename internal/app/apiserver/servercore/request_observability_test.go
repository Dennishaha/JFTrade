package servercore

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/pkg/observability"
)

func TestRequestObservabilityInjectsStableContextAndRecordsSummary(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	recorder := observability.NewRecorder(5, time.Nanosecond)
	var fields observability.Fields
	router := gin.New()
	router.Use(requestObservabilityMiddleware(recorder))
	router.GET("/failed", func(c *gin.Context) {
		fields = observability.FieldsFromContext(c.Request.Context())
		time.Sleep(time.Millisecond)
		c.Status(http.StatusServiceUnavailable)
	})

	request := httptest.NewRequest(http.MethodGet, "/failed", nil)
	request.Header.Set(requestIDHeader, "request-stable-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if got := response.Header().Get(requestIDHeader); got != "request-stable-1" {
		t.Fatalf("response request id = %q", got)
	}
	if fields.RequestID != "request-stable-1" || fields.Source != "api" {
		t.Fatalf("request context fields = %#v", fields)
	}
	snapshot := recorder.Snapshot()
	if len(snapshot.RecentErrors) != 1 || snapshot.RecentErrors[0].RequestID != "request-stable-1" || snapshot.RecentErrors[0].Importance != observability.ImportanceHigh.String() {
		t.Fatalf("recent errors = %#v", snapshot.RecentErrors)
	}
	if len(snapshot.RecentSlowRequests) != 1 || snapshot.RecentSlowRequests[0].Importance != observability.ImportanceLow.String() {
		t.Fatalf("recent slow requests = %#v", snapshot.RecentSlowRequests)
	}
}

func TestRequestObservabilityReplacesUnsafeRequestID(t *testing.T) {
	recorder := observability.NewRecorder(5, time.Second)
	router := gin.New()
	router.Use(requestObservabilityMiddleware(recorder))
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/ok", nil)
	request.Header.Set(requestIDHeader, "unsafe request id")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	got := response.Header().Get(requestIDHeader)
	if got == "" || got == "unsafe request id" || normalizeRequestID(got) != got {
		t.Fatalf("generated request id = %q", got)
	}
}
