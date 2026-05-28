package definition

import (
	"fmt"
	"strings"

	strategydsl "github.com/jftrade/jftrade-main/pkg/strategy/dsl"
)

const (
	SourceFormatDSLV1 = "dsl-v1"
)

func NormalizeSourceFormat(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return SourceFormatDSLV1
	}
	return normalized
}

func ValidateScript(sourceFormat string, script string) error {
	switch normalized := NormalizeSourceFormat(sourceFormat); normalized {
	case SourceFormatDSLV1:
		return strategydsl.ValidateScript(script)
	default:
		return fmt.Errorf("unsupported strategy source format: %s", normalized)
	}
}

func SupportsInstantiation(sourceFormat string) bool {
	switch NormalizeSourceFormat(sourceFormat) {
	case SourceFormatDSLV1:
		return true
	default:
		return false
	}
}
