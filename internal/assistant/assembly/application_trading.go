package assembly

import (
	"context"
	"fmt"
	"strings"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (a *ApplicationAdapter) executionOrders(ctx context.Context, input BrokerReadInput) (any, int, error) {
	service := a.trading()
	if service == nil {
		return nil, 0, fmt.Errorf("trading service is unavailable")
	}
	orders, err := service.ExecutionOrdersSnapshot(ctx)
	if err != nil {
		return nil, 0, err
	}
	filtered := make([]trdsrv.ExecutionOrder, 0, len(orders.Orders))
	for _, order := range orders.Orders {
		if !executionOrderMatchesInput(order, input) {
			continue
		}
		if input.ActiveOnly && trdsrv.IsCanonicalTerminalOrderStatus(order.Status) {
			continue
		}
		filtered = append(filtered, order)
	}
	return filtered, len(filtered), nil
}

func executionOrderMatchesInput(order trdsrv.ExecutionOrder, input BrokerReadInput) bool {
	if !strings.EqualFold(strings.TrimSpace(order.BrokerID), "futu") {
		return false
	}
	if input.TradingEnvironment != "" && !strings.EqualFold(order.TradingEnvironment, input.TradingEnvironment) {
		return false
	}
	if input.AccountID != "" && strings.TrimSpace(order.AccountID) != strings.TrimSpace(input.AccountID) {
		return false
	}
	return input.Market == "" || strings.EqualFold(order.Market, input.Market)
}

func (a *ApplicationAdapter) executionOrderEvents(internalOrderID string) (any, error) {
	service := a.trading()
	if service == nil {
		return nil, fmt.Errorf("trading service is unavailable")
	}
	events, err := service.ExecutionOrderEvents(context.Background(), internalOrderID)
	if err != nil {
		return nil, err
	}
	return events, nil
}

func (a *ApplicationAdapter) brokerOrders(ctx context.Context, input BrokerReadInput) (any, error) {
	service, err := a.requireTrading()
	if err != nil {
		return nil, err
	}
	scope, err := normalizeTradingBrokerScope(input.Scope)
	if err != nil {
		return nil, err
	}
	return service.Orders(ctx, trdsrv.OrdersQuery{
		ReadQuery: brokerReadQuery(service, input),
		Scope:     scope,
		Symbol:    strings.TrimSpace(input.Symbol),
		StartTime: strings.TrimSpace(input.StartTime),
		EndTime:   strings.TrimSpace(input.EndTime),
		Statuses:  mergeBrokerValues(input.Status, input.Statuses),
	})
}

func (a *ApplicationAdapter) brokerFills(ctx context.Context, input BrokerReadInput) (any, error) {
	service, err := a.requireTrading()
	if err != nil {
		return nil, err
	}
	scope, err := normalizeTradingBrokerScope(input.Scope)
	if err != nil {
		return nil, err
	}
	return service.Fills(ctx, trdsrv.FillsQuery{
		ReadQuery: brokerReadQuery(service, input),
		Scope:     scope,
		Symbol:    strings.TrimSpace(input.Symbol),
		StartTime: strings.TrimSpace(input.StartTime),
		EndTime:   strings.TrimSpace(input.EndTime),
	})
}

func (a *ApplicationAdapter) brokerCashFlows(ctx context.Context, input BrokerReadInput) (any, error) {
	service, err := a.requireTrading()
	if err != nil {
		return nil, err
	}
	clearingDate := strings.TrimSpace(input.ClearingDate)
	if clearingDate == "" {
		return nil, fmt.Errorf("query parameter clearingDate is required")
	}
	return service.CashFlows(ctx, broker.CashFlowQuery{
		ReadQuery:    brokerReadQuery(service, input),
		ClearingDate: clearingDate,
		Direction:    strings.TrimSpace(input.Direction),
	})
}

func (a *ApplicationAdapter) brokerFees(ctx context.Context, input BrokerReadInput) (any, error) {
	service, err := a.requireTrading()
	if err != nil {
		return nil, err
	}
	orderIDs := mergeBrokerValues(input.OrderIDEx, input.OrderIDExList)
	if len(orderIDs) == 0 {
		return nil, fmt.Errorf("query parameter orderIdEx is required")
	}
	return service.OrderFees(ctx, broker.OrderFeeQuery{
		ReadQuery:     brokerReadQuery(service, input),
		OrderIDExList: orderIDs,
	})
}

func (a *ApplicationAdapter) brokerMarginRatios(ctx context.Context, input BrokerReadInput) (any, error) {
	service, err := a.requireTrading()
	if err != nil {
		return nil, err
	}
	readQuery := brokerReadQuery(service, input)
	symbols, err := trdsrv.NormalizeSymbols(readQuery.Market, input.Symbols)
	if err != nil {
		return nil, err
	}
	if len(symbols) == 0 {
		return nil, fmt.Errorf("query parameter symbol is required")
	}
	return service.MarginRatios(ctx, broker.MarginRatioQuery{
		ReadQuery: readQuery,
		Symbols:   symbols,
	})
}

func (a *ApplicationAdapter) requireTrading() (*trdsrv.Service, error) {
	service := a.trading()
	if service == nil {
		return nil, fmt.Errorf("trading service is unavailable")
	}
	return service, nil
}

func brokerReadQuery(service *trdsrv.Service, input BrokerReadInput) broker.ReadQuery {
	return service.ReadQuery("futu", input.TradingEnvironment, input.AccountID, input.Market)
}

func normalizeTradingBrokerScope(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "CURRENT":
		return "CURRENT", nil
	case "HISTORY":
		return "HISTORY", nil
	default:
		return "", fmt.Errorf("query parameter scope is invalid")
	}
}

func mergeBrokerValues(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var values []string
	for _, group := range groups {
		for _, raw := range group {
			for part := range strings.SplitSeq(raw, ",") {
				value := strings.TrimSpace(part)
				key := strings.ToUpper(value)
				if value == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				values = append(values, value)
			}
		}
	}
	return values
}
