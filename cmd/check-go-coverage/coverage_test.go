package main

import (
	"bytes"
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

func TestAnalyzeProfilesRejectsEmptyBusinessCoverage(t *testing.T) {
	_, err := analyzeProfiles(nil)
	assert.EqualError(t, err, "coverage profile contains no source files")

	profiles := parseTestProfile(t, `mode: set
github.com/jftrade/jftrade-main/cmd/tool/main.go:1.1,2.1 1 1
`)
	_, err = analyzeProfiles(profiles)
	assert.EqualError(t, err, "coverage profile contains no business statements")
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

func TestEvaluateCoverageAppliesFutuModuleThresholdOverride(t *testing.T) {
	analysis := coverageAnalysis{
		business: coverageStats{covered: 100, total: 100},
		ordinary: []scopeCoverage{
			{scope: "pkg/futu", coverageStats: coverageStats{covered: 89, total: 100}},
			{scope: "internal/ordinary", coverageStats: coverageStats{covered: 85, total: 100}},
		},
	}
	violations := evaluateCoverage(analysis, config{
		businessThreshold: 90,
		criticalThreshold: 95,
		moduleThreshold:   85,
	})
	require.Equal(t, []string{"ordinary Go coverage for pkg/futu is 89.00%, below 90.00%"}, violations)

	analysis.ordinary[0].covered = 90
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
		ordinary: []scopeCoverage{
			{scope: "internal/a", coverageStats: coverageStats{covered: 1, total: 2}},
			{scope: "internal/z", coverageStats: coverageStats{covered: 2, total: 2}},
		},
	}
	var output bytes.Buffer
	require.NoError(t, printCoverageReport(&output, analysis, config{businessThreshold: 90}))
	assert.Contains(t, output.String(), "raw=90.00%")
	assert.Contains(t, output.String(), "internal/missing                           n/a (0/0)")
	assert.Less(t, strings.Index(output.String(), "internal/a"), strings.Index(output.String(), "internal/z"))
}

func parseTestProfile(t *testing.T, profile string) []*cover.Profile {
	t.Helper()
	profiles, err := cover.ParseProfilesFromReader(strings.NewReader(profile))
	require.NoError(t, err)
	return profiles
}
