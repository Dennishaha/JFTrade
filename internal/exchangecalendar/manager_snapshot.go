package exchangecalendar

import (
	"fmt"
	"strings"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func (m *Manager) restoreCachedSnapshots() {
	if m == nil || m.store == nil {
		return
	}
	snapshots, errs := m.store.LoadSnapshots()
	for _, snapshot := range snapshots {
		if err := m.validateCachedSnapshot(snapshot); err != nil {
			m.recordOperationFailure(snapshot.SourceID, fmt.Errorf("discard invalid cached snapshot %s/%s: %w", snapshot.MarketCode, snapshot.SourceID, err))
			if deleteErr := m.store.DeleteSnapshot(snapshot); deleteErr != nil {
				m.recordOperationFailure(snapshot.SourceID, deleteErr)
			}
			continue
		}
		m.cacheSnapshot(snapshot)
		m.recordSuccess(snapshot.SourceID, snapshot)
	}
	for _, err := range errs {
		m.recordOperationFailure(BuiltinSourceID, err)
	}
}

func (m *Manager) validateCachedSnapshot(snapshot marketcalendar.CalendarSnapshot) error {
	if strings.TrimSpace(snapshot.SourceID) == "" {
		return fmt.Errorf("missing sourceId")
	}
	market := normalizeMarket(snapshot.MarketCode)
	if market == "" {
		return fmt.Errorf("missing marketCode")
	}
	template, ok := m.builtin.Template(market)
	if !ok && (market == "SH" || market == "SZ") {
		template, ok = m.builtin.Template("CN")
	}
	if !ok {
		return fmt.Errorf("unsupported market %q", snapshot.MarketCode)
	}
	if snapshot.From.IsZero() || snapshot.To.IsZero() {
		return fmt.Errorf("missing snapshot range")
	}
	if snapshot.To.Before(snapshot.From) {
		return fmt.Errorf("invalid snapshot range")
	}
	for _, schedule := range snapshot.Schedules {
		if schedule.Date.IsZero() {
			return fmt.Errorf("schedule has empty date")
		}
		if !dateWithinFetchRange(schedule.Date, snapshot.From, snapshot.To, template) {
			return fmt.Errorf("schedule %s outside snapshot range", schedule.Date.Format("2006-01-02"))
		}
	}
	if source, ok := m.registry.Source(snapshot.SourceID); ok {
		if validator, ok := source.(SnapshotValidator); ok {
			if err := validator.ValidateSnapshot(market, snapshot.Schedules, snapshot.From, snapshot.To); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) cacheSnapshot(snapshot marketcalendar.CalendarSnapshot) {
	if m == nil {
		return
	}
	keys := m.snapshotCacheKeys(snapshot)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		m.snapshots[key] = snapshot
	}
}

func (m *Manager) cachedSnapshot(sourceID string, market string, day time.Time) (marketcalendar.CalendarSnapshot, bool) {
	key := m.snapshotCacheKey(sourceID, market, day)
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot, ok := m.snapshots[key]
	return snapshot, ok
}

func (m *Manager) overrideSchedule(market string, day time.Time) (marketcalendar.TradingDaySchedule, string, bool) {
	if schedule, ok := manualOverrideSchedule(m.settings(), m.builtin, market, day); ok {
		return schedule, ManualOverrideSourceID, true
	}
	settings := m.settings()
	policy := policyForMarket(settings, market)
	template, ok := m.builtin.Template(market)
	if !ok {
		return marketcalendar.TradingDaySchedule{}, "", false
	}
	for _, source := range m.registry.OrderedSources(market, policy) {
		for _, candidateMarket := range candidateSnapshotMarkets(market) {
			snapshot, ok := m.cachedSnapshot(source.ID(), candidateMarket, day)
			if !ok || !snapshotFresh(snapshot, policy, m.currentTime()) {
				continue
			}
			if !marketcalendar.SnapshotCoversDay(snapshot, template, day) {
				continue
			}
			if schedule, ok := marketcalendar.SnapshotSchedule(snapshot, candidateMarket, marketcalendar.DayStart(template, day)); ok {
				schedule.SourceID = source.ID()
				return schedule, source.ID(), true
			}
		}
	}
	return marketcalendar.TradingDaySchedule{}, "", false
}

func (m *Manager) coverageSource(market string, day time.Time) (string, bool) {
	if m == nil {
		return "", false
	}
	settings := m.settings()
	policy := policyForMarket(settings, market)
	template, ok := m.builtin.Template(market)
	if !ok {
		return "", false
	}
	localDay := marketcalendar.DayStart(template, day)
	for _, source := range m.registry.OrderedSources(market, policy) {
		for _, candidateMarket := range candidateSnapshotMarkets(market) {
			snapshot, ok := m.cachedSnapshot(source.ID(), candidateMarket, localDay)
			if !ok || !snapshotFresh(snapshot, policy, m.currentTime()) {
				continue
			}
			if marketcalendar.SnapshotCoversDay(snapshot, template, localDay) {
				return source.ID(), true
			}
		}
	}
	return "", false
}

func (m *Manager) snapshotCacheKey(sourceID string, market string, at time.Time) string {
	return snapshotCacheKeyForYear(sourceID, market, m.snapshotLocalYear(market, at))
}

func (m *Manager) snapshotCacheKeys(snapshot marketcalendar.CalendarSnapshot) []string {
	startYear := m.snapshotLocalYear(snapshot.MarketCode, snapshot.From)
	endYear := startYear
	if !snapshot.To.IsZero() {
		endYear = m.snapshotLocalYear(snapshot.MarketCode, snapshot.To)
	}
	if endYear < startYear {
		endYear = startYear
	}
	keys := make([]string, 0, endYear-startYear+1)
	for year := startYear; year <= endYear; year++ {
		keys = append(keys, snapshotCacheKeyForYear(snapshot.SourceID, snapshot.MarketCode, year))
	}
	return keys
}

func (m *Manager) snapshotLocalYear(market string, at time.Time) int {
	year := at.UTC().Year()
	if m != nil && m.builtin != nil {
		if template, ok := m.builtin.Template(normalizeMarket(market)); ok {
			year = at.In(marketcalendar.LoadLocation(template)).Year()
		}
	}
	return year
}

func snapshotCacheKeyForYear(sourceID string, market string, year int) string {
	return fmt.Sprintf("%s|%s|%04d", strings.TrimSpace(sourceID), normalizeMarket(market), year)
}

func snapshotFresh(snapshot marketcalendar.CalendarSnapshot, policy jfsettings.ExchangeCalendarSourcePolicy, now time.Time) bool {
	if !snapshot.ValidUntil.IsZero() && now.After(snapshot.ValidUntil) {
		return false
	}
	if policy.StaleAfterHours > 0 && !snapshot.FetchedAt.IsZero() {
		if snapshot.FetchedAt.Add(time.Duration(policy.StaleAfterHours) * time.Hour).Before(now) {
			return false
		}
	}
	return true
}
