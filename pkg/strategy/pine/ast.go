package pine

import (
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

type NodeKind string

const (
	NodeKindVersion         NodeKind = "version"
	NodeKindStrategy        NodeKind = "strategy"
	NodeKindAssignment      NodeKind = "assignment"
	NodeKindTupleAssignment NodeKind = "tuple_assignment"
	NodeKindIf              NodeKind = "if"
	NodeKindElse            NodeKind = "else"
	NodeKindFor             NodeKind = "for"
	NodeKindWhile           NodeKind = "while"
	NodeKindLoopControl     NodeKind = "loop_control"
	NodeKindCall            NodeKind = "call"
	NodeKindVisual          NodeKind = "visual"
	NodeKindCollection      NodeKind = "collection"
	NodeKindDeclaration     NodeKind = "declaration"
	NodeKindUnsupported     NodeKind = "unsupported"
)

type AST struct {
	SourceFormat string    `json:"sourceFormat"`
	Lines        []ASTLine `json:"lines"`
	Nodes        []ASTNode `json:"nodes,omitempty"`
}

type ASTNode struct {
	Line     ASTLine   `json:"line"`
	Children []ASTNode `json:"children,omitempty"`
}

type ASTLine struct {
	Kind       NodeKind                  `json:"kind"`
	Line       int                       `json:"line"`
	Column     int                       `json:"column"`
	EndLine    int                       `json:"endLine"`
	EndColumn  int                       `json:"endColumn"`
	Indent     int                       `json:"indent"`
	Text       string                    `json:"text"`
	Name       string                    `json:"name,omitempty"`
	Type       string                    `json:"type,omitempty"`
	Expression string                    `json:"expression,omitempty"`
	Mode       strategyir.AssignmentMode `json:"mode,omitempty"`
}

func parseAST(lines []parsedLine) (*AST, []Diagnostic) {
	ast := &AST{SourceFormat: SourceFormatPineV6, Lines: make([]ASTLine, 0, len(lines))}
	diagnostics := make([]Diagnostic, 0)
	for _, line := range lines {
		astLine := ASTLine{
			Kind:      classifyASTLine(line.trimmed),
			Line:      line.number,
			Column:    line.indent + 1,
			EndLine:   line.number,
			EndColumn: line.indent + 1 + len(line.trimmed),
			Indent:    line.indent,
			Text:      line.trimmed,
		}
		if astLine.EndColumn <= astLine.Column {
			astLine.EndColumn = astLine.Column + 1
		}
		if match := typedAssignmentPattern.FindStringSubmatch(line.trimmed); match != nil {
			astLine.Kind = NodeKindCollection
			astLine.Type = normalizeTypeAnnotation(match[2])
			astLine.Name = strings.TrimSpace(match[3])
			astLine.Expression = strings.TrimSpace(match[5])
			astLine.Mode = assignmentModeFromMatch(strings.TrimSpace(match[1]), strings.TrimSpace(match[4]))
		} else if match := assignmentPattern.FindStringSubmatch(line.trimmed); match != nil {
			astLine.Kind = NodeKindAssignment
			astLine.Name = strings.TrimSpace(match[2])
			astLine.Expression = strings.TrimSpace(match[4])
			if containsCollectionNamespace(astLine.Expression) {
				astLine.Kind = NodeKindCollection
			}
			astLine.Mode = assignmentModeFromMatch(strings.TrimSpace(match[1]), strings.TrimSpace(match[3]))
		} else if match := generalTuplePattern.FindStringSubmatch(line.trimmed); match != nil {
			astLine.Kind = NodeKindTupleAssignment
			names := splitArguments(match[1])
			if len(names) > 0 {
				astLine.Name = strings.TrimSpace(names[0])
			}
			astLine.Expression = strings.TrimSpace(match[3])
			if strings.TrimSpace(match[2]) == ":=" {
				astLine.Mode = strategyir.AssignmentModeReassign
			} else {
				astLine.Mode = strategyir.AssignmentModeLet
			}
		}
		if diagnostic, ok := unsupportedSyntaxDiagnostic(line); ok {
			if !preserveUnsupportedDiagnosticKind(astLine.Kind) {
				astLine.Kind = NodeKindUnsupported
			}
			diagnostics = append(diagnostics, diagnostic)
		}
		ast.Lines = append(ast.Lines, astLine)
	}
	ast.Nodes = buildStructuredASTNodes(ast.Lines)
	return ast, diagnostics
}

func buildStructuredASTNodes(lines []ASTLine) []ASTNode {
	roots := make([]ASTNode, 0)
	type nodePath struct {
		indent int
		path   []int
	}
	stack := make([]nodePath, 0)
	for _, line := range lines {
		for len(stack) > 0 && line.Indent <= stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, ASTNode{Line: line})
			stack = append(stack, nodePath{indent: line.Indent, path: []int{len(roots) - 1}})
			continue
		}
		parentPath := stack[len(stack)-1].path
		parent := astNodeAtPath(roots, parentPath)
		parent.Children = append(parent.Children, ASTNode{Line: line})
		childPath := append(append([]int(nil), parentPath...), len(parent.Children)-1)
		stack = append(stack, nodePath{indent: line.Indent, path: childPath})
	}
	return roots
}

func astNodeAtPath(nodes []ASTNode, path []int) *ASTNode {
	current := &nodes[path[0]]
	for _, index := range path[1:] {
		current = &current.Children[index]
	}
	return current
}

func assignmentModeFromMatch(prefix string, operator string) strategyir.AssignmentMode {
	switch {
	case prefix != "":
		return strategyir.AssignmentModeVar
	case operator == ":=":
		return strategyir.AssignmentModeReassign
	default:
		return strategyir.AssignmentModeLet
	}
}

func normalizeTypeAnnotation(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func classifyASTLine(trimmed string) NodeKind {
	lower := strings.ToLower(strings.TrimSpace(trimmed))
	switch {
	case strings.HasPrefix(lower, "//@version"):
		return NodeKindVersion
	case strings.HasPrefix(lower, "strategy("):
		return NodeKindStrategy
	case isVisualOnlyCall(lower):
		return NodeKindVisual
	case strings.HasPrefix(lower, "type "), strings.HasPrefix(lower, "method "), strings.HasPrefix(lower, "import "), strings.HasPrefix(lower, "export "), strings.HasPrefix(lower, "library("):
		return NodeKindDeclaration
	case strings.Contains(lower, "array."), strings.Contains(lower, "matrix."), strings.Contains(lower, "map."):
		return NodeKindCollection
	case strings.HasPrefix(lower, "if "):
		return NodeKindIf
	case strings.HasPrefix(lower, "for "):
		return NodeKindFor
	case strings.HasPrefix(lower, "while "):
		return NodeKindWhile
	case lower == "break" || lower == "continue":
		return NodeKindLoopControl
	case lower == "else" || lower == "else:":
		return NodeKindElse
	case strings.Contains(lower, "("):
		return NodeKindCall
	default:
		return NodeKindUnsupported
	}
}

func containsCollectionNamespace(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "array.") || strings.Contains(lower, "matrix.") || strings.Contains(lower, "map.")
}

func preserveUnsupportedDiagnosticKind(kind NodeKind) bool {
	return kind == NodeKindCollection || kind == NodeKindDeclaration
}
