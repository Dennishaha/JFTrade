package runmodel

import "time"

// RunConfig describes a single backtest run.
type RunConfig struct {
	DBPath           string       `json:"dbPath"`
	Market           string       `json:"market,omitempty"`
	Symbol           string       `json:"symbol"`
	Interval         string       `json:"interval"`
	SourceFormat     string       `json:"sourceFormat"`
	StartTime        time.Time    `json:"startTime"`
	EndTime          time.Time    `json:"endTime"`
	StrategyScript   string       `json:"strategyScript"`
	InitialBalance   float64      `json:"initialBalance"`
	WarmupCandles    int          `json:"warmupCandles"`
	QuoteCurrency    string       `json:"quoteCurrency"`
	RehabType        string       `json:"rehabType"`
	UseExtendedHours *bool        `json:"useExtendedHours,omitempty"`
	InstrumentType   string       `json:"instrumentType,omitempty"`
	TradingCosts     TradingCosts `json:"tradingCosts"`
}

// TradingCosts describes broker-side and market-side costs for a backtest run.
type TradingCosts struct {
	BrokerFees FeeSchedule `json:"brokerFees"`
	MarketFees FeeSchedule `json:"marketFees"`
}

// FeeSchedule selects a preset/script/default fee schedule or carries custom rules.
type FeeSchedule struct {
	Mode     string    `json:"mode,omitempty"`
	PresetID string    `json:"presetId,omitempty"`
	Rules    []FeeRule `json:"rules,omitempty"`
}

// FeeRule is a versioned, source-attributed transaction fee rule.
type FeeRule struct {
	ID            string   `json:"id"`
	Label         string   `json:"label"`
	Category      string   `json:"category"`
	Side          string   `json:"side,omitempty"`
	Basis         string   `json:"basis"`
	Rate          float64  `json:"rate,omitempty"`
	FixedAmount   float64  `json:"fixedAmount,omitempty"`
	MinAmount     float64  `json:"minAmount,omitempty"`
	MaxAmount     float64  `json:"maxAmount,omitempty"`
	MaxRate       float64  `json:"maxRate,omitempty"`
	Rounding      string   `json:"rounding,omitempty"`
	Currency      string   `json:"currency,omitempty"`
	AppliesTo     []string `json:"appliesTo,omitempty"`
	EffectiveFrom string   `json:"effectiveFrom,omitempty"`
	EffectiveTo   string   `json:"effectiveTo,omitempty"`
	SourceURL     string   `json:"sourceUrl,omitempty"`
}
