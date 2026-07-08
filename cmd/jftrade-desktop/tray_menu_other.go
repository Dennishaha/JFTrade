//go:build !darwin

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func showDesktopTrayMenu(systemTray *application.SystemTray) {
	if systemTray != nil {
		systemTray.OpenMenu()
	}
}
