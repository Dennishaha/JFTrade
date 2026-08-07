package futu

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/watchlist"
	"github.com/jftrade/jftrade-main/pkg/broker"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
)

type fakeBrokerWatchlistReader struct {
	groups      []broker.WatchlistGroup
	members     map[string][]broker.WatchlistSecurity
	groupCalls  int
	memberCalls int
	freshCalls  int
}

func (f *fakeBrokerWatchlistReader) ListWatchlistGroups(context.Context) ([]broker.WatchlistGroup, error) {
	f.groupCalls++
	return append([]broker.WatchlistGroup(nil), f.groups...), nil
}

func (f *fakeBrokerWatchlistReader) ListWatchlistGroupSecurities(_ context.Context, name string) ([]broker.WatchlistSecurity, error) {
	f.memberCalls++
	return append([]broker.WatchlistSecurity(nil), f.members[name]...), nil
}

func (f *fakeBrokerWatchlistReader) ListWatchlistGroupsFresh(ctx context.Context) ([]broker.WatchlistGroup, error) {
	return f.ListWatchlistGroups(ctx)
}

func (f *fakeBrokerWatchlistReader) ListWatchlistGroupSecuritiesFresh(_ context.Context, name string) ([]broker.WatchlistSecurity, error) {
	f.freshCalls++
	return append([]broker.WatchlistSecurity(nil), f.members[name]...), nil
}

func TestFutuWatchlistReaderMarksDuplicateNamesAmbiguousAndCachesReads(t *testing.T) {
	name := "Tencent"
	fake := &fakeBrokerWatchlistReader{
		groups:  []broker.WatchlistGroup{{Name: "科技", Type: "custom"}, {Name: " 科技 ", Type: "system"}, {Name: "港股", Type: "system"}},
		members: map[string][]broker.WatchlistSecurity{"港股": {{InstrumentID: "HK.00700", Name: &name}}},
	}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	reader := newFutuWatchlistReader(func() (broker.WatchlistGroupReader, error) { return fake, nil }, func() bool { return true })
	reader.now = func() time.Time { return now }

	groups, err := reader.ListGroups(t.Context())
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	if len(groups) != 3 || !groups[0].Ambiguous || !groups[1].Ambiguous || groups[2].Ambiguous {
		t.Fatalf("groups = %#v", groups)
	}
	if groups[0].RemoteGroupID == groups[1].RemoteGroupID || groups[2].RemoteGroupID == "" {
		t.Fatalf("remote ids are not stable and unique: %#v", groups)
	}
	if _, err := reader.ListGroupMembers(t.Context(), groups[0].RemoteGroupID); err == nil {
		t.Fatal("ambiguous group was importable")
	}
	members, err := reader.ListGroupMembers(t.Context(), groups[2].RemoteGroupID)
	if err != nil || len(members) != 1 || members[0].InstrumentID != "HK.00700" {
		t.Fatalf("members = %#v err=%v", members, err)
	}
	if _, err := reader.ListGroupMembers(t.Context(), groups[2].RemoteGroupID); err != nil {
		t.Fatalf("cached ListGroupMembers: %v", err)
	}
	if fake.groupCalls != 1 || fake.memberCalls != 1 {
		t.Fatalf("broker calls groups=%d members=%d, want 1/1", fake.groupCalls, fake.memberCalls)
	}
	freshMembers, err := reader.ListGroupMembersFresh(t.Context(), groups[2].RemoteGroupID)
	if err != nil || len(freshMembers) != 1 || fake.freshCalls != 1 {
		t.Fatalf("fresh members=%#v calls=%d err=%v", freshMembers, fake.freshCalls, err)
	}
}

func TestRemoteMembersKeepBrokerCodeAndSecurityIDAsSeparateAliases(t *testing.T) {
	brokerCode, securityID := "00700", "700"
	members := remoteMembersFromBroker([]broker.WatchlistSecurity{{
		InstrumentID: "HK.00700", BrokerCode: &brokerCode, BrokerSecurityID: &securityID,
	}})
	if len(members) != 1 || members[0].BrokerCode != brokerCode || members[0].SecurityID != securityID || members[0].BrokerCode == members[0].SecurityID {
		t.Fatalf("members = %#v", members)
	}
}

type recordingBatchSnapshotSource struct {
	mu         sync.Mutex
	batches    [][]string
	failID     string
	failMarket string
	marketErr  error
	err        error
}

