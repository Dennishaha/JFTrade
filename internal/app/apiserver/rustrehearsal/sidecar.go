// Package rustrehearsal owns the private Rust product sidecar process used by
// explicit migration rehearsals. It never selects a public production owner.
package rustrehearsal

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	EnvProfile            = "JFTRADE_RUST_REHEARSAL_PROFILE"
	EnvExecutable         = "JFTRADE_RUST_API_EXECUTABLE"
	ReadOnlyProfile       = "read-only-shadow.v1"
	ProtocolVersion       = "jftrade-product-rehearsal.v1"
	InternalProxyProtocol = "jftrade-go-rehearsal.v1"
	InternalProxyHeader   = "X-JFTrade-Internal-Proxy"
	AccessSurfaceHeader   = "X-JFTrade-Access-Surface"
	defaultBind           = "127.0.0.1:0"
	defaultReadyLimit     = 5 * time.Second
	defaultStopLimit      = 5 * time.Second
	defaultKillLimit      = 2 * time.Second
	maxReadyBytes         = 64 * 1024
)

var readOnlyCapabilities = []string{
	"GET /api/v1/adk/agent-templates",
	"GET /api/v1/research/screens/catalog",
	"GET /api/v1/settings/adk",
	"GET /api/v1/settings/adk/mcp",
	"GET /api/v1/settings/backtest-market-data-provider",
	"GET /api/v1/settings/brokers",
	"GET /api/v1/settings/data-management/databases",
	"GET /api/v1/settings/exchange-calendars",
	"GET /api/v1/settings/execution",
	"GET /api/v1/settings/market-data-provider",
	"GET /api/v1/settings/onboarding",
	"GET /api/v1/settings/pine-worker",
	"GET /api/v1/settings/security",
	"GET /api/v1/settings/system-notifications",
	"GET /api/v1/settings/ui",
	"GET /api/v1/system/futu-opend/install-guide",
	"GET /api/v1/system/real-trade-approvals",
	"GET /api/v1/system/real-trade-hard-stop-events",
	"GET /api/v1/system/real-trade-hard-stops",
	"GET /api/v1/system/real-trade-kill-switch",
	"GET /api/v1/system/real-trade-kill-switch-events",
	"GET /api/v1/system/real-trade-risk-events",
	"GET /api/v1/system/real-trade-risk-limits",
	"GET /api/v1/system/runtime-dependencies",
	"GET /api/v1/system/status",
	"GET /api/v1/system/storage/overview",
}

// Config contains composition-owned process inputs. None are user settings.
type Config struct {
	Profile      string
	Executable   string
	Arguments    []string
	Environment  []string
	SettingsPath string
	Bind         string
	ReadyTimeout time.Duration
	StopTimeout  time.Duration
	KillTimeout  time.Duration
}

type readyRecord struct {
	Event              string   `json:"event"`
	Address            string   `json:"address"`
	Owner              string   `json:"owner"`
	OwnedRoutes        int      `json:"ownedRoutes"`
	ProtocolVersion    string   `json:"protocolVersion"`
	RouteProfile       string   `json:"routeProfile"`
	RouteProfileDigest string   `json:"routeProfileDigest"`
	Capabilities       []string `json:"capabilities"`
	ResourceSHA256     string   `json:"resourceSha256"`
}

// Handle owns one verified Rust sidecar and its private routing credentials.
type Handle struct {
	cmd          *exec.Cmd
	done         chan struct{}
	endpoint     string
	token        string
	profile      string
	capabilities []string
	stopTimeout  time.Duration
	killTimeout  time.Duration

	mu        sync.Mutex
	waitErr   error
	stopping  bool
	closeOnce sync.Once
	closeErr  error
}

// StartFromEnvironment returns nil when rehearsal is not explicitly enabled.
func StartFromEnvironment(ctx context.Context, settingsPath string) (*Handle, error) {
	profile := strings.TrimSpace(os.Getenv(EnvProfile))
	if profile == "" {
		return nil, nil
	}
	executable, err := resolveExecutable(strings.TrimSpace(os.Getenv(EnvExecutable)))
	if err != nil {
		return nil, err
	}
	return Start(ctx, Config{
		Profile:      profile,
		Executable:   executable,
		SettingsPath: settingsPath,
	})
}

