package exchangecalendar

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	calendarstore "github.com/jftrade/jftrade-main/internal/store/exchangecalendar"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

type Manager struct {
	builtin          *marketcalendar.BuiltinResolver
	registry         *SourceRegistry
	store            *calendarstore.Store
	settingsProvider func() jfsettings.ExchangeCalendarSettings
	now              func() time.Time
	alertSink        func(SourceAlert)

	mu        sync.RWMutex
	snapshots map[string]marketcalendar.CalendarSnapshot
	statuses  map[string]*SourceStatus

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	reloadCh  chan struct{}
	wg        sync.WaitGroup
}

type Option func(*Manager)

func WithClock(now func() time.Time) Option {
	return func(m *Manager) {
		if now != nil {
			m.now = now
		}
	}
}

func WithRegistry(registry *SourceRegistry) Option {
	return func(m *Manager) {
		if registry != nil {
			m.registry = registry
		}
	}
}

func WithAlertSink(sink func(SourceAlert)) Option {
	return func(m *Manager) {
		if sink != nil {
			m.alertSink = sink
		}
	}
}

func NewManager(store *calendarstore.Store, settingsProvider func() jfsettings.ExchangeCalendarSettings, opts ...Option) *Manager {
	manager := &Manager{
		builtin:          marketcalendar.NewBuiltinResolver(),
		registry:         DefaultRegistry(nil),
		store:            store,
		settingsProvider: settingsProvider,
		now: func() time.Time {
			return time.Now().UTC()
		},
		snapshots: map[string]marketcalendar.CalendarSnapshot{},
		statuses:  map[string]*SourceStatus{},
		stopCh:    make(chan struct{}),
		reloadCh:  make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(manager)
	}
	manager.restoreCachedSnapshots()
	return manager
}

func (m *Manager) Start() {
	if m == nil {
		return
	}
	m.startOnce.Do(func() {
		m.wg.Add(1)
		go m.run()
		m.NotifySettingsChanged()
	})
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
	m.wg.Wait()
	return nil
}

func (m *Manager) NotifySettingsChanged() {
	if m == nil {
		return
	}
	select {
	case m.reloadCh <- struct{}{}:
	default:
	}
}

func (m *Manager) Template(marketCode string) (marketcalendar.MarketTemplate, bool) {
	if m == nil {
		return marketcalendar.MarketTemplate{}, false
	}
	return m.builtin.Template(normalizeMarket(marketCode))
}

func (m *Manager) Schedule(marketCode string, day time.Time) (marketcalendar.TradingDaySchedule, bool) {
	if m == nil {
		return marketcalendar.TradingDaySchedule{}, false
	}
	normalizedMarket := normalizeMarket(marketCode)
	template, ok := m.builtin.Template(normalizedMarket)
	if !ok || day.IsZero() {
		return marketcalendar.TradingDaySchedule{}, false
	}
	localDay := marketcalendar.DayStart(template, day)
	if schedule, sourceID, ok := m.overrideSchedule(normalizedMarket, localDay); ok {
		schedule.MarketCode = normalizedMarket
		if strings.TrimSpace(schedule.SourceID) == "" {
			schedule.SourceID = sourceID
		}
		return schedule, true
	}
	return m.builtin.Schedule(normalizedMarket, localDay)
}

func (m *Manager) RefreshAll(ctx context.Context) map[string]any {
	return m.refresh(ctx, "")
}

func (m *Manager) RefreshMarket(ctx context.Context, market string) map[string]any {
	return m.refresh(ctx, market)
}

func (m *Manager) ProbeAll(ctx context.Context) map[string]any {
	return m.probe(ctx, "")
}

func (m *Manager) ProbeMarket(ctx context.Context, market string) map[string]any {
	return m.probe(ctx, market)
}

func (m *Manager) Status() map[string]any {
	settings := m.settings()
	markets := settings.WarmupMarkets
	if len(markets) == 0 {
		markets = []string{"US", "HK", "CN"}
	}
	now := m.now()
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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
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
			now := m.now().In(loc)
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
		"requestedAt":   m.now().Format(time.RFC3339Nano),
		"warmupMarkets": normalizeMarkets(markets),
	}
}

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
	checkedAt := m.now().Format(time.RFC3339Nano)
	visitedSources := map[string]struct{}{}
	for _, market := range markets {
		policy := policyForMarket(settings, market)
		template, ok := m.builtin.Template(market)
		if !ok {
			continue
		}
		loc := marketcalendar.LoadLocation(template)
		now := m.now().In(loc)
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
	key := snapshotCacheKey(snapshot.SourceID, snapshot.MarketCode, snapshot.From)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshots[key] = snapshot
}

