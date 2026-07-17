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

var moduleThresholdOverrides = map[string]float64{
	"internal/marketdata":               100.0,
	"internal/integration/futu":         100.0,
	"pkg/futu":                          100.0,
	"internal/api/marketdata":           100.0,
	"internal/app/apiserver/servercore": 95.0,
}

type exclusionCategory string

const (
	exclusionGenerated exclusionCategory = "generated"
	exclusionVendored  exclusionCategory = "vendored"
	exclusionTooling   exclusionCategory = "tooling"
	exclusionDesktop   exclusionCategory = "desktop"
)

type exclusionRule struct {
	scope    string
	category exclusionCategory
	reason   string
}

// exclusionRules intentionally name only code that is not owned backend
// behavior. Product-owned internal packages and the API command are covered.
// Keeping these rules explicit prevents a broad directory prefix from quietly
// removing new production code from the coverage gate.
var exclusionRules = []exclusionRule{
	{scope: "docs/swagger", category: exclusionGenerated, reason: "generated Swagger document"},
	{scope: "pkg/futu/pb", category: exclusionGenerated, reason: "generated Futu OpenD protobuf bindings"},
	{scope: "pkg/strategy/pineworker/pineworkerpb", category: exclusionGenerated, reason: "generated Pine worker protobuf bindings"},
	{scope: "pkg/bbgo", category: exclusionVendored, reason: "vendored upstream bbgo components"},
	{scope: "cmd/check-go-coverage", category: exclusionTooling, reason: "coverage gate implementation"},
	{scope: "cmd/generate-futu-proto", category: exclusionTooling, reason: "protobuf generator"},
	{scope: "cmd/generate-pine-spec-docs", category: exclusionTooling, reason: "documentation generator"},
	{scope: "cmd/generate-pineworker-proto", category: exclusionTooling, reason: "protobuf generator"},
	{scope: "cmd/internal/protogen", category: exclusionTooling, reason: "protobuf generator support"},
	{scope: "scripts", category: exclusionTooling, reason: "repository maintenance scripts"},
	{scope: "cmd/jftrade-desktop", category: exclusionDesktop, reason: "desktop client delivery adapter"},
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

type excludedScopeCoverage struct {
	rule exclusionRule
	coverageStats
}

type coverageAnalysis struct {
	raw      coverageStats
	business coverageStats
	critical []scopeCoverage
	ordinary []scopeCoverage
	excluded []excludedScopeCoverage
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
	excluded := make([]excludedScopeCoverage, len(exclusionRules))
	for index, rule := range exclusionRules {
		excluded[index].rule = rule
	}
	analysis := coverageAnalysis{critical: critical, excluded: excluded}

	for _, profile := range profiles {
		stats := profileCoverage(profile)
		analysis.raw.add(stats)
		fileName := normalizeProfilePath(profile.FileName)
		if index, excluded := exclusionIndex(fileName); excluded {
			analysis.excluded[index].add(stats)
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

func exclusionIndex(fileName string) (int, bool) {
	for index, rule := range exclusionRules {
		if containsScope(fileName, rule.scope) {
			return index, true
		}
	}
	return 0, false
}

func containsScope(fileName, scope string) bool {
	return strings.Contains("/"+fileName+"/", "/"+scope+"/")
}

func packageScope(fileName string) (string, bool) {
	for _, root := range []string{"cmd", "internal", "pkg"} {
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
		threshold := cfg.moduleThreshold
		if override, ok := moduleThresholdOverrides[scope.scope]; ok {
			threshold = override
		}
		if actual := scope.percentage(); actual < threshold {
			violations = append(violations, fmt.Sprintf(
				"ordinary Go coverage for %s is %.2f%%, below %.2f%%",
				scope.scope, actual, threshold,
			))
		}
	}
	return violations
}

func printCoverageReport(writer io.Writer, analysis coverageAnalysis, cfg config) error {
	if _, err := fmt.Fprintf(writer, "Go coverage: raw=%.2f%% (%d/%d statements) business=%.2f%% (%d/%d statements) threshold=%.2f%%\n",
		analysis.raw.percentage(), analysis.raw.covered, analysis.raw.total,
		analysis.business.percentage(), analysis.business.covered, analysis.business.total,
		cfg.businessThreshold,
	); err != nil {
		return fmt.Errorf("write coverage summary: %w", err)
	}
	for _, scope := range analysis.excluded {
		if scope.total == 0 {
			continue
		}
		if _, err := fmt.Fprintf(writer, "Excluded Go coverage: %-10s %-42s %.2f%% (%d/%d) %s\n",
			scope.rule.category,
			scope.rule.scope,
			scope.percentage(),
			scope.covered,
			scope.total,
			scope.rule.reason,
		); err != nil {
			return fmt.Errorf("write excluded coverage: %w", err)
		}
	}
	for _, scope := range analysis.critical {
		if scope.total == 0 {
			if _, err := fmt.Fprintf(writer, "Critical Go coverage: %-42s n/a (0/0)\n", scope.scope); err != nil {
				return fmt.Errorf("write critical coverage: %w", err)
			}
			continue
		}
		if _, err := fmt.Fprintf(writer, "Critical Go coverage: %-42s %.2f%% (%d/%d)\n",
			scope.scope, scope.percentage(), scope.covered, scope.total,
		); err != nil {
			return fmt.Errorf("write critical coverage: %w", err)
		}
	}
	for _, scope := range analysis.ordinary {
		if _, err := fmt.Fprintf(writer, "Ordinary Go coverage: %-42s %.2f%% (%d/%d)\n",
			scope.scope, scope.percentage(), scope.covered, scope.total,
		); err != nil {
			return fmt.Errorf("write ordinary coverage: %w", err)
		}
	}
	return nil
}
