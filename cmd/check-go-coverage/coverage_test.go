package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/cover"
)

func TestAnalyzeProfilesAppliesScopesAndExclusions(t *testing.T) {
	profiles := parseTestProfile(t, `mode: set
github.com/jftrade/jftrade-main/internal/api/backtest/service.go:1.1,2.1 5 1
github.com/jftrade/jftrade-main/internal/api/backtest/sub/helper.go:1.1,2.1 4 0
github.com/jftrade/jftrade-main/internal/zeta/service.go:1.1,2.1 2 1
github.com/jftrade/jftrade-main/pkg/bbgo/generated.go:1.1,2.1 7 0
github.com/jftrade/jftrade-main/scripts/tool.go:1.1,2.1 3 0
`)
	analysis, err := analyzeProfiles(profiles)
	require.NoError(t, err)

	assert.Equal(t, coverageStats{covered: 7, total: 21}, analysis.raw)
	assert.Equal(t, coverageStats{covered: 7, total: 11}, analysis.business)
	assert.Equal(t, scopeCoverage{
		scope:         "internal/api/backtest",
		coverageStats: coverageStats{covered: 5, total: 5},
	}, analysis.critical[0])
	assert.Equal(t, []scopeCoverage{
		{scope: "internal/api/backtest/sub", coverageStats: coverageStats{total: 4}},
		{scope: "internal/zeta", coverageStats: coverageStats{covered: 2, total: 2}},
	}, analysis.ordinary)
	require.Len(t, analysis.excluded, len(exclusionRules))
	assert.Equal(t, coverageStats{total: 7}, analysis.excluded[3].coverageStats)
	assert.Equal(t, coverageStats{total: 3}, analysis.excluded[9].coverageStats)
}

func TestAnalyzeProfilesNormalizesWindowsSeparators(t *testing.T) {
	profiles := []*cover.Profile{{
		FileName: `github.com\jftrade\jftrade-main\internal\api\backtest\service.go`,
		Blocks:   []cover.ProfileBlock{{NumStmt: 3, Count: 1}},
	}}
	analysis, err := analyzeProfiles(profiles)
	require.NoError(t, err)
	assert.Equal(t, 3, analysis.critical[0].covered)
	assert.Empty(t, analysis.ordinary)
}

func TestCoverageStatsPercentageAndPackageScopeBoundaries(t *testing.T) {
	assert.Zero(t, (coverageStats{}).percentage())
	assert.Equal(t, 62.5, (coverageStats{covered: 5, total: 8}).percentage())

	for _, test := range []struct {
		name     string
		fileName string
		want     string
		ok       bool
	}{
		{name: "direct command", fileName: "cmd/jftrade-api/main.go", want: "cmd/jftrade-api", ok: true},
		{name: "direct package", fileName: "pkg/example/service.go", want: "pkg/example", ok: true},
		{name: "embedded internal path", fileName: "github.com/jftrade/jftrade-main/internal/example/service.go", want: "internal/example", ok: true},
		{name: "unsupported root", fileName: "docs/example.go"},
	} {
		t.Run(test.name, func(t *testing.T) {
			scope, ok := packageScope(test.fileName)
			assert.Equal(t, test.ok, ok)
			assert.Equal(t, test.want, scope)
		})
	}
}

func TestAnalyzeProfilesRejectsEmptyBusinessCoverage(t *testing.T) {
	_, err := analyzeProfiles(nil)
	assert.EqualError(t, err, "coverage profile contains no source files")

	profiles := parseTestProfile(t, `mode: set
github.com/jftrade/jftrade-main/cmd/generate-futu-proto/main.go:1.1,2.1 1 1
`)
	_, err = analyzeProfiles(profiles)
	assert.EqualError(t, err, "coverage profile contains no business statements")
}

