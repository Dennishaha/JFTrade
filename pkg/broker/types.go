package broker

import "time"

// ReadQuery selects a specific broker trading account context for read/write operations.
type ReadQuery struct {
	BrokerID           string `json:"brokerId,omitempty"`
	AccountID          string `json:"accountId"`
	TradingEnvironment string `json:"tradingEnvironment"` // "SIMULATE" | "REAL"
	Market             string `json:"market"`             // "HK" | "US" | "SH" | "SZ" | ...
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
	AccountID               string   `json:"accountId"`
	TradingEnvironment      string   `json:"tradingEnvironment"`
	Market                  string   `json:"market"`
	AccountType             string   `json:"accountType,omitempty"`
	Currency                *string  `json:"currency,omitempty"`
	TotalAssets             *float64 `json:"totalAssets,omitempty"`
	SecuritiesAssets        *float64 `json:"securitiesAssets,omitempty"`
	FundAssets              *float64 `json:"fundAssets,omitempty"`
	BondAssets              *float64 `json:"bondAssets,omitempty"`
	Cash                    *float64 `json:"cash,omitempty"`
	MarketValue             *float64 `json:"marketValue,omitempty"`
	LongMarketValue         *float64 `json:"longMarketValue,omitempty"`
	ShortMarketValue        *float64 `json:"shortMarketValue,omitempty"`
	PurchasingPower         *float64 `json:"purchasingPower,omitempty"`
	ShortSellingPower       *float64 `json:"shortSellingPower,omitempty"`
	NetCashPower            *float64 `json:"netCashPower,omitempty"`
	AvailableWithdrawalCash *float64 `json:"availableWithdrawalCash,omitempty"`
	MaxWithdrawal           *float64 `json:"maxWithdrawal,omitempty"`
	AvailableFunds          *float64 `json:"availableFunds,omitempty"`
	FrozenCash              *float64 `json:"frozenCash,omitempty"`
	PendingAsset            *float64 `json:"pendingAsset,omitempty"`
	UnrealizedPnl           *float64 `json:"unrealizedPnl,omitempty"`
	RealizedPnl             *float64 `json:"realizedPnl,omitempty"`
	InitialMargin           *float64 `json:"initialMargin,omitempty"`
	MaintenanceMargin       *float64 `json:"maintenanceMargin,omitempty"`
	MarginCallMargin        *float64 `json:"marginCallMargin,omitempty"`
	RiskStatus              *string  `json:"riskStatus,omitempty"`
	// Margin & Financing 融资融券
	DebtCash         *float64                  `json:"debtCash,omitempty"`
	IsPDT            *bool                     `json:"isPdt,omitempty"`
	PDTSeq           *string                   `json:"pdtSeq,omitempty"`
	BeginningDTBP    *float64                  `json:"beginningDTBP,omitempty"`
	RemainingDTBP    *float64                  `json:"remainingDTBP,omitempty"`
	DTCallAmount     *float64                  `json:"dtCallAmount,omitempty"`
	DTStatus         *string                   `json:"dtStatus,omitempty"`
	ExposureLevel    *string                   `json:"exposureLevel,omitempty"`
	ExposureLimit    *float64                  `json:"exposureLimit,omitempty"`
	UsedLimit        *float64                  `json:"usedLimit,omitempty"`
	RemainingLimit   *float64                  `json:"remainingLimit,omitempty"`
	CurrencyBalances []CurrencyBalanceSnapshot `json:"currencyBalances,omitempty"`
	MarketAssets     []MarketAssetSnapshot     `json:"marketAssets,omitempty"`
}

type CurrencyBalanceSnapshot struct {
	AccountID               string   `json:"accountId"`
	TradingEnvironment      string   `json:"tradingEnvironment"`
	Currency                string   `json:"currency"`
	Cash                    *float64 `json:"cash,omitempty"`
	AvailableWithdrawalCash *float64 `json:"availableWithdrawalCash,omitempty"`
	NetCashPower            *float64 `json:"netCashPower,omitempty"`
}

type MarketAssetSnapshot struct {
	AccountID          string   `json:"accountId"`
	TradingEnvironment string   `json:"tradingEnvironment"`
	Market             string   `json:"market"`
	Assets             *float64 `json:"assets,omitempty"`
}

// --- Positions ---

