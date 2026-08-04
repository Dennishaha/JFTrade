package marketdataapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdataassets"
)

const (
	sidecarStopTimeout = 5 * time.Second
	sidecarKillTimeout = 2 * time.Second
	sidecarHost        = "127.0.0.1"
	sourceProbeTimeout = 3 * time.Second
)

// SidecarConfig contains application-owned process arguments. It is never
// loaded from user settings.
type SidecarConfig struct {
	Executable  string
	Arguments   []string
	Environment []string
	Host        string
	Port        int
}

type sidecarLifecycle interface {
	EnsureStarted() (string, error)
	Stop() error
	Close() error
}

type sidecarProcess interface {
	Running() bool
	Close() error
}

type sidecarStarter func(SidecarConfig) (sidecarProcess, error)

type sidecarExecutable struct {
	path        string
	arguments   []string
	environment []string
	started     func()
	cleanup     func() error
}

type sidecarExecutableResolver func() (sidecarExecutable, error)
type sidecarPortAllocator func() (int, error)

var (
	ErrMarketDataSidecarUnavailable = errors.New("embedded market-data sidecar is unavailable")
	// ErrYFinanceSidecarUnavailable is retained for source compatibility.
	ErrYFinanceSidecarUnavailable = ErrMarketDataSidecarUnavailable
)

type sidecarManager struct {
	mu           sync.Mutex
	endpoint     string
	process      sidecarProcess
	cleanup      func() error
	resolve      sidecarExecutableResolver
	allocatePort sidecarPortAllocator
	start        sidecarStarter
	cacheDir     string
}

func newSidecarManager(cacheDir string) *sidecarManager {
	manager := &sidecarManager{
		allocatePort: allocateMarketDataSidecarPort,
		start:        startMarketDataSidecar,
		cacheDir:     strings.TrimSpace(cacheDir),
	}
	manager.resolve = func() (sidecarExecutable, error) {
		return resolveMarketDataSidecarExecutable(manager.cacheDir)
	}
	return manager
}

func (m *sidecarManager) EnsureStarted() (string, error) {
	if m == nil {
		return "", fmt.Errorf("market-data sidecar manager is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process != nil && m.process.Running() && m.endpoint != "" {
		return m.endpoint, nil
	}
	if err := m.stopLocked(); err != nil {
		return "", err
	}
	resolve := m.resolve
	if resolve == nil {
		resolve = func() (sidecarExecutable, error) {
			return resolveMarketDataSidecarExecutable(m.cacheDir)
		}
	}
	executable, err := resolve()
	if err != nil {
		return "", err
	}
	cleanup := executable.cleanup
	if cleanup == nil {
		cleanup = func() error { return nil }
	}
	allocatePort := m.allocatePort
	if allocatePort == nil {
		allocatePort = allocateMarketDataSidecarPort
	}
	port, err := allocatePort()
	if err != nil {
		return "", errors.Join(err, cleanup())
	}
	config := SidecarConfig{
		Executable:  executable.path,
		Arguments:   append([]string(nil), executable.arguments...),
		Environment: append([]string(nil), executable.environment...),
		Host:        sidecarHost,
		Port:        port,
	}
	start := m.start
	if start == nil {
		start = startMarketDataSidecar
	}
	process, err := start(config)
	if err != nil {
		return "", errors.Join(fmt.Errorf("start market-data sidecar: %w", err), cleanup())
	}
	m.endpoint = "http://" + net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	m.process = process
	m.cleanup = cleanup
	if executable.started != nil {
		executable.started()
	}
	return m.endpoint, nil
}

func (m *sidecarManager) Stop() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

func (m *sidecarManager) Close() error {
	return m.Stop()
}

func (m *sidecarManager) stopLocked() error {
	if m == nil {
		return nil
	}
	var stopErr error
	if m.process != nil {
		stopErr = m.process.Close()
		if stopErr != nil {
			return fmt.Errorf("stop market-data sidecar: %w", stopErr)
		}
	}
	cleanup := m.cleanup
	m.endpoint = ""
	m.process = nil
	if cleanup != nil {
		if err := cleanup(); err != nil {
			m.cleanup = cleanup
			return fmt.Errorf("clean up market-data sidecar: %w", err)
		}
	}
	m.cleanup = nil
	return nil
}

func allocateMarketDataSidecarPort() (int, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(sidecarHost, "0"))
	if err != nil {
		return 0, fmt.Errorf("allocate market-data sidecar port: %w", err)
	}
	defer func() { _ = listener.Close() }()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok || address.Port < 1 {
		return 0, fmt.Errorf("allocate market-data sidecar port: invalid listener address %q", listener.Addr())
	}
	return address.Port, nil
}

