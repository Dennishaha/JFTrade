package futu

import (
	"context"
	"strings"
	"sync"
	"time"

	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
)

// futuAdapter implements broker.Broker by delegating to a Futu Exchange.
type futuAdapter struct {
	exchange  *Exchange
	watchlist *futuWatchlistReader

	capabilityMu                sync.RWMutex
	lastConnectStatus           *futuConnectCapabilityStatus
	lastQuoteRights             *futuQuoteCapabilityRights
	capabilityAccounts          []broker.Account
	capabilityAccountsExpiresAt time.Time

	predictionStreamMu        sync.Mutex
	predictionStreamClients   map[*opend.Client]struct{}
	predictionStreamListeners map[uint64]func(broker.PredictionMarketUpdate)
	predictionSubscriptions   map[string]broker.PredictionSubscription
	predictionStreamNextID    uint64
}

// NewBrokerAdapter wraps a Futu Exchange as a broker.Broker.
func NewBrokerAdapter(exchange *Exchange) broker.Broker {
	adapter := &futuAdapter{
		exchange:                  exchange,
		watchlist:                 newFutuWatchlistReader(exchange),
		predictionStreamClients:   make(map[*opend.Client]struct{}),
		predictionStreamListeners: make(map[uint64]func(broker.PredictionMarketUpdate)),
		predictionSubscriptions:   make(map[string]broker.PredictionSubscription),
	}
	if exchange != nil {
		exchange.OnSystemNotify(adapter.captureCapabilityNotification)
	}
	return adapter
}

func (a *futuAdapter) ID() string { return string(Name) }

func (a *futuAdapter) Descriptor() broker.Descriptor {
	return broker.Descriptor{
		ID:                string(Name),
		DisplayName:       "Futu OpenAPI via OpenD",
		SecurityFirm:      "Futu/Moomoo via OpenD",
		CapabilityVersion: broker.BuiltinCapabilityCatalog.Version,
		Environments:      []string{"SIMULATE", "REAL"},
		Capabilities: []broker.MarketCapability{
			futuMarketCapability("HK"),
			futuMarketCapability("US"),
			futuMarketCapability("SH"),
			futuMarketCapability("SZ"),
		},
		Notes: []string{
			"OpenD 10.9.6908 product and research protocols are broker-neutral above the adapter boundary.",
			"Prediction-market availability is determined at runtime and currently requires an eligible Moomoo US environment.",
			"Snapshot polling does not consume Basic Quote subscriptions; streaming is restricted to visible instruments.",
		},
	}
}

func futuMarketCapability(market string) broker.MarketCapability {
	return broker.MarketCapability{
		Market:        market,
		SupportsQuote: true,
		SupportsTrade: true,
		ReadFeatures: map[string]any{
			"funds":            map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}},
			"positions":        map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}},
			"orders":           map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}, "supportsHistory": true},
			"fills":            map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}, "supportsHistory": true},
			"cashFlows":        map[string]any{"supportedEnvironments": []string{"REAL"}, "requiresClearingDate": true},
			"orderFees":        map[string]any{"supportedEnvironments": []string{"REAL"}, "requiresOrderIdEx": true},
			"marginRatios":     map[string]any{"supportedEnvironments": []string{"REAL"}, "requiresSymbols": true},
			"maxTradeQuantity": map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}, "requiresPrice": true},
			"quote":            map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}, "requiresSymbols": true},
			"klines":           map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}, "requiresSymbol": true},
			"securityInfo":     map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}, "requiresSymbols": true},
			"securitySnapshot": map[string]any{"supportedEnvironments": []string{"SIMULATE", "REAL"}, "requiresSymbols": true},
			"unlockTrade":      map[string]any{"supportedEnvironments": []string{"REAL"}, "requiresPassword": true},
			"orderBook":        map[string]any{"defaultNum": 10, "minNum": 1, "maxNum": 50, "numPresets": []int32{5, 10, 20, 50}, "supportsRealTimePush": true},
		},
		Features: futuFeatureCapabilities(market),
	}
}

