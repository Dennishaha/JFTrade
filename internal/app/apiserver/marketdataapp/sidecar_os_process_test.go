package marketdataapp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOSSidecarProcessValidatesLaunchesAndStopsRealChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt process semantics are POSIX-specific")
	}
	if _, err := startMarketDataSidecar(SidecarConfig{}); err == nil {
		t.Fatal("startMarketDataSidecar accepted an empty executable")
	}
	for _, port := range []int{0, 65536} {
		_, err := startMarketDataSidecar(SidecarConfig{Executable: os.Args[0], Host: sidecarHost, Port: port})
		if err == nil || !strings.Contains(err.Error(), "between 1 and 65535") {
			t.Fatalf("port %d error = %v", port, err)
		}
	}
	if _, err := startMarketDataSidecar(SidecarConfig{
		Executable: os.Args[0], Host: "0.0.0.0", Port: 7788,
	}); err == nil || !strings.Contains(err.Error(), sidecarHost) {
		t.Fatalf("non-loopback managed host error = %v", err)
	}
	if _, err := startMarketDataSidecar(SidecarConfig{
		Executable: filepath.Join(t.TempDir(), "missing-helper"), Host: sidecarHost, Port: 7788,
	}); err == nil {
		t.Fatal("startMarketDataSidecar accepted a missing executable")
	}

	helper := buildSidecarHelper(t)
	capturePath := filepath.Join(t.TempDir(), "sidecar-args")
	t.Setenv("JFTRADE_TEST_SIDECAR_CAPTURE", capturePath)
	process, err := startMarketDataSidecar(SidecarConfig{
		Executable: helper, Host: sidecarHost, Port: 7788,
	})
	if err != nil {
		t.Fatalf("startMarketDataSidecar: %v", err)
	}
	concrete, ok := process.(*osSidecarProcess)
	if !ok || !concrete.Running() {
		t.Fatalf("started process = %#v, running=%v", process, process.Running())
	}
	captured := waitForCapturedSidecarArgs(t, capturePath)
	for _, expected := range []string{
		"--host", "127.0.0.1", "--port", "7788", "PYTHONUNBUFFERED=1",
	} {
		if !strings.Contains(captured, expected) {
			t.Fatalf("captured child launch missing %q: %q", expected, captured)
		}
	}
	if err := concrete.Close(); err != nil {
		t.Fatalf("Close running child: %v", err)
	}
	if concrete.Running() {
		t.Fatal("child remained running after Close")
	}
	if err := concrete.Close(); err != nil {
		t.Fatalf("repeated Close: %v", err)
	}

	waitErr := errors.New("wait bookkeeping failed")
	concrete.mu.Lock()
	concrete.waitErr = waitErr
	concrete.mu.Unlock()
	if err := concrete.Close(); !errors.Is(err, waitErr) {
		t.Fatalf("non-exit wait error = %v", err)
	}
}

func TestOSSidecarProcessTreatsNaturalExitAsStoppedAndCloseable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper executable uses POSIX signal semantics")
	}
	helper := buildSidecarHelper(t)
	t.Setenv("JFTRADE_TEST_SIDECAR_EXIT", "1")
	process, err := startMarketDataSidecar(SidecarConfig{
		Executable: helper, Host: sidecarHost, Port: 7788,
	})
	if err != nil {
		t.Fatalf("startMarketDataSidecar: %v", err)
	}
	concrete := process.(*osSidecarProcess)
	waitForProcessExit(t, concrete)
	if concrete.Running() {
		t.Fatal("naturally exited child reported running")
	}
	if err := concrete.Close(); err != nil {
		t.Fatalf("Close should normalize child ExitError: %v", err)
	}

	var nilProcess *osSidecarProcess
	if nilProcess.Running() {
		t.Fatal("nil process reported running")
	}
	if err := nilProcess.Close(); err != nil {
		t.Fatalf("nil process Close: %v", err)
	}
	empty := &osSidecarProcess{}
	if empty.Running() {
		t.Fatal("empty process reported running")
	}
	if err := empty.Close(); err != nil {
		t.Fatalf("empty process Close: %v", err)
	}
}

func TestOSSidecarProcessBoundsGracefulStopBeforeKillingChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM fallback is POSIX-specific")
	}
	helper := buildSidecarHelper(t)
	capturePath := filepath.Join(t.TempDir(), "sidecar-args")
	t.Setenv("JFTRADE_TEST_SIDECAR_CAPTURE", capturePath)
	t.Setenv("JFTRADE_TEST_SIDECAR_IGNORE_TERM", "1")
	process, err := startMarketDataSidecar(SidecarConfig{
		Executable: helper, Host: sidecarHost, Port: 7788,
	})
	if err != nil {
		t.Fatalf("startMarketDataSidecar: %v", err)
	}
	concrete := process.(*osSidecarProcess)
	concrete.stopTimeout = 20 * time.Millisecond
	concrete.killTimeout = time.Second
	waitForCapturedSidecarArgs(t, capturePath)

	started := time.Now()
	if err := concrete.Close(); err != nil {
		t.Fatalf("Close stubborn child: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("Close stubborn child blocked for %s", elapsed)
	}
	if concrete.Running() {
		t.Fatal("stubborn child remained running after kill")
	}
}

func TestWaitForSidecarDoneHasExplicitTimeout(t *testing.T) {
	started := time.Now()
	if waitForSidecarDone(make(chan struct{}), 20*time.Millisecond) {
		t.Fatal("wait reported an open done channel as complete")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("bounded wait blocked for %s", elapsed)
	}
	done := make(chan struct{})
	close(done)
	if !waitForSidecarDone(done, time.Second) {
		t.Fatal("wait did not observe completed process")
	}
}

func TestProcessAlreadyFinishedIsASuccessfulStop(t *testing.T) {
	if !processStopSucceeded(nil) || !processStopSucceeded(os.ErrProcessDone) {
		t.Fatal("completed process stop was treated as a failure")
	}
	if processStopSucceeded(errors.New("permission denied")) {
		t.Fatal("unrelated process error was treated as a successful stop")
	}
}

func buildSidecarHelper(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "main.go")
	binaryPath := filepath.Join(tempDir, "sidecar-helper")
	source := `package main

import (
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	if os.Getenv("JFTRADE_TEST_SIDECAR_IGNORE_TERM") != "" {
		signal.Ignore(syscall.SIGTERM)
	}
	if path := os.Getenv("JFTRADE_TEST_SIDECAR_CAPTURE"); path != "" {
		payload := strings.Join(os.Args[1:], "\n") + "\nPYTHONUNBUFFERED=" + os.Getenv("PYTHONUNBUFFERED")
		_ = os.WriteFile(path, []byte(payload), 0o600)
	}
	if os.Getenv("JFTRADE_TEST_SIDECAR_EXIT") != "" {
		os.Exit(7)
	}
	if os.Getenv("JFTRADE_TEST_SIDECAR_IGNORE_TERM") != "" {
		select {}
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatalf("write sidecar helper: %v", err)
	}
	command := exec.Command("go", "build", "-o", binaryPath, sourcePath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build sidecar helper: %v\n%s", err, output)
	}
	return binaryPath
}

func waitForCapturedSidecarArgs(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		payload, err := os.ReadFile(path)
		if err == nil {
			return string(payload)
		}
		if time.Now().After(deadline) {
			t.Fatalf("sidecar helper did not capture args: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForProcessExit(t *testing.T, process *osSidecarProcess) {
	t.Helper()
	select {
	case <-process.done:
	case <-time.After(5 * time.Second):
		t.Fatal("sidecar helper did not exit")
	}
}
