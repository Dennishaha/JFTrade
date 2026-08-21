package rustrehearsal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const helperModeEnv = "JFTRADE_TEST_RUST_REHEARSAL_HELPER"

func TestStartValidatesReadyTokenAndReapsTheChild(t *testing.T) {
	if got := capabilityDigest(readOnlyCapabilities); got != "5f5654f93253a014d0ea113168bd49c88454f5c4c214ae9a72102a539ccf74cd" {
		t.Fatalf("read-only route profile digest = %q", got)
	}
	handle, err := Start(t.Context(), helperConfig(t, "ready"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if handle.Profile() != ReadOnlyProfile || !strings.HasPrefix(handle.Endpoint(), "http://127.0.0.1:") {
		t.Fatalf("verified runtime = profile %q endpoint %q", handle.Profile(), handle.Endpoint())
	}
	if len(handle.BearerToken()) != 64 || len(handle.Capabilities()) != len(readOnlyCapabilities) {
		t.Fatalf("verified credentials/capabilities are incomplete")
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-handle.done:
	case <-time.After(time.Second):
		t.Fatal("child was not reaped")
	}
}

func TestStartFailsClosedAndReapsMalformedOrMismatchedReady(t *testing.T) {
	for _, mode := range []string{"malformed", "capability-mismatch", "token-mismatch", "crash"} {
		t.Run(mode, func(t *testing.T) {
			_, err := Start(t.Context(), helperConfig(t, mode))
			if err == nil {
				t.Fatal("Start unexpectedly succeeded")
			}
		})
	}
}

func TestStartFailsClosedWhenRequestedPortIsOccupied(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	config := helperConfig(t, "ready")
	config.Bind = listener.Addr().String()
	if _, err := Start(t.Context(), config); err == nil {
		t.Fatal("Start unexpectedly succeeded on an occupied port")
	}
}

func TestStartFromEnvironmentIsDisabledByDefaultAndRejectsUnknownProfiles(t *testing.T) {
	t.Setenv(EnvProfile, "")
	handle, err := StartFromEnvironment(t.Context(), filepath.Join(t.TempDir(), "settings.json"))
	if err != nil || handle != nil {
		t.Fatalf("disabled runtime = %#v, %v", handle, err)
	}
	t.Setenv(EnvProfile, "unknown")
	t.Setenv(EnvExecutable, helperExecutable(t))
	if _, err := StartFromEnvironment(t.Context(), filepath.Join(t.TempDir(), "settings.json")); err == nil {
		t.Fatal("unknown profile unexpectedly started")
	}
}

func TestSidecarConfigAndReadyValidationFailClosed(t *testing.T) {
	executable := helperExecutable(t)
	valid := Config{
		Profile: ReadOnlyProfile, Executable: executable,
		SettingsPath: filepath.Join(t.TempDir(), "settings.json"), Bind: "127.0.0.1:0",
	}
	invalidConfigs := []Config{
		{Profile: "unknown", Executable: executable, SettingsPath: valid.SettingsPath, Bind: valid.Bind},
		{Profile: ReadOnlyProfile, Executable: "relative", SettingsPath: valid.SettingsPath, Bind: valid.Bind},
		{Profile: ReadOnlyProfile, Executable: filepath.Join(t.TempDir(), "missing"), SettingsPath: valid.SettingsPath, Bind: valid.Bind},
		{Profile: ReadOnlyProfile, Executable: t.TempDir(), SettingsPath: valid.SettingsPath, Bind: valid.Bind},
		{Profile: ReadOnlyProfile, Executable: executable, Bind: valid.Bind},
		{Profile: ReadOnlyProfile, Executable: executable, SettingsPath: valid.SettingsPath, Bind: "invalid"},
		{Profile: ReadOnlyProfile, Executable: executable, SettingsPath: valid.SettingsPath, Bind: "0.0.0.0:1"},
	}
	for index, config := range invalidConfigs {
		if err := validateConfig(config); err == nil {
			t.Fatalf("invalid config %d was accepted", index)
		}
	}
	if err := validateConfig(valid); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	resourceHash, err := sha256File(executable)
	if err != nil {
		t.Fatalf("hash helper: %v", err)
	}
	base := readyRecord{
		Event: "ready", Address: "127.0.0.1:1234", Owner: "rust-read-only-shadow",
		OwnedRoutes: len(readOnlyCapabilities), ProtocolVersion: ProtocolVersion, RouteProfile: ReadOnlyProfile,
		RouteProfileDigest: capabilityDigest(readOnlyCapabilities),
		Capabilities:       append([]string(nil), readOnlyCapabilities...), ResourceSHA256: resourceHash,
	}
	mutations := []func(*readyRecord){
		func(record *readyRecord) { record.Event = "wrong" },
		func(record *readyRecord) { record.ProtocolVersion = "wrong" },
		func(record *readyRecord) { record.OwnedRoutes-- },
		func(record *readyRecord) { record.RouteProfileDigest = "wrong" },
		func(record *readyRecord) { record.ResourceSHA256 = "wrong" },
		func(record *readyRecord) { record.Address = "invalid" },
		func(record *readyRecord) { record.Address = "0.0.0.0:1234" },
	}
	for index, mutate := range mutations {
		record := base
		record.Capabilities = append([]string(nil), base.Capabilities...)
		mutate(&record)
		if err := validateReady(record, ReadOnlyProfile, resourceHash); err == nil {
			t.Fatalf("invalid ready record %d was accepted", index)
		}
	}
	if err := validateReady(base, ReadOnlyProfile, resourceHash); err != nil {
		t.Fatalf("valid ready record rejected: %v", err)
	}
}

func TestSidecarHelpersCoverNilTimeoutAndPathBoundaries(t *testing.T) {
	var handle *Handle
	if handle.Endpoint() != "" || handle.BearerToken() != "" || handle.Profile() != "" || handle.Capabilities() != nil {
		t.Fatal("nil handle exposed runtime state")
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("nil handle close: %v", err)
	}
	closed := make(chan struct{})
	close(closed)
	if !waitDone(closed, time.Second) || waitDone(make(chan struct{}), time.Millisecond) {
		t.Fatal("waitDone did not distinguish completion from timeout")
	}
	if normalizeWaitError(nil) != nil || normalizeWaitError(&exec.ExitError{}) != nil {
		t.Fatal("expected process exits must normalize to nil")
	}
	sentinel := fmt.Errorf("sentinel")
	if !errors.Is(normalizeWaitError(sentinel), sentinel) ||
		!errors.Is(normalizeProcessStopError(sentinel), sentinel) ||
		normalizeProcessStopError(os.ErrProcessDone) != nil {
		t.Fatal("unexpected process error normalization")
	}
	if _, err := resolveExecutable("relative"); err == nil {
		t.Fatal("relative executable override was accepted")
	}
	abs := filepath.Join(t.TempDir(), "..", "binary")
	resolved, err := resolveExecutable(abs)
	if err != nil || resolved != filepath.Clean(abs) {
		t.Fatalf("absolute override = %q, %v", resolved, err)
	}
	if _, err := sha256File(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing executable hash succeeded")
	}
	if _, err := sha256File(t.TempDir()); err == nil {
		t.Fatal("directory executable hash succeeded")
	}
	request, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	if verifiedAccessSurface(request) != "" {
		t.Fatal("unverified request surface was trusted")
	}
}

func TestBuiltRustProductSidecarCompletesTheGoReadinessHandshake(t *testing.T) {
	executable := strings.TrimSpace(os.Getenv("JFTRADE_TEST_RUST_REHEARSAL_EXECUTABLE"))
	if executable == "" {
		t.Skip("set JFTRADE_TEST_RUST_REHEARSAL_EXECUTABLE to exercise the built Rust binary")
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	handle, err := Start(t.Context(), Config{
		Profile: ReadOnlyProfile, Executable: executable, SettingsPath: settingsPath,
		ReadyTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Start built Rust sidecar: %v", err)
	}
	if len(handle.Capabilities()) != 26 {
		t.Fatalf("capabilities = %d", len(handle.Capabilities()))
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("Close built Rust sidecar: %v", err)
	}
}

func helperConfig(t *testing.T, mode string) Config {
	t.Helper()
	return Config{
		Profile:      ReadOnlyProfile,
		Executable:   helperExecutable(t),
		Arguments:    []string{"-test.run=^TestRustRehearsalHelperProcess$"},
		Environment:  []string{helperModeEnv + "=" + mode},
		SettingsPath: filepath.Join(t.TempDir(), "settings.json"),
		ReadyTimeout: 2 * time.Second,
		StopTimeout:  500 * time.Millisecond,
		KillTimeout:  500 * time.Millisecond,
	}
}

func helperExecutable(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return executable
}

func TestRustRehearsalHelperProcess(t *testing.T) {
	mode := os.Getenv(helperModeEnv)
	if mode == "" {
		return
	}
	if mode == "malformed" {
		_, _ = fmt.Fprintln(os.Stdout, "not-json")
		time.Sleep(time.Hour)
	}
	if mode == "crash" {
		os.Exit(17)
	}
	listener, err := net.Listen("tcp", os.Getenv("JFTRADE_RUST_API_BIND"))
	if err != nil {
		os.Exit(18)
	}
	resourceHash, err := sha256File(helperExecutable(t))
	if err != nil {
		os.Exit(19)
	}
	capabilities := append([]string(nil), readOnlyCapabilities...)
	if mode == "capability-mismatch" {
		capabilities = capabilities[:len(capabilities)-1]
	}
	record := readyRecord{
		Event: "ready", Address: listener.Addr().String(), Owner: "rust-read-only-shadow",
		OwnedRoutes: len(capabilities), ProtocolVersion: ProtocolVersion, RouteProfile: ReadOnlyProfile,
		RouteProfileDigest: capabilityDigest(capabilities), Capabilities: capabilities,
		ResourceSHA256: resourceHash,
	}
	if err := json.NewEncoder(os.Stdout).Encode(record); err != nil {
		os.Exit(20)
	}
	expectedToken := os.Getenv("JFTRADE_DESKTOP_TOKEN")
	if mode == "token-mismatch" {
		expectedToken = "different-token"
	}
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+expectedToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		os.Exit(21)
	}
	_ = server.Shutdown(context.Background())
}
