package rustrehearsal

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
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
