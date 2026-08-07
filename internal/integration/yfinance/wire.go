package yfinance

import "encoding/json"

type remoteErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type remoteHealth struct {
	OK              bool   `json:"ok"`
	YFinanceVersion string `json:"yfinance_version"`
	RuntimeState    string `json:"runtime_state"`
	WarmupError     string `json:"warmup_error"`
}

type remoteMarkets struct {
	Markets []remoteMarketProfile `json:"markets"`
}

type remoteMarketProfile struct {
	Code                   string                `json:"code"`
	ResolvedMarket         string                `json:"resolved_market"`
	PreferredPrefix        string                `json:"preferred_prefix"`
	DisplayName            string                `json:"display_name"`
	QuoteCurrency          string                `json:"quote_currency"`
	Timezone               string                `json:"timezone"`
	SupportsExtendedHours  bool                  `json:"supports_extended_hours"`
	RequiresExchangePrefix bool                  `json:"requires_exchange_prefix"`
	Aliases                []string              `json:"aliases"`
	RegularSessions        []remoteTradingWindow `json:"regular_sessions"`
	Precision              remotePrecision       `json:"precision"`
	TickSize               json.Number           `json:"tick_size"`
}

type remoteTradingWindow struct {
	StartMinute int    `json:"start_minute"`
	EndMinute   int    `json:"end_minute"`
	Label       string `json:"label"`
}

type remotePrecision struct {
	Price int `json:"price"`
	Quote int `json:"quote"`
}

type remoteSearch struct {
	Entries []remoteInstrument `json:"entries"`
}

type remoteInstrument struct {
	Market           string   `json:"market"`
	ResolvedMarket   string   `json:"resolved_market"`
	InstrumentID     string   `json:"instrument_id"`
	Code             string   `json:"code"`
	Symbol           string   `json:"symbol"`
	Name             string   `json:"name"`
	SecurityType     string   `json:"security_type"`
	Exchange         string   `json:"exchange"`
	Selectable       bool     `json:"selectable"`
	Source           string   `json:"source"`
	SupportedPeriods []string `json:"supported_periods"`
}

type remoteSecurity struct {
	Market            string       `json:"market"`
	Symbol            string       `json:"symbol"`
	InstrumentID      string       `json:"instrument_id"`
	Name              string       `json:"name"`
	Exchange          string       `json:"exchange"`
	Currency          string       `json:"currency"`
	Timezone          string       `json:"timezone"`
	SecurityType      string       `json:"security_type"`
	Industry          string       `json:"industry"`
	Sector            string       `json:"sector"`
	Website           string       `json:"website"`
	BusinessSummary   string       `json:"business_summary"`
	MarketCap         *json.Number `json:"market_cap"`
	TrailingPE        *json.Number `json:"trailing_pe"`
	ForwardPE         *json.Number `json:"forward_pe"`
	TrailingEPS       *json.Number `json:"trailing_eps"`
	ForwardEPS        *json.Number `json:"forward_eps"`
	DividendRate      *json.Number `json:"dividend_rate"`
	DividendYield     *json.Number `json:"dividend_yield"`
	FiftyTwoWeekHigh  *json.Number `json:"fifty_two_week_high"`
	FiftyTwoWeekLow   *json.Number `json:"fifty_two_week_low"`
	AverageVolume     *json.Number `json:"average_volume"`
	SharesOutstanding *json.Number `json:"shares_outstanding"`
	Source            string       `json:"source"`
	SupportedPeriods  []string     `json:"supported_periods"`
}

type remoteSnapshotQuote struct {
	Price       *json.Number `json:"price"`
	HighPrice   *json.Number `json:"high_price"`
	LowPrice    *json.Number `json:"low_price"`
	Volume      *json.Number `json:"volume"`
	Turnover    *json.Number `json:"turnover"`
	ChangeValue *json.Number `json:"change_value"`
	ChangeRate  *json.Number `json:"change_rate"`
	QuoteAt     string       `json:"quote_at"`
}

type remoteSnapshot struct {
	Market             string               `json:"market"`
	Symbol             string               `json:"symbol"`
	InstrumentID       string               `json:"instrument_id"`
	Price              *json.Number         `json:"price"`
	Bid                *json.Number         `json:"bid"`
	Ask                *json.Number         `json:"ask"`
	OpenPrice          *json.Number         `json:"open_price"`
	HighPrice          *json.Number         `json:"high_price"`
	LowPrice           *json.Number         `json:"low_price"`
	PreviousClosePrice *json.Number         `json:"previous_close_price"`
	LastClosePrice     *json.Number         `json:"last_close_price"`
	RegularQuote       *remoteSnapshotQuote `json:"regular_quote"`
	PreMarketQuote     *remoteSnapshotQuote `json:"pre_market_quote"`
	AfterMarketQuote   *remoteSnapshotQuote `json:"after_market_quote"`
	Volume             *json.Number         `json:"volume"`
	Turnover           *json.Number         `json:"turnover"`
	QuoteAt            string               `json:"quote_at"`
	ObservedAt         string               `json:"observed_at"`
	Source             string               `json:"source"`
}

type remoteCandles struct {
	Market        string         `json:"market"`
	Symbol        string         `json:"symbol"`
	InstrumentID  string         `json:"instrument_id"`
	Period        string         `json:"period"`
	ExtendedHours bool           `json:"extended_hours"`
	TotalReturned int            `json:"total_returned"`
	HasMore       *bool          `json:"has_more"`
	NextBefore    string         `json:"next_before"`
	Source        string         `json:"source"`
	Candles       []remoteCandle `json:"candles"`
}

type remoteCandle struct {
	At     string       `json:"at"`
	Open   *json.Number `json:"open"`
	High   *json.Number `json:"high"`
	Low    *json.Number `json:"low"`
	Close  *json.Number `json:"close"`
	Volume *json.Number `json:"volume"`
}
