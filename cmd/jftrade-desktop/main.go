// Command jftrade-desktop runs JFTrade as a Wails desktop application.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/jftrade/jftrade-main/internal/app/apiserver"
	desktopapp "github.com/jftrade/jftrade-main/internal/desktop"
	desktopicons "github.com/jftrade/jftrade-main/internal/desktop/icons"
	"github.com/jftrade/jftrade-main/internal/frontendassets"
	"github.com/jftrade/jftrade-main/internal/live"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"

	// Embed IANA timezone database for desktop builds on minimal systems.
	_ "time/tzdata"
)

const desktopWebviewZoom = 1.0

const (
	desktopSettingsURL = "/settings"
	desktopDocsURL     = "/docs/"
)

func main() {
	configureDesktopEnvironment()

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	state := newDesktopAppState(stopSignals)
	app, linkService, notificationSink := newDesktopApplication(state)
	window := newDesktopMainWindow(app, state)
	configureDesktopSystemTray(app, window, linkService, state)
	startDesktopAPI(ctx, state, notificationSink)
	quitDesktopOnSignal(ctx, app, state)

	if err := app.Run(); err != nil {
		log.Fatalf("JFTrade desktop failed: %v", err)
	}
}

func configureDesktopEnvironment() {
	if os.Getenv("DISABLE_MARKETS_CACHE") == "" {
		jftradeLogError(os.Setenv("DISABLE_MARKETS_CACHE", "1"))
	}
}

type desktopShutdownFunc func(context.Context) error

type desktopAppState struct {
	shutdown     desktopShutdownFunc
	shutdownOnce sync.Once
	exiting      atomic.Bool
	stopSignals  context.CancelFunc
	mainWindow   desktopWindowHider
}

func newDesktopAppState(stopSignals context.CancelFunc) *desktopAppState {
	return &desktopAppState{
		shutdown:    func(context.Context) error { return nil },
		stopSignals: stopSignals,
	}
}

func (state *desktopAppState) shouldQuit() bool {
	return shouldQuitDesktopApp(&state.exiting, state.mainWindow)
}

func (state *desktopAppState) shutdownApp() {
	state.shutdownOnce.Do(func() {
		state.stopSignals()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		jftradeLogError(state.shutdown(shutdownCtx))
	})
}

func (state *desktopAppState) quit(app *application.App) {
	state.exiting.Store(true)
	app.Quit()
}

func newDesktopApplication(state *desktopAppState) (*application.App, *DesktopLinkService, *desktopNotificationSink) {
	var notificationService *notifications.NotificationService
	linkService := &DesktopLinkService{}
	services := []application.Service{application.NewService(linkService)}
	if runtime.GOOS != "darwin" || macBundleIdentifier() != "" {
		notificationService = notifications.New()
		services = append(services, application.NewService(notificationService))
	} else {
		log.Printf("JFTrade desktop system notifications disabled: macOS requires running from JFTrade.app with CFBundleIdentifier")
	}
	notificationSink := newDesktopNotificationSink(notificationService, resolveSettingsPath())

	app := application.New(application.Options{
		Name:        "JFTrade",
		Description: "JFTrade desktop trading console",
		Icon:        desktopicons.Application,
		Services:    services,
		Assets:      desktopAssets(),
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		ShouldQuit: func() bool {
			return state.shouldQuit()
		},
		OnShutdown: func() {
			state.shutdownApp()
		},
	})
	linkService.app = app
	return app, linkService, notificationSink
}

func newDesktopMainWindow(app *application.App, state *desktopAppState) application.Window {
	window := app.Window.NewWithOptions(mainWindowOptions())
	state.mainWindow = window
	window.SetZoom(desktopWebviewZoom)
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if state.exiting.Load() {
			return
		}
		window.Hide()
		e.Cancel()
	})
	return window
}

func configureDesktopSystemTray(app *application.App, window application.Window, linkService *DesktopLinkService, state *desktopAppState) {
	systemTray := app.SystemTray.New()
	systemTray.SetTooltip("JFTrade")
	if runtime.GOOS == "darwin" {
		systemTray.SetIcon(desktopicons.TrayLight)
	} else {
		systemTray.SetIcon(desktopicons.TrayLight).SetDarkModeIcon(desktopicons.TrayDark)
	}

	menu := newDesktopTrayMenu(window, linkService, func() {
		state.quit(app)
	})
	systemTray.SetMenu(menu)
	configureDesktopTrayMenuClick(systemTray, runtime.GOOS)
}

func startDesktopAPI(ctx context.Context, state *desktopAppState, notificationSink *desktopNotificationSink) {
	var err error
	state.shutdown, err = apiserver.StartDesktop(ctx, notificationSink.Notify)
	if err != nil {
		log.Fatalf("JFTrade desktop API startup failed: %v", err)
	}
}

