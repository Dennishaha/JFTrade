package futu

import (
	"context"
	"fmt"
	"maps"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	"github.com/jftrade/jftrade-main/pkg/market"
)

const (
	stockScreenSnapshotCacheTTL = 15 * time.Second
	stockScreenSnapshotPageSize = 300
)

type stockScreenSnapshotFetch func(context.Context, []string) (map[string]broker.SecuritySnapshotItem, error)

type cachedStockScreenSnapshot struct {
	item      *broker.SecuritySnapshotItem
	expiresAt time.Time
}

// stockScreenSnapshotCoordinator keeps delayed StockScreen reads bounded even
// when several quote surfaces ask for the same unavailable BasicQot symbol.
// A nil item is a short-lived negative cache entry.
type stockScreenSnapshotCoordinator struct {
	mu       sync.Mutex
	cache    map[string]cachedStockScreenSnapshot
	now      func() time.Time
	cacheTTL time.Duration
	flights  singleflight.Group
}

func newStockScreenSnapshotCoordinator() *stockScreenSnapshotCoordinator {
	return &stockScreenSnapshotCoordinator{
		cache:    make(map[string]cachedStockScreenSnapshot),
		now:      time.Now,
		cacheTTL: stockScreenSnapshotCacheTTL,
	}
}

func (c *stockScreenSnapshotCoordinator) query(
	ctx context.Context,
	symbols []string,
	fetch stockScreenSnapshotFetch,
) (map[string]broker.SecuritySnapshotItem, error) {
	canonical, err := canonicalStockScreenSnapshotSymbols(symbols)
	if err != nil {
		return nil, err
	}
	if len(canonical) == 0 {
		return map[string]broker.SecuritySnapshotItem{}, nil
	}
	if c == nil || fetch == nil {
		return nil, fmt.Errorf("futu stock-screen snapshot fallback is unavailable")
	}

	result, missing := c.cached(canonical)
	if len(missing) == 0 {
		return result, nil
	}
	key := strings.Join(missing, "\x00")
	flight := c.flights.DoChan(key, func() (any, error) {
		items, fetchErr := fetch(ctx, missing)
		if fetchErr != nil {
			return nil, fetchErr
		}
		c.store(missing, items)
		return cloneStockScreenSnapshotMap(items), nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case completed := <-flight:
		if completed.Err != nil {
			return nil, completed.Err
		}
		items, ok := completed.Val.(map[string]broker.SecuritySnapshotItem)
		if !ok {
			return nil, fmt.Errorf("futu stock-screen snapshot fallback returned an invalid result")
		}
		for symbol, item := range items {
			result[symbol] = cloneStockScreenSnapshotItem(item)
		}
		return result, nil
	}
}

func canonicalStockScreenSnapshotSymbols(symbols []string) ([]string, error) {
	seen := make(map[string]struct{}, len(symbols))
	canonical := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		_, normalized, err := futuSecurityFromSymbol(symbol)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		canonical = append(canonical, normalized)
	}
	sort.Strings(canonical)
	return canonical, nil
}

func (c *stockScreenSnapshotCoordinator) cached(symbols []string) (map[string]broker.SecuritySnapshotItem, []string) {
	now := c.clock()
	result := make(map[string]broker.SecuritySnapshotItem, len(symbols))
	missing := make([]string, 0, len(symbols))
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, symbol := range symbols {
		entry, ok := c.cache[symbol]
		if !ok || !entry.expiresAt.After(now) {
			delete(c.cache, symbol)
			missing = append(missing, symbol)
			continue
		}
		if entry.item != nil {
			result[symbol] = cloneStockScreenSnapshotItem(*entry.item)
		}
	}
	return result, missing
}

func (c *stockScreenSnapshotCoordinator) store(symbols []string, items map[string]broker.SecuritySnapshotItem) {
	expiresAt := c.clock().Add(c.cacheTTL)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, symbol := range symbols {
		entry := cachedStockScreenSnapshot{expiresAt: expiresAt}
		if item, ok := items[symbol]; ok {
			copy := cloneStockScreenSnapshotItem(item)
			entry.item = &copy
		}
		c.cache[symbol] = entry
	}
}

