package adk

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHTTPFetchToolAdditionalNetworkBranches(t *testing.T) {
	oldTransport := http.DefaultTransport
	t.Cleanup(func() {
		http.DefaultTransport = oldTransport
	})

	t.Run("redirect loops hit the explicit redirect cap", func(t *testing.T) {
		http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"http://8.8.8.8/loop"}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})
		if _, err := httpFetchTool(context.Background(), map[string]any{"url": "http://8.8.8.8/loop"}); err == nil || !strings.Contains(err.Error(), "too many redirects") {
			t.Fatalf("httpFetchTool redirect loop err = %v, want redirect cap", err)
		}
	})

	t.Run("response body read errors are surfaced", func(t *testing.T) {
		wantErr := errors.New("fetch read failed")
		http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(streamErrorReader{err: wantErr}),
				Request:    req,
			}, nil
		})
		if _, err := httpFetchTool(context.Background(), map[string]any{"url": "http://8.8.8.8/read-error"}); !errors.Is(err, wantErr) {
			t.Fatalf("httpFetchTool read err = %v, want %v", err, wantErr)
		}
	})
}
