package pineruntime

import (
	exprast "github.com/expr-lang/expr/ast"
)

type seriesNumber struct {
	Current     float64
	Previous    float64
	HasCurrent  bool
	HasPrevious bool
}

type objectFieldReader interface {
	FieldValue(name string) (any, bool)
}

type objectSeriesReader interface {
	SeriesField(name string) (current float64, previous float64, hasCurrent bool, hasPrevious bool, ok bool)
}

type preferredScalarReader interface {
	PreferredScalarValue() (float64, bool)
}

type scalarValueReader interface {
	ScalarValue() (float64, bool)
}

func readObjectField(base any, property string) (any, bool) {
	switch values := base.(type) {
	case map[string]any:
		value, ok := values[property]
		if !ok {
			return missingObjectField, true
		}
		return value, true
	case objectFieldReader:
		value, ok := values.FieldValue(property)
		if !ok {
			return missingObjectField, true
		}
		return value, true
	default:
		return nil, false
	}
}

func memberPropertyName(node exprast.Node) (string, bool) {
	switch typed := node.(type) {
	case *exprast.StringNode:
		return typed.Value, true
	case *exprast.IdentifierNode:
		return typed.Value, true
	default:
		return "", false
	}
}
