//go:build !release_assets

package main

func currentDesktopBuildProfile() desktopBuildProfile {
	return developmentDesktopBuildProfile()
}
