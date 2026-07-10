package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestDesktopWindowStatePathIsReleaseOnly(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if got := desktopWindowStatePath(desktopBuildProfile{}, settingsPath); got != "" {
		t.Fatalf("development desktop state path = %q, want empty", got)
	}
	if got := desktopWindowStatePath(desktopBuildProfile{Release: true}, settingsPath); got != filepath.Join(filepath.Dir(settingsPath), desktopStateFilename) {
		t.Fatalf("release desktop state path = %q", got)
	}
}

func TestDesktopWindowStateRoundTrip(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "runtime", desktopStateFilename)
	window := &fakeDesktopStateWindow{bounds: application.Rect{X: 40, Y: 60, Width: 1280, Height: 820}}
	store := newDesktopWindowStateStore(statePath, window, desktopWindowState{})
	if err := store.flush(); err != nil {
		t.Fatalf("flush desktop state: %v", err)
	}
	state, ok, err := loadDesktopWindowState(statePath)
	if err != nil || !ok {
		t.Fatalf("load desktop state = (%#v, %v, %v)", state, ok, err)
	}
	if state.X != 40 || state.Y != 60 || state.Width != 1280 || state.Height != 820 || state.Maximised {
		t.Fatalf("desktop state = %#v", state)
	}
	if mode := mustFileMode(t, statePath); mode.Perm() != 0o600 {
		t.Fatalf("desktop state mode = %v, want 0600", mode.Perm())
	}
}

func TestApplyDesktopWindowState(t *testing.T) {
	base := mainWindowOptions(desktopBuildProfile{ApplicationName: "JFTrade"})
	saved := desktopWindowState{
		Version: desktopWindowStateVersion, X: -1200, Y: 50, Width: 1400, Height: 900, Maximised: true,
	}
	options := applyDesktopWindowState(base, saved, true)
	if options.InitialPosition != application.WindowXY || options.X != -1200 || options.Y != 50 {
		t.Fatalf("restored position = %#v", options)
	}
	if options.Width != 1400 || options.Height != 900 || options.StartState != application.WindowStateMaximised {
		t.Fatalf("restored size/state = %#v", options)
	}
}

func TestEnsureDesktopWindowVisibleMovesOffscreenWindow(t *testing.T) {
	window := &fakeDesktopStateWindow{bounds: application.Rect{X: 9000, Y: 9000, Width: 1280, Height: 820}}
	screens := []*application.Screen{{
		IsPrimary: true,
		WorkArea:  application.Rect{X: 0, Y: 0, Width: 1920, Height: 1080},
	}}
	ensureDesktopWindowVisible(window, screens)
	if window.bounds.X != 320 || window.bounds.Y != 130 {
		t.Fatalf("recovered bounds = %#v", window.bounds)
	}
}

type fakeDesktopStateWindow struct {
	bounds    application.Rect
	maximised bool
}

func (w *fakeDesktopStateWindow) Bounds() application.Rect { return w.bounds }
func (w *fakeDesktopStateWindow) IsMaximised() bool        { return w.maximised }
func (w *fakeDesktopStateWindow) SetPosition(x, y int) {
	w.bounds.X = x
	w.bounds.Y = y
}

func mustFileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode()
}
