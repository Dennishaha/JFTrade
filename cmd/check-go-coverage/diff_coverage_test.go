package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/cover"
)

type fakeGitRunner struct {
	args            []string
	calls           [][]string
	directory       string
	output          string
	stderr          string
	err             error
	untrackedOutput string
	untrackedStderr string
	untrackedErr    error
}

func (runner *fakeGitRunner) Run(directory string, args []string, stdout, stderr io.Writer) error {
	runner.directory = directory
	runner.args = append([]string(nil), args...)
	runner.calls = append(runner.calls, append([]string(nil), args...))
	if len(args) > 0 && args[0] == "ls-files" {
		_, _ = io.WriteString(stdout, runner.untrackedOutput)
		_, _ = io.WriteString(stderr, runner.untrackedStderr)
		return runner.untrackedErr
	}
	_, _ = io.WriteString(stdout, runner.output)
	_, _ = io.WriteString(stderr, runner.stderr)
	return runner.err
}

func TestParseChangedGoLinesCollectsAddedAndModifiedHunks(t *testing.T) {
	changed, err := parseChangedGoLines(`diff --git a/internal/ordinary/file.go b/internal/ordinary/file.go
index 1111111..2222222 100644
--- a/internal/ordinary/file.go
+++ b/internal/ordinary/file.go
@@ -2 +2,3 @@
 changed
@@ -10,2 +12 @@
 changed
diff --git a/pkg/futu/opend/client.go b/pkg/futu/opend/client.go
index 1111111..2222222 100644
--- a/pkg/futu/opend/client.go
+++ b/pkg/futu/opend/client.go
@@ -1 +1 @@
 changed
`)
	require.NoError(t, err)
	assert.Len(t, changed.files, 2)
	assert.Equal(t, []lineRange{{start: 2, end: 4}, {start: 12, end: 12}}, changed.lines["internal/ordinary/file.go"])
	assert.Equal(t, []lineRange{{start: 1, end: 1}}, changed.lines["pkg/futu/opend/client.go"])
}

func TestParseChangedGoLinesReportsPureRenameWithoutInventingChangedStatements(t *testing.T) {
	changed, err := parseChangedGoLines(`diff --git a/internal/old.go b/internal/new.go
similarity index 100%
rename from internal/old.go
rename to internal/new.go
`)
	require.NoError(t, err)
	assert.Equal(t, map[string]struct{}{"internal/new.go": {}}, changed.files)
	assert.Empty(t, changed.lines)
}

func TestParseChangedGoLinesRejectsMalformedHunk(t *testing.T) {
	_, err := parseChangedGoLines("+++ b/internal/file.go\n@@ malformed\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse git diff hunk")
}

func TestParseGitDiffPathSupportsQuotedPathsAndRejectsTraversal(t *testing.T) {
	fileName, err := parseGitDiffPath(`"b/internal/path with spaces.go"`)
	require.NoError(t, err)
	assert.Equal(t, "internal/path with spaces.go", fileName)

	_, err = parseGitDiffPath("b/../outside.go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid path")
}

func TestParseGitListedPathPreservesLeadingDiffPrefixDirectories(t *testing.T) {
	fileName, err := parseGitListedPath("a/internal/ordinary/untracked.go")
	require.NoError(t, err)
	assert.Equal(t, "a/internal/ordinary/untracked.go", fileName)

	fileName, err = parseGitListedPath("b/internal/ordinary/untracked.go")
	require.NoError(t, err)
	assert.Equal(t, "b/internal/ordinary/untracked.go", fileName)
}

func TestChangedGoLinesForRefBuildsWorkingTreeDiffCommand(t *testing.T) {
	runner := &fakeGitRunner{output: `diff --git a/internal/file.go b/internal/file.go
--- a/internal/file.go
+++ b/internal/file.go
@@ -1 +1 @@
changed
`}
	changed, err := changedGoLinesForRef("/repo", "origin/main", runner)
	require.NoError(t, err)
	assert.Equal(t, "/repo", runner.directory)
	assert.Equal(t, []string{
		"diff", "--no-ext-diff", "--unified=0", "--find-renames=50%", "origin/main", "--", ":(glob)**/*.go",
	}, runner.args)
	require.Len(t, runner.calls, 2)
	assert.Equal(t, []string{
		"ls-files", "--others", "--exclude-standard", "--", ":(glob)**/*.go",
	}, runner.calls[0])
	assert.Equal(t, []lineRange{{start: 1, end: 1}}, changed.lines["internal/file.go"])
}

