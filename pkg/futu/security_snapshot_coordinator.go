package futu

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/broker"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
)

const (
	securitySnapshotCacheTTL       = 3 * time.Second
	securitySnapshotCallLimit      = 54
	securitySnapshotCallWindow     = 30 * time.Second
	securitySnapshotHKBatchSize    = 20
	securitySnapshotOtherBatchSize = 400
)

type securitySnapshotFetch func(context.Context, []string) (map[string]*qotgetsecuritysnapshotpb.Snapshot, error)

type cachedSecuritySnapshot struct {
	snapshot  *qotgetsecuritysnapshotpb.Snapshot
	expiresAt time.Time
}

type securitySnapshotCoordinator struct {
	mu         sync.Mutex
	cache      map[string]cachedSecuritySnapshot
	calls      []time.Time
	now        func() time.Time
	cacheTTL   time.Duration
	callLimit  int
	callWindow time.Duration
	flights    singleflight.Group
}

func newSecuritySnapshotCoordinator() *securitySnapshotCoordinator {
	return &securitySnapshotCoordinator{
		cache:      make(map[string]cachedSecuritySnapshot),
		now:        time.Now,
		cacheTTL:   securitySnapshotCacheTTL,
		callLimit:  securitySnapshotCallLimit,
		callWindow: securitySnapshotCallWindow,
	}
}

func (c *securitySnapshotCoordinator) query(
	ctx context.Context,
	symbols []string,
	fetch securitySnapshotFetch,
) (map[string]*qotgetsecuritysnapshotpb.Snapshot, error) {
	canonical, err := canonicalSecuritySnapshotSymbols(symbols)
	if err != nil {
		return nil, err
	}
	if len(canonical) == 0 {
		return map[string]*qotgetsecuritysnapshotpb.Snapshot{}, nil
	}
	if c == nil || fetch == nil {
		return nil, fmt.Errorf("futu security snapshot coordinator is unavailable")
	}

	result, missing := c.cached(canonical)
	for _, batch := range securitySnapshotBatches(missing) {
		fetched, fetchErr := c.fetchBatch(ctx, batch, fetch)
		if fetchErr != nil {
			return nil, fetchErr
		}
		for symbol, snapshot := range fetched {
			result[symbol] = cloneSecuritySnapshot(snapshot)
		}
	}
	return result, nil
}

func canonicalSecuritySnapshotSymbols(symbols []string) ([]string, error) {
	seen := make(map[string]struct{}, len(symbols))
	canonical := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		_, normalized, err := futuSecurityFromSymbol(symbol)
		if err != nil {
			return nil, broker.NewSymbolScopedSnapshotError(err)
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		canonical = append(canonical, normalized)
	}
	sort.Strings(canonical)
	return canonical, nil
}

func securitySnapshotBatches(symbols []string) [][]string {
	hk := make([]string, 0, len(symbols))
	other := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		if strings.HasPrefix(symbol, "HK.") {
			hk = append(hk, symbol)
		} else {
			other = append(other, symbol)
		}
	}
	batches := make([][]string, 0, 2)
	batches = appendSecuritySnapshotBatches(batches, hk, securitySnapshotHKBatchSize)
	batches = appendSecuritySnapshotBatches(batches, other, securitySnapshotOtherBatchSize)
	return batches
}

func appendSecuritySnapshotBatches(batches [][]string, symbols []string, size int) [][]string {
	for start := 0; start < len(symbols); start += size {
		end := min(start+size, len(symbols))
		batches = append(batches, append([]string(nil), symbols[start:end]...))
	}
	return batches
}

func (c *securitySnapshotCoordinator) cached(symbols []string) (map[string]*qotgetsecuritysnapshotpb.Snapshot, []string) {
	now := c.clock()
	result := make(map[string]*qotgetsecuritysnapshotpb.Snapshot, len(symbols))
	missing := make([]string, 0, len(symbols))
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, symbol := range symbols {
		entry, ok := c.cache[symbol]
		if !ok || !entry.expiresAt.After(now) || entry.snapshot == nil {
			delete(c.cache, symbol)
			missing = append(missing, symbol)
			continue
		}
		result[symbol] = cloneSecuritySnapshot(entry.snapshot)
	}
	return result, missing
}