func quitDesktopOnSignal(ctx context.Context, app *application.App, state *desktopAppState) {
	go func() {
		<-ctx.Done()
		state.quit(app)
	}()
}

func mainWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:            "main",
		Title:           "JFTrade",
		URL:             "/",
		Width:           1280,
		Height:          820,
		MinWidth:        1024,
		MinHeight:       700,
		InitialPosition: application.WindowCentered,
		Zoom:            desktopWebviewZoom,
	}
}

type desktopWindowHider interface {
	Hide() application.Window
}

func shouldQuitDesktopApp(exiting *atomic.Bool, window desktopWindowHider) bool {
	if exiting != nil && exiting.Load() {
		return true
	}
	if window != nil {
		window.Hide()
	}
	desktopDockIconHider()
	return false
}

func newDesktopTrayMenu(window application.Window, linkService *DesktopLinkService, quit func()) *application.Menu {
	menu := application.NewMenu()
	menu.Add("打开 JFTrade").OnClick(func(*application.Context) {
		if window == nil {
			return
		}
		window.SetURL("/")
		window.Show().Focus()
	})
	menu.Add("设置").OnClick(func(*application.Context) {
		if window == nil {
			return
		}
		window.SetURL(desktopSettingsURL)
		window.Show().Focus()
	})
	menu.Add("文档").OnClick(func(*application.Context) {
		if linkService == nil || linkService.app == nil {
			return
		}
		linkService.openDocsWindow(desktopDocsURL)
	})
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) {
		if quit != nil {
			quit()
		}
	})
	return menu
}

func configureDesktopTrayMenuClick(systemTray *application.SystemTray, goos string) {
	if systemTray == nil || !shouldUseExplicitTrayMenuClick(goos) {
		return
	}
	systemTray.OnClick(func() {
		showDesktopTrayMenu(systemTray)
	})
	systemTray.OnRightClick(func() {
		showDesktopTrayMenu(systemTray)
	})
}

func shouldUseExplicitTrayMenuClick(goos string) bool {
	return goos == "darwin" || goos == "windows"
}

func desktopAssets() application.AssetOptions {
	frontendFS, available, err := frontendassets.FileSystem()
	if err != nil {
		log.Printf("JFTrade embedded frontend assets unavailable: %v", err)
	}
	if available && frontendFS != nil {
		return application.AssetOptions{
			Handler: newDesktopAssetHandler(application.AssetFileServerFS(frontendFS), frontendFS, desktopRuntimeAPIBaseURL()),
		}
	}
	return application.AlphaAssets
}

func newDesktopAssetHandler(next http.Handler, frontendFS fs.FS, apiBaseURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := desktopCleanAssetPath(r.URL.Path)
		if cleanPath == "/runtime-config.js" {
			writeDesktopRuntimeConfig(w, r, apiBaseURL)
			return
		}
		if !shouldServeDesktopIndex(r, cleanPath) {
			next.ServeHTTP(w, r)
			return
		}

		capture := newDesktopResponseCapture()
		next.ServeHTTP(capture, r)
		if capture.statusCode() != http.StatusNotFound {
			capture.replay(w)
			return
		}
		if serveDesktopIndex(w, r, frontendFS) {
			return
		}
		capture.replay(w)
	})
}

func desktopCleanAssetPath(requestPath string) string {
	cleanPath := path.Clean("/" + strings.TrimSpace(requestPath))
	if cleanPath == "." {
		return "/"
	}
	return cleanPath
}

func shouldServeDesktopIndex(r *http.Request, cleanPath string) bool {
	if r == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return false
	}
	if cleanPath == "/" {
		return true
	}
	if cleanPath == "/assets" || strings.HasPrefix(cleanPath, "/assets/") ||
		strings.HasPrefix(cleanPath, "/api/") ||
		strings.HasPrefix(cleanPath, "/swagger") ||
		strings.HasPrefix(cleanPath, "/docs") ||
		strings.Contains(path.Base(cleanPath), ".") {
		return false
	}

	for _, route := range []string{
		"/oobe",
		"/workspace",
		"/system",
		"/settings",
		"/account",
		"/risk",
		"/broker",
		"/portfolio",
		"/execution",
		"/strategy",
		"/adk",
		"/backtest",
	} {
		if cleanPath == route || strings.HasPrefix(cleanPath, route+"/") {
			return true
		}
	}
	return false
}

func serveDesktopIndex(w http.ResponseWriter, r *http.Request, frontendFS fs.FS) bool {
	if frontendFS == nil {
		return false
	}
	data, err := fs.ReadFile(frontendFS, "index.html")
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(data))
	return true
}

type desktopResponseCapture struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newDesktopResponseCapture() *desktopResponseCapture {
	return &desktopResponseCapture{header: make(http.Header)}
}