func (m *Manager) cachedSnapshot(sourceID string, market string, day time.Time) (marketcalendar.CalendarSnapshot, bool) {
	key := snapshotCacheKey(sourceID, market, day)
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
			if !ok || !snapshotFresh(snapshot, policy, m.now()) {
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
			if !ok || !snapshotFresh(snapshot, policy, m.now()) {
				continue
			}
			if marketcalendar.SnapshotCoversDay(snapshot, template, localDay) {
				return source.ID(), true
			}
		}
	}
	return "", false
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

func (m *Manager) recordSuccess(sourceID string, snapshot marketcalendar.CalendarSnapshot) {
	if m == nil {
		return
	}
	now := m.now()
	var alert *SourceAlert
	m.mu.Lock()
	status := m.statuses[sourceID]
	if status == nil {
		status = &SourceStatus{SourceID: sourceID}
		m.statuses[sourceID] = status
	}
	status.LastSuccessAt = now
	status.LastError = ""
	status.ConsecutiveFailures = 0
	status.NextRefreshAt = time.Time{}
	status.LastSnapshotFetchedAt = snapshot.FetchedAt
	alert = recordHealthyStateLocked(status, normalizeMarket(snapshot.MarketCode), now)
	m.mu.Unlock()
	m.emitAlert(alert)
}

func (m *Manager) recordOperationFailure(sourceID string, err error) {
	if m == nil {
		return
	}
	now := m.now()
	m.mu.Lock()
	status := m.statuses[sourceID]
	if status == nil {
		status = &SourceStatus{SourceID: sourceID}
		m.statuses[sourceID] = status
	}
	status.LastFailureAt = now
	if err != nil {
		status.LastError = err.Error()
	}
	status.ConsecutiveFailures++
	backoffHours := status.ConsecutiveFailures
	if backoffHours > 24 {
		backoffHours = 24
	}
	status.NextRefreshAt = now.Add(time.Duration(backoffHours) * time.Hour)
	m.mu.Unlock()
}

func (m *Manager) recordSourceFailure(sourceID string, market string, err error, kind string) {
	if m == nil {
		return
	}
	now := m.now()
	var alert *SourceAlert
	m.mu.Lock()
	status := m.statuses[sourceID]
	if status == nil {
		status = &SourceStatus{SourceID: sourceID}
		m.statuses[sourceID] = status
	}
	status.LastFailureAt = now
	if err != nil {
		status.LastError = err.Error()
	} else {
		status.LastError = ""
	}
	status.ConsecutiveFailures++
	backoffHours := status.ConsecutiveFailures
	if backoffHours > 24 {
		backoffHours = 24
	}
	status.NextRefreshAt = now.Add(time.Duration(backoffHours) * time.Hour)
	alert = recordUnhealthyStateLocked(status, normalizeMarket(market), now, sourceFailureAlert(status.SourceID, market, kind, err))
	m.mu.Unlock()
	m.emitAlert(alert)
}

func (m *Manager) recordProbeSuccess(sourceID string, market string, scheduleCount int) {
	if m == nil {
		return
	}
	now := m.now()
	var alert *SourceAlert
	m.mu.Lock()
	status := m.statuses[sourceID]
	if status == nil {
		status = &SourceStatus{SourceID: sourceID}
		m.statuses[sourceID] = status
	}
	status.LastProbeAt = now
	status.LastProbeSuccessAt = status.LastProbeAt
	status.LastProbeStatus = "healthy"
	status.LastProbeError = ""
	status.LastProbeMarket = normalizeMarket(market)
	status.LastProbeSchedules = scheduleCount
	alert = recordHealthyStateLocked(status, normalizeMarket(market), now)
	m.mu.Unlock()
	m.emitAlert(alert)
}

