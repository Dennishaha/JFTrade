package pine

import (
	"fmt"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

var executableCollectionOperations = map[string]map[string]bool{
	"array": {
		"new": true, "new_float": true, "new_int": true, "new_bool": true, "new_string": true,
		"get": true, "set": true, "push": true, "pop": true, "shift": true, "unshift": true,
		"insert": true, "remove": true, "first": true, "last": true, "size": true, "clear": true,
		"copy": true, "slice": true, "reverse": true, "fill": true, "includes": true,
		"indexof": true, "lastindexof": true, "min": true, "max": true, "avg": true, "sum": true,
		"from": true, "concat": true, "join": true, "sort": true, "sort_indices": true,
		"binary_search": true, "median": true, "mode": true, "range": true,
		"abs": true, "binary_search_leftmost": true, "binary_search_rightmost": true,
		"percentrank": true, "percentile_nearest_rank": true, "percentile_linear_interpolation": true,
		"stdev": true, "variance": true, "covariance": true,
	},
	"map": {
		"new": true, "get": true, "put": true, "remove": true, "contains": true, "size": true, "clear": true,
		"copy": true, "keys": true, "values": true,
	},
	"matrix": {
		"new": true, "get": true, "set": true, "rows": true, "columns": true,
		"fill": true, "copy": true, "reshape": true, "add_row": true, "add_col": true,
		"remove_row": true, "remove_col": true,
	},
}

var mutatingCollectionOperations = map[string]bool{
	"new": true, "new_float": true, "new_int": true, "new_bool": true, "new_string": true,
	"set": true, "push": true, "pop": true, "shift": true, "unshift": true, "insert": true,
	"remove": true, "clear": true, "put": true, "reverse": true, "fill": true,
	"sort": true, "concat": true, "reshape": true, "add_row": true, "add_col": true,
	"remove_row": true, "remove_col": true,
}

var readableCollectionOperations = map[string]bool{
	"new": true, "new_float": true, "new_int": true, "new_bool": true, "new_string": true,
	"get": true, "first": true, "last": true, "size": true, "contains": true, "rows": true, "columns": true,
	"copy": true, "slice": true, "includes": true, "indexof": true, "lastindexof": true,
	"min": true, "max": true, "avg": true, "sum": true, "from": true, "join": true,
	"sort_indices": true, "binary_search": true, "median": true, "mode": true, "range": true,
	"abs": true, "binary_search_leftmost": true, "binary_search_rightmost": true,
	"percentrank": true, "percentile_nearest_rank": true, "percentile_linear_interpolation": true,
	"stdev": true, "variance": true, "covariance": true,
	"keys": true, "values": true,
}

func (s *parseState) parseCollectionStatement(line parsedLine) (strategyir.Statement, bool, error) {
	if statement, handled, err := s.parseTypedCollectionStatement(line); handled || err != nil {
		return statement, handled, err
	}
	if statement, handled, err := s.parseAssignedCollectionStatement(line); handled || err != nil {
		return statement, handled, err
	}
	return s.parseStandaloneCollectionStatement(line)
}

func (s *parseState) normalizeCollectionArguments(lineNumber int, arguments []string) ([]string, error) {
	normalized := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		expression := s.normalizeExpression(argument)
		if err := s.takeNormalizationErr(lineNumber); err != nil {
			return nil, err
		}
		if err := validateExpression(lineNumber, "collection argument", expression); err != nil {
			return nil, err
		}
		normalized = append(normalized, expression)
	}
	return normalized, nil
}