func (c *securitySnapshotCoordinator) fetchBatch(
	ctx context.Context,
	batch []string,
	fetch securitySnapshotFetch,
) (map[string]*qotgetsecuritysnapshotpb.Snapshot, error) {
	key := strings.Join(batch, "\x00")
	flight := c.flights.DoChan(key, func() (any, error) {
		if retryAfter, ok := c.takeCall(); !ok {
			return nil, broker.NewSnapshotRateLimitError(retryAfter, nil)
		}
		snapshots, err := fetch(ctx, batch)
		if err != nil {
			return nil, classifySecuritySnapshotError(err)
		}
		c.store(snapshots)
		return cloneSecuritySnapshotMap(snapshots), nil
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case completed := <-flight:
		if completed.Err != nil {
			return nil, completed.Err
		}
		snapshots, ok := completed.Val.(map[string]*qotgetsecuritysnapshotpb.Snapshot)
		if !ok {
			return nil, fmt.Errorf("futu security snapshot coordinator returned an invalid result")
		}
		return cloneSecuritySnapshotMap(snapshots), nil
	}
}

func (c *securitySnapshotCoordinator) store(snapshots map[string]*qotgetsecuritysnapshotpb.Snapshot) {
	if len(snapshots) == 0 {
		return
	}
	expiresAt := c.clock().Add(c.cacheTTL)
	c.mu.Lock()
	defer c.mu.Unlock()
	for symbol, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		c.cache[strings.ToUpper(strings.TrimSpace(symbol))] = cachedSecuritySnapshot{
			snapshot: cloneSecuritySnapshot(snapshot), expiresAt: expiresAt,
		}
	}
}

func (c *securitySnapshotCoordinator) takeCall() (time.Duration, bool) {
	now := c.clock()
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := now.Add(-c.callWindow)
	first := 0
	for first < len(c.calls) && !c.calls[first].After(cutoff) {
		first++
	}
	if first > 0 {
		c.calls = append(c.calls[:0], c.calls[first:]...)
	}
	if c.callLimit <= 0 || c.callWindow <= 0 || len(c.calls) >= c.callLimit {
		retryAfter := time.Second
		if len(c.calls) > 0 && c.callWindow > 0 {
			retryAfter = c.calls[0].Add(c.callWindow).Sub(now)
		}
		return max(retryAfter, time.Millisecond), false
	}
	c.calls = append(c.calls, now)
	return 0, true
}

func (c *securitySnapshotCoordinator) clock() time.Time {
	if c != nil && c.now != nil {
		return c.now()
	}
	return time.Now()
}

func classifySecuritySnapshotError(err error) error {
	if err == nil || errors.Is(err, broker.ErrSnapshotRateLimited) {
		return err
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "每30秒最多60次") ||
		strings.Contains(message, "频率太高") ||
		strings.Contains(message, "frequency too high") ||
		strings.Contains(message, "too many requests") {
		return broker.NewSnapshotRateLimitError(securitySnapshotCallWindow, err)
	}
	return err
}

func cloneSecuritySnapshotMap(values map[string]*qotgetsecuritysnapshotpb.Snapshot) map[string]*qotgetsecuritysnapshotpb.Snapshot {
	cloned := make(map[string]*qotgetsecuritysnapshotpb.Snapshot, len(values))
	for symbol, snapshot := range values {
		cloned[symbol] = cloneSecuritySnapshot(snapshot)
	}
	return cloned
}

func cloneSecuritySnapshot(snapshot *qotgetsecuritysnapshotpb.Snapshot) *qotgetsecuritysnapshotpb.Snapshot {
	if snapshot == nil {
		return nil
	}
	return proto.Clone(snapshot).(*qotgetsecuritysnapshotpb.Snapshot)
}
