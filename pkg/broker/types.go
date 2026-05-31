package broker

import "time"

// ReadQuery selects a specific broker trading account context for read/write operations.
type ReadQuery struct {
	BrokerID             string `json:"brokerId,omitempty"`
	AccountID            string `json:"accountId"`
	TradingEnvironment   string `json:"tradingEnvironment"` // "SIMULATE" | "REAL"
	Market               string `json:"market"`             // "HK" | "US" | "SH" | "SZ" | ...
}

// Account represents a discovered trading account.
type Account struct {
	ID                   string   `json:"accountId"`
	BrokerID             string   `json:"brokerId"`
	TradingEnvironment   string   `json:"tradingEnvironment"`
	Market               string   `json:"market,omitempty"`
	AccountType          string   `json:"accountType,omitempty"`
	AccountRole          *string  `json:"accountRole,omitempty"`
	SecurityFirm         *string  `json:"securityFirm,omitempty"`
	MarketAuthorities    []string `json:"marketAuthorities,omitempty"`
	SimulatedAccountType *string  `json:"simulatedAccountType,omitempty"`
}

// --- Funds ---

type FundsSnapshot struct {
	AccountID               string                    `json:"accountId"`
	TradingEnvironment      string                    `json:"tradingEnvironment"`
	Market                  string                    `json:"market"`
	AccountType             string                    `json:"accountType,omitempty"`
	Currency                *string                   `json:"currency,omitempty"`
	TotalAssets             *float64                  `json:"totalAssets,omitempty"`
	SecuritiesAssets        *float64                  `json:"securitiesAssets,omitempty"`
	FundAssets              *float64                  `json:"fundAssets,omitempty"`
	BondAssets              *float64                  `json:"bondAssets,omitempty"`
	Cash                    *float64                  `json:"cash,omitempty"`
	MarketValue             *float64                  `json:"marketValue,omitempty"`
	LongMarketValue         *float64                  `json:"longMarketValue,omitempty"`
	ShortMarketValue        *float64                  `json:"shortMarketValue,omitempty"`
	PurchasingPower         *float64                  `json:"purchasingPower,omitempty"`
	ShortSellingPower       *float64                  `json:"shortSellingPower,omitempty"`
	NetCashPower            *float64                  `json:"netCashPower,omitempty"`
	AvailableWithdrawalCash *float64                  `json:"availableWithdrawalCash,omitempty"`
	MaxWithdrawal           *float64                  `json:"maxWithdrawal,omitempty"`
	AvailableFunds          *float64                  `json:"availableFunds,omitempty"`
	FrozenCash              *float64                  `json:"frozenCash,omitempty"`
	PendingAsset            *float64                  `json:"pendingAsset,omitempty"`
	UnrealizedPnl           *float64                  `json:"unrealizedPnl,omitempty"`
	RealizedPnl             *float64                  `json:"realizedPnl,omitempty"`
	InitialMargin           *float64                  `json:"initialMargin,omitempty"`
	MaintenanceMargin       *float64                  `json:"maintenanceMargin,omitempty"`
	MarginCallMargin        *float64                  `json:"marginCallMargin,omitempty"`
	RiskStatus              *string                   `json:"riskStatus,omitempty"`
	CurrencyBalances        []CurrencyBalanceSnapshot `json:"currencyBalances,omitempty"`
	MarketAssets            []MarketAssetSnapshot     `json:"marketAssets,omitempty"`
}

type CurrencyBalanceSnapshot struct {
	AccountID               string  `json:"accountId"`
	TradingEnvironment      string  `json:"tradingEnvironment"`
	Currency                string  `json:"currency"`
	Cash                    *float64 `json:"cash,omitempty"`
	AvailableWithdrawalCash *float64 `json:"availableWithdrawalCash,omitempty"`
	NetCashPower            *float64 `json:"netCashPower,omitempty"`
}

type MarketAssetSnapshot struct {
	AccountID          string  `json:"accountId"`
	TradingEnvironment string  `json:"tradingEnvironment"`
	Market             string  `json:"market"`
	Assets             *float64 `json:"assets,omitempty"`
}

// --- Positions ---

type PositionSnapshot struct {
	AccountID          string   `json:"accountId"`
	TradingEnvironment string   `json:"tradingEnvironment"`
	Market             string   `json:"market"`
	Symbol             string   `json:"symbol"`
	SymbolName         *string  `json:"symbolName,omitempty"`
	Quantity           float64  `json:"quantity"`
	SellableQuantity   float64  `json:"sellableQuantity"`
	LastPrice          float64  `json:"lastPrice"`
	CostPrice          *float64 `json:"costPrice,omitempty"`
	AverageCostPrice   *float64 `json:"averageCostPrice,omitempty"`
	MarketValue        float64  `json:"marketValue"`
	UnrealizedPnl      *float64 `json:"unrealizedPnl,omitempty"`
	RealizedPnl        *float64 `json:"realizedPnl,omitempty"`
	PnlRatio           *float64 `json:"pnlRatio,omitempty"`
	Currency           *string  `json:"currency,omitempty"`
}

