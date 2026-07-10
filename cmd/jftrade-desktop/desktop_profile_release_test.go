//go:build release_assets

package main

import (
	"path/filepath"
	"testing"
)

func TestReleaseDesktopBuildProfile(t *testing.T) {
	profile := currentDesktopBuildProfile()
	if profile.Channel != "release" || !profile.Release || !profile.UpdateChecksEnabled {
		t.Fatalf("release profile = %#v", profile)
	}
	if profile.ApplicationName != "JFTrade" || profile.ProductIdentifier != "com.jftrade.desktop" {
		t.Fatalf("release identity = %#v", profile)
	}
	if profile.SingleInstanceID != "com.jftrade.desktop" || profile.DefaultAPIBind != "127.0.0.1:6699" {
		t.Fatalf("release isolation = %#v", profile)
	}
	defaults, err := profile.launchDefaults()
	if err != nil {
		t.Fatalf("launchDefaults: %v", err)
	}
	if filepath.Base(filepath.Dir(defaults.SettingsPath)) != "JFTrade" || filepath.Dir(defaults.SettingsPath) != filepath.Dir(defaults.BacktestDBPath) {
		t.Fatalf("release paths = %#v", defaults)
	}
}
