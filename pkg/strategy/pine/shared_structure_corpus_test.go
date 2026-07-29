package pine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

type sharedStructureCorpusCase struct {
	ID                      string         `json:"id"`
	Dimensions              []string       `json:"dimensions"`
	Source                  string         `json:"source"`
	ExpectedStatementKinds  map[string]int `json:"expectedStatementKinds"`
	ExpectedVisualKinds     map[string]int `json:"expectedVisualBlockKinds"`
	ExpectedVisualSemantics []string       `json:"expectedVisualSemantics"`
	ExpectedBranches        map[string]int `json:"expectedBranches"`
	ExpectedBranchTargets   map[string]int `json:"expectedBranchTargets"`
	ExpectedTradeSemantics  []string       `json:"expectedTradeSemantics"`
	ExpectedMaxIfDepth      int            `json:"expectedMaxIfDepth"`
}

var requiredSharedStatementKinds = []string{
	"let", "if", "log", "notify", "order", "exit", "cancel",
}

func TestSharedPineStructureCorpusMatchesBackendIR(t *testing.T) {
	t.Parallel()

	corpus := loadSharedStructureCorpus(t)
	assertSharedCorpusCoverage(t, corpus)
	for _, corpusCase := range corpus {
		corpusCase := corpusCase
		t.Run(corpusCase.ID, func(t *testing.T) {
			t.Parallel()

			analysis := AnalyzeScript(corpusCase.Source, AnalysisOptions{})
			if !analysis.OK || analysis.Program == nil {
				t.Fatalf("AnalyzeScript() failed: diagnostics = %#v", analysis.Diagnostics)
			}
			actual := countProgramStatementKinds(analysis.Program)
			if !reflect.DeepEqual(actual, corpusCase.ExpectedStatementKinds) {
				t.Fatalf("statement kinds = %#v, want %#v", actual, corpusCase.ExpectedStatementKinds)
			}
			branches, branchTargets, maxIfDepth := summarizeBranches(analysis.Program)
			if !reflect.DeepEqual(branches, corpusCase.ExpectedBranches) {
				t.Fatalf("branches = %#v, want %#v", branches, corpusCase.ExpectedBranches)
			}
			if !reflect.DeepEqual(branchTargets, corpusCase.ExpectedBranchTargets) {
				t.Fatalf("branch targets = %#v, want %#v", branchTargets, corpusCase.ExpectedBranchTargets)
			}
			if maxIfDepth != corpusCase.ExpectedMaxIfDepth {
				t.Fatalf("max if depth = %d, want %d", maxIfDepth, corpusCase.ExpectedMaxIfDepth)
			}
			tradeSemantics := summarizeTradeSemantics(analysis.Program, corpusCase.Source)
			if !reflect.DeepEqual(tradeSemantics, corpusCase.ExpectedTradeSemantics) {
				t.Fatalf("trade semantics = %#v, want %#v", tradeSemantics, corpusCase.ExpectedTradeSemantics)
			}
		})
	}
}

func assertSharedCorpusCoverage(t *testing.T, corpus []sharedStructureCorpusCase) {
	t.Helper()

	coveredKinds := make(map[string]bool)
	coveredDimensions := make(map[string]bool)
	seenIDs := make(map[string]bool)
	for _, corpusCase := range corpus {
		if corpusCase.ID == "" || seenIDs[corpusCase.ID] {
			t.Fatalf("corpus case id %q must be non-empty and unique", corpusCase.ID)
		}
		seenIDs[corpusCase.ID] = true
		if len(corpusCase.Dimensions) == 0 || len(corpusCase.ExpectedVisualKinds) == 0 || len(corpusCase.ExpectedVisualSemantics) == 0 || corpusCase.ExpectedBranchTargets == nil || corpusCase.ExpectedTradeSemantics == nil {
			t.Fatalf("corpus case %q must declare business dimensions and visual block kinds", corpusCase.ID)
		}
		for kind := range corpusCase.ExpectedStatementKinds {
			coveredKinds[kind] = true
		}
		for _, dimension := range corpusCase.Dimensions {
			coveredDimensions[dimension] = true
		}
	}
	for _, kind := range requiredSharedStatementKinds {
		if !coveredKinds[kind] {
			t.Errorf("shared Pine corpus does not cover stable statement kind %q", kind)
		}
	}
	for _, dimension := range []string{"input", "state", "mtf", "nested-if", "else", "notification", "risk-metadata"} {
		if !coveredDimensions[dimension] {
			t.Errorf("shared Pine corpus does not cover business dimension %q", dimension)
		}
	}
}

func loadSharedStructureCorpus(t *testing.T) []sharedStructureCorpusCase {
	t.Helper()

	path := filepath.Join("..", "..", "..", "tests", "fixtures", "pine-structure-corpus.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shared Pine structure corpus: %v", err)
	}
	var corpus []sharedStructureCorpusCase
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatalf("decode shared Pine structure corpus: %v", err)
	}
	if len(corpus) == 0 {
		t.Fatal("shared Pine structure corpus is empty")
	}
	return corpus
}

