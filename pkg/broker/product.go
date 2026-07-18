package broker

import "time"

// ProductClass is the broker-neutral product taxonomy used by public APIs,
// execution, tools, and capability routing.
type ProductClass string

const (
	ProductClassEquity        ProductClass = "equity"
	ProductClassFund          ProductClass = "fund"
	ProductClassOption        ProductClass = "option"
	ProductClassWarrant       ProductClass = "warrant"
	ProductClassCBBC          ProductClass = "cbbc"
	ProductClassFuture        ProductClass = "future"
	ProductClassEventContract ProductClass = "event_contract"
	ProductClassIndex         ProductClass = "index"
	ProductClassBond          ProductClass = "bond"
	ProductClassPlate         ProductClass = "plate"
	ProductClassUnknown       ProductClass = "unknown"
)

// MarketSegment separates products sharing a geographic market without
// inventing another market identifier. Prediction contracts therefore remain
// in US and use MarketSegmentPrediction.
type MarketSegment string

const (
	MarketSegmentSecurities  MarketSegment = "securities"
	MarketSegmentDerivatives MarketSegment = "derivatives"
	MarketSegmentPrediction  MarketSegment = "prediction"
)

type QuantityMode string

const (
	QuantityModeUnits     QuantityMode = "units"
	QuantityModeContracts QuantityMode = "contracts"
	QuantityModeAmount    QuantityMode = "amount"
)

type OrderKind string

const (
	OrderKindSingle      OrderKind = "single"
	OrderKindOptionCombo OrderKind = "option_combo"
	OrderKindEventSingle OrderKind = "event_single"
	OrderKindEventParlay OrderKind = "event_parlay"
)

// Instrument is the normalized product identity returned by broker-neutral
// services. Broker-specific protobuf messages and enum values must not be
// embedded in this type.
type Instrument struct {
	InstrumentID   string        `json:"instrumentId"`
	Code           string        `json:"code"`
	Name           string        `json:"name,omitempty"`
	ProductClass   ProductClass  `json:"productClass"`
	MarketSegment  MarketSegment `json:"marketSegment"`
	QuoteMarket    string        `json:"quoteMarket"`
	TradeMarket    string        `json:"tradeMarket,omitempty"`
	Venue          string        `json:"venue,omitempty"`
	Currency       string        `json:"currency,omitempty"`
	UnderlyingCode string        `json:"underlyingCode,omitempty"`
	OptionType     string        `json:"optionType,omitempty"`
	StrikePrice    *float64      `json:"strikePrice,omitempty"`
	ExpiryDate     string        `json:"expiryDate,omitempty"`
	ContractSize   *float64      `json:"contractSize,omitempty"`
	PriceTick      *float64      `json:"priceTick,omitempty"`
	QuantityMode   QuantityMode  `json:"quantityMode"`
	Event          *EventProduct `json:"event,omitempty"`
}

// EventProduct carries prediction-market lifecycle identity and state.
type EventProduct struct {
	CategoryID     string `json:"categoryId,omitempty"`
	CompetitionID  string `json:"competitionId,omitempty"`
	SeriesID       string `json:"seriesId,omitempty"`
	EventID        string `json:"eventId,omitempty"`
	ContractID     string `json:"contractId,omitempty"`
	Status         string `json:"status,omitempty"`
	CloseAt        string `json:"closeAt,omitempty"`
	SettlementAt   string `json:"settlementAt,omitempty"`
	SettlementSide string `json:"settlementSide,omitempty"`
}

// ProviderAttribution is included in every advanced market/research response.
type ProviderAttribution struct {
	BrokerID        string          `json:"brokerId"`
	SecurityFirm    string          `json:"securityFirm,omitempty"`
	FeatureID       FeatureID       `json:"featureId"`
	Capability      CapabilityState `json:"capability"`
	SelectionReason string          `json:"selectionReason"`
	ResolvedAt      time.Time       `json:"resolvedAt"`
	AsOf            time.Time       `json:"asOf"`
}
