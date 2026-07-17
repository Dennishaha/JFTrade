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
		return NormalizeSecuritySettings(jfsettings.SecuritySettings{
			WebAccessEnabled:    s.data.Security.WebAccessEnabled,
			PublicAccessEnabled: s.data.Security.PublicAccessEnabled,
			WebPort:             s.data.Security.WebPort,
			PasswordHash:        s.data.Security.PasswordHash,
		})
	}
	return DefaultSecuritySettings()
}

func (s *Store) SaveSecuritySettings(input jfsettings.SecuritySettings) (jfsettings.SecuritySettings, error) {
	normalized := NormalizeSecuritySettings(input)

	s.mu.Lock()
	previous := s.data.Security
	s.data.Security = &storedSecuritySettings{
		WebAccessEnabled:    normalized.WebAccessEnabled,
		PublicAccessEnabled: normalized.PublicAccessEnabled,
		WebPort:             normalized.WebPort,
		PasswordHash:        normalized.PasswordHash,
	}
	err := s.persistLocked()
	if err != nil {
		s.data.Security = previous
	}
	s.mu.Unlock()
	return normalized, err
}

func (s *Store) SystemNotificationSettings() jfsettings.SystemNotificationSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.SystemNotifications != nil {
		return NormalizeSystemNotificationSettings(*s.data.SystemNotifications)
	}
	return DefaultSystemNotificationSettings()
}

func (s *Store) SaveSystemNotificationSettings(input jfsettings.SystemNotificationSettings) (jfsettings.SystemNotificationSettings, error) {
	normalized := NormalizeSystemNotificationSettings(input)

	s.mu.Lock()
	s.data.SystemNotifications = systemNotificationSettingsPointer(normalized)
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
	previous := s.data.ADK
	s.data.ADK = adkRuntimeSettingsPointer(normalized)
	err := s.persistLocked()
	if err != nil {
		s.data.ADK = previous
	}
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
	return jfsettings.SecuritySettings{WebPort: jfsettings.DefaultWebAccessPort}
}

func NormalizeSecuritySettings(input jfsettings.SecuritySettings) jfsettings.SecuritySettings {
	settings := jfsettings.SecuritySettings{
		WebAccessEnabled:    input.WebAccessEnabled,
		PublicAccessEnabled: input.PublicAccessEnabled,
		WebPort:             input.WebPort,
		PasswordHash:        strings.TrimSpace(input.PasswordHash),
	}
	if settings.WebPort == 0 {
		settings.WebPort = jfsettings.DefaultWebAccessPort
	}
	settings.PasswordConfigured = settings.PasswordHash != ""
	if !settings.WebAccessEnabled || !settings.PasswordConfigured {
		settings.WebAccessEnabled = false
		settings.PublicAccessEnabled = false
	}
	return settings
}

func DefaultSystemNotificationSettings() jfsettings.SystemNotificationSettings {
	return jfsettings.SystemNotificationSettings{
		Enabled:      true,
		Mode:         "important",
		Levels:       []string{"warn", "error"},
		Categories:   []string{"broker.connection", "strategy.order.signal", "execution.order", "execution.fill"},
		SoundEnabled: true,
	}
}

func NormalizeSystemNotificationSettings(input jfsettings.SystemNotificationSettings) jfsettings.SystemNotificationSettings {
	defaults := DefaultSystemNotificationSettings()
	settings := input
	settings.Mode = normalizeSystemNotificationMode(settings.Mode)
	if settings.Mode == "" {
		settings.Mode = defaults.Mode
	}
	settings.Levels = normalizeStringList(settings.Levels, true)
	settings.Categories = normalizeStringList(settings.Categories, false)
	if settings.Mode == "important" {
		settings.Levels = append([]string(nil), defaults.Levels...)
		settings.Categories = append([]string(nil), defaults.Categories...)
	}
	if settings.Mode == "all" {
		settings.Levels = nil
		settings.Categories = nil
	}
	return settings
}

func normalizeSystemNotificationMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "important", "all", "custom":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeStringList(values []string, lower bool) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if lower {
			item = strings.ToLower(item)
		}
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized
}

func systemNotificationSettingsPointer(value jfsettings.SystemNotificationSettings) *jfsettings.SystemNotificationSettings {
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

func (s *Store) MCPServerSettings() jfsettings.MCPServerSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.MCPServer != nil {
		return NormalizeMCPServerSettings(jfsettings.MCPServerSettings{
			Enabled:   s.data.MCPServer.Enabled,
			Port:      s.data.MCPServer.Port,
			AuthMode:  s.data.MCPServer.AuthMode,
			TokenHash: s.data.MCPServer.TokenHash,
		})
	}
	return DefaultMCPServerSettings()
}

func (s *Store) SaveMCPServerSettings(input jfsettings.MCPServerSettings) (jfsettings.MCPServerSettings, error) {
	normalized := NormalizeMCPServerSettings(input)

	s.mu.Lock()
	previous := s.data.MCPServer
	s.data.MCPServer = &storedMCPServerSettings{
		Enabled:   normalized.Enabled,
		Port:      normalized.Port,
		AuthMode:  normalized.AuthMode,
		TokenHash: normalized.TokenHash,
	}
	err := s.persistLocked()
	if err != nil {
		s.data.MCPServer = previous
	}
	s.mu.Unlock()
	return normalized, err
}

func DefaultMCPServerSettings() jfsettings.MCPServerSettings {
	return jfsettings.MCPServerSettings{
		Enabled:  false,
		Port:     jfsettings.DefaultMCPServerPort,
		AuthMode: "token",
	}
}

func NormalizeMCPServerSettings(input jfsettings.MCPServerSettings) jfsettings.MCPServerSettings {
	settings := input
	if settings.Port == 0 || settings.Port < jfsettings.MinWebAccessPort || settings.Port > jfsettings.MaxWebAccessPort {
		settings.Port = jfsettings.DefaultMCPServerPort
	}
	switch strings.ToLower(strings.TrimSpace(settings.AuthMode)) {
	case "none":
		settings.AuthMode = "none"
	default:
		settings.AuthMode = "token"
	}
	settings.TokenConfigured = strings.TrimSpace(settings.TokenHash) != ""
	return settings
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
