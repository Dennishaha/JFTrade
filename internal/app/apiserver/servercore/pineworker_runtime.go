package servercore

import (
	"log"

	"github.com/jftrade/jftrade-main/internal/pineworkerassets"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	"github.com/jftrade/jftrade-main/internal/strategy/pineruntime"
	"github.com/jftrade/jftrade-main/pkg/jftsettings"
)

const (
	envPineWorkerDisabled        = pineruntime.EnvDisabled
	envPineWorkerBundle          = pineruntime.EnvBundle
	envPineWorkerRuntime         = pineruntime.EnvRuntime
	envPineWorkerBacktestWorkers = pineruntime.EnvBacktestWorkers
	envPineWorkerInstanceWorkers = pineruntime.EnvInstanceWorkers
	envPineWorkerStartPort       = pineruntime.EnvStartPort
	envPineWorkerRequestTimeout  = pineruntime.EnvRequestTimeout
)

type pineWorkerRunner = pineruntime.Runner
type pineWorkerRuntimeConfig = pineruntime.Config

var (
	newPineWorkerLauncher pineruntime.LauncherFactory = pineruntime.NewNodeLauncher
	newPineWorkerDialer   pineruntime.DialerFactory   = pineruntime.NewGRPCDialer
	selectPineWorkerAsset pineruntime.AssetSelector   = pineworkerassets.Select
)

func (s *serverApplication) startPineWorkerManagers() (pineWorkerRunner, pineWorkerRunner) {
	manager, _, _ := s.runtimes.EnsurePineWorker(func() *pineruntime.Manager {
		return pineruntime.NewManager(pineWorkerRuntimeOptions()...)
	})
	config, enabled, err := manager.Reconfigure(s.pineWorkerSettings(), func(backtest, instance pineruntime.Runner) {
		publishPineWorkerRunners(s, backtest, instance)
	})
	if err != nil {
		log.Printf("JFTrade PineTS worker manager disabled by invalid config: %v", err)
		return nil, nil
	}
	if !enabled {
		log.Printf("JFTrade PineTS worker manager not started: %s is not configured and no embedded worker asset is available; run `pnpm run dev:api:pineworker` or set %s=/absolute/path/to/worker.mjs", envPineWorkerBundle, envPineWorkerBundle)
		return nil, nil
	}
	log.Printf("JFTrade PineTS worker managers configured: source=%s runtime=%s backtestLimit=%d instanceLimit=%d host=%s mode=ephemeral proto=%s cwd=%s", config.Source(), config.RuntimePath, config.BacktestWorkers, config.InstanceWorkers, config.Host, config.ProtoPath, config.WorkDir)
	return manager.Runners()
}

func pineWorkerRuntimeOptions() []pineruntime.Option {
	return []pineruntime.Option{
		pineruntime.WithAssetSelector(selectPineWorkerAsset),
		pineruntime.WithLauncherFactory(newPineWorkerLauncher),
		pineruntime.WithDialerFactory(newPineWorkerDialer),
		pineruntime.WithRuntimeResolver(resolvePineWorkerRuntime),
	}
}

func (s *serverApplication) pineWorkerSettings() jftsettings.PineWorkerSettings {
	if s == nil || s.store == nil {
		return settingsfile.DefaultPineWorkerSettings()
	}
	return persistenceOnlySettingsStore(s.store).PineWorkerSettings()
}

func (s *serverApplication) applyPineWorkerSettings(settings jftsettings.PineWorkerSettings) {
	if s == nil {
		return
	}
	manager, unmanagedBacktest, unmanagedInstance := s.runtimes.EnsurePineWorker(func() *pineruntime.Manager {
		return pineruntime.NewManager(pineWorkerRuntimeOptions()...)
	})
	if _, _, err := manager.Reconfigure(settingsfile.NormalizePineWorkerSettings(settings), func(backtest, instance pineruntime.Runner) {
		publishPineWorkerRunners(s, backtest, instance)
	}); err != nil {
		log.Printf("JFTrade PineTS worker manager disabled by invalid config: %v", err)
	}
	_ = pineruntime.CloseRunners(unmanagedBacktest, unmanagedInstance)
}

func publishPineWorkerRunners(s *serverApplication, backtestRunner pineruntime.Runner, instanceRunner pineruntime.Runner) {
	s.runtimes.SetPineWorkerRunners(backtestRunner, instanceRunner)
	if s.backtestSvc != nil {
		s.backtestSvc.SetPineWorkerRunner(backtestRunner)
	}
}

func resolvePineWorkerRuntime(settings jftsettings.PineWorkerSettings) string {
	return resolveNodeDependencyRuntime(settings).effectivePath
}