func (c *stockScreenSnapshotCoordinator) clock() time.Time {
	if c != nil && c.now != nil {
		return c.now().UTC()
	}
	return time.Now().UTC()
}

func cloneStockScreenSnapshotMap(values map[string]broker.SecuritySnapshotItem) map[string]broker.SecuritySnapshotItem {
	cloned := make(map[string]broker.SecuritySnapshotItem, len(values))
	for symbol, item := range values {
		cloned[symbol] = cloneStockScreenSnapshotItem(item)
	}
	return cloned
}

func cloneStockScreenSnapshotItem(item broker.SecuritySnapshotItem) broker.SecuritySnapshotItem {
	item.Name = cloneStringPtr(item.Name)
	item.SecurityType = cloneStringPtr(item.SecurityType)
	item.IsSuspended = cloneBoolPtr(item.IsSuspended)
	item.LastPrice = cloneFloat64Ptr(item.LastPrice)
	item.BidPrice = cloneFloat64Ptr(item.BidPrice)
	item.AskPrice = cloneFloat64Ptr(item.AskPrice)
	item.PreviousClose = cloneFloat64Ptr(item.PreviousClose)
	item.OpenPrice = cloneFloat64Ptr(item.OpenPrice)
	item.HighPrice = cloneFloat64Ptr(item.HighPrice)
	item.LowPrice = cloneFloat64Ptr(item.LowPrice)
	item.Volume = cloneFloat64Ptr(item.Volume)
	item.Turnover = cloneFloat64Ptr(item.Turnover)
	item.LotSize = cloneInt32Ptr(item.LotSize)
	item.UpdateTime = cloneStringPtr(item.UpdateTime)
	item.Session = cloneStringPtr(item.Session)
	return item
}

// QuerySnapshotFallback returns delayed OpenD StockScreen values for symbols
// whose BasicQot subscription cannot be established. It is intentionally an
// adapter capability rather than a second market-data provider.
func (a *futuAdapter) QuerySnapshotFallback(
	ctx context.Context,
	query broker.SecuritySnapshotQuery,
) (*broker.SecuritySnapshotResult, error) {
	if a == nil || a.exchange == nil {
		return nil, fmt.Errorf("futu stock-screen snapshot fallback is unavailable")
	}
	if len(query.Symbols) == 0 {
		return nil, fmt.Errorf("futu: QuerySnapshotFallback requires at least one symbol")
	}
	coordinator := a.stockScreenSnapshotCoordinator()
	if coordinator == nil {
		return nil, fmt.Errorf("futu stock-screen snapshot fallback is unavailable")
	}
	items, err := coordinator.query(ctx, query.Symbols, a.fetchStockScreenSnapshots)
	if err != nil {
		return nil, err
	}
	result := &broker.SecuritySnapshotResult{AccountID: query.AccountID}
	for _, symbol := range query.Symbols {
		canonical := strings.ToUpper(strings.TrimSpace(symbol))
		if item, ok := items[canonical]; ok {
			result.Snapshots = append(result.Snapshots, item)
		}
	}
	return result, nil
}

func (a *futuAdapter) stockScreenSnapshotCoordinator() *stockScreenSnapshotCoordinator {
	if a == nil {
		return nil
	}
	a.snapshotFallbackMu.Lock()
	defer a.snapshotFallbackMu.Unlock()
	if a.snapshotFallback == nil {
		a.snapshotFallback = newStockScreenSnapshotCoordinator()
	}
	return a.snapshotFallback
}