type PositionSnapshot struct {
	AccountID          string        `json:"accountId"`
	TradingEnvironment string        `json:"tradingEnvironment"`
	Market             string        `json:"market"`
	Symbol             string        `json:"symbol"`
	SymbolName         *string       `json:"symbolName,omitempty"`
	ProductClass       ProductClass  `json:"productClass,omitempty"`
	MarketSegment      MarketSegment `json:"marketSegment,omitempty"`
	Quantity           float64       `json:"quantity"`
	SellableQuantity   float64       `json:"sellableQuantity"`
	LastPrice          float64       `json:"lastPrice"`
	CostPrice          *float64      `json:"costPrice,omitempty"`
	AverageCostPrice   *float64      `json:"averageCostPrice,omitempty"`
	MarketValue        float64       `json:"marketValue"`
	UnrealizedPnl      *float64      `json:"unrealizedPnl,omitempty"`
	RealizedPnl        *float64      `json:"realizedPnl,omitempty"`
	PnlRatio           *float64      `json:"pnlRatio,omitempty"`
	Currency           *string       `json:"currency,omitempty"`
	ComboID            *uint64       `json:"comboId,omitempty"`
	StrategyType       *string       `json:"strategyType,omitempty"`
	PositionType       *string       `json:"positionType,omitempty"`
	PayoutIfWin        *float64      `json:"payoutIfWin,omitempty"`
}

// --- Orders ---

type OrderSnapshot struct {
	AccountID          string             `json:"accountId"`
	TradingEnvironment string             `json:"tradingEnvironment"`
	Market             string             `json:"market"`
	OrderKind          OrderKind          `json:"orderKind,omitempty"`
	ProductClass       ProductClass       `json:"productClass,omitempty"`
	QuantityMode       QuantityMode       `json:"quantityMode,omitempty"`
	BrokerOrderID      string             `json:"brokerOrderId"`
	BrokerOrderIDEx    *string            `json:"brokerOrderIdEx,omitempty"`
	Symbol             string             `json:"symbol"`
	SymbolName         *string            `json:"symbolName,omitempty"`
	Side               string             `json:"side"`
	OrderType          string             `json:"orderType"`
	Status             string             `json:"status"`
	Quantity           float64            `json:"quantity"`
	Amount             *float64           `json:"amount,omitempty"`
	Legs               []OrderLegSnapshot `json:"legs,omitempty"`
	FilledQuantity     *float64           `json:"filledQuantity,omitempty"`
	Price              *float64           `json:"price,omitempty"`
	FilledAveragePrice *float64           `json:"filledAveragePrice,omitempty"`
	SubmittedAt        string             `json:"submittedAt,omitempty"`
	UpdatedAt          string             `json:"updatedAt,omitempty"`
	Remark             *string            `json:"remark,omitempty"`
	LastError          *string            `json:"lastError,omitempty"`
	TimeInForce        *string            `json:"timeInForce,omitempty"`
	Currency           *string            `json:"currency,omitempty"`
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
	Payout             *float64 `json:"payout,omitempty"`
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
	AccountID          string                 `json:"accountId"`
	TradingEnvironment string                 `json:"tradingEnvironment"`
	Market             string                 `json:"market"`
	BrokerOrderIDEx    string                 `json:"brokerOrderIdEx"`
	FeeAmount          *float64               `json:"feeAmount,omitempty"`
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
	Symbol         string       `json:"symbol"`
	ProductClass   ProductClass `json:"productClass,omitempty"`
	QuantityMode   QuantityMode `json:"quantityMode,omitempty"`
	Side           string       `json:"side"`
	OrderType      string       `json:"orderType"`
	Price          *float64     `json:"price,omitempty"`
	StopPrice      *float64     `json:"stopPrice,omitempty"`
	Quantity       float64      `json:"quantity"`
	Amount         *float64     `json:"amount,omitempty"`
	PredictionSide string       `json:"predictionSide,omitempty"`
	TimeInForce    *string      `json:"timeInForce,omitempty"`
	ClientOrderID  string       `json:"clientOrderId,omitempty"`
	Remark         *string      `json:"remark,omitempty"`
	Session        *string      `json:"session,omitempty"`
	FillOutsideRTH *bool        `json:"fillOutsideRTH,omitempty"`
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
	Type       string // "order" | "fill"
	Order      *OrderSnapshot
	Fill       *OrderFillSnapshot
	ReceivedAt time.Time
}

// --- Quote ---

type QuoteQuery struct {
	ReadQuery
	Symbols []string `json:"symbols"`
}

