package futu

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

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
	CurrencyBalances        []BrokerCurrencyBalanceSnapshot
	MarketAssets            []BrokerMarketAssetSnapshot
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
}

type BrokerOrderSnapshot struct {
	AccountID          string
	TradingEnvironment string
	Market             string
	BrokerOrderID      string
	BrokerOrderIDEx    *string
	Symbol             string
	SymbolName         *string
	Side               string
	OrderType          string
	Status             string
	Quantity           float64
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

type resolvedTradeAccount struct {
	AccountID          string
	TradingEnvironment string
	Market             string
	AccountType        string
	protoAccountID     uint64
	protoTrdEnv        int32
	protoTrdMarket     int32
}

func (e *Exchange) queryAccount(ctx context.Context) (*types.Account, error) {
	var (
		selected resolvedTradeAccount
		funds    *trdcommonpb.Funds
	)

	if err := e.withClient(ctx, func(client *opend.Client) error {
		resolved, err := e.resolveTradeAccountWithClient(ctx, client, BrokerReadQuery{})
		if err != nil {
			return err
		}
		selected = resolved
		funds, err = client.GetFunds(ctx, resolved.header())
		return err
	}); err != nil {
		return nil, err
	}

	account := types.NewAccount()
	account.RawAccount = funds
	account.AccountType = bbgoAccountTypeFromRuntimeAccountType(selected.AccountType)
	account.CanTrade = true
	account.CanDeposit = true
	account.CanWithdraw = true
	if funds != nil && funds.TotalAssets != nil {
		account.TotalAccountValue = fixedpoint.NewFromFloat(funds.GetTotalAssets())
	}
	account.UpdateBalances(balanceMapFromFunds(funds, selected.Market))
	return account, nil
}

func (e *Exchange) queryAccountBalances(ctx context.Context) (types.BalanceMap, error) {
	snapshot, err := e.QueryBrokerFunds(ctx, BrokerReadQuery{})
	if err != nil {
		return nil, err
	}
	return balanceMapFromBrokerFunds(snapshot), nil
}

func (e *Exchange) queryOpenOrders(ctx context.Context, symbol string) ([]types.Order, error) {
	orders, err := e.QueryBrokerOrders(ctx, BrokerReadQuery{}, symbol)
	if err != nil {
		return nil, err
	}
	result := make([]types.Order, 0, len(orders))
	for _, order := range orders {
		result = append(result, bbgoOrderFromBrokerOrder(order))
	}
	return result, nil
}

// QueryBrokerFunds returns a normalized funds snapshot for the selected Futu
// trading account context.
func (e *Exchange) QueryBrokerFunds(ctx context.Context, query BrokerReadQuery) (*BrokerFundsSnapshot, error) {
	var snapshot *BrokerFundsSnapshot
	if err := e.withClient(ctx, func(client *opend.Client) error {
		resolved, err := e.resolveTradeAccountWithClient(ctx, client, query)
		if err != nil {
			return err
		}
		funds, err := client.GetFunds(ctx, resolved.header())
		if err != nil {
			return err
		}
		snapshot = brokerFundsSnapshotFromProto(resolved, funds)
		return nil
	}); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// QueryBrokerPositions returns normalized broker positions for the selected
// trading account context.
func (e *Exchange) QueryBrokerPositions(ctx context.Context, query BrokerReadQuery) ([]BrokerPositionSnapshot, error) {
	var snapshots []BrokerPositionSnapshot
	if err := e.withClient(ctx, func(client *opend.Client) error {
		resolved, err := e.resolveTradeAccountWithClient(ctx, client, query)
		if err != nil {
			return err
		}
		positions, err := client.GetPositionList(ctx, resolved.header(), nil)
		if err != nil {
			return err
		}
		snapshots = make([]BrokerPositionSnapshot, 0, len(positions))
		for _, position := range positions {
			if position == nil {
				continue
			}
			snapshots = append(snapshots, brokerPositionSnapshotFromProto(resolved, position))
		}
		sort.Slice(snapshots, func(i, j int) bool {
			if snapshots[i].Market != snapshots[j].Market {
				return snapshots[i].Market < snapshots[j].Market
			}
			return snapshots[i].Symbol < snapshots[j].Symbol
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return snapshots, nil
}

// QueryBrokerOrders returns normalized active broker orders for the selected
// trading account context.
func (e *Exchange) QueryBrokerOrders(ctx context.Context, query BrokerReadQuery, symbol string) ([]BrokerOrderSnapshot, error) {
	var snapshots []BrokerOrderSnapshot
	if err := e.withClient(ctx, func(client *opend.Client) error {
		resolved, err := e.resolveTradeAccountWithClient(ctx, client, query)
		if err != nil {
			return err
		}

		var filter *trdcommonpb.TrdFilterConditions
		canonicalSymbol := strings.TrimSpace(strings.ToUpper(symbol))
		if canonicalSymbol != "" {
			filter = &trdcommonpb.TrdFilterConditions{CodeList: []string{canonicalSymbol}}
		}

		orders, err := client.GetOrderList(ctx, resolved.header(), filter)
		if err != nil {
			return err
		}

		snapshots = make([]BrokerOrderSnapshot, 0, len(orders))
		for _, order := range orders {
			if order == nil || !brokerOrderIsWorking(order.GetOrderStatus()) {
				continue
			}
			if canonicalSymbol != "" && !strings.EqualFold(strings.TrimSpace(order.GetCode()), canonicalSymbol) {
				continue
			}
			snapshots = append(snapshots, brokerOrderSnapshotFromProto(resolved, order))
		}

		sort.Slice(snapshots, func(i, j int) bool {
			left := brokerOrderSortKey(snapshots[i])
			right := brokerOrderSortKey(snapshots[j])
			if !left.Equal(right) {
				return left.After(right)
			}
			return snapshots[i].BrokerOrderID > snapshots[j].BrokerOrderID
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return snapshots, nil
}

func (e *Exchange) resolveTradeAccountWithClient(ctx context.Context, client *opend.Client, query BrokerReadQuery) (resolvedTradeAccount, error) {
	accounts, err := client.GetAccountList(ctx)
	if err != nil {
		return resolvedTradeAccount{}, err
	}
	if len(accounts) == 0 {
		return resolvedTradeAccount{}, fmt.Errorf("futu exchange: no trading accounts discovered")
	}

	normalized := normalizeBrokerReadQuery(query)
	candidates := make([]resolvedTradeAccount, 0, len(accounts))
	for _, account := range accounts {
		candidate, ok, err := candidateTradeAccountFromProto(account, normalized)
		if err != nil {
			return resolvedTradeAccount{}, err
		}
		if ok {
			candidates = append(candidates, candidate)
		}
	}

	if len(candidates) == 0 {
		if normalized.AccountID != "" {
			return resolvedTradeAccount{}, fmt.Errorf("futu exchange: account %s not found for tradingEnvironment=%s market=%s", normalized.AccountID, normalized.TradingEnvironment, normalized.Market)
		}
		return resolvedTradeAccount{}, fmt.Errorf("futu exchange: no trading account matched tradingEnvironment=%s market=%s", normalized.TradingEnvironment, normalized.Market)
	}

	if normalized.TradingEnvironment == "" {
		if preferred := filterResolvedTradeAccountsByEnvironment(candidates, "SIMULATE"); len(preferred) > 0 {
			candidates = preferred
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		leftPriority := resolvedTradeAccountPriority(candidates[i])
		rightPriority := resolvedTradeAccountPriority(candidates[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if candidates[i].TradingEnvironment != candidates[j].TradingEnvironment {
			return candidates[i].TradingEnvironment < candidates[j].TradingEnvironment
		}
		if candidates[i].AccountID != candidates[j].AccountID {
			return candidates[i].AccountID < candidates[j].AccountID
		}
		return candidates[i].Market < candidates[j].Market
	})

	return candidates[0], nil
}

func candidateTradeAccountFromProto(account *trdcommonpb.TrdAcc, query BrokerReadQuery) (resolvedTradeAccount, bool, error) {
	if account == nil {
		return resolvedTradeAccount{}, false, nil
	}

	runtimeAccount := runtimeAccountFromProto(account)
	accountID := runtimeAccount.AccountID
	protoAccountID := strconv.FormatUint(account.GetAccID(), 10)
	if query.AccountID != "" && !strings.EqualFold(query.AccountID, accountID) && !strings.EqualFold(query.AccountID, protoAccountID) {
		return resolvedTradeAccount{}, false, nil
	}
	if query.TradingEnvironment != "" && !strings.EqualFold(query.TradingEnvironment, runtimeAccount.TradingEnvironment) {
		return resolvedTradeAccount{}, false, nil
	}

	selectedMarket, selectedMarketCode, ok, err := resolveTradeMarket(account, query.Market)
	if err != nil {
		return resolvedTradeAccount{}, false, err
	}
	if !ok {
		return resolvedTradeAccount{}, false, nil
	}

	return resolvedTradeAccount{
		AccountID:          accountID,
		TradingEnvironment: runtimeAccount.TradingEnvironment,
		Market:             selectedMarket,
		AccountType:        runtimeAccount.AccountType,
		protoAccountID:     account.GetAccID(),
		protoTrdEnv:        account.GetTrdEnv(),
		protoTrdMarket:     selectedMarketCode,
	}, true, nil
}

func resolveTradeMarket(account *trdcommonpb.TrdAcc, requested string) (string, int32, bool, error) {
	normalizedRequested := strings.ToUpper(strings.TrimSpace(requested))
	authList := account.GetTrdMarketAuthList()
	if normalizedRequested != "" {
		if len(authList) > 0 {
			for _, rawMarket := range authList {
				if runtimeMarketAuthority(rawMarket) == normalizedRequested {
					return normalizedRequested, rawMarket, true, nil
				}
			}
			return "", 0, false, nil
		}
		rawMarket, ok := trdMarketFromNormalized(normalizedRequested)
		if !ok {
			return "", 0, false, fmt.Errorf("futu exchange: unsupported market %q", requested)
		}
		return normalizedRequested, int32(rawMarket), true, nil
	}

	for _, rawMarket := range authList {
		normalizedMarket := runtimeMarketAuthority(rawMarket)
		if normalizedMarket == "" {
			continue
		}
		return normalizedMarket, rawMarket, true, nil
	}

	return "HK", int32(trdcommonpb.TrdMarket_TrdMarket_HK), true, nil
}

func trdMarketFromNormalized(market string) (trdcommonpb.TrdMarket, bool) {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "HK":
		return trdcommonpb.TrdMarket_TrdMarket_HK, true
	case "US":
		return trdcommonpb.TrdMarket_TrdMarket_US, true
	case "CN":
		return trdcommonpb.TrdMarket_TrdMarket_CN, true
	case "SG":
		return trdcommonpb.TrdMarket_TrdMarket_SG, true
	case "AU":
		return trdcommonpb.TrdMarket_TrdMarket_AU, true
	case "JP":
		return trdcommonpb.TrdMarket_TrdMarket_JP, true
	case "MY":
		return trdcommonpb.TrdMarket_TrdMarket_MY, true
	case "CA":
		return trdcommonpb.TrdMarket_TrdMarket_CA, true
	case "CRYPTO":
		return trdcommonpb.TrdMarket_TrdMarket_Crypto, true
	case "FUTURES":
		return trdcommonpb.TrdMarket_TrdMarket_Futures, true
	default:
		return 0, false
	}
}

func normalizeBrokerReadQuery(query BrokerReadQuery) BrokerReadQuery {
	return BrokerReadQuery{
		AccountID:          strings.TrimSpace(query.AccountID),
		TradingEnvironment: strings.ToUpper(strings.TrimSpace(query.TradingEnvironment)),
		Market:             strings.ToUpper(strings.TrimSpace(query.Market)),
	}
}

func filterResolvedTradeAccountsByEnvironment(candidates []resolvedTradeAccount, tradingEnvironment string) []resolvedTradeAccount {
	filtered := make([]resolvedTradeAccount, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.TradingEnvironment, tradingEnvironment) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func resolvedTradeAccountPriority(candidate resolvedTradeAccount) int {
	switch candidate.TradingEnvironment {
	case "SIMULATE":
		return 0
	case "REAL":
		return 1
	default:
		return 2
	}
}

func (account resolvedTradeAccount) header() *trdcommonpb.TrdHeader {
	return &trdcommonpb.TrdHeader{
		TrdEnv:    proto.Int32(account.protoTrdEnv),
		AccID:     proto.Uint64(account.protoAccountID),
		TrdMarket: proto.Int32(account.protoTrdMarket),
	}
}

func brokerFundsSnapshotFromProto(account resolvedTradeAccount, funds *trdcommonpb.Funds) *BrokerFundsSnapshot {
	if funds == nil {
		funds = &trdcommonpb.Funds{}
	}

	snapshot := &BrokerFundsSnapshot{
		AccountID:               account.AccountID,
		TradingEnvironment:      account.TradingEnvironment,
		Market:                  account.Market,
		AccountType:             account.AccountType,
		Currency:                optionalEnumStringPtr(funds.Currency, trdcommonpb.Currency_name),
		TotalAssets:             cloneFloat64Ptr(funds.TotalAssets),
		SecuritiesAssets:        cloneFloat64Ptr(funds.SecuritiesAssets),
		FundAssets:              cloneFloat64Ptr(funds.FundAssets),
		BondAssets:              cloneFloat64Ptr(funds.BondAssets),
		Cash:                    cloneFloat64Ptr(funds.Cash),
		MarketValue:             cloneFloat64Ptr(funds.MarketVal),
		LongMarketValue:         cloneFloat64Ptr(funds.LongMv),
		ShortMarketValue:        cloneFloat64Ptr(funds.ShortMv),
		PurchasingPower:         cloneFloat64Ptr(funds.Power),
		ShortSellingPower:       cloneFloat64Ptr(funds.MaxPowerShort),
		NetCashPower:            cloneFloat64Ptr(funds.NetCashPower),
		AvailableWithdrawalCash: cloneFloat64Ptr(funds.AvlWithdrawalCash),
		MaxWithdrawal:           cloneFloat64Ptr(funds.MaxWithdrawal),
		AvailableFunds:          cloneFloat64Ptr(funds.AvailableFunds),
		FrozenCash:              cloneFloat64Ptr(funds.FrozenCash),
		PendingAsset:            cloneFloat64Ptr(funds.PendingAsset),
		UnrealizedPnl:           cloneFloat64Ptr(funds.UnrealizedPL),
		RealizedPnl:             cloneFloat64Ptr(funds.RealizedPL),
		InitialMargin:           cloneFloat64Ptr(funds.InitialMargin),
		MaintenanceMargin:       cloneFloat64Ptr(funds.MaintenanceMargin),
		MarginCallMargin:        cloneFloat64Ptr(funds.MarginCallMargin),
		RiskStatus:              optionalEnumStringPtr(funds.RiskStatus, trdcommonpb.CltRiskStatus_name),
	}

	snapshot.CurrencyBalances = make([]BrokerCurrencyBalanceSnapshot, 0, len(funds.GetCashInfoList()))
	for _, cashInfo := range funds.GetCashInfoList() {
		if cashInfo == nil {
			continue
		}
		currency := optionalEnumStringPtr(cashInfo.Currency, trdcommonpb.Currency_name)
		if currency == nil {
			continue
		}
		snapshot.CurrencyBalances = append(snapshot.CurrencyBalances, BrokerCurrencyBalanceSnapshot{
			AccountID:               account.AccountID,
			TradingEnvironment:      account.TradingEnvironment,
			Currency:                *currency,
			Cash:                    cloneFloat64Ptr(cashInfo.Cash),
			AvailableWithdrawalCash: cloneFloat64Ptr(cashInfo.AvailableBalance),
			NetCashPower:            cloneFloat64Ptr(cashInfo.NetCashPower),
		})
	}

	snapshot.MarketAssets = make([]BrokerMarketAssetSnapshot, 0, len(funds.GetMarketInfoList()))
	for _, marketInfo := range funds.GetMarketInfoList() {
		if marketInfo == nil {
			continue
		}
		market := runtimeMarketAuthority(marketInfo.GetTrdMarket())
		if market == "" {
			continue
		}
		snapshot.MarketAssets = append(snapshot.MarketAssets, BrokerMarketAssetSnapshot{
			AccountID:          account.AccountID,
			TradingEnvironment: account.TradingEnvironment,
			Market:             market,
			Assets:             cloneFloat64Ptr(marketInfo.Assets),
		})
	}

	return snapshot
}

func brokerPositionSnapshotFromProto(account resolvedTradeAccount, position *trdcommonpb.Position) BrokerPositionSnapshot {
	market := runtimeMarketAuthority(position.GetTrdMarket())
	if market == "" {
		market = marketFromSymbol(position.GetCode(), account.Market)
	}

	return BrokerPositionSnapshot{
		AccountID:          account.AccountID,
		TradingEnvironment: account.TradingEnvironment,
		Market:             market,
		Symbol:             strings.TrimSpace(strings.ToUpper(position.GetCode())),
		SymbolName:         optionalNonEmptyString(position.GetName()),
		Quantity:           position.GetQty(),
		SellableQuantity:   position.GetCanSellQty(),
		LastPrice:          position.GetPrice(),
		CostPrice:          preferredFloat64Ptr(position.DilutedCostPrice, position.CostPrice),
		AverageCostPrice:   cloneFloat64Ptr(position.AverageCostPrice),
		MarketValue:        position.GetVal(),
		UnrealizedPnl:      preferredFloat64Ptr(position.UnrealizedPL, position.PlVal),
		RealizedPnl:        cloneFloat64Ptr(position.RealizedPL),
		PnlRatio:           preferredFloat64Ptr(position.AveragePlRatio, position.PlRatio),
		Currency:           optionalEnumStringPtr(position.Currency, trdcommonpb.Currency_name),
	}
}

func brokerOrderSnapshotFromProto(account resolvedTradeAccount, order *trdcommonpb.Order) BrokerOrderSnapshot {
	market := runtimeMarketAuthority(order.GetTrdMarket())
	if market == "" {
		market = marketFromSymbol(order.GetCode(), account.Market)
	}

	return BrokerOrderSnapshot{
		AccountID:          account.AccountID,
		TradingEnvironment: account.TradingEnvironment,
		Market:             market,
		BrokerOrderID:      strconv.FormatUint(order.GetOrderID(), 10),
		BrokerOrderIDEx:    optionalNonEmptyString(order.GetOrderIDEx()),
		Symbol:             strings.TrimSpace(strings.ToUpper(order.GetCode())),
		SymbolName:         optionalNonEmptyString(order.GetName()),
		Side:               normalizeRuntimeEnum(enumName(order.GetTrdSide(), trdcommonpb.TrdSide_name)),
		OrderType:          normalizeRuntimeEnum(enumName(order.GetOrderType(), trdcommonpb.OrderType_name)),
		Status:             normalizeRuntimeEnum(enumName(order.GetOrderStatus(), trdcommonpb.OrderStatus_name)),
		Quantity:           order.GetQty(),
		FilledQuantity:     cloneFloat64Ptr(order.FillQty),
		Price:              cloneFloat64Ptr(order.Price),
		FilledAveragePrice: cloneFloat64Ptr(order.FillAvgPrice),
		SubmittedAt:        formatBrokerOrderTime(order.CreateTimestamp, order.GetCreateTime()),
		UpdatedAt:          formatBrokerOrderTime(order.UpdateTimestamp, order.GetUpdateTime()),
		Remark:             optionalNonEmptyString(order.GetRemark()),
		LastError:          optionalNonEmptyString(order.GetLastErrMsg()),
		TimeInForce:        optionalEnumStringPtr(order.TimeInForce, trdcommonpb.TimeInForce_name),
		Currency:           optionalEnumStringPtr(order.Currency, trdcommonpb.Currency_name),
	}
}

func balanceMapFromBrokerFunds(snapshot *BrokerFundsSnapshot) types.BalanceMap {
	balances := types.BalanceMap{}
	if snapshot == nil {
		return balances
	}
	for _, balance := range snapshot.CurrencyBalances {
		balances[balance.Currency] = types.Balance{
			Currency:          balance.Currency,
			Available:         fixedpointFromPtr(balance.AvailableWithdrawalCash, balance.Cash),
			Locked:            fixedpointFromPtr(nil, nil),
			NetAsset:          fixedpointFromPtr(balance.Cash, nil),
			MaxWithdrawAmount: fixedpointFromPtr(balance.AvailableWithdrawalCash, nil),
		}
	}
	if len(balances) > 0 {
		return balances
	}

	currency := defaultCurrencyForMarket(snapshot.Market)
	if snapshot.Currency != nil && *snapshot.Currency != "" {
		currency = *snapshot.Currency
	}
	if currency == "" {
		currency = "HKD"
	}
	balances[currency] = types.Balance{
		Currency:          currency,
		Available:         fixedpointFromPtr(snapshot.AvailableWithdrawalCash, snapshot.Cash),
		Locked:            fixedpointFromDifference(snapshot.Cash, snapshot.AvailableWithdrawalCash, snapshot.FrozenCash),
		NetAsset:          fixedpointFromPtr(snapshot.Cash, nil),
		MaxWithdrawAmount: fixedpointFromPtr(snapshot.MaxWithdrawal, snapshot.AvailableWithdrawalCash),
	}
	return balances
}

func balanceMapFromFunds(funds *trdcommonpb.Funds, market string) types.BalanceMap {
	return balanceMapFromBrokerFunds(brokerFundsSnapshotFromProto(resolvedTradeAccount{Market: market}, funds))
}

func bbgoAccountTypeFromRuntimeAccountType(accountType string) types.AccountType {
	switch strings.ToUpper(strings.TrimSpace(accountType)) {
	case "MARGIN":
		return types.AccountTypeMargin
	case "DERIVATIVES":
		return types.AccountTypeFutures
	default:
		return types.AccountTypeSpot
	}
}

func bbgoOrderFromBrokerOrder(order BrokerOrderSnapshot) types.Order {
	createdAt := parseBrokerOrderTime(order.SubmittedAt)
	updatedAt := parseBrokerOrderTime(order.UpdatedAt)
	market := inferMarket(order.Symbol)

	return types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol:      order.Symbol,
			Side:        bbgoSideFromBrokerOrderSide(order.Side),
			Type:        bbgoOrderTypeFromBrokerOrderType(order.OrderType),
			Price:       fixedpoint.NewFromFloat(optionalFloat64Value(order.Price)),
			Quantity:    fixedpoint.NewFromFloat(order.Quantity),
			TimeInForce: bbgoTimeInForceFromBrokerOrder(order.TimeInForce),
			Market:      market,
		},
		Exchange:         Name,
		OrderID:          parseUint64(order.BrokerOrderID),
		Status:           bbgoOrderStatusFromBrokerOrderStatus(order.Status),
		OriginalStatus:   order.Status,
		ExecutedQuantity: fixedpoint.NewFromFloat(optionalFloat64Value(order.FilledQuantity)),
		IsWorking:        bbgoOrderStatusFromBrokerOrderStatus(order.Status).Closed() == false,
		CreationTime:     types.Time(createdAt),
		UpdateTime:       types.Time(updatedAt),
	}
}

func bbgoSideFromBrokerOrderSide(side string) types.SideType {
	switch strings.ToUpper(strings.TrimSpace(side)) {
	case "SELL", "SELLSHORT":
		return types.SideTypeSell
	default:
		return types.SideTypeBuy
	}
}

func bbgoOrderTypeFromBrokerOrderType(orderType string) types.OrderType {
	switch strings.ToUpper(strings.TrimSpace(orderType)) {
	case "MARKET", "TWAP_MARKET", "VWAP_MARKET":
		return types.OrderTypeMarket
	case "STOP", "TRAILINGSTOP":
		return types.OrderTypeStopMarket
	case "STOPLIMIT", "TRAILINGSTOPLIMIT":
		return types.OrderTypeStopLimit
	case "MARKETIFTOUCHED":
		return types.OrderTypeTakeProfitMarket
	case "LIMITIFTOUCHED":
		return types.OrderTypeTakeProfit
	default:
		return types.OrderTypeLimit
	}
}

func bbgoTimeInForceFromBrokerOrder(timeInForce *string) types.TimeInForce {
	if timeInForce == nil {
		return ""
	}
	switch strings.ToUpper(strings.TrimSpace(*timeInForce)) {
	case "IOC":
		return types.TimeInForceIOC
	case "FOK":
		return types.TimeInForceFOK
	case "GTT":
		return types.TimeInForceGTT
	case "GTC":
		return types.TimeInForceGTC
	default:
		return ""
	}
}

func bbgoOrderStatusFromBrokerOrderStatus(status string) types.OrderStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FILLED_ALL":
		return types.OrderStatusFilled
	case "FILLED_PART", "CANCELLING_PART", "CANCELLED_PART":
		return types.OrderStatusPartiallyFilled
	case "CANCELLED_ALL":
		return types.OrderStatusCanceled
	case "SUBMITFAILED", "FAILED", "DISABLED", "DELETED", "FILLCANCELLED":
		return types.OrderStatusRejected
	case "TIMEOUT":
		return types.OrderStatusNew
	default:
		return types.OrderStatusNew
	}
}

func brokerOrderIsWorking(status int32) bool {
	switch trdcommonpb.OrderStatus(status) {
	case trdcommonpb.OrderStatus_OrderStatus_Filled_All,
		trdcommonpb.OrderStatus_OrderStatus_Cancelled_Part,
		trdcommonpb.OrderStatus_OrderStatus_Cancelled_All,
		trdcommonpb.OrderStatus_OrderStatus_SubmitFailed,
		trdcommonpb.OrderStatus_OrderStatus_Failed,
		trdcommonpb.OrderStatus_OrderStatus_Disabled,
		trdcommonpb.OrderStatus_OrderStatus_Deleted,
		trdcommonpb.OrderStatus_OrderStatus_FillCancelled:
		return false
	default:
		return true
	}
}

func brokerOrderSortKey(order BrokerOrderSnapshot) time.Time {
	updatedAt := parseBrokerOrderTime(order.UpdatedAt)
	if !updatedAt.IsZero() {
		return updatedAt
	}
	return parseBrokerOrderTime(order.SubmittedAt)
}

func parseBrokerOrderTime(value string) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return parsed
	}
	for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func formatBrokerOrderTime(timestamp *float64, fallback string) string {
	if timestamp != nil && *timestamp > 0 {
		seconds := int64(*timestamp)
		nanos := int64((*timestamp - float64(seconds)) * float64(time.Second))
		return time.Unix(seconds, nanos).UTC().Format(time.RFC3339Nano)
	}
	parsed := parseBrokerOrderTime(fallback)
	if parsed.IsZero() {
		return strings.TrimSpace(fallback)
	}
	return parsed.Format(time.RFC3339Nano)
}

func marketFromSymbol(symbol string, fallback string) string {
	trimmed := strings.TrimSpace(strings.ToUpper(symbol))
	if strings.HasPrefix(trimmed, "HK.") {
		return "HK"
	}
	if strings.HasPrefix(trimmed, "US.") {
		return "US"
	}
	if strings.HasPrefix(trimmed, "SH.") || strings.HasPrefix(trimmed, "SZ.") {
		return "CN"
	}
	if strings.HasPrefix(trimmed, "CN.") {
		return "CN"
	}
	if strings.HasPrefix(trimmed, "SG.") {
		return "SG"
	}
	if strings.HasPrefix(trimmed, "JP.") {
		return "JP"
	}
	if strings.HasPrefix(trimmed, "AU.") {
		return "AU"
	}
	if strings.HasPrefix(trimmed, "MY.") {
		return "MY"
	}
	if strings.HasPrefix(trimmed, "CA.") {
		return "CA"
	}
	if fallback != "" {
		return fallback
	}
	return "HK"
}

func defaultCurrencyForMarket(market string) string {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "US":
		return "USD"
	case "CN":
		return "CNY"
	case "SG":
		return "SGD"
	case "JP":
		return "JPY"
	case "CA":
		return "CAD"
	case "AU":
		return "AUD"
	default:
		return "HKD"
	}
}

func optionalEnumStringPtr(value *int32, names map[int32]string) *string {
	if value == nil {
		return nil
	}
	normalized := normalizeRuntimeEnum(enumName(*value, names))
	if normalized == "" || normalized == "UNKNOWN" {
		return nil
	}
	return &normalized
}

func optionalNonEmptyString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func preferredFloat64Ptr(primary *float64, fallback *float64) *float64 {
	if primary != nil {
		return cloneFloat64Ptr(primary)
	}
	return cloneFloat64Ptr(fallback)
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func fixedpointFromPtr(primary *float64, fallback *float64) fixedpoint.Value {
	if primary != nil {
		return fixedpoint.NewFromFloat(*primary)
	}
	if fallback != nil {
		return fixedpoint.NewFromFloat(*fallback)
	}
	return fixedpoint.Zero
}

func fixedpointFromDifference(total *float64, available *float64, fallback *float64) fixedpoint.Value {
	if total != nil && available != nil {
		value := *total - *available
		if value < 0 {
			value = 0
		}
		return fixedpoint.NewFromFloat(value)
	}
	if fallback != nil {
		return fixedpoint.NewFromFloat(*fallback)
	}
	return fixedpoint.Zero
}

func optionalFloat64Value(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func parseUint64(value string) uint64 {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