func futuFeatureCapabilities(market string) []broker.FeatureCapability {
	features := make([]broker.FeatureCapability, 0, len(broker.BuiltinCapabilityCatalog.Features))
	for _, definition := range broker.BuiltinCapabilityCatalog.Features {
		if !futuFeatureSupportsMarket(definition.ID, market) {
			continue
		}
		state := broker.CapabilityAvailable
		reasonCode := ""
		reason := ""
		requiresQuoteRight := definition.Access == broker.FeatureAccessRead
		requiresAccount := definition.Access != broker.FeatureAccessRead
		if strings.HasPrefix(string(definition.ID), "prediction.") {
			state = broker.CapabilityDegraded
			reasonCode = "RUNTIME_ELIGIBILITY_REQUIRED"
			reason = "Prediction markets require an eligible Moomoo US environment."
			requiresAccount = true
		}
		features = append(features, broker.FeatureCapability{
			ID:                 definition.ID,
			Markets:            []string{market},
			SupportedPeriods:   futuFeatureSupportedPeriods(definition.ID),
			ProductClasses:     futuFeatureProducts(definition.ID),
			MarketSegments:     futuFeatureSegments(definition.ID),
			Access:             definition.Access,
			State:              state,
			ReasonCode:         reasonCode,
			Reason:             reason,
			RequiresConnection: true,
			RequiresAccount:    requiresAccount,
			RequiresQuoteRight: requiresQuoteRight,
		})
	}
	return features
}

func futuFeatureSupportedPeriods(id broker.FeatureID) []string {
	if id != broker.FeatureMarketCandles {
		return nil
	}
	return futuCandlePeriods()
}

func futuFeatureSupportsMarket(id broker.FeatureID, market string) bool {
	value := string(id)
	if strings.HasPrefix(value, "prediction.") {
		return market == "US"
	}
	if id == broker.FeatureWarrants {
		return market == "HK"
	}
	if id == broker.FeatureFutures {
		return market == "HK" || market == "US"
	}
	if strings.HasPrefix(value, "derivatives.option") || strings.Contains(value, "option_event") ||
		strings.HasPrefix(value, "execution.combo") {
		return market == "HK" || market == "US"
	}
	return true
}

func futuFeatureProducts(id broker.FeatureID) []broker.ProductClass {
	value := string(id)
	switch {
	case strings.HasPrefix(value, "prediction."):
		return []broker.ProductClass{broker.ProductClassEventContract}
	case strings.HasPrefix(value, "execution.combo"):
		return []broker.ProductClass{broker.ProductClassOption, broker.ProductClassEventContract}
	case strings.HasPrefix(value, "execution.order"), id == broker.FeatureExecutionBuyingPower:
		return []broker.ProductClass{
			broker.ProductClassEquity, broker.ProductClassFund, broker.ProductClassOption,
			broker.ProductClassWarrant, broker.ProductClassCBBC, broker.ProductClassFuture,
			broker.ProductClassEventContract,
		}
	case strings.Contains(value, "option"):
		return []broker.ProductClass{broker.ProductClassOption}
	case id == broker.FeatureWarrants:
		return []broker.ProductClass{broker.ProductClassWarrant, broker.ProductClassCBBC}
	case id == broker.FeatureFutures:
		return []broker.ProductClass{broker.ProductClassFuture}
	default:
		return []broker.ProductClass{
			broker.ProductClassEquity,
			broker.ProductClassFund,
			broker.ProductClassIndex,
			broker.ProductClassBond,
			broker.ProductClassPlate,
		}
	}
}

func futuFeatureSegments(id broker.FeatureID) []broker.MarketSegment {
	value := string(id)
	switch {
	case strings.HasPrefix(value, "prediction."):
		return []broker.MarketSegment{broker.MarketSegmentPrediction}
	case strings.HasPrefix(value, "execution."):
		return []broker.MarketSegment{
			broker.MarketSegmentSecurities, broker.MarketSegmentDerivatives, broker.MarketSegmentPrediction,
		}
	case strings.HasPrefix(value, "derivatives."):
		return []broker.MarketSegment{broker.MarketSegmentDerivatives}
	default:
		return []broker.MarketSegment{broker.MarketSegmentSecurities}
	}
}

