//go:build darwin

package main

import (
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func showDesktopTrayMenu(systemTray *application.SystemTray) {
	if systemTray != nil {
		time.AfterFunc(20*time.Millisecond, systemTray.OpenMenu)
	}
}