func (s *parseState) parseTypedCollectionStatement(line parsedLine) (strategyir.Statement, bool, error) {
	match := typedAssignmentPattern.FindStringSubmatch(line.trimmed)
	if match == nil {
		return nil, false, nil
	}
	namespace, annotationTypeArgs := collectionTypeAnnotationInfo(match[2])
	name := strings.TrimSpace(match[3])
	operator := strings.TrimSpace(match[4])
	expression := strings.TrimSpace(match[5])
	call, args, ok := parseFunctionCallText(expression)
	if !ok {
		return nil, true, fmt.Errorf("pine line %d: typed collection declarations require an executable collection constructor", line.number)
	}
	callNamespace, operation, callTypeArgs, ok := parseExecutableCollectionCall(call)
	if !ok || !strings.HasPrefix(operation, "new") {
		return nil, true, fmt.Errorf("pine line %d: typed collection declarations require an executable collection constructor", line.number)
	}
	if namespace != callNamespace {
		return nil, true, fmt.Errorf("pine line %d: %s declaration cannot be initialized with %s", line.number, namespace, call)
	}
	typeArgs := annotationTypeArgs
	if typeArgs == "" {
		typeArgs = callTypeArgs
	}
	normalizedArgs, err := s.normalizeCollectionArguments(line.number, args)
	if err != nil {
		return nil, true, err
	}
	s.collectionNamespaces[strings.ToLower(name)] = namespace
	mode := assignmentMode(strings.TrimSpace(match[1]), operator)
	return collectionStatement(line, namespace, operation, "", name, typeArgs, normalizedArgs, mode), true, nil
}

func (s *parseState) parseAssignedCollectionStatement(line parsedLine) (strategyir.Statement, bool, error) {
	match := assignmentPattern.FindStringSubmatch(line.trimmed)
	if match == nil {
		return nil, false, nil
	}
	name := strings.TrimSpace(match[2])
	expression := strings.TrimSpace(match[4])
	call, args, ok := parseFunctionCallText(expression)
	if !ok {
		return nil, false, nil
	}
	namespace, operation, typeArgs, ok := s.resolveExecutableCollectionCall(call)
	if !ok {
		return nil, false, nil
	}
	mode := assignmentMode(strings.TrimSpace(match[1]), strings.TrimSpace(match[3]))
	if collectionConstructorOperation(namespace, operation) {
		return s.parseCollectionConstructorAssignment(line, name, namespace, operation, typeArgs, args, mode)
	}
	return s.parseCollectionOperationAssignment(line, name, expression, namespace, operation, typeArgs, args, mode)
}

func (s *parseState) parseCollectionConstructorAssignment(line parsedLine, name string, namespace string, operation string, typeArgs string, args []string, mode strategyir.AssignmentMode) (strategyir.Statement, bool, error) {
	normalizedArgs, err := s.normalizeCollectionArguments(line.number, args)
	if err != nil {
		return nil, true, err
	}
	s.collectionNamespaces[strings.ToLower(name)] = namespace
	return collectionStatement(line, namespace, operation, "", name, typeArgs, normalizedArgs, mode), true, nil
}

func (s *parseState) parseCollectionOperationAssignment(line parsedLine, name string, expression string, namespace string, operation string, typeArgs string, args []string, mode strategyir.AssignmentMode) (strategyir.Statement, bool, error) {
	target, normalizedArgs, err := s.collectionTargetAndArguments(namespace, functionCallNameText(expression), args)
	if err != nil {
		return nil, true, fmt.Errorf("pine line %d: %w", line.number, err)
	}
	normalizedArgs, err = s.normalizeCollectionArguments(line.number, normalizedArgs)
	if err != nil {
		return nil, true, err
	}
	s.updateCollectionResultNamespace(name, namespace, operation)
	return collectionStatement(line, namespace, operation, target, name, typeArgs, normalizedArgs, mode), true, nil
}

func (s *parseState) parseStandaloneCollectionStatement(line parsedLine) (strategyir.Statement, bool, error) {
	call, args, ok := parseFunctionCallText(line.trimmed)
	if !ok {
		return nil, false, nil
	}
	namespace, operation, typeArgs, ok := s.resolveExecutableCollectionCall(call)
	if !ok {
		return nil, false, nil
	}
	if !mutatingCollectionOperations[operation] || collectionConstructorOperation(namespace, operation) {
		return nil, true, fmt.Errorf("pine line %d: collection call %s must be assigned or used in an expression", line.number, call)
	}
	target, normalizedArgs, err := s.collectionTargetAndArguments(namespace, functionCallNameText(line.trimmed), args)
	if err != nil {
		return nil, true, fmt.Errorf("pine line %d: %w", line.number, err)
	}
	normalizedArgs, err = s.normalizeCollectionArguments(line.number, normalizedArgs)
	if err != nil {
		return nil, true, err
	}
	return collectionStatement(line, namespace, operation, target, "", typeArgs, normalizedArgs, strategyir.AssignmentModeLet), true, nil
}