type QuoteSnapshot struct {
	AccountID  string      `json:"accountId"`
	Symbol     string      `json:"symbol"`
	SymbolName *string     `json:"symbolName,omitempty"`
	LastPrice  float64     `json:"lastPrice"`
	OpenPrice  *float64    `json:"openPrice,omitempty"`
	HighPrice  *float64    `json:"highPrice,omitempty"`
	LowPrice   *float64    `json:"lowPrice,omitempty"`
	LastClose  *float64    `json:"lastClose,omitempty"`
	Volume     float64     `json:"volume"`
	Turnover   *float64    `json:"turnover,omitempty"`
	QuoteAt    string      `json:"quoteAt,omitempty"`
	Quotes     []QuoteItem `json:"quotes,omitempty"`
}

type QuoteItem struct {
	Symbol     string   `json:"symbol"`
	SymbolName *string  `json:"symbolName,omitempty"`
	LastPrice  float64  `json:"lastPrice"`
	OpenPrice  *float64 `json:"openPrice,omitempty"`
	HighPrice  *float64 `json:"highPrice,omitempty"`
	LowPrice   *float64 `json:"lowPrice,omitempty"`
	Volume     float64  `json:"volume"`
	Turnover   *float64 `json:"turnover,omitempty"`
}

// --- K-Line ---

type KLineQuery struct {
	ReadQuery
	Symbol   string `json:"symbol"`
	Period   string `json:"period"` // 1m, 5m, 15m, 30m, 60m, 1d, 1w, 1M
	FromTime string `json:"fromTime,omitempty"`
	ToTime   string `json:"toTime,omitempty"`
	Limit    int32  `json:"limit,omitempty"`
}

type KLineSnapshot struct {
	AccountID string      `json:"accountId"`
	Symbol    string      `json:"symbol"`
	Period    string      `json:"period"`
	KLines    []KLineItem `json:"klines"`
}

type KLineItem struct {
	Time       string   `json:"time"`
	Open       *float64 `json:"open,omitempty"`
	Close      *float64 `json:"close,omitempty"`
	High       *float64 `json:"high,omitempty"`
	Low        *float64 `json:"low,omitempty"`
	Volume     *float64 `json:"volume,omitempty"`
	Turnover   *float64 `json:"turnover,omitempty"`
	ChangeRate *float64 `json:"changeRate,omitempty"`
}

// --- Security Info ---

type SecurityInfoQuery struct {
	ReadQuery
	Symbols []string `json:"symbols"`
}

type SecurityInfoSnapshot struct {
	AccountID  string             `json:"accountId"`
	Securities []SecurityInfoItem `json:"securities"`
}

type SecurityInfoItem struct {
	Symbol       string  `json:"symbol"`
	Name         *string `json:"name,omitempty"`
	SecurityType *string `json:"securityType,omitempty"`
	LotSize      *int32  `json:"lotSize,omitempty"`
	ListTime     *string `json:"listTime,omitempty"`
	IsDelisted   *bool   `json:"isDelisted,omitempty"`
}

// --- Security Search ---

type SecuritySearchQuery struct {
	ReadQuery
	Keyword string `json:"keyword"`
	Limit   int32  `json:"limit,omitempty"`
}

type SecuritySearchSnapshot struct {
	AccountID string               `json:"accountId"`
	Entries   []SecuritySearchItem `json:"entries"`
}

type SecuritySearchItem struct {
	Market       string `json:"market"`
	Symbol       string `json:"symbol"`
	Name         string `json:"name,omitempty"`
	SecurityType string `json:"securityType,omitempty"`
	IsWatched    bool   `json:"isWatched,omitempty"`
}

// --- Market Rules ---

type MarketRuleQuery struct {
	ReadQuery
	Symbols []string `json:"symbols"`
}

type MarketRuleSnapshot struct {
	AccountID string           `json:"accountId"`
	Rules     []MarketRuleItem `json:"rules"`
	Warnings  []string         `json:"warnings,omitempty"`
}

type MarketRuleItem struct {
	Symbol      string   `json:"symbol"`
	LotSize     *int32   `json:"lotSize,omitempty"`
	MinQuantity *float64 `json:"minQuantity,omitempty"`
	StepSize    *float64 `json:"stepSize,omitempty"`
}

// --- Security Snapshot ---

type SecuritySnapshotQuery struct {
	ReadQuery
	Symbols []string `json:"symbols"`
}

