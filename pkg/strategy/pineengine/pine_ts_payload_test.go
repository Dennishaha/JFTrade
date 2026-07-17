package pineengine

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExternalModeFromEnvAndDisabledShadowPayload(t *testing.T) {
	t.Setenv("JFTRADE_PINETS_MODE", " shadow ")
	if got := ExternalModeFromEnv(); got != ModeShadow {
		t.Fatalf("ExternalModeFromEnv(shadow) = %q, want shadow", got)
	}
	t.Setenv("JFTRADE_PINETS_MODE", "community-agpl")
	if got := ExternalModeFromEnv(); got != ModeCommunityAGPL {
		t.Fatalf("ExternalModeFromEnv(community-agpl) = %q", got)
	}
	t.Setenv("JFTRADE_PINETS_MODE", "unknown")
	if got := ExternalModeFromEnv(); got != ModeOff {
		t.Fatalf("ExternalModeFromEnv(unknown) = %q, want off", got)
	}

	t.Setenv("JFTRADE_PINETS_MODE", "off")
	payload := ShadowPayloadForScript("plot(close)")
	if payload.Enabled || payload.Status != "disabled" || payload.DifferenceSummary["evaluated"] != false {
		t.Fatalf("disabled shadow payload = %#v", payload)
	}
}

func TestShadowPayloadReportsWorkerStartupFailure(t *testing.T) {
	t.Setenv("JFTRADE_PINETS_MODE", "shadow")
	t.Setenv("JFTRADE_PINETS_WORKER_PATH", filepath.Join(t.TempDir(), "missing-worker.mjs"))

	payload := ShadowPayloadForScript("plot(close)")
	if !payload.Enabled || payload.OK || payload.Status != "shadow_error" {
		t.Fatalf("shadow startup failure payload = %#v", payload)
	}
	if payload.Repository != "https://github.com/LuxAlgo/PineTS" {
		t.Fatalf("Repository = %q", payload.Repository)
	}
	if len(payload.Diagnostics) != 1 || !strings.Contains(payload.Diagnostics[0].Message, "pinets worker script unavailable") {
		t.Fatalf("Diagnostics = %#v", payload.Diagnostics)
	}
}

func TestShadowPayloadRunsConfiguredWorkerAndReturnsExternalResult(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable: %v", err)
	}
	t.Setenv("JFTRADE_PINETS_MODE", ModeShadow)
	t.Setenv("JFTRADE_PINETS_WORKER_PATH", "")
	payload := ShadowPayloadForScript(`//@version=6
indicator("Shadow payload")
plot(close)`)
	if !payload.Enabled || !payload.OK || payload.Mode != ModeShadow || payload.Status != "shadow_ok" {
		t.Fatalf("shadow payload = %#v", payload)
	}
	if payload.Repository != "https://github.com/LuxAlgo/PineTS" || payload.DifferenceSummary["plots"] == 0 {
		t.Fatalf("shadow payload metadata/result = %#v", payload)
	}
}

func TestCommunityAGPLModeBlocksExecutionWhenNoticeCannotBeFound(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller did not expose the package source path")
	}
	notice := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "legal", "third-party-notices.md"))
	hiddenNotice := notice + ".coverage-test-hidden"
	if err := os.Rename(notice, hiddenNotice); err != nil {
		t.Fatalf("temporarily hide third-party notice: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Rename(hiddenNotice, notice); err != nil {
			t.Fatalf("restore third-party notice: %v", err)
		}
	})
	t.Setenv("JFTRADE_PINETS_MODE", ModeCommunityAGPL)
	payload := ShadowPayloadForScript("plot(close)")
	if !payload.Enabled || payload.OK || payload.Mode != ModeCommunityAGPL || payload.Status != "compliance_error" {
		t.Fatalf("community notice failure payload = %#v", payload)
	}
	if len(payload.Diagnostics) != 1 || payload.Diagnostics[0].Code != "PINETS_AGPL_NOTICE_MISSING" {
		t.Fatalf("community notice diagnostics = %#v", payload.Diagnostics)
	}
}

