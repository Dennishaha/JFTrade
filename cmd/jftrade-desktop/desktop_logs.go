package main

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
)

const (
	desktopLogsURL        = "/desktop-logs"
	desktopLogsWindowName = "logs"

	desktopLogEventReady      = "jftrade:desktop-log:ready"
	desktopLogEventSnapshot   = "jftrade:desktop-log:snapshot"
	desktopLogEventAppend     = "jftrade:desktop-log:append"
	desktopLogEventSelectDay  = "jftrade:desktop-log:select-day"
	desktopLogEventOpenFolder = "jftrade:desktop-log:open-folder"
)

type desktopLogManager struct {
	logDir       string
	settingsPath string
	now          func() time.Time
	original     io.Writer
	openFolder   func(goos string, dir string) error

	mu        sync.Mutex
	file      *os.File
	fileDay   string
	remainder string
	app       *application.App

	daySelectCancels map[string]func()
}

type desktopLogFile struct {
	Day  string `json:"day"`
	Path string `json:"path"`
}

type desktopLogLine struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

type desktopLogSnapshot struct {
	Day    string           `json:"day"`
	LogDir string           `json:"logDir"`
	Files  []desktopLogFile `json:"files"`
	Lines  []desktopLogLine `json:"lines"`
	Error  string           `json:"error,omitempty"`
}

type desktopLogAppend struct {
	Day  string         `json:"day"`
	Line desktopLogLine `json:"line"`
}

func configureDesktopLogging() *desktopLogManager {
	settingsPath := desktopLogSettingsPath()
	manager, err := newDesktopLogManager(settingsPath, os.Stderr, time.Now)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "JFTrade desktop log file unavailable: %v\n", err)
		return nil
	}
	log.SetOutput(manager)
	slog.SetDefault(slog.New(slog.NewTextHandler(manager, nil)))
	return manager
}

func desktopLogSettingsPath() string {
	if path := strings.TrimSpace(os.Getenv("JFTRADE_SETTINGS_PATH")); path != "" {
		return path
	}
	return apiruntime.ResolveLaunchDefaults(true).SettingsPath
}

func newDesktopLogManager(settingsPath string, original io.Writer, now func() time.Time) (*desktopLogManager, error) {
	if now == nil {
		now = time.Now
	}
	manager := &desktopLogManager{
		logDir:       apiruntime.DeriveDesktopLogDir(settingsPath),
		settingsPath: settingsPath,
		now:          now,
		original:     original,
		openFolder:   openDesktopLogFolder,
	}
	if err := os.MkdirAll(manager.logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create desktop log directory %s: %w", manager.logDir, err)
	}
	return manager, nil
}

func (m *desktopLogManager) bindApp(app *application.App) {
	if m == nil || app == nil {
		return
	}
	m.mu.Lock()
	m.app = app
	m.mu.Unlock()

	app.Event.On(desktopLogEventReady, func(*application.CustomEvent) {
		m.emitSnapshot(m.today())
	})
	app.Event.On(desktopLogEventOpenFolder, func(*application.CustomEvent) {
		if err := m.openFolder(runtime.GOOS, m.logDir); err != nil {
			m.emitSnapshotWithError(m.today(), err)
		}
	})
}

func (m *desktopLogManager) Write(p []byte) (int, error) {
	if m == nil {
		return len(p), nil
	}
	if m.original != nil {
		_, _ = m.original.Write(p)
	}

	lines, err := m.writeAndCollectLines(p)
	if err != nil && m.original != nil {
		_, _ = fmt.Fprintf(m.original, "JFTrade desktop log write failed: %v\n", err)
	}
	for _, line := range lines {
		m.emitAppend(line)
	}
	return len(p), err
}

func (m *desktopLogManager) writeAndCollectLines(p []byte) ([]desktopLogAppend, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	day := m.todayLocked()
	var writeErr error
	if err := m.ensureFileLocked(day); err != nil {
		writeErr = err
	} else if m.file != nil {
		_, writeErr = m.file.Write(p)
	}

	m.remainder += string(p)
	var result []desktopLogAppend
	for {
		index := strings.IndexByte(m.remainder, '\n')
		if index < 0 {
			break
		}
		raw := strings.TrimRight(m.remainder[:index], "\r")
		m.remainder = m.remainder[index+1:]
		if strings.TrimSpace(raw) == "" {
			continue
		}
		result = append(result, desktopLogAppend{
			Day:  day,
			Line: desktopLogLine{Level: parseDesktopLogLevel(raw), Text: raw},
		})
	}
	return result, writeErr
}