func (m *Manager) recordProbeFailure(sourceID string, market string, err error, kind string) {
	if m == nil {
		return
	}
	now := m.now()
	var alert *SourceAlert
	m.mu.Lock()
	status := m.statuses[sourceID]
	if status == nil {
		status = &SourceStatus{SourceID: sourceID}
		m.statuses[sourceID] = status
	}
	status.LastProbeAt = now
	status.LastProbeFailureAt = status.LastProbeAt
	status.LastProbeStatus = "unhealthy"
	status.LastProbeMarket = normalizeMarket(market)
	status.LastProbeSchedules = 0
	if err != nil {
		status.LastProbeError = err.Error()
	} else {
		status.LastProbeError = ""
	}
	alert = recordUnhealthyStateLocked(status, normalizeMarket(market), now, sourceFailureAlert(status.SourceID, market, kind, err))
	m.mu.Unlock()
	m.emitAlert(alert)
}

func (m *Manager) settings() jfsettings.ExchangeCalendarSettings {
	if m == nil || m.settingsProvider == nil {
		return jfsettings.ExchangeCalendarSettings{}
	}
	return m.settingsProvider()
}

func snapshotCacheKey(sourceID string, market string, at time.Time) string {
	return fmt.Sprintf("%s|%s|%04d", strings.TrimSpace(sourceID), normalizeMarket(market), at.Year())
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

func policyForMarket(settings jfsettings.ExchangeCalendarSettings, market string) jfsettings.ExchangeCalendarSourcePolicy {
	normalizedMarket := normalizeMarket(market)
	for _, policy := range settings.SourcePolicies {
		if normalizeMarket(policy.Market) == normalizedMarket {
			return policy
		}
	}
	if normalizedMarket == "SH" || normalizedMarket == "SZ" {
		for _, policy := range settings.SourcePolicies {
			if normalizeMarket(policy.Market) == "CN" {
				return policy
			}
		}
	}
	return jfsettings.ExchangeCalendarSourcePolicy{
		Market:            normalizedMarket,
		FallbackToBuiltin: true,
	}
}

func sourceEnabled(settings jfsettings.ExchangeCalendarSettings, sourceID string) bool {
	if sourceID == ManualOverrideSourceID || sourceID == BuiltinSourceID {
		return true
	}
	for _, policy := range settings.SourcePolicies {
		if len(policy.EnabledSourceIDs) == 0 {
			continue
		}
		for _, id := range policy.EnabledSourceIDs {
			if strings.TrimSpace(id) == sourceID {
				return true
			}
		}
	}
	return false
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

func manualOverrideSchedule(settings jfsettings.ExchangeCalendarSettings, builtin *marketcalendar.BuiltinResolver, market string, day time.Time) (marketcalendar.TradingDaySchedule, bool) {
	if builtin == nil {
		return marketcalendar.TradingDaySchedule{}, false
	}
	template, ok := builtin.Template(market)
	if !ok {
		return marketcalendar.TradingDaySchedule{}, false
	}
	dayKey := marketcalendar.DayStart(template, day).Format("2006-01-02")
	for _, override := range settings.ManualOverrides {
		overrideMarket := normalizeMarket(override.Market)
		if overrideMarket != normalizeMarket(market) && (overrideMarket != "CN" || (normalizeMarket(market) != "SH" && normalizeMarket(market) != "SZ")) {
			continue
		}
		if strings.TrimSpace(override.Date) != dayKey {
			continue
		}
		return marketcalendar.TradingDaySchedule{
			MarketCode: normalizeMarket(market),
			Date:       marketcalendar.DayStart(template, day),
			Status:     manualStatus(override.Status),
			Sessions:   manualSessions(override.Sessions),
			Reason:     strings.TrimSpace(override.Reason),
			SourceID:   ManualOverrideSourceID,
			Observed:   override.Observed,
		}, true
	}
	return marketcalendar.TradingDaySchedule{}, false
}

func manualStatus(status string) marketcalendar.TradingDayStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(marketcalendar.TradingDayOpen):
		return marketcalendar.TradingDayOpen
	case string(marketcalendar.TradingDayClosed):
		return marketcalendar.TradingDayClosed
	case string(marketcalendar.TradingDayEarlyClose):
		return marketcalendar.TradingDayEarlyClose
	case string(marketcalendar.TradingDaySpecial):
		return marketcalendar.TradingDaySpecial
	default:
		return marketcalendar.TradingDayUnknown
	}
}

func manualSessions(sessions []jfsettings.ExchangeCalendarSessionWindow) []marketcalendar.SessionWindow {
	normalized := make([]marketcalendar.SessionWindow, 0, len(sessions))
	for _, session := range sessions {
		normalized = append(normalized, marketcalendar.SessionWindow{
			Kind:        manualSessionKind(session.Kind),
			StartMinute: session.StartMinute,
			EndMinute:   session.EndMinute,
		})
	}
	return marketcalendar.NormalizeSessions(normalized)
}

