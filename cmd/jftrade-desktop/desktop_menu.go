package main

import "github.com/wailsapp/wails/v3/pkg/application"

func openDesktopSettings(window application.Window) {
	if window == nil {
		return
	}
	window.SetURL(desktopSettingsURL)
	window.Show().Focus()
}
