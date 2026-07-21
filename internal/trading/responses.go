package trading

import (
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// BrokerReadFeatureCapability 描述单个券商读取能力的约束。
// 可选字段使用 omitempty，保持历史 descriptor 中按能力种类输出不同键集合的形状。
type BrokerReadFeatureCapability struct {
	SupportedEnvironments []string `json:"supportedEnvironments" binding:"required"`
	SupportsHistory       bool     `json:"supportsHistory,omitempty"`
	RequiresSymbols       bool     `json:"requiresSymbols,omitempty"`
	RequiresClearingDate  bool     `json:"requiresClearingDate,omitempty"`
	RequiresPrice         bool     `json:"requiresPrice,omitempty"`
	RequiresOrderIDEx     bool     `json:"requiresOrderIdEx,omitempty"`
	DefaultNum            int      `json:"defaultNum,omitempty"`
	MinNum                int      `json:"minNum,omitempty"`
	MaxNum                int      `json:"maxNum,omitempty"`
	NumPresets            []int32  `json:"numPresets,omitempty"`
	SupportsRealTimePush  bool     `json:"supportsRealTimePush,omitempty"`
}

// BrokerMarketCapability 描述券商在单个市场上的能力。
type BrokerMarketCapability struct {
	Market        string                                 `json:"market" binding:"required"`
	SupportsQuote bool                                   `json:"supportsQuote" binding:"required"`
	SupportsTrade bool                                   `json:"supportsTrade" binding:"required"`
	ReadFeatures  map[string]BrokerReadFeatureCapability `json:"readFeatures" binding:"required"`
}

// BrokerRuntimeDescriptor 是 runtime 响应使用的静态券商描述符。
type BrokerRuntimeDescriptor struct {
	ID           string                   `json:"id" binding:"required"`
	DisplayName  string                   `json:"displayName" binding:"required"`
	Environments []string                 `json:"environments" binding:"required"`
	Capabilities []BrokerMarketCapability `json:"capabilities" binding:"required"`
	Notes        []string                 `json:"notes" binding:"required"`
}

// BrokerRuntimeConnection 是 runtime 会话连接配置的公开视图。
type BrokerRuntimeConnection struct {
	Host                string `json:"host" binding:"required"`
	APIPort             int    `json:"apiPort" binding:"required"`
	WebSocketPort       int    `json:"websocketPort" binding:"required"`
	Port                int    `json:"port" binding:"required"`
	UseEncryption       bool   `json:"useEncryption" binding:"required"`
	MarketDataTransport string `json:"marketDataTransport" binding:"required"`
}

// BrokerRuntimeMarketState 是 OpenD 返回的单市场状态。
type BrokerRuntimeMarketState struct {
	Market string `json:"market" binding:"required"`
	State  string `json:"state" binding:"required"`
}

// BrokerRuntimeGlobalState 是可选的 OpenD 全局登录与市场状态。
type BrokerRuntimeGlobalState struct {
	QuoteLoggedIn bool                       `json:"quoteLoggedIn" binding:"required"`
	TradeLoggedIn bool                       `json:"tradeLoggedIn" binding:"required"`
	ServerVersion *string                    `json:"serverVersion" binding:"required" extensions:"x-nullable"`
	ProgramStatus *string                    `json:"programStatus" binding:"required" extensions:"x-nullable"`
	Timestamp     *string                    `json:"timestamp" binding:"required" extensions:"x-nullable"`
	Markets       []BrokerRuntimeMarketState `json:"markets" binding:"required"`
}

// BrokerRuntimeLiveClients 是 JFTrade 行情 WebSocket 客户端计数。
type BrokerRuntimeLiveClients struct {
	Connected int  `json:"connected" binding:"required"`
	Limit     int  `json:"limit" binding:"required"`
	AtLimit   bool `json:"atLimit" binding:"required"`
}

// BrokerRuntimeSession 是券商运行会话状态。
type BrokerRuntimeSession struct {
	BrokerID             string                    `json:"brokerId" binding:"required"`
	DisplayName          string                    `json:"displayName" binding:"required"`
	Connection           BrokerRuntimeConnection   `json:"connection" binding:"required"`
	Connectivity         string                    `json:"connectivity" binding:"required"`
	CheckedAt            string                    `json:"checkedAt" binding:"required"`
	LastError            *string                   `json:"lastError" binding:"required" extensions:"x-nullable"`
	GlobalState          *BrokerRuntimeGlobalState `json:"globalState" binding:"required" extensions:"x-nullable"`
	AccountsDiscovered   int                       `json:"accountsDiscovered" binding:"required"`
	LiveWebSocketClients BrokerRuntimeLiveClients  `json:"liveWebSocketClients" binding:"required"`
}

// BrokerRuntimeAccount 是 runtime 响应中的规范化券商账户。
type BrokerRuntimeAccount struct {
	AccountID            string   `json:"accountId" binding:"required"`
	TradingEnvironment   string   `json:"tradingEnvironment" binding:"required"`
	AccountType          string   `json:"accountType" binding:"required"`
	AccountRole          *string  `json:"accountRole" binding:"required" extensions:"x-nullable"`
	SecurityFirm         *string  `json:"securityFirm" binding:"required" extensions:"x-nullable"`
	MarketAuthorities    []string `json:"marketAuthorities" binding:"required"`
	SimulatedAccountType *string  `json:"simulatedAccountType" binding:"required" extensions:"x-nullable"`
}

// BrokerRuntimeResponse GET /brokers/{id}/runtime 响应。
type BrokerRuntimeResponse struct {
	Descriptor BrokerRuntimeDescriptor `json:"descriptor" binding:"required"`
	Session    BrokerRuntimeSession    `json:"session" binding:"required"`
	Accounts   []BrokerRuntimeAccount  `json:"accounts" binding:"required"`
}

func emptyBrokerRuntimeResponse() *BrokerRuntimeResponse {
	return &BrokerRuntimeResponse{
		Descriptor: BrokerRuntimeDescriptor{
			Environments: []string{}, Capabilities: []BrokerMarketCapability{}, Notes: []string{},
		},
		Accounts: []BrokerRuntimeAccount{},
	}
}

// BrokerReadStatus 是所有券商读取响应共享的连接状态字段。
// JSON 键与历史 map 响应保持一致：lastError 在无错误时序列化为 null。
type BrokerReadStatus struct {
	CheckedAt    string  `json:"checkedAt" binding:"required"`
	Connectivity string  `json:"connectivity" binding:"required"`
	LastError    *string `json:"lastError"`
}

func connectedReadStatus() BrokerReadStatus {
	return BrokerReadStatus{CheckedAt: now(), Connectivity: "connected"}
}

func readErrorStatus(err error) BrokerReadStatus {
	message := err.Error()
	return BrokerReadStatus{CheckedAt: now(), Connectivity: connectivity(err), LastError: &message}
}

// BrokerFundsSummary 券商资金汇总。字段集合与历史 map 响应一致：
// 指针字段为空时序列化为 null 而不是省略键。
type BrokerFundsSummary struct {
	AccountID               string   `json:"accountId" binding:"required"`
	TradingEnvironment      string   `json:"tradingEnvironment" binding:"required"`
	Market                  string   `json:"market" binding:"required"`
	Currency                *string  `json:"currency"`
	TotalAssets             *float64 `json:"totalAssets"`
	SecuritiesAssets        *float64 `json:"securitiesAssets"`
	FundAssets              *float64 `json:"fundAssets"`
	BondAssets              *float64 `json:"bondAssets"`
	Cash                    *float64 `json:"cash"`
	MarketValue             *float64 `json:"marketValue"`
	LongMarketValue         *float64 `json:"longMarketValue"`
	ShortMarketValue        *float64 `json:"shortMarketValue"`
	PurchasingPower         *float64 `json:"purchasingPower"`
	ShortSellingPower       *float64 `json:"shortSellingPower"`
	NetCashPower            *float64 `json:"netCashPower"`
	AvailableWithdrawalCash *float64 `json:"availableWithdrawalCash"`
	MaxWithdrawal           *float64 `json:"maxWithdrawal"`
	AvailableFunds          *float64 `json:"availableFunds"`
	FrozenCash              *float64 `json:"frozenCash"`
	PendingAsset            *float64 `json:"pendingAsset"`
	UnrealizedPnl           *float64 `json:"unrealizedPnl"`
	RealizedPnl             *float64 `json:"realizedPnl"`
	InitialMargin           *float64 `json:"initialMargin"`
	MaintenanceMargin       *float64 `json:"maintenanceMargin"`
	MarginCallMargin        *float64 `json:"marginCallMargin"`
	RiskStatus              *string  `json:"riskStatus"`
	DebtCash                *float64 `json:"debtCash"`
	IsPDT                   *bool    `json:"isPdt"`
	PDTSeq                  *string  `json:"pdtSeq"`
	BeginningDTBP           *float64 `json:"beginningDTBP"`
	RemainingDTBP           *float64 `json:"remainingDTBP"`
	DTCallAmount            *float64 `json:"dtCallAmount"`
	DTStatus                *string  `json:"dtStatus"`
	ExposureLevel           *string  `json:"exposureLevel"`
	ExposureLimit           *float64 `json:"exposureLimit"`
	UsedLimit               *float64 `json:"usedLimit"`
	RemainingLimit          *float64 `json:"remainingLimit"`
}

// BrokerCurrencyBalance 单币种资金余额。
type BrokerCurrencyBalance struct {
	AccountID               string   `json:"accountId" binding:"required"`
	TradingEnvironment      string   `json:"tradingEnvironment" binding:"required"`
	Currency                string   `json:"currency" binding:"required"`
	Cash                    *float64 `json:"cash"`
	AvailableWithdrawalCash *float64 `json:"availableWithdrawalCash"`
	NetCashPower            *float64 `json:"netCashPower"`
}

// BrokerMarketAsset 单市场资产。
type BrokerMarketAsset struct {
	AccountID          string   `json:"accountId" binding:"required"`
	TradingEnvironment string   `json:"tradingEnvironment" binding:"required"`
	Market             string   `json:"market" binding:"required"`
	Assets             *float64 `json:"assets"`
}

// BrokerFundsResponse GET /brokers/{id}/funds 响应。
type BrokerFundsResponse struct {
	BrokerReadStatus
	Summary          *BrokerFundsSummary     `json:"summary"`
	CurrencyBalances []BrokerCurrencyBalance `json:"currencyBalances" binding:"required"`
	MarketAssets     []BrokerMarketAsset     `json:"marketAssets" binding:"required"`
}

func fundsReadError(err error) *BrokerFundsResponse {
	return &BrokerFundsResponse{
		BrokerReadStatus: readErrorStatus(err),
		CurrencyBalances: []BrokerCurrencyBalance{},
		MarketAssets:     []BrokerMarketAsset{},
	}
}

// BrokerPosition 券商持仓条目。
type BrokerPosition struct {
	AccountID          string   `json:"accountId" binding:"required"`
	TradingEnvironment string   `json:"tradingEnvironment" binding:"required"`
	Market             string   `json:"market" binding:"required"`
	Symbol             string   `json:"symbol" binding:"required"`
	SymbolName         *string  `json:"symbolName"`
	Quantity           float64  `json:"quantity" binding:"required"`
	SellableQuantity   float64  `json:"sellableQuantity" binding:"required"`
	LastPrice          float64  `json:"lastPrice" binding:"required"`
	CostPrice          *float64 `json:"costPrice"`
	AverageCostPrice   *float64 `json:"averageCostPrice"`
	MarketValue        float64  `json:"marketValue" binding:"required"`
	UnrealizedPnl      *float64 `json:"unrealizedPnl"`
	RealizedPnl        *float64 `json:"realizedPnl"`
	PnlRatio           *float64 `json:"pnlRatio"`
	Currency           *string  `json:"currency"`
}

// BrokerPositionsResponse GET /brokers/{id}/positions 响应。
type BrokerPositionsResponse struct {
	BrokerReadStatus
	Positions []BrokerPosition `json:"positions" binding:"required"`
}

func positionsReadError(err error) *BrokerPositionsResponse {
	return &BrokerPositionsResponse{
		BrokerReadStatus: readErrorStatus(err),
		Positions:        []BrokerPosition{},
	}
}

// BrokerOrder 券商订单条目。
type BrokerOrder struct {
	AccountID          string   `json:"accountId" binding:"required"`
	TradingEnvironment string   `json:"tradingEnvironment" binding:"required"`
	Market             string   `json:"market" binding:"required"`
	BrokerOrderID      string   `json:"brokerOrderId" binding:"required"`
	BrokerOrderIDEx    *string  `json:"brokerOrderIdEx"`
	Symbol             string   `json:"symbol" binding:"required"`
	SymbolName         *string  `json:"symbolName"`
	Side               string   `json:"side" binding:"required"`
	OrderType          string   `json:"orderType" binding:"required"`
	Status             string   `json:"status" binding:"required"`
	Quantity           float64  `json:"quantity" binding:"required"`
	FilledQuantity     *float64 `json:"filledQuantity"`
	Price              *float64 `json:"price"`
	FilledAveragePrice *float64 `json:"filledAveragePrice"`
	SubmittedAt        string   `json:"submittedAt" binding:"required"`
	UpdatedAt          string   `json:"updatedAt" binding:"required"`
	Remark             *string  `json:"remark"`
	LastError          *string  `json:"lastError"`
	TimeInForce        *string  `json:"timeInForce"`
	Currency           *string  `json:"currency"`
}

// BrokerOrdersResponse GET /brokers/{id}/orders 响应。
type BrokerOrdersResponse struct {
	BrokerReadStatus
	Orders []BrokerOrder `json:"orders" binding:"required"`
}

func ordersReadError(err error) *BrokerOrdersResponse {
	return &BrokerOrdersResponse{
		BrokerReadStatus: readErrorStatus(err),
		Orders:           []BrokerOrder{},
	}
}

// BrokerFill 券商成交条目。
type BrokerFill struct {
	AccountID          string   `json:"accountId" binding:"required"`
	TradingEnvironment string   `json:"tradingEnvironment" binding:"required"`
	Market             string   `json:"market" binding:"required"`
	BrokerOrderID      string   `json:"brokerOrderId" binding:"required"`
	BrokerOrderIDEx    *string  `json:"brokerOrderIdEx"`
	BrokerFillID       string   `json:"brokerFillId" binding:"required"`
	BrokerFillIDEx     *string  `json:"brokerFillIdEx"`
	Symbol             string   `json:"symbol" binding:"required"`
	SymbolName         *string  `json:"symbolName"`
	Side               string   `json:"side" binding:"required"`
	FilledQuantity     float64  `json:"filledQuantity" binding:"required"`
	FillPrice          *float64 `json:"fillPrice"`
	FilledAt           string   `json:"filledAt" binding:"required"`
	Status             *string  `json:"status"`
}

// BrokerFillsResponse GET /brokers/{id}/fills 响应。
type BrokerFillsResponse struct {
	BrokerReadStatus
	Fills []BrokerFill `json:"fills" binding:"required"`
}

func fillsReadError(err error) *BrokerFillsResponse {
	return &BrokerFillsResponse{
		BrokerReadStatus: readErrorStatus(err),
		Fills:            []BrokerFill{},
	}
}

// BrokerCashFlowsResponse GET /brokers/{id}/cash-flows 响应。
type BrokerCashFlowsResponse struct {
	BrokerReadStatus
	CashFlows []broker.CashFlowSnapshot `json:"cashFlows" binding:"required"`
}

func cashFlowsReadError(err error) *BrokerCashFlowsResponse {
	return &BrokerCashFlowsResponse{
		BrokerReadStatus: readErrorStatus(err),
		CashFlows:        []broker.CashFlowSnapshot{},
	}
}

// BrokerOrderFeesResponse GET /brokers/{id}/order-fees 响应。
type BrokerOrderFeesResponse struct {
	BrokerReadStatus
	Fees []broker.OrderFeeSnapshot `json:"fees" binding:"required"`
}

func orderFeesReadError(err error) *BrokerOrderFeesResponse {
	return &BrokerOrderFeesResponse{
		BrokerReadStatus: readErrorStatus(err),
		Fees:             []broker.OrderFeeSnapshot{},
	}
}

// BrokerMarginRatiosResponse GET /brokers/{id}/margin-ratios 响应。
type BrokerMarginRatiosResponse struct {
	BrokerReadStatus
	MarginRatios []broker.MarginRatioSnapshot `json:"marginRatios" binding:"required"`
}

func marginRatiosReadError(err error) *BrokerMarginRatiosResponse {
	return &BrokerMarginRatiosResponse{
		BrokerReadStatus: readErrorStatus(err),
		MarginRatios:     []broker.MarginRatioSnapshot{},
	}
}

// BrokerMaxTradeQuantityResponse GET /brokers/{id}/max-trade-qtys 响应。
type BrokerMaxTradeQuantityResponse struct {
	BrokerReadStatus
	MaxTradeQuantity *broker.MaxTradeQuantitySnapshot `json:"maxTradeQuantity"`
}

func maxTradeQuantityReadError(err error) *BrokerMaxTradeQuantityResponse {
	return &BrokerMaxTradeQuantityResponse{BrokerReadStatus: readErrorStatus(err)}
}

// BrokerQuoteResponse GET /brokers/{id}/quote 响应。
// 历史错误响应使用 quotes 键（空数组）而不是 quote 键，两个键都保持可选以兼容两种形状。
type BrokerQuoteResponse struct {
	BrokerReadStatus
	Quote  *broker.QuoteSnapshot `json:"quote,omitempty"`
	Quotes any                   `json:"quotes,omitempty"`
}

func quoteReadError(err error) *BrokerQuoteResponse {
	return &BrokerQuoteResponse{
		BrokerReadStatus: readErrorStatus(err),
		Quotes:           []any{},
	}
}

// BrokerKLinesResponse GET /brokers/{id}/klines 响应。
// klines 键在成功时承载 *broker.KLineSnapshot 对象，失败时承载空数组，因此保持 any。
type BrokerKLinesResponse struct {
	BrokerReadStatus
	KLines any `json:"klines" binding:"required"`
}

func klinesReadError(err error) *BrokerKLinesResponse {
	return &BrokerKLinesResponse{
		BrokerReadStatus: readErrorStatus(err),
		KLines:           []any{},
	}
}

// BrokerSecuritiesResponse GET /brokers/{id}/securities 响应。
// securities 键在成功时承载 *broker.SecuritySnapshotResult 对象，失败时承载空数组，因此保持 any。
type BrokerSecuritiesResponse struct {
	BrokerReadStatus
	Securities any `json:"securities" binding:"required"`
}

func securitiesReadError(err error) *BrokerSecuritiesResponse {
	return &BrokerSecuritiesResponse{
		BrokerReadStatus: readErrorStatus(err),
		Securities:       []any{},
	}
}

// BrokerPlaceOrderResponse POST /brokers/{id}/orders 响应。
type BrokerPlaceOrderResponse struct {
	PlacedAt string                   `json:"placedAt" binding:"required"`
	Order    *broker.PlaceOrderResult `json:"order" binding:"required"`
}

// BrokerCancelOrdersResponse DELETE /brokers/{id}/orders 响应。
type BrokerCancelOrdersResponse struct {
	CancelledAt string `json:"cancelledAt" binding:"required"`
	Cancelled   int    `json:"cancelled" binding:"required"`
}

// BrokerUnlockTradeResponse POST /brokers/{id}/unlock 响应。
type BrokerUnlockTradeResponse struct {
	UnlockedAt string `json:"unlockedAt" binding:"required"`
	Unlocked   bool   `json:"unlocked" binding:"required"`
}

// PortfolioCashBalance portfolio 现金余额条目。
type PortfolioCashBalance struct {
	BrokerID           string  `json:"brokerId" binding:"required"`
	TradingEnvironment string  `json:"tradingEnvironment" binding:"required"`
	AccountID          string  `json:"accountId" binding:"required"`
	Currency           string  `json:"currency" binding:"required"`
	CashBalance        float64 `json:"cashBalance" binding:"required"`
	UpdatedAt          string  `json:"updatedAt" binding:"required"`
	CreatedAt          string  `json:"createdAt" binding:"required"`
}

// PortfolioCashBalancesResponse GET /portfolio/{id}/cash-balances 响应。
type PortfolioCashBalancesResponse struct {
	Balances []PortfolioCashBalance `json:"balances" binding:"required"`
}

// PortfolioPosition portfolio 持仓条目。
type PortfolioPosition struct {
	BrokerID           string  `json:"brokerId" binding:"required"`
	TradingEnvironment string  `json:"tradingEnvironment" binding:"required"`
	AccountID          string  `json:"accountId" binding:"required"`
	Market             string  `json:"market" binding:"required"`
	Symbol             string  `json:"symbol" binding:"required"`
	Quantity           float64 `json:"quantity" binding:"required"`
	AveragePrice       float64 `json:"averagePrice" binding:"required"`
	MarketValue        float64 `json:"marketValue" binding:"required"`
	UpdatedAt          string  `json:"updatedAt" binding:"required"`
	CreatedAt          string  `json:"createdAt" binding:"required"`
}

// PortfolioPositionsResponse GET /portfolio/{id}/positions 响应。
type PortfolioPositionsResponse struct {
	Positions []PortfolioPosition `json:"positions" binding:"required"`
}
