package servercore

import (
	"testing"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	"github.com/shopspring/decimal"
)

func TestStrategyRuntimeNilBoundariesIgnoreMarketTicks(t *testing.T) {
	server := &Server{}
	runtime := liveruntime.NewManager(liveruntime.Dependencies{})
	server.runtimes.SetStrategyRuntime(runtime, runtime)
	server.handlePushMarketdataTick(mdsrv.Tick{
		Kind: mdsrv.TickKindTrade, InstrumentID: "US.AAPL", VolumeDelta: decimal.NewFromInt(-1),
	})
}
