package futu

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
	trdgetmaxtrdqtyspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmaxtrdqtys"
)

const (
	marginRatioCacheTTL         = 30 * time.Second
	marginRatioCacheFallbackTTL = 2 * time.Minute
)

type marginRatioCacheEntry struct {
	snapshots []BrokerMarginRatioSnapshot
	updatedAt time.Time
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

// QueryBrokerHistoryOrders returns normalized historical broker orders for the
// selected trading account context.
func (e *Exchange) QueryBrokerHistoryOrders(ctx context.Context, query BrokerOrderHistoryQuery) ([]BrokerOrderSnapshot, error) {
	var snapshots []BrokerOrderSnapshot
	if err := e.withClient(ctx, func(client *opend.Client) error {
		resolved, err := e.resolveTradeAccountWithClient(ctx, client, query.BrokerReadQuery)
		if err != nil {
			return err
		}

		orders, err := client.GetHistoryOrderList(ctx, resolved.header(), brokerTradeFilterConditions(query.Symbol, query.StartTime, query.EndTime, resolved.protoTrdMarket), brokerOrderStatusFilterValues(query.Statuses))
		if err != nil {
			return err
		}

		snapshots = make([]BrokerOrderSnapshot, 0, len(orders))
		canonicalSymbol := strings.TrimSpace(strings.ToUpper(query.Symbol))
		for _, order := range orders {
			if order == nil {
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

// QueryBrokerHistoryOrderFills returns normalized historical broker fills for
// the selected trading account context.
func (e *Exchange) QueryBrokerHistoryOrderFills(ctx context.Context, query BrokerOrderFillHistoryQuery) ([]BrokerOrderFillSnapshot, error) {
	var snapshots []BrokerOrderFillSnapshot
	if err := e.withClient(ctx, func(client *opend.Client) error {
		resolved, err := e.resolveTradeAccountWithClient(ctx, client, query.BrokerReadQuery)
		if err != nil {
			return err
		}

		fills, err := client.GetHistoryOrderFillList(ctx, resolved.header(), brokerTradeFilterConditions(query.Symbol, query.StartTime, query.EndTime, resolved.protoTrdMarket))
		if err != nil {
			return err
		}

		snapshots = make([]BrokerOrderFillSnapshot, 0, len(fills))
		canonicalSymbol := strings.TrimSpace(strings.ToUpper(query.Symbol))
		for _, fill := range fills {
			if fill == nil {
				continue
			}
			if canonicalSymbol != "" && !strings.EqualFold(strings.TrimSpace(fill.GetCode()), canonicalSymbol) {
				continue
			}
			snapshots = append(snapshots, brokerOrderFillSnapshotFromProto(resolved, fill))
		}

		sort.Slice(snapshots, func(i, j int) bool {
			left := brokerOrderFillSortKey(snapshots[i])
			right := brokerOrderFillSortKey(snapshots[j])
			if !left.Equal(right) {
				return left.After(right)
			}
			return snapshots[i].BrokerFillID > snapshots[j].BrokerFillID
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return snapshots, nil
}

// QueryBrokerOrderFills returns normalized current-session broker fills for the
// selected trading account context.
func (e *Exchange) QueryBrokerOrderFills(ctx context.Context, query BrokerOrderFillQuery) ([]BrokerOrderFillSnapshot, error) {
	var snapshots []BrokerOrderFillSnapshot
	if err := e.withClient(ctx, func(client *opend.Client) error {
		resolved, err := e.resolveTradeAccountWithClient(ctx, client, query.BrokerReadQuery)
		if err != nil {
			return err
		}

		fills, err := client.GetOrderFillList(ctx, resolved.header(), brokerTradeFilterConditions(query.Symbol, query.StartTime, query.EndTime, resolved.protoTrdMarket))
		if err != nil {
			return err
		}

		snapshots = make([]BrokerOrderFillSnapshot, 0, len(fills))
		canonicalSymbol := strings.TrimSpace(strings.ToUpper(query.Symbol))
		for _, fill := range fills {
			if fill == nil {
				continue
			}
			if canonicalSymbol != "" && !strings.EqualFold(strings.TrimSpace(fill.GetCode()), canonicalSymbol) {
				continue
			}
			snapshots = append(snapshots, brokerOrderFillSnapshotFromProto(resolved, fill))
		}

		sort.Slice(snapshots, func(i, j int) bool {
			left := brokerOrderFillSortKey(snapshots[i])
			right := brokerOrderFillSortKey(snapshots[j])
			if !left.Equal(right) {
				return left.After(right)
			}
			return snapshots[i].BrokerFillID > snapshots[j].BrokerFillID
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return snapshots, nil
}

// QueryBrokerOrderFees returns normalized broker order fees for the selected
// trading account context.
func (e *Exchange) QueryBrokerOrderFees(ctx context.Context, query BrokerOrderFeeQuery) ([]BrokerOrderFeeSnapshot, error) {
	var snapshots []BrokerOrderFeeSnapshot
	if err := e.withClient(ctx, func(client *opend.Client) error {
		resolved, err := e.resolveTradeAccountWithClient(ctx, client, query.BrokerReadQuery)
		if err != nil {
			return err
		}

		fees, err := client.GetOrderFee(ctx, resolved.header(), query.OrderIDExList)
		if err != nil {
			return err
		}

		snapshots = make([]BrokerOrderFeeSnapshot, 0, len(fees))
		for _, fee := range fees {
			if fee == nil {
				continue
			}
			snapshots = append(snapshots, brokerOrderFeeSnapshotFromProto(resolved, fee))
		}

		sort.Slice(snapshots, func(i, j int) bool {
			return snapshots[i].BrokerOrderIDEx < snapshots[j].BrokerOrderIDEx
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return snapshots, nil
}

// QueryBrokerMarginRatios returns margin-ratio data for the requested symbols.
func (e *Exchange) QueryBrokerMarginRatios(ctx context.Context, query BrokerMarginRatioQuery) ([]BrokerMarginRatioSnapshot, error) {
	var snapshots []BrokerMarginRatioSnapshot
	resolveQuery := query.BrokerReadQuery
	resolveQuery.TradingEnvironment = "REAL"
	if resolveQuery.Market == "" && len(query.Symbols) > 0 {
		resolveQuery.Market = marketFromSymbol(query.Symbols[0], "")
	}
	cacheKey := marginRatioCacheKey(resolveQuery, query.Symbols)
	if cached, ok := e.getMarginRatioCache(cacheKey, marginRatioCacheTTL); ok {
		return cached, nil
	}
	if err := e.withClient(ctx, func(client *opend.Client) error {
		resolved, err := e.resolveTradeAccountWithClient(ctx, client, resolveQuery)
		if err != nil {
			return err
		}

		qotSecurityListRequest := make([]*qotcommonpb.Security, 0, len(query.Symbols))
		for _, symbol := range query.Symbols {
			security, canonical, err := futuSecurityFromSymbol(symbol)
			if err != nil {
				return err
			}
			qotSecurityListRequest = append(qotSecurityListRequest, security)
			_ = canonical
		}
		header := &trdcommonpb.TrdHeader{TrdEnv: proto.Int32(int32(trdcommonpb.TrdEnv_TrdEnv_Real)), AccID: proto.Uint64(resolved.protoAccountID), TrdMarket: proto.Int32(int32(resolved.protoTrdMarket))}
		infoList, err := marginRatioInfoListWithUnknownStockRecovery(ctx, client, header, qotSecurityListRequest)
		if err != nil {
			if isMarginRatioRateLimitedError(err) {
				if cached, ok := e.getMarginRatioCache(cacheKey, marginRatioCacheFallbackTTL); ok {
					snapshots = cached
					return nil
				}
			}
			return err
		}

		snapshots = make([]BrokerMarginRatioSnapshot, 0, len(infoList))
		for _, info := range infoList {
			if info == nil {
				continue
			}
			snapshots = append(snapshots, brokerMarginRatioSnapshotFromProto(resolved, info))
		}
		sort.Slice(snapshots, func(i, j int) bool {
			return snapshots[i].Symbol < snapshots[j].Symbol
		})
		return nil
	}); err != nil {
		return nil, err
	}
	e.setMarginRatioCache(cacheKey, snapshots)
	return snapshots, nil
}

func marginRatioInfoListWithUnknownStockRecovery(
	ctx context.Context,
	client *opend.Client,
	header *trdcommonpb.TrdHeader,
	securities []*qotcommonpb.Security,
) ([]*trdgetmarginratiopb.MarginRatioInfo, error) {
	remaining := append([]*qotcommonpb.Security(nil), securities...)
	for {
		if len(remaining) == 0 {
			return []*trdgetmarginratiopb.MarginRatioInfo{}, nil
		}
		infoList, err := client.GetMarginRatio(ctx, header, remaining)
		if err == nil {
			return infoList, nil
		}
		if !isUnknownStockError(err) {
			return nil, err
		}
		unknownCode, ok := extractUnknownStockCode(err)
		if !ok {
			return nil, err
		}
		next := make([]*qotcommonpb.Security, 0, len(remaining))
		for _, security := range remaining {
			if security == nil {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(security.GetCode()), unknownCode) {
				continue
			}
			next = append(next, security)
		}
		if len(next) == len(remaining) {
			return nil, err
		}
		remaining = next
	}
}

func isUnknownStockError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "未知股票") || strings.Contains(lower, "unknown stock") || strings.Contains(lower, "unknown security")
}

func isMarginRatioRateLimitedError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "频率太高") || (strings.Contains(lower, "too high") && strings.Contains(lower, "request")) || strings.Contains(lower, "每30秒最多10次") || strings.Contains(lower, "rate limit")
}

func extractUnknownStockCode(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	message := strings.TrimSpace(err.Error())
	markers := []string{"未知股票", "unknown stock", "unknown security"}
	for _, marker := range markers {
		idx := strings.Index(strings.ToLower(message), strings.ToLower(marker))
		if idx < 0 {
			continue
		}
		remainder := strings.TrimSpace(message[idx+len(marker):])
		fields := strings.Fields(remainder)
		if len(fields) == 0 {
			return "", false
		}
		code := strings.Trim(fields[0], "\"'.,;:()[]{}")
		code = strings.ToUpper(strings.TrimSpace(code))
		if code == "" {
			return "", false
		}
		return code, true
	}
	return "", false
}

func marginRatioCacheKey(query BrokerReadQuery, symbols []string) string {
	normalizedSymbols := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		normalized := strings.ToUpper(strings.TrimSpace(symbol))
		if normalized == "" {
			continue
		}
		normalizedSymbols = append(normalizedSymbols, normalized)
	}
	sort.Strings(normalizedSymbols)
	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(query.AccountID)),
		strings.ToUpper(strings.TrimSpace(query.TradingEnvironment)),
		strings.ToUpper(strings.TrimSpace(query.Market)),
		strings.Join(normalizedSymbols, ","),
	}, "|")
}

func (e *Exchange) getMarginRatioCache(key string, maxAge time.Duration) ([]BrokerMarginRatioSnapshot, bool) {
	if key == "" {
		return nil, false
	}
	e.marginRatioCacheMu.RLock()
	entry, ok := e.marginRatioCache[key]
	e.marginRatioCacheMu.RUnlock()
	if !ok || time.Since(entry.updatedAt) > maxAge {
		return nil, false
	}
	return append([]BrokerMarginRatioSnapshot(nil), entry.snapshots...), true
}

func (e *Exchange) setMarginRatioCache(key string, snapshots []BrokerMarginRatioSnapshot) {
	if key == "" {
		return
	}
	copySnapshots := append([]BrokerMarginRatioSnapshot(nil), snapshots...)
	e.marginRatioCacheMu.Lock()
	e.marginRatioCache[key] = marginRatioCacheEntry{snapshots: copySnapshots, updatedAt: time.Now().UTC()}
	e.marginRatioCacheMu.Unlock()
}

// QueryBrokerCashFlows returns account cash-flow snapshots.
func (e *Exchange) QueryBrokerCashFlows(ctx context.Context, query BrokerCashFlowQuery) ([]BrokerCashFlowSnapshot, error) {
	var snapshots []BrokerCashFlowSnapshot
	if err := e.withClient(ctx, func(client *opend.Client) error {
		resolved, err := e.resolveTradeAccountWithClient(ctx, client, query.BrokerReadQuery)
		if err != nil {
			return err
		}
		direction := cashFlowDirectionValue(query.Direction)
		flows, err := client.GetFlowSummary(ctx, resolved.header(), strings.TrimSpace(query.ClearingDate), direction)
		if err != nil {
			return err
		}

		snapshots = make([]BrokerCashFlowSnapshot, 0, len(flows))
		for _, flow := range flows {
			if flow == nil {
				continue
			}
			snapshots = append(snapshots, brokerCashFlowSnapshotFromProto(resolved, flow))
		}
		sort.Slice(snapshots, func(i, j int) bool {
			left := optionalStringValue(snapshots[i].ClearingDate)
			right := optionalStringValue(snapshots[j].ClearingDate)
			if left != right {
				return left > right
			}
			return optionalStringValue(snapshots[i].CashFlowID) > optionalStringValue(snapshots[j].CashFlowID)
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return snapshots, nil
}

// QueryBrokerMaxTradeQuantity returns the maximum tradable quantity snapshot
// for a target symbol and order shape.
func (e *Exchange) QueryBrokerMaxTradeQuantity(ctx context.Context, query BrokerMaxTradeQuantityQuery) (*BrokerMaxTradeQuantitySnapshot, error) {
	canonicalSymbol := strings.TrimSpace(strings.ToUpper(query.Symbol))
	if canonicalSymbol == "" {
		return nil, fmt.Errorf("futu exchange: symbol is required")
	}
	if query.Price <= 0 {
		return nil, fmt.Errorf("futu exchange: price must be positive")
	}
	orderType, normalizedOrderType, ok := trdOrderTypeFromBrokerOrderType(query.OrderType)
	if !ok {
		return nil, fmt.Errorf("futu exchange: unsupported orderType %q", query.OrderType)
	}

	resolveQuery := query.BrokerReadQuery
	if resolveQuery.Market == "" {
		resolveQuery.Market = marketFromSymbol(canonicalSymbol, "")
	}

	var snapshot *BrokerMaxTradeQuantitySnapshot
	if err := e.withClient(ctx, func(client *opend.Client) error {
		resolved, err := e.resolveTradeAccountWithClient(ctx, client, resolveQuery)
		if err != nil {
			return err
		}
		code, secMarket, err := tradeSecurityInfoFromSymbol(canonicalSymbol)
		if err != nil {
			return err
		}
		request := &trdgetmaxtrdqtyspb.C2S{
			Header:    resolved.header(),
			OrderType: proto.Int32(int32(orderType)),
			Code:      proto.String(code),
			Price:     proto.Float64(query.Price),
			SecMarket: proto.Int32(int32(secMarket)),
		}
		if trimmed := strings.TrimSpace(query.OrderIDEx); trimmed != "" {
			request.OrderIDEx = proto.String(trimmed)
		}
		if query.AdjustSideAndLimit != nil {
			request.AdjustPrice = proto.Bool(*query.AdjustSideAndLimit != 0)
			request.AdjustSideAndLimit = proto.Float64(*query.AdjustSideAndLimit)
		}
		if query.Session != nil {
			if session, ok := sessionValue(*query.Session); ok {
				request.Session = proto.Int32(session)
			} else {
				return fmt.Errorf("futu exchange: unsupported session %q", *query.Session)
			}
		}
		if query.PositionID != nil {
			request.PositionID = proto.Uint64(*query.PositionID)
		}

		maxQtys, err := client.GetMaxTrdQtys(ctx, request)
		if err != nil {
			return err
		}
		snapshot = brokerMaxTradeQuantitySnapshotFromProto(resolved, canonicalSymbol, normalizedOrderType, query.Price, maxQtys)
		return nil
	}); err != nil {
		return nil, err
	}
	return snapshot, nil
}
