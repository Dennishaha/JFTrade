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
	menu := newDesktopTrayMenu(nil, nil, nil, nil)

	for _, label := range []string{"打开 JFTrade", "查看日志", "设置", "文档", "退出"} {
		if menu.FindByLabel(label) == nil {
			t.Fatalf("tray menu missing %q", label)
		}
	}
	if menu.FindByLabel("通知设置") != nil {
		t.Fatal("tray menu should use 设置 instead of 通知设置")
	}
}

func TestDesktopLogWindowOptionsUseInlineHTML(t *testing.T) {
	options := desktopLogWindowOptions()

	if options.URL != "" {
		t.Fatalf("log window URL = %q, want inline HTML without frontend route", options.URL)
	}
	if options.HTML == "" {
		t.Fatal("log window HTML is empty")
	}
	if !options.AllowSimpleEventEmit {
		t.Fatal("log window must allow simple event emit for inline HTML controls")
	}
	if strings.Contains(options.HTML, "/wails/runtime.js") {
		t.Fatal("inline log window must not import /wails/runtime.js")
	}
	if !strings.Contains(options.HTML, "window.wails.Events") {
		t.Fatal("inline log window should use Wails inline event shim")
	}
	if !strings.Contains(options.HTML, "window._wails.invoke") {
		t.Fatal("inline log window should wait for Wails invoke before emitting startup events")
	}
	requireDesktopLogHTML(t, options.HTML, []string{
		`const state = { day: "", logDir: "", files: [], lines: [], level: "ALL", keyword: "", page: 1, pageSize: 200 }`,
		`<input id="keyword" type="search" placeholder="过滤日志内容">`,
		`<select id="pageSize">`,
		`<option value="100">100</option>`,
		`<option value="200" selected>200</option>`,
		`<option value="500">500</option>`,
		`<option value="1000">1000</option>`,
		`<button id="firstPage" type="button">首页</button>`,
		`<button id="prevPage" type="button">上一页</button>`,
		`<button id="nextPage" type="button">下一页</button>`,
		`<button id="lastPage" type="button">末页</button>`,
	})
}

func TestDesktopLogViewerHTMLPaginationAndFilteringScript(t *testing.T) {
	html := desktopLogsHTML

	requireDesktopLogHTML(t, html, []string{
		`function visibleLines()`,
		`const keyword = state.keyword.trim().toLowerCase();`,
		`if (state.level !== "ALL" && line.level !== state.level) return false;`,
		`return String(line.text || "").toLowerCase().indexOf(keyword) >= 0;`,
		`function totalPagesFor(count)`,
		`return Math.max(1, Math.ceil(count / state.pageSize));`,
		`function clampPage(count)`,
		`function goToLastPage()`,
		`state.page = totalPagesFor(visibleLines().length);`,
		`renderPager(lines.length, pages);`,
		`const start = (state.page - 1) * state.pageSize;`,
		`const pageLines = lines.slice(start, start + state.pageSize);`,
	})
	requireDesktopLogHTML(t, html, []string{
		`levelSelect.addEventListener("change", function () {`,
		`keywordInput.addEventListener("input", function () {`,
		`pageSizeSelect.addEventListener("change", function () {`,
		`state.page = 1;`,
		`firstPage.addEventListener("click", function () { state.page = 1; renderLogs(); });`,
		`prevPage.addEventListener("click", function () { state.page -= 1; renderLogs(); });`,
		`nextPage.addEventListener("click", function () { state.page += 1; renderLogs(); });`,
		`lastPage.addEventListener("click", function () { state.page = totalPagesFor(visibleLines().length); renderLogs(); });`,
	})
	requireDesktopLogHTML(t, html, []string{
		`goToLastPage();`,
		`const wasLastPage = state.page >= totalPagesFor(visibleLines().length);`,
		`if (wasLastPage) goToLastPage();`,
	})
	if strings.Contains(html, "maxRows") || strings.Contains(html, "splice(0, state.lines.length") {
		t.Fatal("desktop log viewer must not truncate loaded or appended log lines")
	}
}

func requireDesktopLogHTML(t *testing.T, html string, values []string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(html, value) {
			t.Fatalf("desktop log viewer HTML missing %q", value)
		}
	}
}

func TestDesktopAssetHandlerServesLogViewer(t *testing.T) {
	handler := newDesktopAssetHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}), fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<main>JFTrade</main>")}}, "http://127.0.0.1:6699")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, desktopLogsURL, nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, value := range []string{desktopLogEventReady, desktopLogEventSnapshot, desktopLogEventAppend, desktopLogEventSelectDay, `eventSelectDay + ":" + day`, "关键词", "每页", "上一页", "下一页", "打开日志文件夹"} {
		if !strings.Contains(body, value) {
			t.Fatalf("desktop log viewer missing %q", value)
		}
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

func TestListDesktopLogFilesAndReadsAllLines(t *testing.T) {
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

	files, err := listDesktopLogFiles(logDir)
	if err != nil {
		t.Fatalf("listDesktopLogFiles: %v", err)
	}
	if len(files) != 2 || files[0].Day != "2026-07-10" || files[1].Day != "2026-07-08" {
		t.Fatalf("files = %+v", files)
	}
	lines, err := readDesktopLogLines(filepath.Join(logDir, "desktop-2026-07-10.log"))
	if err != nil {
		t.Fatalf("readDesktopLogLines: %v", err)
	}
	if len(lines) != 2 || lines[0].Level != "WARN" || lines[0].Text != "WARN latest" || lines[1].Level != "ERROR" || lines[1].Text != "ERROR final" {
		t.Fatalf("lines = %+v", lines)
	}
}

func TestDesktopLogSnapshotIncludesAllLinesBeyondPreviousTailLimit(t *testing.T) {
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

	manager, err := newDesktopLogManager(settingsPath, nil, func() time.Time {
		return time.Date(2026, 7, 10, 12, 0, 0, 0, time.Local)
	})
	if err != nil {
		t.Fatalf("newDesktopLogManager: %v", err)
	}

	snapshot := manager.snapshot("2026-07-10")
	if snapshot.Error != "" {
		t.Fatalf("snapshot error = %q", snapshot.Error)
	}
	if len(snapshot.Lines) != 2005 {
		t.Fatalf("snapshot lines = %d, want all 2005", len(snapshot.Lines))
	}
	if snapshot.Lines[0].Text != "INFO line-0001" || snapshot.Lines[2004].Text != "INFO line-2005" {
		t.Fatalf("snapshot boundary lines = %q ... %q", snapshot.Lines[0].Text, snapshot.Lines[2004].Text)
	}
}

func TestListDesktopLogFilesMissingDirReturnsEmpty(t *testing.T) {
	files, err := listDesktopLogFiles(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("listDesktopLogFiles missing dir: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("files = %+v, want empty", files)
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
