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
