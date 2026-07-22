package futu

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetusersecuritygrouppb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetusersecuritygroup"
)

const (
	futuWatchlistReadLimit  = 10
	futuWatchlistReadWindow = 30 * time.Second
	futuWatchlistCacheTTL   = 30 * time.Second
)

// ErrWatchlistReadRateLimited reports that the local guard has exhausted the
// OpenD 3213/3222 allowance. Cached results are returned before this guard is
// consulted, so normal repeated discovery does not consume the allowance.
var ErrWatchlistReadRateLimited = errors.New("futu: watchlist read rate limit exceeded (10 calls per 30 seconds)")

type futuWatchlistReader struct {
	exchange    *Exchange
	now         func() time.Time
	ttl         time.Duration
	loadGroups  func(context.Context) ([]*qotgetusersecuritygrouppb.GroupData, error)
	loadMembers func(context.Context, string) ([]*qotcommonpb.SecurityStaticInfo, error)

	fetchMu sync.Mutex
	cacheMu sync.RWMutex
	groups  futuWatchlistGroupCacheEntry
	members map[string]futuWatchlistMemberCacheEntry
	gate    futuWatchlistReadGate
}

type futuWatchlistGroupCacheEntry struct {
	expiresAt time.Time
	value     []broker.WatchlistGroup
}

type futuWatchlistMemberCacheEntry struct {
	expiresAt time.Time
	value     []broker.WatchlistSecurity
}

type futuWatchlistReadGate struct {
	mu     sync.Mutex
	calls  []time.Time
	limit  int
	window time.Duration
}

func newFutuWatchlistReader(exchange *Exchange) *futuWatchlistReader {
	reader := &futuWatchlistReader{
		exchange: exchange,
		now:      time.Now,
		ttl:      futuWatchlistCacheTTL,
		members:  make(map[string]futuWatchlistMemberCacheEntry),
		gate: futuWatchlistReadGate{
			limit:  futuWatchlistReadLimit,
			window: futuWatchlistReadWindow,
		},
	}
	reader.loadGroups = func(ctx context.Context) ([]*qotgetusersecuritygrouppb.GroupData, error) {
		var remoteGroups []*qotgetusersecuritygrouppb.GroupData
		err := exchange.withRetryingClient(ctx, func(client *opend.Client) error {
			var err error
			remoteGroups, err = client.GetUserSecurityGroups(
				ctx,
				qotgetusersecuritygrouppb.GroupType_GroupType_All,
			)
			return err
		})
		return remoteGroups, err
	}
	reader.loadMembers = func(ctx context.Context, groupName string) ([]*qotcommonpb.SecurityStaticInfo, error) {
		var remoteSecurities []*qotcommonpb.SecurityStaticInfo
		err := exchange.withRetryingClient(ctx, func(client *opend.Client) error {
			var err error
			remoteSecurities, err = client.GetUserSecurities(ctx, groupName)
			return err
		})
		return remoteSecurities, err
	}
	return reader
}

func (a *futuAdapter) ListWatchlistGroups(ctx context.Context) ([]broker.WatchlistGroup, error) {
	return a.watchlist.ListWatchlistGroups(ctx)
}

func (a *futuAdapter) ListWatchlistGroupSecurities(ctx context.Context, groupName string) ([]broker.WatchlistSecurity, error) {
	return a.watchlist.ListWatchlistGroupSecurities(ctx, groupName)
}

func (r *futuWatchlistReader) ListWatchlistGroups(ctx context.Context) ([]broker.WatchlistGroup, error) {
	return r.listWatchlistGroups(ctx, false)
}

func (a *futuAdapter) ListWatchlistGroupsFresh(ctx context.Context) ([]broker.WatchlistGroup, error) {
	return a.watchlist.ListWatchlistGroupsFresh(ctx)
}

func (a *futuAdapter) ListWatchlistGroupSecuritiesFresh(ctx context.Context, groupName string) ([]broker.WatchlistSecurity, error) {
	return a.watchlist.ListWatchlistGroupSecuritiesFresh(ctx, groupName)
}

