package pine

import (
	"fmt"
	"regexp"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

const SourceFormatPineV6 = "pine-v6"
const maxHistoryLookback = 500
const maxUDFExpansionDepth = 8
const maxStaticForIterations = 100
const maxStaticForDepth = 2
const maxRuntimeLoopDepth = 4
const maxRuntimeLoopIterations = 1000

type Compilation struct {
	Program      *strategyir.Program
	Requirements strategyir.Requirements
	Warnings     []string
}

type parsedLine struct {
	number  int
	raw     string
	trimmed string
	indent  int
}

type parseState struct {
	lines                []parsedLine
	warnings             []string
	longEntryIDs         map[string]bool
	shortEntryIDs        map[string]bool
	udfs                 map[string]pineUDF
	expressionAliases    map[string]string
	sourceAliases        map[string]string
	valueAliases         map[string]string
	collectionNamespaces map[string]string
	udtTypes             map[string]strategyir.TypeDefinition
	udtMethods           map[string][]strategyir.MethodDefinition
	objectTypes          map[string]string
	objectPersistent     map[string]bool
	typeDefinitions      []strategyir.TypeDefinition
	methodDefinitions    []strategyir.MethodDefinition
	loopVariables        map[string]bool
	forDepth             int
	runtimeLoopDepth     int
	normalizationErr     error
	entryPolicyCache     map[int]string
	strategyMetadata     strategyir.StrategyMetadata
	regexpCache          map[string]*regexp.Regexp
}

type pineUDF struct {
	Name string
	Args []string
	Body string
	Line int
}

func (s *parseState) cachedRegexp(key string, pattern string) *regexp.Regexp {
	if s == nil {
		return regexp.MustCompile(pattern)
	}
	if compiled, ok := s.regexpCache[key]; ok {
		return compiled
	}
	if s.regexpCache == nil {
		s.regexpCache = map[string]*regexp.Regexp{}
	}
	compiled := regexp.MustCompile(pattern)
	s.regexpCache[key] = compiled
	return compiled
}

func Compile(script string) (Compilation, error) {
	analysis := AnalyzeScript(script, AnalysisOptions{})
	if !analysis.OK {
		if err := diagnosticError(analysis.Diagnostics); err != nil {
			return Compilation{}, err
		}
		return Compilation{}, fmt.Errorf("pine script is not valid")
	}
	return Compilation{
		Program:      analysis.Program,
		Requirements: analysis.Requirements,
		Warnings:     analysis.Warnings,
	}, nil
}

func compileLoweredAST(script string, lines []parsedLine, ast *AST) (Compilation, error) {
	state := newParseState(script, lines, ast)
	if len(state.lines) == 0 {
		return Compilation{}, fmt.Errorf("pine script is required")
	}
	program := &strategyir.Program{SourceFormat: SourceFormatPineV6}
	headerState, err := state.scanCompilationHeaders(program)
	if err != nil {
		return Compilation{}, err
	}
	if err := validateCompilationHeaders(headerState); err != nil {
		return Compilation{}, err
	}
	statements, _, err := state.parseBlock(headerState.executableStart, -1)
	if err != nil {
		return Compilation{}, err
	}
	return buildCompilationResult(program, state, statements), nil
}

type compilationHeaderState struct {
	versionSeen     bool
	strategySeen    bool
	executableStart int
}

func newParseState(script string, lines []parsedLine, ast *AST) *parseState {
	state := &parseState{
		lines:                nil,
		longEntryIDs:         map[string]bool{},
		shortEntryIDs:        map[string]bool{},
		udfs:                 map[string]pineUDF{},
		expressionAliases:    map[string]string{},
		sourceAliases:        map[string]string{},
		valueAliases:         map[string]string{},
		collectionNamespaces: map[string]string{},
		udtTypes:             map[string]strategyir.TypeDefinition{},
		udtMethods:           map[string][]strategyir.MethodDefinition{},
		objectTypes:          map[string]string{},
		objectPersistent:     map[string]bool{},
		loopVariables:        map[string]bool{},
		entryPolicyCache:     buildEntryPolicyCache(script),
		regexpCache:          map[string]*regexp.Regexp{},
	}
	state.lines = parsedLinesFromStructuredAST(ast, lines)
	return state
}

func (s *parseState) scanCompilationHeaders(program *strategyir.Program) (compilationHeaderState, error) {
	state := compilationHeaderState{}
	for index, line := range s.lines {
		if line.indent > 0 {
			state.executableStart = index
			return state, nil
		}
		handled, err := s.applyCompilationHeaderLine(program, line, index, &state)
		if err != nil {
			return state, err
		}
		if handled {
			continue
		}
		state.executableStart = index
		return state, nil
	}
	state.executableStart = len(s.lines)
	return state, nil
}

func (s *parseState) applyCompilationHeaderLine(program *strategyir.Program, line parsedLine, index int, state *compilationHeaderState) (bool, error) {
	lower := strings.ToLower(line.trimmed)
	switch {
	case strings.HasPrefix(lower, "//@version"):
		if !strings.Contains(strings.ReplaceAll(lower, " ", ""), "//@version=6") {
			return false, fmt.Errorf("pine line %d: JFTrade requires //@version=6", line.number)
		}
		state.versionSeen = true
		state.executableStart = index + 1
		return true, nil
	case strings.HasPrefix(lower, "strategy("):
		metadata, warnings := parseStrategyDeclaration(line.trimmed)
		s.warnings = append(s.warnings, warnings...)
		program.Metadata = metadata
		s.strategyMetadata = metadata
		state.strategySeen = true
		state.executableStart = index + 1
		return true, nil
	case strings.HasPrefix(lower, "indicator("), strings.HasPrefix(lower, "study("), strings.HasPrefix(lower, "library("):
		return false, fmt.Errorf("pine line %d: JFTrade can execute strategy(...) scripts only", line.number)
	case isVisualOnlyCall(lower):
		s.warnings = append(s.warnings, fmt.Sprintf("pine line %d: visual-only call %q is ignored by JFTrade", line.number, callName(line.trimmed)))
		state.executableStart = index + 1
		return true, nil
	default:
		return false, nil
	}
}

func validateCompilationHeaders(state compilationHeaderState) error {
	if !state.versionSeen {
		return fmt.Errorf("pine script requires //@version=6")
	}
	if !state.strategySeen {
		return fmt.Errorf("pine script requires strategy(...) declaration")
	}
	return nil
}

func buildCompilationResult(program *strategyir.Program, state *parseState, statements []strategyir.Statement) Compilation {
	if len(statements) == 0 {
		statements = []strategyir.Statement{&strategyir.LogStmt{
			Range:   strategyir.SourceRange{StartLine: 1, EndLine: 1},
			Message: "pine strategy has no executable statements",
		}}
	}
	program.Metadata = state.strategyMetadata
	program.Types = append([]strategyir.TypeDefinition(nil), state.typeDefinitions...)
	program.Methods = append([]strategyir.MethodDefinition(nil), state.methodDefinitions...)
	program.Hooks = []strategyir.HookBlock{{
		Kind:       strategyir.HookKLineClose,
		Range:      strategyir.SourceRange{StartLine: statements[0].SourceRange().StartLine, EndLine: statements[len(statements)-1].SourceRange().EndLine},
		Statements: statements,
	}}
	return Compilation{Program: program, Warnings: state.warnings}
}

func parsedLinesFromStructuredAST(ast *AST, fallback []parsedLine) []parsedLine {
	if ast == nil || len(ast.Nodes) == 0 {
		return fallback
	}
	byLine := make(map[int]parsedLine, len(fallback))
	for _, line := range fallback {
		byLine[line.number] = line
	}
	result := make([]parsedLine, 0, len(fallback))
	var appendNodes func([]ASTNode)
	appendNodes = func(nodes []ASTNode) {
		for _, node := range nodes {
			if line, ok := byLine[node.Line.Line]; ok {
				result = append(result, line)
			}
			appendNodes(node.Children)
		}
	}
	appendNodes(ast.Nodes)
	if len(result) != len(fallback) {
		return fallback
	}
	return result
}

func ParseScript(script string) (*strategyir.Program, error) {
	compilation, err := Compile(script)
	if err != nil {
		return nil, err
	}
	return compilation.Program, nil
}

func ValidateScript(script string) error {
	_, err := Compile(script)
	return err
}
