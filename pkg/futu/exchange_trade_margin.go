package futu

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	trdgetmarginratiopb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdgetmarginratio"
)

const (
	marginRatioCacheTTL         = 30 * time.Second
	marginRatioCacheFallbackTTL = 2 * time.Minute
)

type marginRatioCacheEntry struct {
	snapshots []BrokerMarginRatioSnapshot
	updatedAt time.Time
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
	if err := e.withRetryingClient(ctx, func(client *opend.Client) error {
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
		header := &trdcommonpb.TrdHeader{TrdEnv: new(int32(trdcommonpb.TrdEnv_TrdEnv_Real)), AccID: new(resolved.protoAccountID), TrdMarket: new(resolved.protoTrdMarket)}
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

		snapshots = brokerMarginRatioSnapshotsFromProto(resolved, infoList)
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
		next, removed := removeUnknownMarginSecurity(remaining, unknownCode)
		if !removed {
			return nil, err
		}
		remaining = next
	}
}

func brokerMarginRatioSnapshotsFromProto(account resolvedTradeAccount, infos []*trdgetmarginratiopb.MarginRatioInfo) []BrokerMarginRatioSnapshot {
	snapshots := make([]BrokerMarginRatioSnapshot, 0, len(infos))
	for _, info := range infos {
		if info != nil {
			snapshots = append(snapshots, brokerMarginRatioSnapshotFromProto(account, info))
		}
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Symbol < snapshots[j].Symbol })
	return snapshots
}

func removeUnknownMarginSecurity(securities []*qotcommonpb.Security, unknownCode string) ([]*qotcommonpb.Security, bool) {
	next := make([]*qotcommonpb.Security, 0, len(securities))
	removed := false
	for _, security := range securities {
		if security == nil || strings.EqualFold(strings.TrimSpace(security.GetCode()), unknownCode) {
			removed = true
			continue
		}
		next = append(next, security)
	}
	return next, removed
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