func (s *parseState) updateCollectionResultNamespace(name string, namespace string, operation string) {
	key := strings.ToLower(name)
	if resultNamespace := collectionResultNamespace(namespace, operation); resultNamespace != "" {
		s.collectionNamespaces[key] = resultNamespace
		return
	}
	delete(s.collectionNamespaces, key)
}

type collectionHistoryCall struct {
	start     int
	end       int
	name      string
	lookback  string
	operation string
	args      string
}

func (s *parseState) lowerCollectionHistoryReadCalls(expression string) (string, error) {
	result := expression
	for {
		call, ok := findCollectionHistoryCall(result)
		if !ok {
			return result, nil
		}
		namespace := s.collectionNamespaces[strings.ToLower(call.name)]
		if namespace == "" {
			return result, fmt.Errorf("collection history reference %s[%s].%s requires a known collection variable", call.name, call.lookback, call.operation)
		}
		if namespace != "array" {
			return result, fmt.Errorf("collection history is supported only for arrays")
		}
		if !collectionHistoryReadOperation(call.operation) {
			return result, fmt.Errorf("collection history supports only read operations get/size/first/last")
		}
		result = result[:call.start] + collectionHistoryReplacement(call) + result[call.end:]
	}
}

func findCollectionHistoryCall(expression string) (collectionHistoryCall, bool) {
	match := collectionHistoryCall{start: -1}
	rewriteOutsideStringLiterals(expression, func(segment string) string {
		if match.start >= 0 {
			return segment
		}
		offset := strings.Index(expression, segment)
		match = findCollectionHistoryCallInSegment(segment, offset)
		return segment
	})
	return match, match.start >= 0
}

func findCollectionHistoryCallInSegment(segment string, offset int) collectionHistoryCall {
	for search := 0; search < len(segment); search++ {
		match, nextSearch, ok := parseCollectionHistoryCallAt(segment, offset, search)
		if ok {
			return match
		}
		search = nextSearch
	}
	return collectionHistoryCall{start: -1}
}

func parseCollectionHistoryCallAt(segment string, offset int, search int) (collectionHistoryCall, int, bool) {
	openBracket := strings.Index(segment[search:], "[")
	if openBracket < 0 {
		return collectionHistoryCall{}, len(segment), false
	}
	openBracket += search
	nameStart, nameEnd, ok := collectionHistoryNameRange(segment, openBracket)
	if !ok {
		return collectionHistoryCall{}, openBracket + 1, false
	}
	closeBracket, rawLookback, ok := collectionHistoryLookback(segment, openBracket)
	if !ok {
		return collectionHistoryCall{}, closeBracket + 1, false
	}
	methodStart, methodEnd, closeParen, ok := collectionHistoryMethodRange(segment, closeBracket)
	if !ok {
		return collectionHistoryCall{}, max(methodEnd, closeBracket+1), false
	}
	callText := segment[nameStart : closeParen+1]
	return collectionHistoryCall{
		start:     offset + nameStart,
		end:       offset + closeParen + 1,
		name:      strings.TrimSpace(segment[nameStart:nameEnd]),
		lookback:  rawLookback,
		operation: strings.ToLower(strings.TrimSpace(segment[methodStart:methodEnd])),
		args:      collectionHistoryCallArgs(callText),
	}, closeParen, true
}

func collectionHistoryNameRange(segment string, openBracket int) (int, int, bool) {
	nameEnd := openBracket
	nameStart := nameEnd - 1
	for nameStart >= 0 {
		ch := segment[nameStart]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' {
			nameStart--
			continue
		}
		break
	}
	nameStart++
	return nameStart, nameEnd, nameStart < nameEnd
}

func collectionHistoryLookback(segment string, openBracket int) (int, string, bool) {
	closeBracket := strings.Index(segment[openBracket:], "]")
	if closeBracket < 0 {
		return 0, "", false
	}
	closeBracket += openBracket
	rawLookback := strings.TrimSpace(segment[openBracket+1 : closeBracket])
	if !numberPattern.MatchString(rawLookback) || strings.Contains(rawLookback, ".") || strings.HasPrefix(rawLookback, "-") {
		return closeBracket, "", false
	}
	return closeBracket, rawLookback, true
}