func TestExternalEnginePayloadFromResultMapsSuccessAndFailure(t *testing.T) {
	success := ExternalEnginePayloadFromResult(ModeShadow, RunIndicatorResponse{
		OK:            true,
		EngineVersion: "pinets-test",
		License:       "AGPL-3.0-only",
		Diagnostics: []Diagnostic{{
			Severity: "warning",
			Code:     "TEST_WARN",
			Message:  "mapped through",
		}},
		Plots:   map[string]Plot{"SMA": {Title: "SMA", Data: []any{1.0, 2.0}}},
		Signals: map[string]any{"cross": true},
	}, nil)
	if !success.Enabled || !success.OK || success.Status != "shadow_ok" {
		t.Fatalf("success payload = %#v", success)
	}
	if success.Engine != PinetsShadowEngineID || success.DifferenceSummary["plots"] != 1 || success.DifferenceSummary["signals"] != 1 {
		t.Fatalf("success payload summary = %#v", success)
	}

	failure := ExternalEnginePayloadFromResult(ModeCommunityAGPL, RunIndicatorResponse{}, errors.New("worker crashed"))
	if !failure.Enabled || failure.OK || failure.Status != "shadow_error" {
		t.Fatalf("failure payload = %#v", failure)
	}
	if len(failure.Diagnostics) != 1 || !strings.Contains(failure.Diagnostics[0].Message, "worker crashed") {
		t.Fatalf("failure diagnostics = %#v", failure.Diagnostics)
	}

	payloadMap := PayloadMap(success)
	if payloadMap["enabled"] != true || payloadMap["mode"] != ModeShadow || payloadMap["status"] != "shadow_ok" {
		t.Fatalf("PayloadMap(success) = %#v", payloadMap)
	}
}

func TestPinetsWorkerClientCallHandlesProtocolFailures(t *testing.T) {
	t.Run("read failure includes stderr", func(t *testing.T) {
		client, writer := testPinetsClientWithReader("")
		client.stderr.WriteString("syntax error on stderr\n")
		var info EngineInfo
		err := client.call(context.Background(), "engineInfo", nil, &info)
		if err == nil || !strings.Contains(err.Error(), "syntax error on stderr") {
			t.Fatalf("call() error = %v, want stderr suffix", err)
		}
		if !writer.closed {
			t.Fatal("stdin was not closed after read failure")
		}
	})

	t.Run("mismatched response id closes worker", func(t *testing.T) {
		client, writer := testPinetsClientWithReader(`{"id":99,"ok":true,"result":{}}` + "\n")
		var info EngineInfo
		err := client.call(context.Background(), "engineInfo", nil, &info)
		if err == nil || !strings.Contains(err.Error(), "response id = 99") {
			t.Fatalf("call() error = %v, want response id mismatch", err)
		}
		if !writer.closed || client.stdin != nil || client.reader != nil {
			t.Fatalf("client was not closed after mismatched id: writer.closed=%v stdin=%v reader=%v", writer.closed, client.stdin, client.reader)
		}
	})

	t.Run("worker error gets default message", func(t *testing.T) {
		client, _ := testPinetsClientWithReader(`{"id":1,"ok":false,"error":{"code":"PINETS_FAIL"}}` + "\n")
		err := client.call(context.Background(), "runIndicator", RunIndicatorRequest{}, nil)
		if err == nil || err.Error() != "PINETS_FAIL: pinets worker failed" {
			t.Fatalf("call() worker error = %v", err)
		}
	})

	t.Run("invalid result json reports decode context", func(t *testing.T) {
		client, _ := testPinetsClientWithReader(`{"id":1,"ok":true,"result":123}` + "\n")
		var info EngineInfo
		err := client.call(context.Background(), "engineInfo", nil, &info)
		if err == nil || !strings.Contains(err.Error(), "decode pinets worker result") {
			t.Fatalf("call() result decode error = %v", err)
		}
	})

	t.Run("write failure closes worker", func(t *testing.T) {
		client := &PinetsWorkerClient{
			cmd:    &exec.Cmd{},
			stdin:  testFailingWriteCloser{},
			reader: bufio.NewReader(strings.NewReader(`{"id":1,"ok":true,"result":{}}` + "\n")),
		}
		err := client.call(context.Background(), "engineInfo", nil, nil)
		if err == nil || !strings.Contains(err.Error(), "write pinets worker request") {
			t.Fatalf("call() write error = %v", err)
		}
		if client.stdin != nil || client.reader != nil {
			t.Fatalf("client was not closed after write failure: stdin=%v reader=%v", client.stdin, client.reader)
		}
	})
}

