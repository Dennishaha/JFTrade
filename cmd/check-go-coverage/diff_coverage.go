package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/cover"
)

type gitRunner interface {
	Run(directory string, args []string, stdout, stderr io.Writer) error
}

type execGitRunner struct{}

func (execGitRunner) Run(directory string, args []string, stdout, stderr io.Writer) error {
	command := exec.Command("git", args...)
	command.Dir = directory
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

type lineRange struct {
	start int
	end   int
}

type changedGoLines struct {
	files map[string]struct{}
	lines map[string][]lineRange
}

type diffCoverageAnalysis struct {
	base                string
	changedFiles        int
	ordinary            coverageStats
	critical            []scopeCoverage
	missingProfileFiles []string
}

var changedLinesHunkPattern = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

func analyzeDiffCoverage(
	repoRoot string,
	diffBase string,
	profiles []*cover.Profile,
	runner gitRunner,
) (diffCoverageAnalysis, error) {
	changed, err := changedGoLinesForRef(repoRoot, diffBase, runner)
	if err != nil {
		return diffCoverageAnalysis{}, err
	}
	analysis := diffCoverageAnalysis{
		base:         diffBase,
		changedFiles: len(changed.files),
	}
	critical := make(map[string]scopeCoverage)
	profileFiles := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		fileName := normalizeProfilePath(profile.FileName)
		relativeFileName := repoRelativeProfilePath(fileName)
		profileFiles[relativeFileName] = struct{}{}
		lineRanges, found := changed.lines[relativeFileName]
		if !found {
			continue
		}
		if _, excluded := exclusionIndex(fileName); excluded {
			continue
		}

		criticalScope := ""
		criticalDomain := ""
		if scope, ok := packageScope(fileName); ok {
			if domain, isCritical := criticalDomainForScope(scope); isCritical {
				criticalScope = scope
				criticalDomain = domain
			}
		}
		for _, block := range profile.Blocks {
			if block.NumStmt == 0 || !blockOverlapsChangedLines(block, lineRanges) {
				continue
			}
			stats := coverageStats{total: block.NumStmt}
			if block.Count > 0 {
				stats.covered = block.NumStmt
			}
			if criticalScope != "" {
				criticalStats := critical[criticalScope]
				criticalStats.scope = criticalScope
				criticalStats.domain = criticalDomain
				criticalStats.add(stats)
				critical[criticalScope] = criticalStats
				continue
			}
			analysis.ordinary.add(stats)
		}
	}
	criticalScopes := make([]string, 0, len(critical))
	for scope := range critical {
		criticalScopes = append(criticalScopes, scope)
	}
	sort.Strings(criticalScopes)
	for _, scope := range criticalScopes {
		analysis.critical = append(analysis.critical, critical[scope])
	}
	missingProfileFiles, err := changedExecutableFilesWithoutProfile(repoRoot, changed, profileFiles)
	if err != nil {
		return diffCoverageAnalysis{}, err
	}
	analysis.missingProfileFiles = missingProfileFiles
	return analysis, nil
}

