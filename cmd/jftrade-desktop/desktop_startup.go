package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/jftrade/jftrade-main/internal/app/apiserver"
	"github.com/jftrade/jftrade-main/internal/live"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

const (
	desktopStartupStateStarting = "starting"
	desktopStartupStateReady    = "ready"
	desktopStartupStateFailed   = "failed"

	desktopStartupPhaseNativeReady = "native-ready"
	desktopStartupPhaseAPIStarting = "api-starting"
	desktopStartupPhaseAPIReady    = "api-ready"
	desktopStartupPhaseAPIFailed   = "api-failed"
)

// DesktopStartupSnapshot is the desktop-only startup contract consumed before
// the local HTTP API is available.
type DesktopStartupSnapshot struct {
	State     string `json:"state"`
	Phase     string `json:"phase"`
	Message   string `json:"message"`
	StartedAt string `json:"startedAt"`
}

// DesktopStartupService exposes local API startup state to the loading screen.
type DesktopStartupService struct {
	mu       sync.RWMutex
	snapshot DesktopStartupSnapshot
	app      *application.App
	state    *desktopAppState
}

func newDesktopStartupService(state *desktopAppState, now time.Time) *DesktopStartupService {
	return &DesktopStartupService{
		state: state,
		snapshot: DesktopStartupSnapshot{
			State:     desktopStartupStateStarting,
			Phase:     desktopStartupPhaseNativeReady,
			Message:   "正在启动本地服务…",
			StartedAt: now.UTC().Format(time.RFC3339Nano),
		},
	}
}

// Snapshot returns a consistent point-in-time startup status.
func (s *DesktopStartupService) Snapshot() DesktopStartupSnapshot {
	if s == nil {
		return DesktopStartupSnapshot{
			State:   desktopStartupStateFailed,
			Phase:   desktopStartupPhaseAPIFailed,
			Message: "桌面启动服务不可用",
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

// Quit exits the desktop application from the startup failure screen.
func (s *DesktopStartupService) Quit() {
	if s == nil || s.app == nil || s.state == nil {
		return
	}
	s.state.quit(s.app)
}

func (s *DesktopStartupService) update(state string, phase string, message string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.snapshot.State = state
	s.snapshot.Phase = phase
	s.snapshot.Message = message
	s.mu.Unlock()
}

type desktopAPIStarter func(
	context.Context,
	apiserver.DesktopRuntimeConfig,
	func(live.Event) live.NotificationDelivery,
) (func(context.Context) error, error)

func startDesktopAPIAsync(
	ctx context.Context,
	state *desktopAppState,
	startup *DesktopStartupService,
	notificationSink *desktopNotificationSink,
	runtimeConfig apiserver.DesktopRuntimeConfig,
) {
	startDesktopAPIAsyncWith(
		ctx,
		state,
		startup,
		notificationSink,
		runtimeConfig,
		apiserver.StartDesktopWithConfig,
	)
}

func startDesktopAPIAsyncWith(
	ctx context.Context,
	state *desktopAppState,
	startup *DesktopStartupService,
	notificationSink *desktopNotificationSink,
	runtimeConfig apiserver.DesktopRuntimeConfig,
	start desktopAPIStarter,
) {
	var notify func(live.Event) live.NotificationDelivery
	if notificationSink != nil {
		notify = notificationSink.Notify
	}
	state.startupOnce.Do(func() {
		startup.update(
			desktopStartupStateStarting,
			desktopStartupPhaseAPIStarting,
			"正在启动本地 API 与行情服务…",
		)
		state.startupStarted.Store(true)
		go func() {
			defer close(state.startupDone)
			started := time.Now()
			shutdown, err := start(
				ctx,
				runtimeConfig,
				notify,
			)
			if err != nil {
				if ctx.Err() != nil || state.exiting.Load() {
					return
				}
				log.Printf("JFTrade desktop API startup failed after %s: %v", time.Since(started), err)
				startup.update(
					desktopStartupStateFailed,
					desktopStartupPhaseAPIFailed,
					"本地服务启动失败，请查看日志后重新启动应用。",
				)
				return
			}
			if !state.installShutdown(shutdown) {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				besteffort.LogError(shutdown(shutdownCtx))
				return
			}
			log.Printf("JFTrade desktop API ready in %s", time.Since(started))
			startup.update(
				desktopStartupStateReady,
				desktopStartupPhaseAPIReady,
				"本地服务已就绪",
			)
		}()
	})
}