func (a *futuAdapter) DiscoverAccounts(ctx context.Context) ([]broker.Account, error) {
	accounts, err := a.exchange.DiscoverAccounts(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]broker.Account, len(accounts))
	for i, acc := range accounts {
		result[i] = broker.Account{
			ID:                   acc.AccountID,
			BrokerID:             string(Name),
			TradingEnvironment:   acc.TradingEnvironment,
			AccountType:          acc.AccountType,
			AccountRole:          acc.AccountRole,
			SecurityFirm:         acc.SecurityFirm,
			MarketAuthorities:    acc.MarketAuthorities,
			SimulatedAccountType: acc.SimulatedAccountType,
		}
	}
	return result, nil
}

func (a *futuAdapter) Trading() broker.TradingService {
	return &futuTradingService{exchange: a.exchange}
}

func (a *futuAdapter) MarketData() broker.MarketDataReader {
	return &futuMarketDataReader{exchange: a.exchange}
}

func (a *futuAdapter) QuerySecuritySnapshot(
	ctx context.Context,
	query broker.SecuritySnapshotQuery,
) (*broker.SecuritySnapshotResult, error) {
	return a.MarketData().QuerySecuritySnapshot(ctx, query)
}

func (a *futuAdapter) QueryMarketRules(ctx context.Context, query broker.MarketRuleQuery) (*broker.MarketRuleSnapshot, error) {
	return (&futuMarketDataReader{exchange: a.exchange}).QueryMarketRules(ctx, query)
}

// --- Trading Service ---

type futuTradingService struct {
	exchange *Exchange
}

func (s *futuTradingService) PlaceOrder(ctx context.Context, query broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error) {
	futuQuery := BrokerPlaceOrderQuery{
		BrokerReadQuery: BrokerReadQuery{
			AccountID:          query.AccountID,
			TradingEnvironment: query.TradingEnvironment,
			Market:             query.Market,
		},
		Session:        query.Session,
		FillOutsideRTH: query.FillOutsideRTH,
		Amount:         query.Amount,
		PredictionSide: query.PredictionSide,
	}

	submitOrder := bbgoSubmitOrderFromBrokerPlaceOrder(query)
	result, err := s.exchange.PlaceBrokerOrder(ctx, futuQuery, submitOrder)
	if err != nil {
		return nil, err
	}

	return &broker.PlaceOrderResult{
		AccountID:          result.AccountID,
		TradingEnvironment: result.TradingEnvironment,
		Market:             result.Market,
		BrokerOrderID:      formatOrderID(result.Order.OrderID),
		BrokerOrderIDEx:    result.BrokerOrderIDEx,
		Status:             brokerOrderStatus(result.Order),
	}, nil
}

func brokerOrderStatus(order bbgotypes.Order) string {
	if status := strings.TrimSpace(order.OriginalStatus); status != "" {
		return status
	}
	return string(order.Status)
}

func (s *futuTradingService) CancelOrders(ctx context.Context, query broker.ReadQuery, orders ...broker.CancelOrder) error {
	futuQuery := BrokerReadQuery{
		AccountID:          query.AccountID,
		TradingEnvironment: query.TradingEnvironment,
		Market:             query.Market,
	}

	bbgoOrders := make([]bbgotypes.Order, len(orders))
	for i, o := range orders {
		bbgoOrders[i] = bbgotypes.Order{
			SubmitOrder: bbgotypes.SubmitOrder{
				Symbol: o.Symbol,
			},
			OrderID: o.OrderID,
		}
	}
	return s.exchange.CancelBrokerOrders(ctx, futuQuery, bbgoOrders...)
}

