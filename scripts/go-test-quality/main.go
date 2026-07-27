package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const defaultExemptionsPath = "scripts/go-test-quality-exemptions.json"

var failureMethods = map[string]struct{}{
	"Error": {}, "Errorf": {}, "Fail": {}, "FailNow": {}, "Fatal": {}, "Fatalf": {},
}

type options struct {
	repoRoot      string
	base          string
	exemptions    string
	exemptionsSet bool
}

type sourceFile struct {
	path     string
	contents []byte
}

type candidate struct {
	Path string
	Name string
	Line int
}

type exemption struct {
	Path   string `json:"path"`
	Test   string `json:"test"`
	Reason string `json:"reason"`
}

type exemptionDocument struct {
	Exemptions []exemption `json:"exemptions"`
}

type parsedFunction struct {
	decl             *ast.FuncDecl
	file             *ast.File
	path             string
	line             int
	assertionAliases map[string]struct{}
	testingAliases   map[string]struct{}
	testingNames     map[string]struct{}
}

type packageAnalysis struct {
	functions map[string]*parsedFunction
	tests     []*parsedFunction
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(opts.repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	base, err := resolveBase(root, opts.base)
	if err != nil {
		return err
	}
	mergeBase, err := gitOutput(root, "merge-base", base, "HEAD")
	if err != nil {
		return fmt.Errorf("resolve merge base for %s: %w", base, err)
	}
	mergeBase = strings.TrimSpace(mergeBase)
	if mergeBase == "" {
		return fmt.Errorf("resolve merge base for %s: Git returned an empty object ID", base)
	}

	currentSources, err := currentTestSources(root)
	if err != nil {
		return err
	}
	currentCandidates, err := analyzeSources(currentSources)
	if err != nil {
		return fmt.Errorf("analyze current Go tests: %w", err)
	}

	baseSources, err := treeTestSources(root, mergeBase)
	if err != nil {
		return err
	}
	baseCandidates, err := analyzeSources(baseSources)
	if err != nil {
		return fmt.Errorf("analyze Go tests at %s: %w", mergeBase, err)
	}
	baseCandidateIDs := candidateIDs(baseCandidates)

	renames, err := renamedTestPaths(root, mergeBase)
	if err != nil {
		return err
	}
	exemptions, err := readExemptions(root, opts.exemptions, opts.exemptionsSet)
	if err != nil {
		return err
	}
	currentCandidateIDs := candidateIDs(currentCandidates)
	if err := validateExemptions(exemptions, currentCandidateIDs); err != nil {
		return err
	}

	newCandidates, err := reportCandidates(
		stdout,
		currentCandidates,
		baseCandidateIDs,
		renames,
		exemptions,
	)
	if err != nil {
		return err
	}
	if len(newCandidates) == 0 {
		return writef(stdout, "Go test assertion policy passed: no new unexempted assertion gaps.\n")
	}
	if err := writef(stderr, "New Go tests must assert a business result or declare a reasoned exemption:\n"); err != nil {
		return err
	}
	for _, item := range newCandidates {
		if err := writef(stderr, "- %s:%d %s\n", item.Path, item.Line, item.Name); err != nil {
			return err
		}
	}
	return errors.New("go test assertion policy failed")
}

func reportCandidates(
	output io.Writer,
	candidates []candidate,
	baseline map[string]struct{},
	renames map[string]string,
	exemptions map[string]exemption,
) ([]candidate, error) {
	var newCandidates []candidate
	if err := writef(output, "Go test assertion report: %d test(s) have no recognized assertion.\n", len(candidates)); err != nil {
		return nil, err
	}
	for _, item := range candidates {
		id := candidateID(item.Path, item.Name)
		if entry, ok := exemptions[id]; ok {
			if err := writef(output, "- %s:%d %s [exempt: %s]\n", item.Path, item.Line, item.Name, entry.Reason); err != nil {
				return nil, err
			}
			continue
		}
		if isBaselineCandidate(item, baseline, renames) {
			if err := writef(output, "- %s:%d %s [legacy]\n", item.Path, item.Line, item.Name); err != nil {
				return nil, err
			}
			continue
		}
		newCandidates = append(newCandidates, item)
		if err := writef(output, "- %s:%d %s [new]\n", item.Path, item.Line, item.Name); err != nil {
			return nil, err
		}
	}
	return newCandidates, nil
}

func writef(output io.Writer, format string, arguments ...any) error {
	if _, err := fmt.Fprintf(output, format, arguments...); err != nil {
		return fmt.Errorf("write test assertion report: %w", err)
	}
	return nil
}

func parseOptions(args []string) (options, error) {
	opts := options{repoRoot: ".", exemptions: defaultExemptionsPath}
	flags := flag.NewFlagSet("go-test-quality", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.repoRoot, "repo-root", opts.repoRoot, "repository root")
	flags.StringVar(&opts.base, "base", "", "Git base ref")
	flags.Func("exemptions", "reasoned exemption file", func(value string) error {
		opts.exemptions = value
		opts.exemptionsSet = true
		return nil
	})
	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf(
			"usage: go-test-quality [--repo-root path] [--base git-ref] [--exemptions path]: %w",
			err,
		)
	}
	if flags.NArg() != 0 {
		return options{}, errors.New("usage: go-test-quality [--repo-root path] [--base git-ref] [--exemptions path]")
	}
	return opts, nil
}

