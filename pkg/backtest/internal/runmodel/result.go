package runmodel

import "maps"

import "sync"

// TradeEvent is a single filled trade for chart rendering.
type TradeEvent struct {
	Time  string  `json:"time"`
	Side  string  `json:"side"`
	Price string  `json:"price"`
	Qty   string  `json:"qty"`
	PnL   float64 `json:"pnl,omitempty"`
}

// OrderBookEntry captures a single backtest order and its latest fill outcome.
// A submitted order that later fills is merged into the same row.
type OrderBookEntry struct {
	OrderID        string `json:"orderId"`
	ClientOrderID  string `json:"clientOrderId,omitempty"`
	Symbol         string `json:"symbol"`
	Side           string `json:"side"`
	Quantity       string `json:"quantity"`
	OrderType      string `json:"orderType,omitempty"`
	OrderPrice     string `json:"orderPrice,omitempty"`
	SubmittedAt    string `json:"submittedAt,omitempty"`
	Status         string `json:"status"`
	FilledQuantity string `json:"filledQuantity,omitempty"`
	FilledPrice    string `json:"filledPrice,omitempty"`
	FilledAt       string `json:"filledAt,omitempty"`
}

// PnLPoint is a single point on the equity curve.
type PnLPoint struct {
	Time   string  `json:"time"`
	Equity float64 `json:"equity"`
}

// DrawdownPoint is a single point on the drawdown curve.
type DrawdownPoint struct {
	Time     string  `json:"time"`
	Drawdown float64 `json:"drawdown"`
}

// Candle is a single OHLCV bar for chart rendering.
type Candle struct {
	Time   string `json:"time"`
	Open   string `json:"open"`
	High   string `json:"high"`
	Low    string `json:"low"`
	Close  string `json:"close"`
	Volume string `json:"volume"`
}

// RunResult holds the output of a backtest run.
type RunResult struct {
	Symbol          string           `json:"symbol"`
	Interval        string           `json:"interval"`
	StartTime       string           `json:"startTime"`
	EndTime         string           `json:"endTime"`
	QuoteCurrency   string           `json:"quoteCurrency"`
	FinalBalance    float64          `json:"finalBalance"`
	PnL             float64          `json:"pnl"`
	MaxDrawdown     float64          `json:"maxDrawdown"`
	CurrentDrawdown float64          `json:"currentDrawdown"`
	TotalTrades     int              `json:"totalTrades"`
	WinRate         float64          `json:"winRate"`
	Trades          []TradeEvent     `json:"trades,omitempty"`
	OrderBook       []OrderBookEntry `json:"orderBook,omitempty"`
	PnLCurve        []PnLPoint       `json:"pnlCurve,omitempty"`
	DrawdownCurve   []DrawdownPoint  `json:"drawdownCurve,omitempty"`
	Candles         []Candle         `json:"candles,omitempty"`
	Logs            []string         `json:"logs"`
	Error           string           `json:"error,omitempty"`

	mu                     sync.Mutex
	RuntimeErrors          []string       `json:"runtimeErrors,omitempty"`
	RuntimeErrorCounts     map[string]int `json:"runtimeErrorCounts,omitempty"`
	RuntimeErrorTotal      int            `json:"runtimeErrorTotal,omitempty"`
	RuntimeErrorsTruncated bool           `json:"runtimeErrorsTruncated,omitempty"`
	runtimeErrorSeen       map[string]struct{}
}

func (r *RunResult) Snapshot() *RunResult {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return &RunResult{
		Symbol:                 r.Symbol,
		Interval:               r.Interval,
		StartTime:              r.StartTime,
		EndTime:                r.EndTime,
		QuoteCurrency:          r.QuoteCurrency,
		FinalBalance:           r.FinalBalance,
		PnL:                    r.PnL,
		MaxDrawdown:            r.MaxDrawdown,
		CurrentDrawdown:        r.CurrentDrawdown,
		TotalTrades:            r.TotalTrades,
		WinRate:                r.WinRate,
		Trades:                 append([]TradeEvent(nil), r.Trades...),
		OrderBook:              append([]OrderBookEntry(nil), r.OrderBook...),
		PnLCurve:               append([]PnLPoint(nil), r.PnLCurve...),
		DrawdownCurve:          append([]DrawdownPoint(nil), r.DrawdownCurve...),
		Candles:                append([]Candle(nil), r.Candles...),
		Logs:                   append([]string(nil), r.Logs...),
		Error:                  r.Error,
		RuntimeErrors:          append([]string(nil), r.RuntimeErrors...),
		RuntimeErrorTotal:      r.RuntimeErrorTotal,
		RuntimeErrorsTruncated: r.RuntimeErrorsTruncated,
		RuntimeErrorCounts: func() map[string]int {
			if len(r.RuntimeErrorCounts) == 0 {
				return nil
			}
			clone := make(map[string]int, len(r.RuntimeErrorCounts))
			maps.Copy(clone, r.RuntimeErrorCounts)
			return clone
		}(),
	}
}

func (r *RunResult) AddRuntimeError(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.RuntimeErrorCounts == nil {
		r.RuntimeErrorCounts = map[string]int{}
	}
	r.RuntimeErrorTotal++
	r.RuntimeErrorCounts[msg]++
	if r.runtimeErrorSeen == nil {
		r.runtimeErrorSeen = map[string]struct{}{}
		for _, existing := range r.RuntimeErrors {
			r.runtimeErrorSeen[existing] = struct{}{}
		}
	}
	if _, exists := r.runtimeErrorSeen[msg]; exists {
		return
	}
	const maxRuntimeErrorSamples = 100
	if len(r.RuntimeErrors) >= maxRuntimeErrorSamples {
		r.RuntimeErrorsTruncated = true
		return
	}
	r.runtimeErrorSeen[msg] = struct{}{}
	r.RuntimeErrors = append(r.RuntimeErrors, msg)
}
