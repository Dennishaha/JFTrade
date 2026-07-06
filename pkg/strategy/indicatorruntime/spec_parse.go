package indicatorruntime

import (
	"regexp"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

var indicatorKeyPattern = regexp.MustCompile(`ctx\.indicators\[(?:"([^"]+)"|'([^']+)')\]`)

func parseIndicatorRequirements(script string) indicatorRequirements {
	keys := make([]string, 0)
	for _, match := range indicatorKeyPattern.FindAllStringSubmatch(script, -1) {
		key := strings.TrimSpace(firstNonEmpty(match[1], match[2]))
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}

	requirements, jftradeErr1 := parseIndicatorRequirementKeys(keys, false)
	jftradeLogError(jftradeErr1)
	return requirements
}

func indicatorRequirementsFromPlan(plan strategyir.Requirements) (indicatorRequirements, error) {
	keys := make([]string, 0, len(plan.Indicators))
	for _, requirement := range plan.Indicators {
		key := strings.TrimSpace(requirement.Key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}

	return parseIndicatorRequirementKeys(keys, true)
}

func parseIndicatorRequirementKeys(keys []string, strict bool) (indicatorRequirements, error) {
	builder := newIndicatorRequirementSetBuilder(strict)
	for _, rawKey := range keys {
		if err := builder.parseKey(rawKey); err != nil {
			return indicatorRequirements{}, err
		}
	}
	return builder.build(), nil
}
