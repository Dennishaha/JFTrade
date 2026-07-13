package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestMainWindowOptionsUseWebZoom(t *testing.T) {
	options := mainWindowOptions(currentDesktopBuildProfile())

	if options.Zoom != desktopWebviewZoom {
		t.Fatalf("window zoom = %v, want %v", options.Zoom, desktopWebviewZoom)
	}
	if options.Zoom != 1.0 {
		t.Fatalf("window zoom = %v, want browser 100%%", options.Zoom)
	}
	if options.CSS != "" {
		t.Fatalf("window CSS = %q, want no desktop scale override", options.CSS)
	}
	if options.UseApplicationMenu {
		t.Fatal("main window should hide the native application menu")
	}
}

func TestDesktopSingleInstanceOptionsAreChannelScoped(t *testing.T) {
	profile := desktopBuildProfile{SingleInstanceID: "com.jftrade.desktop.dev"}
	secondLaunches := 0
	options := desktopSingleInstanceOptions(profile, func() { secondLaunches++ })
	if options.UniqueID != profile.SingleInstanceID {
		t.Fatalf("single instance ID = %q", options.UniqueID)
	}
	options.OnSecondInstanceLaunch(application.SecondInstanceData{})
	if secondLaunches != 1 {
		t.Fatalf("second launch callbacks = %d, want 1", secondLaunches)
	}
}

func TestDesktopBuildChannelsCanCoexist(t *testing.T) {
	development := developmentDesktopBuildProfile()
	release := releaseDesktopBuildProfile()
	if development.ApplicationName == release.ApplicationName || development.ProductIdentifier == release.ProductIdentifier {
		t.Fatalf("application identities overlap: dev=%#v release=%#v", development, release)
	}
	if development.SingleInstanceID == release.SingleInstanceID || development.DefaultAPIBind == release.DefaultAPIBind {
		t.Fatalf("runtime identities overlap: dev=%#v release=%#v", development, release)
	}
	if development.UpdateChecksEnabled || !release.UpdateChecksEnabled {
		t.Fatalf("update policies = dev:%v release:%v", development.UpdateChecksEnabled, release.UpdateChecksEnabled)
	}
}

func TestDesktopRuntimeConfigDisablesAuth(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)

	writeDesktopRuntimeConfig(recorder, request, "http://127.0.0.1:6699", "desktop-token")

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
	if !strings.Contains(body, `"desktopApiToken":"desktop-token"`) {
		t.Fatalf("runtime config did not include desktop API token: %q", body)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestDesktopAssetHandlerOverridesRuntimeConfig(t *testing.T) {
	nextCalled := false
	handler := newDesktopAssetHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}), fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<main>JFTrade</main>")}}, "http://127.0.0.1:6699", "")

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
	}, "http://127.0.0.1:6699", "")

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
	}, "http://127.0.0.1:6699", "")

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
	profile := currentDesktopBuildProfile()
	menu := newDesktopTrayMenu(nil, nil, nil, profile, nil)

	for _, label := range []string{"打开 " + profile.ApplicationName, "查看日志", "设置", "文档", "退出"} {
		if menu.FindByLabel(label) == nil {
			t.Fatalf("tray menu missing %q", label)
		}
	}
	if menu.FindByLabel("通知设置") != nil {
		t.Fatal("tray menu should use 设置 instead of 通知设置")
	}
	if (menu.FindByLabel("检查更新…") != nil) != profile.UpdateChecksEnabled {
		t.Fatalf("update menu availability does not match profile: %#v", profile)
	}
}

func TestDesktopLogWindowOptionsUseVueRoute(t *testing.T) {
	options := desktopLogWindowOptions("JFTrade Dev")
	if options.URL != desktopLogsURL || options.HTML != "" {
		t.Fatalf("log window options = %#v, want Vue route without inline HTML", options)
	}
	if options.Title != "JFTrade Dev 日志" || options.Zoom != desktopWebviewZoom {
		t.Fatalf("log window identity = %#v", options)
	}
}

func TestDesktopAssetHandlerServesLogViewer(t *testing.T) {
	handler := newDesktopAssetHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}), fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<main>JFTrade</main>")}}, "http://127.0.0.1:6699", "")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, desktopLogsURL, nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "<main>JFTrade</main>") {
		t.Fatalf("body = %q, want SPA index", recorder.Body.String())
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