type SecuritySnapshotResult struct {
	AccountID string                 `json:"accountId"`
	Snapshots []SecuritySnapshotItem `json:"snapshots"`
}

type SecuritySnapshotItem struct {
	Symbol        string                   `json:"symbol"`
	Name          *string                  `json:"name,omitempty"`
	SecurityType  *string                  `json:"securityType,omitempty"`
	ProductClass  ProductClass             `json:"productClass,omitempty"`
	MarketSegment MarketSegment            `json:"marketSegment,omitempty"`
	IsSuspended   *bool                    `json:"isSuspended,omitempty"`
	LastPrice     *float64                 `json:"lastPrice,omitempty"`
	BidPrice      *float64                 `json:"bidPrice,omitempty"`
	AskPrice      *float64                 `json:"askPrice,omitempty"`
	PreviousClose *float64                 `json:"previousClose,omitempty"`
	OpenPrice     *float64                 `json:"openPrice,omitempty"`
	HighPrice     *float64                 `json:"highPrice,omitempty"`
	LowPrice      *float64                 `json:"lowPrice,omitempty"`
	Volume        *float64                 `json:"volume,omitempty"`
	Turnover      *float64                 `json:"turnover,omitempty"`
	PERate        *float64                 `json:"peRate,omitempty"`
	PBRate        *float64                 `json:"pbRate,omitempty"`
	LotSize       *int32                   `json:"lotSize,omitempty"`
	UpdateTime    *string                  `json:"updateTime,omitempty"`
	ObservedAt    time.Time                `json:"observedAt"`
	Session       *string                  `json:"session,omitempty"`
	PreMarket     *ExtendedSessionSnapshot `json:"preMarket,omitempty"`
	AfterMarket   *ExtendedSessionSnapshot `json:"afterMarket,omitempty"`
	Overnight     *ExtendedSessionSnapshot `json:"overnight,omitempty"`
	Option        *OptionSnapshotData      `json:"option,omitempty"`
	Warrant       *WarrantSnapshotData     `json:"warrant,omitempty"`
	Future        *FutureSnapshotData      `json:"future,omitempty"`
	Fund          *FundSnapshotData        `json:"fund,omitempty"`
}

// OptionSnapshotData is the broker-neutral derivative payload returned by a
// non-streaming batch snapshot. It deliberately carries strings instead of
// broker enum numbers.
type OptionSnapshotData struct {
	OptionType           string   `json:"optionType,omitempty"`
	UnderlyingCode       string   `json:"underlyingCode,omitempty"`
	ExpiryDate           string   `json:"expiryDate,omitempty"`
	StrikePrice          float64  `json:"strikePrice"`
	ContractSize         float64  `json:"contractSize"`
	ContractMultiplier   *float64 `json:"contractMultiplier,omitempty"`
	OpenInterest         int32    `json:"openInterest"`
	NetOpenInterest      *int32   `json:"netOpenInterest,omitempty"`
	ImpliedVolatility    float64  `json:"impliedVolatility"`
	Premium              float64  `json:"premium"`
	Delta                float64  `json:"delta"`
	Gamma                float64  `json:"gamma"`
	Vega                 float64  `json:"vega"`
	Theta                float64  `json:"theta"`
	Rho                  float64  `json:"rho"`
	DaysToExpiry         *int32   `json:"daysToExpiry,omitempty"`
	ContractNominalValue *float64 `json:"contractNominalValue,omitempty"`
}

type WarrantSnapshotData struct {
	WarrantType        string   `json:"warrantType,omitempty"`
	UnderlyingCode     string   `json:"underlyingCode,omitempty"`
	IssuerCode         *string  `json:"issuerCode,omitempty"`
	MaturityDate       string   `json:"maturityDate,omitempty"`
	LastTradeDate      string   `json:"lastTradeDate,omitempty"`
	StrikePrice        float64  `json:"strikePrice"`
	RecoveryPrice      float64  `json:"recoveryPrice"`
	ConversionRate     float64  `json:"conversionRate"`
	StreetVolume       int64    `json:"streetVolume"`
	IssueVolume        int64    `json:"issueVolume"`
	StreetRate         float64  `json:"streetRate"`
	ImpliedVolatility  float64  `json:"impliedVolatility"`
	Premium            float64  `json:"premium"`
	Delta              float64  `json:"delta"`
	Leverage           *float64 `json:"leverage,omitempty"`
	BreakEvenPoint     *float64 `json:"breakEvenPoint,omitempty"`
	PriceRecoveryRatio *float64 `json:"priceRecoveryRatio,omitempty"`
}