func (a *futuAdapter) fetchStockScreenSnapshots(
	ctx context.Context,
	symbols []string,
) (map[string]broker.SecuritySnapshotItem, error) {
	staticInfo, err := a.exchange.queryStaticInfoList(ctx, symbols)
	if err != nil {
		return nil, err
	}
	type requestGroup struct {
		market  int64
		stockID []uint64
		symbols map[uint64]string
		names   map[string]*string
	}
	groups := make(map[int64]*requestGroup)
	for _, symbol := range symbols {
		info := staticInfo[symbol]
		if info == nil || info.GetBasic() == nil || info.GetBasic().GetId() <= 0 {
			continue
		}
		marketValue, ok := stockScreenMarketValue(stockScreenSymbolMarket(symbol))
		if !ok {
			continue
		}
		group := groups[marketValue]
		if group == nil {
			group = &requestGroup{market: marketValue, symbols: make(map[uint64]string), names: make(map[string]*string)}
			groups[marketValue] = group
		}
		stockID := uint64(info.GetBasic().GetId())
		group.stockID = append(group.stockID, stockID)
		group.symbols[stockID] = symbol
		if name := strings.TrimSpace(info.GetBasic().GetName()); name != "" {
			group.names[symbol] = &name
		}
	}

	marketValues := make([]int, 0, len(groups))
	for value := range groups {
		marketValues = append(marketValues, int(value))
	}
	sort.Ints(marketValues)
	result := make(map[string]broker.SecuritySnapshotItem, len(symbols))
	for _, rawMarket := range marketValues {
		group := groups[int64(rawMarket)]
		for start := 0; start < len(group.stockID); start += stockScreenSnapshotPageSize {
			end := min(start+stockScreenSnapshotPageSize, len(group.stockID))
			payload, queryErr := a.queryStockScreenSnapshotPage(ctx, group.market, group.stockID[start:end])
			if queryErr != nil {
				return nil, queryErr
			}
			maps.Copy(result, stockScreenSnapshotItems(payload, group.symbols, group.names, time.Now().UTC()))
		}
	}
	return result, nil
}

func stockScreenSymbolMarket(symbol string) string {
	market, _, ok := strings.Cut(strings.ToUpper(strings.TrimSpace(symbol)), ".")
	if !ok {
		return ""
	}
	return market
}

func stockScreenMarketValue(marketCode string) (int64, bool) {
	switch strings.ToUpper(strings.TrimSpace(marketCode)) {
	case "HK":
		return 1, true
	case "US":
		return 2, true
	case "CN", "SH", "SZ":
		return 3, true
	case "SG":
		return 4, true
	case "CA":
		return 5, true
	case "AU":
		return 6, true
	case "JP":
		return 7, true
	case "MY":
		return 8, true
	default:
		return 0, false
	}
}

