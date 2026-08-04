package marketdataapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdataassets"
)

func TestSidecarManagerStartsReusesAndStopsManagedExecutable(t *testing.T) {
	starter := &sidecarStarterStub{}
	cleanupCalls := 0
	startedCalls := 0
	manager := &sidecarManager{
		resolve: func() (sidecarExecutable, error) {
			return sidecarExecutable{
				path: "/tmp/yfinance-sidecar",
				started: func() {
					startedCalls++
				},
				cleanup: func() error {
					cleanupCalls++
					return nil
				},
			}, nil
		},
		allocatePort: func() (int, error) { return 43123, nil },
		start:        starter.Start,
	}

	endpoint, err := manager.EnsureStarted()
	if err != nil {
		t.Fatalf("EnsureStarted: %v", err)
	}
	if endpoint != "http://127.0.0.1:43123" || len(starter.configs) != 1 || startedCalls != 1 {
		t.Fatalf("started endpoint/configs = %q/%#v", endpoint, starter.configs)
	}
	config := starter.configs[0]
	if config.Executable != "/tmp/yfinance-sidecar" || config.Host != sidecarHost || config.Port != 43123 {
		t.Fatalf("sidecar config = %#v", config)
	}
	if reused, err := manager.EnsureStarted(); err != nil || reused != endpoint || len(starter.configs) != 1 {
		t.Fatalf("reused endpoint = %q, err=%v, starts=%d", reused, err, len(starter.configs))
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if starter.processes[0].closeCalls != 1 || cleanupCalls != 1 || manager.process != nil || manager.endpoint != "" {
		t.Fatalf("stopped state = process %#v endpoint %q cleanup %d", manager.process, manager.endpoint, cleanupCalls)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close(empty): %v", err)
	}
}

func TestSidecarManagerRestartsExitedProcessAndCleansPreviousMaterialization(t *testing.T) {
	starter := &sidecarStarterStub{}
	resolveCalls := 0
	cleanupCalls := 0
	manager := &sidecarManager{
		resolve: func() (sidecarExecutable, error) {
			resolveCalls++
			return sidecarExecutable{
				path: "/tmp/yfinance-sidecar",
				cleanup: func() error {
					cleanupCalls++
					return nil
				},
			}, nil
		},
		allocatePort: func() (int, error) { return 43120 + resolveCalls, nil },
		start:        starter.Start,
	}
	if _, err := manager.EnsureStarted(); err != nil {
		t.Fatalf("first EnsureStarted: %v", err)
	}
	starter.processes[0].running = false
	endpoint, err := manager.EnsureStarted()
	if err != nil {
		t.Fatalf("restart EnsureStarted: %v", err)
	}
	if endpoint != "http://127.0.0.1:43122" || resolveCalls != 2 || cleanupCalls != 1 || len(starter.processes) != 2 {
		t.Fatalf("restart = endpoint %q resolves %d cleanups %d starts %d", endpoint, resolveCalls, cleanupCalls, len(starter.processes))
	}
}

func TestSidecarManagerCleansMaterializationAfterPreparationFailures(t *testing.T) {
	allocateErr := errors.New("port allocation failed")
	cleanupCalls := 0
	startedCalls := 0
	manager := &sidecarManager{
		resolve: func() (sidecarExecutable, error) {
			return sidecarExecutable{
				path:    "/tmp/helper",
				started: func() { startedCalls++ },
				cleanup: func() error {
					cleanupCalls++
					return nil
				},
			}, nil
		},
		allocatePort: func() (int, error) { return 0, allocateErr },
	}
	if _, err := manager.EnsureStarted(); !errors.Is(err, allocateErr) || cleanupCalls != 1 {
		t.Fatalf("allocation failure = err %v cleanup %d", err, cleanupCalls)
	}

	startErr := errors.New("start failed")
	starter := &sidecarStarterStub{errors: []error{startErr}}
	manager.allocatePort = func() (int, error) { return 43123, nil }
	manager.start = starter.Start
	if _, err := manager.EnsureStarted(); !errors.Is(err, startErr) || cleanupCalls != 2 {
		t.Fatalf("start failure = err %v cleanup %d", err, cleanupCalls)
	}
	if manager.process != nil || manager.endpoint != "" {
		t.Fatalf("failed start retained state: %#v/%q", manager.process, manager.endpoint)
	}
	if startedCalls != 0 {
		t.Fatalf("post-start hook ran %d times after failed starts", startedCalls)
	}
}

func TestSidecarManagerRetainsProcessUntilStopSucceeds(t *testing.T) {
	stopErr := errors.New("stop failed")
	process := &sidecarProcessStub{running: true, closeErr: stopErr}
	manager := &sidecarManager{endpoint: "http://127.0.0.1:43123", process: process}
	if err := manager.Stop(); !errors.Is(err, stopErr) {
		t.Fatalf("Stop error = %v", err)
	}
	if manager.process != process || manager.endpoint == "" {
		t.Fatalf("failed Stop discarded retry state: %#v/%q", manager.process, manager.endpoint)
	}
	process.closeErr = nil
	if err := manager.Stop(); err != nil {
		t.Fatalf("retry Stop: %v", err)
	}
	if process.closeCalls != 2 || manager.process != nil || manager.endpoint != "" {
		t.Fatalf("retry state = calls %d process %#v endpoint %q", process.closeCalls, manager.process, manager.endpoint)
	}
}

func TestSidecarExecutableDevelopmentOverrideAndManagerBoundaries(t *testing.T) {
	if !marketdataassets.DevelopmentOverridesAllowed() {
		t.Skip("development overrides are disabled in release-assets builds")
	}
	helper := filepath.Join(t.TempDir(), "yfinance-sidecar")
	if err := os.WriteFile(helper, []byte("helper"), 0o700); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	t.Setenv("JFTRADE_YFINANCE_SIDECAR", helper)
	executable, err := resolveMarketDataSidecarExecutable("")
	if err != nil || executable.path != helper {
		t.Fatalf("resolve override = %#v, %v", executable, err)
	}

	t.Setenv("JFTRADE_YFINANCE_SIDECAR", "")
	restorePythonRuntimeProbeOutput(t, func(context.Context, string, string, ...string) ([]byte, error) {
		return []byte(`{"version":[3,11,9],"missing":[]}`), nil
	})
	sourceRuntime, err := resolveMarketDataSidecarExecutable("")
	if err != nil {
		t.Fatalf("resolve source runtime: %v", err)
	}
	if sourceRuntime.path == "" || !reflect.DeepEqual(sourceRuntime.arguments, []string{"-m", "marketdata_sidecar.main"}) ||
		len(sourceRuntime.environment) != 1 || !strings.HasPrefix(sourceRuntime.environment[0], "PYTHONPATH=") {
		t.Fatalf("source runtime = %#v", sourceRuntime)
	}
	var nilManager *sidecarManager
	if _, err := nilManager.EnsureStarted(); err == nil {
		t.Fatal("nil manager started")
	}
	if err := nilManager.Stop(); err != nil {
		t.Fatalf("nil manager Stop: %v", err)
	}
	manager := newSidecarManager("")
	if manager.resolve == nil || manager.allocatePort == nil || manager.start == nil {
		t.Fatalf("newSidecarManager = %#v", manager)
	}
	port, err := allocateMarketDataSidecarPort()
	if err != nil || port < 1 || port > 65535 {
		t.Fatalf("allocated port = %d, %v", port, err)
	}
}

func TestDevelopmentOverrideRejectsMissingAndNonFilePaths(t *testing.T) {
	if !marketdataassets.DevelopmentOverridesAllowed() {
		t.Skip("development overrides are disabled in release-assets builds")
	}
	t.Setenv("JFTRADE_YFINANCE_SIDECAR", "relative/yfinance-sidecar")
	if _, err := resolveMarketDataSidecarExecutable(""); err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("relative override error = %v", err)
	}
	t.Setenv("JFTRADE_YFINANCE_SIDECAR", filepath.Join(t.TempDir(), "missing"))
	if _, err := resolveMarketDataSidecarExecutable(""); err == nil || !strings.Contains(err.Error(), "inspect") {
		t.Fatalf("missing override error = %v", err)
	}
	t.Setenv("JFTRADE_YFINANCE_SIDECAR", t.TempDir())
	if _, err := resolveMarketDataSidecarExecutable(""); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory override error = %v", err)
	}
}