type FutureSnapshotData struct {
	LastSettlementPrice float64  `json:"lastSettlementPrice"`
	OpenInterest        int32    `json:"openInterest"`
	OpenInterestChange  int32    `json:"openInterestChange"`
	LastTradeDate       string   `json:"lastTradeDate,omitempty"`
	LastTradeTimestamp  *float64 `json:"lastTradeTimestamp,omitempty"`
	IsMainContract      bool     `json:"isMainContract"`
}

type FundSnapshotData struct {
	DividendYield         float64 `json:"dividendYield"`
	AssetsUnderManagement float64 `json:"assetsUnderManagement"`
	OutstandingUnits      int64   `json:"outstandingUnits"`
	NetAssetValue         float64 `json:"netAssetValue"`
	Premium               float64 `json:"premium"`
	AssetClass            string  `json:"assetClass,omitempty"`
}

// ExtendedSessionSnapshot carries the optional pre-market, after-hours, or
// overnight block returned by a snapshot API.
type ExtendedSessionSnapshot struct {
	Price      *float64 `json:"price,omitempty"`
	HighPrice  *float64 `json:"highPrice,omitempty"`
	LowPrice   *float64 `json:"lowPrice,omitempty"`
	Volume     *float64 `json:"volume,omitempty"`
	Turnover   *float64 `json:"turnover,omitempty"`
	Change     *float64 `json:"change,omitempty"`
	ChangeRate *float64 `json:"changeRate,omitempty"`
	Amplitude  *float64 `json:"amplitude,omitempty"`
}

// WatchlistGroup is a remote broker watchlist group discovered for import.
// Ambiguous is true when more than one remote group has the same normalized
// name and the broker API cannot address them independently.
type WatchlistGroup struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Ambiguous bool   `json:"ambiguous,omitempty"`
}

// WatchlistSecurity is a broker watchlist member normalized to JFTrade's
// canonical instrument ID. BrokerCode and BrokerSecurityID remain aliases only.
type WatchlistSecurity struct {
	InstrumentID     string  `json:"instrumentId"`
	Name             *string `json:"name,omitempty"`
	SecurityType     *string `json:"securityType,omitempty"`
	BrokerCode       *string `json:"brokerCode,omitempty"`
	BrokerSecurityID *string `json:"brokerSecurityId,omitempty"`
}

// --- Quote Subscribe ---

type QuoteSubscribeRequest struct {
	ReadQuery
	Symbols  []string `json:"symbols"`
	SubTypes []string `json:"subTypes,omitempty"`
}

// --- Order Book (Depth) ---

// OrderBookQuery selects a security and the number of depth levels to retrieve.
type OrderBookQuery struct {
	ReadQuery
	Symbol string `json:"symbol"`
	Num    int32  `json:"num,omitempty"` // number of bid/ask levels, default 10
}

// OrderBookSnapshot holds the bid and ask depth for a single security.
type OrderBookSnapshot struct {
	AccountID      string           `json:"accountId"`
	Symbol         string           `json:"symbol"`
	Name           *string          `json:"name,omitempty"`
	SvrRecvTimeBid *string          `json:"svrRecvTimeBid,omitempty"`
	SvrRecvTimeAsk *string          `json:"svrRecvTimeAsk,omitempty"`
	Bids           []OrderBookLevel `json:"bids"`
	Asks           []OrderBookLevel `json:"asks"`
}

// OrderBookLevel represents one price level in the order book.
type OrderBookLevel struct {
	Price      float64               `json:"price"`
	Volume     float64               `json:"volume"`
	OrderCount int32                 `json:"orderCount"`
	DetailList []OrderBookDetailItem `json:"detailList,omitempty"` // SF行情特有
}

// OrderBookDetailItem represents a single order within a price level.
type OrderBookDetailItem struct {
	OrderID int64   `json:"orderId"`
	Volume  float64 `json:"volume"`
}

// OrderBookSubscribeRequest is a subscribe request for real-time order book pushes.
type OrderBookSubscribeRequest struct {
	ReadQuery
	Symbols []string `json:"symbols"`
	Num     int32    `json:"num,omitempty"` // number of levels
}

// --- Unlock Trade ---

type UnlockTradeRequest struct {
	ReadQuery
	Unlock      bool   `json:"unlock"`
	PasswordMD5 string `json:"passwordMd5,omitempty"`
}
