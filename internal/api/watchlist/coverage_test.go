package watchlist

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

func TestWriteErrorMapsAllDomainErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrUnavailable, http.StatusServiceUnavailable, "WATCHLIST_UNAVAILABLE"},
		{domain.ErrNotFound, http.StatusNotFound, "WATCHLIST_NOT_FOUND"},
		{domain.ErrValidation, http.StatusBadRequest, "WATCHLIST_INVALID"},
		{domain.ErrAmbiguousRemoteGroup, http.StatusConflict, "WATCHLIST_REMOTE_GROUP_AMBIGUOUS"},
		{domain.ErrProtectedGroup, http.StatusConflict, "WATCHLIST_GROUP_PROTECTED"},
		{domain.ErrPreviewExpired, http.StatusConflict, "WATCHLIST_PREVIEW_EXPIRED"},
		{domain.ErrStalePreview, http.StatusConflict, "WATCHLIST_PREVIEW_STALE"},
		{domain.ErrConflict, http.StatusConflict, "WATCHLIST_CONFLICT"},
		{errors.New("unexpected"), http.StatusInternalServerError, "WATCHLIST_FAILED"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		writeError(context, test.err)
		if recorder.Code != test.status {
			t.Fatalf("writeError(%v) status = %d, want %d", test.err, recorder.Code, test.status)
		}
		var envelope httpserver.Envelope
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if envelope.Error == nil || envelope.Error.Code != test.code {
			t.Fatalf("writeError(%v) envelope = %#v", test.err, envelope)
		}
	}
}

func TestRouteHelpersCoverBindingAndDeleteErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	successRecorder := httptest.NewRecorder()
	successContext, _ := gin.CreateTestContext(successRecorder)
	successContext.Params = gin.Params{{Key: "groupId", Value: "group-1"}}
	var valid groupURI
	if !bindURI(successContext, &valid, "invalid") || valid.GroupID != "group-1" {
		t.Fatalf("valid URI binding = %#v", valid)
	}

	failureRecorder := httptest.NewRecorder()
	failureContext, _ := gin.CreateTestContext(failureRecorder)
	var invalid groupURI
	if bindURI(failureContext, &invalid, "invalid") || failureRecorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid URI status = %d", failureRecorder.Code)
	}

	deleteRecorder := httptest.NewRecorder()
	deleteContext, _ := gin.CreateTestContext(deleteRecorder)
	deleteContext.Request = httptest.NewRequest(http.MethodDelete, "/bindings", nil)
	deleteBinding(deleteContext, domain.NewService(nil), "binding-1")
	if deleteRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("deleteBinding unavailable status = %d", deleteRecorder.Code)
	}
}

func TestBindQueryRejectsMalformedAndInvalidValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		target string
	}{
		{name: "malformed escape", target: "/watchlists?limit=%zz"},
		{name: "invalid integer", target: "/watchlists?limit=not-a-number"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodGet, test.target, nil)
			var query struct {
				Limit int `form:"limit"`
			}
			if bindQuery(context, &query, "invalid query") {
				t.Fatal("bindQuery accepted invalid query")
			}
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}
