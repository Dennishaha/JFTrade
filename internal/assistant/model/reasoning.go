package model

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

var providerReasoningEffortOrder = []ReasoningEffort{
	ReasoningEffortLow,
	ReasoningEffortMedium,
	ReasoningEffortHigh,
	ReasoningEffortXHigh,
	ReasoningEffortMax,
}

func NormalizeReasoningEffort(value ReasoningEffort) ReasoningEffort {
	normalized := ReasoningEffort(strings.ToLower(strings.TrimSpace(string(value))))
	switch normalized {
	case ReasoningEffortLow, ReasoningEffortMedium, ReasoningEffortHigh,
		ReasoningEffortXHigh, ReasoningEffortMax:
		return normalized
	default:
		return ""
	}
}

func NormalizeOptionalReasoningEffort(value ReasoningEffort) ReasoningEffort {
	if strings.TrimSpace(string(value)) == "" {
		return ""
	}
	return NormalizeReasoningEffort(value)
}

func ValidateOptionalReasoningEffort(value ReasoningEffort) error {
	normalized := ReasoningEffort(strings.ToLower(strings.TrimSpace(string(value))))
	switch normalized {
	case "", ReasoningEffortLow, ReasoningEffortMedium, ReasoningEffortHigh,
		ReasoningEffortXHigh, ReasoningEffortMax:
		return nil
	default:
		return fmt.Errorf("invalid reasoning effort %q", value)
	}
}

func NormalizeProviderTestMode(mode ProviderTestMode) (ProviderTestMode, error) {
	mode = ProviderTestMode(strings.ToLower(strings.TrimSpace(string(mode))))
	if mode == "" {
		return ProviderTestModeQuick, nil
	}
	if mode != ProviderTestModeQuick && mode != ProviderTestModeFull {
		return "", fmt.Errorf("invalid provider test mode %q", mode)
	}
	return mode, nil
}

// DefaultProviderReasoningConfig supplies the Responses request field.
// Supported levels are always opt-in through explicit mappings.
func DefaultProviderReasoningConfig() ProviderReasoningConfig {
	return ProviderReasoningConfig{RequestField: "reasoning.effort", Mappings: []ProviderReasoningMapping{}}
}

func NormalizeProviderReasoningConfig(config ProviderReasoningConfig) ProviderReasoningConfig {
	if strings.TrimSpace(config.RequestField) == "" {
		config.RequestField = DefaultProviderReasoningConfig().RequestField
	} else {
		config.RequestField = strings.TrimSpace(config.RequestField)
	}
	if config.Mappings == nil {
		config.Mappings = []ProviderReasoningMapping{}
	}
	for index := range config.Mappings {
		config.Mappings[index].Effort = ReasoningEffort(strings.ToLower(strings.TrimSpace(string(config.Mappings[index].Effort))))
		config.Mappings[index].Value = strings.TrimSpace(config.Mappings[index].Value)
	}
	sort.SliceStable(config.Mappings, func(left, right int) bool {
		return reasoningEffortIndex(config.Mappings[left].Effort) < reasoningEffortIndex(config.Mappings[right].Effort)
	})
	return config
}

func ProviderReasoningMappingValue(config ProviderReasoningConfig, effort ReasoningEffort) (string, bool) {
	effort = NormalizeReasoningEffort(effort)
	if effort == "" {
		return "", false
	}
	for _, mapping := range config.Mappings {
		if mapping.Effort == effort {
			return mapping.Value, true
		}
	}
	return "", false
}

func ResolveProviderReasoning(provider Provider, effort ReasoningEffort) (string, string, error) {
	rawEffort := strings.TrimSpace(string(effort))
	if rawEffort == "" {
		return "", "", nil
	}
	effort = NormalizeReasoningEffort(effort)
	if effort == "" {
		return "", "", fmt.Errorf("invalid reasoning effort %q", rawEffort)
	}
	config := NormalizeProviderReasoningConfig(provider.ReasoningConfig)
	if err := ValidateProviderReasoningConfig(config); err != nil {
		return "", "", err
	}
	value, supported := ProviderReasoningMappingValue(config, effort)
	if !supported {
		return "", "", fmt.Errorf("%w: %s", ErrProviderReasoningUnsupported, effort)
	}
	return config.RequestField, value, nil
}

func ValidateProviderReasoningConfig(config ProviderReasoningConfig) error {
	if err := validateProviderReasoningField(config.RequestField); err != nil {
		return err
	}
	seen := make(map[ReasoningEffort]struct{}, len(config.Mappings))
	for _, mapping := range config.Mappings {
		effort := NormalizeReasoningEffort(mapping.Effort)
		if effort == "" {
			return fmt.Errorf("invalid provider reasoning effort %q", mapping.Effort)
		}
		if _, exists := seen[effort]; exists {
			return fmt.Errorf("duplicate provider reasoning effort %q", effort)
		}
		seen[effort] = struct{}{}
		value := strings.TrimSpace(mapping.Value)
		if value == "" {
			return fmt.Errorf("provider reasoning value for %q is required", effort)
		}
		if len([]rune(value)) > 128 {
			return fmt.Errorf("provider reasoning value for %q is too long", effort)
		}
		for _, character := range value {
			if unicode.IsControl(character) {
				return fmt.Errorf("provider reasoning value for %q contains a control character", effort)
			}
		}
	}
	return nil
}

func validateProviderReasoningField(field string) error {
	field = strings.TrimSpace(field)
	if field == "" {
		return fmt.Errorf("provider reasoning request field is required")
	}
	if len([]rune(field)) > 128 {
		return fmt.Errorf("provider reasoning request field is too long")
	}
	reserved := map[string]struct{}{
		"model": {}, "messages": {}, "input": {}, "stream": {}, "tools": {},
		"tool_choice": {}, "temperature": {},
	}
	segments := strings.Split(field, ".")
	for index, segment := range segments {
		if segment == "" {
			return fmt.Errorf("provider reasoning request field contains an empty segment")
		}
		if index == 0 {
			if _, blocked := reserved[segment]; blocked {
				return fmt.Errorf("provider reasoning request field %q conflicts with request field %q", field, segment)
			}
		}
		for characterIndex, character := range segment {
			valid := character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || characterIndex > 0 && character >= '0' && character <= '9'
			if !valid || characterIndex == 0 && character >= '0' && character <= '9' {
				return fmt.Errorf("provider reasoning request field %q is not a dot path", field)
			}
		}
	}
	return nil
}

func reasoningEffortIndex(value ReasoningEffort) int {
	for index, effort := range providerReasoningEffortOrder {
		if effort == value {
			return index
		}
	}
	return len(providerReasoningEffortOrder) + 1
}
