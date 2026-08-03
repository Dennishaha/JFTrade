package marketdataapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckPythonRuntimeDependencyValidatesVersionAndModules(t *testing.T) {
	python := configureDependencyTestPythonSourceRuntime(t)
	tests := []struct {
		name       string
		probe      PythonRuntimeProbeResult
		wantStatus string
		wantText   string
	}{
		{name: "available", probe: PythonRuntimeProbeResult{Available: true, DetectedVersion: "3.11.9"}, wantStatus: pythonRuntimeDependencyStatusOK, wantText: "runtime modules"},
		{name: "outdated", probe: PythonRuntimeProbeResult{DetectedVersion: "3.10.14", Outdated: true}, wantStatus: pythonRuntimeDependencyStatusOutdated, wantText: "below"},
		{name: "invalid", probe: PythonRuntimeProbeResult{Err: errors.New("invalid probe output"), Output: "PyPy unknown"}, wantStatus: pythonRuntimeDependencyStatusError, wantText: "check failed"},
		{name: "missing modules", probe: PythonRuntimeProbeResult{DetectedVersion: "3.12.1", MissingModules: []string{"yfinance", "curl_cffi"}}, wantStatus: pythonRuntimeDependencyStatusError, wantText: "yfinance,curl_cffi"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			restorePythonRuntimeDependencyProbe(t, func(_ context.Context, resolution PythonRuntimeResolution) PythonRuntimeProbeResult {
				if resolution.ResolvedPath != python || resolution.SourcePath == "" {
					t.Fatalf("probe resolution=%#v", resolution)
				}
				return test.probe
			})
			result := CheckPythonRuntimeDependency(context.Background())
			if result["status"] != test.wantStatus || !strings.Contains(result["message"].(string), test.wantText) {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestCheckPythonRuntimeDependencyReportsMissingSourceRuntime(t *testing.T) {
	t.Setenv(EnvYFinanceSidecar, "")
	t.Setenv(EnvYFinanceDevPython, "")
	t.Setenv(EnvYFinanceDevPythonPath, t.TempDir())

	restorePythonRuntimeLookPath(t, func(string) (string, error) {
		return "", os.ErrNotExist
	})
	result := CheckPythonRuntimeDependency(context.Background())
	if result["status"] != pythonRuntimeDependencyStatusMissing || result["configurable"] != false || result["required"] != true {
		t.Fatalf("automatic result = %#v", result)
	}
	message := result["message"].(string)
	if !strings.Contains(message, "Tried:") || !strings.Contains(message, EnvYFinanceDevPython) || !strings.Contains(message, ".venv") {
		t.Fatalf("automatic result = %#v", result)
	}
}

func TestCheckPythonRuntimeDependencyReportsManagedHelpers(t *testing.T) {
	helper := filepath.Join(t.TempDir(), "yfinance-helper")
	if err := os.WriteFile(helper, []byte("helper"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvYFinanceSidecar, helper)
	result := CheckPythonRuntimeDependency(context.Background())
	if result["status"] != pythonRuntimeDependencyStatusOK || result["required"] != false ||
		!strings.Contains(result["message"].(string), "configured frozen") {
		t.Fatalf("external helper result = %#v", result)
	}

	missingHelper := filepath.Join(t.TempDir(), "missing-helper")
	t.Setenv(EnvYFinanceSidecar, missingHelper)
	result = CheckPythonRuntimeDependency(context.Background())
	if result["status"] != pythonRuntimeDependencyStatusError || !strings.Contains(result["message"].(string), "unavailable") {
		t.Fatalf("missing helper result = %#v", result)
	}

	embedded := PythonRuntimeResolution{Mode: PythonRuntimeModeEmbedded, Available: true, Source: "bundled"}
	result = checkManagedPythonRuntimeDependency(basePythonRuntimeDependency(embedded), embedded)
	if result["status"] != pythonRuntimeDependencyStatusOK || !strings.Contains(result["message"].(string), "bundled") {
		t.Fatalf("embedded helper result = %#v", result)
	}
}

func TestCheckPythonRuntimeDependencyReportsTimeout(t *testing.T) {
	configureDependencyTestPythonSourceRuntime(t)
	previousTimeout := pythonRuntimeDependencyCheckTimeout
	pythonRuntimeDependencyCheckTimeout = time.Millisecond
	t.Cleanup(func() { pythonRuntimeDependencyCheckTimeout = previousTimeout })
	restorePythonRuntimeDependencyProbe(t, func(ctx context.Context, _ PythonRuntimeResolution) PythonRuntimeProbeResult {
		<-ctx.Done()
		return PythonRuntimeProbeResult{DetectedVersion: "3.12.1"}
	})

	result := CheckPythonRuntimeDependency(context.Background())
	if result["status"] != pythonRuntimeDependencyStatusError || !strings.Contains(result["message"].(string), "timed out") {
		t.Fatalf("timeout result = %#v", result)
	}
}

func TestSummarizePythonRuntimeCommandErrorBoundsOutput(t *testing.T) {
	err := errors.New("exit status 1")
	if got := summarizePythonRuntimeCommandError(err, ""); got != err.Error() {
		t.Fatalf("empty output summary = %q", got)
	}
	longOutput := strings.Repeat("prefix", 100) + " useful tail "
	got := summarizePythonRuntimeCommandError(err, longOutput)
	if !strings.HasPrefix(got, err.Error()+": ") || !strings.Contains(got, "useful tail") || len(got) > len(err.Error())+2+500 {
		t.Fatalf("bounded output summary = %q", got)
	}
}

func configureDependencyTestPythonSourceRuntime(t *testing.T) string {
	t.Helper()
	python := filepath.Join(t.TempDir(), "python")
	if err := os.WriteFile(python, []byte("python"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvYFinanceSidecar, "")
	t.Setenv(EnvYFinanceDevPython, python)
	t.Setenv(EnvYFinanceDevPythonPath, t.TempDir())
	return python
}

func restorePythonRuntimeDependencyProbe(
	t *testing.T,
	probe func(context.Context, PythonRuntimeResolution) PythonRuntimeProbeResult,
) {
	t.Helper()
	previous := pythonRuntimeDependencyProbe
	pythonRuntimeDependencyProbe = probe
	t.Cleanup(func() { pythonRuntimeDependencyProbe = previous })
}
