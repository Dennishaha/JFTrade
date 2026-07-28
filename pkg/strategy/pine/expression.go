package pine

import (
	"fmt"
	"strings"

	exprast "github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

func parseExpression(expression string) (exprast.Node, error) {
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