// --- Orders ---

type OrderSnapshot struct {
	AccountID          string   `json:"accountId"`
	TradingEnvironment string   `json:"tradingEnvironment"`
	Market             string   `json:"market"`
	BrokerOrderID      string   `json:"brokerOrderId"`
	BrokerOrderIDEx    *string  `json:"brokerOrderIdEx,omitempty"`
	Symbol             string   `json:"symbol"`
	SymbolName         *string  `json:"symbolName,omitempty"`
	Side               string   `json:"side"`
	OrderType          string   `json:"orderType"`
	Status             string   `json:"status"`
	Quantity           float64  `json:"quantity"`
	FilledQuantity     *float64 `json:"filledQuantity,omitempty"`
	Price              *float64 `json:"price,omitempty"`
	FilledAveragePrice *float64 `json:"filledAveragePrice,omitempty"`
	SubmittedAt        string   `json:"submittedAt,omitempty"`
	UpdatedAt          string   `json:"updatedAt,omitempty"`
	Remark             *string  `json:"remark,omitempty"`
	LastError          *string  `json:"lastError,omitempty"`
	TimeInForce        *string  `json:"timeInForce,omitempty"`
	Currency           *string  `json:"currency,omitempty"`
}

type OrderHistoryQuery struct {
	ReadQuery
	Symbol    string   `json:"symbol,omitempty"`
	StartTime string   `json:"startTime,omitempty"`
	EndTime   string   `json:"endTime,omitempty"`
	Statuses  []string `json:"statuses,omitempty"`
}

// --- Order Fills ---

type OrderFillSnapshot struct {
	AccountID          string   `json:"accountId"`
	TradingEnvironment string   `json:"tradingEnvironment"`
	Market             string   `json:"market"`
	BrokerOrderID      string   `json:"brokerOrderId"`
	BrokerOrderIDEx    *string  `json:"brokerOrderIdEx,omitempty"`
	BrokerFillID       string   `json:"brokerFillId"`
	BrokerFillIDEx     *string  `json:"brokerFillIdEx,omitempty"`
	Symbol             string   `json:"symbol"`
	SymbolName         *string  `json:"symbolName,omitempty"`
	Side               string   `json:"side"`
	FilledQuantity     float64  `json:"filledQuantity"`
	FillPrice          *float64 `json:"fillPrice,omitempty"`
	FilledAt           string   `json:"filledAt,omitempty"`
	Status             *string  `json:"status,omitempty"`
}

type OrderFillQuery struct {
	ReadQuery
	Symbol    string `json:"symbol,omitempty"`
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`
}

type OrderFillHistoryQuery struct {
	ReadQuery
	Symbol    string `json:"symbol,omitempty"`
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`
}

// --- Order Fees ---

type OrderFeeQuery struct {
	ReadQuery
	OrderIDExList []string `json:"orderIdExList,omitempty"`
}

type OrderFeeItemSnapshot struct {
	Title string  `json:"title"`
	Value float64 `json:"value"`
}

type OrderFeeSnapshot struct {
	AccountID          string                `json:"accountId"`
	TradingEnvironment string                `json:"tradingEnvironment"`
	Market             string                `json:"market"`
	BrokerOrderIDEx    string                `json:"brokerOrderIdEx"`
	FeeAmount          *float64              `json:"feeAmount,omitempty"`
	FeeItems           []OrderFeeItemSnapshot `json:"feeItems,omitempty"`
}

// --- Margin Ratios ---

type MarginRatioQuery struct {
	ReadQuery
	Symbols []string `json:"symbols,omitempty"`
}

