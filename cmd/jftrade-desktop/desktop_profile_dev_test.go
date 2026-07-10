//go:build !release_assets

package main

import (
	"path/filepath"
	"testing"
)

func TestDevelopmentDesktopBuildProfile(t *testing.T) {
	profile := currentDesktopBuildProfile()
	if profile.Channel != "dev" || profile.Release {
		t.Fatalf("development profile = %#v", profile)
	}
	if profile.ApplicationName != "JFTrade Dev" || profile.ProductIdentifier != "com.jftrade.desktop.dev" {
		t.Fatalf("development identity = %#v", profile)
	}
	if profile.SingleInstanceID != "com.jftrade.desktop.dev" || profile.DefaultAPIBind != desktopDevelopmentAPIBind {
		t.Fatalf("development isolation = %#v", profile)
	}
	defaults, err := profile.launchDefaults()
	if err != nil {
		t.Fatalf("launchDefaults: %v", err)
	}
	if defaults.SettingsPath != filepath.Join("var", "jftrade-api", "settings.json") || defaults.BacktestDBPath != filepath.Join("var", "jftrade-api", "backtest.db") {
		t.Fatalf("development paths = %#v", defaults)
	}
}
