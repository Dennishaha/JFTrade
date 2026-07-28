package indicatorwarmup

import (
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func indicatorRequirementsFromPlan(plan strategyir.Requirements) (indicatorRequirements, error) {
	keys := make([]string, 0, len(plan.Indicators))
	for _, requirement := range plan.Indicators {
		key := strings.TrimSpace(requirement.Key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}

	return parseIndicatorRequirementKeys(keys)
}

func parseIndicatorRequirementKeys(keys []string) (indicatorRequirements, error) {
	builder := newIndicatorRequirementSetBuilder()
	for _, rawKey := range keys {
		if err := builder.parseKey(rawKey); err != nil {
			return indicatorRequirements{}, err
		}
	}
	return builder.build(), nil
}