type MarginRatioSnapshot struct {
	AccountID               string   `json:"accountId"`
	TradingEnvironment      string   `json:"tradingEnvironment"`
	Market                  string   `json:"market"`
	Symbol                  string   `json:"symbol"`
	IsLongPermit            *bool    `json:"isLongPermit,omitempty"`
	IsShortPermit           *bool    `json:"isShortPermit,omitempty"`
	ShortPoolRemain         *float64 `json:"shortPoolRemain,omitempty"`
	ShortFeeRate            *float64 `json:"shortFeeRate,omitempty"`
	AlertLongRatio          *float64 `json:"alertLongRatio,omitempty"`
	AlertShortRatio         *float64 `json:"alertShortRatio,omitempty"`
	InitialMarginLongRatio  *float64 `json:"initialMarginLongRatio,omitempty"`
	InitialMarginShortRatio *float64 `json:"initialMarginShortRatio,omitempty"`
	MarginCallLongRatio     *float64 `json:"marginCallLongRatio,omitempty"`
	MarginCallShortRatio    *float64 `json:"marginCallShortRatio,omitempty"`
	MaintenanceLongRatio    *float64 `json:"maintenanceLongRatio,omitempty"`
	MaintenanceShortRatio   *float64 `json:"maintenanceShortRatio,omitempty"`
}

// --- Cash Flows ---

type CashFlowQuery struct {
	ReadQuery
	ClearingDate string `json:"clearingDate,omitempty"`
	Direction    string `json:"direction,omitempty"`
}

type CashFlowSnapshot struct {
	AccountID          string   `json:"accountId"`
	TradingEnvironment string   `json:"tradingEnvironment"`
	Market             string   `json:"market"`
	CashFlowID         *string  `json:"cashFlowId,omitempty"`
	ClearingDate       *string  `json:"clearingDate,omitempty"`
	SettlementDate     *string  `json:"settlementDate,omitempty"`
	Currency           *string  `json:"currency,omitempty"`
	CashFlowType       *string  `json:"cashFlowType,omitempty"`
	CashFlowDirection  *string  `json:"cashFlowDirection,omitempty"`
	CashFlowAmount     *float64 `json:"cashFlowAmount,omitempty"`
	CashFlowRemark     *string  `json:"cashFlowRemark,omitempty"`
}

// --- Max Trade Quantity ---

type MaxTradeQuantityQuery struct {
	ReadQuery
	Symbol             string   `json:"symbol"`
	OrderType          string   `json:"orderType"`
	Price              float64  `json:"price"`
	OrderIDEx          string   `json:"orderIdEx,omitempty"`
	AdjustSideAndLimit *float64 `json:"adjustSideAndLimit,omitempty"`
	Session            *string  `json:"session,omitempty"`
	PositionID         *uint64  `json:"positionId,omitempty"`
}

type MaxTradeQuantitySnapshot struct {
	AccountID           string   `json:"accountId"`
	TradingEnvironment  string   `json:"tradingEnvironment"`
	Market              string   `json:"market"`
	Symbol              string   `json:"symbol"`
	OrderType           string   `json:"orderType"`
	Price               float64  `json:"price"`
	MaxCashBuy          float64  `json:"maxCashBuy"`
	MaxCashAndMarginBuy *float64 `json:"maxCashAndMarginBuy,omitempty"`
	MaxPositionSell     float64  `json:"maxPositionSell"`
	MaxSellShort        *float64 `json:"maxSellShort,omitempty"`
	MaxBuyBack          *float64 `json:"maxBuyBack,omitempty"`
	// Broker-specific extra fields. Populated by brokers that support them.
	LongRequiredIM  *float64 `json:"longRequiredIm,omitempty"`
	ShortRequiredIM *float64 `json:"shortRequiredIm,omitempty"`
	Session         *string  `json:"session,omitempty"`
}

// --- Trading (Write) ---

type PlaceOrderQuery struct {
	ReadQuery
	Symbol         string   `json:"symbol"`
	Side           string   `json:"side"`
	OrderType      string   `json:"orderType"`
	Price          *float64 `json:"price,omitempty"`
	Quantity       float64  `json:"quantity"`
	TimeInForce    *string  `json:"timeInForce,omitempty"`
	ClientOrderID  string   `json:"clientOrderId,omitempty"`
	Remark         *string  `json:"remark,omitempty"`
	Session        *string  `json:"session,omitempty"`
	FillOutsideRTH *bool    `json:"fillOutsideRTH,omitempty"`
}

type PlaceOrderResult struct {
	AccountID          string  `json:"accountId"`
	TradingEnvironment string  `json:"tradingEnvironment"`
	Market             string  `json:"market"`
	BrokerOrderID      string  `json:"brokerOrderId"`
	BrokerOrderIDEx    *string `json:"brokerOrderIdEx,omitempty"`
	Status             string  `json:"status"`
}

type CancelOrder struct {
	OrderID       uint64
	BrokerOrderID string
	Symbol        string
}

// --- Push Updates ---

type OrderUpdateEvent struct {
	Type    string      // "order" | "fill"
	Order   *OrderSnapshot
	Fill    *OrderFillSnapshot
	ReceivedAt time.Time
}
