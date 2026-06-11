package expression

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