func resolveMarketDataSidecarExecutable(cacheDir string) (sidecarExecutable, error) {
	cacheDir = strings.TrimSpace(cacheDir)
	path, source := environmentOverride(EnvMarketDataSidecar, EnvYFinanceSidecar)
	if path != "" && marketdataassets.DevelopmentOverridesAllowed() {
		absolute, err := validateAbsoluteRegularFile(path, source)
		if err != nil {
			return sidecarExecutable{}, err
		}
		return sidecarExecutable{path: absolute}, nil
	}
	if marketdataassets.DevelopmentOverridesAllowed() {
		resolution := ResolvePythonRuntime()
		if !resolution.Available || resolution.ResolvedPath == "" {
			return sidecarExecutable{}, fmt.Errorf(
				"resolve market-data Python source runtime: %w",
				pythonRuntimeMissingError(resolution),
			)
		}
		probeCtx, cancel := context.WithTimeout(context.Background(), sourceProbeTimeout)
		probe := ProbePythonRuntime(probeCtx, resolution)
		probeErr := pythonRuntimeProbeError(probeCtx, probe)
		cancel()
		if !probe.Available {
			return sidecarExecutable{}, probeErr
		}
		return sidecarExecutable{
			path: resolution.ResolvedPath, arguments: []string{"-m", "marketdata_sidecar.main"},
			environment: []string{"PYTHONPATH=" + resolution.SourcePath},
		}, nil
	}
	if cacheDir != "" {
		materialized, available, err := marketdataassets.MaterializeCached(cacheDir)
		if err == nil && available && materialized != nil && materialized.Path != "" {
			return sidecarExecutable{
				path: materialized.Path,
				started: func() {
					marketdataassets.PruneCached(cacheDir, materialized.SHA256)
				},
			}, nil
		}
		if err != nil {
			log.Printf("JFTrade persistent market-data sidecar cache unavailable; using temporary asset: %v", err)
		}
	}
	materialized, available, err := marketdataassets.Materialize()
	if err != nil {
		return sidecarExecutable{}, fmt.Errorf("materialize embedded market-data sidecar: %w", err)
	}
	if !available || materialized == nil || materialized.Path == "" {
		return sidecarExecutable{}, ErrMarketDataSidecarUnavailable
	}
	return sidecarExecutable{path: materialized.Path, cleanup: materialized.Cleanup}, nil
}

func pythonRuntimeProbeError(
	ctx context.Context,
	probe PythonRuntimeProbeResult,
) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("validate market-data Python source runtime: timed out")
	}
	if probe.Err != nil {
		return fmt.Errorf("validate market-data Python source runtime: %w", probe.Err)
	}
	if probe.Outdated {
		return fmt.Errorf("validate market-data Python source runtime: Python %s is below 3.11", probe.DetectedVersion)
	}
	if len(probe.MissingModules) > 0 {
		return fmt.Errorf(
			"validate market-data Python source runtime: missing modules: %s",
			strings.Join(probe.MissingModules, ","),
		)
	}
	return fmt.Errorf("validate market-data Python source runtime: unavailable")
}

func validateAbsoluteRegularFile(value string, name string) (string, error) {
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("%s must be an absolute path", name)
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", name, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s must name a regular file", name)
	}
	return absolute, nil
}

