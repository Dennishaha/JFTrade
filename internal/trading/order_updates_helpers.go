package trading

import (
	"log"
	"sort"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func BuildOrderUpdateQueries(accounts []Account, brokerID, fallbackMarket string) []OrderQuery {
	queries := make([]OrderQuery, 0, len(accounts))
	seen := make(map[string]struct{})
	for _, account := range accounts {
		markets := append([]string(nil), account.MarketAuthorities...)
		if len(markets) == 0 {
			markets = []string{fallbackMarket}
		}
		for _, market := range markets {
			query := OrderQuery{
				BrokerID: account.BrokerID, TradingEnvironment: strings.TrimSpace(account.TradingEnvironment),
				AccountID: strings.TrimSpace(account.ID), Market: strings.ToUpper(strings.TrimSpace(market)),
			}
			key := OrderUpdateSubscriptionKey(query)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			queries = append(queries, query)
		}
	}
	if len(queries) == 0 && strings.TrimSpace(brokerID) != "" && strings.TrimSpace(fallbackMarket) != "" {
		queries = append(queries, OrderQuery{
			BrokerID: strings.TrimSpace(brokerID), TradingEnvironment: "SIMULATE",
			Market: strings.ToUpper(strings.TrimSpace(fallbackMarket)),
		})
	}
	sort.Slice(queries, func(i, j int) bool {
		return OrderUpdateSubscriptionKey(queries[i]) < OrderUpdateSubscriptionKey(queries[j])
	})
	return queries
}

func OrderUpdateSubscriptionKey(query OrderQuery) string {
	return strings.Join([]string{
		strings.TrimSpace(query.BrokerID), strings.ToUpper(strings.TrimSpace(query.TradingEnvironment)),
		strings.TrimSpace(query.AccountID), strings.ToUpper(strings.TrimSpace(query.Market)),
	}, "|")
}

func IsTerminalOrderStatus(status string) bool {
	return IsCanonicalTerminalOrderStatus(CanonicalBrokerOrderStatus(status))
}

func orderUpdatePushSubscriptionKey(accounts []Account, queries []OrderQuery) string {
	accountKeys := make([]string, 0, len(accounts))
	for _, account := range accounts {
		markets := make([]string, 0, len(account.MarketAuthorities))
		for _, market := range account.MarketAuthorities {
			if trimmed := strings.ToUpper(strings.TrimSpace(market)); trimmed != "" {
				markets = append(markets, trimmed)
			}
		}
		sort.Strings(markets)
		accountKeys = append(accountKeys, strings.Join([]string{
			strings.TrimSpace(account.BrokerID),
			strings.ToUpper(strings.TrimSpace(account.TradingEnvironment)),
			strings.TrimSpace(account.ID),
			strings.Join(markets, ","),
		}, "|"))
	}
	sort.Strings(accountKeys)

	queryKeys := make([]string, 0, len(queries))
	for _, query := range queries {
		queryKeys = append(queryKeys, OrderUpdateSubscriptionKey(query))
	}
	sort.Strings(queryKeys)
	return strings.Join([]string{
		strings.Join(accountKeys, ";"),
		strings.Join(queryKeys, ";"),
	}, "\n")
}

func (w *OrderUpdatesWorker) now() time.Time {
	return w.config.Now().UTC()
}

func queryForOrder(brokerID, accountID, environment, market string) OrderQuery {
	return OrderQuery{BrokerID: brokerID, AccountID: accountID, TradingEnvironment: environment, Market: market}
}

func sameOrder(order Order, orderID string, orderIDEx *string) bool {
	if orderIDEx != nil && order.BrokerOrderIDEx != nil && strings.TrimSpace(*orderIDEx) == strings.TrimSpace(*order.BrokerOrderIDEx) {
		return true
	}
	return strings.TrimSpace(orderID) != "" && strings.TrimSpace(orderID) == strings.TrimSpace(order.BrokerOrderID)
}

func cloneAccounts(accounts []Account) []Account {
	out := make([]Account, len(accounts))
	for i, account := range accounts {
		out[i] = account
		out[i].MarketAuthorities = append([]string(nil), account.MarketAuthorities...)
	}
	return out
}

func cloneOrders(orders []Order) []Order {
	out := make([]Order, len(orders))
	for i, order := range orders {
		out[i] = cloneOrder(order)
	}
	return out
}

func cloneOrder(order Order) Order {
	order.BrokerOrderIDEx = cloneString(order.BrokerOrderIDEx)
	order.SymbolName = cloneString(order.SymbolName)
	order.Amount = cloneFloat(order.Amount)
	order.Legs = append([]broker.OrderLegSnapshot(nil), order.Legs...)
	order.FilledQuantity = cloneFloat(order.FilledQuantity)
	order.Price = cloneFloat(order.Price)
	order.FilledAveragePrice = cloneFloat(order.FilledAveragePrice)
	order.Remark = cloneString(order.Remark)
	order.LastError = cloneString(order.LastError)
	order.TimeInForce = cloneString(order.TimeInForce)
	order.Currency = cloneString(order.Currency)
	return order
}

func cloneFill(fill Fill) Fill {
	fill.BrokerOrderIDEx = cloneString(fill.BrokerOrderIDEx)
	fill.BrokerFillIDEx = cloneString(fill.BrokerFillIDEx)
	fill.SymbolName = cloneString(fill.SymbolName)
	fill.FillPrice = cloneFloat(fill.FillPrice)
	fill.Status = cloneString(fill.Status)
	fill.Payout = cloneFloat(fill.Payout)
	return fill
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func stringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