func (f *recordingBatchSnapshotSource) QuerySecuritySnapshot(_ context.Context, query broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	f.mu.Lock()
	f.batches = append(f.batches, append([]string(nil), query.Symbols...))
	f.mu.Unlock()
	if f.failID != "" && slices.Contains(query.Symbols, f.failID) {
		return nil, broker.NewSymbolScopedSnapshotError(fmt.Errorf("permission denied for %s", f.failID))
	}
	if f.failMarket != "" {
		if slices.ContainsFunc(query.Symbols, func(symbol string) bool {
			return marketpkg.SymbolMarket(symbol) == f.failMarket
		}) {
			return nil, f.marketErr
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	items := make([]broker.SecuritySnapshotItem, 0, len(query.Symbols))
	for _, symbol := range query.Symbols {
		price, previous := 101.0, 100.0
		items = append(items, broker.SecuritySnapshotItem{Symbol: symbol, LastPrice: &price, PreviousClose: &previous, ObservedAt: time.Date(2026, 7, 11, 4, 0, 0, 0, time.UTC)})
	}
	return &broker.SecuritySnapshotResult{Snapshots: items}, nil
}

type symbolAvailabilitySnapshotSource struct {
	mu          sync.Mutex
	batches     [][]string
	unavailable map[string]error
}

type fallbackBatchSnapshotSource struct {
	*recordingBatchSnapshotSource
	mu            sync.Mutex
	fallbackCalls [][]string
	fallbackItems map[string]broker.SecuritySnapshotItem
	fallbackErr   error
}

func (f *fallbackBatchSnapshotSource) QuerySnapshotFallback(_ context.Context, query broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	f.mu.Lock()
	f.fallbackCalls = append(f.fallbackCalls, append([]string(nil), query.Symbols...))
	f.mu.Unlock()
	if f.fallbackErr != nil {
		return nil, f.fallbackErr
	}
	items := make([]broker.SecuritySnapshotItem, 0, len(query.Symbols))
	for _, symbol := range query.Symbols {
		if item, ok := f.fallbackItems[symbol]; ok {
			items = append(items, item)
		}
	}
	return &broker.SecuritySnapshotResult{Snapshots: items}, nil
}

func (f *symbolAvailabilitySnapshotSource) QuerySecuritySnapshot(_ context.Context, query broker.SecuritySnapshotQuery) (*broker.SecuritySnapshotResult, error) {
	f.mu.Lock()
	f.batches = append(f.batches, append([]string(nil), query.Symbols...))
	f.mu.Unlock()
	for _, symbol := range query.Symbols {
		if err := f.unavailable[symbol]; err != nil {
			return nil, broker.NewSymbolScopedSnapshotError(err)
		}
	}
	items := make([]broker.SecuritySnapshotItem, 0, len(query.Symbols))
	for _, symbol := range query.Symbols {
		price, previous := 101.0, 100.0
		items = append(items, broker.SecuritySnapshotItem{Symbol: symbol, LastPrice: &price, PreviousClose: &previous})
	}
	return &broker.SecuritySnapshotResult{Snapshots: items}, nil
}

func TestFutuWatchlistSnapshotDoesNotSplitGlobalOrCanceledFailures(t *testing.T) {
	ids := make([]string, 400)
	for index := range ids {
		ids[index] = fmt.Sprintf("US.TEST%d", index)
	}
	tests := []struct {
		name        string
		context     func() context.Context
		err         error
		wantMessage string
	}{
		{name: "service failure", context: context.Background, err: errors.New("OpenD quote service unavailable")},
		{name: "rate limited", context: context.Background, err: broker.NewSnapshotRateLimitError(time.Second, nil)},
		{
			name:        "permission denied",
			context:     context.Background,
			err:         errors.New("无权限获取SH.000001的行情，请检查A股市场指数行情权限"),
			wantMessage: "无权限获取SH.000001的行情，请检查A股市场指数行情权限",
		},
		{name: "canceled", context: func() context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}, err: context.Canceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &recordingBatchSnapshotSource{err: test.err}
			source := newFutuWatchlistSnapshotSource(func() (broker.BatchSnapshotSource, error) { return fake, nil })
			quotes, itemErrors, err := source.BatchSnapshots(test.context(), ids)
			if err != nil || len(quotes) != 0 || len(itemErrors) != len(ids) {
				t.Fatalf("quotes=%d errors=%d err=%v", len(quotes), len(itemErrors), err)
			}
			if len(fake.batches) != 1 {
				t.Fatalf("snapshot calls = %d, want 1", len(fake.batches))
			}
			if test.wantMessage != "" {
				for _, itemError := range itemErrors {
					if itemError.Message != test.wantMessage {
						t.Fatalf("snapshot error message = %q, want %q", itemError.Message, test.wantMessage)
					}
				}
			}
		})
	}
}

