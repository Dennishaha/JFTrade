package liveapp

import (
	"time"

	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/shopspring/decimal"
)

var maxLegacyFixedpointVolume = decimal.RequireFromString("92233720368.54775807")

// DispatchMarketdataTick fans a push trade into the market-data workflow and
// strategy runtime. The callbacks keep liveapp independent from the HTTP shell.
func DispatchMarketdataTick(
	marketData *mdsrv.Service,
	strategyRuntime *liveruntime.Manager,
	emitWorkflowEvent func(assistantassembly.WorkflowEvent),
	tick mdsrv.Tick,
) {
	if tick.Kind != mdsrv.TickKindTrade {
		return
	}
	if marketData != nil {
		payload := marketData.LiveTick(&tick, "")
		if emitWorkflowEvent != nil {
			emitWorkflowEvent(assistantassembly.WorkflowEvent{
				ID:       "market-data.tick|" + tick.InstrumentID + "|" + tick.ObservedAt,
				Type:     "market-data.tick",
				Source:   "market-data",
				EntityID: tick.InstrumentID,
				At:       tick.ObservedAt,
				Payload:  payload,
			})
		}
	}
	if strategyRuntime == nil {
		return
	}
	trade, ok := MarketTradeFromTick(tick)
	if ok {
		strategyRuntime.HandleMarketTrade(trade)
	}
}

// MarketTradeFromTick converts a market-data trade into the legacy strategy
// runtime representation without accepting invalid or unrepresentable values.
func MarketTradeFromTick(tick mdsrv.Tick) (bbgotypes.Trade, bool) {
	if tick.Kind != mdsrv.TickKindTrade || tick.VolumeDelta.IsNegative() {
		return bbgotypes.Trade{}, false
	}
	price, err := fixedpoint.NewFromString(tick.Price.String())
	if err != nil {
		return bbgotypes.Trade{}, false
	}
	quantity := fixedpoint.Zero
	if tick.VolumeDelta.Abs().LessThanOrEqual(maxLegacyFixedpointVolume) {
		if converted, err := fixedpoint.NewFromString(tick.VolumeDelta.String()); err == nil && !converted.IsInf() {
			quantity = converted
		}
	}
	tradeAt := time.Now().UTC()
	if parsed := ParseHTTPTime(tick.QuoteAt); !parsed.IsZero() {
		tradeAt = parsed
	}
	volumeDelta := tick.VolumeDelta
	cumulativeVolume := tick.Volume
	return bbgotypes.Trade{
		Exchange:         "futu",
		Symbol:           tick.InstrumentID,
		Price:            price,
		Quantity:         quantity,
		VolumeDelta:      &volumeDelta,
		CumulativeVolume: &cumulativeVolume,
		Time:             bbgotypes.Time(tradeAt),
	}, true
}

// ParseHTTPTime accepts the timestamp forms emitted by the HTTP market-data
// contract and normalizes them to UTC.
func ParseHTTPTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
