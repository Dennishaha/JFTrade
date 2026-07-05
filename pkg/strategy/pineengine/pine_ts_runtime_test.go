package pineengine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShadowPayloadForScriptExecutesRepositoryWorker(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable: %v", err)
	}
	t.Setenv("JFTRADE_PINETS_MODE", ModeShadow)
	payload := ShadowPayloadForScript(`//@version=6
indicator("shadow boundary")
plot(close, "close")`)
	if !payload.Enabled || !payload.OK || payload.Mode != ModeShadow || payload.Status != "shadow_ok" {
		t.Fatalf("ShadowPayloadForScript() = %#v", payload)
	}
	if payload.Repository != "https://github.com/LuxAlgo/PineTS" || payload.Engine != PinetsShadowEngineID {
		t.Fatalf("shadow engine identity = repository:%q engine:%q", payload.Repository, payload.Engine)
	}
	if payload.DifferenceSummary["evaluated"] != true {
		t.Fatalf("shadow difference summary = %#v", payload.DifferenceSummary)
	}
}

func TestRepositoryRelativePineTSAssetsAreDiscoverable(t *testing.T) {
	oldWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	repositoryRoot, err := filepath.Abs(filepath.Join(oldWorkingDir, "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	if err := os.Chdir(repositoryRoot); err != nil {
		t.Fatalf("chdir repository root: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWorkingDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	t.Setenv("JFTRADE_PINETS_WORKER_PATH", "")

	if got := DefaultWorkerPath(); got != filepath.Join("scripts", "pinets-worker.mjs") {
		t.Fatalf("DefaultWorkerPath() = %q", got)
	}
	if !thirdPartyNoticeAvailable() {
		t.Fatal("thirdPartyNoticeAvailable() = false from repository root")
	}
}

func TestPinetsWorkerClientProtocolBoundaryFailures(t *testing.T) {
	t.Run("engine info surfaces startup failure", func(t *testing.T) {
		client := NewPinetsWorkerClient("node", filepath.Join(t.TempDir(), "missing-worker.mjs"))
		if _, err := client.EngineInfo(t.Context()); err == nil || !strings.Contains(err.Error(), "script unavailable") {
			t.Fatalf("EngineInfo(missing worker) error = %v", err)
		}
	})

	t.Run("unsupported params fail before write", func(t *testing.T) {
		client, writer := testPinetsClientWithReader(`{"id":1,"ok":true,"result":{}}` + "\n")
		err := client.call(t.Context(), "unsupported", func() {}, nil)
		if err == nil || !strings.Contains(err.Error(), "unsupported type") {
			t.Fatalf("call(unsupported params) error = %v", err)
		}
		if writer.Len() != 0 {
			t.Fatalf("unsupported params wrote %d bytes", writer.Len())
		}
	})

	t.Run("malformed response", func(t *testing.T) {
		client, _ := testPinetsClientWithReader("not-json\n")
		var info EngineInfo
		err := client.call(t.Context(), "engineInfo", nil, &info)
		if err == nil || !strings.Contains(err.Error(), "decode pinets worker response") {
			t.Fatalf("call(malformed response) error = %v", err)
		}
	})

	t.Run("successful command without result target", func(t *testing.T) {
		client, _ := testPinetsClientWithReader(`{"id":1,"ok":true,"result":{"ignored":true}}` + "\n")
		if err := client.call(context.Background(), "health", nil, nil); err != nil {
			t.Fatalf("call(nil result) error = %v", err)
		}
	})

	t.Run("invalid node executable", func(t *testing.T) {
		workerPath := filepath.Join(t.TempDir(), "worker.mjs")
		if err := os.WriteFile(workerPath, []byte("process.exit(0)"), 0o600); err != nil {
			t.Fatalf("write worker script: %v", err)
		}
		client := NewPinetsWorkerClient(filepath.Join(t.TempDir(), "missing-node"), workerPath)
		if err := client.ensureStartedLocked(t.Context()); err == nil || !strings.Contains(err.Error(), "start pinets worker") {
			t.Fatalf("ensureStartedLocked(invalid node) error = %v", err)
		}
	})
}