func summarizeBranches(program *strategyir.Program) (map[string]int, map[string]int, int) {
	branches := map[string]int{"true": 0, "false": 0}
	branchTargets := make(map[string]int)
	maxDepth := 0
	for _, hook := range program.Hooks {
		summarizeStatementBranches(hook.Statements, 0, branches, branchTargets, &maxDepth)
	}
	return branches, branchTargets, maxDepth
}

func summarizeStatementBranches(
	statements []strategyir.Statement,
	depth int,
	branches map[string]int,
	branchTargets map[string]int,
	maxDepth *int,
) {
	for _, statement := range statements {
		switch typed := statement.(type) {
		case *strategyir.IfStmt:
			ifDepth := depth + 1
			*maxDepth = max(*maxDepth, ifDepth)
			if len(typed.Then) > 0 {
				branches["true"]++
				branchTargets["true:"+string(typed.Then[0].Kind())]++
			}
			if len(typed.Else) > 0 {
				branches["false"]++
				branchTargets["false:"+string(typed.Else[0].Kind())]++
			}
			summarizeStatementBranches(typed.Then, ifDepth, branches, branchTargets, maxDepth)
			summarizeStatementBranches(typed.Else, ifDepth, branches, branchTargets, maxDepth)
		case *strategyir.LoopStmt:
			summarizeStatementBranches(typed.Body, depth, branches, branchTargets, maxDepth)
		}
	}
}

func summarizeTradeSemantics(program *strategyir.Program, source string) []string {
	semantics := make([]string, 0)
	for _, hook := range program.Hooks {
		collectTradeSemantics(hook.Statements, &semantics)
	}
	metadata := program.Metadata
	if strings.Contains(source, "strategy.risk.allow_entry_in") {
		semantics = append(semantics, "risk:allowEntryIn:"+metadata.AllowedEntryDirection)
	}
	if metadata.MaxDrawdownValue > 0 {
		semantics = append(semantics, fmt.Sprintf(
			"risk:maxDrawdown:%s:%s:%s",
			formatTradeNumber(metadata.MaxDrawdownValue),
			metadata.MaxDrawdownType,
			metadata.MaxDrawdownAlert,
		))
	}
	sort.Strings(semantics)
	return semantics
}

func collectTradeSemantics(statements []strategyir.Statement, semantics *[]string) {
	for _, statement := range statements {
		switch typed := statement.(type) {
		case *strategyir.IfStmt:
			collectTradeSemantics(typed.Then, semantics)
			collectTradeSemantics(typed.Else, semantics)
		case *strategyir.LoopStmt:
			collectTradeSemantics(typed.Body, semantics)
		case *strategyir.OrderStmt:
			*semantics = append(*semantics, fmt.Sprintf(
				"order:%s:%s:%s:%s:%s:limit=%t:stop=%t:immediate=%t",
				tradeField(string(typed.Intent)), tradeField(typed.ID), tradeField(string(typed.Action)),
				tradeField(typed.QuantityMode), tradeField(typed.QuantityExpression),
				typed.LimitExpression != "", typed.StopExpression != "", typed.Immediate,
			))
		case *strategyir.ExitStmt:
			*semantics = append(*semantics, fmt.Sprintf(
				"exit:%s:%s:%s:%s:stop=%t:limit=%t:profit=%t:loss=%t:trailPoints=%t:trailPrice=%t:trailOffset=%t",
				tradeField(typed.FromEntry), tradeField(typed.Direction), tradeField(typed.QuantityMode),
				tradeField(typed.QuantityExpression), typed.StopExpression != "", typed.LimitExpression != "",
				typed.ProfitExpression != "", typed.LossExpression != "", typed.TrailPoints != "",
				typed.TrailPrice != "", typed.TrailOffset != "",
			))
		case *strategyir.CancelStmt:
			id := typed.ID
			if typed.All {
				id = "*"
			}
			*semantics = append(*semantics, "cancel:"+tradeField(id))
		}
	}
}

func tradeField(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return strings.TrimSpace(value)
}

func formatTradeNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func countProgramStatementKinds(program *strategyir.Program) map[string]int {
	counts := make(map[string]int)
	for _, hook := range program.Hooks {
		countStatementKinds(hook.Statements, counts)
	}
	return counts
}

func countStatementKinds(statements []strategyir.Statement, counts map[string]int) {
	for _, statement := range statements {
		counts[string(statement.Kind())]++
		switch typed := statement.(type) {
		case *strategyir.IfStmt:
			countStatementKinds(typed.Then, counts)
			countStatementKinds(typed.Else, counts)
		case *strategyir.LoopStmt:
			countStatementKinds(typed.Body, counts)
		}
	}
}
