package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestMainWindowOptionsUseWebZoom(t *testing.T) {
	options := mainWindowOptions()

	if options.Zoom != desktopWebviewZoom {
		t.Fatalf("window zoom = %v, want %v", options.Zoom, desktopWebviewZoom)
	}
	if options.Zoom != 1.0 {
		t.Fatalf("window zoom = %v, want browser 100%%", options.Zoom)
	}
	if options.CSS != "" {
		t.Fatalf("window CSS = %q, want no desktop scale override", options.CSS)
	}
}

func TestDesktopRuntimeConfigDisablesAuth(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)

	writeDesktopRuntimeConfig(recorder, request, "http://127.0.0.1:6699")

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", recorder.Code, body)
	}
	if !strings.Contains(body, `"authRequired":false`) {
		t.Fatalf("runtime config did not disable auth: %q", body)
	}
	if !strings.Contains(body, `"desktopMode":true`) {
		t.Fatalf("runtime config did not enable desktop mode: %q", body)
	}
	if !strings.Contains(body, `"apiBaseUrl":"http://127.0.0.1:6699"`) {
		t.Fatalf("runtime config did not include API base URL: %q", body)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestDesktopAssetHandlerOverridesRuntimeConfig(t *testing.T) {
	nextCalled := false
	handler := newDesktopAssetHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}), fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<main>JFTrade</main>")}}, "http://127.0.0.1:6699")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil))

	if nextCalled {
		t.Fatal("runtime config request should not reach underlying asset handler")
	}
	if !strings.Contains(recorder.Body.String(), `"authRequired":false`) {
		t.Fatalf("runtime config did not disable auth: %q", recorder.Body.String())
	}
}

func TestDesktopAssetHandlerServesIndexForSPARoute(t *testing.T) {
	handler := newDesktopAssetHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}), fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<main>JFTrade settings</main>")},
	}, "http://127.0.0.1:6699")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/settings/system-notifications", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "JFTrade settings") {
		t.Fatalf("body = %q, want desktop index", recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
}

func TestDesktopAssetHandlerDoesNotFallbackForMissingStaticAsset(t *testing.T) {
	handler := newDesktopAssetHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}), fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<main>JFTrade</main>")},
	}, "http://127.0.0.1:6699")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %q; missing static assets must not use SPA fallback", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "JFTrade") {
		t.Fatalf("body = %q, should not include index fallback", recorder.Body.String())
	}
}

func TestDesktopTrayMenuLabels(t *testing.T) {
	menu := newDesktopTrayMenu(nil, nil, nil)

	for _, label := range []string{"打开 JFTrade", "设置", "文档", "退出"} {
		if menu.FindByLabel(label) == nil {
			t.Fatalf("tray menu missing %q", label)
		}
	}
	if menu.FindByLabel("通知设置") != nil {
		t.Fatal("tray menu should use 设置 instead of 通知设置")
	}
}

func TestShouldUseExplicitTrayMenuClick(t *testing.T) {
	tests := []struct {
		goos string
		want bool
	}{
		{goos: "darwin", want: true},
		{goos: "windows", want: true},
		{goos: "linux", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			if got := shouldUseExplicitTrayMenuClick(tt.goos); got != tt.want {
				t.Fatalf("shouldUseExplicitTrayMenuClick(%q) = %v, want %v", tt.goos, got, tt.want)
			}
		})
	}
}

func TestNormalizeDesktopDocsURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "root docs", in: "/docs", want: "/docs/"},
		{name: "absolute docs path", in: "/docs/index.html", want: "/docs/"},
		{name: "relative docs path", in: "docs/reference/index.html", want: "/docs/reference/"},
		{name: "docs fragment", in: "/docs/reference/index.html#section", want: "/docs/reference/#section"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDesktopDocsURL(tt.in)
			if err != nil {
				t.Fatalf("normalizeDesktopDocsURL(%q) error = %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeDesktopDocsURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeDesktopDocsURLRejectsUnsafePaths(t *testing.T) {
	for _, in := range []string{"", "../docs/index.html", "/docs/../settings", "/docs/%2e%2e/settings", "/settings", "javascript:alert(1)", "file:///tmp/doc.html", "/docs/index.html\x00"} {
		t.Run(in, func(t *testing.T) {
			if got, err := normalizeDesktopDocsURL(in); err == nil {
				t.Fatalf("normalizeDesktopDocsURL(%q) = %q, want error", in, got)
			}
		})
	}
}

func TestShouldQuitDesktopAppOnlyAllowsExplicitTrayQuit(t *testing.T) {
	var exiting atomic.Bool
	window := &fakeDesktopWindowHider{}
	dockHideCalls := 0
	previousDockIconHider := desktopDockIconHider
	desktopDockIconHider = func() {
		dockHideCalls++
	}
	t.Cleanup(func() {
		desktopDockIconHider = previousDockIconHider
	})

	if shouldQuitDesktopApp(&exiting, window) {
		t.Fatal("shouldQuitDesktopApp returned true before explicit tray quit")
	}
	if exiting.Load() {
		t.Fatal("Dock/system quit should not set exiting flag")
	}
	if window.hideCalls != 1 {
		t.Fatalf("window hideCalls = %d, want 1", window.hideCalls)
	}
	if dockHideCalls != 1 {
		t.Fatalf("dockHideCalls = %d, want 1", dockHideCalls)
	}

	exiting.Store(true)
	if !shouldQuitDesktopApp(&exiting, window) {
		t.Fatal("shouldQuitDesktopApp returned false after explicit tray quit")
	}
	if window.hideCalls != 1 {
		t.Fatalf("window hideCalls after explicit quit = %d, want unchanged 1", window.hideCalls)
	}
	if dockHideCalls != 1 {
		t.Fatalf("dockHideCalls after explicit quit = %d, want unchanged 1", dockHideCalls)
	}
}

type fakeDesktopWindowHider struct {
	hideCalls int
}

func (w *fakeDesktopWindowHider) Hide() application.Window {
	w.hideCalls++
	return nil
}

func TestSanitizeDesktopExternalURL(t *testing.T) {
	got, ok, err := sanitizeDesktopExternalURL("https://nodejs.org/")
	if err != nil {
		t.Fatalf("sanitizeDesktopExternalURL returned error: %v", err)
	}
	if !ok || got != "https://nodejs.org/" {
		t.Fatalf("sanitizeDesktopExternalURL = (%q, %v), want accepted https URL", got, ok)
	}

	for _, in := range []string{"javascript:alert(1)", "file:///tmp/doc.html", "ftp://example.com/doc"} {
		t.Run(in, func(t *testing.T) {
			if got, ok, err := sanitizeDesktopExternalURL(in); !ok || err == nil {
				t.Fatalf("sanitizeDesktopExternalURL(%q) = (%q, %v, %v), want rejected scheme", in, got, ok, err)
			}
		})
	}
}