func collectionHistoryMethodRange(segment string, closeBracket int) (int, int, int, bool) {
	after := strings.TrimLeft(segment[closeBracket+1:], " \t")
	skipped := len(segment[closeBracket+1:]) - len(after)
	if !strings.HasPrefix(after, ".") {
		return 0, closeBracket + 1, 0, false
	}
	methodStart := closeBracket + 1 + skipped + 1
	methodEnd := methodStart
	for methodEnd < len(segment) {
		ch := segment[methodEnd]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' {
			methodEnd++
			continue
		}
		break
	}
	if methodEnd == methodStart || methodEnd >= len(segment) || segment[methodEnd] != '(' {
		return methodStart, methodEnd, 0, false
	}
	closeParen := matchingParen(segment, methodEnd)
	if closeParen < 0 {
		return methodStart, methodEnd, 0, false
	}
	return methodStart, methodEnd, closeParen, true
}

func collectionHistoryCallArgs(call string) string {
	open := strings.LastIndex(call, "(")
	close := strings.LastIndex(call, ")")
	if open < 0 || close <= open {
		return ""
	}
	return strings.TrimSpace(call[open+1 : close])
}

func collectionHistoryReplacement(call collectionHistoryCall) string {
	replacement := "collection_array_" + call.operation + "(history(" + call.name + ", " + call.lookback + ")"
	if call.args != "" {
		replacement += ", " + call.args
	}
	return replacement + ")"
}

func collectionHistoryReadOperation(operation string) bool {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "get", "size", "first", "last", "join", "median", "mode", "range", "stdev", "variance":
		return true
	default:
		return false
	}
}

func collectionStatement(line parsedLine, namespace string, operation string, target string, resultName string, typeArgs string, args []string, mode strategyir.AssignmentMode) *strategyir.CollectionStmt {
	return &strategyir.CollectionStmt{
		Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Namespace:  namespace,
		Operation:  operation,
		Target:     target,
		ResultName: resultName,
		TypeArgs:   strings.TrimSpace(typeArgs),
		Arguments:  append([]string(nil), args...),
		Mode:       mode,
	}
}

func collectionConstructorOperation(namespace string, operation string) bool {
	operation = strings.ToLower(strings.TrimSpace(operation))
	if strings.HasPrefix(operation, "new") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(namespace), "array") && operation == "from"
}

func functionCallNameText(expression string) string {
	trimmed := strings.TrimSpace(expression)
	open := strings.Index(trimmed, "(")
	if open <= 0 {
		return ""
	}
	return strings.TrimSpace(trimmed[:open])
}

func collectionResultNamespace(namespace string, operation string) string {
	switch strings.ToLower(strings.TrimSpace(namespace)) {
	case "array":
		switch strings.ToLower(strings.TrimSpace(operation)) {
		case "new", "new_float", "new_int", "new_bool", "new_string", "copy", "slice", "from", "sort_indices", "abs":
			return "array"
		}
	case "map":
		switch strings.ToLower(strings.TrimSpace(operation)) {
		case "new", "copy":
			return "map"
		case "keys", "values":
			return "array"
		}
	case "matrix":
		switch strings.ToLower(strings.TrimSpace(operation)) {
		case "new", "copy":
			return "matrix"
		case "remove_row", "remove_col":
			return "array"
		}
	}
	return ""
}

func parseExecutableCollectionCall(call string) (string, string, string, bool) {
	lower := strings.ToLower(strings.TrimSpace(call))
	dot := strings.Index(lower, ".")
	if dot <= 0 {
		return "", "", "", false
	}
	namespace := lower[:dot]
	operationWithType := lower[dot+1:]
	operation := operationWithType
	typeArgs := ""
	if open := strings.Index(operationWithType, "<"); open >= 0 && strings.HasSuffix(operationWithType, ">") {
		operation = operationWithType[:open]
		typeArgs = strings.TrimSpace(operationWithType[open+1 : len(operationWithType)-1])
	}
	if !executableCollectionOperations[namespace][operation] {
		return "", "", "", false
	}
	return namespace, operation, typeArgs, true
}

