package backtest

func (r *RunResult) Snapshot() *RunResult {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return &RunResult{
		Symbol:        r.Symbol,
		Interval:      r.Interval,
		StartTime:     r.StartTime,
		EndTime:       r.EndTime,
		QuoteCurrency: r.QuoteCurrency,
		FinalBalance:  r.FinalBalance,
		PnL:           r.PnL,
		TotalTrades:   r.TotalTrades,
		WinRate:       r.WinRate,
		Trades:        append([]TradeEvent(nil), r.Trades...),
		OrderBook:     append([]OrderBookEntry(nil), r.OrderBook...),
		PnLCurve:      append([]PnLPoint(nil), r.PnLCurve...),
		Candles:       append([]Candle(nil), r.Candles...),
		Logs:          append([]string(nil), r.Logs...),
		Error:         r.Error,
		RuntimeErrors: append([]string(nil), r.RuntimeErrors...),
	}
}

func (r *RunResult) addRuntimeError(msg string) {
	r.mu.Lock()
	r.RuntimeErrors = append(r.RuntimeErrors, msg)
	r.mu.Unlock()
}