func TestDesktopLogManagerWritesOriginalAndRotatesByDay(t *testing.T) {
	tempDir := t.TempDir()
	settingsPath := filepath.Join(tempDir, "runtime", "settings.json")
	var original bytes.Buffer
	now := time.Date(2026, 7, 9, 23, 59, 0, 0, time.Local)
	manager, err := newDesktopLogManager(settingsPath, &original, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newDesktopLogManager: %v", err)
	}
	t.Cleanup(func() { _ = manager.close() })

	if _, err := manager.Write([]byte("INFO first line\n")); err != nil {
		t.Fatalf("write day 1: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := manager.Write([]byte("ERROR second line\n")); err != nil {
		t.Fatalf("write day 2: %v", err)
	}
	if !strings.Contains(original.String(), "INFO first line\nERROR second line\n") {
		t.Fatalf("original output = %q", original.String())
	}

	firstPath := filepath.Join(tempDir, "runtime", "logs", "desktop-2026-07-09.log")
	secondPath := filepath.Join(tempDir, "runtime", "logs", "desktop-2026-07-10.log")
	firstData, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read first log: %v", err)
	}
	secondData, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("read second log: %v", err)
	}
	if string(firstData) != "INFO first line\n" {
		t.Fatalf("first log = %q", firstData)
	}
	if string(secondData) != "ERROR second line\n" {
		t.Fatalf("second log = %q", secondData)
	}
}

