package jftradeapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSwaggerUIAvailable(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/swagger/")
	if err != nil {
		t.Fatalf("GET swagger UI: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("swagger status = %d", resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("swagger content-type = %q", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read swagger UI body: %v", err)
	}
	html := string(body)
	if !strings.Contains(html, "SwaggerUIBundle") {
		t.Fatalf("swagger UI page missing bundle bootstrap: %s", html)
	}
	if !strings.Contains(html, "/openapi.json") {
		t.Fatalf("swagger UI page missing openapi url: %s", html)
	}
	if strings.Contains(html, "cdn.jsdelivr.net") {
		t.Fatalf("swagger UI page still references external CDN: %s", html)
	}

	assetResp, err := http.Get(srv.URL + "/swagger/swagger-ui.css")
	if err != nil {
		t.Fatalf("GET swagger css asset: %v", err)
	}
	defer assetResp.Body.Close()
	if assetResp.StatusCode != http.StatusOK {
		t.Fatalf("swagger css status = %d", assetResp.StatusCode)
	}
	if contentType := assetResp.Header.Get("Content-Type"); !strings.Contains(contentType, "text/css") {
		t.Fatalf("swagger css content-type = %q", contentType)
	}
}

func TestOpenAPISpecExposesCorePaths(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("GET openapi spec: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("openapi status = %d", resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("openapi content-type = %q", contentType)
	}

	var spec struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Title string `json:"title"`
		} `json:"info"`
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		t.Fatalf("decode openapi spec: %v", err)
	}

	if spec.OpenAPI != "3.0.3" {
		t.Fatalf("openapi version = %q", spec.OpenAPI)
	}
	if spec.Info.Title != "JFTrade Debug API" {
		t.Fatalf("openapi title = %q", spec.Info.Title)
	}
	if _, ok := spec.Paths["/api/v1/system/status"]; !ok {
		t.Fatalf("missing /api/v1/system/status in spec")
	}
	if _, ok := spec.Paths["/api/v1/market-data/candles/{market}/{symbol}"]; !ok {
		t.Fatalf("missing candle endpoint in spec")
	}
	if _, ok := spec.Paths["/api/v1/stream/live"]; !ok {
		t.Fatalf("missing live sse endpoint in spec")
	}
}
