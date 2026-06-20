package futu

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetmaxtrdqtyspb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmaxtrdqtys"
)

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
			OrderType: new(int32(orderType)),
			Code:      new(code),
			Price:     new(query.Price),
			SecMarket: new(int32(secMarket)),
		}
		if trimmed := strings.TrimSpace(query.OrderIDEx); trimmed != "" {
			request.OrderIDEx = new(trimmed)
		}
		if query.AdjustSideAndLimit != nil {
			request.AdjustPrice = new(*query.AdjustSideAndLimit != 0)
			request.AdjustSideAndLimit = new(*query.AdjustSideAndLimit)
		}
		if query.Session != nil {
			if session, ok := sessionValue(*query.Session); ok {
				request.Session = new(session)
			} else {
				return fmt.Errorf("futu exchange: unsupported session %q", *query.Session)
			}
		}
		if query.PositionID != nil {
			request.PositionID = new(*query.PositionID)
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
