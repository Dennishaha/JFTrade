package servercore

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/internal/watchlist"
	"github.com/jftrade/jftrade-main/pkg/broker"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
)

const (
	futuWatchlistSourceID = "futu:default"
	futuWatchlistCacheTTL = 3 * time.Second
	futuHKSnapshotBatch   = 20
	futuSnapshotBatch     = 400
	futuSnapshotReadLimit = 60
	futuSnapshotWindow    = 30 * time.Second
)

type watchlistGroupReaderProvider func() (broker.WatchlistGroupReader, error)
type watchlistSourceProbe func(context.Context) error

type futuWatchlistReader struct {
	reader      watchlistGroupReaderProvider
	enabled     func() bool
	probe       watchlistSourceProbe
	now         func() time.Time
	cacheTTL    time.Duration
	mu          sync.Mutex
	groups      []watchlist.RemoteGroup
	groupsUntil time.Time
	members     map[string]cachedWatchlistMembers
}

type cachedWatchlistMembers struct {
	items []watchlist.RemoteMember
	until time.Time
}

func newFutuWatchlistReader(provider watchlistGroupReaderProvider, enabled func() bool, probes ...watchlistSourceProbe) *futuWatchlistReader {
	reader := &futuWatchlistReader{
		reader: provider, enabled: enabled, now: time.Now, cacheTTL: futuWatchlistCacheTTL,
		members: make(map[string]cachedWatchlistMembers),
	}
	if len(probes) > 0 {
		reader.probe = probes[0]
	}
	return reader
}

func (r *futuWatchlistReader) Source(ctx context.Context) (watchlist.Source, error) {
	source := watchlist.Source{
		ID: futuWatchlistSourceID, Broker: "futu", DisplayName: "Futu OpenD", Status: "ready", UpdatedAt: r.clock().UTC(),
	}
	if r == nil || r.enabled == nil || !r.enabled() {
		source.Status = "disabled"
		source.Error = "Futu integration is disabled"
		return source, nil
	}
	if r.reader == nil {
		source.Status = "unavailable"
		source.Error = "Futu watchlist reader is unavailable"
		return source, nil
	}
	if _, err := r.reader(); err != nil {
		source.Status = "unavailable"
		source.Error = err.Error()
		return source, nil
	}
	if r.probe != nil {
		if err := r.probe(ctx); err != nil {
			source.Status = "unavailable"
			source.Error = err.Error()
		}
	}
	return source, nil
}

func (r *futuWatchlistReader) ListGroups(ctx context.Context) ([]watchlist.RemoteGroup, error) {
	if r == nil || r.reader == nil {
		return nil, watchlist.ErrUnavailable
	}
	now := r.clock()
	r.mu.Lock()
	if now.Before(r.groupsUntil) {
		groups := append([]watchlist.RemoteGroup(nil), r.groups...)
		r.mu.Unlock()
		return groups, nil
	}
	r.mu.Unlock()
	reader, err := r.reader()
	if err != nil {
		return nil, err
	}
	remote, err := reader.ListWatchlistGroups(ctx)
	if err != nil {
		return nil, err
	}
	groups := normalizeFutuWatchlistGroups(remote, now.UTC())
	r.mu.Lock()
	r.groups = append([]watchlist.RemoteGroup(nil), groups...)
	r.groupsUntil = now.Add(r.cacheTTL)
	r.mu.Unlock()
	return groups, nil
}

func (r *futuWatchlistReader) ListGroupMembers(ctx context.Context, remoteGroupID string) ([]watchlist.RemoteMember, error) {
	remoteGroupID = strings.TrimSpace(remoteGroupID)
	if remoteGroupID == "" {
		return nil, fmt.Errorf("%w: remoteGroupId is required", watchlist.ErrValidation)
	}
	groups, err := r.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	group, found := remoteGroupByID(groups, remoteGroupID)
	if !found {
		return nil, watchlist.ErrNotFound
	}
	if group.Ambiguous {
		return nil, watchlist.ErrAmbiguousRemoteGroup
	}
	now := r.clock()
	r.mu.Lock()
	if cached, ok := r.members[remoteGroupID]; ok && now.Before(cached.until) {
		items := append([]watchlist.RemoteMember(nil), cached.items...)
		r.mu.Unlock()
		return items, nil
	}
	r.mu.Unlock()
	reader, err := r.reader()
	if err != nil {
		return nil, err
	}
	securities, err := reader.ListWatchlistGroupSecurities(ctx, group.Name)
	if err != nil {
		return nil, err
	}
	items := remoteMembersFromBroker(securities)
	r.mu.Lock()
	r.members[remoteGroupID] = cachedWatchlistMembers{items: append([]watchlist.RemoteMember(nil), items...), until: now.Add(r.cacheTTL)}
	r.mu.Unlock()
	return items, nil
}

