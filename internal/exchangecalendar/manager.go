package exchangecalendar

import (
	"context"
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

const defaultWarmupRefreshTimeout = 60 * time.Second

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

func (m *Manager) settings() jfsettings.ExchangeCalendarSettings {
	if m == nil || m.settingsProvider == nil {
		return jfsettings.ExchangeCalendarSettings{}
	}
	return m.settingsProvider()
}

func (m *Manager) currentTime() time.Time {
	if m == nil || m.now == nil {
		return time.Now().UTC()
	}
	return m.now().UTC()
}