func TestDesktopLogLevelParsing(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{line: `time=2026-07-09T01:02:03Z level=ERROR msg="failed"`, want: "ERROR"},
		{line: `{"level":"WARN","msg":"slow"}`, want: "WARN"},
		{line: `[DEBUG] probe payload`, want: "DEBUG"},
		{line: `INFO server ready`, want: "INFO"},
		{line: `2026/07/09 12:00:00 plain backend log`, want: "INFO"},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			if got := parseDesktopLogLevel(tt.line); got != tt.want {
				t.Fatalf("parseDesktopLogLevel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListDesktopLogDaysAndReadsFilteredPage(t *testing.T) {
	logDir := t.TempDir()
	for _, item := range []struct {
		name string
		body string
	}{
		{name: "desktop-2026-07-08.log", body: "INFO older\n"},
		{name: "desktop-2026-07-10.log", body: "WARN latest\nERROR final\n"},
		{name: "ignore.log", body: "INFO ignored\n"},
	} {
		if err := os.WriteFile(filepath.Join(logDir, item.name), []byte(item.body), 0o644); err != nil {
			t.Fatalf("write %s: %v", item.name, err)
		}
	}

	days, err := listDesktopLogDays(logDir)
	if err != nil {
		t.Fatalf("listDesktopLogDays: %v", err)
	}
	if len(days) != 2 || days[0].Day != "2026-07-10" || days[1].Day != "2026-07-08" {
		t.Fatalf("days = %+v", days)
	}
	page, err := readDesktopLogPage(filepath.Join(logDir, "desktop-2026-07-10.log"), logDir, "2026-07-10", "ERROR", "final", 0, 200)
	if err != nil {
		t.Fatalf("readDesktopLogPage: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Level != "ERROR" || page.Items[0].Text != "ERROR final" {
		t.Fatalf("page = %+v", page)
	}
}

func TestDesktopLogPageCapsLimitAndPaginatesAllLines(t *testing.T) {
	tempDir := t.TempDir()
	settingsPath := filepath.Join(tempDir, "runtime", "settings.json")
	logDir := filepath.Join(tempDir, "runtime", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir logDir: %v", err)
	}

	var body strings.Builder
	for i := 1; i <= 2005; i++ {
		_, _ = fmt.Fprintf(&body, "INFO line-%04d\n", i)
	}
	if err := os.WriteFile(filepath.Join(logDir, "desktop-2026-07-10.log"), []byte(body.String()), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	manager, err := newDesktopLogManager(settingsPath, nil, func() time.Time { return time.Date(2026, 7, 10, 12, 0, 0, 0, time.Local) })
	if err != nil {
		t.Fatalf("newDesktopLogManager: %v", err)
	}

	service := newDesktopLogService(manager)
	first, err := service.ReadPage("2026-07-10", "ALL", "", 0, 1000)
	if err != nil {
		t.Fatalf("ReadPage first: %v", err)
	}
	if first.Total != 2005 || first.Limit != desktopLogPageMaximum || len(first.Items) != desktopLogPageMaximum || first.NextOffset == nil || *first.NextOffset != 500 {
		t.Fatalf("first page = %+v", first)
	}
	last, err := service.ReadPage("2026-07-10", "ALL", "", 2000, 500)
	if err != nil {
		t.Fatalf("ReadPage last: %v", err)
	}
	if len(last.Items) != 5 || last.NextOffset != nil || last.Items[4].Text != "INFO line-2005" {
		t.Fatalf("last page = %+v", last)
	}
}

func TestDesktopLogPageTailOffsetReturnsLastPageInFileOrder(t *testing.T) {
	tests := []struct {
		name       string
		total      int
		wantOffset int
		wantCount  int
		wantFirst  string
		wantLast   string
	}{
		{name: "partial last page", total: 2005, wantOffset: 2000, wantCount: 5, wantFirst: "INFO line-2001", wantLast: "INFO line-2005"},
		{name: "full last page", total: 2000, wantOffset: 1500, wantCount: 500, wantFirst: "INFO line-1501", wantLast: "INFO line-2000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logDir := t.TempDir()
			path := filepath.Join(logDir, "desktop-2026-07-10.log")
			var body strings.Builder
			for index := 1; index <= tt.total; index++ {
				_, _ = fmt.Fprintf(&body, "INFO line-%04d\n", index)
			}
			if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
				t.Fatalf("write log: %v", err)
			}

			page, err := readDesktopLogPage(path, logDir, "2026-07-10", "ALL", "", desktopLogPageLatest, 500)
			if err != nil {
				t.Fatalf("readDesktopLogPage tail: %v", err)
			}
			if page.Offset != tt.wantOffset || page.Total != tt.total || len(page.Items) != tt.wantCount {
				t.Fatalf("tail page offset/total/count = %d/%d/%d, want %d/%d/%d", page.Offset, page.Total, len(page.Items), tt.wantOffset, tt.total, tt.wantCount)
			}
			if page.NextOffset != nil {
				t.Fatalf("tail page nextOffset = %d, want nil", *page.NextOffset)
			}
			if page.Items[0].Text != tt.wantFirst || page.Items[len(page.Items)-1].Text != tt.wantLast {
				t.Fatalf("tail page range = %q..%q, want %q..%q", page.Items[0].Text, page.Items[len(page.Items)-1].Text, tt.wantFirst, tt.wantLast)
			}
		})
	}
}

func TestDesktopLogPageTailOffsetAppliesFiltersBeforePaging(t *testing.T) {
	logDir := t.TempDir()
	path := filepath.Join(logDir, "desktop-2026-07-10.log")
	body := "INFO ignored\nWARN match-old\nERROR ignored\nWARN match-latest\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	page, err := readDesktopLogPage(path, logDir, "2026-07-10", "WARN", "match", desktopLogPageLatest, 1)
	if err != nil {
		t.Fatalf("readDesktopLogPage filtered tail: %v", err)
	}
	if page.Offset != 1 || page.Total != 2 || len(page.Items) != 1 || page.Items[0].Text != "WARN match-latest" {
		t.Fatalf("filtered tail page = %+v", page)
	}
}

func TestListDesktopLogDaysMissingDirReturnsEmpty(t *testing.T) {
	days, err := listDesktopLogDays(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("listDesktopLogDays missing dir: %v", err)
	}
	if len(days) != 0 {
		t.Fatalf("days = %+v, want empty", days)
	}
}

func TestDesktopOpenFolderCommand(t *testing.T) {
	tests := []struct {
		goos string
		name string
		args []string
	}{
		{goos: "darwin", name: "open", args: []string{"/tmp/logs"}},
		{goos: "windows", name: "explorer", args: []string{"/tmp/logs"}},
		{goos: "linux", name: "xdg-open", args: []string{"/tmp/logs"}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			name, args, err := desktopOpenFolderCommand(tt.goos, "/tmp/logs")
			if err != nil {
				t.Fatalf("desktopOpenFolderCommand: %v", err)
			}
			if name != tt.name || strings.Join(args, "\x00") != strings.Join(tt.args, "\x00") {
				t.Fatalf("command = %s %#v, want %s %#v", name, args, tt.name, tt.args)
			}
		})
	}
}