func TestChangedGoLinesForRefIncludesGitDiagnostics(t *testing.T) {
	runner := &fakeGitRunner{err: errors.New("exit status 128"), stderr: "unknown revision"}
	_, err := changedGoLinesForRef("/repo", "missing", runner)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git diff against \"missing\"")
	assert.Contains(t, err.Error(), "unknown revision")
}

func TestAnalyzeDiffCoverageSeparatesOrdinaryAndCriticalPackages(t *testing.T) {
	profiles := []*cover.Profile{
		{
			FileName: "github.com/jftrade/jftrade-main/internal/ordinary/file.go",
			Blocks: []cover.ProfileBlock{
				{StartLine: 3, EndLine: 5, NumStmt: 3, Count: 1},
				{StartLine: 8, EndLine: 8, NumStmt: 2, Count: 0},
			},
		},
		{
			FileName: "github.com/jftrade/jftrade-main/pkg/futu/opend/client.go",
			Blocks: []cover.ProfileBlock{
				{StartLine: 10, EndLine: 11, NumStmt: 2, Count: 0},
			},
		},
		{
			FileName: "github.com/jftrade/jftrade-main/pkg/futu/pb/generated.go",
			Blocks: []cover.ProfileBlock{
				{StartLine: 4, EndLine: 4, NumStmt: 5, Count: 0},
			},
		},
	}
	runner := &fakeGitRunner{output: `diff --git a/internal/ordinary/file.go b/internal/ordinary/file.go
--- a/internal/ordinary/file.go
+++ b/internal/ordinary/file.go
@@ -1 +3,2 @@
changed
diff --git a/pkg/futu/opend/client.go b/pkg/futu/opend/client.go
--- a/pkg/futu/opend/client.go
+++ b/pkg/futu/opend/client.go
@@ -9 +10 @@
changed
diff --git a/pkg/futu/pb/generated.go b/pkg/futu/pb/generated.go
--- a/pkg/futu/pb/generated.go
+++ b/pkg/futu/pb/generated.go
@@ -3 +4 @@
changed
`}
	analysis, err := analyzeDiffCoverage("/repo", "base", profiles, runner)
	require.NoError(t, err)
	assert.Equal(t, 3, analysis.changedFiles)
	assert.Equal(t, coverageStats{covered: 3, total: 3}, analysis.ordinary)
	require.Equal(t, []scopeCoverage{{
		scope:         "pkg/futu/opend",
		domain:        "futu",
		coverageStats: coverageStats{total: 2},
	}}, analysis.critical)

	violations := evaluateDiffCoverage(analysis, config{diffThreshold: 90, criticalDiffThreshold: 95})
	require.Len(t, violations, 1)
	assert.Contains(t, violations[0], "pkg/futu/opend against base is 0.00%, below 95.00%")
}

func TestAnalyzeDiffCoverageFailsExecutableSourceWithoutProfile(t *testing.T) {
	repoRoot := t.TempDir()
	writeChangedGoSource(t, repoRoot, "internal/ordinary/missing.go", `package ordinary

// Config is intentionally type-only.
type Config struct{ Name string }

func Run(value string) string {
	return value
}
`)
	runner := &fakeGitRunner{output: `diff --git a/internal/ordinary/missing.go b/internal/ordinary/missing.go
--- a/internal/ordinary/missing.go
+++ b/internal/ordinary/missing.go
@@ -0,0 +1,8 @@
+package ordinary
+
+// Config is intentionally type-only.
+type Config struct{ Name string }
+
+func Run(value string) string {
+	return value
+}
`}

	analysis, err := analyzeDiffCoverage(repoRoot, "base", nil, runner)
	require.NoError(t, err)
	assert.Equal(t, []string{"internal/ordinary/missing.go"}, analysis.missingProfileFiles)

	violations := evaluateDiffCoverage(analysis, config{diffThreshold: 90, criticalDiffThreshold: 95})
	require.Len(t, violations, 1)
	assert.Contains(t, violations[0], "contains executable behavior but has no coverage profile data")

	var output bytes.Buffer
	require.NoError(t, printDiffCoverageReport(&output, analysis, config{}))
	assert.Contains(t, output.String(), "missing coverage profile for changed executable source internal/ordinary/missing.go")
}

