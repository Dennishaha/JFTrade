package exchangecalendar

import (
	"context"
	"sort"
	"strings"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

const (
	BuiltinSourceID        = marketcalendar.BuiltinSourceID
	ManualOverrideSourceID = "manual_override"
)

type Source interface {
	ID() string
	Kind() string
	Markets() []string
	Authority() string
	Fetch(ctx context.Context, market string, from time.Time, to time.Time) (marketcalendar.CalendarSnapshot, error)
}

type SnapshotValidator interface {
	ValidateSnapshot(market string, schedules []marketcalendar.TradingDaySchedule, from time.Time, to time.Time) error
}

type SourceDescriptor struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Authority string   `json:"authority"`
	Markets   []string `json:"markets"`
}

type SourceStatus struct {
	SourceID              string    `json:"sourceId"`
	Enabled               bool      `json:"enabled"`
	LastSuccessAt         time.Time `json:"lastSuccessAt,omitempty"`
	LastFailureAt         time.Time `json:"lastFailureAt,omitempty"`
	LastError             string    `json:"lastError,omitempty"`
	ConsecutiveFailures   int       `json:"consecutiveFailures,omitempty"`
	NextRefreshAt         time.Time `json:"nextRefreshAt,omitempty"`
	LastSnapshotFetchedAt time.Time `json:"lastSnapshotFetchedAt,omitempty"`
	LastProbeAt           time.Time `json:"lastProbeAt,omitempty"`
	LastProbeSuccessAt    time.Time `json:"lastProbeSuccessAt,omitempty"`
	LastProbeFailureAt    time.Time `json:"lastProbeFailureAt,omitempty"`
	LastProbeStatus       string    `json:"lastProbeStatus,omitempty"`
	LastProbeError        string    `json:"lastProbeError,omitempty"`
	LastProbeMarket       string    `json:"lastProbeMarket,omitempty"`
	LastProbeSchedules    int       `json:"lastProbeSchedules,omitempty"`
	HealthState           string    `json:"healthState,omitempty"`
	HealthFingerprint     string    `json:"healthFingerprint,omitempty"`
	LastAlertAt           time.Time `json:"lastAlertAt,omitempty"`
	LastAlertStatus       string    `json:"lastAlertStatus,omitempty"`
	LastAlertFingerprint  string    `json:"lastAlertFingerprint,omitempty"`
}

type SourceAlert struct {
	SourceID    string `json:"sourceId"`
	Market      string `json:"market"`
	Level       string `json:"level"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	Fingerprint string `json:"fingerprint"`
}

type SourceRegistry struct {
	sources map[string]Source
	order   []string
}

func NewSourceRegistry() *SourceRegistry {
	return &SourceRegistry{sources: map[string]Source{}}
}

func (r *SourceRegistry) Register(source Source) {
	if r == nil || source == nil {
		return
	}
	id := strings.TrimSpace(source.ID())
	if id == "" {
		return
	}
	if _, ok := r.sources[id]; !ok {
		r.order = append(r.order, id)
	}
	r.sources[id] = source
}

func (r *SourceRegistry) Source(id string) (Source, bool) {
	if r == nil {
		return nil, false
	}
	source, ok := r.sources[strings.TrimSpace(id)]
	return source, ok
}

func (r *SourceRegistry) OrderedSources(market string, policy jfsettings.ExchangeCalendarSourcePolicy) []Source {
	if r == nil {
		return nil
	}
	normalizedMarket := normalizeMarket(market)
	enabled := map[string]struct{}{}
	for _, id := range policy.EnabledSourceIDs {
		enabled[strings.TrimSpace(id)] = struct{}{}
	}
	seen := map[string]struct{}{}
	ordered := make([]Source, 0, len(r.order))
	appendIfEligible := func(id string) {
		if id == "" {
			return
		}
		source, ok := r.Source(id)
		if !ok {
			return
		}
		if len(enabled) > 0 {
			if _, ok := enabled[id]; !ok {
				return
			}
		}
		if !sourceSupportsMarket(source, normalizedMarket) {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ordered = append(ordered, source)
	}
	for _, id := range policy.PreferredSourceIDs {
		appendIfEligible(strings.TrimSpace(id))
	}
	for _, id := range r.order {
		appendIfEligible(id)
	}
	return ordered
}

func (r *SourceRegistry) Descriptors() []SourceDescriptor {
	if r == nil {
		return nil
	}
	descriptors := make([]SourceDescriptor, 0, len(r.order))
	for _, id := range r.order {
		source, ok := r.sources[id]
		if !ok {
			continue
		}
		descriptors = append(descriptors, describeSource(source))
	}
	sort.SliceStable(descriptors, func(i, j int) bool { return descriptors[i].ID < descriptors[j].ID })
	return descriptors
}

func describeSource(source Source) SourceDescriptor {
	if source == nil {
		return SourceDescriptor{}
	}
	return SourceDescriptor{
		ID:        strings.TrimSpace(source.ID()),
		Kind:      strings.TrimSpace(source.Kind()),
		Authority: strings.TrimSpace(source.Authority()),
		Markets:   normalizeMarkets(source.Markets()),
	}
}

func sourceSupportsMarket(source Source, market string) bool {
	for _, candidate := range normalizeMarkets(source.Markets()) {
		switch {
		case candidate == market:
			return true
		case candidate == "CN" && (market == "SH" || market == "SZ"):
			return true
		}
	}
	return false
}

func normalizeMarkets(markets []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(markets))
	for _, market := range markets {
		normalized := normalizeMarket(market)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func normalizeMarket(market string) string {
	return strings.ToUpper(strings.TrimSpace(market))
}

func candidateSnapshotMarkets(market string) []string {
	switch normalizeMarket(market) {
	case "SH", "SZ":
		return []string{normalizeMarket(market), "CN"}
	case "CN":
		return []string{"CN", "SH", "SZ"}
	default:
		return []string{normalizeMarket(market)}
	}
}