// ListGroupMembersFresh is used only by import commit. It intentionally
// bypasses both connector caches and asks the broker adapter for a fresh 3213
// read (the adapter also refreshes 3222 to catch rename/ambiguity changes).
func (r *futuWatchlistReader) ListGroupMembersFresh(ctx context.Context, remoteGroupID string) ([]watchlist.RemoteMember, error) {
	name, err := futuRemoteGroupName(remoteGroupID)
	if err != nil {
		return nil, err
	}
	reader, err := r.reader()
	if err != nil {
		return nil, err
	}
	fresh, ok := reader.(broker.FreshWatchlistGroupReader)
	if !ok {
		return nil, fmt.Errorf("%w: Futu fresh watchlist reads are unavailable", watchlist.ErrUnavailable)
	}
	securities, err := fresh.ListWatchlistGroupSecuritiesFresh(ctx, name)
	if err != nil {
		return nil, err
	}
	return remoteMembersFromBroker(securities), nil
}

func remoteMembersFromBroker(securities []broker.WatchlistSecurity) []watchlist.RemoteMember {
	items := make([]watchlist.RemoteMember, 0, len(securities))
	for _, security := range securities {
		item := watchlist.RemoteMember{InstrumentID: security.InstrumentID}
		if security.Name != nil {
			item.Name = strings.TrimSpace(*security.Name)
		}
		if security.SecurityType != nil {
			item.Type = strings.TrimSpace(*security.SecurityType)
		}
		if security.BrokerCode != nil {
			item.BrokerCode = strings.TrimSpace(*security.BrokerCode)
		}
		if security.BrokerSecurityID != nil {
			item.SecurityID = strings.TrimSpace(*security.BrokerSecurityID)
		}
		items = append(items, item)
	}
	return items
}

func (r *futuWatchlistReader) clock() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now()
}

func normalizeFutuWatchlistGroups(groups []broker.WatchlistGroup, observedAt time.Time) []watchlist.RemoteGroup {
	counts := make(map[string]int, len(groups))
	for _, group := range groups {
		counts[watchlistRemoteNameKey(group.Name)]++
	}
	ordinals := make(map[string]int, len(groups))
	result := make([]watchlist.RemoteGroup, 0, len(groups))
	for _, group := range groups {
		name := strings.TrimSpace(group.Name)
		groupType := strings.ToLower(strings.TrimSpace(group.Type))
		key := watchlistRemoteNameKey(name)
		ordinal := ordinals[key]
		ordinals[key]++
		result = append(result, watchlist.RemoteGroup{
			SourceID: futuWatchlistSourceID, RemoteGroupID: futuRemoteGroupID(groupType, name, ordinal, counts[key]),
			Name: name, Type: groupType, Ambiguous: group.Ambiguous || counts[key] > 1, ObservedAt: observedAt,
		})
	}
	return result
}

func watchlistRemoteNameKey(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func futuRemoteGroupID(groupType, name string, ordinal, count int) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSpace(groupType) + "\x00" + strings.TrimSpace(name)))
	if count > 1 {
		return "futu-group:" + payload + "." + strconv.Itoa(ordinal+1)
	}
	return "futu-group:" + payload
}

func futuRemoteGroupName(remoteGroupID string) (string, error) {
	encoded := strings.TrimPrefix(strings.TrimSpace(remoteGroupID), "futu-group:")
	if encoded == remoteGroupID || encoded == "" {
		return "", fmt.Errorf("%w: invalid Futu remote group id", watchlist.ErrValidation)
	}
	if dot := strings.LastIndexByte(encoded, '.'); dot >= 0 {
		encoded = encoded[:dot]
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: invalid Futu remote group id", watchlist.ErrValidation)
	}
	parts := strings.SplitN(string(payload), "\x00", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", fmt.Errorf("%w: invalid Futu remote group id", watchlist.ErrValidation)
	}
	return strings.TrimSpace(parts[1]), nil
}

func remoteGroupByID(groups []watchlist.RemoteGroup, remoteGroupID string) (watchlist.RemoteGroup, bool) {
	for _, group := range groups {
		if group.RemoteGroupID == remoteGroupID {
			return group, true
		}
	}
	return watchlist.RemoteGroup{}, false
}

type batchSnapshotProvider func() (broker.BatchSnapshotSource, error)

type futuWatchlistSnapshotSource struct {
	source batchSnapshotProvider
	now    func() time.Time
	gate   fixedWindowCallGate
}

