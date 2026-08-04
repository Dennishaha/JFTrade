package marketdataapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/internal/marketdataassets"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
)

const (
	EnvMarketDataSidecar       = "JFTRADE_MARKETDATA_SIDECAR"
	EnvMarketDataDevPython     = "JFTRADE_MARKETDATA_DEV_PYTHON"
	EnvMarketDataDevPythonPath = "JFTRADE_MARKETDATA_DEV_PYTHONPATH"

	// Deprecated yfinance-specific environment variables remain lower-priority
	// aliases so existing development and test environments keep working.
	EnvYFinanceSidecar       = "JFTRADE_YFINANCE_SIDECAR"
	EnvYFinanceDevPython     = "JFTRADE_YFINANCE_DEV_PYTHON"
	EnvYFinanceDevPythonPath = "JFTRADE_YFINANCE_DEV_PYTHONPATH"

	PythonRuntimeModeEmbedded       = "embedded"
	PythonRuntimeModeExternalHelper = "external-helper"
	PythonRuntimeModeSource         = "source"
)

var (
	pythonRuntimeLookPath    = exec.LookPath
	pythonRuntimeStat        = os.Stat
	pythonRuntimeWorkingDir  = os.Getwd
	pythonRuntimeGOOS        = runtime.GOOS
	selectMarketDataAsset    = marketdataassets.Select
	pythonRuntimeProbeOutput = func(
		ctx context.Context, path string, sourcePath string, args ...string,
	) ([]byte, error) {
		command := exec.CommandContext(ctx, path, args...)
		command.Env = append(os.Environ(), "PYTHONPATH="+sourcePath)
		return command.CombinedOutput()
	}
)

// PythonRuntimeResolution describes the Python provider selected by the same
// rules used to launch the market-data sidecar.
type PythonRuntimeResolution struct {
	Mode            string
	Required        bool
	Configurable    bool
	Available       bool
	EffectivePath   string
	ResolvedPath    string
	Source          string
	SourcePath      string
	AttemptedPaths  []string
	ResolutionError error
}

type pythonRuntimeCandidate struct {
	path   string
	source string
}

// PythonRuntimeProbeResult is the shared source-runtime validation result used
// by dependency status and sidecar startup.
type PythonRuntimeProbeResult struct {
	Available       bool
	DetectedVersion string
	MissingModules  []string
	Outdated        bool
	Output          string
	Err             error
}

type pythonRuntimeProbePayload struct {
	Version []int    `json:"version"`
	Missing []string `json:"missing"`
}

// ResolvePythonRuntime selects embedded/helper Python for frozen runtimes and
// a host interpreter for source development. It does not execute Python.
func ResolvePythonRuntime() PythonRuntimeResolution {
	if !marketdataassets.DevelopmentOverridesAllowed() {
		return resolveEmbeddedPythonRuntime()
	}
	if helper, source := environmentOverride(EnvMarketDataSidecar, EnvYFinanceSidecar); helper != "" {
		return resolveExternalHelperPythonRuntime(helper, source)
	}
	return resolveSourcePythonRuntime()
}

// ProbePythonRuntime checks Python 3.11+ and only the shared source-sidecar
// bootstrap modules. Provider-specific dependencies remain isolated behind
// their own lazy health checks, so one failed import cannot block the process.
func ProbePythonRuntime(
	ctx context.Context,
	resolution PythonRuntimeResolution,
) PythonRuntimeProbeResult {
	if resolution.Mode != PythonRuntimeModeSource {
		return PythonRuntimeProbeResult{Available: resolution.Available, Err: resolution.ResolutionError}
	}
	if !resolution.Available || resolution.ResolvedPath == "" {
		return PythonRuntimeProbeResult{Err: pythonRuntimeMissingError(resolution)}
	}
	const script = "import importlib.util,json,sys;required=('marketdata_sidecar','fastapi','uvicorn');missing=[name for name in required if importlib.util.find_spec(name) is None];print(json.dumps({'version':list(sys.version_info[:3]),'missing':missing}))"
	output, err := pythonRuntimeProbeOutput(
		ctx, resolution.ResolvedPath, resolution.SourcePath, "-c", script,
	)
	result := PythonRuntimeProbeResult{Output: strings.TrimSpace(string(output)), Err: err}
	if err != nil {
		return result
	}
	payload := pythonRuntimeProbePayload{}
	if err := json.Unmarshal(output, &payload); err != nil {
		result.Err = fmt.Errorf("parse Python runtime probe: %w", err)
		return result
	}
	if len(payload.Version) < 2 {
		result.Err = fmt.Errorf("python runtime probe returned an invalid version")
		return result
	}
	version := append(payload.Version, 0)
	result.DetectedVersion = strings.Join([]string{
		strconv.Itoa(version[0]), strconv.Itoa(version[1]), strconv.Itoa(version[2]),
	}, ".")
	result.MissingModules = append([]string(nil), payload.Missing...)
	result.Outdated = version[0] < 3 || version[0] == 3 && version[1] < 11
	result.Available = !result.Outdated && len(result.MissingModules) == 0
	return result
}

