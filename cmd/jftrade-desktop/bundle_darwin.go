package main

import "github.com/wailsapp/wails/v3/pkg/mac"

func macBundleIdentifier() string {
	return mac.GetBundleID()
}