func newFutuWatchlistSnapshotSource(provider batchSnapshotProvider) *futuWatchlistSnapshotSource {
	return &futuWatchlistSnapshotSource{
		source: provider, now: time.Now,
		gate: fixedWindowCallGate{limit: futuSnapshotReadLimit, window: futuSnapshotWindow},
	}
}

func (s *futuWatchlistSnapshotSource) BatchSnapshots(ctx context.Context, instrumentIDs []string) ([]watchlist.Quote, []watchlist.QuoteError, error) {
	if s == nil || s.source == nil {
		return nil, nil, watchlist.ErrUnavailable
	}
	source, err := s.source()
	if err != nil {
		return nil, nil, err
	}
	byMarket := make(map[string][]string)
	for _, instrumentID := range instrumentIDs {
		byMarket[marketpkg.SymbolMarket(instrumentID)] = append(byMarket[marketpkg.SymbolMarket(instrumentID)], instrumentID)
	}
	markets := make([]string, 0, len(byMarket))
	for market := range byMarket {
		markets = append(markets, market)
	}
	sort.Strings(markets)
	items := make(map[string]broker.SecuritySnapshotItem, len(instrumentIDs))
	itemErrors := make([]watchlist.QuoteError, 0)
	for _, market := range markets {
		batchSize := futuSnapshotBatch
		if market == "HK" {
			batchSize = futuHKSnapshotBatch
		}
		ids := byMarket[market]
		for start := 0; start < len(ids); start += batchSize {
			end := min(start+batchSize, len(ids))
			batchItems, batchErrors := queryFutuSnapshotBatch(ctx, source, ids[start:end], s.allowSnapshotCall)
			maps.Copy(items, batchItems)
			itemErrors = append(itemErrors, batchErrors...)
		}
	}
	now := time.Now().UTC()
	if s.now != nil {
		now = s.now().UTC()
	}
	quotes := make([]watchlist.Quote, 0, len(items))
	for _, instrumentID := range instrumentIDs {
		item, ok := items[instrumentID]
		if !ok {
			continue
		}
		quotes = append(quotes, watchlistQuoteFromBrokerSnapshot(instrumentID, item, now))
	}
	return quotes, itemErrors, nil
}

func (s *futuWatchlistSnapshotSource) allowSnapshotCall() bool {
	now := time.Now()
	if s != nil && s.now != nil {
		now = s.now()
	}
	return s.gate.allow(now)
}

func queryFutuSnapshotBatch(ctx context.Context, source broker.BatchSnapshotSource, instrumentIDs []string, allow func() bool) (map[string]broker.SecuritySnapshotItem, []watchlist.QuoteError) {
	if allow != nil && !allow() {
		itemErrors := make([]watchlist.QuoteError, 0, len(instrumentIDs))
		for _, instrumentID := range instrumentIDs {
			itemErrors = append(itemErrors, watchlist.QuoteError{
				InstrumentID: instrumentID, Code: "SNAPSHOT_RATE_LIMITED", Message: "Futu SecuritySnapshot rate limit is exhausted; retry after 30 seconds",
			})
		}
		return nil, itemErrors
	}
	result, err := source.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{Symbols: instrumentIDs})
	if err != nil {
		if len(instrumentIDs) == 1 || ctx.Err() != nil || !broker.IsSymbolScopedSnapshotError(err) {
			itemErrors := make([]watchlist.QuoteError, 0, len(instrumentIDs))
			for _, instrumentID := range instrumentIDs {
				itemErrors = append(itemErrors, watchlist.QuoteError{
					InstrumentID: instrumentID, Code: "SNAPSHOT_UNAVAILABLE", Message: err.Error(),
				})
			}
			return nil, itemErrors
		}
		middle := len(instrumentIDs) / 2
		leftItems, leftErrors := queryFutuSnapshotBatch(ctx, source, instrumentIDs[:middle], allow)
		rightItems, rightErrors := queryFutuSnapshotBatch(ctx, source, instrumentIDs[middle:], allow)
		for id, item := range rightItems {
			if leftItems == nil {
				leftItems = make(map[string]broker.SecuritySnapshotItem)
			}
			leftItems[id] = item
		}
		return leftItems, append(leftErrors, rightErrors...)
	}
	items := make(map[string]broker.SecuritySnapshotItem, len(instrumentIDs))
	if result != nil {
		for _, item := range result.Snapshots {
			canonical, parseErr := watchlist.NormalizeInstrumentID(item.Symbol)
			if parseErr == nil {
				items[canonical] = item
			}
		}
	}
	errorsByID := make([]watchlist.QuoteError, 0)
	for _, instrumentID := range instrumentIDs {
		if _, ok := items[instrumentID]; !ok {
			errorsByID = append(errorsByID, watchlist.QuoteError{InstrumentID: instrumentID, Code: "SNAPSHOT_NOT_RETURNED", Message: "Futu OpenD did not return a snapshot for this instrument"})
		}
	}
	return items, errorsByID
}