// Start launches, authenticates, and fully validates one sidecar before it is
// made available to the Go router.
func Start(ctx context.Context, config Config) (*Handle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	config = normalizeConfig(config)
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	resourceHash, err := sha256File(config.Executable)
	if err != nil {
		return nil, err
	}
	token, err := randomToken()
	if err != nil {
		return nil, fmt.Errorf("generate Rust rehearsal bearer: %w", err)
	}
	cmd := exec.Command(config.Executable, config.Arguments...)
	cmd.Env = append(os.Environ(), config.Environment...)
	cmd.Env = append(cmd.Env,
		"JFTRADE_RUST_API_BIND="+config.Bind,
		"JFTRADE_SETTINGS_PATH="+config.SettingsPath,
		"JFTRADE_DESKTOP_TOKEN="+token,
		"JFTRADE_RUST_INTERNAL_PROXY_PROTOCOL="+InternalProxyProtocol,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open Rust rehearsal readiness pipe: %w", err)
	}
	cmd.Stderr = log.Writer()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Rust rehearsal sidecar: %w", err)
	}
	handle := &Handle{
		cmd: cmd, done: make(chan struct{}), token: token, profile: config.Profile,
		stopTimeout: config.StopTimeout, killTimeout: config.KillTimeout,
	}
	go handle.wait()
	record, err := awaitReady(ctx, handle, stdout, config.ReadyTimeout)
	if err == nil {
		err = validateReady(record, config.Profile, resourceHash)
	}
	if err == nil {
		handle.endpoint = "http://" + record.Address
		handle.capabilities = append([]string(nil), record.Capabilities...)
		err = probeAuthenticated(ctx, handle.endpoint, token, config.ReadyTimeout)
	}
	if err != nil {
		return nil, errors.Join(err, handle.Close())
	}
	return handle, nil
}

func normalizeConfig(config Config) Config {
	config.Profile = strings.TrimSpace(config.Profile)
	config.Executable = strings.TrimSpace(config.Executable)
	config.SettingsPath = strings.TrimSpace(config.SettingsPath)
	config.Bind = strings.TrimSpace(config.Bind)
	if config.Bind == "" {
		config.Bind = defaultBind
	}
	if config.ReadyTimeout <= 0 {
		config.ReadyTimeout = defaultReadyLimit
	}
	if config.StopTimeout <= 0 {
		config.StopTimeout = defaultStopLimit
	}
	if config.KillTimeout <= 0 {
		config.KillTimeout = defaultKillLimit
	}
	return config
}

