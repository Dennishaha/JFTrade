package futu

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
)

func TestSecuritySnapshotCoordinatorCachesClonesAndUsesMarketBatches(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	coordinator := newSecuritySnapshotCoordinator()
	coordinator.now = func() time.Time { return now }
	var batches [][]string
	fetch := func(_ context.Context, symbols []string) (map[string]*qotgetsecuritysnapshotpb.Snapshot, error) {
		batches = append(batches, append([]string(nil), symbols...))
		return testSecuritySnapshotMap(t, symbols), nil
	}

	symbols := []string{"US.AAPL", "HK.00700", "SH.600519", "SZ.000001", "HK.00941", "US.AAPL"}
	first, err := coordinator.query(t.Context(), symbols, fetch)
	if err != nil || len(first) != 5 || len(batches) != 2 {
		t.Fatalf("first query snapshots=%d batches=%#v err=%v", len(first), batches, err)
	}
	if !slices.Equal(batches[0], []string{"HK.00700", "HK.00941"}) ||
		!slices.Equal(batches[1], []string{"SH.600519", "SZ.000001", "US.AAPL"}) {
		t.Fatalf("snapshot batches = %#v", batches)
	}
	first["US.AAPL"].Basic.Name = new("mutated")
	second, err := coordinator.query(t.Context(), symbols, fetch)
	if err != nil || len(batches) != 2 || second["US.AAPL"].GetBasic().GetName() == "mutated" {
		t.Fatalf("cached query batches=%d name=%q err=%v", len(batches), second["US.AAPL"].GetBasic().GetName(), err)
	}

	now = now.Add(securitySnapshotCacheTTL)
	if _, err := coordinator.query(t.Context(), symbols, fetch); err != nil || len(batches) != 4 {
		t.Fatalf("expired cache batches=%d err=%v", len(batches), err)
	}
}

func TestSecuritySnapshotCoordinatorCoalescesConcurrentRequests(t *testing.T) {
	coordinator := newSecuritySnapshotCoordinator()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	fetch := func(_ context.Context, symbols []string) (map[string]*qotgetsecuritysnapshotpb.Snapshot, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return testSecuritySnapshotMap(t, symbols), nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, err := coordinator.query(context.Background(), []string{"US.AAPL"}, fetch)
			errs <- err
		})
	}
	<-started
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("coalesced query error = %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("snapshot calls = %d, want 1", calls.Load())
	}
}

func TestSecuritySnapshotCoordinatorEnforcesSlidingBudgetAndDoesNotCacheFailures(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	coordinator := newSecuritySnapshotCoordinator()
	coordinator.now = func() time.Time { return now }
	coordinator.callLimit = 2
	var calls int
	fetch := func(_ context.Context, symbols []string) (map[string]*qotgetsecuritysnapshotpb.Snapshot, error) {
		calls++
		if symbols[0] == "US.FAIL" && calls == 1 {
			return nil, errors.New("temporary failure")
		}
		return testSecuritySnapshotMap(t, symbols), nil
	}

	if _, err := coordinator.query(t.Context(), []string{"US.FAIL"}, fetch); err == nil {
		t.Fatal("first failed query succeeded")
	}
	if _, err := coordinator.query(t.Context(), []string{"US.FAIL"}, fetch); err != nil {
		t.Fatalf("failure was cached: %v", err)
	}
	_, err := coordinator.query(t.Context(), []string{"US.THIRD"}, fetch)
	if !errors.Is(err, broker.ErrSnapshotRateLimited) {
		t.Fatalf("third call error = %v", err)
	}
	retryAfter, ok := broker.SnapshotRetryAfter(err)
	if !ok || retryAfter != securitySnapshotCallWindow {
		t.Fatalf("retryAfter = %v, %t", retryAfter, ok)
	}
	if calls != 2 {
		t.Fatalf("rate-limited request reached OpenD: calls=%d", calls)
	}
	now = now.Add(securitySnapshotCallWindow + time.Millisecond)
	if _, err := coordinator.query(t.Context(), []string{"US.THIRD"}, fetch); err != nil || calls != 3 {
		t.Fatalf("released budget calls=%d err=%v", calls, err)
	}
}

