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

	"github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	"github.com/jftrade/jftrade-main/internal/yfinanceassets"
)

const (
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
	selectYFinanceAsset      = yfinanceassets.Select
	pythonRuntimeProbeOutput = func(
		ctx context.Context, path string, sourcePath string, args ...string,
	) ([]byte, error) {
		command := exec.CommandContext(ctx, path, args...)
		command.Env = append(os.Environ(), "PYTHONPATH="+sourcePath)
		return command.CombinedOutput()
	}
)

// PythonRuntimeResolution describes the Python provider selected by the same
// rules used to launch the yfinance sidecar.
type PythonRuntimeResolution struct {
	Mode            string
	Required        bool
	Configurable    bool
	Available       bool
	ConfiguredPath  string
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
func ResolvePythonRuntime(settings jftsettings.RuntimeDependencySettings) PythonRuntimeResolution {
	configured := settingsfile.NormalizeExecutablePath(settings.PythonBinaryPath)
	if !yfinanceassets.DevelopmentOverridesAllowed() {
		return resolveEmbeddedPythonRuntime(configured)
	}
	if helper := strings.TrimSpace(os.Getenv(EnvYFinanceSidecar)); helper != "" {
		return resolveExternalHelperPythonRuntime(configured, helper)
	}
	return resolveSourcePythonRuntime(configured)
}

// ProbePythonRuntime checks Python 3.11+ and the source sidecar modules without
// importing heavy dependencies, so it does not defeat helper background warmup.
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
	const script = "import importlib.util,json,sys;required=('yfinance_sidecar','fastapi','uvicorn','yfinance','curl_cffi');missing=[name for name in required if importlib.util.find_spec(name) is None];print(json.dumps({'version':list(sys.version_info[:3]),'missing':missing}))"
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

func resolveEmbeddedPythonRuntime(configured string) PythonRuntimeResolution {
	resolution := PythonRuntimeResolution{
		Mode: PythonRuntimeModeEmbedded, Source: "bundled",
		ConfiguredPath: configured,
	}
	_, available, err := selectYFinanceAsset()
	resolution.Available = available && err == nil
	resolution.ResolutionError = err
	if err == nil && !available {
		resolution.ResolutionError = ErrYFinanceSidecarUnavailable
	}
	return resolution
}

func resolveExternalHelperPythonRuntime(configured string, helper string) PythonRuntimeResolution {
	resolution := PythonRuntimeResolution{
		Mode: PythonRuntimeModeExternalHelper, Source: "external-helper",
		ConfiguredPath: configured, EffectivePath: helper,
	}
	path, err := validateAbsoluteRegularFile(helper, EnvYFinanceSidecar)
	resolution.ResolvedPath = path
	resolution.Available = err == nil
	resolution.ResolutionError = err
	return resolution
}

func resolveSourcePythonRuntime(configured string) PythonRuntimeResolution {
	sourcePath, sourceErr := resolveYFinanceSourcePath()
	resolution := PythonRuntimeResolution{
		Mode: PythonRuntimeModeSource, Required: true, Configurable: true,
		ConfiguredPath: configured, SourcePath: sourcePath,
		ResolutionError: sourceErr,
	}
	candidates := sourcePythonRuntimeCandidates(configured, sourcePath)
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

func sourcePythonRuntimeCandidates(configured string, sourcePath string) []pythonRuntimeCandidate {
	if value := settingsfile.NormalizeExecutablePath(os.Getenv(EnvYFinanceDevPython)); value != "" {
		return []pythonRuntimeCandidate{{path: value, source: "env:" + EnvYFinanceDevPython}}
	}
	if configured != "" {
		return []pythonRuntimeCandidate{{path: configured, source: "settings"}}
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

func resolveYFinanceSourcePath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv(EnvYFinanceDevPythonPath)); configured != "" {
		if !filepath.IsAbs(configured) {
			return configured, fmt.Errorf("%s must be an absolute path", EnvYFinanceDevPythonPath)
		}
		return validateDirectory(configured, EnvYFinanceDevPythonPath)
	}
	workingDir, err := pythonRuntimeWorkingDir()
	if err != nil {
		return "", fmt.Errorf("resolve yfinance workspace root: %w", err)
	}
	for current := workingDir; ; current = filepath.Dir(current) {
		candidate := filepath.Join(current, "workers", "yfinance-sidecar", "src")
		if path, candidateErr := validateDirectory(candidate, "yfinance source path"); candidateErr == nil {
			return path, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return "", fmt.Errorf("yfinance source path was not found from %s", workingDir)
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
