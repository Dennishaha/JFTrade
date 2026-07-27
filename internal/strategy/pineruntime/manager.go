package pineruntime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jftrade/jftrade-main/pkg/jftsettings"
)

// Publisher installs a newly resolved runner pair into its consumers before
// the previous pair is retired.
type Publisher func(backtest Runner, instance Runner)

// Manager owns the current backtest and live-instance runner pair.
type Manager struct {
	deps dependencies

	mu       sync.RWMutex
	backtest Runner
	instance Runner
	closed   bool
}

func NewManager(options ...Option) *Manager {
	return &Manager{deps: applyOptions(options)}
}

// Reconfigure builds and publishes a complete runner pair atomically. Invalid
// or disabled configuration publishes a nil pair, matching startup behavior.
func (manager *Manager) Reconfigure(settings jftsettings.PineWorkerSettings, publish Publisher) (Config, bool, error) {
	if manager == nil {
		return Config{}, false, fmt.Errorf("pine worker runtime manager is nil")
	}
	config, enabled, resolveErr := resolveConfig(settings, manager.deps)
	var backtest Runner
	var instance Runner
	buildErr := resolveErr
	if buildErr == nil && enabled {
		backtest, instance, buildErr = manager.buildPair(config)
	}
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		_ = closeRunner(backtest)
		_ = closeRunner(instance)
		return Config{}, false, fmt.Errorf("pine worker runtime manager is closed")
	}
	previousBacktest := manager.backtest
	previousInstance := manager.instance
	manager.backtest = backtest
	manager.instance = instance
	if publish != nil {
		publish(backtest, instance)
	}
	manager.mu.Unlock()
	retirePair(previousBacktest, previousInstance)
	if buildErr != nil {
		return Config{}, false, buildErr
	}
	return config, enabled, nil
}

func (manager *Manager) buildPair(config Config) (Runner, Runner, error) {
	backtest, err := newEphemeralRunner(config, runnerBacktest, manager.deps)
	if err != nil {
		return nil, nil, fmt.Errorf("create backtest runner: %w", err)
	}
	instance, err := newEphemeralRunner(config, runnerInstance, manager.deps)
	if err != nil {
		_ = closeRunner(backtest)
		return nil, nil, fmt.Errorf("create instance runner: %w", err)
	}
	return backtest, instance, nil
}

func (manager *Manager) Runners() (Runner, Runner) {
	if manager == nil {
		return nil, nil
	}
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.backtest, manager.instance
}

func (manager *Manager) Close() error {
	if manager == nil {
		return nil
	}
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return nil
	}
	manager.closed = true
	backtest := manager.backtest
	instance := manager.instance
	manager.backtest = nil
	manager.instance = nil
	manager.mu.Unlock()
	return CloseRunners(backtest, instance)
}

// CloseRunners closes a runner pair and preserves the identity of each error.
func CloseRunners(backtest Runner, instance Runner) error {
	var closeErr error
	if err := closeRunner(backtest); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("backtestPineWorkerRunner close: %w", err))
	}
	if err := closeRunner(instance); err != nil {
		closeErr = errors.Join(closeErr, fmt.Errorf("instancePineWorkerRunner close: %w", err))
	}
	return closeErr
}

func retirePair(backtest Runner, instance Runner) {
	_ = closeRunner(backtest)
	_ = closeRunner(instance)
}

func closeRunner(runner Runner) error {
	if runner == nil {
		return nil
	}
	closer, ok := runner.(interface{ Close(context.Context) error })
	if !ok {
		return nil
	}
	return closer.Close(context.Background())
}