func (r *futuWatchlistReader) ListWatchlistGroupsFresh(ctx context.Context) ([]broker.WatchlistGroup, error) {
	return r.listWatchlistGroups(ctx, true)
}

func (r *futuWatchlistReader) listWatchlistGroups(ctx context.Context, fresh bool) ([]broker.WatchlistGroup, error) {
	now := r.now()
	if !fresh {
		if cached, ok := r.cachedGroups(now); ok {
			return cached, nil
		}
	}

	r.fetchMu.Lock()
	defer r.fetchMu.Unlock()
	now = r.now()
	if !fresh {
		if cached, ok := r.cachedGroups(now); ok {
			return cached, nil
		}
	}
	if !r.gate.allow(now) {
		return nil, ErrWatchlistReadRateLimited
	}
	if r.loadGroups == nil {
		return nil, fmt.Errorf("futu: watchlist group loader is unavailable")
	}
	remoteGroups, err := r.loadGroups(ctx)
	if err != nil {
		return nil, err
	}

	groups := convertFutuWatchlistGroups(remoteGroups)

	r.cacheMu.Lock()
	r.groups = futuWatchlistGroupCacheEntry{
		expiresAt: now.Add(r.ttl),
		value:     cloneWatchlistGroups(groups),
	}
	r.cacheMu.Unlock()
	return groups, nil
}

func (r *futuWatchlistReader) ListWatchlistGroupSecurities(ctx context.Context, groupName string) ([]broker.WatchlistSecurity, error) {
	return r.listWatchlistGroupSecurities(ctx, groupName, false)
}

func (r *futuWatchlistReader) ListWatchlistGroupSecuritiesFresh(ctx context.Context, groupName string) ([]broker.WatchlistSecurity, error) {
	return r.listWatchlistGroupSecurities(ctx, groupName, true)
}

func (r *futuWatchlistReader) listWatchlistGroupSecurities(ctx context.Context, groupName string, fresh bool) ([]broker.WatchlistSecurity, error) {
	requestedName := strings.TrimSpace(groupName)
	if requestedName == "" {
		return nil, fmt.Errorf("futu: watchlist group name is required")
	}

	groups, err := r.listWatchlistGroups(ctx, fresh)
	if err != nil {
		return nil, err
	}
	normalizedName := normalizeWatchlistGroupName(requestedName)
	matchedName := ""
	matchCount := 0
	for _, group := range groups {
		if normalizeWatchlistGroupName(group.Name) != normalizedName {
			continue
		}
		matchedName = group.Name
		matchCount++
	}
	if matchCount == 0 {
		return nil, fmt.Errorf("futu: watchlist group %q not found", requestedName)
	}
	if matchCount > 1 {
		return nil, fmt.Errorf("futu: watchlist group %q is ambiguous; rename duplicate groups in Futu first", requestedName)
	}

	now := r.now()
	if !fresh {
		if cached, ok := r.cachedMembers(normalizedName, now); ok {
			return cached, nil
		}
	}

	r.fetchMu.Lock()
	defer r.fetchMu.Unlock()
	now = r.now()
	if !fresh {
		if cached, ok := r.cachedMembers(normalizedName, now); ok {
			return cached, nil
		}
	}
	if !r.gate.allow(now) {
		return nil, ErrWatchlistReadRateLimited
	}

	if r.loadMembers == nil {
		return nil, fmt.Errorf("futu: watchlist member loader is unavailable")
	}
	remoteSecurities, err := r.loadMembers(ctx, matchedName)
	if err != nil {
		return nil, err
	}

	securities := convertFutuWatchlistSecurities(remoteSecurities)

	r.cacheMu.Lock()
	r.members[normalizedName] = futuWatchlistMemberCacheEntry{
		expiresAt: now.Add(r.ttl),
		value:     cloneWatchlistSecurities(securities),
	}
	r.cacheMu.Unlock()
	return securities, nil
}

func (r *futuWatchlistReader) cachedGroups(now time.Time) ([]broker.WatchlistGroup, bool) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()
	if r.groups.expiresAt.IsZero() || !now.Before(r.groups.expiresAt) {
		return nil, false
	}
	return cloneWatchlistGroups(r.groups.value), true
}

