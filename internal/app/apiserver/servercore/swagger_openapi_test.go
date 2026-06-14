package servercore

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestSwaggerUIAvailable(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

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
	if !strings.Contains(html, "swagger-ui-bundle.js") {
		t.Fatalf("swagger UI page missing bundle bootstrap: %s", html)
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

	initResp, err := http.Get(srv.URL + "/swagger/swagger-initializer.js")
	if err != nil {
		t.Fatalf("GET swagger initializer asset: %v", err)
	}
	defer initResp.Body.Close()
	initBody, err := io.ReadAll(initResp.Body)
	if err != nil {
		t.Fatalf("read swagger initializer asset: %v", err)
	}
	if !strings.Contains(string(initBody), "doc.json") {
		t.Fatalf("swagger initializer missing doc.json url: %s", string(initBody))
	}
}

func TestOpenAPISpecExposesCorePaths(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := http.Get(srv.URL + "/swagger/doc.json")
	if err != nil {
		t.Fatalf("GET swagger doc json: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("swagger doc status = %d", resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("swagger doc content-type = %q", contentType)
	}

	var spec struct {
		Swagger string `json:"swagger"`
		Info    struct {
			Title string `json:"title"`
		} `json:"info"`
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		t.Fatalf("decode swagger doc json: %v", err)
	}

	if spec.Swagger != "2.0" {
		t.Fatalf("swagger version = %q", spec.Swagger)
	}
	if spec.Info.Title != "JFTrade Debug API" {
		t.Fatalf("swagger title = %q", spec.Info.Title)
	}
	if _, ok := spec.Paths["/api/v1/system/status"]; !ok {
		t.Fatalf("missing /api/v1/system/status in spec")
	}
	if _, ok := spec.Paths["/api/v1/market-data/candles/{market}/{symbol}"]; !ok {
		t.Fatalf("missing candle endpoint in spec")
	}
	if _, ok := spec.Paths["/api/v1/ws/live"]; !ok {
		t.Fatalf("missing live websocket endpoint in spec")
	}
}