func (s *parseState) resolveExecutableCollectionCall(call string) (string, string, string, bool) {
	if namespace, operation, typeArgs, ok := parseExecutableCollectionCall(call); ok {
		return namespace, operation, typeArgs, true
	}
	dot := strings.Index(call, ".")
	if dot <= 0 {
		return "", "", "", false
	}
	target := strings.ToLower(strings.TrimSpace(call[:dot]))
	namespace, ok := s.collectionNamespaces[target]
	operation := strings.ToLower(strings.TrimSpace(call[dot+1:]))
	if !ok {
		lastDot := strings.LastIndex(call, ".")
		if lastDot <= 0 || lastDot == len(call)-1 {
			return "", "", "", false
		}
		targetExpression := strings.TrimSpace(call[:lastDot])
		operation = strings.ToLower(strings.TrimSpace(call[lastDot+1:]))
		namespace = s.collectionNamespaceForTargetExpression(targetExpression)
		if namespace == "" {
			return "", "", "", false
		}
	}
	if !executableCollectionOperations[namespace][operation] {
		return "", "", "", false
	}
	return namespace, operation, "", true
}

func (s *parseState) collectionNamespaceForTargetExpression(target string) string {
	trimmed := strings.TrimSpace(target)
	if namespace := s.collectionNamespaces[strings.ToLower(trimmed)]; namespace != "" {
		return namespace
	}
	dot := strings.Index(trimmed, ".")
	if dot <= 0 {
		return ""
	}
	objectName := strings.TrimSpace(trimmed[:dot])
	fieldName := strings.TrimSpace(trimmed[dot+1:])
	typeName := s.objectTypes[strings.ToLower(objectName)]
	if typeName == "" {
		return ""
	}
	definition := s.udtTypes[strings.ToLower(typeName)]
	field, ok := objectFieldDefinition(definition, fieldName)
	if !ok {
		return ""
	}
	return collectionNamespaceFromType(field.Type)
}

func collectionNamespaceFromType(typeName string) string {
	lower := strings.ToLower(strings.TrimSpace(typeName))
	switch {
	case lower == "array" || strings.HasPrefix(lower, "array<"):
		return "array"
	case lower == "map" || strings.HasPrefix(lower, "map<"):
		return "map"
	case lower == "matrix" || strings.HasPrefix(lower, "matrix<"):
		return "matrix"
	default:
		return ""
	}
}

func (s *parseState) collectionTargetAndArguments(namespace string, call string, args []string) (string, []string, error) {
	dot := strings.Index(call, ".")
	receiver := strings.TrimSpace(call[:dot])
	if strings.EqualFold(receiver, namespace) {
		if len(args) == 0 {
			return "", nil, fmt.Errorf("%s requires a collection argument", call)
		}
		return strings.TrimSpace(args[0]), append([]string(nil), args[1:]...), nil
	}
	if namespaceForTarget := s.collectionNamespaceForTargetExpression(strings.TrimSpace(call[:strings.LastIndex(call, ".")])); namespaceForTarget == namespace {
		return strings.TrimSpace(call[:strings.LastIndex(call, ".")]), append([]string(nil), args...), nil
	}
	return receiver, append([]string(nil), args...), nil
}

func supportedExecutableCollectionLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if match := typedAssignmentPattern.FindStringSubmatch(trimmed); match != nil {
		call, _, ok := parseFunctionCallText(strings.TrimSpace(match[5]))
		if !ok {
			return false
		}
		_, operation, _, ok := parseExecutableCollectionCall(call)
		return ok && strings.HasPrefix(operation, "new")
	}
	if match := assignmentPattern.FindStringSubmatch(trimmed); match != nil {
		call, _, ok := parseFunctionCallText(strings.TrimSpace(match[4]))
		if ok {
			_, _, _, ok = parseExecutableCollectionCall(call)
			if ok {
				return true
			}
		}
		return lineContainsOnlySupportedCollectionCalls(trimmed)
	}
	call, _, ok := parseFunctionCallText(trimmed)
	if !ok {
		matches := collectionCallPattern.FindAllStringSubmatch(trimmed, -1)
		if len(matches) == 0 {
			return false
		}
		for _, match := range matches {
			if len(match) < 3 || !executableCollectionOperations[strings.ToLower(match[1])][strings.ToLower(match[2])] {
				return false
			}
		}
		return true
	}
	_, operation, _, ok := parseExecutableCollectionCall(call)
	return ok && mutatingCollectionOperations[operation] && !strings.HasPrefix(operation, "new")
}

