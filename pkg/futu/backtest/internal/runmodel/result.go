package runmodel

import "sync"

// TradeEvent is a single filled trade for chart rendering.
type TradeEvent struct {
	Time  string  `json:"time"`
	Side  string  `json:"side"`
	Price float64 `json:"price"`
	Qty   float64 `json:"qty"`
	PnL   float64 `json:"pnl,omitempty"`
}

// OrderBookEntry captures a single backtest order and its latest fill outcome.
// A submitted order that later fills is merged into the same row.
type OrderBookEntry struct {
	OrderID        string  `json:"orderId"`
	ClientOrderID  string  `json:"clientOrderId,omitempty"`
	Symbol         string  `json:"symbol"`
	Side           string  `json:"side"`
	Quantity       float64 `json:"quantity"`
	OrderType      string  `json:"orderType,omitempty"`
	OrderPrice     float64 `json:"orderPrice,omitempty"`
	SubmittedAt    string  `json:"submittedAt,omitempty"`
	Status         string  `json:"status"`
	FilledQuantity float64 `json:"filledQuantity,omitempty"`
	FilledPrice    float64 `json:"filledPrice,omitempty"`
	FilledAt       string  `json:"filledAt,omitempty"`
}

// PnLPoint is a single point on the equity curve.
type PnLPoint struct {
	Time   string  `json:"time"`
	Equity float64 `json:"equity"`
}

// Candle is a single OHLCV bar for chart rendering.
type Candle struct {
	Time   string  `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// RunResult holds the output of a backtest run.
type RunResult struct {
	Symbol        string           `json:"symbol"`
	Interval      string           `json:"interval"`
	StartTime     string           `json:"startTime"`
	EndTime       string           `json:"endTime"`
	QuoteCurrency string           `json:"quoteCurrency"`
	FinalBalance  float64          `json:"finalBalance"`
	PnL           float64          `json:"pnl"`
	TotalTrades   int              `json:"totalTrades"`
	WinRate       float64          `json:"winRate"`
	Trades        []TradeEvent     `json:"trades,omitempty"`
	OrderBook     []OrderBookEntry `json:"orderBook,omitempty"`
	PnLCurve      []PnLPoint       `json:"pnlCurve,omitempty"`
	Candles       []Candle         `json:"candles,omitempty"`
	Logs          []string         `json:"logs"`
	Error         string           `json:"error,omitempty"`

	mu            sync.Mutex
	RuntimeErrors []string `json:"runtimeErrors,omitempty"`
}

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

func (r *RunResult) AddRuntimeError(msg string) {
	r.mu.Lock()
	r.RuntimeErrors = append(r.RuntimeErrors, msg)
	r.mu.Unlock()
}