func TestWorkerErrorStringAndStderrSuffix(t *testing.T) {
	if got := (workerError{Message: "plain failure"}).Error(); got != "plain failure" {
		t.Fatalf("workerError without code = %q", got)
	}
	if got := (workerError{Code: "E", Message: "with code"}).Error(); got != "E: with code" {
		t.Fatalf("workerError with code = %q", got)
	}
	client := &PinetsWorkerClient{}
	if got := client.stderrSuffix(); got != "" {
		t.Fatalf("empty stderr suffix = %q", got)
	}
	client.stderr.WriteString("first\nsecond\n")
	if got := client.stderrSuffix(); got != ": first\nsecond" {
		t.Fatalf("stderr suffix = %q", got)
	}
}

func TestPinetsWorkerConfigurationAndStartupErrors(t *testing.T) {
	t.Setenv("JFTRADE_PINETS_WORKER_PATH", "/tmp/custom-pinets-worker.mjs")
	if got := DefaultWorkerPath(); got != "/tmp/custom-pinets-worker.mjs" {
		t.Fatalf("DefaultWorkerPath(env) = %q", got)
	}
	if !thirdPartyNoticeAvailable() {
		t.Fatal("thirdPartyNoticeAvailable() = false, want repository notice to be discoverable")
	}

	client := NewPinetsWorkerClient("node", "/path/does/not/exist.mjs")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.ensureStartedLocked(ctx); err == nil || !strings.Contains(err.Error(), "pinets worker script unavailable") {
		t.Fatalf("ensureStartedLocked(missing worker) error = %v", err)
	}
}

func TestPinetsWorkerClientCapturesBoundedStderr(t *testing.T) {
	client := &PinetsWorkerClient{}
	input := strings.Repeat("worker diagnostic line\n", 300)
	client.captureStderr(strings.NewReader(input))
	if client.stderr.Len() == 0 {
		t.Fatal("captureStderr did not record stderr")
	}
	if client.stderr.Len() > 4096+len("worker diagnostic line\n") {
		t.Fatalf("captured stderr length = %d, want bounded near 4096", client.stderr.Len())
	}
}

func testPinetsClientWithReader(stdout string) (*PinetsWorkerClient, *testWriteCloser) {
	writer := &testWriteCloser{}
	return &PinetsWorkerClient{
		cmd:    &exec.Cmd{},
		stdin:  writer,
		reader: bufio.NewReader(strings.NewReader(stdout)),
	}, writer
}

type testWriteCloser struct {
	bytes.Buffer
	closed bool
}

func (w *testWriteCloser) Close() error {
	w.closed = true
	return nil
}

var _ io.WriteCloser = (*testWriteCloser)(nil)

type testFailingWriteCloser struct{}

func (testFailingWriteCloser) Write(_ []byte) (int, error) {
	return 0, errors.New("disk full")
}

func (testFailingWriteCloser) Close() error {
	return nil
}

var _ io.WriteCloser = testFailingWriteCloser{}
