package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/jftrade/jftrade-main/internal/buildinfo"
)

func configureDesktopApplicationMenu(app *application.App, window application.Window, linkService *DesktopLinkService, updateService *DesktopUpdateService, state *desktopAppState, profile desktopBuildProfile) *application.Menu {
	menu := app.NewMenu()
	if runtime.GOOS == "darwin" {
		menu.AddRole(application.AppMenu)
	}

	applicationMenu := menu.AddSubmenu(profile.ApplicationName)
	applicationMenu.Add("打开主窗口").SetAccelerator("CmdOrCtrl+1").OnClick(func(*application.Context) {
		restoreDesktopMainWindow(state)
	})
	applicationMenu.Add("设置…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) {
		openDesktopSettings(window)
	})
	applicationMenu.Add("查看日志").SetAccelerator("CmdOrCtrl+Shift+L").OnClick(func(*application.Context) {
		if linkService != nil && linkService.app != nil {
			openDesktopLogWindow(linkService.app, profile.ApplicationName)
		}
	})
	if profile.UpdateChecksEnabled {
		applicationMenu.Add("检查更新…").OnClick(func(*application.Context) {
			checkDesktopUpdateInteractively(window, app, updateService)
		})
	}
	applicationMenu.AddSeparator()
	applicationMenu.Add("退出").SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) {
		state.quit(app)
	})

	menu.AddRole(application.EditMenu)
	menu.AddRole(application.WindowMenu)
	helpMenu := menu.AddSubmenu("帮助")
	helpMenu.Add("文档").OnClick(func(*application.Context) {
		if linkService != nil {
			linkService.openDocsWindow(desktopDocsURL)
		}
	})
	helpMenu.Add("关于 / 版本").OnClick(func(*application.Context) {
		if window != nil {
			window.Info("%s", desktopAboutText(profile))
		}
	})

	app.Menu.Set(menu)
	return menu
}

func desktopAboutText(profile desktopBuildProfile) string {
	build := buildinfo.Snapshot()
	return fmt.Sprintf(
		"%s\n版本: %s\n提交: %s\n构建时间: %s\n运行平台: %s/%s",
		profile.ApplicationName,
		strings.TrimSpace(fmt.Sprint(build["version"])),
		strings.TrimSpace(fmt.Sprint(build["commit"])),
		strings.TrimSpace(fmt.Sprint(build["buildTime"])),
		strings.TrimSpace(fmt.Sprint(build["goos"])),
		strings.TrimSpace(fmt.Sprint(build["goarch"])),
	)
}

func openDesktopSettings(window application.Window) {
	if window == nil {
		return
	}
	window.SetURL(desktopSettingsURL)
	window.Show().Focus()
}