func TestExclusionRulesAreExplicitAndDoNotHideBackendEntrypoints(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		category exclusionCategory
		excluded bool
	}{
		{name: "generated swagger", fileName: "github.com/jftrade/jftrade-main/docs/swagger/docs.go", category: exclusionGenerated, excluded: true},
		{name: "generated futu protobuf", fileName: "github.com/jftrade/jftrade-main/pkg/futu/pb/common/common.pb.go", category: exclusionGenerated, excluded: true},
		{name: "vendored bbgo", fileName: "github.com/jftrade/jftrade-main/pkg/bbgo/types/order.go", category: exclusionVendored, excluded: true},
		{name: "generator tooling", fileName: "github.com/jftrade/jftrade-main/cmd/generate-futu-proto/main.go", category: exclusionTooling, excluded: true},
		{name: "desktop adapter", fileName: "github.com/jftrade/jftrade-main/cmd/jftrade-desktop/main.go", category: exclusionDesktop, excluded: true},
		{name: "declarative OpenAPI routes", fileName: "github.com/jftrade/jftrade-main/internal/api/watchlist/openapi.go", category: exclusionContract, excluded: true},
		{name: "watchlist handlers", fileName: "github.com/jftrade/jftrade-main/internal/api/watchlist/routes.go"},
		{name: "api entrypoint", fileName: "github.com/jftrade/jftrade-main/cmd/jftrade-api/main.go"},
		{name: "build metadata", fileName: "github.com/jftrade/jftrade-main/internal/buildinfo/buildinfo.go"},
		{name: "frontend asset wrapper", fileName: "github.com/jftrade/jftrade-main/internal/frontendassets/dev.go"},
		{name: "pine worker asset wrapper", fileName: "github.com/jftrade/jftrade-main/internal/pineworkerassets/assets.go"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			index, excluded := exclusionIndex(test.fileName)
			assert.Equal(t, test.excluded, excluded)
			if excluded {
				assert.Equal(t, test.category, exclusionRules[index].category)
			}
		})
	}
}

func TestPackageScopeIncludesAPICommand(t *testing.T) {
	scope, ok := packageScope("github.com/jftrade/jftrade-main/cmd/jftrade-api/main.go")
	assert.True(t, ok)
	assert.Equal(t, "cmd/jftrade-api", scope)
}

func TestEvaluateCoverageAggregatesViolations(t *testing.T) {
	analysis := coverageAnalysis{
		business: coverageStats{covered: 89, total: 100},
		critical: []scopeCoverage{
			{scope: "internal/critical", coverageStats: coverageStats{covered: 94, total: 100}},
			{scope: "internal/missing"},
		},
		ordinary: []scopeCoverage{
			{scope: "internal/ordinary", coverageStats: coverageStats{covered: 84, total: 100}},
		},
	}
	violations := evaluateCoverage(analysis, config{
		businessThreshold: 90,
		criticalThreshold: 95,
		moduleThreshold:   85,
	})
	require.Len(t, violations, 4)
	assert.Contains(t, strings.Join(violations, "\n"), "business coverage 89.00%")
	assert.Contains(t, strings.Join(violations, "\n"), "internal/critical is 94.00%")
	assert.Contains(t, strings.Join(violations, "\n"), "internal/missing has no coverage data")
	assert.Contains(t, strings.Join(violations, "\n"), "internal/ordinary is 84.00%")
}

func TestEvaluateCoverageAllowsExactThresholds(t *testing.T) {
	analysis := coverageAnalysis{
		business: coverageStats{covered: 90, total: 100},
		critical: []scopeCoverage{{
			scope: "internal/critical", coverageStats: coverageStats{covered: 95, total: 100},
		}},
		ordinary: []scopeCoverage{{
			scope: "internal/ordinary", coverageStats: coverageStats{covered: 85, total: 100},
		}},
	}
	assert.Empty(t, evaluateCoverage(analysis, config{
		businessThreshold: 90,
		criticalThreshold: 95,
		moduleThreshold:   85,
	}))
}

