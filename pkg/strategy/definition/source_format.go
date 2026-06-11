package definition

import (
	"fmt"
	"strings"

	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

const (
	SourceFormatPineV6 = strategypine.SourceFormatPineV6
)

func NormalizeSourceFormat(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return SourceFormatPineV6
	}
	return normalized
}

func ValidateScript(sourceFormat string, script string) error {
	switch normalized := NormalizeSourceFormat(sourceFormat); normalized {
	case SourceFormatPineV6:
		return strategypine.ValidateScript(script)
	default:
		return fmt.Errorf("unsupported strategy source format: %s", normalized)
	}
}

func SupportsInstantiation(sourceFormat string) bool {
	switch NormalizeSourceFormat(sourceFormat) {
	case SourceFormatPineV6:
		return true
	default:
		return false
	}
}