func validateConfig(config Config) error {
	if config.Profile != ReadOnlyProfile {
		return fmt.Errorf("unsupported Rust rehearsal profile %q", config.Profile)
	}
	if config.Executable == "" || !filepath.IsAbs(config.Executable) {
		return fmt.Errorf("Rust rehearsal executable must be an absolute path")
	}
	info, err := os.Stat(config.Executable)
	if err != nil {
		return fmt.Errorf("inspect Rust rehearsal executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("Rust rehearsal executable must be a regular file")
	}
	if config.SettingsPath == "" {
		return fmt.Errorf("Rust rehearsal settings path is required")
	}
	host, _, err := net.SplitHostPort(config.Bind)
	if err != nil {
		return fmt.Errorf("invalid Rust rehearsal bind %q: %w", config.Bind, err)
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("Rust rehearsal bind must use a loopback IP")
	}
	return nil
}

func awaitReady(ctx context.Context, handle *Handle, stdout io.Reader, timeout time.Duration) (readyRecord, error) {
	type result struct {
		record readyRecord
		err    error
	}
	ready := make(chan result, 1)
	go func() {
		scanner := bufio.NewScanner(io.LimitReader(stdout, maxReadyBytes+1))
		scanner.Buffer(make([]byte, 1024), maxReadyBytes)
		if !scanner.Scan() {
			err := scanner.Err()
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			ready <- result{err: fmt.Errorf("read Rust rehearsal readiness: %w", err)}
			return
		}
		var record readyRecord
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil {
			ready <- result{err: fmt.Errorf("decode Rust rehearsal readiness: %w", err)}
			return
		}
		ready <- result{record: record}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-ready:
		return result.record, result.err
	case <-handle.done:
		return readyRecord{}, fmt.Errorf("Rust rehearsal exited before readiness: %w", handle.processError())
	case <-ctx.Done():
		return readyRecord{}, fmt.Errorf("Rust rehearsal startup canceled: %w", ctx.Err())
	case <-timer.C:
		return readyRecord{}, fmt.Errorf("Rust rehearsal readiness timed out after %s", timeout)
	}
}

func validateReady(record readyRecord, profile string, resourceHash string) error {
	if record.Event != "ready" || record.Owner != "rust-read-only-shadow" {
		return fmt.Errorf("Rust rehearsal reported invalid event or owner")
	}
	if record.ProtocolVersion != ProtocolVersion || record.RouteProfile != profile {
		return fmt.Errorf("Rust rehearsal protocol or route profile mismatch")
	}
	if record.OwnedRoutes != len(readOnlyCapabilities) || !slices.Equal(record.Capabilities, readOnlyCapabilities) {
		return fmt.Errorf("Rust rehearsal capability list mismatch")
	}
	if record.RouteProfileDigest != capabilityDigest(readOnlyCapabilities) {
		return fmt.Errorf("Rust rehearsal route profile digest mismatch")
	}
	if record.ResourceSHA256 != resourceHash {
		return fmt.Errorf("Rust rehearsal resource hash mismatch")
	}
	host, port, err := net.SplitHostPort(record.Address)
	if err != nil || port == "0" || port == "" {
		return fmt.Errorf("Rust rehearsal reported invalid address %q", record.Address)
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("Rust rehearsal reported non-loopback address %q", record.Address)
	}
	return nil
}

func probeAuthenticated(ctx context.Context, endpoint string, token string, timeout time.Duration) error {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint+"/api/v1/system/status", nil)
	if err != nil {
		return fmt.Errorf("create Rust rehearsal authenticated probe: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(InternalProxyHeader, InternalProxyProtocol)
	req.Header.Set(AccessSurfaceHeader, "desktop")
	response, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return fmt.Errorf("probe Rust rehearsal readiness: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Rust rehearsal authenticated probe returned status %d", response.StatusCode)
	}
	return nil
}

func (h *Handle) wait() {
	err := h.cmd.Wait()
	h.mu.Lock()
	h.waitErr = err
	stopping := h.stopping
	h.mu.Unlock()
	close(h.done)
	if !stopping {
		log.Printf("JFTrade Rust rehearsal sidecar exited unexpectedly: %v", err)
	}
}

// Endpoint returns the verified private loopback base URL.
func (h *Handle) Endpoint() string {
	if h == nil {
		return ""
	}
	return h.endpoint
}

// BearerToken returns the per-process credential for private Go-to-Rust calls.
func (h *Handle) BearerToken() string {
	if h == nil {
		return ""
	}
	return h.token
}

// Profile returns the verified route profile.
func (h *Handle) Profile() string {
	if h == nil {
		return ""
	}
	return h.profile
}

// Capabilities returns a copy of the exact verified operation set.
func (h *Handle) Capabilities() []string {
	if h == nil {
		return nil
	}
	return append([]string(nil), h.capabilities...)
}

// Close stops and reaps the child. The lock-free OS process ownership means a
// crash cannot leave an unreaped process owned by this Go parent.
func (h *Handle) Close() error {
	if h == nil {
		return nil
	}
	h.closeOnce.Do(func() { h.closeErr = h.closeProcess() })
	return h.closeErr
}

func (h *Handle) closeProcess() error {
	h.mu.Lock()
	h.stopping = true
	h.mu.Unlock()
	select {
	case <-h.done:
		return normalizeWaitError(h.processError())
	default:
	}
	killed, stopErr := requestProcessStop(h.cmd.Process)
	waitLimit := h.stopTimeout
	if killed {
		waitLimit = h.killTimeout
	}
	if waitDone(h.done, waitLimit) {
		return errors.Join(stopErr, normalizeWaitError(h.processError()))
	}
	killErr := h.cmd.Process.Kill()
	if !waitDone(h.done, h.killTimeout) {
		return errors.Join(stopErr, killErr, fmt.Errorf("reap Rust rehearsal sidecar timed out"))
	}
	return errors.Join(stopErr, normalizeProcessStopError(killErr), normalizeWaitError(h.processError()))
}

func (h *Handle) processError() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.waitErr
}

func waitDone(done <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func normalizeWaitError(err error) error {
	var exitErr *exec.ExitError
	if err == nil || errors.As(err, &exitErr) {
		return nil
	}
	return err
}

func normalizeProcessStopError(err error) error {
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func resolveExecutable(override string) (string, error) {
	if override != "" {
		if !filepath.IsAbs(override) {
			return "", fmt.Errorf("%s must be an absolute path", EnvExecutable)
		}
		return filepath.Clean(override), nil
	}
	name := "jftrade-api-rust"
	if filepath.Ext(os.Args[0]) == ".exe" {
		name += ".exe"
	}
	if current, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(current), name)
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	candidate, err := filepath.Abs(filepath.Join("target", "debug", name))
	if err == nil {
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("resolve Rust rehearsal executable: set %s to a built jftrade-api-rust binary", EnvExecutable)
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open Rust rehearsal executable for hashing: %w", err)
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("hash Rust rehearsal executable: %w", err)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func capabilityDigest(capabilities []string) string {
	digest := sha256.New()
	for _, capability := range capabilities {
		_, _ = io.WriteString(digest, capability)
		_, _ = io.WriteString(digest, "\n")
	}
	return hex.EncodeToString(digest.Sum(nil))
}
