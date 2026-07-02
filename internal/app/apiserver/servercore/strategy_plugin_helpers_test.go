package servercore

import (
	"strings"
	"testing"
)

func TestStrategyPluginCompatibilityDetectsHostBuildDrift(t *testing.T) {
	host := currentPluginBuildTuple()
	compatible := buildPluginCompatibility(&strategyPluginArtifact{Path: "plugin.so", Build: host})
	if !compatible.Supported || compatible.RequiresRebuild || compatible.Artifact == nil {
		t.Fatalf("compatible artifact = %#v", compatible)
	}

	stale := host
	stale.GoVersion = "go0.0"
	drifted := buildPluginCompatibility(&strategyPluginArtifact{Path: "plugin.so", Build: stale})
	if !drifted.RequiresRebuild || drifted.Reason == nil || !strings.Contains(*drifted.Reason, "does not match") {
		t.Fatalf("drifted artifact = %#v", drifted)
	}

	if samePluginBuildTuple(host, strategyPluginBuildTuple{JFTradeVersion: host.JFTradeVersion}) {
		t.Fatal("partial build tuple should not match current host")
	}
	withTags := host
	withTags.BuildTags = []string{"netgo", "sqlite"}
	sameTags := withTags
	if !samePluginBuildTuple(withTags, sameTags) {
		t.Fatal("identical build tags should match")
	}
	sameTags.BuildTags = []string{"sqlite", "netgo"}
	if samePluginBuildTuple(withTags, sameTags) {
		t.Fatal("build tag order drift should require rebuild")
	}
}

func TestStrategyPluginOperationAndUninstallGuidanceAreShellSafe(t *testing.T) {
	operationID := buildPluginOperationID("My Plugin")
	if !strings.HasPrefix(operationID, "my-plugin-") || strings.Contains(operationID, " ") {
		t.Fatalf("operation ID = %q", operationID)
	}

	guidance := buildPluginUninstallGuidance("plugin-1", "/tmp/jftrade/O'Brien/plugin.so")
	if guidance.PluginID != "plugin-1" || guidance.Exists {
		t.Fatalf("guidance identity = %#v", guidance)
	}
	if !strings.Contains(guidance.Commands.Posix, "'\\''") {
		t.Fatalf("posix uninstall command is not safely quoted: %q", guidance.Commands.Posix)
	}
	if !strings.Contains(guidance.Commands.PowerShell, "O''Brien") {
		t.Fatalf("powershell uninstall command is not safely quoted: %q", guidance.Commands.PowerShell)
	}
}