func lineContainsOnlySupportedCollectionCalls(line string) bool {
	matches := collectionCallPattern.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return false
	}
	for _, match := range matches {
		if len(match) < 3 || !executableCollectionOperations[strings.ToLower(match[1])][strings.ToLower(match[2])] {
			return false
		}
	}
	return true
}

func (s *parseState) lowerCollectionReadCalls(expression string) (string, error) {
	result := expression
	for {
		start, call, args, close, ok := nextCollectionReadCall(result, s.collectionNamespaces)
		if !ok {
			return result, nil
		}
		namespace, operation, _, direct := parseExecutableCollectionCall(call)
		target := ""
		if direct {
			if !readableCollectionOperations[operation] {
				return result, fmt.Errorf("collection operation %s is not valid in an expression", call)
			}
			if !collectionConstructorOperation(namespace, operation) {
				if len(args) == 0 {
					return result, fmt.Errorf("collection operation %s is not valid in an expression", call)
				}
				target = strings.TrimSpace(args[0])
				args = args[1:]
			}
		} else {
			dot := strings.LastIndex(call, ".")
			target = strings.TrimSpace(call[:dot])
			namespace = s.collectionNamespaceForTargetExpression(target)
			operation = strings.ToLower(strings.TrimSpace(call[dot+1:]))
			if !readableCollectionOperations[operation] {
				return result, fmt.Errorf("collection operation %s is not valid in an expression", call)
			}
		}
		replacementArgs := append([]string(nil), args...)
		if target != "" {
			replacementArgs = append([]string{target}, args...)
		}
		replacement := fmt.Sprintf("collection_%s_%s(%s)", namespace, operation, strings.Join(replacementArgs, ", "))
		result = result[:start] + replacement + result[close+1:]
	}
}

//nolint:funlen
func nextCollectionReadCall(expression string, namespaces map[string]string) (int, string, []string, int, bool) {
	bestStart := -1
	bestCall := ""
	bestArgs := []string(nil)
	bestClose := -1
	bestOpen := -1
	for _, match := range collectionCallPattern.FindAllStringSubmatchIndex(expression, -1) {
		if len(match) < 8 {
			continue
		}
		namespace := strings.ToLower(expression[match[2]:match[3]])
		operation := strings.ToLower(expression[match[4]:match[5]])
		if !readableCollectionOperations[operation] {
			continue
		}
		open := match[1] - 1
		close := matchingParen(expression, open)
		if close < 0 || open <= bestOpen {
			continue
		}
		bestStart = match[0]
		bestCall = namespace + "." + operation
		bestArgs = splitArguments(expression[open+1 : close])
		bestClose = close
		bestOpen = open
	}
	for _, match := range objectCallPattern.FindAllStringSubmatchIndex(expression, -1) {
		if len(match) < 6 {
			continue
		}
		target := strings.TrimSpace(expression[match[2]:match[3]])
		namespace := namespaces[strings.ToLower(target)]
		operation := strings.ToLower(strings.TrimSpace(expression[match[4]:match[5]]))
		if namespace == "" || !readableCollectionOperations[operation] {
			continue
		}
		open := match[1] - 1
		close := matchingParen(expression, open)
		if close < 0 || open <= bestOpen {
			continue
		}
		bestStart = match[0]
		bestCall = target + "." + operation
		bestArgs = splitArguments(expression[open+1 : close])
		bestClose = close
		bestOpen = open
	}
	for _, match := range objectFieldCollectionCallPattern.FindAllStringSubmatchIndex(expression, -1) {
		if len(match) < 8 {
			continue
		}
		target := strings.TrimSpace(expression[match[2]:match[3]]) + "." + strings.TrimSpace(expression[match[4]:match[5]])
		namespace := namespaces[strings.ToLower(target)]
		operation := strings.ToLower(strings.TrimSpace(expression[match[6]:match[7]]))
		if namespace == "" || !readableCollectionOperations[operation] {
			continue
		}
		open := match[1] - 1
		close := matchingParen(expression, open)
		if close < 0 || open < bestOpen {
			continue
		}
		bestStart = match[0]
		bestCall = target + "." + operation
		bestArgs = splitArguments(expression[open+1 : close])
		bestClose = close
		bestOpen = open
	}
	return bestStart, bestCall, bestArgs, bestClose, bestStart >= 0
}