// --- Market Data Reader ---

type futuMarketDataReader struct {
	exchange *Exchange
}

func (r *futuMarketDataReader) QueryFunds(ctx context.Context, query broker.ReadQuery) (*broker.FundsSnapshot, error) {
	futuQuery := BrokerReadQuery{
		AccountID:          query.AccountID,
		TradingEnvironment: query.TradingEnvironment,
		Market:             query.Market,
	}
	snapshot, err := r.exchange.QueryBrokerFunds(ctx, futuQuery)
	if err != nil {
		return nil, err
	}
	return convertFundsSnapshot(snapshot), nil
}

func (r *futuMarketDataReader) QueryPositions(ctx context.Context, query broker.ReadQuery) ([]broker.PositionSnapshot, error) {
	futuQuery := BrokerReadQuery{
		AccountID:          query.AccountID,
		TradingEnvironment: query.TradingEnvironment,
		Market:             query.Market,
	}
	snapshots, err := r.exchange.QueryBrokerPositions(ctx, futuQuery)
	if err != nil {
		return nil, err
	}
	result := make([]broker.PositionSnapshot, len(snapshots))
	for i, s := range snapshots {
		result[i] = convertPositionSnapshot(s)
	}
	return result, nil
}

func (r *futuMarketDataReader) QueryOrders(ctx context.Context, query broker.ReadQuery, symbol string) ([]broker.OrderSnapshot, error) {
	futuQuery := BrokerReadQuery{
		AccountID:          query.AccountID,
		TradingEnvironment: query.TradingEnvironment,
		Market:             query.Market,
	}
	snapshots, err := r.exchange.QueryBrokerOrders(ctx, futuQuery, symbol)
	if err != nil {
		return nil, err
	}
	result := make([]broker.OrderSnapshot, len(snapshots))
	for i, s := range snapshots {
		result[i] = convertOrderSnapshot(s)
	}
	return result, nil
}

func (r *futuMarketDataReader) QueryHistoryOrders(ctx context.Context, query broker.OrderHistoryQuery) ([]broker.OrderSnapshot, error) {
	futuQuery := BrokerOrderHistoryQuery{
		BrokerReadQuery: BrokerReadQuery{
			AccountID:          query.AccountID,
			TradingEnvironment: query.TradingEnvironment,
			Market:             query.Market,
		},
		Symbol:    query.Symbol,
		StartTime: query.StartTime,
		EndTime:   query.EndTime,
		Statuses:  query.Statuses,
	}
	snapshots, err := r.exchange.QueryBrokerHistoryOrders(ctx, futuQuery)
	if err != nil {
		return nil, err
	}
	result := make([]broker.OrderSnapshot, len(snapshots))
	for i, s := range snapshots {
		result[i] = convertOrderSnapshot(s)
	}
	return result, nil
}

func (r *futuMarketDataReader) QueryOrderFills(ctx context.Context, query broker.OrderFillQuery) ([]broker.OrderFillSnapshot, error) {
	futuQuery := BrokerOrderFillQuery{
		BrokerReadQuery: BrokerReadQuery{
			AccountID:          query.AccountID,
			TradingEnvironment: query.TradingEnvironment,
			Market:             query.Market,
		},
		Symbol:    query.Symbol,
		StartTime: query.StartTime,
		EndTime:   query.EndTime,
	}
	snapshots, err := r.exchange.QueryBrokerOrderFills(ctx, futuQuery)
	if err != nil {
		return nil, err
	}
	result := make([]broker.OrderFillSnapshot, len(snapshots))
	for i, s := range snapshots {
		result[i] = convertOrderFillSnapshot(s)
	}
	return result, nil
}