func changedGoLinesForRef(repoRoot, diffBase string, runner gitRunner) (changedGoLines, error) {
	untrackedFiles, err := untrackedGoFiles(repoRoot, runner)
	if err != nil {
		return changedGoLines{}, err
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{
		"diff",
		"--no-ext-diff",
		"--unified=0",
		"--find-renames=50%",
		diffBase,
		"--",
		":(glob)**/*.go",
	}
	if err := runner.Run(repoRoot, args, &stdout, &stderr); err != nil {
		if details := strings.TrimSpace(stderr.String()); details != "" {
			return changedGoLines{}, fmt.Errorf("git diff against %q: %w: %s", diffBase, err, details)
		}
		return changedGoLines{}, fmt.Errorf("git diff against %q: %w", diffBase, err)
	}
	changed, err := parseChangedGoLines(stdout.String())
	if err != nil {
		return changedGoLines{}, fmt.Errorf("parse git diff against %q: %w", diffBase, err)
	}
	for _, fileName := range untrackedFiles {
		changed.files[fileName] = struct{}{}
		changed.lines[fileName] = append(changed.lines[fileName], lineRange{start: 1, end: int(^uint(0) >> 1)})
	}
	for fileName, lineRanges := range changed.lines {
		changed.lines[fileName] = mergeLineRanges(lineRanges)
	}
	return changed, nil
}

func untrackedGoFiles(repoRoot string, runner gitRunner) ([]string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	args := []string{
		"ls-files",
		"--others",
		"--exclude-standard",
		"--",
		":(glob)**/*.go",
	}
	if err := runner.Run(repoRoot, args, &stdout, &stderr); err != nil {
		if details := strings.TrimSpace(stderr.String()); details != "" {
			return nil, fmt.Errorf("list untracked Go files: %w: %s", err, details)
		}
		return nil, fmt.Errorf("list untracked Go files: %w", err)
	}

	files := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(stdout.String()))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		fileName, err := parseGitListedPath(scanner.Text())
		if err != nil {
			return nil, fmt.Errorf("parse untracked Go file path: %w", err)
		}
		if strings.HasSuffix(fileName, ".go") {
			files[fileName] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read untracked Go file list: %w", err)
	}

	fileNames := make([]string, 0, len(files))
	for fileName := range files {
		fileNames = append(fileNames, fileName)
	}
	sort.Strings(fileNames)
	return fileNames, nil
}

func parseChangedGoLines(diff string) (changedGoLines, error) {
	changed := changedGoLines{
		files: make(map[string]struct{}),
		lines: make(map[string][]lineRange),
	}
	currentFile := ""
	scanner := bufio.NewScanner(strings.NewReader(diff))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		switch {
		case strings.HasPrefix(line, "+++ "):
			fileName, err := parseGitDiffPath(strings.TrimPrefix(line, "+++ "))
			if err != nil {
				return changedGoLines{}, err
			}
			currentFile = fileName
			if currentFile != "" && strings.HasSuffix(currentFile, ".go") {
				changed.files[currentFile] = struct{}{}
			}
		case strings.HasPrefix(line, "rename to "):
			fileName, err := parseGitDiffPath(strings.TrimPrefix(line, "rename to "))
			if err != nil {
				return changedGoLines{}, err
			}
			if fileName != "" && strings.HasSuffix(fileName, ".go") {
				changed.files[fileName] = struct{}{}
			}
		case strings.HasPrefix(line, "@@ "):
			if currentFile == "" || !strings.HasSuffix(currentFile, ".go") {
				continue
			}
			lineRange, err := parseChangedLineRange(line)
			if err != nil {
				return changedGoLines{}, err
			}
			if lineRange.end >= lineRange.start {
				changed.lines[currentFile] = append(changed.lines[currentFile], lineRange)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return changedGoLines{}, fmt.Errorf("read git diff: %w", err)
	}
	for fileName, lineRanges := range changed.lines {
		changed.lines[fileName] = mergeLineRanges(lineRanges)
	}
	return changed, nil
}

func parseGitDiffPath(value string) (string, error) {
	value, err := parseGitPathValue(value)
	if err != nil {
		return "", err
	}
	value = strings.TrimPrefix(strings.TrimPrefix(value, "a/"), "b/")
	return cleanGitRepoPath(value)
}

func parseGitListedPath(value string) (string, error) {
	value, err := parseGitPathValue(value)
	if err != nil {
		return "", err
	}
	return cleanGitRepoPath(value)
}

func parseGitPathValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "/dev/null" {
		return "", nil
	}
	if strings.HasPrefix(value, "\"") {
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("parse quoted git path %q: %w", value, err)
		}
		value = unquoted
	} else if tab := strings.IndexByte(value, '\t'); tab >= 0 {
		value = value[:tab]
	}
	return value, nil
}

func cleanGitRepoPath(value string) (string, error) {
	value = path.Clean(strings.TrimPrefix(value, "./"))
	if value == "." || value == "" || value == ".." || strings.HasPrefix(value, "../") {
		return "", errors.New("git diff contains an invalid path")
	}
	return value, nil
}

