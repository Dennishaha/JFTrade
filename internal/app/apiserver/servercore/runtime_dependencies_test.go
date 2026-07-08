package servercore

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestCheckNodeRuntimeDependencyOK(t *testing.T) {
	windowsNodePath := `C:\Program Files\nodejs\node.exe`
	restoreRuntimeDependencyProbe(t,
		func(path string) (string, error) {
			if path != windowsNodePath {
				t.Fatalf("LookPath path = %q, want configured path", path)
			}
			return path, nil
		},
		func(_ context.Context, path string, args ...string) ([]byte, error) {
			if path != windowsNodePath || strings.Join(args, " ") != "--version" {
				t.Fatalf("command = %s %v, want node --version", path, args)
			}
			return []byte("v22.1.0\n"), nil
		},
	)

	result := checkNodeRuntimeDependency(context.Background(), jftsettings.PineWorkerSettings{NodeBinaryPath: ` "` + windowsNodePath + `" `})
	if result["status"] != runtimeDependencyStatusOK {
		t.Fatalf("status = %#v, result = %#v", result["status"], result)
	}
	if result["detectedVersion"] != "22.1.0" || result["source"] != runtimeDependencySourceSettings || result["effectivePath"] != windowsNodePath {
		t.Fatalf("result = %#v", result)
	}
}

func TestCheckNodeRuntimeDependencyReportsMissing(t *testing.T) {
	setRuntimeDependencyGOOS(t, "linux")
	restoreRuntimeDependencyProbe(t,
		func(path string) (string, error) {
			if path != "node" {
				t.Fatalf("LookPath path = %q, want default node", path)
			}
			return "", exec.ErrNotFound
		},
		nil,
	)

	result := checkNodeRuntimeDependency(context.Background(), jftsettings.PineWorkerSettings{})
	if result["status"] != runtimeDependencyStatusMissing {
		t.Fatalf("status = %#v, result = %#v", result["status"], result)
	}
	if !strings.Contains(result["message"].(string), "not found") {
		t.Fatalf("message = %#v", result["message"])
	}
}

func TestCheckNodeRuntimeDependencyUsesMacOSCommonPathFallback(t *testing.T) {
	setRuntimeDependencyGOOS(t, "darwin")
	lookups := []string{}
	restoreRuntimeDependencyProbe(t,
		func(path string) (string, error) {
			lookups = append(lookups, path)
			if path == "/opt/homebrew/bin/node" {
				return path, nil
			}
			return "", exec.ErrNotFound
		},
		func(_ context.Context, path string, args ...string) ([]byte, error) {
			if path != "/opt/homebrew/bin/node" || strings.Join(args, " ") != "--version" {
				t.Fatalf("command = %s %v, want fallback node --version", path, args)
			}
			return []byte("v22.2.0\n"), nil
		},
	)

	result := checkNodeRuntimeDependency(context.Background(), jftsettings.PineWorkerSettings{})
	if result["status"] != runtimeDependencyStatusOK {
		t.Fatalf("status = %#v, result = %#v", result["status"], result)
	}
	if result["effectivePath"] != "/opt/homebrew/bin/node" || result["source"] != "common:/opt/homebrew/bin/node" {
		t.Fatalf("result = %#v", result)
	}
	if strings.Join(lookups, ",") != "node,/opt/homebrew/bin/node" {
		t.Fatalf("lookups = %#v, want PATH node then Homebrew fallback", lookups)
	}
	if got := resolvePineWorkerRuntime(jftsettings.PineWorkerSettings{}); got != "/opt/homebrew/bin/node" {
		t.Fatalf("resolvePineWorkerRuntime() = %q, want shared fallback", got)
	}
}

func TestCheckNodeRuntimeDependencyMissingMessageListsMacOSAttempts(t *testing.T) {
	setRuntimeDependencyGOOS(t, "darwin")
	restoreRuntimeDependencyProbe(t,
		func(string) (string, error) {
			return "", exec.ErrNotFound
		},
		nil,
	)

	result := checkNodeRuntimeDependency(context.Background(), jftsettings.PineWorkerSettings{})
	message, _ := result["message"].(string)
	if result["status"] != runtimeDependencyStatusMissing {
		t.Fatalf("status = %#v, result = %#v", result["status"], result)
	}
	for _, want := range []string{"Finder", "Tried: node, /opt/homebrew/bin/node", "Settings"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message = %q, want %q", message, want)
		}
	}
}

func TestCheckNodeRuntimeDependencyReportsOutdatedInvalidAndCommandError(t *testing.T) {
	tests := []struct {
		name       string
		output     []byte
		err        error
		wantStatus string
	}{
		{name: "outdated", output: []byte("v20.11.1"), wantStatus: runtimeDependencyStatusOutdated},
		{name: "invalid", output: []byte("not-a-version"), wantStatus: runtimeDependencyStatusError},
		{name: "command error", output: []byte("boom"), err: errors.New("exit status 1"), wantStatus: runtimeDependencyStatusError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreRuntimeDependencyProbe(t,
				func(path string) (string, error) { return path, nil },
				func(context.Context, string, ...string) ([]byte, error) { return tt.output, tt.err },
			)
			result := checkNodeRuntimeDependency(context.Background(), jftsettings.PineWorkerSettings{})
			if result["status"] != tt.wantStatus {
				t.Fatalf("status = %#v, want %s; result = %#v", result["status"], tt.wantStatus, result)
			}
		})
	}
}

func TestRuntimeDependenciesAggregatesRequiredStatus(t *testing.T) {
	restoreRuntimeDependencyProbe(t,
		func(path string) (string, error) { return path, nil },
		func(context.Context, string, ...string) ([]byte, error) { return []byte("v21.0.0"), nil },
	)
	server := &Server{}
	result := server.runtimeDependencies(context.Background())
	if result["allRequiredSatisfied"] != false {
		t.Fatalf("allRequiredSatisfied = %#v, want false", result["allRequiredSatisfied"])
	}
	dependencies, ok := result["dependencies"].([]map[string]any)
	if !ok || len(dependencies) != 1 || dependencies[0]["id"] != "node" {
		t.Fatalf("dependencies = %#v", result["dependencies"])
	}
}

func setRuntimeDependencyGOOS(t *testing.T, goos string) {
	t.Helper()
	previous := runtimeDependencyGOOS
	runtimeDependencyGOOS = goos
	t.Cleanup(func() {
		runtimeDependencyGOOS = previous
	})
}

func restoreRuntimeDependencyProbe(
	t *testing.T,
	lookPath func(string) (string, error),
	output func(context.Context, string, ...string) ([]byte, error),
) {
	t.Helper()
	previousLookPath := runtimeDependencyLookPath
	previousOutput := runtimeDependencyOutput
	if lookPath != nil {
		runtimeDependencyLookPath = lookPath
	}
	if output != nil {
		runtimeDependencyOutput = output
	}
	t.Cleanup(func() {
		runtimeDependencyLookPath = previousLookPath
		runtimeDependencyOutput = previousOutput
	})
}
