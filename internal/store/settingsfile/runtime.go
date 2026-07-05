package settingsfile

import (
	"strings"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func (s *Store) ExecutionSettings() jfsettings.ExecutionSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Execution != nil {
		return NormalizeExecutionSettings(*s.data.Execution)
	}
	return DefaultExecutionSettings()
}

func (s *Store) SaveExecutionSettings(input jfsettings.ExecutionSettings) (jfsettings.ExecutionSettings, error) {
	normalized := NormalizeExecutionSettings(input)

	s.mu.Lock()
	s.data.Execution = executionSettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func (s *Store) SecuritySettings() jfsettings.SecuritySettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Security != nil {
		return NormalizeSecuritySettings(*s.data.Security)
	}
	return DefaultSecuritySettings()
}

func (s *Store) SaveSecuritySettings(input jfsettings.SecuritySettings) (jfsettings.SecuritySettings, error) {
	normalized := NormalizeSecuritySettings(input)

	s.mu.Lock()
	s.data.Security = securitySettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func (s *Store) ADKSettings() jfsettings.ADKRuntimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.ADK != nil {
		return NormalizeADKRuntimeSettings(*s.data.ADK)
	}
	return DefaultADKRuntimeSettings()
}

func (s *Store) SaveADKSettings(input jfsettings.ADKRuntimeSettings) (jfsettings.ADKRuntimeSettings, error) {
	normalized := NormalizeADKRuntimeSettings(input)

	s.mu.Lock()
	s.data.ADK = adkRuntimeSettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func (s *Store) PineWorkerSettings() jfsettings.PineWorkerSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.PineWorker != nil {
		return NormalizePineWorkerSettings(*s.data.PineWorker)
	}
	return DefaultPineWorkerSettings()
}

func (s *Store) SavePineWorkerSettings(input jfsettings.PineWorkerSettings) (jfsettings.PineWorkerSettings, error) {
	normalized := NormalizePineWorkerSettings(input)

	s.mu.Lock()
	s.data.PineWorker = pineWorkerSettingsPointer(normalized)
	err := s.persistLocked()
	s.mu.Unlock()
	return normalized, err
}

func DefaultExecutionSettings() jfsettings.ExecutionSettings {
	return jfsettings.ExecutionSettings{
		DefaultTradingEnvironment:      "SIMULATE",
		BrokerOrderHistoryLookbackDays: 30,
		SeenFillRetentionDays:          90,
	}
}

func NormalizeExecutionSettings(input jfsettings.ExecutionSettings) jfsettings.ExecutionSettings {
	defaults := DefaultExecutionSettings()
	settings := input
	settings.DefaultTradingEnvironment = normalizeExecutionTradingEnvironment(settings.DefaultTradingEnvironment)
	if settings.DefaultTradingEnvironment == "" {
		settings.DefaultTradingEnvironment = defaults.DefaultTradingEnvironment
	}
	if settings.BrokerOrderHistoryLookbackDays < 1 {
		settings.BrokerOrderHistoryLookbackDays = defaults.BrokerOrderHistoryLookbackDays
	}
	if settings.BrokerOrderHistoryLookbackDays > 365 {
		settings.BrokerOrderHistoryLookbackDays = 365
	}
	if settings.SeenFillRetentionDays < 1 {
		settings.SeenFillRetentionDays = defaults.SeenFillRetentionDays
	}
	if settings.SeenFillRetentionDays > 3650 {
		settings.SeenFillRetentionDays = 3650
	}
	return settings
}

func normalizeExecutionTradingEnvironment(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "SIMULATE", "REAL":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return ""
	}
}

func executionSettingsPointer(value jfsettings.ExecutionSettings) *jfsettings.ExecutionSettings {
	return new(value)
}

func DefaultSecuritySettings() jfsettings.SecuritySettings {
	return jfsettings.SecuritySettings{
		AdminAuthRequired: false,
	}
}

func NormalizeSecuritySettings(input jfsettings.SecuritySettings) jfsettings.SecuritySettings {
	return jfsettings.SecuritySettings{
		AdminAuthRequired: input.AdminAuthRequired,
	}
}

func securitySettingsPointer(value jfsettings.SecuritySettings) *jfsettings.SecuritySettings {
	return new(value)
}

func DefaultADKRuntimeSettings() jfsettings.ADKRuntimeSettings {
	return jfsettings.ADKRuntimeSettings{
		RunTimeoutMs:        1_800_000,
		StreamIdleTimeoutMs: 300_000,
	}
}

func NormalizeADKRuntimeSettings(input jfsettings.ADKRuntimeSettings) jfsettings.ADKRuntimeSettings {
	defaults := DefaultADKRuntimeSettings()
	return jfsettings.ADKRuntimeSettings{
		RunTimeoutMs:        clampOrDefaultInt(input.RunTimeoutMs, defaults.RunTimeoutMs, 60_000, 43_200_000),
		StreamIdleTimeoutMs: clampOrDefaultInt(input.StreamIdleTimeoutMs, defaults.StreamIdleTimeoutMs, 30_000, 900_000),
	}
}

func adkRuntimeSettingsPointer(value jfsettings.ADKRuntimeSettings) *jfsettings.ADKRuntimeSettings {
	return new(value)
}

func DefaultPineWorkerSettings() jfsettings.PineWorkerSettings {
	return jfsettings.PineWorkerSettings{
		BacktestWorkerLimit: 2,
		InstanceWorkerLimit: 10,
		NodeBinaryPath:      "",
	}
}

func NormalizePineWorkerSettings(input jfsettings.PineWorkerSettings) jfsettings.PineWorkerSettings {
	return jfsettings.PineWorkerSettings{
		BacktestWorkerLimit: clampInt(input.BacktestWorkerLimit, 1, 1000),
		InstanceWorkerLimit: clampInt(input.InstanceWorkerLimit, 1, 1000),
		NodeBinaryPath:      NormalizeNodeBinaryPath(input.NodeBinaryPath),
	}
}

func NormalizeNodeBinaryPath(input string) string {
	value := strings.TrimSpace(input)
	for len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first != '"' && first != '\'') || first != last {
			break
		}
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	return value
}

func pineWorkerSettingsPointer(value jfsettings.PineWorkerSettings) *jfsettings.PineWorkerSettings {
	return new(value)
}

func clampOrDefaultInt(value int, fallback int, min int, max int) int {
	if value <= 0 {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func clampInt(value int, min int, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