type fixedWindowCallGate struct {
	mu     sync.Mutex
	calls  []time.Time
	limit  int
	window time.Duration
}

func (g *fixedWindowCallGate) allow(now time.Time) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	cutoff := now.Add(-g.window)
	first := 0
	for first < len(g.calls) && !g.calls[first].After(cutoff) {
		first++
	}
	if first > 0 {
		g.calls = append(g.calls[:0], g.calls[first:]...)
	}
	if g.limit <= 0 || g.window <= 0 || len(g.calls) >= g.limit {
		return false
	}
	g.calls = append(g.calls, now)
	return true
}

func watchlistQuoteFromBrokerSnapshot(instrumentID string, item broker.SecuritySnapshotItem, fallbackObservedAt time.Time) watchlist.Quote {
	observedAt := item.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = fallbackObservedAt
	}
	session := ""
	if item.Session != nil {
		session = strings.TrimSpace(*item.Session)
	}
	if session == "" {
		session = string(marketpkg.ClassifySession(instrumentID, observedAt))
	}
	updateTime := parseWatchlistSnapshotTime(instrumentID, item.UpdateTime)
	price := item.LastPrice
	switch marketpkg.Session(session) {
	case marketpkg.SessionPre:
		price = preferredExtendedPrice(item.PreMarket, price)
	case marketpkg.SessionAfter:
		price = preferredExtendedPrice(item.AfterMarket, price)
	case marketpkg.SessionOvernight:
		price = preferredExtendedPrice(item.Overnight, price)
	}
	quote := watchlist.Quote{
		InstrumentID: instrumentID, Name: watchlistStringValue(item.Name), Type: watchlistStringValue(item.SecurityType),
		Source: "futu:security-snapshot", Price: cloneFloat(price), PreviousClose: cloneFloat(item.PreviousClose),
		Session: session, ObservedAt: observedAt, UpdateTime: updateTime,
		Extended: &watchlist.ExtendedQuote{
			Pre:       extendedQuoteBlock(item.PreMarket, observedAt, updateTime, item.PreviousClose),
			After:     extendedQuoteBlock(item.AfterMarket, observedAt, updateTime, item.PreviousClose),
			Overnight: extendedQuoteBlock(item.Overnight, observedAt, updateTime, item.PreviousClose),
		},
	}
	if quote.Extended.Pre == nil && quote.Extended.After == nil && quote.Extended.Overnight == nil {
		quote.Extended = nil
	}
	if price != nil && item.PreviousClose != nil {
		change := *price - *item.PreviousClose
		quote.Change = &change
		if *item.PreviousClose != 0 {
			percent := change / *item.PreviousClose * 100
			quote.ChangePercent = &percent
		}
	}
	return quote
}

func watchlistStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func parseWatchlistSnapshotTime(instrumentID string, value *string) *time.Time {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	raw := strings.TrimSpace(*value)
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		parsed = parsed.UTC()
		return &parsed
	}
	profile, ok := marketpkg.ProfileForSymbol(instrumentID)
	if !ok || profile.Location == nil {
		// OpenD's legacy updateTime has no timezone suffix. If JFTrade has no
		// canonical market timezone, omitting it is safer than labelling the
		// broker-local wall clock as UTC.
		return nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, raw, profile.Location); err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}

func preferredExtendedPrice(block *broker.ExtendedSessionSnapshot, fallback *float64) *float64 {
	if block != nil && block.Price != nil {
		return block.Price
	}
	return fallback
}

func extendedQuoteBlock(value *broker.ExtendedSessionSnapshot, observedAt time.Time, updateTime *time.Time, previousClose *float64) *watchlist.QuoteBlock {
	if value == nil {
		return nil
	}
	block := &watchlist.QuoteBlock{
		Price: cloneFloat(value.Price), Change: cloneFloat(value.Change), ChangePercent: cloneFloat(value.ChangeRate),
		ObservedAt: observedAt, UpdateTime: updateTime,
	}
	if block.Change == nil && value.Price != nil && previousClose != nil {
		change := *value.Price - *previousClose
		block.Change = &change
	}
	if block.ChangePercent == nil && block.Change != nil && previousClose != nil && *previousClose != 0 {
		percent := *block.Change / *previousClose * 100
		block.ChangePercent = &percent
	}
	return block
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
