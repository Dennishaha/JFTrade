package marketdata

import srv "github.com/jftrade/jftrade-main/internal/marketdata"

// SubscriptionInstrument documents a market-data subscription target.
type SubscriptionInstrument struct {
	Channel  string `json:"channel,omitempty"`
	Market   string `json:"market"`
	Symbol   string `json:"symbol"`
	Interval string `json:"interval,omitempty"`
}

// SubscriptionRequest documents the market-data subscription payload.
type SubscriptionRequest struct {
	ConsumerID       string                   `json:"consumerId"`
	ProviderBrokerID string                   `json:"providerBrokerId,omitempty"`
	Instruments      []SubscriptionInstrument `json:"instruments,omitempty"`
}

// SubscriptionHeartbeatRequest documents the subscription heartbeat payload.
type SubscriptionHeartbeatRequest struct {
	ConsumerID       string `json:"consumerId"`
	ProviderBrokerID string `json:"providerBrokerId,omitempty"`
}

// MarketsData documents the market profile list returned by the active provider.
type MarketsData struct {
	DefaultMarket string           `json:"defaultMarket"`
	Markets       []map[string]any `json:"markets"`
}

// MarketInstrumentData documents the common normalized instrument identity.
type MarketInstrumentData struct {
	Market       string `json:"market"`
	Symbol       string `json:"symbol"`
	InstrumentID string `json:"instrumentId"`
}

// MarketQueryMeta documents common provider/cache attribution.
type MarketQueryMeta struct {
	InstrumentID  string `json:"instrumentId"`
	Source        string `json:"source"`
	BrokerID      string `json:"brokerId,omitempty"`
	ResolvedAt    string `json:"resolvedAt"`
	FromCache     bool   `json:"fromCache"`
	ExtendedHours *bool  `json:"extendedHours,omitempty"`
	Session       string `json:"session,omitempty"`
}

// SnapshotExtendedQuote documents a provider-neutral pre/after/overnight quote.
type SnapshotExtendedQuote struct {
	Price            *string `json:"price" extensions:"x-nullable"`
	HighPrice        *string `json:"highPrice" extensions:"x-nullable"`
	LowPrice         *string `json:"lowPrice" extensions:"x-nullable"`
	Volume           *string `json:"volume" extensions:"x-nullable"`
	Turnover         *string `json:"turnover" extensions:"x-nullable"`
	ChangeVal        *string `json:"changeVal" extensions:"x-nullable"`
	ChangeRate       *string `json:"changeRate" extensions:"x-nullable"`
	Amplitude        *string `json:"amplitude" extensions:"x-nullable"`
	QuoteTime        string  `json:"quoteTime"`
	TradingDate      string  `json:"tradingDate,omitempty"`
	ExchangeTimezone string  `json:"exchangeTimezone,omitempty"`
	SessionStartAt   string  `json:"sessionStartAt,omitempty"`
	SessionEndAt     string  `json:"sessionEndAt,omitempty"`
}

// SnapshotExtendedQuotes documents the available extended-session blocks.
type SnapshotExtendedQuotes struct {
	PreMarket   *SnapshotExtendedQuote `json:"preMarket" extensions:"x-nullable"`
	AfterMarket *SnapshotExtendedQuote `json:"afterMarket" extensions:"x-nullable"`
	Overnight   *SnapshotExtendedQuote `json:"overnight" extensions:"x-nullable"`
}

// SnapshotQuote documents the normalized market-data snapshot payload.
type SnapshotQuote struct {
	Price              string                 `json:"price"`
	Bid                *string                `json:"bid" extensions:"x-nullable"`
	Ask                *string                `json:"ask" extensions:"x-nullable"`
	OpenPrice          *string                `json:"openPrice" extensions:"x-nullable"`
	HighPrice          *string                `json:"highPrice" extensions:"x-nullable"`
	LowPrice           *string                `json:"lowPrice" extensions:"x-nullable"`
	PreviousClosePrice *string                `json:"previousClosePrice" extensions:"x-nullable"`
	LastClosePrice     *string                `json:"lastClosePrice" extensions:"x-nullable"`
	Volume             *string                `json:"volume" extensions:"x-nullable"`
	Turnover           *string                `json:"turnover" extensions:"x-nullable"`
	At                 string                 `json:"at"`
	ObservedAt         string                 `json:"observedAt"`
	Session            string                 `json:"session"`
	ExtendedHours      bool                   `json:"extendedHours"`
	Extended           SnapshotExtendedQuotes `json:"extended"`
}

// SecurityDetailsPayload documents the provider-neutral security identity and
// authoritative candle periods. Providers may return additional research
// fields in this object.
type SecurityDetailsPayload struct {
	InstrumentID     string   `json:"instrumentId"`
	Market           string   `json:"market"`
	Symbol           string   `json:"symbol"`
	Name             string   `json:"name"`
	Exchange         string   `json:"exchange,omitempty"`
	Currency         string   `json:"currency,omitempty"`
	Timezone         string   `json:"timezone,omitempty"`
	SecurityType     string   `json:"securityType,omitempty"`
	SupportedPeriods []string `json:"supportedPeriods,omitempty"`
}

// SecurityDetailsData documents the security-details query wrapper.
type SecurityDetailsData struct {
	Request  MarketInstrumentData   `json:"request"`
	Security SecurityDetailsPayload `json:"security"`
	Meta     MarketQueryMeta        `json:"meta"`
}