func TestAnalyzeDiffCoverageAllowsTypeCommentImportAndTestOnlyChangesWithoutProfile(t *testing.T) {
	repoRoot := t.TempDir()
	writeChangedGoSource(t, repoRoot, "internal/ordinary/types.go", `package ordinary

import "time"

// Config documents a passive data shape.
type Config struct {
	CreatedAt time.Time
}
`)
	writeChangedGoSource(t, repoRoot, "internal/ordinary/service_test.go", `package ordinary

func TestRun(t *testing.T) {}
`)
	runner := &fakeGitRunner{output: `diff --git a/internal/ordinary/types.go b/internal/ordinary/types.go
--- a/internal/ordinary/types.go
+++ b/internal/ordinary/types.go
@@ -0,0 +1,8 @@
+package ordinary
+
+import "time"
+
+// Config documents a passive data shape.
+type Config struct {
+	CreatedAt time.Time
+}
diff --git a/internal/ordinary/service_test.go b/internal/ordinary/service_test.go
--- a/internal/ordinary/service_test.go
+++ b/internal/ordinary/service_test.go
@@ -0,0 +1,3 @@
+package ordinary
+
+func TestRun(t *testing.T) {}
`}

	analysis, err := analyzeDiffCoverage(repoRoot, "base", nil, runner)
	require.NoError(t, err)
	assert.Empty(t, analysis.missingProfileFiles)
	assert.Empty(t, evaluateDiffCoverage(analysis, config{}))
}

func TestAnalyzeDiffCoverageIncludesUntrackedExecutableGoSource(t *testing.T) {
	repoRoot := t.TempDir()
	writeChangedGoSource(t, repoRoot, "internal/ordinary/untracked.go", `package ordinary

func Run() {}
`)
	runner := &fakeGitRunner{untrackedOutput: "internal/ordinary/untracked.go\n"}

	analysis, err := analyzeDiffCoverage(repoRoot, "base", nil, runner)
	require.NoError(t, err)
	assert.Equal(t, 1, analysis.changedFiles)
	assert.Equal(t, []string{"internal/ordinary/untracked.go"}, analysis.missingProfileFiles)
	require.Len(t, runner.calls, 2)
	assert.Equal(t, "ls-files", runner.calls[0][0])
	assert.Equal(t, "diff", runner.calls[1][0])
}

func TestUntrackedGoFilesIncludesGitDiagnostics(t *testing.T) {
	runner := &fakeGitRunner{untrackedErr: errors.New("exit status 128"), untrackedStderr: "not a repository"}
	_, err := untrackedGoFiles("/repo", runner)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list untracked Go files")
	assert.Contains(t, err.Error(), "not a repository")
}

func TestEvaluateDiffCoverageGatesEachCriticalPackageSeparately(t *testing.T) {
	analysis := diffCoverageAnalysis{
		base: "base",
		critical: []scopeCoverage{
			{scope: "pkg/futu", domain: "futu", coverageStats: coverageStats{covered: 100, total: 100}},
			{scope: "pkg/futu/opend", domain: "futu", coverageStats: coverageStats{covered: 94, total: 100}},
		},
	}
	violations := evaluateDiffCoverage(analysis, config{diffThreshold: 90, criticalDiffThreshold: 95})
	require.Len(t, violations, 1)
	assert.Contains(t, violations[0], "pkg/futu/opend")
}

func TestPrintDiffCoverageReportHandlesNoExecutableStatementsAndCriticalScopes(t *testing.T) {
	t.Run("no executable statements", func(t *testing.T) {
		var output bytes.Buffer
		err := printDiffCoverageReport(&output, diffCoverageAnalysis{base: "base", changedFiles: 1}, config{
			diffThreshold:         90,
			criticalDiffThreshold: 95,
		})
		require.NoError(t, err)
		assert.Contains(t, output.String(), "n/a (0/0 changed executable statements across 1 changed Go files)")
	})

	t.Run("ordinary and critical", func(t *testing.T) {
		var output bytes.Buffer
		err := printDiffCoverageReport(&output, diffCoverageAnalysis{
			base:     "base",
			ordinary: coverageStats{covered: 9, total: 10},
			critical: []scopeCoverage{{
				scope:         "pkg/futu/opend",
				domain:        "futu",
				coverageStats: coverageStats{covered: 2, total: 2},
			}},
		}, config{diffThreshold: 90, criticalDiffThreshold: 95})
		require.NoError(t, err)
		assert.Contains(t, output.String(), "ordinary=90.00% (9/10 changed executable statements; threshold=90.00%)")
		assert.Contains(t, output.String(), "pkg/futu/opend [futu]")
	})

	t.Run("writer failure", func(t *testing.T) {
		err := printDiffCoverageReport(failingWriter{}, diffCoverageAnalysis{base: "base"}, config{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write diff coverage summary")
	})
}

func writeChangedGoSource(t *testing.T, repoRoot, fileName, source string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(fileName))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))
}
