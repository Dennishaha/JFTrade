package servercore

import (
	"fmt"

	appcomposition "github.com/jftrade/jftrade-main/internal/app/apiserver/application"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/liveapp"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
)

type installationDependencies struct {
	server    *Server
	store     SidecarSettingsStore
	bootstrap serverBootstrap
	state     serverPersistentState
}

type platformBundle struct{ *installationDependencies }
type marketDataBundle struct{ *installationDependencies }
type tradingBundle struct{ *installationDependencies }
type strategyBacktestBundle struct{ *installationDependencies }
type assistantHTTPBundle struct{ *installationDependencies }

func installApplication(server *Server, store SidecarSettingsStore, bootstrap serverBootstrap, state serverPersistentState) error {
	dependencies := &installationDependencies{server: server, store: store, bootstrap: bootstrap, state: state}
	return (appcomposition.Installers[platformBundle, marketDataBundle, tradingBundle, strategyBacktestBundle, assistantHTTPBundle]{
		Platform:         func() (platformBundle, error) { return installPlatform(dependencies) },
		MarketData:       installMarketData,
		Trading:          installTrading,
		StrategyBacktest: installStrategyBacktest,
		AssistantHTTP:    installAssistantHTTP,
		Validate:         server.runtimes.SetupError,
		Rollback:         server.lifecycle.Resources().Rollback,
	}).Run()
}

func installPlatform(dependencies *installationDependencies) (platformBundle, error) {
	if dependencies.server == nil || dependencies.store == nil {
		return platformBundle{}, fmt.Errorf("platform dependencies are unavailable")
	}
	initializeSecurityAndCalendars(dependencies.server, dependencies.store, dependencies.bootstrap.settingsPath)
	return platformBundle{dependencies}, nil
}

func installMarketData(platform platformBundle) (marketDataBundle, error) {
	server := platform.server
	initializeMarketdataRuntime(server)
	if err := initializeMarketdataService(server); err != nil {
		return marketDataBundle{}, err
	}
	server.initializeWatchlistService()
	server.initializeResearchService()
	startLiveNotifications(server)
	return marketDataBundle(platform), nil
}

func installTrading(marketData marketDataBundle) (tradingBundle, error) {
	server := marketData.server
	initializeRealTradeControl(server, marketData.bootstrap)
	service := newTradingService(server)
	if err := ownResource(&server.serverApplication, "trading order updates", service.StopOrderUpdates); err != nil {
		return tradingBundle{}, err
	}
	server.tradingSvc = service
	return tradingBundle(marketData), nil
}

func installStrategyBacktest(trading tradingBundle) (strategyBacktestBundle, error) {
	server := trading.server
	if err := initializeBacktestService(server, trading.state); err != nil {
		return strategyBacktestBundle{}, err
	}
	liveWebSocket := liveapp.NewHandler(newLiveWebSocketBackend(server), liveapp.Options{
		DataInterval:            liveTickDispatchInterval,
		SecurityDetailsInterval: marketdataapp.MarketSecurityDetailsStreamInterval,
		DepthRefreshInterval:    marketdataapp.MarketDepthStreamRefreshInterval,
	})
	strategyRuntime := liveruntime.NewManager(newStrategyRuntimeDependencies(server))
	server.runtimes.SetStrategyRuntime(strategyRuntime, strategyRuntime)
	reconcileStrategyRuntimeStates(server)
	initializeStrategyService(server, trading.state)
	server.runtimes.SetLiveWebSocket(liveWebSocket)
	return strategyBacktestBundle(trading), nil
}

func installAssistantHTTP(strategyBacktest strategyBacktestBundle) (assistantHTTPBundle, error) {
	server := strategyBacktest.server
	initializeSystemService(server, strategyBacktest.bootstrap)
	initializeADKRuntime(server, strategyBacktest.bootstrap)
	initializeRuntimeServices(server, strategyBacktest.store)
	startAssistantWorkflowScheduler(server)
	if err := ownResource(&server.serverApplication, "runtime consumers", server.runtimes.CloseConsumers); err != nil {
		return assistantHTTPBundle{}, err
	}
	return assistantHTTPBundle(strategyBacktest), nil
}
