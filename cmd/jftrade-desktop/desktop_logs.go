package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
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

	desktopLogEventAppend = "jftrade:desktop-log:append"
	desktopLogPageDefault = 200
	desktopLogPageMaximum = 500
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
}

type DesktopLogDay struct {
	Day string `json:"day"`
}

type DesktopLogLine struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

type DesktopLogPage struct {
	Day        string           `json:"day"`
	LogDir     string           `json:"logDir"`
	Items      []DesktopLogLine `json:"items"`
	Offset     int              `json:"offset"`
	Limit      int              `json:"limit"`
	Total      int              `json:"total"`
	NextOffset *int             `json:"nextOffset,omitempty"`
}

type desktopLogAppend struct {
	Day  string         `json:"day"`
	Line DesktopLogLine `json:"line"`
}

// DesktopLogService exposes the desktop-only log viewer surface to generated bindings.
type DesktopLogService struct {
	manager *desktopLogManager
}

func newDesktopLogService(manager *desktopLogManager) *DesktopLogService {
	return &DesktopLogService{manager: manager}
}

func (s *DesktopLogService) ListDays() ([]DesktopLogDay, error) {
	if s == nil || s.manager == nil {
		return nil, fmt.Errorf("desktop log service is unavailable")
	}
	return listDesktopLogDays(s.manager.logDir)
}

func (s *DesktopLogService) ReadPage(day string, level string, query string, offset int, limit int) (DesktopLogPage, error) {
	if s == nil || s.manager == nil {
		return DesktopLogPage{}, fmt.Errorf("desktop log service is unavailable")
	}
	day = strings.TrimSpace(day)
	if day == "" {
		day = s.manager.today()
	}
	path := apiruntime.DeriveDesktopLogPath(s.manager.settingsPath, desktopParseLogDayOrNow(day, s.manager.now()))
	return readDesktopLogPage(path, s.manager.logDir, day, level, query, offset, limit)
}

func (s *DesktopLogService) OpenFolder() error {
	if s == nil || s.manager == nil {
		return fmt.Errorf("desktop log service is unavailable")
	}
	return s.manager.openFolder(runtime.GOOS, s.manager.logDir)
}

func configureDesktopLogging(settingsPath string) *desktopLogManager {
	manager, err := newDesktopLogManager(settingsPath, os.Stderr, time.Now)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "JFTrade desktop log file unavailable: %v\n", err)
		return nil
	}
	log.SetOutput(manager)
	slog.SetDefault(slog.New(slog.NewTextHandler(manager, nil)))
	return manager
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
		result = append(result, desktopLogAppend{Day: day, Line: DesktopLogLine{Level: parseDesktopLogLevel(raw), Text: raw}})
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

func listDesktopLogDays(logDir string) ([]DesktopLogDay, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	days := make([]DesktopLogDay, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if day, ok := desktopLogDayFromFilename(entry.Name()); ok {
			days = append(days, DesktopLogDay{Day: day})
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Day > days[j].Day })
	return days, nil
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

func readDesktopLogPage(path string, logDir string, day string, level string, query string, offset int, limit int) (DesktopLogPage, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = desktopLogPageDefault
	}
	if limit > desktopLogPageMaximum {
		limit = desktopLogPageMaximum
	}
	page := DesktopLogPage{Day: day, LogDir: logDir, Items: []DesktopLogLine{}, Offset: offset, Limit: limit}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return page, nil
		}
		return DesktopLogPage{}, err
	}
	defer func() { _ = file.Close() }()

	level = strings.ToUpper(strings.TrimSpace(level))
	query = strings.ToLower(strings.TrimSpace(query))
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		text := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(text) == "" {
			continue
		}
		line := DesktopLogLine{Level: parseDesktopLogLevel(text), Text: text}
		if level != "" && level != "ALL" && line.Level != level {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(line.Text), query) {
			continue
		}
		index := page.Total
		page.Total++
		if index >= offset && len(page.Items) < limit {
			page.Items = append(page.Items, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return DesktopLogPage{}, err
	}
	if next := offset + len(page.Items); next < page.Total {
		page.NextOffset = &next
	}
	return page, nil
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

func openDesktopLogWindow(app *application.App, applicationName string) {
	if app == nil {
		return
	}
	if window, ok := app.Window.GetByName(desktopLogsWindowName); ok {
		window.Show().Focus()
		return
	}
	window := app.Window.NewWithOptions(desktopLogWindowOptions(applicationName))
	window.SetZoom(desktopWebviewZoom)
	window.Show().Focus()
}

func desktopLogWindowOptions(applicationName string) application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:            desktopLogsWindowName,
		Title:           applicationName + " 日志",
		URL:             desktopLogsURL,
		Width:           1040,
		Height:          720,
		MinWidth:        760,
		MinHeight:       520,
		InitialPosition: application.WindowCentered,
		Zoom:            desktopWebviewZoom,
	}
}