func (r *futuMarketDataReader) QueryHistoryOrderFills(ctx context.Context, query broker.OrderFillHistoryQuery) ([]broker.OrderFillSnapshot, error) {
	futuQuery := BrokerOrderFillHistoryQuery{
		BrokerReadQuery: BrokerReadQuery{
			AccountID:          query.AccountID,
			TradingEnvironment: query.TradingEnvironment,
			Market:             query.Market,
		},
		Symbol:    query.Symbol,
		StartTime: query.StartTime,
		EndTime:   query.EndTime,
	}
	snapshots, err := r.exchange.QueryBrokerHistoryOrderFills(ctx, futuQuery)
	if err != nil {
		return nil, err
	}
	result := make([]broker.OrderFillSnapshot, len(snapshots))
	for i, s := range snapshots {
		result[i] = convertOrderFillSnapshot(s)
	}
	return result, nil
}

func (r *futuMarketDataReader) QueryOrderFees(ctx context.Context, query broker.OrderFeeQuery) ([]broker.OrderFeeSnapshot, error) {
	futuQuery := BrokerOrderFeeQuery{
		BrokerReadQuery: BrokerReadQuery{
			AccountID:          query.AccountID,
			TradingEnvironment: query.TradingEnvironment,
			Market:             query.Market,
		},
		OrderIDExList: query.OrderIDExList,
	}
	snapshots, err := r.exchange.QueryBrokerOrderFees(ctx, futuQuery)
	if err != nil {
		return nil, err
	}
	result := make([]broker.OrderFeeSnapshot, len(snapshots))
	for i, s := range snapshots {
		result[i] = convertOrderFeeSnapshot(s)
	}
	return result, nil
}

func (r *futuMarketDataReader) QueryMarginRatios(ctx context.Context, query broker.MarginRatioQuery) ([]broker.MarginRatioSnapshot, error) {
	futuQuery := BrokerMarginRatioQuery{
		BrokerReadQuery: BrokerReadQuery{
			AccountID:          query.AccountID,
			TradingEnvironment: query.TradingEnvironment,
			Market:             query.Market,
		},
		Symbols: query.Symbols,
	}
	snapshots, err := r.exchange.QueryBrokerMarginRatios(ctx, futuQuery)
	if err != nil {
		return nil, err
	}
	result := make([]broker.MarginRatioSnapshot, len(snapshots))
	for i, s := range snapshots {
		result[i] = convertMarginRatioSnapshot(s)
	}
	return result, nil
}

func (r *futuMarketDataReader) QueryCashFlows(ctx context.Context, query broker.CashFlowQuery) ([]broker.CashFlowSnapshot, error) {
	futuQuery := BrokerCashFlowQuery{
		BrokerReadQuery: BrokerReadQuery{
			AccountID:          query.AccountID,
			TradingEnvironment: query.TradingEnvironment,
			Market:             query.Market,
		},
		ClearingDate: query.ClearingDate,
		Direction:    query.Direction,
	}
	snapshots, err := r.exchange.QueryBrokerCashFlows(ctx, futuQuery)
	if err != nil {
		return nil, err
	}
	result := make([]broker.CashFlowSnapshot, len(snapshots))
	for i, s := range snapshots {
		result[i] = convertCashFlowSnapshot(s)
	}
	return result, nil
}

func (r *futuMarketDataReader) QueryMaxTradeQuantity(ctx context.Context, query broker.MaxTradeQuantityQuery) (*broker.MaxTradeQuantitySnapshot, error) {
	futuQuery := BrokerMaxTradeQuantityQuery{
		BrokerReadQuery: BrokerReadQuery{
			AccountID:          query.AccountID,
			TradingEnvironment: query.TradingEnvironment,
			Market:             query.Market,
		},
		Symbol:             query.Symbol,
		OrderType:          query.OrderType,
		Price:              query.Price,
		OrderIDEx:          query.OrderIDEx,
		AdjustSideAndLimit: query.AdjustSideAndLimit,
		Session:            query.Session,
		PositionID:         query.PositionID,
	}
	snapshot, err := r.exchange.QueryBrokerMaxTradeQuantity(ctx, futuQuery)
	if err != nil {
		return nil, err
	}
	return new(convertMaxTradeQuantitySnapshot(snapshot)), nil
}