func parseChangedLineRange(hunk string) (lineRange, error) {
	matches := changedLinesHunkPattern.FindStringSubmatch(hunk)
	if matches == nil {
		return lineRange{}, fmt.Errorf("parse git diff hunk %q", hunk)
	}
	start, err := strconv.Atoi(matches[1])
	if err != nil {
		return lineRange{}, fmt.Errorf("parse changed hunk start: %w", err)
	}
	count := 1
	if matches[2] != "" {
		count, err = strconv.Atoi(matches[2])
		if err != nil {
			return lineRange{}, fmt.Errorf("parse changed hunk count: %w", err)
		}
	}
	if start < 0 || count < 0 {
		return lineRange{}, errors.New("git diff hunk has a negative line range")
	}
	if count == 0 {
		return lineRange{start: start, end: start - 1}, nil
	}
	if start == 0 {
		return lineRange{}, errors.New("git diff hunk has a zero changed line")
	}
	return lineRange{start: start, end: start + count - 1}, nil
}

func mergeLineRanges(lineRanges []lineRange) []lineRange {
	if len(lineRanges) < 2 {
		return lineRanges
	}
	sort.Slice(lineRanges, func(left, right int) bool {
		if lineRanges[left].start == lineRanges[right].start {
			return lineRanges[left].end < lineRanges[right].end
		}
		return lineRanges[left].start < lineRanges[right].start
	})
	merged := make([]lineRange, 0, len(lineRanges))
	for _, current := range lineRanges {
		if len(merged) == 0 || current.start > merged[len(merged)-1].end+1 {
			merged = append(merged, current)
			continue
		}
		if current.end > merged[len(merged)-1].end {
			merged[len(merged)-1].end = current.end
		}
	}
	return merged
}

func blockOverlapsChangedLines(block cover.ProfileBlock, lineRanges []lineRange) bool {
	for _, lineRange := range lineRanges {
		if lineRange.end < block.StartLine {
			continue
		}
		if lineRange.start > block.EndLine {
			return false
		}
		return true
	}
	return false
}

// changedExecutableFilesWithoutProfile makes an absent profile visible only
// when the changed lines touch a concrete function declaration or its body.
// This keeps comment, import, and type-only edits out of the executable-code
// gate while preventing a newly introduced behavior file from being reported
// as a misleading 0/0 n/a result.
func changedExecutableFilesWithoutProfile(
	repoRoot string,
	changed changedGoLines,
	profileFiles map[string]struct{},
) ([]string, error) {
	fileNames := make([]string, 0, len(changed.files))
	for fileName := range changed.files {
		fileNames = append(fileNames, fileName)
	}
	sort.Strings(fileNames)

	missing := make([]string, 0)
	for _, fileName := range fileNames {
		if strings.HasSuffix(fileName, "_test.go") || len(changed.lines[fileName]) == 0 {
			continue
		}
		if _, excluded := exclusionIndex(fileName); excluded {
			continue
		}
		if _, found := profileFiles[fileName]; found {
			continue
		}
		executable, err := changedLinesTouchFunction(repoRoot, fileName, changed.lines[fileName])
		if err != nil {
			return nil, err
		}
		if executable {
			missing = append(missing, fileName)
		}
	}
	return missing, nil
}

func changedLinesTouchFunction(repoRoot, fileName string, changedLines []lineRange) (bool, error) {
	cleanFileName := path.Clean(fileName)
	if cleanFileName == "." || cleanFileName == ".." || strings.HasPrefix(cleanFileName, "../") {
		return false, fmt.Errorf("read changed Go source %s: invalid repository-relative path", fileName)
	}
	sourcePath := filepath.Join(repoRoot, filepath.FromSlash(cleanFileName))
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return false, fmt.Errorf("read changed Go source %s: %w", fileName, err)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, sourcePath, source, 0)
	if err != nil {
		return false, fmt.Errorf("parse changed Go source %s: %w", fileName, err)
	}
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		startLine := fileSet.PositionFor(function.Pos(), false).Line
		endLine := fileSet.PositionFor(function.End(), false).Line
		if changedLineRangesOverlap(startLine, endLine, changedLines) {
			return true, nil
		}
	}
	return false, nil
}

func changedLineRangesOverlap(startLine, endLine int, changedLines []lineRange) bool {
	for _, changed := range changedLines {
		if changed.end < startLine {
			continue
		}
		if changed.start > endLine {
			return false
		}
		return true
	}
	return false
}