func (a *futuAdapter) queryStockScreenSnapshotPage(
	ctx context.Context,
	marketValue int64,
	stockIDs []uint64,
) (map[string]any, error) {
	if len(stockIDs) == 0 || len(stockIDs) > stockScreenSnapshotPageSize {
		return nil, fmt.Errorf("futu stock-screen snapshot page requires 1..%d stock ids", stockScreenSnapshotPageSize)
	}
	params := stockScreenSnapshotParams(marketValue, stockIDs)
	var payload map[string]any
	err := a.exchange.withRetryingClient(ctx, func(client *opend.Client) error {
		if retryAfter := a.researchScreenRetryAfter(client); retryAfter > 0 {
			return broker.NewSnapshotRateLimitError(retryAfter, nil)
		}
		var callErr error
		payload, callErr = client.CallAdvanced(ctx, "Qot_StockScreen", params)
		return callErr
	})
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func stockScreenSnapshotParams(marketValue int64, stockIDs []uint64) map[string]any {
	simpleProperties := []int32{2201, 2202, 2203, 2204, 2205, 2207, 2208}
	retrieve := make([]any, 0, len(simpleProperties)+2)
	for _, property := range simpleProperties {
		retrieve = append(retrieve, map[string]any{
			"simpleProperty": map[string]any{"name": property},
		})
	}
	for _, property := range []int32{3101} {
		retrieve = append(retrieve, map[string]any{
			"cumulativeProperty": map[string]any{"name": property, "days": 1},
		})
	}
	return map[string]any{
		"filterList": []any{
			map[string]any{
				"simpleFieldQuery": map[string]any{"simpleField": int32(1), "screenValueList": []int64{marketValue}},
			},
			map[string]any{
				"simpleFieldQuery": map[string]any{"simpleField": int32(4), "screenValueList": []int64{1}},
			},
		},
		"retrieveList":      retrieve,
		"watchlistStockIds": append([]uint64(nil), stockIDs...),
		"pageCount":         int32(len(stockIDs)),
	}
}

type stockScreenFallbackValues struct {
	simple     map[int32]float64
	cumulative map[int32]float64
}

func stockScreenSnapshotItems(
	payload map[string]any,
	symbols map[uint64]string,
	names map[string]*string,
	observedAt time.Time,
) map[string]broker.SecuritySnapshotItem {
	rows, _ := payload["dataList"].([]any)
	result := make(map[string]broker.SecuritySnapshotItem, len(rows))
	for _, rawRow := range rows {
		row, ok := rawRow.(map[string]any)
		if !ok {
			continue
		}
		stockID, ok := stockScreenUint64(row["stockId"])
		if !ok {
			continue
		}
		symbol := symbols[stockID]
		if symbol == "" {
			continue
		}
		values := stockScreenValues(row)
		price, ok := values.simple[2201]
		if !ok || !stockScreenPositive(price) {
			continue
		}
		item := broker.SecuritySnapshotItem{
			Symbol: symbol, Source: "futu:stock-screen-delayed", LastPrice: &price, ObservedAt: observedAt.UTC(),
		}
		if name := names[symbol]; name != nil {
			item.Name = cloneStringPtr(name)
		}
		item.OpenPrice = stockScreenOptional(values.simple, 2202)
		item.PreviousClose = stockScreenOptional(values.simple, 2203)
		item.HighPrice = stockScreenOptional(values.simple, 2204)
		item.LowPrice = stockScreenOptional(values.simple, 2205)
		item.BidPrice = stockScreenOptional(values.simple, 2207)
		item.AskPrice = stockScreenOptional(values.simple, 2208)
		if item.PreviousClose == nil {
			item.PreviousClose = stockScreenPreviousClose(price, values)
		}
		session := string(market.ClassifySession(symbol, observedAt))
		item.Session = &session
		result[symbol] = item
	}
	return result
}

func stockScreenValues(row map[string]any) stockScreenFallbackValues {
	values := stockScreenFallbackValues{simple: make(map[int32]float64), cumulative: make(map[int32]float64)}
	results, _ := row["results"].([]any)
	for _, rawResult := range results {
		result, ok := rawResult.(map[string]any)
		if !ok {
			continue
		}
		for _, descriptor := range []struct {
			key    string
			values map[int32]float64
		}{
			{key: "simplePropertyResult", values: values.simple},
			{key: "cumulativePropertyResult", values: values.cumulative},
		} {
			entry, ok := result[descriptor.key].(map[string]any)
			if !ok {
				continue
			}
			property, ok := entry["property"].(map[string]any)
			if !ok {
				continue
			}
			name, ok := stockScreenInt32(property["name"])
			if !ok {
				continue
			}
			value, ok := stockScreenResultNumber(entry)
			if ok {
				descriptor.values[name] = value
			}
		}
	}
	return values
}

func stockScreenResultNumber(entry map[string]any) (float64, bool) {
	for _, key := range []string{"dval", "ival"} {
		if value, ok := stockScreenFloat(entry[key]); ok {
			return value, true
		}
	}
	return 0, false
}

func stockScreenPreviousClose(price float64, values stockScreenFallbackValues) *float64 {
	if change, ok := values.cumulative[3101]; ok && stockScreenFinite(change) {
		previous := price - change
		if stockScreenPositive(previous) {
			return &previous
		}
	}
	return nil
}

func stockScreenOptional(values map[int32]float64, key int32) *float64 {
	value, ok := values[key]
	if !ok || !stockScreenFinite(value) {
		return nil
	}
	return &value
}

func stockScreenPositive(value float64) bool { return stockScreenFinite(value) && value > 0 }

func stockScreenFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func stockScreenFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, stockScreenFinite(typed)
	case float32:
		result := float64(typed)
		return result, stockScreenFinite(result)
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case string:
		result, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return result, err == nil && stockScreenFinite(result)
	default:
		return 0, false
	}
}

func stockScreenUint64(value any) (uint64, bool) {
	if text, ok := value.(string); ok {
		result, err := strconv.ParseUint(strings.TrimSpace(text), 10, 64)
		return result, err == nil
	}
	number, ok := stockScreenFloat(value)
	if !ok || number < 0 || number != math.Trunc(number) || number > float64(^uint64(0)) {
		return 0, false
	}
	return uint64(number), true
}

func stockScreenInt32(value any) (int32, bool) {
	number, ok := stockScreenFloat(value)
	if !ok || number != math.Trunc(number) || number < math.MinInt32 || number > math.MaxInt32 {
		return 0, false
	}
	return int32(number), true
}