func TestSecuritySnapshotCoordinatorClassifiesRemoteRateLimitAndHonorsCancellation(t *testing.T) {
	coordinator := newSecuritySnapshotCoordinator()
	_, err := coordinator.query(t.Context(), []string{"US.AAPL"}, func(context.Context, []string) (map[string]*qotgetsecuritysnapshotpb.Snapshot, error) {
		return nil, fmt.Errorf("获取市场快照频率太高，请求失败，每30秒最多60次")
	})
	if !errors.Is(err, broker.ErrSnapshotRateLimited) {
		t.Fatalf("remote rate-limit error = %v", err)
	}

	canceledCoordinator := newSecuritySnapshotCoordinator()
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	payload := testSecuritySnapshotMap(t, []string{"US.AAPL"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, queryErr := canceledCoordinator.query(ctx, []string{"US.AAPL"}, func(context.Context, []string) (map[string]*qotgetsecuritysnapshotpb.Snapshot, error) {
			defer close(finished)
			close(started)
			<-release
			return payload, nil
		})
		done <- queryErr
	}()
	<-started
	cancel()
	if queryErr := <-done; !errors.Is(queryErr, context.Canceled) {
		t.Fatalf("canceled query error = %v", queryErr)
	}
	close(release)
	<-finished
}

func TestSecuritySnapshotCoordinatorRejectsUnavailableAndInvalidInputs(t *testing.T) {
	var nilExchange *Exchange
	if _, err := nilExchange.querySecuritySnapshotList(t.Context(), []string{"US.AAPL"}); err == nil {
		t.Fatal("nil exchange query error = nil")
	}

	exchange := NewExchange("")
	if _, err := exchange.querySecuritySnapshotListDirect(t.Context(), []string{"BAD"}); err == nil {
		t.Fatal("invalid direct snapshot query error = nil")
	}
	empty, err := exchange.querySecuritySnapshotListDirect(t.Context(), nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty direct snapshot query = %#v, %v", empty, err)
	}
	server, connectedExchange := coverageMarginExchange(t)
	server.setSecuritySnapshots([]*qotgetsecuritysnapshotpb.Snapshot{testTencentSecuritySnapshot()})
	duplicates, err := connectedExchange.querySecuritySnapshotListDirect(
		t.Context(), []string{"HK.00700", " hk.00700 "},
	)
	if err != nil || len(duplicates) != 1 {
		t.Fatalf("duplicate direct snapshot query = %#v, %v", duplicates, err)
	}

	var nilCoordinator *securitySnapshotCoordinator
	if _, err := nilCoordinator.query(t.Context(), []string{"US.AAPL"}, func(context.Context, []string) (map[string]*qotgetsecuritysnapshotpb.Snapshot, error) {
		return nil, nil
	}); err == nil {
		t.Fatal("nil coordinator query error = nil")
	}
	coordinator := newSecuritySnapshotCoordinator()
	if _, err := coordinator.query(t.Context(), []string{"US.AAPL"}, nil); err == nil {
		t.Fatal("nil fetch query error = nil")
	}
}

func TestSecuritySnapshotCoordinatorHandlesEmptyAndUnexpectedResults(t *testing.T) {
	coordinator := newSecuritySnapshotCoordinator()
	coordinator.now = nil
	coordinator.store(nil)
	coordinator.store(map[string]*qotgetsecuritysnapshotpb.Snapshot{"US.AAPL": nil})
	if coordinator.clock().IsZero() {
		t.Fatal("default coordinator clock returned zero time")
	}
	if got := classifySecuritySnapshotError(nil); got != nil {
		t.Fatalf("classify nil = %v", got)
	}
	rateLimited := broker.NewSnapshotRateLimitError(time.Second, nil)
	if got := classifySecuritySnapshotError(rateLimited); got != rateLimited {
		t.Fatalf("classify existing rate limit = %v, want original", got)
	}
	if cloneSecuritySnapshot(nil) != nil {
		t.Fatal("clone nil snapshot returned a value")
	}

	started := make(chan struct{})
	release := make(chan struct{})
	flight := coordinator.flights.DoChan("US.AAPL", func() (any, error) {
		close(started)
		<-release
		return "unexpected", nil
	})
	<-started
	timer := time.AfterFunc(10*time.Millisecond, func() { close(release) })
	_, err := coordinator.fetchBatch(t.Context(), []string{"US.AAPL"}, func(context.Context, []string) (map[string]*qotgetsecuritysnapshotpb.Snapshot, error) {
		t.Fatal("coalesced fetch unexpectedly called")
		return nil, nil
	})
	if !timer.Stop() {
		<-flight
	} else {
		close(release)
		<-flight
	}
	if err == nil || err.Error() != "futu security snapshot coordinator returned an invalid result" {
		t.Fatalf("unexpected flight result error = %v", err)
	}
}

func testSecuritySnapshotMap(t *testing.T, symbols []string) map[string]*qotgetsecuritysnapshotpb.Snapshot {
	t.Helper()
	result := make(map[string]*qotgetsecuritysnapshotpb.Snapshot, len(symbols))
	for _, symbol := range symbols {
		security, canonical, err := futuSecurityFromSymbol(symbol)
		if err != nil {
			t.Fatalf("futuSecurityFromSymbol(%q): %v", symbol, err)
		}
		name := canonical
		result[canonical] = &qotgetsecuritysnapshotpb.Snapshot{
			Basic: &qotgetsecuritysnapshotpb.SnapshotBasicData{Security: security, Name: &name},
		}
	}
	return result
}

func resetSecuritySnapshotCoordinator(exchange *Exchange) {
	if exchange == nil {
		return
	}
	exchange.securitySnapshotCoordinatorMu.Lock()
	exchange.securitySnapshots = newSecuritySnapshotCoordinator()
	exchange.securitySnapshotCoordinatorMu.Unlock()
}