type osSidecarProcess struct {
	cmd         *exec.Cmd
	done        chan struct{}
	stopping    atomic.Bool
	mu          sync.Mutex
	waitErr     error
	stopTimeout time.Duration
	killTimeout time.Duration
}

func startMarketDataSidecar(config SidecarConfig) (sidecarProcess, error) {
	config.Executable = strings.TrimSpace(config.Executable)
	config.Host = strings.TrimSpace(config.Host)
	if config.Executable == "" {
		return nil, fmt.Errorf("market-data sidecar executable is required")
	}
	if config.Host != sidecarHost {
		return nil, fmt.Errorf("market-data sidecar host must be %s", sidecarHost)
	}
	if config.Port < 1 || config.Port > 65535 {
		return nil, fmt.Errorf("market-data sidecar port must be between 1 and 65535")
	}
	args := append([]string(nil), config.Arguments...)
	args = append(args,
		"--host", config.Host,
		"--port", strconv.Itoa(config.Port),
	)
	cmd := exec.Command(config.Executable, args...)
	cmd.Env = append(os.Environ(), config.Environment...)
	cmd.Env = append(cmd.Env, "PYTHONUNBUFFERED=1")
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	process := &osSidecarProcess{
		cmd:         cmd,
		done:        make(chan struct{}),
		stopTimeout: sidecarStopTimeout,
		killTimeout: sidecarKillTimeout,
	}
	go process.wait()
	return process, nil
}

func (p *osSidecarProcess) wait() {
	err := p.cmd.Wait()
	p.mu.Lock()
	p.waitErr = err
	p.mu.Unlock()
	close(p.done)
	if !p.stopping.Load() {
		if err != nil {
			log.Printf("JFTrade market-data sidecar exited: %v", err)
		} else {
			log.Printf("JFTrade market-data sidecar exited unexpectedly")
		}
	}
}

func (p *osSidecarProcess) Running() bool {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func (p *osSidecarProcess) Close() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	p.stopping.Store(true)
	killed, stopErr := p.requestStop()
	waitTimeout := durationOrDefault(p.stopTimeout, sidecarStopTimeout)
	if killed {
		waitTimeout = durationOrDefault(p.killTimeout, sidecarKillTimeout)
	}
	if !waitForSidecarDone(p.done, waitTimeout) {
		killErr := p.cmd.Process.Kill()
		killTimeout := durationOrDefault(p.killTimeout, sidecarKillTimeout)
		if !waitForSidecarDone(p.done, killTimeout) {
			return errors.Join(
				stopErr,
				wrapSidecarKillError(killErr, p.Running()),
				fmt.Errorf("wait for killed market-data sidecar timed out after %s", killTimeout),
			)
		}
	}
	if stopErr != nil {
		return stopErr
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var exitErr *exec.ExitError
	if p.waitErr == nil || errors.As(p.waitErr, &exitErr) {
		return nil
	}
	return p.waitErr
}

func (p *osSidecarProcess) requestStop() (bool, error) {
	if !p.Running() {
		return false, nil
	}
	killed, err := terminateSidecarProcess(p.cmd.Process)
	if processStopSucceeded(err) || !p.Running() {
		return killed, nil
	}
	killErr := p.cmd.Process.Kill()
	if processStopSucceeded(killErr) || !p.Running() {
		return true, nil
	}
	return true, errors.Join(
		fmt.Errorf("terminate market-data sidecar: %w", err),
		fmt.Errorf("kill market-data sidecar: %w", killErr),
	)
}

func waitForSidecarDone(done <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func durationOrDefault(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func wrapSidecarKillError(err error, running bool) error {
	if processStopSucceeded(err) || !running {
		return nil
	}
	return fmt.Errorf("kill market-data sidecar: %w", err)
}

func processStopSucceeded(err error) bool {
	return err == nil || errors.Is(err, os.ErrProcessDone)
}