func (m *desktopLogManager) ensureFileLocked(day string) error {
	if m.file != nil && m.fileDay == day {
		return nil
	}
	if m.file != nil {
		_ = m.file.Close()
		m.file = nil
	}
	if err := os.MkdirAll(m.logDir, 0o755); err != nil {
		return err
	}
	path := apiruntime.DeriveDesktopLogPath(m.settingsPath, desktopParseLogDayOrNow(day, m.now()))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	m.file = file
	m.fileDay = day
	return nil
}

func (m *desktopLogManager) close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.file == nil {
		return nil
	}
	err := m.file.Close()
	m.file = nil
	return err
}

func (m *desktopLogManager) today() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.todayLocked()
}

func (m *desktopLogManager) todayLocked() string {
	return m.now().Local().Format("2006-01-02")
}

func (m *desktopLogManager) emitAppend(payload desktopLogAppend) {
	m.mu.Lock()
	app := m.app
	m.mu.Unlock()
	if app != nil {
		app.Event.Emit(desktopLogEventAppend, payload)
	}
}

func (m *desktopLogManager) emitSnapshot(day string) {
	m.emitSnapshotWithError(day, nil)
}

func (m *desktopLogManager) emitSnapshotWithError(day string, inputErr error) {
	snapshot := m.snapshot(day)
	if inputErr != nil {
		snapshot.Error = inputErr.Error()
	}
	m.mu.Lock()
	app := m.app
	m.mu.Unlock()
	if app != nil {
		app.Event.Emit(desktopLogEventSnapshot, snapshot)
	}
}

func (m *desktopLogManager) snapshot(day string) desktopLogSnapshot {
	if strings.TrimSpace(day) == "" {
		day = m.today()
	}
	files, err := listDesktopLogFiles(m.logDir)
	snapshot := desktopLogSnapshot{Day: day, LogDir: m.logDir, Files: files}
	if err != nil {
		snapshot.Error = err.Error()
		return snapshot
	}
	m.bindDaySelectEvents(files)
	path := apiruntime.DeriveDesktopLogPath(m.settingsPath, desktopParseLogDayOrNow(day, m.now()))
	lines, err := readDesktopLogLines(path)
	if err != nil && !os.IsNotExist(err) {
		snapshot.Error = err.Error()
		return snapshot
	}
	snapshot.Lines = lines
	return snapshot
}

func (m *desktopLogManager) bindDaySelectEvents(files []desktopLogFile) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.app == nil {
		return
	}
	if m.daySelectCancels == nil {
		m.daySelectCancels = make(map[string]func())
	}
	for _, file := range files {
		day := file.Day
		if strings.TrimSpace(day) == "" {
			continue
		}
		eventName := desktopLogSelectDayEventName(day)
		if _, ok := m.daySelectCancels[eventName]; ok {
			continue
		}
		m.daySelectCancels[eventName] = m.app.Event.On(eventName, func(*application.CustomEvent) {
			m.emitSnapshot(day)
		})
	}
}

func desktopLogSelectDayEventName(day string) string {
	return desktopLogEventSelectDay + ":" + day
}

func listDesktopLogFiles(logDir string) ([]desktopLogFile, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	files := make([]desktopLogFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		day, ok := desktopLogDayFromFilename(name)
		if !ok {
			continue
		}
		files = append(files, desktopLogFile{Day: day, Path: filepath.Join(logDir, name)})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Day > files[j].Day
	})
	return files, nil
}

func desktopLogDayFromFilename(name string) (string, bool) {
	if !strings.HasPrefix(name, "desktop-") || !strings.HasSuffix(name, ".log") {
		return "", false
	}
	day := strings.TrimSuffix(strings.TrimPrefix(name, "desktop-"), ".log")
	if _, err := time.Parse("2006-01-02", day); err != nil {
		return "", false
	}
	return day, true
}

func readDesktopLogLines(path string) ([]desktopLogLine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rawLines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	lines := make([]desktopLogLine, 0, len(rawLines))
	for _, raw := range rawLines {
		raw = strings.TrimRight(raw, "\r")
		if strings.TrimSpace(raw) == "" {
			continue
		}
		lines = append(lines, desktopLogLine{Level: parseDesktopLogLevel(raw), Text: raw})
	}
	return lines, nil
}

