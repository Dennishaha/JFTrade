package jftradeapi

import (
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

func buildPluginCompatibility(artifact *strategyPluginArtifact) strategyPluginCompatibility {
	host := currentPluginBuildTuple()
	compatibility := strategyPluginCompatibility{
		Mode:      pluginBuildMode,
		Supported: runtime.GOOS != "windows",
		Host:      host,
	}
	if !compatibility.Supported {
		reason := "go plugin is unsupported on windows hosts"
		compatibility.Reason = &reason
	}
	if artifact == nil {
		return compatibility
	}
	artifactBuild := artifact.Build
	compatibility.Artifact = &artifactBuild
	compatibility.RequiresRebuild = !samePluginBuildTuple(host, artifactBuild)
	if compatibility.RequiresRebuild {
		reason := "artifact build tuple does not match the current jftrade host"
		compatibility.Reason = &reason
	}
	return compatibility
}

func currentPluginBuildTuple() strategyPluginBuildTuple {
	return strategyPluginBuildTuple{
		JFTradeVersion: currentJFTradeVersion(),
		GoVersion:      runtime.Version(),
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		BuildMode:      pluginBuildMode,
	}
}

func currentJFTradeVersion() string {
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		version := strings.TrimSpace(buildInfo.Main.Version)
		if version != "" {
			return version
		}
	}
	return "devel"
}

func samePluginBuildTuple(left strategyPluginBuildTuple, right strategyPluginBuildTuple) bool {
	if left.JFTradeVersion != right.JFTradeVersion || left.GoVersion != right.GoVersion || left.GOOS != right.GOOS || left.GOARCH != right.GOARCH || left.BuildMode != right.BuildMode {
		return false
	}
	if len(left.BuildTags) != len(right.BuildTags) {
		return false
	}
	for index := range left.BuildTags {
		if left.BuildTags[index] != right.BuildTags[index] {
			return false
		}
	}
	return true
}

func buildPluginOperationID(pluginID string) string {
	return strings.ToLower(strings.ReplaceAll(pluginID, " ", "-")) + "-" + time.Now().UTC().Format("20060102150405.000000000")
}

func buildPluginUninstallGuidance(pluginID string, installPath string) strategyPluginUninstallGuidance {
	guidance := strategyPluginUninstallGuidance{
		PluginID: pluginID,
		Path:     installPath,
		Exists:   false,
	}
	guidance.Commands.Posix = "rm -f " + shellQuote(installPath)
	guidance.Commands.PowerShell = "Remove-Item -LiteralPath '" + strings.ReplaceAll(installPath, "'", "''") + "' -Force"
	return guidance
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