// SnapshotData documents the single-instrument snapshot wrapper.
type SnapshotData struct {
	Request  MarketInstrumentData `json:"request"`
	Snapshot SnapshotQuote        `json:"snapshot"`
	Meta     MarketQueryMeta      `json:"meta"`
}

// CandleRequestData documents the normalized candle request echoed by the API.
type CandleRequestData struct {
	Instrument MarketInstrumentData `json:"instrument"`
	Period     string               `json:"period"`
	Limit      int                  `json:"limit"`
}

// CandlePaginationData documents historical pagination state.
type CandlePaginationData struct {
	HasMore    bool   `json:"hasMore"`
	NextBefore string `json:"nextBefore,omitempty"`
}

// CandlesData documents a candle query result.
type CandlesData struct {
	Request       CandleRequestData     `json:"request"`
	Candles       []map[string]any      `json:"candles"`
	TotalReturned int                   `json:"totalReturned"`
	Pagination    *CandlePaginationData `json:"pagination,omitempty"`
	Meta          MarketQueryMeta       `json:"meta"`
}

// DepthRequestData documents the requested order-book depth.
type DepthRequestData struct {
	Market       string `json:"market"`
	Symbol       string `json:"symbol"`
	InstrumentID string `json:"instrumentId"`
	Num          int    `json:"num"`
}

// DepthData documents a normalized order-book response.
type DepthData struct {
	Request DepthRequestData `json:"request"`
	Depth   map[string]any   `json:"depth"`
	Meta    MarketQueryMeta  `json:"meta"`
}

// SubscriptionEntryData documents one logical subscription lease.
type SubscriptionEntryData struct {
	Key                   string   `json:"key"`
	Channel               string   `json:"channel"`
	Market                string   `json:"market"`
	Symbol                string   `json:"symbol"`
	InstrumentID          string   `json:"instrumentId"`
	Interval              *string  `json:"interval" extensions:"x-nullable"`
	DepthLevel            *int     `json:"depthLevel" extensions:"x-nullable"`
	Consumers             []string `json:"consumers"`
	RefCount              int      `json:"refCount"`
	CreatedAt             string   `json:"createdAt"`
	UpdatedAt             string   `json:"updatedAt"`
	BrokerState           string   `json:"brokerState,omitempty"`
	SubscribedAt          *string  `json:"subscribedAt,omitempty"`
	UnsubscribeEligibleAt *string  `json:"unsubscribeEligibleAt,omitempty"`
	LastError             *string  `json:"lastError,omitempty"`
}

// SubscriptionQuotaBucketData documents quota usage for one market.
type SubscriptionQuotaBucketData struct {
	Market    string `json:"market"`
	Used      int    `json:"used"`
	Limit     *int   `json:"limit" extensions:"x-nullable"`
	Remaining *int   `json:"remaining" extensions:"x-nullable"`
}

// SubscriptionQuotaData documents aggregate physical subscription quota.
type SubscriptionQuotaData struct {
	TotalUsed      int                           `json:"totalUsed"`
	TotalLimit     *int                          `json:"totalLimit" extensions:"x-nullable"`
	TotalRemaining *int                          `json:"totalRemaining" extensions:"x-nullable"`
	ByMarket       []SubscriptionQuotaBucketData `json:"byMarket"`
}

// SubscriptionsData documents logical demand plus broker reconciliation state.
type SubscriptionsData struct {
	TotalActiveSubscriptions int                     `json:"totalActiveSubscriptions"`
	ConsumerID               string                  `json:"consumerId,omitempty"`
	ProviderBrokerID         string                  `json:"providerBrokerId,omitempty"`
	Action                   string                  `json:"action,omitempty"`
	Instruments              []srv.InstrumentRef     `json:"instruments,omitempty"`
	DesiredCount             int                     `json:"desiredCount,omitempty"`
	OwnActiveCount           int                     `json:"ownActiveCount,omitempty"`
	PendingReleaseCount      int                     `json:"pendingReleaseCount,omitempty"`
	TotalUsedQuota           *int                    `json:"totalUsedQuota,omitempty"`
	RemainQuota              *int                    `json:"remainQuota,omitempty"`
	Quota                    SubscriptionQuotaData   `json:"quota"`
	Entries                  []SubscriptionEntryData `json:"entries"`
	BrokerState              map[string]any          `json:"brokerState,omitempty"`
	Transport                map[string]any          `json:"transport,omitempty"`
	Released                 bool                    `json:"released,omitempty"`
	Cleared                  bool                    `json:"cleared,omitempty"`
}

// NormalizeInstrumentRequest documents accepted instrument aliases.
type NormalizeInstrumentRequest struct {
	Market       string `json:"market,omitempty"`
	Symbol       string `json:"symbol,omitempty"`
	Code         string `json:"code,omitempty"`
	InstrumentID string `json:"instrumentId,omitempty"`
}

// NormalizeInstrumentData documents the canonical instrument identity.
type NormalizeInstrumentData struct {
	Market         string `json:"market"`
	Prefix         string `json:"prefix"`
	Code           string `json:"code"`
	Symbol         string `json:"symbol"`
	InstrumentID   string `json:"instrumentId"`
	ResolvedMarket string `json:"resolvedMarket"`
}
