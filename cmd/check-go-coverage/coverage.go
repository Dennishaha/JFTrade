package main

import (
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"golang.org/x/tools/cover"
)

var criticalScopes = []string{
	"internal/api/backtest",
	"internal/api/httpserver",
	"internal/api/live",
	"internal/api/middleware",
	"internal/api/settings",
	"internal/api/system",
	"internal/app/apiserver/lifecycle",
	"internal/store/sqliteschema",
	"pkg/futu/opend",
	"pkg/strategy/ir",
	"pkg/strategy/pineworker",
}

var excludedScopes = []string{
	"cmd",
	"docs/swagger",
	"scripts",
	"internal/buildinfo",
	"internal/frontendassets",
	"internal/pineworkerassets",
	"pkg/bbgo",
	"pkg/futu/pb",
	"pkg/strategy/pineworker/pineworkerpb",
}

type coverageStats struct {
	covered int
	total   int
}

func (s coverageStats) percentage() float64 {
	if s.total == 0 {
		return 0
	}
	return float64(s.covered) * 100 / float64(s.total)
}

func (s *coverageStats) add(other coverageStats) {
	s.covered += other.covered
	s.total += other.total
}

type scopeCoverage struct {
	scope string
	coverageStats
}

type coverageAnalysis struct {
	raw      coverageStats
	business coverageStats
	critical []scopeCoverage
	ordinary []scopeCoverage
}

func analyzeProfiles(profiles []*cover.Profile) (coverageAnalysis, error) {
	if len(profiles) == 0 {
		return coverageAnalysis{}, errors.New("coverage profile contains no source files")
	}

	criticalIndex := make(map[string]int, len(criticalScopes))
	critical := make([]scopeCoverage, len(criticalScopes))
	for index, scope := range criticalScopes {
		criticalIndex[scope] = index
		critical[index].scope = scope
	}
	ordinary := make(map[string]coverageStats)
	analysis := coverageAnalysis{critical: critical}

	for _, profile := range profiles {
		stats := profileCoverage(profile)
		analysis.raw.add(stats)
		fileName := normalizeProfilePath(profile.FileName)
		if isExcluded(fileName) {
			continue
		}
		analysis.business.add(stats)

		scope, ok := packageScope(fileName)
		if !ok {
			continue
		}
		if index, found := criticalIndex[scope]; found {
			analysis.critical[index].add(stats)
			continue
		}
		moduleStats := ordinary[scope]
		moduleStats.add(stats)
		ordinary[scope] = moduleStats
	}
	if analysis.business.total == 0 {
		return coverageAnalysis{}, errors.New("coverage profile contains no business statements")
	}

	scopes := make([]string, 0, len(ordinary))
	for scope := range ordinary {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	for _, scope := range scopes {
		analysis.ordinary = append(analysis.ordinary, scopeCoverage{scope: scope, coverageStats: ordinary[scope]})
	}
	return analysis, nil
}

func profileCoverage(profile *cover.Profile) coverageStats {
	var stats coverageStats
	for _, block := range profile.Blocks {
		stats.total += block.NumStmt
		if block.Count > 0 {
			stats.covered += block.NumStmt
		}
	}
	return stats
}

func normalizeProfilePath(fileName string) string {
	return strings.Trim(strings.ReplaceAll(fileName, "\\", "/"), "/")
}

func isExcluded(fileName string) bool {
	for _, scope := range excludedScopes {
		if containsScope(fileName, scope) {
			return true
		}
	}
	return false
}

func containsScope(fileName, scope string) bool {
	return strings.Contains("/"+fileName+"/", "/"+scope+"/")
}

func packageScope(fileName string) (string, bool) {
	for _, root := range []string{"internal", "pkg"} {
		marker := "/" + root + "/"
		if strings.HasPrefix(fileName, root+"/") {
			return path.Dir(fileName), true
		}
		if index := strings.Index(fileName, marker); index >= 0 {
			return path.Dir(fileName[index+1:]), true
		}
	}
	return "", false
}

func evaluateCoverage(analysis coverageAnalysis, cfg config) []string {
	var violations []string
	if actual := analysis.business.percentage(); actual < cfg.businessThreshold {
		violations = append(violations, fmt.Sprintf(
			"Go business coverage %.2f%% is below %.2f%%", actual, cfg.businessThreshold,
		))
	}
	for _, scope := range analysis.critical {
		if scope.total == 0 {
			violations = append(violations, fmt.Sprintf("critical package %s has no coverage data", scope.scope))
			continue
		}
		if actual := scope.percentage(); actual < cfg.criticalThreshold {
			violations = append(violations, fmt.Sprintf(
				"critical Go coverage for %s is %.2f%%, below %.2f%%",
				scope.scope, actual, cfg.criticalThreshold,
			))
		}
	}
	for _, scope := range analysis.ordinary {
		if actual := scope.percentage(); actual < cfg.moduleThreshold {
			violations = append(violations, fmt.Sprintf(
				"ordinary Go coverage for %s is %.2f%%, below %.2f%%",
				scope.scope, actual, cfg.moduleThreshold,
			))
		}
	}
	return violations
}

func printCoverageReport(writer io.Writer, analysis coverageAnalysis, cfg config) {
	fmt.Fprintf(writer, "Go coverage: raw=%.2f%% (%d/%d statements) business=%.2f%% (%d/%d statements) threshold=%.2f%%\n",
		analysis.raw.percentage(), analysis.raw.covered, analysis.raw.total,
		analysis.business.percentage(), analysis.business.covered, analysis.business.total,
		cfg.businessThreshold,
	)
	for _, scope := range analysis.critical {
		if scope.total == 0 {
			fmt.Fprintf(writer, "Critical Go coverage: %-42s n/a (0/0)\n", scope.scope)
			continue
		}
		fmt.Fprintf(writer, "Critical Go coverage: %-42s %.2f%% (%d/%d)\n",
			scope.scope, scope.percentage(), scope.covered, scope.total,
		)
	}
	for _, scope := range analysis.ordinary {
		fmt.Fprintf(writer, "Ordinary Go coverage: %-42s %.2f%% (%d/%d)\n",
			scope.scope, scope.percentage(), scope.covered, scope.total,
		)
	}
}
