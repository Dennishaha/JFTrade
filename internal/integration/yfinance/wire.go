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

type remoteNews struct {
	Market       string            `json:"market"`
	Symbol       string            `json:"symbol"`
	InstrumentID string            `json:"instrument_id"`
	Entries      []remoteNewsEntry `json:"entries"`
	Source       string            `json:"source"`
}

type remoteNewsEntry struct {
	Title       *string `json:"title"`
	Link        *string `json:"link"`
	Publisher   *string `json:"publisher"`
	PublishedAt *string `json:"published_at"`
	Summary     *string `json:"summary"`
}

type remoteCorporateActions struct {
	Market       string                  `json:"market"`
	Symbol       string                  `json:"symbol"`
	InstrumentID string                  `json:"instrument_id"`
	Events       []remoteCorporateAction `json:"events"`
	Source       string                  `json:"source"`
}

type remoteCorporateAction struct {
	Kind   string       `json:"kind"`
	ExDate string       `json:"ex_date"`
	Amount *json.Number `json:"amount"`
	Ratio  *json.Number `json:"ratio"`
}

type remoteRankings struct {
	Market  string               `json:"market"`
	Kind    string               `json:"kind"`
	Entries []remoteRankingEntry `json:"entries"`
	Source  string               `json:"source"`
}

type remoteRankingEntry struct {
	InstrumentID  string       `json:"instrument_id"`
	Name          string       `json:"name"`
	Price         *json.Number `json:"price"`
	ChangeRate    *json.Number `json:"change_rate"`
	ChangeAmount  *json.Number `json:"change_amount"`
	Volume        *json.Number `json:"volume"`
	Turnover      *json.Number `json:"turnover"`
	TurnoverRatio *json.Number `json:"turnover_ratio"`
	PETTM         *json.Number `json:"pe_ttm"`
	MarketCap     *json.Number `json:"market_cap"`
}

type remoteCompanyProfile struct {
	InstrumentID string                      `json:"instrument_id"`
	Market       string                      `json:"market"`
	Symbol       string                      `json:"symbol"`
	Currency     *string                     `json:"currency"`
	Groups       []remoteCompanyProfileGroup `json:"groups"`
}

type remoteCompanyProfileGroup struct {
	Title  string                      `json:"title"`
	Fields []remoteCompanyProfileField `json:"fields"`
}

type remoteCompanyProfileField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type remoteFinancialStatements struct {
	InstrumentID string                           `json:"instrument_id"`
	Statement    string                           `json:"statement"`
	Currency     *string                          `json:"currency"`
	Fields       []remoteFinancialStatementField  `json:"fields"`
	Periods      []remoteFinancialStatementPeriod `json:"periods"`
}

type remoteFinancialStatementField struct {
	FieldID     string `json:"field_id"`
	DisplayName string `json:"display_name"`
}

type remoteFinancialStatementPeriod struct {
	PeriodText string                                   `json:"period_text"`
	Values     map[string]remoteFinancialStatementValue `json:"values"`
}

type remoteFinancialStatementValue struct {
	Data *json.Number `json:"data"`
	YoY  *json.Number `json:"yoy"`
	QoQ  *json.Number `json:"qoq"`
}

type remoteAnalystConsensus struct {
	InstrumentID string                     `json:"instrument_id"`
	Rating       *json.Number               `json:"rating"`
	AnalystCount *json.Number               `json:"analyst_count"`
	TargetPrice  *remoteAnalystTargetPrice  `json:"target_price"`
	Distribution *remoteAnalystDistribution `json:"distribution"`
	UpdateTime   *string                    `json:"update_time"`
}

type remoteAnalystTargetPrice struct {
	Lowest  *json.Number `json:"lowest"`
	Average *json.Number `json:"average"`
	Highest *json.Number `json:"highest"`
}

type remoteAnalystDistribution struct {
	StrongBuy    *json.Number `json:"strong_buy"`
	Buy          *json.Number `json:"buy"`
	Hold         *json.Number `json:"hold"`
	Underperform *json.Number `json:"underperform"`
	Sell         *json.Number `json:"sell"`
}

type remoteOwnership struct {
	InstrumentID string                 `json:"instrument_id"`
	Groups       []remoteOwnershipGroup `json:"groups"`
}

type remoteOwnershipGroup struct {
	Kind       string                `json:"kind"`
	StaticDate *string               `json:"static_date"`
	Items      []remoteOwnershipItem `json:"items"`
}

type remoteOwnershipItem struct {
	Name      string       `json:"name"`
	HolderPct *json.Number `json:"holder_pct"`
}

type remoteScreenRequest struct {
	Market     string                  `json:"market"`
	Conditions []remoteScreenCondition `json:"conditions,omitempty"`
	Sorts      []remoteScreenSort      `json:"sorts,omitempty"`
	Offset     int                     `json:"offset"`
	Limit      int                     `json:"limit"`
}

type remoteScreenCondition struct {
	FactorKey string      `json:"factor_key"`
	Min       json.Number `json:"min,omitempty"`
	Max       json.Number `json:"max,omitempty"`
}

type remoteScreenSort struct {
	FactorKey string `json:"factor_key"`
	Direction string `json:"direction"`
}

type remoteScreenResponse struct {
	Entries    []remoteScreenEntry `json:"entries"`
	Total      int                 `json:"total"`
	HasMore    bool                `json:"has_more"`
	NextOffset *int                `json:"next_offset"`
	AsOf       string              `json:"as_of"`
	Source     string              `json:"source"`
}

type remoteScreenEntry struct {
	InstrumentID  string                 `json:"instrument_id"`
	Name          string                 `json:"name"`
	Symbol        *string                `json:"symbol"`
	Industry      *string                `json:"industry"`
	QuoteCurrency string                 `json:"quote_currency"`
	Values        map[string]json.Number `json:"values"`
}
