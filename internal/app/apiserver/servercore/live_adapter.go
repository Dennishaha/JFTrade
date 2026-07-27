package servercore

import (
	"context"
	"math"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
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
	if s == nil || tick.Kind != mdsrv.TickKindTrade {
		return
	}
	if s.marketdataSvc != nil {
		payload := s.marketdataSvc.LiveTick(&tick, "")
		s.emitWorkflowEvent(assistantassembly.WorkflowEvent{
			ID:       "market-data.tick|" + tick.InstrumentID + "|" + tick.ObservedAt,
			Type:     "market-data.tick",
			Source:   "market-data",
			EntityID: tick.InstrumentID,
			At:       tick.ObservedAt,
			Payload:  payload,
		})
	}
	strategyRuntime := s.runtimes.StrategyRuntime()
	if strategyRuntime == nil {
		return
	}
	trade, ok := marketTradeFromTick(tick)
	if !ok {
		return
	}
	strategyRuntime.HandleMarketTrade(trade)
}

func marketTradeFromTick(tick mdsrv.Tick) (bbgotypes.Trade, bool) {
	if tick.Kind != mdsrv.TickKindTrade || tick.VolumeDelta < 0 || math.IsNaN(tick.VolumeDelta) || math.IsInf(tick.VolumeDelta, 0) {
		return bbgotypes.Trade{}, false
	}
	price, err := fixedpoint.NewFromString(tick.Price.String())
	if err != nil {
		return bbgotypes.Trade{}, false
	}
	quantity := fixedpoint.NewFromFloat(tick.VolumeDelta)
	tradeAt := time.Now().UTC()
	if parsed := httpTime(tick.QuoteAt); !parsed.IsZero() {
		tradeAt = parsed
	}
	return bbgotypes.Trade{
		Exchange: "futu",
		Symbol:   tick.InstrumentID,
		Price:    price,
		Quantity: quantity,
		Time:     bbgotypes.Time(tradeAt),
	}, true
}

func httpTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
