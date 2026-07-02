package pineworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNodeWorkerLauncherDefaultsAndMaterializationBoundaries(t *testing.T) {
	launcher, err := NewNodeWorkerLauncher(NodeWorkerLauncherConfig{
		Bundle: WorkerBundle{Data: []byte("console.log('worker')")},
	})
	if err != nil {
		t.Fatalf("NewNodeWorkerLauncher() error = %v", err)
	}
	if launcher.config.Bundle.Name != "worker.mjs" || launcher.config.RuntimePath != "node" || launcher.config.StopTimeout != 5*time.Second {
		t.Fatalf("launcher defaults = %#v", launcher.config)
	}
	path, err := launcher.materializeBundle(WorkerSpec{WorkerID: "worker-1"})
	if err != nil {
		t.Fatalf("materializeBundle(system temp) error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(path)) })
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read materialized bundle: %v", err)
	}
	if string(raw) != "console.log('worker')" {
		t.Fatalf("materialized bundle = %q", raw)
	}

	root := t.TempDir()
	blockedDir := filepath.Join(root, "blocked")
	if err := os.WriteFile(blockedDir, []byte("file"), 0o600); err != nil {
		t.Fatalf("write blocked temp dir: %v", err)
	}
	blockedLauncher, err := NewNodeWorkerLauncher(NodeWorkerLauncherConfig{
		Bundle:  WorkerBundle{Name: "worker.mjs", Data: []byte("worker")},
		TempDir: blockedDir,
	})
	if err != nil {
		t.Fatalf("NewNodeWorkerLauncher(blocked dir) error = %v", err)
	}
	if _, err := blockedLauncher.materializeBundle(WorkerSpec{WorkerID: "worker-1"}); err == nil || !strings.Contains(err.Error(), "create pine worker temp dir") {
		t.Fatalf("materializeBundle(blocked dir) error = %v", err)
	}

	writeLauncher, err := NewNodeWorkerLauncher(NodeWorkerLauncherConfig{
		Bundle:  WorkerBundle{Name: "missing/worker.mjs", Data: []byte("worker")},
		TempDir: filepath.Join(root, "write-error"),
	})
	if err != nil {
		t.Fatalf("NewNodeWorkerLauncher(write error) error = %v", err)
	}
	if _, err := writeLauncher.materializeBundle(WorkerSpec{WorkerID: "worker-1"}); err == nil || !strings.Contains(err.Error(), "write pine worker bundle") {
		t.Fatalf("materializeBundle(write error) error = %v", err)
	}
}

func TestNodeWorkerLauncherRemovesMaterializedBundleWhenContextAlreadyCanceled(t *testing.T) {
	tempDir := t.TempDir()
	launcher, err := NewNodeWorkerLauncher(NodeWorkerLauncherConfig{
		Bundle:      WorkerBundle{Name: "worker.mjs", Data: []byte("worker")},
		RuntimePath: "/bin/true",
		TempDir:     tempDir,
	})
	if err != nil {
		t.Fatalf("NewNodeWorkerLauncher() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := launcher.Start(ctx, WorkerSpec{WorkerID: "worker-1", Address: "127.0.0.1:50051"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start(canceled) error = %v, want context.Canceled", err)
	}
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("materialized files after canceled start = %#v", entries)
	}
}

func TestOSWorkerProcessDiagnosticsAndNilBoundaries(t *testing.T) {
	var nilProcess *OSWorkerProcess
	if got := nilProcess.Diagnostics(); got != "" {
		t.Fatalf("nil Diagnostics() = %q", got)
	}
	if err := nilProcess.Stop(t.Context()); err != nil {
		t.Fatalf("nil Stop() error = %v", err)
	}

	stdout := bytes.NewBufferString(strings.Repeat("x", 2100))
	process := &OSWorkerProcess{path: "/tmp/worker.mjs", runtimePath: "node", workDir: "/repo", args: []string{"worker.mjs"}, stdout: stdout}
	diagnostics := process.Diagnostics()
	if !strings.Contains(diagnostics, "stdout="+strings.Repeat("x", 2000)) || strings.Contains(diagnostics, strings.Repeat("x", 2001)) {
		t.Fatalf("Diagnostics() did not retain the bounded stdout tail: length=%d", len(diagnostics))
	}
	if got := writerString(io.Discard); got != "" {
		t.Fatalf("writerString(non-stringer) = %q", got)
	}
	plainErr := errors.New("wait failed")
	if got := ignoreProcessExit(plainErr); !errors.Is(got, plainErr) {
		t.Fatalf("ignoreProcessExit(plain) = %v", got)
	}
}

func TestOSWorkerProcessForcesTerminationOnTimeoutAndCancellation(t *testing.T) {
	t.Run("stop timeout", func(t *testing.T) {
		process := startInterruptIgnoringProcess(t, 10*time.Millisecond)
		if err := process.Stop(t.Context()); err != nil {
			t.Fatalf("Stop(timeout) error = %v", err)
		}
		if _, err := os.Stat(process.path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("bundle after Stop(timeout) stat error = %v", err)
		}
	})

	t.Run("caller cancellation", func(t *testing.T) {
		process := startInterruptIgnoringProcess(t, time.Second)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := process.Stop(ctx); err != nil {
			t.Fatalf("Stop(canceled) error = %v", err)
		}
		if _, err := os.Stat(process.path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("bundle after Stop(canceled) stat error = %v", err)
		}
	})
}

func startInterruptIgnoringProcess(t *testing.T, stopTimeout time.Duration) *OSWorkerProcess {
	t.Helper()
	path := filepath.Join(t.TempDir(), "worker.mjs")
	if err := os.WriteFile(path, []byte("worker"), 0o600); err != nil {
		t.Fatalf("write process bundle: %v", err)
	}
	cmd := exec.Command("/bin/sh", "-c", `trap '' INT; exec sleep 10`)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start interrupt-ignoring process: %v", err)
	}
	return &OSWorkerProcess{cmd: cmd, path: path, stopTimeout: stopTimeout}
}
