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
	require.Len(t, analysis.critical, len(requiredCriticalScopes)+1)
	assert.Equal(t, scopeCoverage{
		scope:         "internal/api/backtest",
		domain:        "backtest",
		coverageStats: coverageStats{covered: 5, total: 5},
	}, coverageForScope(t, analysis.critical, "internal/api/backtest"))
	assert.Equal(t, scopeCoverage{
		scope:         "internal/api/backtest/sub",
		domain:        "backtest",
		coverageStats: coverageStats{total: 4},
	}, coverageForScope(t, analysis.critical, "internal/api/backtest/sub"))
	assert.Equal(t, []scopeCoverage{
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
	backtest := coverageForScope(t, analysis.critical, "internal/api/backtest")
	assert.Equal(t, 3, backtest.covered)
	assert.Equal(t, "backtest", backtest.domain)
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

func TestAnalyzeProfilesRetainsRequiredCriticalScopesWithoutProfileData(t *testing.T) {
	profiles := parseTestProfile(t, `mode: set
github.com/jftrade/jftrade-main/internal/ordinary/service.go:1.1,2.1 1 1
`)
	analysis, err := analyzeProfiles(profiles)
	require.NoError(t, err)
	require.Len(t, analysis.critical, len(requiredCriticalScopes))
	assert.Zero(t, coverageForScope(t, analysis.critical, "pkg/futu/opend").total)

	violations := evaluateCoverage(analysis, config{
		businessThreshold: 0,
		criticalThreshold: 95,
		moduleThreshold:   0,
	})
	assert.Contains(t, strings.Join(violations, "\n"), "critical package pkg/futu/opend has no coverage data")
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

func TestEvaluateCoverageUsesSingleOrdinaryThreshold(t *testing.T) {
	analysis := coverageAnalysis{
		business: coverageStats{covered: 100, total: 100},
		ordinary: []scopeCoverage{
			{scope: "internal/marketdata", coverageStats: coverageStats{covered: 85, total: 100}},
			{scope: "internal/integration/futu", coverageStats: coverageStats{covered: 85, total: 100}},
			{scope: "pkg/futu", coverageStats: coverageStats{covered: 85, total: 100}},
			{scope: "internal/api/marketdata", coverageStats: coverageStats{covered: 84, total: 100}},
		},
	}
	violations := evaluateCoverage(analysis, config{
		businessThreshold: 90,
		criticalThreshold: 95,
		moduleThreshold:   85,
	})
	require.Len(t, violations, 1)
	assert.Contains(t, violations[0], "internal/api/marketdata is 84.00%, below 85.00%")
}

func TestCriticalDomainForScopeUsesRiskPrefixesAndPackageBoundaries(t *testing.T) {
	tests := []struct {
		scope  string
		domain string
		found  bool
	}{
		{scope: "internal/trading", domain: "trading", found: true},
		{scope: "internal/api/trading/order", domain: "trading", found: true},
		{scope: "pkg/broker", domain: "trading", found: true},
		{scope: "internal/live", domain: "live", found: true},
		{scope: "internal/api/live/events", domain: "live", found: true},
		{scope: "internal/marketdata", domain: "marketdata", found: true},
		{scope: "pkg/market/us", domain: "marketdata", found: true},
		{scope: "internal/integration/futu", domain: "futu", found: true},
		{scope: "pkg/futu/opend", domain: "futu", found: true},
		{scope: "internal/api/backtest", domain: "backtest", found: true},
		{scope: "pkg/backtest/internal/storage", domain: "backtest", found: true},
		{scope: "internal/strategy/runtimecontrol", domain: "strategy", found: true},
		{scope: "pkg/strategy/pineworker", domain: "strategy", found: true},
		{scope: "internal/security/passwordhash", domain: "security", found: true},
		{scope: "internal/api/middleware", domain: "security", found: true},
		{scope: "internal/store/sqliteschema", domain: "schema-migration", found: true},
		{scope: "internal/app/apiserver/datamigration", domain: "schema-migration", found: true},
		{scope: "internal/marketdatastore"},
		{scope: "pkg/future"},
		{scope: "internal/settings"},
	}
	for _, test := range tests {
		t.Run(test.scope, func(t *testing.T) {
			domain, found := criticalDomainForScope(test.scope)
			assert.Equal(t, test.found, found)
			assert.Equal(t, test.domain, domain)
		})
	}
}

func TestEvaluateCoverageGatesEachCriticalPackageSeparately(t *testing.T) {
	analysis := coverageAnalysis{
		business: coverageStats{covered: 100, total: 100},
		critical: []scopeCoverage{
			{scope: "pkg/futu", domain: "futu", coverageStats: coverageStats{covered: 100, total: 100}},
			{scope: "pkg/futu/opend", domain: "futu", coverageStats: coverageStats{covered: 94, total: 100}},
		},
	}
	violations := evaluateCoverage(analysis, config{
		businessThreshold: 90,
		criticalThreshold: 95,
		moduleThreshold:   85,
	})
	require.Len(t, violations, 1)
	assert.Contains(t, violations[0], "pkg/futu/opend is 94.00%, below 95.00%")
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

func coverageForScope(t *testing.T, scopes []scopeCoverage, want string) scopeCoverage {
	t.Helper()
	for _, scope := range scopes {
		if scope.scope == want {
			return scope
		}
	}
	t.Fatalf("coverage scope %q not found", want)
	return scopeCoverage{}
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
