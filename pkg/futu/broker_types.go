package futu

import "github.com/jftrade/jftrade-main/pkg/broker"

// BrokerReadQuery selects a specific Futu trading account context for read-side
// account, position, and order queries.
type BrokerReadQuery struct {
	AccountID          string
	TradingEnvironment string
	Market             string
}

// BrokerFundsSnapshot is a normalized funds payload exposed through the
// exchange boundary for compatibility routes.
type BrokerFundsSnapshot struct {
	AccountID               string
	TradingEnvironment      string
	Market                  string
	AccountType             string
	Currency                *string
	TotalAssets             *float64
	SecuritiesAssets        *float64
	FundAssets              *float64
	BondAssets              *float64
	Cash                    *float64
	MarketValue             *float64
	LongMarketValue         *float64
	ShortMarketValue        *float64
	PurchasingPower         *float64
	ShortSellingPower       *float64
	NetCashPower            *float64
	AvailableWithdrawalCash *float64
	MaxWithdrawal           *float64
	AvailableFunds          *float64
	FrozenCash              *float64
	PendingAsset            *float64
	UnrealizedPnl           *float64
	RealizedPnl             *float64
	InitialMargin           *float64
	MaintenanceMargin       *float64
	MarginCallMargin        *float64
	RiskStatus              *string
	// --- Margin & Financing 融资融券 ---
	DebtCash         *float64 // 计息金额
	IsPDT            *bool    // 是否PDT账户（美股日内交易限制）
	PDTSeq           *string  // 剩余日内交易次数
	BeginningDTBP    *float64 // 初始日内交易购买力
	RemainingDTBP    *float64 // 剩余日内交易购买力
	DTCallAmount     *float64 // 日内交易待缴金额
	DTStatus         *string  // 日内交易限制状态
	ExposureLevel    *string  // 持仓限额等级
	ExposureLimit    *float64 // 持仓限额
	UsedLimit        *float64 // 已用持仓限额
	RemainingLimit   *float64 // 剩余持仓限额
	CurrencyBalances []BrokerCurrencyBalanceSnapshot
	MarketAssets     []BrokerMarketAssetSnapshot
}

type BrokerCurrencyBalanceSnapshot struct {
	AccountID               string
	TradingEnvironment      string
	Currency                string
	Cash                    *float64
	AvailableWithdrawalCash *float64
	NetCashPower            *float64
}

type BrokerMarketAssetSnapshot struct {
	AccountID          string
	TradingEnvironment string
	Market             string
	Assets             *float64
}

type BrokerPositionSnapshot struct {
	AccountID          string
	TradingEnvironment string
	Market             string
	Symbol             string
	SymbolName         *string
	ProductClass       broker.ProductClass
	MarketSegment      broker.MarketSegment
	Quantity           float64
	SellableQuantity   float64
	LastPrice          float64
	CostPrice          *float64
	AverageCostPrice   *float64
	MarketValue        float64
	UnrealizedPnl      *float64
	RealizedPnl        *float64
	PnlRatio           *float64
	Currency           *string
	ComboID            *uint64
	StrategyType       *string
	PositionType       *string
	PayoutIfWin        *float64
}

type BrokerOrderSnapshot struct {
	AccountID          string
	TradingEnvironment string
	Market             string
	OrderKind          broker.OrderKind
	ProductClass       broker.ProductClass
	QuantityMode       broker.QuantityMode
	BrokerOrderID      string
	BrokerOrderIDEx    *string
	Symbol             string
	SymbolName         *string
	Side               string
	OrderType          string
	Status             string
	Quantity           float64
	Amount             *float64
	Legs               []broker.OrderLegSnapshot
	FilledQuantity     *float64
	Price              *float64
	FilledAveragePrice *float64
	SubmittedAt        string
	UpdatedAt          string
	Remark             *string
	LastError          *string
	TimeInForce        *string
	Currency           *string
}

type BrokerOrderHistoryQuery struct {
	BrokerReadQuery
	Symbol    string
	StartTime string
	EndTime   string
	Statuses  []string
}

type BrokerOrderFillHistoryQuery struct {
	BrokerReadQuery
	Symbol    string
	StartTime string
	EndTime   string
}

type BrokerOrderFillQuery struct {
	BrokerReadQuery
	Symbol    string
	StartTime string
	EndTime   string
}

type BrokerOrderFeeQuery struct {
	BrokerReadQuery
	OrderIDExList []string
}

type BrokerOrderFeeItemSnapshot struct {
	Title string
	Value float64
}

type BrokerOrderFeeSnapshot struct {
	AccountID          string
	TradingEnvironment string
	Market             string
	BrokerOrderIDEx    string
	FeeAmount          *float64
	FeeItems           []BrokerOrderFeeItemSnapshot
}

type BrokerMarginRatioQuery struct {
	BrokerReadQuery
	Symbols []string
}

type BrokerMarginRatioSnapshot struct {
	AccountID               string
	TradingEnvironment      string
	Market                  string
	Symbol                  string
	IsLongPermit            *bool
	IsShortPermit           *bool
	ShortPoolRemain         *float64
	ShortFeeRate            *float64
	AlertLongRatio          *float64
	AlertShortRatio         *float64
	InitialMarginLongRatio  *float64
	InitialMarginShortRatio *float64
	MarginCallLongRatio     *float64
	MarginCallShortRatio    *float64
	MaintenanceLongRatio    *float64
	MaintenanceShortRatio   *float64
}

type BrokerCashFlowQuery struct {
	BrokerReadQuery
	ClearingDate string
	Direction    string
}

type BrokerCashFlowSnapshot struct {
	AccountID          string
	TradingEnvironment string
	Market             string
	CashFlowID         *string
	ClearingDate       *string
	SettlementDate     *string
	Currency           *string
	CashFlowType       *string
	CashFlowDirection  *string
	CashFlowAmount     *float64
	CashFlowRemark     *string
}

type BrokerMaxTradeQuantityQuery struct {
	BrokerReadQuery
	Symbol             string
	OrderType          string
	Price              float64
	OrderIDEx          string
	AdjustSideAndLimit *float64
	Session            *string
	PositionID         *uint64
}

type BrokerMaxTradeQuantitySnapshot struct {
	AccountID           string
	TradingEnvironment  string
	Market              string
	Symbol              string
	OrderType           string
	Price               float64
	MaxCashBuy          float64
	MaxCashAndMarginBuy *float64
	MaxPositionSell     float64
	MaxSellShort        *float64
	MaxBuyBack          *float64
	LongRequiredIM      *float64
	ShortRequiredIM     *float64
	Session             *string
}

type resolvedTradeAccount struct {
	AccountID          string
	TradingEnvironment string
	Market             string
	AccountType        string
	protoAccountID     uint64
	protoTrdEnv        int32
	protoTrdMarket     int32
}
