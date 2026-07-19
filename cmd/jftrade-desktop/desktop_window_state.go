package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	desktopStateFilename       = "desktop-state.json"
	desktopWindowStateVersion  = 1
	desktopStateWriteDebounce  = 250 * time.Millisecond
	desktopVisibleIntersection = 64
)

type desktopWindowState struct {
	Version   int  `json:"version"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

type desktopStateWindow interface {
	Bounds() application.Rect
	IsMaximised() bool
	SetPosition(x, y int)
}

type desktopWindowStateStore struct {
	mu      sync.Mutex
	path    string
	window  desktopStateWindow
	current desktopWindowState
	timer   *time.Timer
	closed  bool
}

func desktopWindowStatePath(profile desktopBuildProfile, settingsPath string) string {
	if !profile.Release {
		return ""
	}
	return filepath.Join(filepath.Dir(settingsPath), desktopStateFilename)
}

func loadDesktopWindowState(path string) (desktopWindowState, bool, error) {
	if path == "" {
		return desktopWindowState{}, false, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return desktopWindowState{}, false, nil
	}
	if err != nil {
		return desktopWindowState{}, false, fmt.Errorf("read desktop window state: %w", err)
	}
	var state desktopWindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return desktopWindowState{}, false, fmt.Errorf("decode desktop window state: %w", err)
	}
	if !state.valid() {
		return desktopWindowState{}, false, fmt.Errorf("desktop window state is invalid")
	}
	return state, true, nil
}

func (s desktopWindowState) valid() bool {
	return s.Version == desktopWindowStateVersion &&
		s.Width >= 1024 && s.Height >= 700 &&
		s.Width <= 32768 && s.Height <= 32768 &&
		s.X >= -100000 && s.X <= 100000 && s.Y >= -100000 && s.Y <= 100000
}

func applyDesktopWindowState(options application.WebviewWindowOptions, state desktopWindowState, ok bool) application.WebviewWindowOptions {
	if !ok || !state.valid() {
		return options
	}
	options.X = state.X
	options.Y = state.Y
	options.Width = state.Width
	options.Height = state.Height
	options.InitialPosition = application.WindowXY
	if state.Maximised {
		options.StartState = application.WindowStateMaximised
	}
	return options
}

func newDesktopWindowStateStore(path string, window desktopStateWindow, initial desktopWindowState) *desktopWindowStateStore {
	if path == "" || window == nil {
		return nil
	}
	if !initial.valid() {
		bounds := window.Bounds()
		initial = desktopWindowState{
			Version:   desktopWindowStateVersion,
			X:         bounds.X,
			Y:         bounds.Y,
			Width:     bounds.Width,
			Height:    bounds.Height,
			Maximised: window.IsMaximised(),
		}
	}
	return &desktopWindowStateStore{path: path, window: window, current: initial}
}

func (s *desktopWindowStateStore) schedule() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.captureLocked()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(desktopStateWriteDebounce, func() {
		besteffort.LogError(s.flush())
	})
}

func (s *desktopWindowStateStore) captureLocked() {
	maximised := s.window.IsMaximised()
	s.current.Version = desktopWindowStateVersion
	s.current.Maximised = maximised
	if maximised {
		return
	}
	bounds := s.window.Bounds()
	if bounds.Width >= 1024 && bounds.Height >= 700 {
		s.current.X = bounds.X
		s.current.Y = bounds.Y
		s.current.Width = bounds.Width
		s.current.Height = bounds.Height
	}
}

func (s *desktopWindowStateStore) flush() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.captureLocked()
	return s.writeLocked()
}

func (s *desktopWindowStateStore) writeLocked() error {
	state := s.current
	if !state.valid() {
		return nil
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode desktop window state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create desktop state directory: %w", err)
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write desktop window state: %w", err)
	}
	if err := os.Rename(temporary, s.path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace desktop window state: %w", err)
	}
	return nil
}

func (s *desktopWindowStateStore) close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	return s.writeLocked()
}

func ensureDesktopWindowVisible(window desktopStateWindow, screens []*application.Screen) {
	if window == nil || len(screens) == 0 {
		return
	}
	bounds := window.Bounds()
	for _, screen := range screens {
		if screen != nil && rectanglesIntersect(bounds, screen.WorkArea, desktopVisibleIntersection) {
			return
		}
	}
	primary := screens[0]
	for _, screen := range screens {
		if screen != nil && screen.IsPrimary {
			primary = screen
			break
		}
	}
	if primary == nil {
		return
	}
	x := primary.WorkArea.X + max(0, (primary.WorkArea.Width-bounds.Width)/2)
	y := primary.WorkArea.Y + max(0, (primary.WorkArea.Height-bounds.Height)/2)
	window.SetPosition(x, y)
}

func rectanglesIntersect(left, right application.Rect, minimum int) bool {
	width := min(left.X+left.Width, right.X+right.Width) - max(left.X, right.X)
	height := min(left.Y+left.Height, right.Y+right.Height) - max(left.Y, right.Y)
	return width >= minimum && height >= minimum
}
