package marketdataapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	pythonRuntimeDependencyID             = "python"
	pythonRuntimeDependencyDisplayName    = "Python"
	pythonRuntimeDependencyMinimum        = "3.11.0"
	pythonRuntimeDependencyHomepage       = "https://www.python.org/downloads/"
	pythonRuntimeDependencyStatusOK       = "ok"
	pythonRuntimeDependencyStatusMissing  = "missing"
	pythonRuntimeDependencyStatusOutdated = "outdated"
	pythonRuntimeDependencyStatusError    = "error"
)

var (
	pythonRuntimeDependencyCheckTimeout = 3 * time.Second
	pythonRuntimeDependencyProbe        = ProbePythonRuntime
)

// CheckPythonRuntimeDependency reports the Python runtime selected by the
// authoritative market-data sidecar resolver.
func CheckPythonRuntimeDependency(ctx context.Context) map[string]any {
	resolution := ResolvePythonRuntime()
	result := basePythonRuntimeDependency(resolution)
	if resolution.Mode != PythonRuntimeModeSource {
		return checkManagedPythonRuntimeDependency(result, resolution)
	}
	if !resolution.Available || resolution.ResolvedPath == "" {
		result["status"] = pythonRuntimeDependencyStatusMissing
		result["message"] = pythonRuntimeMissingMessage(resolution)
		return result
	}
	result["resolvedPath"] = resolution.ResolvedPath
	checkCtx, cancel := context.WithTimeout(ctx, pythonRuntimeDependencyCheckTimeout)
	probe := pythonRuntimeDependencyProbe(checkCtx, resolution)
	timedOut := errors.Is(checkCtx.Err(), context.DeadlineExceeded)
	cancel()
	if probe.DetectedVersion != "" {
		result["detectedVersion"] = probe.DetectedVersion
	}
	if timedOut {
		result["status"] = pythonRuntimeDependencyStatusError
		result["message"] = "Python runtime check timed out."
		return result
	}
	if probe.Err != nil {
		result["status"] = pythonRuntimeDependencyStatusError
		result["message"] = "Python runtime check failed: " + summarizePythonRuntimeCommandError(probe.Err, probe.Output)
		return result
	}
	if probe.Outdated {
		result["status"] = pythonRuntimeDependencyStatusOutdated
		result["message"] = fmt.Sprintf(
			"Python %s is below the required %s.", probe.DetectedVersion, pythonRuntimeDependencyMinimum,
		)
		return result
	}
	if len(probe.MissingModules) > 0 {
		result["status"] = pythonRuntimeDependencyStatusError
		result["message"] = "Python is missing required market-data runtime modules: " + strings.Join(probe.MissingModules, ",")
		return result
	}
	result["status"] = pythonRuntimeDependencyStatusOK
	result["message"] = fmt.Sprintf("Python %s and the market-data runtime modules are available.", probe.DetectedVersion)
	return result
}

func basePythonRuntimeDependency(resolution PythonRuntimeResolution) map[string]any {
	return map[string]any{
		"id": pythonRuntimeDependencyID, "displayName": pythonRuntimeDependencyDisplayName,
		"required": resolution.Required, "configurable": resolution.Configurable,
		"status": pythonRuntimeDependencyStatusError, "minimumVersion": pythonRuntimeDependencyMinimum,
		"detectedVersion": "", "configuredPath": "",
		"effectivePath": resolution.EffectivePath, "resolvedPath": resolution.ResolvedPath,
		"attemptedPaths": resolution.AttemptedPaths, "source": resolution.Source,
		"homepageUrl": pythonRuntimeDependencyHomepage, "message": "",
	}
}

func checkManagedPythonRuntimeDependency(
	result map[string]any,
	resolution PythonRuntimeResolution,
) map[string]any {
	if !resolution.Available {
		result["status"] = pythonRuntimeDependencyStatusError
		result["message"] = fmt.Sprintf("The bundled Python runtime is unavailable: %v", resolution.ResolutionError)
		return result
	}
	result["status"] = pythonRuntimeDependencyStatusOK
	if resolution.Mode == PythonRuntimeModeExternalHelper {
		result["message"] = "Python is supplied by the configured frozen market-data helper."
		return result
	}
	result["message"] = "Python is supplied by the bundled market-data helper."
	return result
}

func pythonRuntimeMissingMessage(resolution PythonRuntimeResolution) string {
	return fmt.Sprintf(
		"Python was not found for the market-data source runtime: %v. Tried: %s. Set %s or create workers/marketdata-sidecar/.venv with Python 3.11+.",
		resolution.ResolutionError, strings.Join(resolution.AttemptedPaths, ", "), EnvMarketDataDevPython,
	)
}

func summarizePythonRuntimeCommandError(err error, output string) string {
	text := strings.TrimSpace(output)
	if text == "" {
		return err.Error()
	}
	if len(text) > 500 {
		text = text[len(text)-500:]
	}
	return strings.TrimSpace(err.Error() + ": " + text)
}
