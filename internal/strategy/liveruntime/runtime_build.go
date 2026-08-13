package liveruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func newStrategyRuntimeSession(
	marketData MarketDataSource,
	markets bbgotypes.MarketMap,
	funds *broker.FundsSnapshot,
	positions []broker.PositionSnapshot,
	market bbgotypes.Market,
	symbol string,
) (*bbgo.ExchangeSession, bbgotypes.StandardStreamEmitter, error) {
	marketStream := marketData.NewStream()
	if marketStream == nil {
		return nil, nil, fmt.Errorf("strategy market-data source returned a nil stream")
	}
	marketStream.SetPublicOnly()
	session := &bbgo.ExchangeSession{Name: "strategy-runtime", Account: bbgotypes.NewAccount(), MarketDataStream: marketStream}
	session.SetMarkets(markets)
	session.Account = buildStrategyRuntimeAccount(funds, positions, market, symbol)
	emitter, ok := session.MarketDataStream.(bbgotypes.StandardStreamEmitter)
	if !ok {
		return nil, nil, fmt.Errorf("strategy market stream does not support kline emission")
	}
	return session, emitter, nil
}

func (m *Manager) newSymbolRuntime(
	runtimeCtx context.Context,
	marketData MarketDataSource,
	account AccountSource,
	instance stratsrv.ManagedInstance,
	symbol string,
	interval bbgotypes.Interval,
	market bbgotypes.Market,
	session *bbgo.ExchangeSession,
	emitter bbgotypes.StandardStreamEmitter,
	funds *broker.FundsSnapshot,
	positions []broker.PositionSnapshot,
) *symbolRuntime {
	return &symbolRuntime{
		instanceID: instance.ID, name: strings.TrimSpace(instance.Definition.Name), symbol: symbol,
		interval: interval, exchange: marketData.Name(), ctx: runtimeCtx,
		marketDataSource: marketData, accountSource: account,
		brokerQuery: strategyRuntimeBrokerReadQuery(instance.Binding), market: market,
		cachedFunds: cloneStrategyRuntimeFundsSnapshot(funds), cachedPositions: cloneStrategyRuntimePositions(positions),
		session: session, emitter: emitter, closedKLineSyncInterval: m.currentClosedKLineSyncInterval(),
		onClosedKLine: func(at time.Time) { m.recordClosedKLine(instance.ID, at) },
		onError:       m.runtimeErrorRecorder(instance.ID, symbol),
	}
}

func (m *Manager) runtimeErrorRecorder(instanceID string, symbol string) func(string) {
	return func(message string) {
		message = strings.TrimSpace(message)
		if message == "" {
			return
		}
		m.recordError(instanceID, message, time.Now().UTC())
		jftradeErr := m.appendRuntimeEvent(
			instanceID,
			fmt.Sprintf("runtime error %s: %s", symbol, message),
			"runtime_error",
			fmt.Sprintf("%s: %s", symbol, message),
		)
		besteffort.LogError(jftradeErr)
	}
}

func (m *Manager) recordIgnoredOrder(instanceID string, symbol string, message string) {
	jftradeErr := m.appendRuntimeEvent(instanceID, fmt.Sprintf("live order ignored %s", symbol), "order_ignored", message)
	besteffort.LogError(jftradeErr)
}
