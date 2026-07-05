package servercore

import (
	"io"
	"net/http"
	"testing"
)

func jftradeTestHTTPGet(t testing.TB, url string) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func jftradeTestHTTPPost(t testing.TB, url string, contentType string, body io.Reader) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}
