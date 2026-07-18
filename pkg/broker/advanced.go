package broker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// FeatureQuery is the common broker-neutral request envelope for optional
// feature families. Params are validated by the business service before an
// adapter sees them.
type FeatureQuery struct {
	BrokerID           string         `json:"brokerId,omitempty"`
	AccountID          string         `json:"accountId,omitempty"`
	TradingEnvironment string         `json:"tradingEnvironment,omitempty"`
	Market             string         `json:"market,omitempty"`
	MarketSegment      MarketSegment  `json:"marketSegment,omitempty"`
	ProductClass       ProductClass   `json:"productClass,omitempty"`
	InstrumentID       string         `json:"instrumentId,omitempty"`
	FeatureID          FeatureID      `json:"featureId"`
	Cursor             string         `json:"cursor,omitempty"`
	PageSize           int            `json:"pageSize,omitempty"`
	Params             map[string]any `json:"params,omitempty"`
}

// FeatureResult is the normalized result envelope. Raw protobuf messages are
// never exposed here; adapters must convert them into JSON-compatible values.
type FeatureResult struct {
	Provider           ProviderAttribution   `json:"provider"`
	ResolvedInstrument *Instrument           `json:"resolvedInstrument,omitempty"`
	AsOf               time.Time             `json:"asOf"`
	Entries            []map[string]any      `json:"entries"`
	NextCursor         string                `json:"nextCursor,omitempty"`
	HasMore            *bool                 `json:"hasMore,omitempty"`
	Total              *int                  `json:"total,omitempty"`
	Warnings           []string              `json:"warnings,omitempty"`
	PartialErrors      []FeaturePartialError `json:"partialErrors,omitempty"`
	Metadata           map[string]any        `json:"metadata,omitempty"`
}

