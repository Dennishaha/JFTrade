//go:build !darwin

package main

var desktopDockIconHider = hideDesktopDockIcon

func hideDesktopDockIcon() {
}