func repoRelativeProfilePath(fileName string) string {
	fileName = normalizeProfilePath(fileName)
	for _, root := range []string{"cmd", "internal", "pkg"} {
		marker := "/" + root + "/"
		if strings.HasPrefix(fileName, root+"/") {
			return fileName
		}
		if index := strings.Index(fileName, marker); index >= 0 {
			return fileName[index+1:]
		}
	}
	return fileName
}

func evaluateDiffCoverage(analysis diffCoverageAnalysis, cfg config) []string {
	var violations []string
	for _, fileName := range analysis.missingProfileFiles {
		violations = append(violations, fmt.Sprintf(
			"changed Go source %s contains executable behavior but has no coverage profile data",
			fileName,
		))
	}
	if analysis.ordinary.total > 0 && analysis.ordinary.percentage() < cfg.diffThreshold {
		violations = append(violations, fmt.Sprintf(
			"ordinary Go diff coverage against %s is %.2f%%, below %.2f%%",
			analysis.base, analysis.ordinary.percentage(), cfg.diffThreshold,
		))
	}
	for _, scope := range analysis.critical {
		if scope.total > 0 && scope.percentage() < cfg.criticalDiffThreshold {
			violations = append(violations, fmt.Sprintf(
				"critical Go diff coverage for %s against %s is %.2f%%, below %.2f%%",
				scope.scope, analysis.base, scope.percentage(), cfg.criticalDiffThreshold,
			))
		}
	}
	return violations
}

func printDiffCoverageReport(writer io.Writer, analysis diffCoverageAnalysis, cfg config) error {
	if analysis.ordinary.total == 0 && len(analysis.critical) == 0 {
		if _, err := fmt.Fprintf(writer,
			"Go diff coverage: ref=%s n/a (0/0 changed executable statements across %d changed Go files)\n",
			analysis.base,
			analysis.changedFiles,
		); err != nil {
			return fmt.Errorf("write diff coverage summary: %w", err)
		}
		return printMissingDiffProfileFiles(writer, analysis.missingProfileFiles)
	}
	if _, err := fmt.Fprintf(writer,
		"Go diff coverage: ref=%s ordinary=%s\n",
		analysis.base,
		formatDiffCoverageStats(analysis.ordinary, cfg.diffThreshold),
	); err != nil {
		return fmt.Errorf("write diff coverage summary: %w", err)
	}
	if len(analysis.critical) == 0 {
		if _, err := fmt.Fprintf(writer,
			"Critical Go diff coverage: n/a (0/0 changed executable statements; threshold=%.2f%%)\n",
			cfg.criticalDiffThreshold,
		); err != nil {
			return fmt.Errorf("write critical diff coverage summary: %w", err)
		}
		return printMissingDiffProfileFiles(writer, analysis.missingProfileFiles)
	}
	for _, scope := range analysis.critical {
		if _, err := fmt.Fprintf(writer,
			"Critical Go diff coverage: %-42s %s\n",
			criticalScopeLabel(scope),
			formatDiffCoverageStats(scope.coverageStats, cfg.criticalDiffThreshold),
		); err != nil {
			return fmt.Errorf("write critical diff coverage summary: %w", err)
		}
	}
	return printMissingDiffProfileFiles(writer, analysis.missingProfileFiles)
}

func printMissingDiffProfileFiles(writer io.Writer, fileNames []string) error {
	for _, fileName := range fileNames {
		if _, err := fmt.Fprintf(writer,
			"Go diff coverage: missing coverage profile for changed executable source %s\n",
			fileName,
		); err != nil {
			return fmt.Errorf("write missing diff coverage profile: %w", err)
		}
	}
	return nil
}

func formatDiffCoverageStats(stats coverageStats, threshold float64) string {
	if stats.total == 0 {
		return fmt.Sprintf("n/a (0/0 changed executable statements; threshold=%.2f%%)", threshold)
	}
	return fmt.Sprintf("%.2f%% (%d/%d changed executable statements; threshold=%.2f%%)",
		stats.percentage(), stats.covered, stats.total, threshold,
	)
}
