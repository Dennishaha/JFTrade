package exchangecalendar

import (
	"sort"
	"strings"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

func (m *Manager) Status() map[string]any {
	settings := m.settings()
	markets := settings.WarmupMarkets
	if len(markets) == 0 {
		markets = []string{"US", "HK", "CN"}
	}
	now := m.currentTime()
	marketRows := make([]map[string]any, 0, len(markets))
	for _, market := range normalizeMarkets(markets) {
		effectiveSource := BuiltinSourceID
		effectiveMode := "builtin_fallback"
		if _, sourceID, ok := m.overrideSchedule(market, now); ok {
			effectiveSource = sourceID
			if sourceID == ManualOverrideSourceID {
				effectiveMode = "manual_override"
			} else {
				effectiveMode = "remote_override"
			}
		} else if sourceID, ok := m.coverageSource(market, now); ok {
			effectiveSource = sourceID
			effectiveMode = "remote_covered_day"
		}
		marketRows = append(marketRows, map[string]any{
			"market":          market,
			"effectiveSource": effectiveSource,
			"effectiveMode":   effectiveMode,
			"effectiveReason": m.marketEffectiveReason(market, effectiveSource, effectiveMode, now),
			"fallbackChain":   m.fallbackChain(market),
			"checkedAt":       now.Format(time.RFC3339Nano),
		})
	}
	return map[string]any{
		"autoRefreshEnabled":   settings.AutoRefreshEnabled,
		"refreshIntervalHours": settings.RefreshIntervalHours,
		"warmupMarkets":        normalizeMarkets(settings.WarmupMarkets),
		"markets":              marketRows,
		"sources":              m.Sources(),
		"snapshots":            m.snapshotSummaries(),
	}
}

func (m *Manager) Sources() []map[string]any {
	settings := m.settings()
	descriptors := append([]SourceDescriptor{
		{ID: ManualOverrideSourceID, Kind: "manual_override", Authority: "settings", Markets: []string{"US", "HK", "CN", "SH", "SZ"}},
		{ID: BuiltinSourceID, Kind: "builtin_rules", Authority: "builtin", Markets: []string{"US", "HK", "CN", "SH", "SZ"}},
	}, m.registry.Descriptors()...)
	sort.SliceStable(descriptors, func(i, j int) bool { return descriptors[i].ID < descriptors[j].ID })
	rows := make([]map[string]any, 0, len(descriptors))
	for _, descriptor := range descriptors {
		status := m.sourceStatus(descriptor.ID)
		rows = append(rows, map[string]any{
			"id":                    descriptor.ID,
			"kind":                  descriptor.Kind,
			"authority":             descriptor.Authority,
			"markets":               descriptor.Markets,
			"enabled":               sourceEnabled(settings, descriptor.ID),
			"availabilityNote":      sourceAvailabilityNote(descriptor.ID),
			"lastSuccessAt":         status.LastSuccessAt,
			"lastFailureAt":         status.LastFailureAt,
			"lastError":             status.LastError,
			"consecutiveFailures":   status.ConsecutiveFailures,
			"nextRefreshAt":         status.NextRefreshAt,
			"lastSnapshotFetchedAt": status.LastSnapshotFetchedAt,
			"lastProbeAt":           status.LastProbeAt,
			"lastProbeSuccessAt":    status.LastProbeSuccessAt,
			"lastProbeFailureAt":    status.LastProbeFailureAt,
			"lastProbeStatus":       status.LastProbeStatus,
			"lastProbeError":        status.LastProbeError,
			"lastProbeMarket":       status.LastProbeMarket,
			"lastProbeSchedules":    status.LastProbeSchedules,
			"healthState":           status.HealthState,
			"healthFingerprint":     status.HealthFingerprint,
			"lastAlertAt":           status.LastAlertAt,
			"lastAlertStatus":       status.LastAlertStatus,
			"lastAlertFingerprint":  status.LastAlertFingerprint,
		})
	}
	return rows
}

func (m *Manager) fallbackChain(market string) []string {
	settings := m.settings()
	policy := policyForMarket(settings, market)
	chain := []string{ManualOverrideSourceID}
	for _, source := range m.registry.OrderedSources(market, policy) {
		chain = append(chain, source.ID())
	}
	if policy.FallbackToBuiltin || len(chain) == 1 {
		chain = append(chain, BuiltinSourceID)
	}
	return chain
}

func (m *Manager) sourceStatus(sourceID string) SourceStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if status, ok := m.statuses[sourceID]; ok && status != nil {
		return *status
	}
	return SourceStatus{SourceID: sourceID}
}

