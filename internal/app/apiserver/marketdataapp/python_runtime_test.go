package marketdataapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/yfinanceassets"
)

func TestResolveSourcePythonRuntimeHonorsEnvironmentAndFallbacks(t *testing.T) {
	if !yfinanceassets.DevelopmentOverridesAllowed() {
		t.Skip("source Python resolution is disabled in release-assets builds")
	}
	source := t.TempDir()
	t.Setenv(EnvYFinanceDevPythonPath, source)

	t.Run("environment override", func(t *testing.T) {
		t.Setenv(EnvYFinanceDevPython, "/env/python")
		restorePythonRuntimeLookPath(t, func(path string) (string, error) {
			if path != "/env/python" {
				t.Fatalf("LookPath = %q, want environment Python", path)
			}
			return path, nil
		})
		got := ResolvePythonRuntime()
		if !got.Available || got.Source != "env:"+EnvYFinanceDevPython || got.ResolvedPath != "/env/python" {
			t.Fatalf("resolution = %#v", got)
		}
	})

	t.Run("workspace venv then PATH", func(t *testing.T) {
		t.Setenv(EnvYFinanceDevPython, "")
		venv := filepath.Join(filepath.Dir(source), ".venv", "bin", "python")
		lookups := []string{}
		restorePythonRuntimeLookPath(t, func(path string) (string, error) {
			lookups = append(lookups, path)
			if path == "python3" {
				return "/usr/bin/python3", nil
			}
			return "", errors.New("missing")
		})
		got := ResolvePythonRuntime()
		if !got.Available || got.Source != "path" || got.ResolvedPath != "/usr/bin/python3" {
			t.Fatalf("resolution = %#v", got)
		}
		if len(lookups) < 2 || lookups[0] != venv || lookups[1] != "python3" {
			t.Fatalf("lookups = %#v", lookups)
		}
	})
}

func TestResolvePythonRuntimeReportsExternalAndEmbeddedHelpers(t *testing.T) {
	if yfinanceassets.DevelopmentOverridesAllowed() {
		helper := filepath.Join(t.TempDir(), "helper")
		if err := os.WriteFile(helper, []byte("helper"), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvYFinanceSidecar, helper)
		got := ResolvePythonRuntime()
		if got.Mode != PythonRuntimeModeExternalHelper || !got.Available || got.Configurable || got.Required {
			t.Fatalf("external helper resolution = %#v", got)
		}
	}

	previous := selectYFinanceAsset
	selectYFinanceAsset = func() (yfinanceassets.Asset, bool, error) {
		return yfinanceassets.Asset{Name: "helper"}, true, nil
	}
	t.Cleanup(func() { selectYFinanceAsset = previous })
	embedded := resolveEmbeddedPythonRuntime()
	if embedded.Mode != PythonRuntimeModeEmbedded || !embedded.Available || embedded.Configurable || embedded.Required {
		t.Fatalf("embedded resolution = %#v", embedded)
	}
}

func TestProbePythonRuntimeValidatesVersionAndModulesWithoutImportingThem(t *testing.T) {
	resolution := PythonRuntimeResolution{
		Mode: PythonRuntimeModeSource, Available: true,
		ResolvedPath: "/python", SourcePath: "/source",
	}
	tests := []struct {
		name      string
		output    string
		err       error
		available bool
		outdated  bool
		missing   int
	}{
		{name: "available", output: `{"version":[3,11,9],"missing":[]}`, available: true},
		{name: "outdated", output: `{"version":[3,10,14],"missing":[]}`, outdated: true},
		{name: "missing modules", output: `{"version":[3,12,1],"missing":["fastapi","yfinance"]}`, missing: 2},
		{name: "command error", output: "boom", err: errors.New("exit status 1")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			restorePythonRuntimeProbeOutput(t, func(_ context.Context, path string, source string, args ...string) ([]byte, error) {
				if path != "/python" || source != "/source" || len(args) != 2 || args[0] != "-c" {
					t.Fatalf("probe command = %q %q %#v", path, source, args)
				}
				return []byte(test.output), test.err
			})
			got := ProbePythonRuntime(context.Background(), resolution)
			if got.Available != test.available || got.Outdated != test.outdated || len(got.MissingModules) != test.missing {
				t.Fatalf("probe = %#v", got)
			}
			if test.err != nil && !errors.Is(got.Err, test.err) {
				t.Fatalf("probe error = %v, want %v", got.Err, test.err)
			}
		})
	}
}

func restorePythonRuntimeLookPath(t *testing.T, lookPath func(string) (string, error)) {
	t.Helper()
	previous := pythonRuntimeLookPath
	pythonRuntimeLookPath = lookPath
	t.Cleanup(func() { pythonRuntimeLookPath = previous })
}

func restorePythonRuntimeProbeOutput(
	t *testing.T,
	output func(context.Context, string, string, ...string) ([]byte, error),
) {
	t.Helper()
	previous := pythonRuntimeProbeOutput
	pythonRuntimeProbeOutput = output
	t.Cleanup(func() { pythonRuntimeProbeOutput = previous })
}
