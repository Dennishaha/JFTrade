package pineworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const processSmokeEnv = "JFTRADE_PINEWORKER_PROCESS_SMOKE"
const realProcessSmokeEnv = "JFTRADE_PINEWORKER_REAL_PROCESS_SMOKE"

func TestWorkerManagerProcessSmokeWithNodeWorker(t *testing.T) {
	if os.Getenv(processSmokeEnv) != "1" {
		t.Skip(processSmokeEnv + "=1 is required for process-level Pine worker smoke")
	}
	manager := startNodeWorkerProcessSmokeManager(t, true, "smoke-mock")
	response := waitForProcessSmokeRunScript(t, manager)
	if response.JobID != "job-1" || len(response.OrderIntents) == 0 || response.Metadata.WorkerID != "pineworker-1" {
		t.Fatalf("unexpected worker response: %#v", response)
	}
	if response.Metadata.PeakRSSBytes <= 0 {
		t.Fatalf("worker peak RSS was not reported: %#v", response.Metadata)
	}
}

func TestWorkerManagerRealPineTSProcessSmoke(t *testing.T) {
	if os.Getenv(realProcessSmokeEnv) != "1" {
		t.Skip(realProcessSmokeEnv + "=1 is required for real PineTS process smoke")
	}
	root := repoRoot(t)
	if !pinetsInstalled(root) {
		t.Fatalf("pinets package is not installed; real PineTS process smoke cannot run")
	}
	manager := startNodeWorkerProcessSmokeManager(t, false, "real-pinets-smoke")
	response := waitForProcessSmokeRunScript(t, manager)
	if response.JobID != "job-1" || response.Metadata.WorkerID != "pineworker-1" {
		t.Fatalf("unexpected real PineTS worker response: %#v", response)
	}
	if response.Metadata.PineTSVersion != "real-pinets-smoke" {
		t.Fatalf("real PineTS smoke did not report expected PineTS version metadata: %#v", response.Metadata)
	}
	if response.Metadata.PineTSVersion == "smoke-mock" {
		t.Fatalf("real PineTS smoke used mock runtime metadata: %#v", response.Metadata)
	}
}

func startNodeWorkerProcessSmokeManager(t *testing.T, mock bool, pineTSVersion string) *WorkerManager {
	t.Helper()
	nodePath, err := nodeExecutable()
	if err != nil {
		t.Skip("node is not installed or not on PATH")
	}
	root := repoRoot(t)
	if missing := missingWorkerRuntimeDeps(root); len(missing) > 0 {
		t.Skip("missing worker runtime dependencies: " + strings.Join(missing, ", "))
	}
	tempDir := t.TempDir()
	const outputName = "worker.mjs"
	workerPath := filepath.Join(tempDir, outputName)
	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancelBuild()
	build := exec.CommandContext(
		buildCtx,
		nodePath, filepath.Join("scripts", "build-pineworker-dev.mjs"),
	)
	build.Dir = root
	build.Env = append(os.Environ(), "JFTRADE_PINEWORKER_DEV_OUT_DIR="+tempDir)
	buildOutput, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("rolldown worker smoke bundle: %v\n%s", err, string(buildOutput))
	}
	bundleData, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("read worker binary: %v", err)
	}
	sum := sha256.Sum256(bundleData)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	launcher, err := NewNodeWorkerLauncher(NodeWorkerLauncherConfig{
		Bundle: WorkerBundle{
			Name:   outputName,
			Data:   bundleData,
			SHA256: hex.EncodeToString(sum[:]),
		},
		RuntimePath:   nodePath,
		TempDir:       tempDir,
		ProtoPath:     filepath.Join(root, "pkg", "strategy", "pineworker", "proto", "pineworker.proto"),
		Mock:          mock,
		StopTimeout:   time.Second,
		PineTSVersion: pineTSVersion,
		Stdout:        &stdout,
		Stderr:        &stderr,
	})
	if err != nil {
		t.Fatalf("NewNodeWorkerLauncher: %v", err)
	}

	port := freeTCPPort(t)
	config := ManagerConfig{
		Workers:   1,
		Host:      "127.0.0.1",
		StartPort: port,
		WorkerConfig: WorkerConfig{
			RequestTimeout: 5 * time.Second,
		},
		Gate: relaxedGate(),
	}
	manager, err := NewWorkerManager(config, launcher, NewGRPCDialer(GRPCDialerConfig{MaxMessageBytes: config.WorkerConfig.MaxMessageBytes}))
	if err != nil {
		t.Fatalf("NewWorkerManager: %v", err)
	}
	startCtx, cancelStart := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStart()
	if err := manager.Start(startCtx); err != nil {
		t.Fatalf("WorkerManager.Start: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	t.Cleanup(func() {
		stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelStop()
		if err := manager.Stop(stopCtx); err != nil {
			t.Fatalf("WorkerManager.Stop: %v", err)
		}
	})
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("pine worker stdout:\n%s", stdout.String())
			t.Logf("pine worker stderr:\n%s", stderr.String())
		}
	})
	return manager
}

func waitForProcessSmokeRunScript(t *testing.T, manager *WorkerManager) RunScriptResponse {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		response, err := manager.RunScript(context.Background(), validClientRequest())
		if err == nil {
			return response
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("RunScript through worker process did not become ready: %v", lastErr)
	return RunScriptResponse{}
}

func missingWorkerRuntimeDeps(root string) []string {
	missing := []string{}
	for _, module := range []string{"@grpc/grpc-js", "@grpc/proto-loader", "rolldown"} {
		if _, err := os.Stat(filepath.Join(root, "node_modules", filepath.FromSlash(module))); err != nil {
			missing = append(missing, module)
		}
	}
	return missing
}

func pinetsInstalled(root string) bool {
	for _, rel := range []string{
		filepath.Join("node_modules", "pinets", "package.json"),
		filepath.Join("workers", "pineworker", "node_modules", "pinets", "package.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			return true
		}
	}
	return false
}

func nodeExecutable() (string, error) {
	if path := strings.TrimSpace(os.Getenv("JFTRADE_NODE_BINARY")); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	if path, err := exec.LookPath("node"); err == nil {
		return path, nil
	}
	return "", exec.ErrNotFound
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free port: %v", err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Fatalf("close free port listener: %v", err)
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}
