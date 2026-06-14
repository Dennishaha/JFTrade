package pineruntime

import (
	"fmt"
	"strings"

	exprast "github.com/expr-lang/expr/ast"
)

func evaluateBuiltinExpression(expression *exprast.BuiltinNode, scope *evaluationScope) (any, error) {
	switch strings.ToLower(strings.TrimSpace(expression.Name)) {
	case "abs":
		return evaluateMathExpression("abs", expression.Arguments, scope)
	case "min", "max", "avg", "round", "round_to_mintick", "floor", "ceil", "sqrt", "pow", "log", "sign":
		return evaluateMathExpression(strings.ToLower(strings.TrimSpace(expression.Name)), expression.Arguments, scope)
	case "sum":
		return evaluateWindowNumericExpression("sum", expression.Arguments, scope)
	default:
		return nil, fmt.Errorf("unsupported builtin function %q", expression.Name)
	}
}

func evaluateCallExpression(expression *exprast.CallNode, scope *evaluationScope) (any, error) {
	name, ok := expression.Callee.(*exprast.IdentifierNode)
	if !ok {
		return nil, fmt.Errorf("unsupported call target %T", expression.Callee)
	}
	functionName := strings.ToLower(strings.TrimSpace(name.Value))
	switch functionName {
	case "cross_over":
		return evaluateCrossExpression(expression.Arguments, scope, true)
	case "cross_under":
		return evaluateCrossExpression(expression.Arguments, scope, false)
	case "divergence_top":
		return evaluateDivergenceExpression(expression.Arguments, scope, "top")
	case "divergence_bottom":
		return evaluateDivergenceExpression(expression.Arguments, scope, "bottom")
	case "previous":
		return evaluatePreviousExpression(expression.Arguments, scope)
	case "history":
		return evaluateHistoryExpression(expression, scope)
	case "ifelse":
		return evaluateIfElseExpression(expression.Arguments, scope)
	case "nz":
		return evaluateNZExpression(expression.Arguments, scope)
	case "abs":
		return evaluateMathExpression(functionName, expression.Arguments, scope)
	case "ma":
		return evaluateMovingAverageExpression(expression.Arguments, scope)
	case "security_source":
		return evaluateSecuritySourceExpression(expression.Arguments, scope)
	case "min", "max", "avg", "round", "round_to_mintick", "floor", "ceil", "sqrt", "pow", "log", "sign":
		return evaluateMathExpression(functionName, expression.Arguments, scope)
	case "stdev":
		return evaluateSourcePeriodIndicatorExpression(functionName, expression.Arguments, scope, "close", "20")
	case "rsi":
		return evaluateSourcePeriodIndicatorExpression(functionName, expression.Arguments, scope, "close", "14")
	case "macd":
		return evaluateMACDExpression(expression.Arguments, scope)
	case "atr":
		return evaluateATRExpression(expression.Arguments, scope)
	case "bollinger":
		return evaluateBollingerExpression(expression.Arguments, scope)
	case "variance":
		return evaluateSourcePeriodIndicatorExpression(functionName, expression.Arguments, scope, "close", "20")
	case "cci":
		return evaluateSourcePeriodIndicatorExpression(functionName, expression.Arguments, scope, "hlc3", "20")
	case "vwap":
		return evaluateSourceIndicatorExpression(functionName, expression.Arguments, scope, "hlc3")
	case "cum":
		return evaluateRequiredSourceIndicatorExpression(functionName, expression.Arguments, scope)
	case "mfi":
		return evaluateSourcePeriodIndicatorExpression(functionName, expression.Arguments, scope, "hlc3", "14")
	case "stoch":
		return evaluateStochExpression(expression.Arguments, scope)
	case "dmi":
		return evaluateDMIExpression(expression.Arguments, scope)
	case "supertrend":
		return evaluateSupertrendExpression(expression.Arguments, scope)
	case "sar":
		return evaluateSARExpression(expression.Arguments, scope)
	case "tr":
		return evaluateTrueRangeExpression(expression.Arguments, scope)
	case "timestamp":
		return evaluateTimestampExpression(expression.Arguments, scope)
	case "barssince":
		return evaluateBarsSinceExpression(expression, scope)
	case "valuewhen":
		return evaluateValueWhenExpression(expression, scope)
	case "highest", "lowest", "highestbars", "lowestbars", "change", "mom", "roc", "range", "mode", "sum":
		return evaluateWindowNumericExpression(functionName, expression.Arguments, scope)
	case "rising", "falling":
		return evaluateWindowBoolExpression(functionName, expression.Arguments, scope)
	case "tostring":
		return evaluateToStringExpression(expression.Arguments, scope)
	default:
		return nil, fmt.Errorf("unsupported function %q", name.Value)
	}
}
