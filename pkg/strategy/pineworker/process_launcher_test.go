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

func TestBinaryWorkerLauncherMaterializesBinaryWithArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script launcher test is unix-specific")
	}
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "args.log")
	cwdPath := filepath.Join(tempDir, "cwd.log")
	workDir := t.TempDir()
	var stderr bytes.Buffer
	script := "#!/bin/sh\npwd > " + shellQuote(cwdPath) + "\nprintf 'worker stderr tail\\n' >&2\nprintf '%s\\n' \"$@\" > " + shellQuote(logPath) + "\n"
	launcher := newScriptLauncher(t, script, BinaryWorkerLauncherConfig{
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
	if !strings.Contains(diagnostics, "cwd="+workDir) || !strings.Contains(diagnostics, "stderr=worker stderr tail") {
		t.Fatalf("diagnostics = %q, want cwd and stderr", diagnostics)
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

func TestBinaryWorkerLauncherRejectsBadChecksum(t *testing.T) {
	_, err := NewBinaryWorkerLauncher(BinaryWorkerLauncherConfig{
		Binary: WorkerBinary{Name: "worker", Data: []byte("x"), SHA256: "bad"},
	})
	if err != nil {
		t.Fatalf("NewBinaryWorkerLauncher error = %v", err)
	}
	launcher, _ := NewBinaryWorkerLauncher(BinaryWorkerLauncherConfig{
		Binary: WorkerBinary{Name: "worker", Data: []byte("x"), SHA256: "bad"},
	})
	_, err = launcher.Start(context.Background(), WorkerSpec{WorkerID: "worker-1", Address: "127.0.0.1:50051"})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Start error = %v, want checksum mismatch", err)
	}
}

func TestBinaryWorkerLauncherStopKillsLongRunningProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script launcher test is unix-specific")
	}
	launcher := newScriptLauncher(t, "#!/bin/sh\nsleep 30\n", BinaryWorkerLauncherConfig{
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

func TestNewBinaryWorkerLauncherRequiresBinary(t *testing.T) {
	_, err := NewBinaryWorkerLauncher(BinaryWorkerLauncherConfig{})
	if err == nil || !strings.Contains(err.Error(), "binary data is required") {
		t.Fatalf("error = %v, want binary data required", err)
	}
}

func TestBinaryWorkerLauncherRejectsNilReceiver(t *testing.T) {
	var launcher *BinaryWorkerLauncher
	_, err := launcher.Start(context.Background(), WorkerSpec{WorkerID: "worker-1", Address: "127.0.0.1:50051"})
	if err == nil || !strings.Contains(err.Error(), "launcher is nil") {
		t.Fatalf("Start error = %v, want launcher nil", err)
	}
}

func TestBinaryWorkerLauncherReturnsStartErrorAndRemovesFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test is unix-specific")
	}
	tempDir := t.TempDir()
	launcher, err := NewBinaryWorkerLauncher(BinaryWorkerLauncherConfig{
		Binary:  WorkerBinary{Name: "bad.sh", Data: []byte("not a shell script")},
		TempDir: tempDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = launcher.Start(context.Background(), WorkerSpec{WorkerID: "worker-1", Address: "127.0.0.1:50051"})
	if err == nil {
		t.Fatal("Start error = nil, want error")
	}
	if _, statErr := os.Stat(filepath.Join(tempDir, "worker-1-bad.sh")); !os.IsNotExist(statErr) {
		t.Fatalf("materialized binary was not removed after start failure: %v", statErr)
	}
}

func newScriptLauncher(t *testing.T, script string, config BinaryWorkerLauncherConfig) *BinaryWorkerLauncher {
	t.Helper()
	sum := sha256.Sum256([]byte(script))
	config.Binary = WorkerBinary{
		Name:   "worker.sh",
		Data:   []byte(script),
		SHA256: hex.EncodeToString(sum[:]),
	}
	launcher, err := NewBinaryWorkerLauncher(config)
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