func resolveBase(repoRoot, configured string) (string, error) {
	base := strings.TrimSpace(configured)
	if base == "" {
		base = strings.TrimSpace(os.Getenv("JFTRADE_DIFF_BASE"))
	}
	if base != "" {
		if allZeroes(base) {
			return "", errors.New("unable to determine a diff base; pass --base or set JFTRADE_DIFF_BASE")
		}
		return base, nil
	}
	for _, candidate := range []string{"origin/main", "HEAD^"} {
		if _, err := gitOutput(repoRoot, "rev-parse", "--verify", candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("unable to determine a diff base; pass --base or set JFTRADE_DIFF_BASE")
}

func allZeroes(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char != '0' {
			return false
		}
	}
	return true
}

func currentTestSources(repoRoot string) ([]sourceFile, error) {
	output, err := gitBytes(repoRoot, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("list current repository files: %w", err)
	}
	seen := make(map[string]struct{})
	var sources []sourceFile
	for _, rawPath := range bytes.Split(output, []byte{0}) {
		filePath := string(rawPath)
		if !strings.HasSuffix(filePath, "_test.go") {
			continue
		}
		if _, ok := seen[filePath]; ok {
			continue
		}
		seen[filePath] = struct{}{}
		contents, readErr := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(filePath)))
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", filePath, readErr)
		}
		sources = append(sources, sourceFile{path: filePath, contents: contents})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].path < sources[j].path })
	return sources, nil
}

func treeTestSources(repoRoot, revision string) ([]sourceFile, error) {
	output, err := gitBytes(repoRoot, "ls-tree", "-r", "--name-only", "-z", revision)
	if err != nil {
		return nil, fmt.Errorf("list Go tests at %s: %w", revision, err)
	}
	var paths []string
	for _, rawPath := range bytes.Split(output, []byte{0}) {
		filePath := string(rawPath)
		if strings.HasSuffix(filePath, "_test.go") {
			paths = append(paths, filePath)
		}
	}
	contents, err := readGitBlobs(repoRoot, revision, paths)
	if err != nil {
		return nil, fmt.Errorf("read Go tests at %s: %w", revision, err)
	}
	sources := make([]sourceFile, 0, len(paths))
	for _, filePath := range paths {
		sources = append(sources, sourceFile{path: filePath, contents: contents[filePath]})
	}
	return sources, nil
}

