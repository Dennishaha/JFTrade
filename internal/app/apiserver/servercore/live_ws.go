package servercore

import (
	"github.com/jftrade/jftrade-main/internal/app/apiserver/liveapp"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	jftsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productsrv "github.com/jftrade/jftrade-main/internal/productfeatures"
)

func newLiveWebSocketBackend(s *Server) *liveapp.Backend {
	options := liveapp.BackendOptions{
		DefaultConnectionLimit:   defaultMaxWebSocketClients,
		SampleFreshnessThreshold: liveHeartbeatStaleThreshold,
	}
	if s == nil {
		return liveapp.NewBackend(options)
	}
	options.ConnectionLimit = func() int {
		if s.store == nil {
			return defaultMaxWebSocketClients
		}
		return s.store.InterfaceSettings(jftsettings.LaunchDefaults{}).LiveWebSocketConnectionLimit
	}
	options.Heartbeat = s.liveHeartbeatEvent
	options.MarketData = func() *mdsrv.Service { return s.marketdataSvc }
	options.ProductFeatures = func() *productsrv.Service { return s.productFeaturesSvc }
	options.NotificationsAfter = s.liveNotificationsAfter
	options.EnsureNotificationBridge = s.ensureLiveNotificationBridge
	options.SubscribeNativeDepth = func(onUpdate func(string)) func() {
		if !s.futuCoordinator().Enabled() {
			return nil
		}
		if runtime := marketdataapp.RuntimeFromService(s.marketdataSvc); runtime != nil && runtime.ActiveProviderID() != "futu" {
			return nil
		}
		marketDataRuntime := s.runtimes.MarketData()
		if marketDataRuntime == nil {
			return nil
		}
		return marketDataRuntime.OnOrderBookUpdate(onUpdate)
	}
	return liveapp.NewBackend(options)
}

func liveStreamStats(s *serverApplication) (count int, limit int, atLimit bool) {
	if s == nil {
		return 0, defaultMaxWebSocketClients, false
	}
	liveWebSocket := s.runtimes.LiveWebSocket()
	if liveWebSocket == nil {
		return 0, defaultMaxWebSocketClients, false
	}
	stats := liveWebSocket.Stats()
	return stats.Connected, stats.Limit, stats.AtLimit
}