func TestFutuWatchlistSnapshotUsesDelayedFallbackWhenSubscriptionQuotaIsFull(t *testing.T) {
	quotaErr := broker.NewSnapshotAvailabilityError(
		broker.SnapshotAvailabilityQuota,
		errors.New("OpenD subscription is full"),
	)
	shPrice, shPrevious := 1500.0, 1490.0
	usPrice, usPrevious := 210.0, 200.0
	provider := &fallbackBatchSnapshotSource{
		recordingBatchSnapshotSource: &recordingBatchSnapshotSource{err: quotaErr},
		fallbackItems: map[string]broker.SecuritySnapshotItem{
			"SH.600519": {
				Symbol: "SH.600519", Source: "futu:stock-screen-delayed", LastPrice: &shPrice, PreviousClose: &shPrevious,
			},
			"US.AAPL": {
				Symbol: "US.AAPL", Source: "futu:stock-screen-delayed", LastPrice: &usPrice, PreviousClose: &usPrevious,
			},
		},
	}
	source := newFutuWatchlistSnapshotSource(func() (broker.BatchSnapshotSource, error) { return provider, nil })

	quotes, itemErrors, err := source.BatchSnapshots(t.Context(), []string{"SH.600519", "US.AAPL"})
	if err != nil {
		t.Fatalf("BatchSnapshots: %v", err)
	}
	if len(itemErrors) != 0 || len(quotes) != 2 {
		t.Fatalf("quotes=%#v errors=%#v", quotes, itemErrors)
	}
	if !sameSnapshotBatches(provider.batches, [][]string{{"SH.600519"}, {"US.AAPL"}}) {
		t.Fatalf("primary snapshot batches = %#v", provider.batches)
	}
	if !sameSnapshotBatches(provider.fallbackCalls, [][]string{{"SH.600519"}, {"US.AAPL"}}) {
		t.Fatalf("fallback snapshot batches = %#v", provider.fallbackCalls)
	}
	for _, quote := range quotes {
		if quote.Source != "futu:stock-screen-delayed" || quote.Change == nil || quote.ChangePercent == nil {
			t.Fatalf("delayed fallback quote = %#v", quote)
		}
	}
}

func sameSnapshotBatches(left, right [][]string) bool {
	return slices.EqualFunc(left, right, func(a, b []string) bool {
		return slices.Equal(a, b)
	})
}

func TestFutuWatchlistSnapshotUsesMarketSpecificChunksWithPerItemErrors(t *testing.T) {
	ids := make([]string, 0, 423)
	for index := 1; index <= 21; index++ {
		ids = append(ids, fmt.Sprintf("HK.%05d", index))
	}
	ids = append(ids, "SH.600519", "SZ.000001")
	for index := 1; index <= 401; index++ {
		ids = append(ids, fmt.Sprintf("US.TEST%d", index))
	}
	ids = append(ids, "US.BAD")
	fake := &recordingBatchSnapshotSource{failID: "US.BAD"}
	source := newFutuWatchlistSnapshotSource(func() (broker.BatchSnapshotSource, error) { return fake, nil })
	source.now = func() time.Time { return time.Date(2026, 7, 11, 4, 0, 0, 0, time.UTC) }

	quotes, itemErrors, err := source.BatchSnapshots(t.Context(), ids)
	if err != nil {
		t.Fatalf("BatchSnapshots: %v", err)
	}
	if len(quotes) != len(ids)-1 || len(itemErrors) != 1 || itemErrors[0].InstrumentID != "US.BAD" {
		t.Fatalf("quotes=%d errors=%#v", len(quotes), itemErrors)
	}
	markets := make(map[string]struct{})
	for _, batch := range fake.batches {
		if len(batch) == 0 {
			t.Fatal("empty snapshot batch")
		}
		market := marketpkg.SymbolMarket(batch[0])
		markets[market] = struct{}{}
		for _, symbol := range batch[1:] {
			if marketpkg.SymbolMarket(symbol) != market {
				t.Fatalf("mixed market batch = %#v", batch)
			}
		}
		if market == "HK" && len(batch) > futuHKSnapshotBatch {
			t.Fatalf("HK batch size = %d, want <= %d", len(batch), futuHKSnapshotBatch)
		} else if len(batch) > futuSnapshotBatch {
			t.Fatalf("%s batch size = %d, want <= %d", market, len(batch), futuSnapshotBatch)
		}
	}
	for _, market := range []string{"HK", "SH", "SZ", "US"} {
		if _, ok := markets[market]; !ok {
			t.Fatalf("market %s was not queried: %#v", market, fake.batches)
		}
	}
	if quotes[0].Change == nil || *quotes[0].Change != 1 || quotes[0].ChangePercent == nil || *quotes[0].ChangePercent != 1 {
		t.Fatalf("quote change calculation = %#v", quotes[0])
	}
}

