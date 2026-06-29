package pineworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBunWorkerLauncherMaterializesBundleWithArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script launcher test is unix-specific")
	}
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "args.log")
	cwdPath := filepath.Join(tempDir, "cwd.log")
	workDir := t.TempDir()
	var stderr bytes.Buffer
	script := "#!/bin/sh\npwd > " + shellQuote(cwdPath) + "\nprintf 'worker stderr tail\\n' >&2\nprintf '%s\\n' \"$@\" > " + shellQuote(logPath) + "\n"
	launcher := newScriptLauncher(t, script, BunWorkerLauncherConfig{
		RuntimePath:     "/bin/sh",
		TempDir:         tempDir,
		WorkDir:         workDir,
		ProtoPath:       "proto/pineworker.proto",
		MaxMessageBytes: 1234,
		PineTSVersion:   "pinets-test",
		Mock:            true,
		ExtraArgs:       []string{"--extra", "value"},
		Stderr:          &stderr,
	})

	process, err := launcher.Start(context.Background(), WorkerSpec{WorkerID: "worker-1", Address: "127.0.0.1:50051", Port: 50051})
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	waitForFile(t, logPath)
	diagnostics := process.(*OSWorkerProcess).Diagnostics()
	if err := process.Stop(context.Background()); err != nil {
		t.Fatalf("Stop error = %v", err)
	}
	rawCWD, err := os.ReadFile(cwdPath)
	if err != nil {
		t.Fatalf("read cwd log: %v", err)
	}
	if filepath.Clean(strings.TrimSpace(string(rawCWD))) != filepath.Clean(workDir) {
		t.Fatalf("worker cwd = %q, want %q", strings.TrimSpace(string(rawCWD)), workDir)
	}
	if !strings.Contains(diagnostics, "cwd="+workDir) || !strings.Contains(diagnostics, "runtime=/bin/sh") || !strings.Contains(diagnostics, "stderr=worker stderr tail") {
		t.Fatalf("diagnostics = %q, want cwd, runtime, and stderr", diagnostics)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	args := string(raw)
	for _, want := range []string{"--address", "127.0.0.1:50051", "--worker-id", "worker-1", "--max-message-bytes", "1234", "--mock", "true", "--extra", "value"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q missing %q", args, want)
		}
	}
}

func TestBunWorkerLauncherRejectsBadChecksum(t *testing.T) {
	_, err := NewBunWorkerLauncher(BunWorkerLauncherConfig{
		Bundle: WorkerBundle{Name: "worker.js", Data: []byte("x"), SHA256: "bad"},
	})
	if err != nil {
		t.Fatalf("NewBunWorkerLauncher error = %v", err)
	}
	launcher, _ := NewBunWorkerLauncher(BunWorkerLauncherConfig{
		Bundle: WorkerBundle{Name: "worker.js", Data: []byte("x"), SHA256: "bad"},
	})
	_, err = launcher.Start(context.Background(), WorkerSpec{WorkerID: "worker-1", Address: "127.0.0.1:50051"})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Start error = %v, want checksum mismatch", err)
	}
}

func TestBunWorkerLauncherStopKillsLongRunningProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script launcher test is unix-specific")
	}
	launcher := newScriptLauncher(t, "#!/bin/sh\nsleep 30\n", BunWorkerLauncherConfig{
		TempDir:     t.TempDir(),
		StopTimeout: 10 * time.Millisecond,
	})
	process, err := launcher.Start(context.Background(), WorkerSpec{WorkerID: "worker-1", Address: "127.0.0.1:50051"})
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := process.Stop(ctx); err != nil {
		t.Fatalf("Stop error = %v", err)
	}
}

func TestNewBunWorkerLauncherRequiresBundle(t *testing.T) {
	_, err := NewBunWorkerLauncher(BunWorkerLauncherConfig{})
	if err == nil || !strings.Contains(err.Error(), "bundle data is required") {
		t.Fatalf("error = %v, want bundle data required", err)
	}
}

func TestBunWorkerLauncherRejectsNilReceiver(t *testing.T) {
	var launcher *BunWorkerLauncher
	_, err := launcher.Start(context.Background(), WorkerSpec{WorkerID: "worker-1", Address: "127.0.0.1:50051"})
	if err == nil || !strings.Contains(err.Error(), "launcher is nil") {
		t.Fatalf("Start error = %v, want launcher nil", err)
	}
}

func TestBunWorkerLauncherReturnsStartErrorAndRemovesFile(t *testing.T) {
	tempDir := t.TempDir()
	launcher, err := NewBunWorkerLauncher(BunWorkerLauncherConfig{
		Bundle:      WorkerBundle{Name: "worker.js", Data: []byte("invalid bundle")},
		RuntimePath: filepath.Join(tempDir, "missing-bun"),
		TempDir:     tempDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = launcher.Start(context.Background(), WorkerSpec{WorkerID: "worker-1", Address: "127.0.0.1:50051"})
	if err == nil {
		t.Fatal("Start error = nil, want error")
	}
	if _, statErr := os.Stat(filepath.Join(tempDir, "worker-1-worker.js")); !os.IsNotExist(statErr) {
		t.Fatalf("materialized bundle was not removed after start failure: %v", statErr)
	}
}

func newScriptLauncher(t *testing.T, script string, config BunWorkerLauncherConfig) *BunWorkerLauncher {
	t.Helper()
	sum := sha256.Sum256([]byte(script))
	config.Bundle = WorkerBundle{
		Name:   "worker.js",
		Data:   []byte(script),
		SHA256: hex.EncodeToString(sum[:]),
	}
	if config.RuntimePath == "" {
		config.RuntimePath = "/bin/sh"
	}
	launcher, err := NewBunWorkerLauncher(config)
	if err != nil {
		t.Fatal(err)
	}
	return launcher
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("file did not appear: %s", path)
}
