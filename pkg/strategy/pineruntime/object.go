package pineruntime

import (
	"fmt"
	"strings"

	exprast "github.com/expr-lang/expr/ast"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (r *strategyRuntime) executeObjectStatement(statement *strategyir.ObjectStmt, scope *evaluationScope) error {
	if statement == nil {
		return nil
	}
	arguments := make([]any, len(statement.Arguments))
	for index, expression := range statement.Arguments {
		value, err := evaluateExpression(expression, scope)
		if err != nil {
			return fmt.Errorf("pine line %d: object argument %d: %w", statement.Range.StartLine, index+1, err)
		}
		arguments[index] = value
	}
	var value any
	var err error
	switch statement.Operation {
	case "constructor":
		value, err = r.constructObject(statement.TypeName, arguments, scope)
	case "method":
		value, err = r.callObjectMethod(statement.TypeName, statement.Method, statement.Target, arguments, scope)
	case "field_set":
		value, err = r.setObjectField(statement.TypeName, statement.Method, statement.Target, arguments, scope)
	default:
		err = fmt.Errorf("unsupported object operation %q", statement.Operation)
	}
	if err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if strings.TrimSpace(statement.ResultName) == "" {
		return nil
	}
	switch statement.Mode {
	case strategyir.AssignmentModeVar:
		if r != nil && r.persistentValues != nil {
			if previous, ok := r.persistentValues[statement.ResultName]; ok {
				value = previous
			} else {
				r.persistentValues[statement.ResultName] = value
			}
		}
		scope.setVariable(statement.ResultName, value)
	case strategyir.AssignmentModeReassign:
		scope.assignVariable(statement.ResultName, value)
	default:
		scope.setVariable(statement.ResultName, value)
	}
	return nil
}

func (r *strategyRuntime) constructObject(typeName string, arguments []any, scope *evaluationScope) (map[string]any, error) {
	definition, ok := r.objectTypeDefinition(typeName)
	if !ok {
		return nil, fmt.Errorf("unknown type %q", typeName)
	}
	if len(arguments) > len(definition.Fields) {
		return nil, fmt.Errorf("%s.new received too many arguments", definition.Name)
	}
	object := make(map[string]any, len(definition.Fields)+1)
	object["__type"] = definition.Name
	for index, field := range definition.Fields {
		if index < len(arguments) {
			if err := validateCollectionValue(field.Type, arguments[index]); err != nil {
				return nil, fmt.Errorf("%s.%s: %w", definition.Name, field.Name, err)
			}
			object[field.Name] = arguments[index]
			continue
		}
		value, err := evaluateExpression(field.Default, scope)
		if err != nil {
			return nil, fmt.Errorf("%s.%s default: %w", definition.Name, field.Name, err)
		}
		if err := validateCollectionValue(field.Type, value); err != nil {
			return nil, fmt.Errorf("%s.%s default: %w", definition.Name, field.Name, err)
		}
		object[field.Name] = value
	}
	return object, nil
}

func (r *strategyRuntime) callObjectMethod(typeName, methodName, target string, arguments []any, scope *evaluationScope) (any, error) {
	method, ok := r.objectMethodDefinition(typeName, methodName, len(arguments))
	if !ok {
		return nil, fmt.Errorf("method %s.%s has no matching overload", typeName, methodName)
	}
	receiver, ok := scope.variable(target)
	if !ok {
		return nil, fmt.Errorf("unknown object %q", target)
	}
	return r.callObjectMethodValue(method, typeName, methodName, receiver, arguments, scope)
}

func (r *strategyRuntime) callObjectMethodValue(method strategyir.MethodDefinition, typeName, methodName string, receiver any, arguments []any, scope *evaluationScope) (any, error) {
	methodScope := scope.clone()
	methodScope.setVariable(method.ReceiverName, receiver)
	for index, parameter := range method.Parameters {
		if index < len(arguments) {
			methodScope.setVariable(parameter.Name, arguments[index])
			continue
		}
		if strings.TrimSpace(parameter.Default) == "" {
			return nil, fmt.Errorf("method %s.%s missing argument %s", typeName, methodName, parameter.Name)
		}
		value, err := evaluateExpression(parameter.Default, methodScope)
		if err != nil {
			return nil, fmt.Errorf("method default %s: %w", parameter.Name, err)
		}
		methodScope.setVariable(parameter.Name, value)
	}
	return evaluateExpression(method.Body, methodScope)
}

func evaluateObjectMethodExpression(arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if len(arguments) < 3 {
		return nil, fmt.Errorf("object_method requires type, method and receiver")
	}
	typeRaw, err := evaluateAST(arguments[0], scope)
	if err != nil {
		return nil, err
	}
	methodRaw, err := evaluateAST(arguments[1], scope)
	if err != nil {
		return nil, err
	}
	typeName, ok := typeRaw.(string)
	if !ok || strings.TrimSpace(typeName) == "" {
		return nil, fmt.Errorf("object_method type must be a string")
	}
	methodName, ok := methodRaw.(string)
	if !ok || strings.TrimSpace(methodName) == "" {
		return nil, fmt.Errorf("object_method method must be a string")
	}
	receiver, err := evaluateAST(arguments[2], scope)
	if err != nil {
		return nil, err
	}
	values := make([]any, 0, len(arguments)-3)
	for _, argument := range arguments[3:] {
		value, err := evaluateAST(argument, scope)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if scope == nil || scope.runtime == nil {
		return nil, fmt.Errorf("object_method requires a runtime")
	}
	method, ok := scope.runtime.objectMethodDefinition(typeName, methodName, len(values))
	if !ok {
		return nil, fmt.Errorf("method %s.%s has no matching overload", typeName, methodName)
	}
	return scope.runtime.callObjectMethodValue(method, typeName, methodName, receiver, values, scope)
}

func (r *strategyRuntime) setObjectField(typeName, fieldName, target string, arguments []any, scope *evaluationScope) (any, error) {
	if len(arguments) != 1 {
		return nil, fmt.Errorf("field assignment requires one value")
	}
	definition, ok := r.objectTypeDefinition(typeName)
	if !ok {
		return nil, fmt.Errorf("unknown type %q", typeName)
	}
	field, ok := runtimeObjectFieldDefinition(definition, fieldName)
	if !ok {
		return nil, fmt.Errorf("type %s has no field %s", typeName, fieldName)
	}
	if err := validateCollectionValue(field.Type, arguments[0]); err != nil {
		return nil, fmt.Errorf("%s.%s: %w", typeName, fieldName, err)
	}
	receiver, ok := scope.variable(target)
	if !ok {
		return nil, fmt.Errorf("unknown object %q", target)
	}
	object, ok := receiver.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", target)
	}
	object[field.Name] = arguments[0]
	return nil, nil
}

func runtimeObjectFieldDefinition(definition strategyir.TypeDefinition, name string) (strategyir.ObjectField, bool) {
	for _, field := range definition.Fields {
		if strings.EqualFold(field.Name, name) {
			return field, true
		}
	}
	return strategyir.ObjectField{}, false
}

func (r *strategyRuntime) objectTypeDefinition(name string) (strategyir.TypeDefinition, bool) {
	if r == nil || r.program == nil {
		return strategyir.TypeDefinition{}, false
	}
	for _, definition := range r.program.Types {
		if strings.EqualFold(definition.Name, name) {
			return definition, true
		}
	}
	return strategyir.TypeDefinition{}, false
}

func (r *strategyRuntime) objectMethodDefinition(typeName, methodName string, argCount int) (strategyir.MethodDefinition, bool) {
	if r == nil || r.program == nil {
		return strategyir.MethodDefinition{}, false
	}
	for _, method := range r.program.Methods {
		if !strings.EqualFold(method.ReceiverType, typeName) || !strings.EqualFold(method.Name, methodName) {
			continue
		}
		required := 0
		for _, parameter := range method.Parameters {
			if strings.TrimSpace(parameter.Default) == "" {
				required++
			}
		}
		if argCount >= required && argCount <= len(method.Parameters) {
			return method, true
		}
	}
	return strategyir.MethodDefinition{}, false
}
