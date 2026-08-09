package servercore

import (
	"context"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/liveapp"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

const (
	liveTickDispatchInterval     = 250 * time.Millisecond
	liveTickFallbackPollInterval = mdsrv.FallbackPollInterval
	liveTickSampleFreshness      = mdsrv.TickFreshness
	tickCacheRetention           = mdsrv.CacheRetention
	liveHeartbeatStaleThreshold  = liveTickFallbackPollInterval + liveTickSampleFreshness
	liveStreamConnectTimeout     = mdsrv.StreamConnectTimeout
	defaultSSEClientRetry        = 5 * time.Second
)

func (s *serverApplication) ensureLiveMarketStream(context.Context, []string) {
	if s != nil && s.marketdataSvc != nil {
		s.marketdataSvc.WakeCollector()
	}
}

func (s *serverApplication) handlePushMarketdataTick(tick mdsrv.Tick) {
	if s == nil {
		return
	}
	liveapp.DispatchMarketdataTick(s.marketdataSvc, s.runtimes.StrategyRuntime(), s.emitWorkflowEvent, tick)
}

func marketTradeFromTick(tick mdsrv.Tick) (bbgotypes.Trade, bool) {
	return liveapp.MarketTradeFromTick(tick)
}

func httpTime(value string) time.Time {
	return liveapp.ParseHTTPTime(value)
}
