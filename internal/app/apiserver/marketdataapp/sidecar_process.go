package marketdataapp

import (
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

	"github.com/jftrade/jftrade-main/internal/yfinanceassets"
)

const (
	sidecarStopTimeout = 5 * time.Second
	sidecarKillTimeout = 2 * time.Second
	sidecarHost        = "127.0.0.1"
)

// SidecarConfig contains application-owned process arguments. It is never
// loaded from user settings.
type SidecarConfig struct {
	Executable string
	Host       string
	Port       int
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
	path    string
	cleanup func() error
}

type sidecarExecutableResolver func() (sidecarExecutable, error)
type sidecarPortAllocator func() (int, error)

var ErrYFinanceSidecarUnavailable = errors.New("embedded yfinance sidecar is unavailable")

type sidecarManager struct {
	mu           sync.Mutex
	endpoint     string
	process      sidecarProcess
	cleanup      func() error
	resolve      sidecarExecutableResolver
	allocatePort sidecarPortAllocator
	start        sidecarStarter
}

func newSidecarManager() *sidecarManager {
	return &sidecarManager{
		resolve:      resolveYFinanceSidecarExecutable,
		allocatePort: allocateYFinanceSidecarPort,
		start:        startYFinanceSidecar,
	}
}

func (m *sidecarManager) EnsureStarted() (string, error) {
	if m == nil {
		return "", fmt.Errorf("yfinance sidecar manager is unavailable")
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
		resolve = resolveYFinanceSidecarExecutable
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
		allocatePort = allocateYFinanceSidecarPort
	}
	port, err := allocatePort()
	if err != nil {
		return "", errors.Join(err, cleanup())
	}
	config := SidecarConfig{Executable: executable.path, Host: sidecarHost, Port: port}
	start := m.start
	if start == nil {
		start = startYFinanceSidecar
	}
	process, err := start(config)
	if err != nil {
		return "", errors.Join(fmt.Errorf("start yfinance sidecar: %w", err), cleanup())
	}
	m.endpoint = "http://" + net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	m.process = process
	m.cleanup = cleanup
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
			return fmt.Errorf("stop yfinance sidecar: %w", stopErr)
		}
	}
	cleanup := m.cleanup
	m.endpoint = ""
	m.process = nil
	if cleanup != nil {
		if err := cleanup(); err != nil {
			m.cleanup = cleanup
			return fmt.Errorf("clean up yfinance sidecar: %w", err)
		}
	}
	m.cleanup = nil
	return nil
}

func allocateYFinanceSidecarPort() (int, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(sidecarHost, "0"))
	if err != nil {
		return 0, fmt.Errorf("allocate yfinance sidecar port: %w", err)
	}
	defer func() { _ = listener.Close() }()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok || address.Port < 1 {
		return 0, fmt.Errorf("allocate yfinance sidecar port: invalid listener address %q", listener.Addr())
	}
	return address.Port, nil
}

func resolveYFinanceSidecarExecutable() (sidecarExecutable, error) {
	path := strings.TrimSpace(os.Getenv("JFTRADE_YFINANCE_SIDECAR"))
	if path != "" {
		if !filepath.IsAbs(path) {
			return sidecarExecutable{}, fmt.Errorf("JFTRADE_YFINANCE_SIDECAR must be an absolute path")
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return sidecarExecutable{}, fmt.Errorf("resolve JFTRADE_YFINANCE_SIDECAR: %w", err)
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return sidecarExecutable{}, fmt.Errorf("inspect JFTRADE_YFINANCE_SIDECAR: %w", err)
		}
		if !info.Mode().IsRegular() {
			return sidecarExecutable{}, fmt.Errorf("JFTRADE_YFINANCE_SIDECAR must name a regular file")
		}
		return sidecarExecutable{path: absolute}, nil
	}
	materialized, available, err := yfinanceassets.Materialize()
	if err != nil {
		return sidecarExecutable{}, fmt.Errorf("materialize embedded yfinance sidecar: %w", err)
	}
	if !available || materialized == nil || materialized.Path == "" {
		return sidecarExecutable{}, ErrYFinanceSidecarUnavailable
	}
	return sidecarExecutable{path: materialized.Path, cleanup: materialized.Cleanup}, nil
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

func startYFinanceSidecar(config SidecarConfig) (sidecarProcess, error) {
	config.Executable = strings.TrimSpace(config.Executable)
	config.Host = strings.TrimSpace(config.Host)
	if config.Executable == "" {
		return nil, fmt.Errorf("yfinance sidecar executable is required")
	}
	if config.Host != sidecarHost {
		return nil, fmt.Errorf("yfinance sidecar host must be %s", sidecarHost)
	}
	if config.Port < 1 || config.Port > 65535 {
		return nil, fmt.Errorf("yfinance sidecar port must be between 1 and 65535")
	}
	args := []string{
		"--host", config.Host,
		"--port", strconv.Itoa(config.Port),
	}
	cmd := exec.Command(config.Executable, args...)
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
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
			log.Printf("JFTrade yfinance sidecar exited: %v", err)
		} else {
			log.Printf("JFTrade yfinance sidecar exited unexpectedly")
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
				fmt.Errorf("wait for killed yfinance sidecar timed out after %s", killTimeout),
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
		fmt.Errorf("terminate yfinance sidecar: %w", err),
		fmt.Errorf("kill yfinance sidecar: %w", killErr),
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
	return fmt.Errorf("kill yfinance sidecar: %w", err)
}

func processStopSucceeded(err error) bool {
	return err == nil || errors.Is(err, os.ErrProcessDone)
}
