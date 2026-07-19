package runmodel

import (
	"fmt"
	"maps"
	"strings"
	"sync"
)

// TradeEvent is a single filled trade for chart rendering.
type TradeEvent struct {
	Time        string  `json:"time"`
	Side        string  `json:"side"`
	Price       string  `json:"price"`
	Qty         string  `json:"qty"`
	PnL         float64 `json:"pnl,omitempty"`
	BrokerFee   float64 `json:"brokerFee,omitempty"`
	MarketFee   float64 `json:"marketFee,omitempty"`
	TotalFee    float64 `json:"totalFee,omitempty"`
	FeeCurrency string  `json:"feeCurrency,omitempty"`
}

// OrderBookEntry captures a single backtest order and its latest fill outcome.
// A submitted order that later fills is merged into the same row.
type OrderBookEntry struct {
	OrderID        string  `json:"orderId"`
	ClientOrderID  string  `json:"clientOrderId,omitempty"`
	Symbol         string  `json:"symbol"`
	Side           string  `json:"side"`
	Quantity       string  `json:"quantity"`
	OrderType      string  `json:"orderType,omitempty"`
	OrderPrice     string  `json:"orderPrice,omitempty"`
	SubmittedAt    string  `json:"submittedAt,omitempty"`
	Status         string  `json:"status"`
	FilledQuantity string  `json:"filledQuantity,omitempty"`
	FilledPrice    string  `json:"filledPrice,omitempty"`
	FilledAt       string  `json:"filledAt,omitempty"`
	BrokerFee      float64 `json:"brokerFee,omitempty"`
	MarketFee      float64 `json:"marketFee,omitempty"`
	TotalFee       float64 `json:"totalFee,omitempty"`
	FeeCurrency    string  `json:"feeCurrency,omitempty"`
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

// Candle is a single OHLCV bar for chart rendering in backtest results.
// Presentation DTO, not a numeric model: OHLCV are decimal-exact strings
// (fixed-point .String()) and Time is the bar's close timestamp (bbgo
// EndTime) formatted as RFC3339Nano UTC. It is built in exactly one place —
// the backtest result collector from bbgo K-lines — and persisted as part of
// RunResult. Worker runtimes use the numeric pineworker.Candle wire type.
type Candle struct {
	Time   string `json:"time"`
	Open   string `json:"open"`
	High   string `json:"high"`
	Low    string `json:"low"`
	Close  string `json:"close"`
	Volume string `json:"volume"`
}

// FeeBreakdownEntry is an aggregated fee amount for one configured fee rule.
type FeeBreakdownEntry struct {
	RuleID   string  `json:"ruleId"`
	Label    string  `json:"label"`
	Group    string  `json:"group"`
	Category string  `json:"category"`
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
	Count    int     `json:"count"`
}

// RunResult holds the output of a backtest run.
type RunResult struct {
	Symbol            string              `json:"symbol"`
	Interval          string              `json:"interval"`
	StartTime         string              `json:"startTime"`
	EndTime           string              `json:"endTime"`
	QuoteCurrency     string              `json:"quoteCurrency"`
	FinalBalance      float64             `json:"finalBalance"`
	PnL               float64             `json:"pnl"`
	TotalBrokerFees   float64             `json:"totalBrokerFees,omitempty"`
	TotalMarketFees   float64             `json:"totalMarketFees,omitempty"`
	TotalFees         float64             `json:"totalFees,omitempty"`
	FeeBreakdown      []FeeBreakdownEntry `json:"feeBreakdown,omitempty"`
	TradingCosts      TradingCosts        `json:"tradingCosts"`
	MaxDrawdown       float64             `json:"maxDrawdown"`
	CurrentDrawdown   float64             `json:"currentDrawdown"`
	TotalTrades       int                 `json:"totalTrades"`
	WinRate           float64             `json:"winRate"`
	Trades            []TradeEvent        `json:"trades,omitempty"`
	OrderBook         []OrderBookEntry    `json:"orderBook,omitempty"`
	PnLCurve          []PnLPoint          `json:"pnlCurve,omitempty"`
	DrawdownCurve     []DrawdownPoint     `json:"drawdownCurve,omitempty"`
	Candles           []Candle            `json:"candles,omitempty"`
	Logs              []string            `json:"logs"`
	Warnings          []string            `json:"warnings,omitempty"`
	WarningTotal      int                 `json:"warningTotal,omitempty"`
	WarningsTruncated bool                `json:"warningsTruncated,omitempty"`
	IgnoredOrders     int                 `json:"ignoredOrders,omitempty"`
	ExecutionModel    string              `json:"executionModel,omitempty"`
	Error             string              `json:"error,omitempty"`

	mu                     sync.Mutex
	RuntimeErrors          []string       `json:"runtimeErrors,omitempty"`
	RuntimeErrorCounts     map[string]int `json:"runtimeErrorCounts,omitempty"`
	RuntimeErrorTotal      int            `json:"runtimeErrorTotal,omitempty"`
	RuntimeErrorsTruncated bool           `json:"runtimeErrorsTruncated,omitempty"`
	runtimeErrorSeen       map[string]struct{}
	warningGroupCounts     map[string]int
	warningGroupIndexes    map[string]int
	warningGroupMessages   map[string]string
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
		TotalBrokerFees:        r.TotalBrokerFees,
		TotalMarketFees:        r.TotalMarketFees,
		TotalFees:              r.TotalFees,
		FeeBreakdown:           append([]FeeBreakdownEntry(nil), r.FeeBreakdown...),
		TradingCosts:           cloneTradingCosts(r.TradingCosts),
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
		Warnings:               append([]string(nil), r.Warnings...),
		WarningTotal:           r.WarningTotal,
		WarningsTruncated:      r.WarningsTruncated,
		IgnoredOrders:          r.IgnoredOrders,
		ExecutionModel:         r.ExecutionModel,
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

func cloneTradingCosts(costs TradingCosts) TradingCosts {
	return TradingCosts{
		BrokerFees: cloneFeeSchedule(costs.BrokerFees),
		MarketFees: cloneFeeSchedule(costs.MarketFees),
	}
}

func cloneFeeSchedule(schedule FeeSchedule) FeeSchedule {
	rules := make([]FeeRule, len(schedule.Rules))
	copy(rules, schedule.Rules)
	for index := range rules {
		rules[index].AppliesTo = append([]string(nil), schedule.Rules[index].AppliesTo...)
	}
	return FeeSchedule{
		Mode:     schedule.Mode,
		PresetID: schedule.PresetID,
		Rules:    rules,
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

func (r *RunResult) AddWarning(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addWarningLocked(msg)
}

func (r *RunResult) AddIgnoredOrderWarning(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.IgnoredOrders++
	r.addWarningLocked(msg)
}

func (r *RunResult) AddIgnoredOrderWarningGroup(key string, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.IgnoredOrders++
	key = strings.TrimSpace(key)
	if key == "" {
		r.addWarningLocked(msg)
		return
	}
	if r.warningGroupCounts == nil {
		r.warningGroupCounts = map[string]int{}
		r.warningGroupIndexes = map[string]int{}
		r.warningGroupMessages = map[string]string{}
	}
	if count := r.warningGroupCounts[key]; count > 0 {
		count++
		r.warningGroupCounts[key] = count
		if index, ok := r.warningGroupIndexes[key]; ok && index >= 0 && index < len(r.Warnings) {
			r.Warnings[index] = groupedWarningMessage(r.warningGroupMessages[key], count)
		}
		return
	}
	r.warningGroupCounts[key] = 1
	r.warningGroupMessages[key] = msg
	r.WarningTotal++
	const maxWarningSamples = 100
	if len(r.Warnings) >= maxWarningSamples {
		r.WarningsTruncated = true
		r.warningGroupIndexes[key] = -1
		return
	}
	r.warningGroupIndexes[key] = len(r.Warnings)
	r.Warnings = append(r.Warnings, msg)
}

func (r *RunResult) addWarningLocked(msg string) {
	r.WarningTotal++
	const maxWarningSamples = 100
	if len(r.Warnings) >= maxWarningSamples {
		r.WarningsTruncated = true
		return
	}
	r.Warnings = append(r.Warnings, msg)
}

func groupedWarningMessage(msg string, count int) string {
	if count <= 1 {
		return msg
	}
	return fmt.Sprintf("%s (occurred %d times; first occurrence shown)", msg, count)
}