func resolveEmbeddedPythonRuntime() PythonRuntimeResolution {
	resolution := PythonRuntimeResolution{
		Mode: PythonRuntimeModeEmbedded, Source: "bundled",
	}
	_, available, err := selectMarketDataAsset()
	resolution.Available = available && err == nil
	resolution.ResolutionError = err
	if err == nil && !available {
		resolution.ResolutionError = ErrMarketDataSidecarUnavailable
	}
	return resolution
}

func resolveExternalHelperPythonRuntime(helper string, source string) PythonRuntimeResolution {
	resolution := PythonRuntimeResolution{
		Mode: PythonRuntimeModeExternalHelper, Source: "env:" + source,
		EffectivePath: helper,
	}
	path, err := validateAbsoluteRegularFile(helper, source)
	resolution.ResolvedPath = path
	resolution.Available = err == nil
	resolution.ResolutionError = err
	return resolution
}

func resolveSourcePythonRuntime() PythonRuntimeResolution {
	sourcePath, sourceErr := resolveMarketDataSourcePath()
	resolution := PythonRuntimeResolution{
		Mode: PythonRuntimeModeSource, Required: true, Configurable: false,
		SourcePath:      sourcePath,
		ResolutionError: sourceErr,
	}
	candidates := sourcePythonRuntimeCandidates(sourcePath)
	if len(candidates) == 0 {
		candidates = []pythonRuntimeCandidate{{path: "python3", source: "path"}}
	}
	resolution.EffectivePath = candidates[0].path
	resolution.Source = candidates[0].source
	for _, candidate := range candidates {
		resolution.AttemptedPaths = append(resolution.AttemptedPaths, candidate.path)
		resolved, err := pythonRuntimeLookPath(candidate.path)
		if err != nil {
			resolution.ResolutionError = errors.Join(resolution.ResolutionError, err)
			continue
		}
		resolution.EffectivePath = candidate.path
		resolution.Source = candidate.source
		resolution.ResolvedPath = resolved
		resolution.Available = sourceErr == nil
		return resolution
	}
	return resolution
}

func sourcePythonRuntimeCandidates(sourcePath string) []pythonRuntimeCandidate {
	if configured, source := environmentOverride(EnvMarketDataDevPython, EnvYFinanceDevPython); configured != "" {
		value := settingsfile.NormalizeExecutablePath(configured)
		return []pythonRuntimeCandidate{{path: value, source: "env:" + source}}
	}
	candidates := make([]pythonRuntimeCandidate, 0, 6)
	if sourcePath != "" {
		venvBinary := "python"
		venvDir := "bin"
		if pythonRuntimeGOOS == "windows" {
			venvBinary = "python.exe"
			venvDir = "Scripts"
		}
		candidates = append(candidates, pythonRuntimeCandidate{
			path:   filepath.Join(filepath.Dir(sourcePath), ".venv", venvDir, venvBinary),
			source: "workspace-venv",
		})
	}
	candidates = append(candidates,
		pythonRuntimeCandidate{path: "python3", source: "path"},
		pythonRuntimeCandidate{path: "python", source: "path"},
	)
	if pythonRuntimeGOOS == "darwin" {
		candidates = append(candidates,
			pythonRuntimeCandidate{path: "/opt/homebrew/bin/python3", source: "common:/opt/homebrew/bin/python3"},
			pythonRuntimeCandidate{path: "/usr/local/bin/python3", source: "common:/usr/local/bin/python3"},
		)
	}
	return candidates
}

func resolveMarketDataSourcePath() (string, error) {
	if configured, source := environmentOverride(EnvMarketDataDevPythonPath, EnvYFinanceDevPythonPath); configured != "" {
		if !filepath.IsAbs(configured) {
			return configured, fmt.Errorf("%s must be an absolute path", source)
		}
		return validateDirectory(configured, source)
	}
	workingDir, err := pythonRuntimeWorkingDir()
	if err != nil {
		return "", fmt.Errorf("resolve market-data workspace root: %w", err)
	}
	for current := workingDir; ; current = filepath.Dir(current) {
		candidate := filepath.Join(current, "workers", "marketdata-sidecar", "src")
		if path, candidateErr := validateDirectory(candidate, "market-data source path"); candidateErr == nil {
			return path, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return "", fmt.Errorf("market-data source path was not found from %s", workingDir)
}

func environmentOverride(primary string, legacy string) (string, string) {
	if value := strings.TrimSpace(os.Getenv(primary)); value != "" {
		return value, primary
	}
	if value := strings.TrimSpace(os.Getenv(legacy)); value != "" {
		return value, legacy
	}
	return "", primary
}

func validateDirectory(value string, name string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return value, fmt.Errorf("resolve %s: %w", name, err)
	}
	info, err := pythonRuntimeStat(absolute)
	if err != nil {
		return absolute, fmt.Errorf("inspect %s: %w", name, err)
	}
	if !info.IsDir() {
		return absolute, fmt.Errorf("%s must name a directory", name)
	}
	return absolute, nil
}

func pythonRuntimeMissingError(resolution PythonRuntimeResolution) error {
	if resolution.ResolutionError != nil {
		return resolution.ResolutionError
	}
	return fs.ErrNotExist
}
