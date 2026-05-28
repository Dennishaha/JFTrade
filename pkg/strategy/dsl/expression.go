package dsl

import (
	"fmt"
	"strings"

	exprast "github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

func ParseExpression(expression string) (exprast.Node, error) {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return nil, fmt.Errorf("expression is required")
	}
	tree, err := parser.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	return tree.Node, nil
}

func validateExpression(lineNumber int, label string, expression string) error {
	if _, err := ParseExpression(expression); err != nil {
		return fmt.Errorf("dsl line %d: invalid %s %q: %w", lineNumber, label, strings.TrimSpace(expression), err)
	}
	return nil
}