func TestDevelopmentPythonSourceCommandAndExplicitHelperPrecedence(t *testing.T) {
	if !marketdataassets.DevelopmentOverridesAllowed() {
		t.Skip("development overrides are disabled in release-assets builds")
	}
	root := t.TempDir()
	python := filepath.Join(root, "python")
	helper := filepath.Join(root, "helper")
	source := filepath.Join(root, "src")
	if err := os.WriteFile(python, []byte("python"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, []byte("helper"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JFTRADE_YFINANCE_DEV_PYTHON", python)
	t.Setenv("JFTRADE_YFINANCE_DEV_PYTHONPATH", source)
	t.Setenv("JFTRADE_YFINANCE_SIDECAR", helper)
	executable, err := resolveMarketDataSidecarExecutable("")
	if err != nil || executable.path != helper || len(executable.arguments) != 0 {
		t.Fatalf("explicit helper precedence = %#v, %v", executable, err)
	}

	t.Setenv("JFTRADE_YFINANCE_SIDECAR", "")
	restorePythonRuntimeProbeOutput(t, func(context.Context, string, string, ...string) ([]byte, error) {
		return []byte(`{"version":[3,12,1],"missing":[]}`), nil
	})
	executable, err = resolveMarketDataSidecarExecutable("")
	if err != nil || executable.path != python ||
		strings.Join(executable.arguments, " ") != "-m marketdata_sidecar.main" ||
		len(executable.environment) != 1 ||
		executable.environment[0] != "PYTHONPATH="+source {
		t.Fatalf("Python source command = %#v, %v", executable, err)
	}

	starter := &sidecarStarterStub{}
	manager := &sidecarManager{
		resolve:      func() (sidecarExecutable, error) { return executable, nil },
		allocatePort: func() (int, error) { return 43123, nil },
		start:        starter.Start,
	}
	if _, err := manager.EnsureStarted(); err != nil {
		t.Fatal(err)
	}
	config := starter.configs[0]
	if strings.Join(config.Arguments, " ") != "-m marketdata_sidecar.main" ||
		len(config.Environment) != 1 || config.Environment[0] != "PYTHONPATH="+source {
		t.Fatalf("manager command config = %#v", config)
	}
}

func TestDevelopmentPythonSourceCommandRejectsInvalidSourcePath(t *testing.T) {
	if !marketdataassets.DevelopmentOverridesAllowed() {
		t.Skip("development overrides are disabled in release-assets builds")
	}
	root := t.TempDir()
	python := filepath.Join(root, "python")
	source := filepath.Join(root, "src")
	if err := os.WriteFile(python, []byte("python"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JFTRADE_YFINANCE_SIDECAR", "")

	tests := []struct {
		name       string
		pythonPath string
		want       string
	}{
		{name: "relative source path", pythonPath: "relative/src", want: "absolute path"},
		{name: "missing source directory", pythonPath: filepath.Join(root, "missing"), want: "inspect"},
		{name: "source path is file", pythonPath: python, want: "must name a directory"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("JFTRADE_YFINANCE_DEV_PYTHON", python)
			t.Setenv("JFTRADE_YFINANCE_DEV_PYTHONPATH", test.pythonPath)
			_, err := resolveMarketDataSidecarExecutable("")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("resolveMarketDataSidecarExecutable error = %v", err)
			}
		})
	}
}

func TestDevelopmentPythonSourceCommandRejectsInvalidRuntimeBeforeStart(t *testing.T) {
	if !marketdataassets.DevelopmentOverridesAllowed() {
		t.Skip("development overrides are disabled in release-assets builds")
	}
	root := t.TempDir()
	python := filepath.Join(root, "python")
	if err := os.WriteFile(python, []byte("python"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvMarketDataSidecar, "")
	t.Setenv(EnvMarketDataDevPython, python)
	t.Setenv(EnvMarketDataDevPythonPath, root)
	restorePythonRuntimeProbeOutput(t, func(context.Context, string, string, ...string) ([]byte, error) {
		return []byte(`{"version":[3,10,14],"missing":[]}`), nil
	})

	_, err := resolveMarketDataSidecarExecutable("")
	if err == nil || !strings.Contains(err.Error(), "below 3.11") {
		t.Fatalf("invalid source runtime error = %v", err)
	}
}

type sidecarStarterStub struct {
	configs   []SidecarConfig
	processes []*sidecarProcessStub
	errors    []error
}

func (s *sidecarStarterStub) Start(config SidecarConfig) (sidecarProcess, error) {
	s.configs = append(s.configs, config)
	if len(s.errors) > 0 {
		err := s.errors[0]
		s.errors = s.errors[1:]
		if err != nil {
			return nil, err
		}
	}
	process := &sidecarProcessStub{running: true}
	s.processes = append(s.processes, process)
	return process, nil
}

type sidecarProcessStub struct {
	running    bool
	closeCalls int
	closeErr   error
}

func (p *sidecarProcessStub) Running() bool { return p != nil && p.running }

func (p *sidecarProcessStub) Close() error {
	p.closeCalls++
	if p.closeErr == nil {
		p.running = false
	}
	return p.closeErr
}
