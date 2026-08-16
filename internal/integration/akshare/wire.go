package akshare

import "encoding/json"

type remoteErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type remoteHealth struct {
	OK              bool   `json:"ok"`
	Source          string `json:"source"`
	Provider        string `json:"provider"`
	Version         string `json:"version"`
	ProviderVersion string `json:"provider_version"`
	AKShareVersion  string `json:"akshare_version"`
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
	TickSize               *json.Number          `json:"tick_size"`
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
	MarketCap         *json.Number `json:"market_cap"`
	AverageVolume     *json.Number `json:"average_volume"`
	TrailingPE        *json.Number `json:"trailing_pe"`
	SharesOutstanding *json.Number `json:"shares_outstanding"`
	Source            string       `json:"source"`
	SupportedPeriods  []string     `json:"supported_periods"`
}

type remoteSnapshot struct {
	Market             string       `json:"market"`
	Symbol             string       `json:"symbol"`
	InstrumentID       string       `json:"instrument_id"`
	Price              *json.Number `json:"price"`
	Bid                *json.Number `json:"bid"`
	Ask                *json.Number `json:"ask"`
	OpenPrice          *json.Number `json:"open_price"`
	HighPrice          *json.Number `json:"high_price"`
	LowPrice           *json.Number `json:"low_price"`
	PreviousClosePrice *json.Number `json:"previous_close_price"`
	LastClosePrice     *json.Number `json:"last_close_price"`
	Volume             *json.Number `json:"volume"`
	Turnover           *json.Number `json:"turnover"`
	QuoteAt            string       `json:"quote_at"`
	ObservedAt         string       `json:"observed_at"`
	Source             string       `json:"source"`
}

type remoteBatchRequest struct {
	InstrumentIDs []string `json:"instrument_ids"`
}

type remoteBatchError struct {
	InstrumentID string `json:"instrument_id"`
	Code         string `json:"code"`
	Message      string `json:"message"`
}

type remoteBatchSnapshots struct {
	Entries   []remoteSnapshot   `json:"entries"`
	Snapshots []remoteSnapshot   `json:"snapshots"`
	Errors    []remoteBatchError `json:"errors"`
}

func (r remoteBatchSnapshots) values() []remoteSnapshot {
	if r.Entries != nil {
		return r.Entries
	}
	return r.Snapshots
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

type remoteIndexConstituents struct {
	Market       string                   `json:"market"`
	Symbol       string                   `json:"symbol"`
	InstrumentID string                   `json:"instrument_id"`
	Constituents []remoteIndexConstituent `json:"constituents"`
	Source       string                   `json:"source"`
}

type remoteIndexConstituent struct {
	Code   string       `json:"code"`
	Name   string       `json:"name"`
	Weight *json.Number `json:"weight"`
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

type remoteIndustryBoards struct {
	Market string                `json:"market"`
	Kind   string                `json:"kind"`
	Boards []remoteIndustryBoard `json:"boards"`
	Source string                `json:"source"`
}

type remoteIndustryBoard struct {
	Name                   string       `json:"name"`
	ChangeRate             *json.Number `json:"change_rate"`
	Turnover               *json.Number `json:"turnover"`
	Volume                 *json.Number `json:"volume"`
	LeadingStockName       string       `json:"leading_stock_name"`
	LeadingStockChangeRate *json.Number `json:"leading_stock_change_rate"`
}

type remoteIndustryMembers struct {
	Market  string               `json:"market"`
	Kind    string               `json:"kind"`
	Board   string               `json:"board"`
	Entries []remoteRankingEntry `json:"entries"`
	Source  string               `json:"source"`
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

// remoteAnalystConsensus mirrors the sidecar AnalystResponse model; the
// akshare route aggregates Eastmoney research reports, so target_price is
// always null upstream.
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
