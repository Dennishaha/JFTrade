package webaccess

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testFrontendFS(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("JFTrade UI"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("console.log('jftrade')"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestFrontendServesAssetsAndSPAFallback(t *testing.T) {
	frontend := NewFrontendServer(os.DirFS(testFrontendFS(t)))
	for _, tc := range []struct{ path, accept, want string }{
		{"/", "text/html", "JFTrade UI"},
		{"/strategy", "text/html", "JFTrade UI"},
		{"/assets/app.js", "", "console.log"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set("Accept", tc.accept)
		recorder := httptest.NewRecorder()
		frontend.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), tc.want) {
			t.Fatalf("GET %s = %d %q", tc.path, recorder.Code, recorder.Body.String())
		}
	}
	missing := httptest.NewRecorder()
	frontend.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/missing.json", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d", missing.Code)
	}
}

func TestFrontendRuntimeConfigAndAccessSurface(t *testing.T) {
	frontend := NewFrontendServerWithRuntimeConfig(os.DirFS(t.TempDir()), "http://127.0.0.1:6699")
	frontend.SetAuthRequired(true)
	frontend.SetDesktopMode(true)
	req := WithAccessSurface(httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil))
	recorder := httptest.NewRecorder()
	frontend.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), `"desktopMode":true`) || !strings.Contains(recorder.Body.String(), `"apiBaseUrl":"http://example.com"`) {
		t.Fatalf("runtime config = %d %q", recorder.Code, recorder.Body.String())
	}
	head := httptest.NewRecorder()
	frontend.ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/runtime-config.js", nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("HEAD runtime config = %d %q", head.Code, head.Body.String())
	}
}

func TestFrontendDevelopmentProxyOnlyAllowsLoopback(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("vite:" + r.URL.Path)) }))
	t.Cleanup(target.Close)
	frontend := NewFrontendServerWithOptions(nil, "", target.URL)
	recorder := httptest.NewRecorder()
	frontend.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/app.ts", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "vite:/assets/app.ts" {
		t.Fatalf("proxy response = %d %q", recorder.Code, recorder.Body.String())
	}
	if NewFrontendServerWithOptions(nil, "", "http://192.0.2.10:3003") != nil {
		t.Fatal("non-loopback proxy should be rejected")
	}
}

func TestFrontendBoundaryHelpers(t *testing.T) {
	frontend := NewFrontendServer(os.DirFS(testFrontendFS(t)))
	if frontend.HasFile("assets") || frontend.HasFile("missing") {
		t.Fatal("HasFile accepted a directory or missing file")
	}
	if NormalizeFrontendPath("  /alpha/../beta  ") != "/beta" {
		t.Fatal("NormalizeFrontendPath did not clean path")
	}
	post := httptest.NewRecorder()
	if frontend.ServeRequest(post, httptest.NewRequest(http.MethodPost, "/", nil)) || post.Body.Len() != 0 {
		t.Fatal("ServeRequest accepted POST")
	}
	file := httptest.NewRecorder()
	frontend.ServeFile(file, httptest.NewRequest(http.MethodGet, "/missing.js", nil), "/missing.js")
	if file.Code != http.StatusNotFound {
		t.Fatal("ServeFile missing asset was not 404")
	}
	if ShouldServeFrontendIndex(httptest.NewRequest(http.MethodGet, "/", nil), "/api/v1/status") {
		t.Fatal("API path served SPA index")
	}
	if NewFrontendServer(nil) != nil || NewFrontendServerWithOptions(nil, "", "://bad") != nil {
		t.Fatal("frontend was created without assets or a valid proxy")
	}
}

func TestShouldServeFrontendIndexRequestBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		path   string
		accept string
		want   bool
	}{
		{name: "root always allowed", url: "/", path: "/", accept: "", want: true},
		{name: "spa html route", url: "/strategy/live", path: "/strategy/live", accept: "text/html", want: true},
		{name: "spa wildcard route", url: "/strategy/live", path: "/strategy/live", accept: "*/*", want: true},
		{name: "blank path rejected", url: "/", path: "", accept: "text/html", want: false},
		{name: "api route rejected", url: "/api/v1/status", path: "/api/v1/status", accept: "text/html", want: false},
		{name: "swagger route rejected", url: "/swagger/index.html", path: "/swagger/index.html", accept: "text/html", want: false},
		{name: "assets directory rejected", url: "/assets", path: "/assets", accept: "text/html", want: false},
		{name: "assets file rejected", url: "/assets/app.js", path: "/assets/app.js", accept: "text/html", want: false},
		{name: "file extension rejected", url: "/favicon.ico", path: "/favicon.ico", accept: "text/html", want: false},
		{name: "json accept rejected", url: "/strategy/live", path: "/strategy/live", accept: "application/json", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			if got := ShouldServeFrontendIndex(req, tc.path); got != tc.want {
				t.Fatalf("ShouldServeFrontendIndex(%q, %q) = %v, want %v", tc.path, tc.accept, got, tc.want)
			}
		})
	}
}

func TestFrontendRequestSchemeTrustsTLSAndLoopbackProxyOnly(t *testing.T) {
	tlsRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	tlsRequest.TLS = &tls.ConnectionState{}
	if requestScheme(tlsRequest) != "https" {
		t.Fatal("TLS request scheme was not https")
	}
	proxied := httptest.NewRequest(http.MethodGet, "/", nil)
	proxied.RemoteAddr = "127.0.0.1:443"
	proxied.Header.Set("X-Forwarded-Proto", "http, https")
	if requestScheme(proxied) != "https" {
		t.Fatal("loopback proxy scheme was not trusted")
	}
	remote := httptest.NewRequest(http.MethodGet, "/", nil)
	remote.RemoteAddr = "192.0.2.1:443"
	remote.Header.Set("X-Forwarded-Proto", "https")
	if requestScheme(remote) != "http" {
		t.Fatal("remote forwarded scheme was trusted")
	}
}