func manualSessionKind(kind string) marketcalendar.SessionKind {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case string(marketcalendar.SessionPre):
		return marketcalendar.SessionPre
	case string(marketcalendar.SessionRegular):
		return marketcalendar.SessionRegular
	case string(marketcalendar.SessionAfter):
		return marketcalendar.SessionAfter
	case string(marketcalendar.SessionOvernight):
		return marketcalendar.SessionOvernight
	case string(marketcalendar.SessionClosed):
		return marketcalendar.SessionClosed
	default:
		return marketcalendar.SessionUnknown
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

func (m *Manager) emitAlert(alert *SourceAlert) {
	if m == nil || m.alertSink == nil || alert == nil {
		return
	}
	m.alertSink(*alert)
}

func recordHealthyStateLocked(status *SourceStatus, market string, now time.Time) *SourceAlert {
	if status == nil {
		return nil
	}
	previousState := status.HealthState
	previousFingerprint := status.HealthFingerprint
	status.HealthState = "healthy"
	status.HealthFingerprint = ""
	if previousState != "unhealthy" {
		return nil
	}
	alert := &SourceAlert{
		SourceID:    status.SourceID,
		Market:      normalizeMarket(market),
		Level:       "success",
		Kind:        "recovered",
		Title:       "交易所日历源已恢复",
		Message:     fmt.Sprintf("%s 市场日历源 %s 已恢复正常解析。", normalizeMarket(market), status.SourceID),
		Fingerprint: previousFingerprint,
	}
	status.LastAlertAt = now
	status.LastAlertStatus = "recovered"
	status.LastAlertFingerprint = previousFingerprint
	return alert
}

func recordUnhealthyStateLocked(status *SourceStatus, market string, now time.Time, alert SourceAlert) *SourceAlert {
	if status == nil {
		return nil
	}
	fingerprint := strings.TrimSpace(alert.Fingerprint)
	if fingerprint == "" {
		fingerprint = fmt.Sprintf("%s|%s|unknown", status.SourceID, normalizeMarket(market))
		alert.Fingerprint = fingerprint
	}
	shouldNotify := status.HealthState != "unhealthy" || status.HealthFingerprint != fingerprint
	status.HealthState = "unhealthy"
	status.HealthFingerprint = fingerprint
	if !shouldNotify {
		return nil
	}
	alert.SourceID = status.SourceID
	alert.Market = normalizeMarket(market)
	status.LastAlertAt = now
	status.LastAlertStatus = "triggered"
	status.LastAlertFingerprint = fingerprint
	return &alert
}

func sourceFailureAlert(sourceID string, market string, kind string, err error) SourceAlert {
	normalizedMarket := normalizeMarket(market)
	message := ""
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	switch kind {
	case "structure_changed":
		return SourceAlert{
			SourceID:    strings.TrimSpace(sourceID),
			Market:      normalizedMarket,
			Level:       "error",
			Kind:        "structure_changed",
			Title:       "交易所日历源解析异常",
			Message:     fmt.Sprintf("%s 市场日历源 %s 抓取成功但未解析到有效交易日，可能是官网结构发生变化。系统将继续回退到内置日历。", normalizedMarket, sourceID),
			Fingerprint: sourceAlertFingerprint(sourceID, normalizedMarket, "structure_changed", message),
		}
	default:
		return SourceAlert{
			SourceID:    strings.TrimSpace(sourceID),
			Market:      normalizedMarket,
			Level:       "warn",
			Kind:        "fetch_failed",
			Title:       "交易所日历源抓取失败",
			Message:     fmt.Sprintf("%s 市场日历源 %s 抓取失败：%s。系统将继续回退到内置日历。", normalizedMarket, sourceID, defaultAlertDetail(message)),
			Fingerprint: sourceAlertFingerprint(sourceID, normalizedMarket, "fetch_failed", message),
		}
	}
}

func sourceAlertFingerprint(sourceID string, market string, kind string, detail string) string {
	return fmt.Sprintf("%s|%s|%s|%s", strings.TrimSpace(sourceID), normalizeMarket(market), strings.TrimSpace(kind), strings.TrimSpace(detail))
}

func defaultAlertDetail(detail string) string {
	if strings.TrimSpace(detail) == "" {
		return "unknown error"
	}
	return strings.TrimSpace(detail)
}
