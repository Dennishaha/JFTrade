package servercore

import (
	"context"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"

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

type marketTickSample = mdsrv.Tick

func (s *Server) ensureLiveMarketStream(context.Context, []string) {
	if s != nil && s.marketdataSvc != nil {
		s.marketdataSvc.WakeCollector()
	}
}

func (s *Server) handlePushMarketdataTick(tick mdsrv.Tick) {
	if s == nil || s.strategyRuntimeManager == nil || tick.Kind != mdsrv.TickKindTrade {
		return
	}
	price, err := fixedpoint.NewFromString(tick.Price.String())
	if err != nil {
		return
	}
	quantity := fixedpoint.NewFromFloat(tick.Volume)
	tradeAt := time.Now().UTC()
	if parsed := httpTime(tick.QuoteAt); !parsed.IsZero() {
		tradeAt = parsed
	}
	s.strategyRuntimeManager.handleMarketTrade(bbgotypes.Trade{
		Exchange: "futu",
		Symbol:   tick.InstrumentID,
		Price:    price,
		Quantity: quantity,
		Time:     bbgotypes.Time(tradeAt),
	})
}

func httpTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