func (r *futuWatchlistReader) cachedMembers(name string, now time.Time) ([]broker.WatchlistSecurity, bool) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()
	entry, ok := r.members[name]
	if !ok || entry.expiresAt.IsZero() || !now.Before(entry.expiresAt) {
		return nil, false
	}
	return cloneWatchlistSecurities(entry.value), true
}

func (g *futuWatchlistReadGate) allow(now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.limit <= 0 || g.window <= 0 {
		return false
	}
	cutoff := now.Add(-g.window)
	first := 0
	for first < len(g.calls) && !g.calls[first].After(cutoff) {
		first++
	}
	if first > 0 {
		g.calls = append(g.calls[:0], g.calls[first:]...)
	}
	if len(g.calls) >= g.limit {
		return false
	}
	g.calls = append(g.calls, now)
	return true
}

func normalizeWatchlistGroupName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func futuWatchlistGroupType(value int32) string {
	switch qotgetusersecuritygrouppb.GroupType(value) {
	case qotgetusersecuritygrouppb.GroupType_GroupType_Custom:
		return "custom"
	case qotgetusersecuritygrouppb.GroupType_GroupType_System:
		return "system"
	default:
		return "unknown"
	}
}

func convertFutuWatchlistGroups(remoteGroups []*qotgetusersecuritygrouppb.GroupData) []broker.WatchlistGroup {
	groups := make([]broker.WatchlistGroup, 0, len(remoteGroups))
	nameCounts := make(map[string]int, len(remoteGroups))
	for _, group := range remoteGroups {
		if group == nil || strings.TrimSpace(group.GetGroupName()) == "" {
			continue
		}
		nameCounts[normalizeWatchlistGroupName(group.GetGroupName())]++
	}
	for _, group := range remoteGroups {
		if group == nil {
			continue
		}
		name := strings.TrimSpace(group.GetGroupName())
		if name == "" {
			continue
		}
		groups = append(groups, broker.WatchlistGroup{
			Name:      name,
			Type:      futuWatchlistGroupType(group.GetGroupType()),
			Ambiguous: nameCounts[normalizeWatchlistGroupName(name)] > 1,
		})
	}
	return groups
}

func convertFutuWatchlistSecurities(remoteSecurities []*qotcommonpb.SecurityStaticInfo) []broker.WatchlistSecurity {
	securities := make([]broker.WatchlistSecurity, 0, len(remoteSecurities))
	for _, info := range remoteSecurities {
		if info == nil || info.GetBasic() == nil {
			continue
		}
		basic := info.GetBasic()
		instrumentID := securitySymbol(basic.GetSecurity())
		if instrumentID == "" {
			continue
		}
		item := broker.WatchlistSecurity{
			InstrumentID: instrumentID,
			Name:         cloneStringPtr(basic.Name),
			SecurityType: new(enumName(basic.GetSecType(), qotcommonpb.SecurityType_name)),
			BrokerCode:   cloneStringPtr(basic.GetSecurity().Code),
		}
		if basic.Id != nil {
			brokerID := strconv.FormatInt(basic.GetId(), 10)
			item.BrokerSecurityID = &brokerID
		}
		securities = append(securities, item)
	}
	return securities
}

func cloneWatchlistGroups(groups []broker.WatchlistGroup) []broker.WatchlistGroup {
	return append([]broker.WatchlistGroup(nil), groups...)
}

func cloneWatchlistSecurities(securities []broker.WatchlistSecurity) []broker.WatchlistSecurity {
	cloned := make([]broker.WatchlistSecurity, len(securities))
	for i, item := range securities {
		cloned[i] = item
		cloned[i].Name = cloneStringPtr(item.Name)
		cloned[i].SecurityType = cloneStringPtr(item.SecurityType)
		cloned[i].BrokerCode = cloneStringPtr(item.BrokerCode)
		cloned[i].BrokerSecurityID = cloneStringPtr(item.BrokerSecurityID)
	}
	return cloned
}

var _ broker.WatchlistGroupReader = (*futuAdapter)(nil)
var _ broker.FreshWatchlistGroupReader = (*futuAdapter)(nil)