func TestFutuWatchlistSnapshotIsolatesMarketPermissionFailures(t *testing.T) {
	permissionErr := errors.New("无权限获取沪深市场行情")
	tests := []struct {
		name         string
		failedMarket string
		failedID     string
	}{
		{name: "Shanghai", failedMarket: "SH", failedID: "SH.000001"},
		{name: "Shenzhen", failedMarket: "SZ", failedID: "SZ.000001"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ids := []string{test.failedID, "US.AAPL", "US.BABA"}
			fake := &recordingBatchSnapshotSource{failMarket: test.failedMarket, marketErr: permissionErr}
			source := newFutuWatchlistSnapshotSource(func() (broker.BatchSnapshotSource, error) { return fake, nil })

			quotes, itemErrors, err := source.BatchSnapshots(t.Context(), ids)
			if err != nil {
				t.Fatalf("BatchSnapshots: %v", err)
			}
			quoteIDs := make([]string, 0, len(quotes))
			for _, quote := range quotes {
				quoteIDs = append(quoteIDs, quote.InstrumentID)
			}
			if !slices.Equal(quoteIDs, []string{"US.AAPL", "US.BABA"}) {
				t.Fatalf("quote IDs = %#v", quoteIDs)
			}
			if len(itemErrors) != 1 || itemErrors[0].InstrumentID != test.failedID || itemErrors[0].Message != permissionErr.Error() {
				t.Fatalf("item errors = %#v", itemErrors)
			}
			for _, batch := range fake.batches {
				for _, symbol := range batch[1:] {
					if marketpkg.SymbolMarket(symbol) != marketpkg.SymbolMarket(batch[0]) {
						t.Fatalf("mixed market batch = %#v", batch)
					}
				}
			}
		})
	}
}

func TestFutuWatchlistSnapshotIsolatesUnknownAndOTCSymbolErrors(t *testing.T) {
	ids := []string{"US.AAPL", "US.BBKCF", "US.COIN", "US.KXIAY", "US.MSFT"}
	unknown := errors.New("opend GetSecuritySnapshot retType=-1 errCode=0 retMsg=未知股票 BBKCF")
	otc := errors.New("opend GetSecuritySnapshot retType=-1 errCode=0 retMsg=暂不提供美股 OTC 市场行情 KXIAY")
	reader := &symbolAvailabilitySnapshotSource{unavailable: map[string]error{
		"US.BBKCF": unknown,
		"US.KXIAY": otc,
	}}
	source := newFutuWatchlistSnapshotSource(func() (broker.BatchSnapshotSource, error) { return reader, nil })

	quotes, itemErrors, err := source.BatchSnapshots(t.Context(), ids)
	if err != nil {
		t.Fatalf("BatchSnapshots: %v", err)
	}
	quoteIDs := make([]string, 0, len(quotes))
	for _, quote := range quotes {
		quoteIDs = append(quoteIDs, quote.InstrumentID)
	}
	if !slices.Equal(quoteIDs, []string{"US.AAPL", "US.COIN", "US.MSFT"}) {
		t.Fatalf("quote IDs = %#v", quoteIDs)
	}
	errorsByID := make(map[string]watchlist.QuoteError, len(itemErrors))
	for _, itemError := range itemErrors {
		errorsByID[itemError.InstrumentID] = itemError
	}
	if len(itemErrors) != 2 || len(errorsByID) != 2 ||
		errorsByID["US.BBKCF"].Message != unknown.Error() || errorsByID["US.KXIAY"].Message != otc.Error() {
		t.Fatalf("per-symbol errors = %#v", errorsByID)
	}
	for _, id := range quoteIDs {
		if _, found := errorsByID[id]; found {
			t.Fatalf("successful symbol %s inherited an unrelated error", id)
		}
	}
	if len(reader.batches) <= len(ids) || !slices.Equal(reader.batches[0], ids) {
		t.Fatalf("symbol-scoped failures were not isolated: %#v", reader.batches)
	}
}

