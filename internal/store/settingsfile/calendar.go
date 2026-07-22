package settingsfile

import (
	"strings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func (s *Store) ExchangeCalendarSettings() jfsettings.ExchangeCalendarSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Calendars != nil {
		return NormalizeExchangeCalendarSettings(*s.data.Calendars)
	}
	return DefaultExchangeCalendarSettings()
}

func (s *Store) SaveExchangeCalendarSettings(input jfsettings.ExchangeCalendarSettings) (jfsettings.ExchangeCalendarSettings, error) {
	normalized := NormalizeExchangeCalendarSettings(input)

	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.mutateAndPersistLocked(func() {
		s.data.Calendars = exchangeCalendarSettingsPointer(normalized)
	})
	return normalized, err
}

func DefaultExchangeCalendarSettings() jfsettings.ExchangeCalendarSettings {
	return jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:        true,
		ErrorNotificationsEnabled: true,
		RefreshIntervalHours:      24,
		WarmupMarkets:             []string{"US", "HK", "CN"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             "US",
				PreferredSourceIDs: []string{"nyse_official"},
				EnabledSourceIDs:   []string{"nyse_official", "builtin_rules"},
				FallbackToBuiltin:  true,
				RequireOfficial:    false,
				StaleAfterHours:    72,
			},
			{
				Market:             "HK",
				PreferredSourceIDs: []string{"hk_gov_1823_ical"},
				EnabledSourceIDs:   []string{"hk_gov_1823_ical", "builtin_rules"},
				FallbackToBuiltin:  true,
				RequireOfficial:    false,
				StaleAfterHours:    168,
			},
			{
				// Mainland official sources remain registered, but the currently wired
				// public page does not reliably publish the active year. Default to the
				// builtin calendar until a verified structured source is enabled.
				Market:            "CN",
				EnabledSourceIDs:  []string{"builtin_rules"},
				FallbackToBuiltin: true,
				RequireOfficial:   false,
				StaleAfterHours:   168,
			},
		},
	}
}

func NormalizeExchangeCalendarSettings(input jfsettings.ExchangeCalendarSettings) jfsettings.ExchangeCalendarSettings {
	defaults := DefaultExchangeCalendarSettings()
	normalized := input
	normalized.AutoRefreshEnabled = input.AutoRefreshEnabled
	if !input.ErrorNotificationsEnabledSet() && !input.ErrorNotificationsEnabled {
		normalized.ErrorNotificationsEnabled = defaults.ErrorNotificationsEnabled
	}
	normalized = normalized.WithErrorNotificationsEnabledSet(true)
	if normalized.RefreshIntervalHours <= 0 {
		normalized.RefreshIntervalHours = defaults.RefreshIntervalHours
	}
	if normalized.RefreshIntervalHours < 1 {
		normalized.RefreshIntervalHours = 1
	}
	if normalized.RefreshIntervalHours > 24*30 {
		normalized.RefreshIntervalHours = 24 * 30
	}

	if len(normalized.WarmupMarkets) == 0 {
		normalized.WarmupMarkets = append([]string(nil), defaults.WarmupMarkets...)
	} else {
		normalized.WarmupMarkets = normalizeCalendarMarkets(normalized.WarmupMarkets)
	}

	if len(normalized.SourcePolicies) == 0 {
		normalized.SourcePolicies = append([]jfsettings.ExchangeCalendarSourcePolicy(nil), defaults.SourcePolicies...)
	} else {
		normalized.SourcePolicies = normalizeCalendarPolicies(normalized.SourcePolicies)
	}

	normalized.ManualOverrides = normalizeCalendarManualOverrides(normalized.ManualOverrides)
	return normalized
}

func normalizeCalendarMarkets(markets []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(markets))
	for _, market := range markets {
		normalized := strings.ToUpper(strings.TrimSpace(market))
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

func normalizeCalendarPolicies(policies []jfsettings.ExchangeCalendarSourcePolicy) []jfsettings.ExchangeCalendarSourcePolicy {
	normalized := make([]jfsettings.ExchangeCalendarSourcePolicy, 0, len(policies))
	for _, policy := range policies {
		policy.Market = strings.ToUpper(strings.TrimSpace(policy.Market))
		if policy.Market == "" {
			continue
		}
		policy.PreferredSourceIDs = normalizeSourceIDs(policy.PreferredSourceIDs)
		policy.EnabledSourceIDs = normalizeSourceIDs(policy.EnabledSourceIDs)
		if policy.StaleAfterHours < 0 {
			policy.StaleAfterHours = 0
		}
		normalized = append(normalized, policy)
	}
	return normalized
}

func normalizeSourceIDs(ids []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		normalized := normalizeCalendarSourceID(id)
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

func normalizeCalendarSourceID(id string) string {
	switch strings.TrimSpace(id) {
	case "hkex_official":
		return "hk_gov_1823_ical"
	default:
		return strings.TrimSpace(id)
	}
}

func normalizeCalendarManualOverrides(overrides []jfsettings.ExchangeCalendarManualOverride) []jfsettings.ExchangeCalendarManualOverride {
	normalized := make([]jfsettings.ExchangeCalendarManualOverride, 0, len(overrides))
	for _, override := range overrides {
		override.Market = strings.ToUpper(strings.TrimSpace(override.Market))
		override.Date = strings.TrimSpace(override.Date)
		override.Status = strings.ToLower(strings.TrimSpace(override.Status))
		override.Reason = strings.TrimSpace(override.Reason)
		override.Sessions = normalizeCalendarManualSessions(override.Sessions)
		if override.Market == "" || override.Date == "" || override.Status == "" {
			continue
		}
		normalized = append(normalized, override)
	}
	return normalized
}

func normalizeCalendarManualSessions(sessions []jfsettings.ExchangeCalendarSessionWindow) []jfsettings.ExchangeCalendarSessionWindow {
	normalized := make([]jfsettings.ExchangeCalendarSessionWindow, 0, len(sessions))
	for _, session := range sessions {
		session.Kind = strings.ToLower(strings.TrimSpace(session.Kind))
		if session.Kind == "" || session.EndMinute <= session.StartMinute {
			continue
		}
		normalized = append(normalized, session)
	}
	return normalized
}

func exchangeCalendarSettingsPointer(value jfsettings.ExchangeCalendarSettings) *jfsettings.ExchangeCalendarSettings {
	return new(value)
}