func readGitBlobs(repoRoot, revision string, paths []string) (map[string][]byte, error) {
	result := make(map[string][]byte, len(paths))
	if len(paths) == 0 {
		return result, nil
	}
	command := exec.Command("git", "cat-file", "--batch")
	command.Dir = repoRoot
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(stdout)
	for _, filePath := range paths {
		if _, err := fmt.Fprintf(stdin, "%s:%s\n", revision, filePath); err != nil {
			return nil, finishBatch(command, stdin, err)
		}
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, finishBatch(command, stdin, err)
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[1] != "blob" {
			return nil, finishBatch(command, stdin, fmt.Errorf("unexpected cat-file response for %s: %s", filePath, strings.TrimSpace(header)))
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 {
			return nil, finishBatch(command, stdin, fmt.Errorf("invalid blob size for %s", filePath))
		}
		contents := make([]byte, size)
		if _, err := io.ReadFull(reader, contents); err != nil {
			return nil, finishBatch(command, stdin, err)
		}
		if separator, err := reader.ReadByte(); err != nil || separator != '\n' {
			if err == nil {
				err = errors.New("missing blob separator")
			}
			return nil, finishBatch(command, stdin, err)
		}
		result[filePath] = contents
	}
	if err := stdin.Close(); err != nil {
		return nil, err
	}
	if err := command.Wait(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return result, nil
}

func finishBatch(command *exec.Cmd, stdin io.WriteCloser, cause error) error {
	_ = stdin.Close()
	_ = command.Process.Kill()
	_ = command.Wait()
	return cause
}

func analyzeSources(sources []sourceFile) ([]candidate, error) {
	fileSet := token.NewFileSet()
	packages := make(map[string]*packageAnalysis)
	for _, source := range sources {
		file, err := parser.ParseFile(fileSet, source.path, source.contents, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", source.path, err)
		}
		packageKey := path.Dir(source.path) + "\x00" + file.Name.Name
		analysis := packages[packageKey]
		if analysis == nil {
			analysis = &packageAnalysis{functions: make(map[string]*parsedFunction)}
			packages[packageKey] = analysis
		}
		assertionAliases, testingAliases := importAliases(file)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			parsed := &parsedFunction{
				decl:             function,
				file:             file,
				path:             source.path,
				line:             fileSet.Position(function.Pos()).Line,
				assertionAliases: assertionAliases,
				testingAliases:   testingAliases,
				testingNames:     testingVariableNames(function, testingAliases),
			}
			if function.Recv == nil {
				analysis.functions[function.Name.Name] = parsed
			}
			if isTestFunction(function, testingAliases) {
				analysis.tests = append(analysis.tests, parsed)
			}
		}
	}

	var candidates []candidate
	for _, analysis := range packages {
		asserting := assertionFunctions(analysis)
		for _, testFunction := range analysis.tests {
			if !asserting[testFunction.decl.Name.Name] {
				candidates = append(candidates, candidate{
					Path: testFunction.path,
					Name: testFunction.decl.Name.Name,
					Line: testFunction.line,
				})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Path != candidates[j].Path {
			return candidates[i].Path < candidates[j].Path
		}
		if candidates[i].Line != candidates[j].Line {
			return candidates[i].Line < candidates[j].Line
		}
		return candidates[i].Name < candidates[j].Name
	})
	return candidates, nil
}

func importAliases(file *ast.File) (map[string]struct{}, map[string]struct{}) {
	assertions := make(map[string]struct{})
	testing := make(map[string]struct{})
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if importPath == "testing" {
			testing[name] = struct{}{}
		}
		if strings.HasSuffix(importPath, "/testify/assert") ||
			strings.HasSuffix(importPath, "/testify/require") ||
			strings.HasSuffix(importPath, "/gotest.tools/v3/assert") {
			assertions[name] = struct{}{}
		}
	}
	return assertions, testing
}

func isTestFunction(function *ast.FuncDecl, testingAliases map[string]struct{}) bool {
	if function.Recv != nil || !validTestName(function.Name.Name) || function.Type.Params == nil {
		return false
	}
	fields := function.Type.Params.List
	if len(fields) != 1 || len(fields[0].Names) != 1 {
		return false
	}
	return isTestingType(fields[0].Type, testingAliases, "T", false)
}

func validTestName(name string) bool {
	if !strings.HasPrefix(name, "Test") || len(name) == len("Test") {
		return false
	}
	next, _ := utf8.DecodeRuneInString(name[len("Test"):])
	return !unicode.IsLower(next)
}

func testingVariableNames(function *ast.FuncDecl, aliases map[string]struct{}) map[string]struct{} {
	names := make(map[string]struct{})
	addTestingFieldNames(names, function.Type.Params, aliases)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.FuncLit)
		if ok {
			addTestingFieldNames(names, literal.Type.Params, aliases)
		}
		return true
	})
	return names
}

func addTestingFieldNames(names map[string]struct{}, fields *ast.FieldList, aliases map[string]struct{}) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		if !isTestingType(field.Type, aliases, "T", true) {
			continue
		}
		for _, name := range field.Names {
			names[name.Name] = struct{}{}
		}
	}
}

func isTestingType(expression ast.Expr, aliases map[string]struct{}, expected string, allowTB bool) bool {
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	if _, ok := aliases[packageName.Name]; !ok {
		return false
	}
	return selector.Sel.Name == expected || (allowTB && selector.Sel.Name == "TB")
}

func assertionFunctions(analysis *packageAnalysis) map[string]bool {
	asserting := make(map[string]bool)
	for name, function := range analysis.functions {
		if hasDirectAssertion(function) {
			asserting[name] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for name, function := range analysis.functions {
			if asserting[name] || !callsAssertionHelper(function, asserting) {
				continue
			}
			asserting[name] = true
			changed = true
		}
	}
	return asserting
}

func hasDirectAssertion(function *parsedFunction) bool {
	found := false
	ast.Inspect(function.decl.Body, func(node ast.Node) bool {
		if found {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if receiver, ok := selector.X.(*ast.Ident); ok {
			if _, isTesting := function.testingNames[receiver.Name]; isTesting {
				_, found = failureMethods[selector.Sel.Name]
				return !found
			}
		}
		if root := rootIdentifier(selector.X); root != "" {
			_, found = function.assertionAliases[root]
		}
		return !found
	})
	return found
}

func callsAssertionHelper(function *parsedFunction, asserting map[string]bool) bool {
	found := false
	ast.Inspect(function.decl.Body, func(node ast.Node) bool {
		if found {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := calledFunctionName(call.Fun)
		found = name != "" && asserting[name]
		return !found
	})
	return found
}

func calledFunctionName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.IndexExpr:
		return calledFunctionName(typed.X)
	case *ast.IndexListExpr:
		return calledFunctionName(typed.X)
	default:
		return ""
	}
}

func rootIdentifier(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return rootIdentifier(typed.X)
	case *ast.CallExpr:
		return rootIdentifier(typed.Fun)
	case *ast.IndexExpr:
		return rootIdentifier(typed.X)
	case *ast.IndexListExpr:
		return rootIdentifier(typed.X)
	default:
		return ""
	}
}