func (c *desktopResponseCapture) Header() http.Header {
	return c.header
}

func (c *desktopResponseCapture) Write(data []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.body.Write(data)
}

func (c *desktopResponseCapture) WriteHeader(statusCode int) {
	if c.status == 0 {
		c.status = statusCode
	}
}

func (c *desktopResponseCapture) statusCode() int {
	if c.status == 0 {
		return http.StatusOK
	}
	return c.status
}

func (c *desktopResponseCapture) replay(w http.ResponseWriter) {
	for key, values := range c.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(c.statusCode())
	if c.body.Len() > 0 {
		_, _ = w.Write(c.body.Bytes())
	}
}

func writeDesktopRuntimeConfig(w http.ResponseWriter, r *http.Request, apiBaseURL string) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	payload := map[string]any{
		"apiBaseUrl":   strings.TrimRight(apiBaseURL, "/"),
		"authRequired": false,
		"desktopMode":  true,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "failed to encode runtime config", http.StatusInternalServerError)
		return
	}
	_, _ = fmt.Fprintf(
		w,
		"window.__JFTRADE_RUNTIME_CONFIG__ = Object.assign({}, window.__JFTRADE_RUNTIME_CONFIG__, %s);\n",
		data,
	)
}

func desktopRuntimeAPIBaseURL() string {
	config, err := apiserver.ResolveDesktopRuntimeConfig()
	if err != nil {
		log.Printf("JFTrade desktop runtime config unavailable: %v", err)
		return ""
	}
	return config.APIBaseURL
}

func resolveSettingsPath() string {
	config, err := apiserver.ResolveDesktopRuntimeConfig()
	if err != nil {
		log.Printf("JFTrade desktop settings path fallback after runtime config error: %v", err)
		if path := strings.TrimSpace(os.Getenv("JFTRADE_SETTINGS_PATH")); path != "" {
			return path
		}
		return ""
	}
	return config.SettingsPath
}

type desktopNotificationSink struct {
	service      *notifications.NotificationService
	settingsPath string
	authorized   atomic.Bool
}

func newDesktopNotificationSink(service *notifications.NotificationService, settingsPath string) *desktopNotificationSink {
	return &desktopNotificationSink{service: service, settingsPath: settingsPath}
}

func (s *desktopNotificationSink) Notify(event live.Event) live.NotificationDelivery {
	if s == nil || s.service == nil {
		return live.NotificationNotDelivered(live.NotificationDeliveryUnsupported, "desktop system notifications are not available")
	}
	settings := s.currentSettings()
	if !desktopapp.ShouldForwardSystemNotification(settings, event) {
		return live.NotificationNotDelivered(live.NotificationDeliveryFiltered, "notification filtered by desktop settings")
	}
	if !s.ensureAuthorized() {
		return live.NotificationNotDelivered(live.NotificationDeliveryUnauthorized, "operating system notification permission is not authorized")
	}

	options := notifications.NotificationOptions{
		ID:                fmt.Sprintf("jftrade-%d", event.Sequence),
		Title:             strings.TrimSpace(event.Title),
		Body:              strings.TrimSpace(event.Message),
		ThreadID:          desktopapp.NotificationThreadID(event),
		InterruptionLevel: desktopapp.NotificationInterruptionLevel(event.Level),
		Data: map[string]interface{}{
			"sequence": event.Sequence,
			"category": event.Category,
			"source":   event.Source,
		},
	}
	if options.Title == "" {
		options.Title = "JFTrade"
	}
	if !settings.SoundEnabled {
		options.Sound = &notifications.NotificationSound{Silent: true}
	}
	if err := s.service.SendNotification(options); err != nil {
		log.Printf("JFTrade desktop notification failed: %v", err)
		return live.NotificationNotDelivered(live.NotificationDeliveryFailed, err.Error())
	}
	return live.NotificationDelivered("sent to operating system notification center")
}

func (s *desktopNotificationSink) currentSettings() jfsettings.SystemNotificationSettings {
	store, err := settingsfile.New(s.settingsPath)
	if err != nil {
		log.Printf("JFTrade system notification settings unavailable: %v", err)
		return settingsfile.DefaultSystemNotificationSettings()
	}
	return store.SystemNotificationSettings()
}

func (s *desktopNotificationSink) ensureAuthorized() bool {
	if s.authorized.Load() {
		return true
	}
	authorized, err := s.service.CheckNotificationAuthorization()
	if err != nil {
		log.Printf("JFTrade notification authorization check failed: %v", err)
	}
	if !authorized {
		authorized, err = s.service.RequestNotificationAuthorization()
		if err != nil {
			log.Printf("JFTrade notification authorization request failed: %v", err)
		}
	}
	if authorized {
		s.authorized.Store(authorized)
	}
	return authorized
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