func parseDesktopLogLevel(line string) string {
	upper := strings.ToUpper(line)
	for _, item := range []struct {
		level  string
		tokens []string
	}{
		{level: "ERROR", tokens: []string{"LEVEL=ERROR", `"LEVEL":"ERROR"`, " ERROR ", "[ERROR]", " ERROR:", "ERROR "}},
		{level: "WARN", tokens: []string{"LEVEL=WARN", "LEVEL=WARNING", `"LEVEL":"WARN"`, `"LEVEL":"WARNING"`, " WARN ", " WARNING ", "[WARN]", "[WARNING]", "WARN ", "WARNING "}},
		{level: "DEBUG", tokens: []string{"LEVEL=DEBUG", `"LEVEL":"DEBUG"`, " DEBUG ", "[DEBUG]", "DEBUG "}},
		{level: "INFO", tokens: []string{"LEVEL=INFO", `"LEVEL":"INFO"`, " INFO ", "[INFO]", "INFO "}},
	} {
		for _, token := range item.tokens {
			if strings.Contains(upper, token) {
				return item.level
			}
		}
	}
	return "INFO"
}

func desktopParseLogDayOrNow(day string, fallback time.Time) time.Time {
	parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(day), time.Local)
	if err != nil {
		return fallback
	}
	return parsed
}

func openDesktopLogFolder(goos string, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name, args, err := desktopOpenFolderCommand(goos, dir)
	if err != nil {
		return err
	}
	return exec.Command(name, args...).Start()
}

func desktopOpenFolderCommand(goos string, dir string) (string, []string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", nil, fmt.Errorf("log directory is empty")
	}
	switch goos {
	case "darwin":
		return "open", []string{dir}, nil
	case "windows":
		return "explorer", []string{dir}, nil
	default:
		return "xdg-open", []string{dir}, nil
	}
}

func openDesktopLogWindow(app *application.App, manager *desktopLogManager) {
	if app == nil {
		return
	}
	if window, ok := app.Window.GetByName(desktopLogsWindowName); ok {
		window.Show().Focus()
		return
	}
	window := app.Window.NewWithOptions(desktopLogWindowOptions())
	window.SetZoom(desktopWebviewZoom)
	window.Show().Focus()
	if manager != nil {
		manager.emitSnapshot(manager.today())
	}
}

func desktopLogWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:            desktopLogsWindowName,
		Title:           "JFTrade 日志",
		HTML:            desktopLogsHTML,
		Width:           1040,
		Height:          720,
		MinWidth:        760,
		MinHeight:       520,
		InitialPosition: application.WindowCentered,
		Zoom:            desktopWebviewZoom,
		// The log window is fully controlled by this binary and uses Wails'
		// inline event shim so it works in both production and dev mode.
		AllowSimpleEventEmit: true,
	}
}

func serveDesktopLogsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.WriteString(w, desktopLogsHTML)
}

const desktopLogsHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>JFTrade 日志</title>
  <style>
    :root { color-scheme: dark; --bg:#101214; --panel:#171a1d; --line:#262b30; --text:#e7ecef; --muted:#99a3ad; --accent:#6db7ff; --error:#ff6b6b; --warn:#f4b942; --info:#73d28b; --debug:#a99cff; }
    * { box-sizing: border-box; }
    html, body { margin: 0; height: 100%; background: var(--bg); color: var(--text); font: 13px/1.45 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { display: flex; flex-direction: column; min-width: 0; }
    .toolbar { display: flex; align-items: center; gap: 10px; padding: 10px 12px; background: var(--panel); border-bottom: 1px solid var(--line); }
    .title { font-size: 14px; font-weight: 650; white-space: nowrap; }
    .spacer { flex: 1; min-width: 8px; }
    label { color: var(--muted); font-size: 12px; }
    select, input, button { height: 30px; border: 1px solid #30363d; border-radius: 6px; background: #111418; color: var(--text); padding: 0 10px; font: inherit; }
    input { width: 180px; min-width: 120px; }
    button { cursor: pointer; }
    button:disabled { cursor: default; color: #5f6872; border-color: #272c31; }
    button:hover:not(:disabled), input:hover, select:hover { border-color: #4b5560; }
    .path { padding: 8px 12px; color: var(--muted); border-bottom: 1px solid var(--line); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .error { display: none; padding: 8px 12px; color: var(--error); border-bottom: 1px solid rgba(255,107,107,.35); background: rgba(255,107,107,.08); }
    .logs { flex: 1; overflow: auto; padding: 10px 0; font: 12px/1.5 ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace; }
    .row { display: grid; grid-template-columns: 62px minmax(0, 1fr); gap: 8px; padding: 1px 12px; white-space: pre-wrap; overflow-wrap: anywhere; }
    .badge { font-weight: 700; }
    .level-ERROR .badge { color: var(--error); }
    .level-WARN .badge { color: var(--warn); }
    .level-INFO .badge { color: var(--info); }
    .level-DEBUG .badge { color: var(--debug); }
    .empty { color: var(--muted); padding: 24px 12px; }
    .pager { display: flex; align-items: center; gap: 8px; padding: 8px 12px; background: var(--panel); border-top: 1px solid var(--line); color: var(--muted); }
    .pageInfo { min-width: 170px; text-align: center; color: var(--text); }
    .summary { margin-left: auto; white-space: nowrap; }
    @media (max-width: 760px) {
      .toolbar { flex-wrap: wrap; }
      .spacer { display: none; }
      .row { grid-template-columns: 52px minmax(0, 1fr); }
      input { flex: 1; min-width: 160px; }
      .pager { flex-wrap: wrap; }
      .summary { margin-left: 0; width: 100%; }
    }
  </style>
</head>
<body>
  <div class="toolbar">
    <div class="title">JFTrade 日志</div>
    <label for="day">日期</label>
    <select id="day"></select>
    <label for="level">级别</label>
    <select id="level">
      <option value="ALL">ALL</option>
      <option value="ERROR">ERROR</option>
      <option value="WARN">WARN</option>
      <option value="INFO">INFO</option>
      <option value="DEBUG">DEBUG</option>
    </select>
    <label for="keyword">关键词</label>
    <input id="keyword" type="search" placeholder="过滤日志内容">
    <label for="pageSize">每页</label>
    <select id="pageSize">
      <option value="100">100</option>
      <option value="200" selected>200</option>
      <option value="500">500</option>
      <option value="1000">1000</option>
    </select>
    <div class="spacer"></div>
    <button id="openFolder" type="button">打开日志文件夹</button>
  </div>
  <div id="path" class="path"></div>
  <div id="error" class="error"></div>
  <div id="logs" class="logs" role="log" aria-live="polite"></div>
  <div class="pager">
    <button id="firstPage" type="button">首页</button>
    <button id="prevPage" type="button">上一页</button>
    <span id="pageInfo" class="pageInfo">第 1 / 1 页</span>
    <button id="nextPage" type="button">下一页</button>
    <button id="lastPage" type="button">末页</button>
    <span id="summary" class="summary"></span>
  </div>
  <script>
    const Events = window.wails && window.wails.Events ? window.wails.Events : { On: function () {}, Emit: function () {} };
    const eventReady = "jftrade:desktop-log:ready";
    const eventSnapshot = "jftrade:desktop-log:snapshot";
    const eventAppend = "jftrade:desktop-log:append";
    const eventSelectDay = "jftrade:desktop-log:select-day";
    const eventOpenFolder = "jftrade:desktop-log:open-folder";
    const state = { day: "", logDir: "", files: [], lines: [], level: "ALL", keyword: "", page: 1, pageSize: 200 };
    const daySelect = document.getElementById("day");
    const levelSelect = document.getElementById("level");
    const keywordInput = document.getElementById("keyword");
    const pageSizeSelect = document.getElementById("pageSize");
    const logs = document.getElementById("logs");
    const path = document.getElementById("path");
    const error = document.getElementById("error");
    const openFolder = document.getElementById("openFolder");
    const firstPage = document.getElementById("firstPage");
    const prevPage = document.getElementById("prevPage");
    const nextPage = document.getElementById("nextPage");
    const lastPage = document.getElementById("lastPage");
    const pageInfo = document.getElementById("pageInfo");
    const summary = document.getElementById("summary");

    function emit(name) {
      if (!window._wails || typeof window._wails.invoke !== "function") {
        setTimeout(function () { emit(name); }, 30);
        return;
      }
      Events.Emit(name);
    }
    function selectDayEventName(day) {
      return eventSelectDay + ":" + day;
    }
    function escapeText(value) {
      return String(value || "").replace(/[&<>"']/g, function (ch) {
        return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch];
      });
    }
    function showError(message) {
      error.textContent = message || "";
      error.style.display = message ? "block" : "none";
    }
    function renderDays() {
      const days = state.files.map(function (file) { return file.day; });
      if (state.day && days.indexOf(state.day) === -1) days.unshift(state.day);
      daySelect.innerHTML = days.map(function (day) {
        return '<option value="' + escapeText(day) + '">' + escapeText(day) + '</option>';
      }).join("");
      daySelect.value = state.day;
    }
    function visibleLines() {
      const keyword = state.keyword.trim().toLowerCase();
      return state.lines.filter(function (line) {
        if (state.level !== "ALL" && line.level !== state.level) return false;
        if (!keyword) return true;
        return String(line.text || "").toLowerCase().indexOf(keyword) >= 0;
      });
    }
    function totalPagesFor(count) {
      return Math.max(1, Math.ceil(count / state.pageSize));
    }
    function clampPage(count) {
      const pages = totalPagesFor(count);
      if (state.page < 1) state.page = 1;
      if (state.page > pages) state.page = pages;
      return pages;
    }
    function goToLastPage() {
      state.page = totalPagesFor(visibleLines().length);
    }
    function renderPager(count, pages) {
      pageInfo.textContent = "第 " + state.page + " / " + pages + " 页";
      summary.textContent = "匹配 " + count + " 条 / 共 " + state.lines.length + " 条";
      firstPage.disabled = state.page <= 1;
      prevPage.disabled = state.page <= 1;
      nextPage.disabled = state.page >= pages;
      lastPage.disabled = state.page >= pages;
    }
    function renderLogs() {
      const lines = visibleLines();
      const pages = clampPage(lines.length);
      renderPager(lines.length, pages);
      if (lines.length === 0) {
        logs.innerHTML = '<div class="empty">没有匹配的日志</div>';
        return;
      }
      const start = (state.page - 1) * state.pageSize;
      const pageLines = lines.slice(start, start + state.pageSize);
      logs.innerHTML = pageLines.map(function (line) {
        const level = line.level || "INFO";
        return '<div class="row level-' + escapeText(level) + '"><span class="badge">' + escapeText(level) + '</span><span>' + escapeText(line.text) + '</span></div>';
      }).join("");
      logs.scrollTop = logs.scrollHeight;
    }
    function renderSnapshot(data) {
      state.day = data.day || state.day;
      state.logDir = data.logDir || "";
      state.files = Array.isArray(data.files) ? data.files : [];
      state.lines = Array.isArray(data.lines) ? data.lines : [];
      goToLastPage();
      path.textContent = state.logDir ? "日志目录: " + state.logDir : "";
      showError(data.error || "");
      renderDays();
      renderLogs();
    }
    Events.On(eventSnapshot, function (event) {
      renderSnapshot(event.data || {});
    });
    Events.On(eventAppend, function (event) {
      const data = event.data || {};
      if (!data.line || data.day !== state.day) return;
      const wasLastPage = state.page >= totalPagesFor(visibleLines().length);
      state.lines.push(data.line);
      if (wasLastPage) goToLastPage();
      renderLogs();
    });
    daySelect.addEventListener("change", function () {
      emit(selectDayEventName(daySelect.value));
    });
    levelSelect.addEventListener("change", function () {
      state.level = levelSelect.value;
      state.page = 1;
      renderLogs();
    });
    keywordInput.addEventListener("input", function () {
      state.keyword = keywordInput.value;
      state.page = 1;
      renderLogs();
    });
    pageSizeSelect.addEventListener("change", function () {
      state.pageSize = parseInt(pageSizeSelect.value, 10) || 200;
      state.page = 1;
      renderLogs();
    });
    firstPage.addEventListener("click", function () { state.page = 1; renderLogs(); });
    prevPage.addEventListener("click", function () { state.page -= 1; renderLogs(); });
    nextPage.addEventListener("click", function () { state.page += 1; renderLogs(); });
    lastPage.addEventListener("click", function () { state.page = totalPagesFor(visibleLines().length); renderLogs(); });
    openFolder.addEventListener("click", function () {
      emit(eventOpenFolder);
    });
    emit(eventReady);
  </script>
</body>
</html>`