func candidateIDs(candidates []candidate) map[string]struct{} {
	ids := make(map[string]struct{}, len(candidates))
	for _, item := range candidates {
		ids[candidateID(item.Path, item.Name)] = struct{}{}
	}
	return ids
}

func candidateID(filePath, testName string) string {
	return filePath + "\x00" + testName
}

func isBaselineCandidate(item candidate, baseline map[string]struct{}, renames map[string]string) bool {
	if _, ok := baseline[candidateID(item.Path, item.Name)]; ok {
		return true
	}
	oldPath, renamed := renames[item.Path]
	if !renamed {
		return false
	}
	_, ok := baseline[candidateID(oldPath, item.Name)]
	return ok
}

func renamedTestPaths(repoRoot, mergeBase string) (map[string]string, error) {
	output, err := gitBytes(repoRoot, "diff", "--name-status", "-z", "--find-renames", mergeBase, "--")
	if err != nil {
		return nil, fmt.Errorf("inspect test renames since %s: %w", mergeBase, err)
	}
	fields := bytes.Split(output, []byte{0})
	renames := make(map[string]string)
	for index := 0; index < len(fields); {
		status := string(fields[index])
		index++
		if status == "" {
			continue
		}
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if index+1 >= len(fields) {
				return nil, errors.New("parse Git rename output: truncated record")
			}
			oldPath := string(fields[index])
			newPath := string(fields[index+1])
			index += 2
			if strings.HasSuffix(newPath, "_test.go") {
				renames[newPath] = oldPath
			}
			continue
		}
		if index >= len(fields) {
			return nil, errors.New("parse Git diff output: truncated record")
		}
		index++
	}
	return renames, nil
}

func readExemptions(repoRoot, configuredPath string, explicitlySet bool) (map[string]exemption, error) {
	exemptionPath := configuredPath
	if !filepath.IsAbs(exemptionPath) {
		exemptionPath = filepath.Join(repoRoot, filepath.FromSlash(exemptionPath))
	}
	contents, err := os.ReadFile(exemptionPath)
	if errors.Is(err, os.ErrNotExist) && !explicitlySet {
		return map[string]exemption{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Go test assertion exemptions: %w", err)
	}
	var document exemptionDocument
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("parse Go test assertion exemptions: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("parse Go test assertion exemptions: trailing JSON content")
	}
	result := make(map[string]exemption, len(document.Exemptions))
	for _, entry := range document.Exemptions {
		entry.Path = filepath.ToSlash(strings.TrimSpace(entry.Path))
		entry.Test = strings.TrimSpace(entry.Test)
		entry.Reason = strings.TrimSpace(entry.Reason)
		if entry.Path == "" || path.IsAbs(entry.Path) || path.Clean(entry.Path) != entry.Path ||
			!strings.HasSuffix(entry.Path, "_test.go") {
			return nil, fmt.Errorf("invalid exemption path %q", entry.Path)
		}
		if !validTestName(entry.Test) {
			return nil, fmt.Errorf("invalid exempted test name %q", entry.Test)
		}
		if len(entry.Reason) < 12 {
			return nil, fmt.Errorf("exemption %s:%s requires a specific reason of at least 12 characters", entry.Path, entry.Test)
		}
		id := candidateID(entry.Path, entry.Test)
		if _, duplicate := result[id]; duplicate {
			return nil, fmt.Errorf("duplicate Go test assertion exemption: %s:%s", entry.Path, entry.Test)
		}
		result[id] = entry
	}
	return result, nil
}

func validateExemptions(exemptions map[string]exemption, candidates map[string]struct{}) error {
	var stale []string
	for id, entry := range exemptions {
		if _, ok := candidates[id]; !ok {
			stale = append(stale, entry.Path+":"+entry.Test)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	sort.Strings(stale)
	return fmt.Errorf("remove stale Go test assertion exemptions:\n- %s", strings.Join(stale, "\n- "))
}

func gitOutput(repoRoot string, args ...string) (string, error) {
	output, err := gitBytes(repoRoot, args...)
	return string(output), err
}

func gitBytes(repoRoot string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	command.Dir = repoRoot
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, errors.New(message)
	}
	return output, nil
}