func (m *Manager) snapshotSummaries() []map[string]any {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	snapshots := make([]marketcalendar.CalendarSnapshot, 0, len(m.snapshots))
	for _, snapshot := range m.snapshots {
		if strings.TrimSpace(snapshot.SourceID) == "" || snapshot.SourceID == BuiltinSourceID {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	m.mu.RUnlock()

	sort.SliceStable(snapshots, func(i, j int) bool {
		left := snapshotSortKey(snapshots[i])
		right := snapshotSortKey(snapshots[j])
		return left < right
	})

	rows := make([]map[string]any, 0, len(snapshots))
	for _, snapshot := range snapshots {
		rows = append(rows, map[string]any{
			"market":          normalizeMarket(snapshot.MarketCode),
			"sourceId":        strings.TrimSpace(snapshot.SourceID),
			"from":            snapshot.From,
			"to":              snapshot.To,
			"fetchedAt":       snapshot.FetchedAt,
			"validUntil":      snapshot.ValidUntil,
			"schedulesParsed": len(snapshot.Schedules),
			"checksum":        snapshot.Checksum,
			"sampleSchedules": sampleSnapshotSchedules(snapshot.Schedules, 8),
		})
	}
	return rows
}

func snapshotSortKey(snapshot marketcalendar.CalendarSnapshot) string {
	return strings.Join([]string{
		normalizeMarket(snapshot.MarketCode),
		strings.TrimSpace(snapshot.SourceID),
		snapshot.From.Format(time.RFC3339Nano),
	}, "|")
}

func sampleSnapshotSchedules(schedules []marketcalendar.TradingDaySchedule, limit int) []map[string]any {
	if limit <= 0 {
		return nil
	}
	samples := make([]marketcalendar.TradingDaySchedule, 0, limit)
	for _, schedule := range schedules {
		if schedule.Status == marketcalendar.TradingDayOpen {
			continue
		}
		samples = append(samples, schedule)
		if len(samples) >= limit {
			break
		}
	}
	rows := make([]map[string]any, 0, len(samples))
	for _, schedule := range samples {
		rows = append(rows, map[string]any{
			"market":   normalizeMarket(schedule.MarketCode),
			"date":     schedule.Date.Format("2006-01-02"),
			"status":   schedule.Status,
			"reason":   schedule.Reason,
			"sourceId": strings.TrimSpace(schedule.SourceID),
			"observed": schedule.Observed,
			"sessions": schedule.Sessions,
		})
	}
	return rows
}

func (m *Manager) marketEffectiveReason(market string, effectiveSource string, effectiveMode string, now time.Time) string {
	switch effectiveMode {
	case "manual_override":
		return "manual override is active for the checked trading day"
	case "remote_override":
		return "a fresh source snapshot covers the checked trading day"
	case "remote_covered_day":
		return "a fresh source snapshot covers the checked trading day; builtin template supplies the standard session result because that date has no special override"
	}
	settings := m.settings()
	policy := policyForMarket(settings, market)
	enabledRemote := enabledRemoteSourcesForMarket(m.registry, market, policy)
	if len(enabledRemote) == 0 {
		return "current policy uses builtin_rules because no external source is enabled for this market"
	}
	return "builtin_rules is serving this market because no fresh external snapshot currently covers the checked trading day"
}

func enabledRemoteSourcesForMarket(registry *SourceRegistry, market string, policy jfsettings.ExchangeCalendarSourcePolicy) []string {
	if registry == nil {
		return nil
	}
	sources := registry.OrderedSources(market, policy)
	result := make([]string, 0, len(sources))
	for _, source := range sources {
		if source == nil {
			continue
		}
		id := strings.TrimSpace(source.ID())
		if id == "" || id == BuiltinSourceID || id == ManualOverrideSourceID {
			continue
		}
		result = append(result, id)
	}
	return result
}

func sourceAvailabilityNote(sourceID string) string {
	switch strings.TrimSpace(sourceID) {
	case "nyse_official":
		return "Primary US source. The NYSE multi-year holiday table is parsed directly and Nasdaq remains a secondary verifier."
	case "nasdaq_verifier":
		return "Secondary US verifier. Nasdaq currently serves automated requests unreliably in some environments, so it stays available as an opt-in cross-check instead of a default source."
	case "hk_gov_1823_ical":
		return "Official Hong Kong holiday iCal. It publishes a rolling multi-year window, so future-year coverage may appear later than the current anchor year."
	case "mainland_official_notice":
		return "Current adapter targets the SSE English Trading Schedule page. Default policy keeps it disabled until verified current-year mainland schedules are available."
	case BuiltinSourceID:
		return "Offline fallback rules bundled with the application. They remain available even when all external sources fail."
	case ManualOverrideSourceID:
		return "Operator-defined overrides always take precedence over remote and builtin calendars."
	default:
		return ""
	}
}