type FeaturePartialError struct {
	Scope   string `json:"scope"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// OptionZeroDteChainLocator is the broker-neutral continuation token returned
// by a 0DTE underlying screener row. It deliberately excludes OpenD protobuf
// types while preserving the fields required to query that row's contracts.
type OptionZeroDteChainLocator struct {
	ProductCode    string  `json:"productCode"`
	Multiplier     float64 `json:"multiplier,omitempty"`
	ContractSize   float64 `json:"contractSize,omitempty"`
	ExpirationType int32   `json:"expirationType,omitempty"`
}

type MarketMicrostructureReader interface {
	QueryMarketMicrostructure(ctx context.Context, query FeatureQuery) (*FeatureResult, error)
}

type InstrumentProfileReader interface {
	QueryInstrumentProfile(ctx context.Context, query FeatureQuery) (*FeatureResult, error)
}

type DerivativeCatalogReader interface {
	QueryDerivativeCatalog(ctx context.Context, query FeatureQuery) (*FeatureResult, error)
}

type OptionAnalyticsReader interface {
	QueryOptionAnalytics(ctx context.Context, query FeatureQuery) (*FeatureResult, error)
}

type InstrumentResearchReader interface {
	QueryInstrumentResearch(ctx context.Context, query FeatureQuery) (*FeatureResult, error)
}

type MarketResearchReader interface {
	QueryMarketResearch(ctx context.Context, query FeatureQuery) (*FeatureResult, error)
}

type PredictionMarketReader interface {
	QueryPredictionMarket(ctx context.Context, query FeatureQuery) (*FeatureResult, error)
	SubscribePredictionMarket(ctx context.Context, subscription PredictionSubscription) error
	UnsubscribePredictionMarket(ctx context.Context, subscription PredictionSubscription) error
}

type PredictionSubscription struct {
	BrokerID     string   `json:"brokerId,omitempty"`
	AccountID    string   `json:"accountId,omitempty"`
	InstrumentID string   `json:"instrumentId"`
	DataTypes    []string `json:"dataTypes"`
}

type PredictionMarketUpdate struct {
	InstrumentID string           `json:"instrumentId"`
	DataType     string           `json:"dataType"`
	Sequence     string           `json:"sequence,omitempty"`
	AsOf         time.Time        `json:"asOf"`
	Entries      []map[string]any `json:"entries"`
}

type PredictionMarketStreamSource interface {
	OnPredictionMarketUpdate(func(PredictionMarketUpdate)) func()
}

type PredictionQuoteRecord struct {
	QuoteID            string     `json:"quoteId"`
	BrokerID           string     `json:"brokerId"`
	AccountID          string     `json:"accountId"`
	TradingEnvironment string     `json:"tradingEnvironment"`
	MVC                string     `json:"mvc"`
	LegsHash           string     `json:"legsHash"`
	BidPrice           *float64   `json:"bidPrice,omitempty"`
	AskPrice           *float64   `json:"askPrice,omitempty"`
	ShouldRetry        bool       `json:"shouldRetry"`
	ReceivedAt         time.Time  `json:"receivedAt"`
	ExpiresAt          time.Time  `json:"expiresAt"`
	ExpirySource       string     `json:"expirySource"`
	Status             string     `json:"status"`
	ConsumedAt         *time.Time `json:"consumedAt,omitempty"`
	ConsumedPreviewID  string     `json:"consumedPreviewId,omitempty"`
	ConsumedClientID   string     `json:"consumedClientOrderId,omitempty"`
}

type PredictionQuoteStore interface {
	SavePredictionQuote(context.Context, PredictionQuoteRecord) error
	ValidatePredictionQuote(
		context.Context,
		string,
		string,
		string,
		string,
		string,
		string,
	) (PredictionQuoteRecord, error)
	ConsumePredictionQuote(
		context.Context,
		string,
		string,
		string,
		string,
		string,
		string,
		string,
		string,
	) error
}

func PredictionQuoteLegsHash(mvc string, legs []OrderLegIntent) string {
	normalized := make([]OrderLegIntent, len(legs))
	for index, leg := range legs {
		leg.InstrumentID = strings.ToUpper(strings.TrimSpace(leg.InstrumentID))
		leg.Side = strings.ToUpper(strings.TrimSpace(leg.Side))
		leg.PredictionSide = strings.ToUpper(strings.TrimSpace(leg.PredictionSide))
		normalized[index] = leg
	}
	content, _ := json.Marshal(struct {
		MVC  string           `json:"mvc"`
		Legs []OrderLegIntent `json:"legs"`
	}{MVC: strings.TrimSpace(mvc), Legs: normalized})
	digest := sha256.Sum256(content)
	return fmt.Sprintf("%x", digest[:])
}

type TechnicalIndicatorReader interface {
	QueryTechnicalIndicator(ctx context.Context, query FeatureQuery) (*FeatureResult, error)
}

type CustomizationAction struct {
	FeatureID FeatureID      `json:"featureId"`
	BrokerID  string         `json:"brokerId,omitempty"`
	AccountID string         `json:"accountId,omitempty"`
	Action    string         `json:"action"`
	Payload   map[string]any `json:"payload,omitempty"`
}

type CustomizationResult struct {
	Provider ProviderAttribution `json:"provider"`
	Entries  []map[string]any    `json:"entries,omitempty"`
}

type CustomizationService interface {
	QueryCustomization(ctx context.Context, query FeatureQuery) (*FeatureResult, error)
	ApplyCustomization(ctx context.Context, action CustomizationAction) (*CustomizationResult, error)
}

type ProductRuleQuery struct {
	ReadQuery
	FeatureID  FeatureID        `json:"featureId"`
	Instrument Instrument       `json:"instrument"`
	OrderKind  OrderKind        `json:"orderKind"`
	OrderType  string           `json:"orderType,omitempty"`
	Session    string           `json:"session,omitempty"`
	Quantity   *float64         `json:"quantity,omitempty"`
	Amount     *float64         `json:"amount,omitempty"`
	Price      *float64         `json:"price,omitempty"`
	Legs       []OrderLegIntent `json:"legs,omitempty"`
}

type ProductRuleResult struct {
	Allowed           bool                      `json:"allowed"`
	ReasonCode        string                    `json:"reasonCode,omitempty"`
	Reason            string                    `json:"reason,omitempty"`
	NormalizedRequest json.RawMessage           `json:"normalizedRequest,omitempty"`
	Warnings          []string                  `json:"warnings,omitempty"`
	BuyingPowerImpact *float64                  `json:"buyingPowerImpact,omitempty"`
	AccountImpact     *OptionComboAccountImpact `json:"accountImpact,omitempty"`
	OptionAnalysis    *OptionComboAnalysis      `json:"optionAnalysis,omitempty"`
}

type OptionComboAccountImpact struct {
	NLVChange               *float64 `json:"nlvChange,omitempty"`
	InitialMarginChange     *float64 `json:"initialMarginChange,omitempty"`
	MaintenanceMarginChange *float64 `json:"maintenanceMarginChange,omitempty"`
	OptionBuyingPower       *float64 `json:"optionBuyingPower,omitempty"`
	MaxWithdrawalChange     *float64 `json:"maxWithdrawalChange,omitempty"`
	BuyingPowerDecrease     *float64 `json:"buyingPowerDecrease,omitempty"`
}

type OptionComboAnalysis struct {
	Strategy           string    `json:"strategy,omitempty"`
	Bid                *float64  `json:"bid,omitempty"`
	Ask                *float64  `json:"ask,omitempty"`
	MaxProfit          *float64  `json:"maxProfit,omitempty"`
	MaxLoss            *float64  `json:"maxLoss,omitempty"`
	MaxProfitUnlimited bool      `json:"maxProfitUnlimited,omitempty"`
	MaxLossUnlimited   bool      `json:"maxLossUnlimited,omitempty"`
	BreakevenPoints    []float64 `json:"breakevenPoints,omitempty"`
	Probability        *float64  `json:"probability,omitempty"`
	Delta              *float64  `json:"delta,omitempty"`
	Theta              *float64  `json:"theta,omitempty"`
}

type ProductRuleProvider interface {
	ValidateProductOrder(ctx context.Context, query ProductRuleQuery) (*ProductRuleResult, error)
}

type OrderLegIntent struct {
	InstrumentID   string       `json:"instrumentId"`
	ProductClass   ProductClass `json:"productClass"`
	Side           string       `json:"side"`
	Ratio          int          `json:"ratio"`
	Quantity       *float64     `json:"quantity,omitempty"`
	Amount         *float64     `json:"amount,omitempty"`
	Price          *float64     `json:"price,omitempty"`
	PredictionSide string       `json:"predictionSide,omitempty"`
}

type ComboOrderIntent struct {
	ReadQuery
	ClientOrderID  string           `json:"clientOrderId"`
	OrderKind      OrderKind        `json:"orderKind"`
	ProductClass   ProductClass     `json:"productClass"`
	PreviewID      string           `json:"previewId"`
	RFQID          string           `json:"rfqId,omitempty"`
	MVC            string           `json:"mvc,omitempty"`
	UnderlyingID   string           `json:"underlyingInstrumentId,omitempty"`
	OptionStrategy string           `json:"optionStrategy,omitempty"`
	NearExpiry     string           `json:"nearExpiry,omitempty"`
	FarExpiry      string           `json:"farExpiry,omitempty"`
	Spread         *float64         `json:"spread,omitempty"`
	QuoteExpiresAt *time.Time       `json:"quoteExpiresAt,omitempty"`
	Amount         *float64         `json:"amount,omitempty"`
	Price          *float64         `json:"price,omitempty"`
	Legs           []OrderLegIntent `json:"legs"`
}

type ComboOrderResult struct {
	BrokerOrderID string             `json:"brokerOrderId"`
	Status        string             `json:"status"`
	Legs          []OrderLegSnapshot `json:"legs,omitempty"`
}

type OrderLegSnapshot struct {
	BrokerLegID       string       `json:"brokerLegId,omitempty"`
	InstrumentID      string       `json:"instrumentId"`
	ProductClass      ProductClass `json:"productClass,omitempty"`
	Side              string       `json:"side,omitempty"`
	Ratio             int          `json:"ratio,omitempty"`
	PredictionSide    string       `json:"predictionSide,omitempty"`
	RequestedQuantity float64      `json:"requestedQuantity,omitempty"`
	RequestedAmount   float64      `json:"requestedAmount,omitempty"`
	RequestedPrice    float64      `json:"requestedPrice,omitempty"`
	Status            string       `json:"status"`
	FilledQuantity    float64      `json:"filledQuantity,omitempty"`
	FilledAmount      float64      `json:"filledAmount,omitempty"`
	AveragePrice      float64      `json:"averagePrice,omitempty"`
	Fees              float64      `json:"fees,omitempty"`
	Payout            float64      `json:"payout,omitempty"`
}

type ComboTradingService interface {
	PreviewComboOrder(ctx context.Context, intent ComboOrderIntent) (*ProductRuleResult, error)
	PlaceComboOrder(ctx context.Context, intent ComboOrderIntent) (*ComboOrderResult, error)
	CancelComboOrder(ctx context.Context, query ReadQuery, brokerOrderID string) error
}

type EventContractTradingService interface {
	PreviewEventOrder(ctx context.Context, intent ComboOrderIntent) (*ProductRuleResult, error)
	PlaceEventOrder(ctx context.Context, intent ComboOrderIntent) (*ComboOrderResult, error)
	CancelEventOrder(ctx context.Context, query ReadQuery, brokerOrderID string) error
}