func TestWatchlistQuotePreservesSnapshotDisplayMetadataAndAvoidsUnknownTimezoneGuess(t *testing.T) {
	name, securityType := "DBS Group", "SecurityType_Eqty"
	updateTime := "2026-07-11 15:04:05"
	price, volume, turnover := 42.5, 1234.0, 5678.0
	quote := watchlistQuoteFromBrokerSnapshot("SG.D05", broker.SecuritySnapshotItem{
		Symbol: "SG.D05", Name: &name, SecurityType: &securityType,
		LastPrice: &price, Volume: &volume, Turnover: &turnover, UpdateTime: &updateTime,
		Fund: &broker.FundSnapshotData{AssetClass: "Stock"},
	}, time.Date(2026, 7, 11, 7, 4, 6, 0, time.UTC))
	if quote.Name != name || quote.Type != securityType {
		t.Fatalf("quote metadata = %#v", quote)
	}
	if quote.Volume == nil || *quote.Volume != volume ||
		quote.Turnover == nil || *quote.Turnover != turnover ||
		quote.AssetClass != "Stock" {
		t.Fatalf("quote research fields = %#v", quote)
	}
	if quote.UpdateTime != nil {
		t.Fatalf("timezone-less unsupported-market update time must be omitted, got %v", quote.UpdateTime)
	}
}

func TestWatchlistQuoteSelectsExtendedSessionPriceAndChange(t *testing.T) {
	regular, previous := 100.0, 90.0
	tests := []struct {
		session string
		price   float64
		apply   func(*broker.SecuritySnapshotItem, *broker.ExtendedSessionSnapshot)
	}{
		{session: "pre", price: 91, apply: func(item *broker.SecuritySnapshotItem, block *broker.ExtendedSessionSnapshot) { item.PreMarket = block }},
		{session: "after", price: 92, apply: func(item *broker.SecuritySnapshotItem, block *broker.ExtendedSessionSnapshot) {
			item.AfterMarket = block
		}},
		{session: "overnight", price: 93, apply: func(item *broker.SecuritySnapshotItem, block *broker.ExtendedSessionSnapshot) { item.Overnight = block }},
	}
	for _, test := range tests {
		t.Run(test.session, func(t *testing.T) {
			item := broker.SecuritySnapshotItem{
				Symbol: "US.AAPL", LastPrice: &regular, PreviousClose: &previous, Session: &test.session,
			}
			test.apply(&item, &broker.ExtendedSessionSnapshot{Price: &test.price})
			quote := watchlistQuoteFromBrokerSnapshot("US.AAPL", item, time.Now().UTC())
			if quote.Price == nil || *quote.Price != test.price || quote.Change == nil || *quote.Change != test.price-previous || quote.ChangePercent == nil {
				t.Fatalf("quote = %#v", quote)
			}
		})
	}
}

func TestFutuWatchlistSourceIdentityDoesNotUseTradingAccount(t *testing.T) {
	reader := newFutuWatchlistReader(nil, func() bool { return false })
	source, err := reader.Source(context.Background())
	if err != nil || source.ID != SourceID || source.Status != "disabled" {
		t.Fatalf("source = %#v err=%v", source, err)
	}
	if source.ID == "" || source.ID == "account" {
		t.Fatal("watchlist source identity unexpectedly depends on an account")
	}
	if !watchlist.IsConflict(watchlist.ErrStalePreview) {
		t.Fatal("stale preview must be classified as conflict")
	}
}

func TestFutuWatchlistSourceReportsUnavailableRuntimeBeforeDiscovery(t *testing.T) {
	reader := newFutuWatchlistReader(
		func() (broker.WatchlistGroupReader, error) { return nil, errors.New("OpenD runtime is unavailable") },
		func() bool { return true },
	)
	source, err := reader.Source(t.Context())
	if err != nil || source.Status != "unavailable" || source.Error == "" {
		t.Fatalf("source = %#v err=%v", source, err)
	}
}

func TestFutuWatchlistSourceReportsFailedOpenDProbe(t *testing.T) {
	reader := newFutuWatchlistReader(
		func() (broker.WatchlistGroupReader, error) { return &fakeBrokerWatchlistReader{}, nil },
		func() bool { return true },
		func(context.Context) error { return errors.New("connection refused") },
	)
	source, err := reader.Source(t.Context())
	if err != nil || source.Status != "unavailable" || source.Error != "connection refused" {
		t.Fatalf("source = %#v err=%v", source, err)
	}
}
