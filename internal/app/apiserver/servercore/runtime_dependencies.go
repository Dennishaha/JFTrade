package servercore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	"github.com/jftrade/jftrade-main/pkg/jftsettings"
)

const (
	runtimeDependencyNodeID            = "node"
	runtimeDependencyNodeDisplayName   = "Node.js"
	runtimeDependencyNodeMinimum       = "22.0.0"
	runtimeDependencyNodeHomepage      = "https://nodejs.org/"
	runtimeDependencyNodeCheckTimeout  = 2 * time.Second
	runtimeDependencyStatusOK          = "ok"
	runtimeDependencyStatusMissing     = "missing"
	runtimeDependencyStatusOutdated    = "outdated"
	runtimeDependencyStatusError       = "error"
	runtimeDependencySourceSettings    = "settings"
	runtimeDependencySourceDefaultPath = "path"
)

var (
	runtimeDependencyLookPath = exec.LookPath
	runtimeDependencyOutput   = func(ctx context.Context, path string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, path, args...).CombinedOutput()
	}
)

type dependencySemanticVersion struct {
	major int
	minor int
	patch int
}

func (s *Server) runtimeDependencies(ctx context.Context) map[string]any {
	node := checkNodeRuntimeDependency(ctx, s.pineWorkerSettings())
	dependencies := []map[string]any{node}
	allRequiredSatisfied := true
	for _, dependency := range dependencies {
		required, _ := dependency["required"].(bool)
		status, _ := dependency["status"].(string)
		if required && status != runtimeDependencyStatusOK {
			allRequiredSatisfied = false
			break
		}
	}
	return map[string]any{
		"checkedAt":            time.Now().UTC().Format(time.RFC3339Nano),
		"allRequiredSatisfied": allRequiredSatisfied,
		"dependencies":         dependencies,
	}
}

func checkNodeRuntimeDependency(ctx context.Context, settings jftsettings.PineWorkerSettings) map[string]any {
	if ctx == nil {
		ctx = context.Background()
	}
	normalized := settings
	normalized.NodeBinaryPath = settingsfile.NormalizeNodeBinaryPath(normalized.NodeBinaryPath)
	effectivePath, source := resolveNodeDependencyRuntimePath(normalized)
	result := baseNodeRuntimeDependency(normalized.NodeBinaryPath, effectivePath, source)

	resolvedPath, err := runtimeDependencyLookPath(effectivePath)
	if err != nil {
		result["status"] = runtimeDependencyStatusMissing
		result["message"] = nodeMissingMessage(normalized.NodeBinaryPath, err)
		return result
	}
	result["resolvedPath"] = resolvedPath

	checkCtx, cancel := context.WithTimeout(ctx, runtimeDependencyNodeCheckTimeout)
	defer cancel()
	output, err := runtimeDependencyOutput(checkCtx, resolvedPath, "--version")
	if err != nil {
		result["status"] = runtimeDependencyStatusError
		if errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
			result["message"] = "Node.js version check timed out."
		} else {
			result["message"] = fmt.Sprintf("Node.js version check failed: %s", summarizeDependencyCommandError(err, output))
		}
		return result
	}

	versionText := strings.TrimSpace(string(output))
	version, err := parseDependencyNodeVersion(versionText)
	if err != nil {
		result["status"] = runtimeDependencyStatusError
		result["detectedVersion"] = versionText
		result["message"] = fmt.Sprintf("Node.js returned an unrecognized version: %s", versionText)
		return result
	}
	result["detectedVersion"] = version.String()
	minimum := dependencySemanticVersion{major: 22}
	if compareDependencySemanticVersion(version, minimum) < 0 {
		result["status"] = runtimeDependencyStatusOutdated
		result["message"] = fmt.Sprintf("Node.js %s is below the required %s.", version.String(), runtimeDependencyNodeMinimum)
		return result
	}

	result["status"] = runtimeDependencyStatusOK
	result["message"] = fmt.Sprintf("Node.js %s is available.", version.String())
	return result
}

func baseNodeRuntimeDependency(configuredPath string, effectivePath string, source string) map[string]any {
	return map[string]any{
		"id":              runtimeDependencyNodeID,
		"displayName":     runtimeDependencyNodeDisplayName,
		"required":        true,
		"status":          runtimeDependencyStatusError,
		"minimumVersion":  runtimeDependencyNodeMinimum,
		"detectedVersion": "",
		"configuredPath":  configuredPath,
		"effectivePath":   effectivePath,
		"resolvedPath":    "",
		"source":          source,
		"homepageUrl":     runtimeDependencyNodeHomepage,
		"message":         "",
	}
}

func resolveNodeDependencyRuntimePath(settings jftsettings.PineWorkerSettings) (string, string) {
	if value := settingsfile.NormalizeNodeBinaryPath(settings.NodeBinaryPath); value != "" {
		return value, runtimeDependencySourceSettings
	}
	if value := settingsfile.NormalizeNodeBinaryPath(envValue(envPineWorkerRuntime)); value != "" {
		return value, "env:" + envPineWorkerRuntime
	}
	if value := settingsfile.NormalizeNodeBinaryPath(envValue("JFTRADE_NODE_BINARY")); value != "" {
		return value, "env:JFTRADE_NODE_BINARY"
	}
	return "node", runtimeDependencySourceDefaultPath
}

func envValue(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func nodeMissingMessage(configuredPath string, err error) string {
	if strings.TrimSpace(configuredPath) != "" {
		return fmt.Sprintf("Configured Node.js binary was not found or is not executable: %v", err)
	}
	return fmt.Sprintf("Node.js was not found on PATH: %v", err)
}

func summarizeDependencyCommandError(err error, output []byte) string {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return err.Error()
	}
	if len(text) > 500 {
		text = text[len(text)-500:]
	}
	return strings.TrimSpace(err.Error() + ": " + text)
}

func parseDependencyNodeVersion(raw string) (dependencySemanticVersion, error) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return dependencySemanticVersion{}, fmt.Errorf("empty version")
	}
	version := strings.TrimPrefix(fields[0], "v")
	parts := strings.Split(version, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return dependencySemanticVersion{}, fmt.Errorf("invalid version %q", raw)
	}
	numbers := []int{0, 0, 0}
	for index, part := range parts {
		if part == "" {
			return dependencySemanticVersion{}, fmt.Errorf("invalid version %q", raw)
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return dependencySemanticVersion{}, fmt.Errorf("invalid version %q", raw)
		}
		numbers[index] = value
	}
	return dependencySemanticVersion{major: numbers[0], minor: numbers[1], patch: numbers[2]}, nil
}

func compareDependencySemanticVersion(left dependencySemanticVersion, right dependencySemanticVersion) int {
	switch {
	case left.major != right.major:
		return left.major - right.major
	case left.minor != right.minor:
		return left.minor - right.minor
	default:
		return left.patch - right.patch
	}
}

func (version dependencySemanticVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", version.major, version.minor, version.patch)
}
