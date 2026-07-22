package servercore

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (r *strategySymbolRuntime) syncClosedKLinesLoop() {
	if r == nil || strategyRuntimeClosedKLineSyncInterval <= 0 {
		return
	}
	ticker := time.NewTicker(strategyRuntimeClosedKLineSyncInterval)
	defer ticker.Stop()
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.syncClosedKLines()
		}
	}
}

func (r *strategySymbolRuntime) syncClosedKLines() {
	if r == nil || r.runtimeExchange == nil {
		return
	}
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	limit := strategyRuntimeClosedKLineSyncLimit
	if limit <= 0 {
		limit = 8
	}
	klines, err := r.runtimeExchange.QueryKLines(ctx, r.symbol, r.interval, bbgotypes.KLineQueryOptions{Limit: limit})
	if err != nil {
		r.handleRuntimeError(fmt.Errorf("refresh strategy klines for %s: %w", r.symbol, err))
		return
	}
	sort.SliceStable(klines, func(left int, right int) bool {
		return klines[left].StartTime.Time().Before(klines[right].StartTime.Time())
	})
	for index := range klines {
		kline := klines[index]
		if !kline.Closed {
			if index == len(klines)-1 {
				r.setCurrentBucket(new(kline))
			}
			continue
		}
		kline.Closed = true
		r.emitClosedKLine(kline)
	}
}

func (r *strategySymbolRuntime) handleTrade(trade bbgotypes.Trade) {
	tradeTime := trade.Time.Time().UTC()
	if tradeTime.IsZero() {
		tradeTime = time.Now().UTC()
	}
	windowStart, windowEnd := strategyRuntimeBucketWindow(tradeTime, r.interval)
	if trade.Price.Sign() <= 0 {
		return
	}

	tradeKLine := strategyRuntimeTradeKLine(r.exchange, r.symbol, r.interval, trade, windowStart, windowEnd)
	var closed *bbgotypes.KLine

	r.mu.Lock()
	current := r.currentBucket
	switch {
	case current == nil:
		r.currentBucket = &tradeKLine
	case current.StartTime.Time().Equal(windowStart):
		current.Merge(&tradeKLine)
	case windowStart.After(current.StartTime.Time()):
		closedCopy := *current
		closedCopy.Closed = true
		closed = &closedCopy
		r.lastClosedPrice = closedCopy.Close.Float64()
		r.currentBucket = &tradeKLine
	default:
		// Closed historical buckets are reconciled by syncClosedKLines. Folding a
		// late trade into the current bucket corrupts its OHLC and signal input.
	}
	r.mu.Unlock()

	if closed != nil {
		r.emitClosedKLine(*closed)
	}
}

func (r *strategySymbolRuntime) recordClosedKLineState(closed bbgotypes.KLine) bool {
	closedAt := closed.EndTime.Time().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	if !closedAt.After(r.lastClosedKLine) {
		if closed.Close.Sign() > 0 {
			r.lastClosedPrice = closed.Close.Float64()
		}
		return false
	}
	r.lastClosedKLine = closedAt
	if closed.Close.Sign() > 0 {
		r.lastClosedPrice = closed.Close.Float64()
	}
	return true
}

func (r *strategySymbolRuntime) emitClosedKLine(closed bbgotypes.KLine) {
	if !r.recordClosedKLineState(closed) {
		return
	}
	if err := r.refreshBrokerAccount(); err != nil {
		r.handleRuntimeError(err)
	}
	if r.onClosedKLine != nil {
		r.onClosedKLine(closed.EndTime.Time().UTC())
	}
	if r.pineWorkerLive != nil {
		if err := r.pineWorkerLive.onClosedKLine(r.context(), closed); err != nil {
			r.handleRuntimeError(err)
		}
	}
	r.emitter.EmitKLineClosed(closed)
}

func (r *strategySymbolRuntime) context() context.Context {
	if r != nil && r.ctx != nil {
		return r.ctx
	}
	return context.Background()
}

func (r *strategySymbolRuntime) handleRuntimeError(err error) {
	if err == nil {
		return
	}
	if r.onError != nil {
		r.onError(err.Error())
		return
	}
	log.Printf("JFTrade strategy runtime degraded: %v", err)
}

func (r *strategySymbolRuntime) refreshBrokerAccount() error {
	if r == nil || r.runtimeExchange == nil || r.session == nil {
		return nil
	}
	r.accountRefreshMu.Lock()
	defer r.accountRefreshMu.Unlock()

	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	r.accountMu.RLock()
	funds := cloneStrategyRuntimeFundsSnapshot(r.cachedFunds)
	positions := cloneStrategyRuntimePositions(r.cachedPositions)
	r.accountMu.RUnlock()
	freshFunds, err := r.runtimeExchange.QueryBrokerFunds(ctx, r.brokerQuery)
	if err != nil {
		if connectivityFromBrokerReadError(err) != "disconnected" {
			return fmt.Errorf("refresh strategy broker funds for %s: %w", r.symbol, err)
		}
		log.Printf("JFTrade strategy runtime broker funds refresh disconnected for %s: %v", r.symbol, err)
	} else {
		funds = cloneStrategyRuntimeFundsSnapshot(freshFunds)
	}
	freshPositions, err := r.runtimeExchange.QueryBrokerPositions(ctx, r.brokerQuery)
	if err != nil {
		if connectivityFromBrokerReadError(err) != "disconnected" {
			return fmt.Errorf("refresh strategy broker positions for %s: %w", r.symbol, err)
		}
		log.Printf("JFTrade strategy runtime broker positions refresh disconnected for %s: %v", r.symbol, err)
	} else {
		positions = cloneStrategyRuntimePositions(freshPositions)
	}
	account := buildStrategyRuntimeAccount(funds, positions, r.market, r.symbol)
	r.accountMu.Lock()
	r.cachedFunds = funds
	r.cachedPositions = positions
	r.session.Account = account
	r.accountMu.Unlock()
	return nil
}

func (r *strategySymbolRuntime) brokerPositionsSnapshot() []broker.PositionSnapshot {
	if r == nil {
		return nil
	}
	r.accountMu.RLock()
	defer r.accountMu.RUnlock()
	return cloneStrategyRuntimePositions(r.cachedPositions)
}

func (r *strategySymbolRuntime) brokerAccountSnapshot() *bbgotypes.Account {
	if r == nil || r.session == nil {
		return nil
	}
	r.accountMu.RLock()
	defer r.accountMu.RUnlock()
	return r.session.Account
}

func (r *strategySymbolRuntime) currentPrice() float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.currentBucket != nil {
		return r.currentBucket.Close.Float64()
	}
	return r.lastClosedPrice
}

func (r *strategySymbolRuntime) setCurrentBucket(bucket *bbgotypes.KLine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentBucket = bucket
	if bucket != nil {
		r.lastClosedPrice = bucket.Close.Float64()
	}
}

func (r *strategySymbolRuntime) setLastClosedPrice(price float64) {
	r.mu.Lock()
	r.lastClosedPrice = price
	r.mu.Unlock()
}
