package exchangecalendar

import (
	"context"
	"fmt"
	"strings"
	"time"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func (m *Manager) run() {
	defer m.wg.Done()
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		settings := m.settings()
		interval := time.Duration(settings.RefreshIntervalHours) * time.Hour
		if interval <= 0 {
			interval = 24 * time.Hour
		}
		select {
		case <-m.stopCh:
			return
		case <-m.reloadCh:
			if settings.AutoRefreshEnabled {
				m.refreshWarmup()
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(interval)
		case <-timer.C:
			if settings.AutoRefreshEnabled {
				m.refreshWarmup()
			}
			timer.Reset(interval)
		}
	}
}

func (m *Manager) refreshWarmup() {
	ctx, cancel := context.WithTimeout(context.Background(), defaultWarmupRefreshTimeout)
	defer cancel()
	go func() {
		select {
		case <-m.stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	m.refresh(ctx, "")
}

func (m *Manager) refresh(ctx context.Context, targetMarket string) map[string]any {
	settings := m.settings()
	markets := normalizeMarkets(settings.WarmupMarkets)
	if targetMarket != "" {
		markets = refreshMarketsForTarget(targetMarket)
	}
	if len(markets) == 0 {
		markets = []string{"US", "HK", "CN"}
	}
	updated := 0
	failures := 0
	visitedSources := map[string]struct{}{}
	for _, market := range markets {
		policy := policyForMarket(settings, market)
		template, ok := m.builtin.Template(market)
		if !ok {
			continue
		}
		loc := marketcalendar.LoadLocation(template)
		for _, source := range m.registry.OrderedSources(market, policy) {
			if source == nil {
				continue
			}
			if _, ok := visitedSources[source.ID()+"|"+market]; ok {
				continue
			}
			visitedSources[source.ID()+"|"+market] = struct{}{}
			now := m.currentTime().In(loc)
			from := time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, loc)
			to := time.Date(now.Year()+1, time.December, 31, 23, 59, 59, 0, loc)
			snapshot, err := source.Fetch(ctx, market, from, to)
			if err != nil {
				failures++
				m.recordSourceFailure(source.ID(), market, err, "fetch_failed")
				continue
			}
			if len(snapshot.Schedules) == 0 {
				failures++
				m.recordSourceFailure(source.ID(), market, fmt.Errorf("no schedules parsed"), "structure_changed")
				continue
			}
			if strings.TrimSpace(snapshot.SourceID) == "" {
				snapshot.SourceID = source.ID()
			}
			if strings.TrimSpace(snapshot.MarketCode) == "" {
				snapshot.MarketCode = normalizeMarket(market)
			}
			m.cacheSnapshot(snapshot)
			if m.store != nil {
				if err := m.store.SaveSnapshot(snapshot); err != nil {
					failures++
					m.recordOperationFailure(source.ID(), err)
					continue
				}
			}
			updated++
			m.recordSuccess(source.ID(), snapshot)
		}
	}
	return map[string]any{
		"accepted":      true,
		"market":        normalizeMarket(targetMarket),
		"updated":       updated,
		"failures":      failures,
		"requestedAt":   m.currentTime().Format(time.RFC3339Nano),
		"warmupMarkets": normalizeMarkets(markets),
	}
}

//nolint:funlen
func (m *Manager) probe(ctx context.Context, targetMarket string) map[string]any {
	settings := m.settings()
	markets := normalizeMarkets(settings.WarmupMarkets)
	if targetMarket != "" {
		markets = refreshMarketsForTarget(targetMarket)
	}
	if len(markets) == 0 {
		markets = []string{"US", "HK", "CN"}
	}

	results := make([]map[string]any, 0)
	healthy := 0
	failures := 0
	checkedAt := m.currentTime().Format(time.RFC3339Nano)
	visitedSources := map[string]struct{}{}
	for _, market := range markets {
		policy := policyForMarket(settings, market)
		template, ok := m.builtin.Template(market)
		if !ok {
			continue
		}
		loc := marketcalendar.LoadLocation(template)
		now := m.currentTime().In(loc)
		from := time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, loc)
		to := time.Date(now.Year()+1, time.December, 31, 23, 59, 59, 0, loc)
		for _, source := range m.registry.OrderedSources(market, policy) {
			if source == nil {
				continue
			}
			key := source.ID() + "|" + market
			if _, ok := visitedSources[key]; ok {
				continue
			}
			visitedSources[key] = struct{}{}

			snapshot, err := source.Fetch(ctx, market, from, to)
			if err != nil {
				failures++
				m.recordProbeFailure(source.ID(), market, err, "fetch_failed")
				results = append(results, map[string]any{
					"sourceId": source.ID(),
					"market":   market,
					"status":   "unhealthy",
					"error":    err.Error(),
				})
				continue
			}

			if len(snapshot.Schedules) == 0 {
				err := fmt.Errorf("no schedules parsed")
				failures++
				m.recordProbeFailure(source.ID(), market, err, "structure_changed")
				results = append(results, map[string]any{
					"sourceId":        source.ID(),
					"market":          market,
					"status":          "unhealthy",
					"error":           err.Error(),
					"fetchedAt":       snapshot.FetchedAt,
					"validUntil":      snapshot.ValidUntil,
					"schedulesParsed": 0,
				})
				continue
			}

			healthy++
			m.recordProbeSuccess(source.ID(), market, len(snapshot.Schedules))
			results = append(results, map[string]any{
				"sourceId":        source.ID(),
				"market":          market,
				"status":          "healthy",
				"fetchedAt":       snapshot.FetchedAt,
				"validUntil":      snapshot.ValidUntil,
				"schedulesParsed": len(snapshot.Schedules),
				"checksum":        snapshot.Checksum,
			})
		}
	}

	return map[string]any{
		"accepted":   true,
		"market":     normalizeMarket(targetMarket),
		"checkedAt":  checkedAt,
		"healthy":    healthy,
		"failures":   failures,
		"results":    results,
		"probeScope": normalizeMarkets(markets),
	}
}

func refreshMarketsForTarget(market string) []string {
	switch normalizeMarket(market) {
	case "", "CN", "SH", "SZ":
		return []string{"CN"}
	default:
		return []string{normalizeMarket(market)}
	}
}