func TestEvaluateCoverageAppliesExactLifecyclePackageThresholdOverrides(t *testing.T) {
	strict := []string{
		"internal/marketdata",
		"internal/integration/futu",
		"pkg/futu",
		"internal/api/marketdata",
	}
	ordinary := make([]scopeCoverage, 0, len(strict)+2)
	for _, scope := range strict {
		ordinary = append(ordinary, scopeCoverage{scope: scope, coverageStats: coverageStats{covered: 99, total: 100}})
	}
	ordinary = append(ordinary, scopeCoverage{scope: "internal/app/apiserver/servercore", coverageStats: coverageStats{covered: 94, total: 100}})
	ordinary = append(ordinary, scopeCoverage{scope: "internal/ordinary", coverageStats: coverageStats{covered: 85, total: 100}})
	analysis := coverageAnalysis{
		business: coverageStats{covered: 100, total: 100},
		ordinary: ordinary,
	}
	violations := evaluateCoverage(analysis, config{
		businessThreshold: 90,
		criticalThreshold: 95,
		moduleThreshold:   85,
	})
	require.Len(t, violations, len(strict)+1)
	for _, scope := range strict {
		assert.Contains(t, strings.Join(violations, "\n"), "ordinary Go coverage for "+scope+" is 99.00%, below 100.00%")
	}
	assert.Contains(t, strings.Join(violations, "\n"), "ordinary Go coverage for internal/app/apiserver/servercore is 94.00%, below 95.00%")

	for index := range strict {
		analysis.ordinary[index].covered = 100
	}
	analysis.ordinary[len(strict)].covered = 95
	assert.Empty(t, evaluateCoverage(analysis, config{
		businessThreshold: 90,
		criticalThreshold: 95,
		moduleThreshold:   85,
	}))
}

func TestPrintCoverageReportIncludesMissingAndSortedScopes(t *testing.T) {
	analysis := coverageAnalysis{
		raw:      coverageStats{covered: 9, total: 10},
		business: coverageStats{covered: 8, total: 10},
		critical: []scopeCoverage{{scope: "internal/missing"}},
		excluded: []excludedScopeCoverage{{
			rule:          exclusionRule{scope: "pkg/example/generated", category: exclusionGenerated, reason: "generated test fixture"},
			coverageStats: coverageStats{covered: 1, total: 2},
		}},
		ordinary: []scopeCoverage{
			{scope: "internal/a", coverageStats: coverageStats{covered: 1, total: 2}},
			{scope: "internal/z", coverageStats: coverageStats{covered: 2, total: 2}},
		},
	}
	var output bytes.Buffer
	require.NoError(t, printCoverageReport(&output, analysis, config{businessThreshold: 90}))
	assert.Contains(t, output.String(), "raw=90.00%")
	assert.Contains(t, output.String(), "Excluded Go coverage: generated  pkg/example/generated")
	assert.Contains(t, output.String(), "internal/missing                           n/a (0/0)")
	assert.Less(t, strings.Index(output.String(), "internal/a"), strings.Index(output.String(), "internal/z"))
}

func TestPrintCoverageReportReturnsWriterErrorsForEachSection(t *testing.T) {
	analysis := coverageAnalysis{
		raw:      coverageStats{covered: 10, total: 10},
		business: coverageStats{covered: 10, total: 10},
		excluded: []excludedScopeCoverage{{
			rule:          exclusionRule{scope: "pkg/generated", category: exclusionGenerated, reason: "generated fixture"},
			coverageStats: coverageStats{covered: 1, total: 1},
		}},
		critical: []scopeCoverage{
			{scope: "internal/missing"},
			{scope: "internal/covered", coverageStats: coverageStats{covered: 1, total: 1}},
		},
		ordinary: []scopeCoverage{{scope: "internal/ordinary", coverageStats: coverageStats{covered: 1, total: 1}}},
	}
	for _, test := range []struct {
		name          string
		allowedWrites int
		want          string
	}{
		{name: "excluded", allowedWrites: 1, want: "write excluded coverage"},
		{name: "missing critical", allowedWrites: 2, want: "write critical coverage"},
		{name: "covered critical", allowedWrites: 3, want: "write critical coverage"},
		{name: "ordinary", allowedWrites: 4, want: "write ordinary coverage"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := printCoverageReport(&failAfterWrites{remaining: test.allowedWrites}, analysis, config{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func parseTestProfile(t *testing.T, profile string) []*cover.Profile {
	t.Helper()
	profiles, err := cover.ParseProfilesFromReader(strings.NewReader(profile))
	require.NoError(t, err)
	return profiles
}

type failAfterWrites struct {
	remaining int
}

func (writer *failAfterWrites) Write(data []byte) (int, error) {
	if writer.remaining == 0 {
		return 0, errors.New("output unavailable")
	}
	writer.remaining--
	return len(data), nil
}
