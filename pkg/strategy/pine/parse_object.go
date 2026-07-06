package pine

import (
	"fmt"
	"strings"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func (s *parseState) parseExecutableTypeDefinition(index int) (int, error) {
	line := s.lines[index]
	if line.indent != 0 {
		return index, fmt.Errorf("pine line %d: type declarations must be top level", line.number)
	}
	name := firstDeclarationName(strings.TrimSpace(line.trimmed[len("type "):]))
	if !identifierPattern.MatchString(name) {
		return index, fmt.Errorf("pine line %d: invalid type name %q", line.number, name)
	}
	if _, exists := s.udtTypes[strings.ToLower(name)]; exists {
		return index, fmt.Errorf("pine line %d: type %s is already declared", line.number, name)
	}
	fields := make([]strategyir.ObjectField, 0)
	next := index + 1
	seen := map[string]bool{}
	for next < len(s.lines) && s.lines[next].indent > line.indent {
		fieldLine := s.lines[next]
		field := parseSemanticParameter(fieldLine.trimmed)
		if field.Type == "" || field.Name == "" || !identifierPattern.MatchString(field.Name) {
			return index, fmt.Errorf("pine line %d: type field requires 'type name [= default]'", fieldLine.number)
		}
		key := strings.ToLower(field.Name)
		if seen[key] {
			return index, fmt.Errorf("pine line %d: type %s repeats field %s", fieldLine.number, name, field.Name)
		}
		seen[key] = true
		defaultValue := strings.TrimSpace(field.Default)
		if defaultValue == "" {
			defaultValue = "na"
		} else {
			defaultValue = s.normalizeExpression(defaultValue)
			if err := s.takeNormalizationErr(fieldLine.number); err != nil {
				return index, err
			}
			if err := validateExpression(fieldLine.number, "type field default", defaultValue); err != nil {
				return index, err
			}
		}
		fields = append(fields, strategyir.ObjectField{Name: field.Name, Type: field.Type, Default: defaultValue})
		next++
	}
	if len(fields) == 0 {
		return index, fmt.Errorf("pine line %d: type %s requires at least one field", line.number, name)
	}
	definition := strategyir.TypeDefinition{
		Range:  strategyir.SourceRange{StartLine: line.number, EndLine: s.lines[next-1].number},
		Name:   name,
		Fields: fields,
	}
	s.udtTypes[strings.ToLower(name)] = definition
	s.typeDefinitions = append(s.typeDefinitions, definition)
	return next, nil
}

//nolint:funlen
func (s *parseState) parseExecutableMethodDefinition(index int) (int, error) {
	line := s.lines[index]
	if line.indent != 0 {
		return index, fmt.Errorf("pine line %d: method declarations must be top level", line.number)
	}
	afterKeyword := strings.TrimSpace(line.trimmed[len("method "):])
	before, after, ok := strings.Cut(afterKeyword, "=>")
	declarationText := afterKeyword
	body := ""
	if ok {
		declarationText = strings.TrimSpace(before)
		body = strings.TrimSpace(after)
	}
	name, receiver, parameters := parseMethodDeclaration(declarationText)
	if receiver == nil || receiver.Type == "" || receiver.Name == "" {
		return index, fmt.Errorf("pine line %d: method %s requires a typed receiver", line.number, name)
	}
	if _, ok := s.udtTypes[strings.ToLower(receiver.Type)]; !ok {
		return index, fmt.Errorf("pine line %d: method %s receiver type %s is not declared", line.number, name, receiver.Type)
	}
	next := index + 1
	if body == "" {
		if next >= len(s.lines) || s.lines[next].indent <= line.indent {
			return index, fmt.Errorf("pine line %d: method %s requires one pure expression body", line.number, name)
		}
		endIndex := next
		for endIndex < len(s.lines) && s.lines[endIndex].indent > line.indent {
			endIndex++
		}
		compiledBody, compileErr := compileUDFBody(s.lines[next:endIndex])
		if compileErr != nil {
			return index, fmt.Errorf("pine line %d: method %s: %w", line.number, name, compileErr)
		}
		body = compiledBody
		next = endIndex
	}
	if !requestSecurityExpressionIsPure(body) || strings.Contains(body, ":=") || strings.Contains(strings.ToLower(body), strings.ToLower(name)+"(") {
		return index, fmt.Errorf("pine line %d: method %s must be side-effect-free and non-recursive", line.number, name)
	}
	body = s.normalizeExpression(body)
	if err := s.takeNormalizationErr(line.number); err != nil {
		return index, err
	}
	if err := validateExpression(line.number, "method body", body); err != nil {
		return index, err
	}
	params := make([]strategyir.ObjectParameter, 0)
	for _, parameter := range parameters[1:] {
		if parameter.Name == "" {
			return index, fmt.Errorf("pine line %d: method %s parameter name is required", line.number, name)
		}
		defaultValue := strings.TrimSpace(parameter.Default)
		if defaultValue != "" {
			defaultValue = s.normalizeExpression(defaultValue)
			if err := s.takeNormalizationErr(line.number); err != nil {
				return index, err
			}
			if err := validateExpression(line.number, "method parameter default", defaultValue); err != nil {
				return index, err
			}
		}
		params = append(params, strategyir.ObjectParameter{Name: parameter.Name, Type: parameter.Type, Default: defaultValue})
	}
	definition := strategyir.MethodDefinition{
		Range:        strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Name:         name,
		ReceiverType: receiver.Type,
		ReceiverName: receiver.Name,
		Parameters:   params,
		Body:         body,
	}
	key := strings.ToLower(receiver.Type) + "." + strings.ToLower(name)
	s.udtMethods[key] = append(s.udtMethods[key], definition)
	s.methodDefinitions = append(s.methodDefinitions, definition)
	return next, nil
}

//nolint:funlen
func (s *parseState) parseObjectStatement(line parsedLine) (strategyir.Statement, bool, error) {
	if match := objectFieldAssignPattern.FindStringSubmatch(line.trimmed); match != nil {
		target := strings.TrimSpace(match[1])
		fieldName := strings.TrimSpace(match[2])
		operator := strings.TrimSpace(match[3])
		if operator != ":=" {
			return nil, true, fmt.Errorf("pine line %d: object field updates must use :=", line.number)
		}
		typeName := s.objectTypes[strings.ToLower(target)]
		if typeName == "" {
			return nil, false, nil
		}
		definition := s.udtTypes[strings.ToLower(typeName)]
		field, ok := objectFieldDefinition(definition, fieldName)
		if !ok {
			return nil, true, fmt.Errorf("pine line %d: type %s has no field %s", line.number, typeName, fieldName)
		}
		expression := s.normalizeExpression(strings.TrimSpace(match[4]))
		if err := s.takeNormalizationErr(line.number); err != nil {
			return nil, true, err
		}
		if err := validateExpression(line.number, "object field assignment", expression); err != nil {
			return nil, true, err
		}
		return &strategyir.ObjectStmt{
			Range:     strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Operation: "field_set",
			TypeName:  definition.Name,
			Method:    field.Name,
			Target:    target,
			Arguments: []string{expression},
		}, true, nil
	}

	match := assignmentPattern.FindStringSubmatch(line.trimmed)
	if match == nil {
		return nil, false, nil
	}
	name := strings.TrimSpace(match[2])
	call, args, ok := parseFunctionCallText(strings.TrimSpace(match[4]))
	if !ok {
		return nil, false, nil
	}
	dot := strings.Index(call, ".")
	if dot <= 0 {
		return nil, false, nil
	}
	receiver := strings.TrimSpace(call[:dot])
	member := strings.TrimSpace(call[dot+1:])
	mode := assignmentMode(strings.TrimSpace(match[1]), strings.TrimSpace(match[3]))
	if definition, ok := s.udtTypes[strings.ToLower(receiver)]; ok && strings.EqualFold(member, "new") {
		normalized, err := s.normalizeObjectConstructorArguments(line.number, args, definition.Fields)
		if err != nil {
			return nil, true, err
		}
		if len(normalized) > len(definition.Fields) {
			return nil, true, fmt.Errorf("pine line %d: %s.new expects at most %d arguments", line.number, definition.Name, len(definition.Fields))
		}
		s.objectTypes[strings.ToLower(name)] = definition.Name
		s.objectPersistent[strings.ToLower(name)] = mode == strategyir.AssignmentModeVar
		for _, field := range definition.Fields {
			if namespace := collectionNamespaceFromType(field.Type); namespace != "" {
				s.collectionNamespaces[strings.ToLower(name+"."+field.Name)] = namespace
			}
		}
		return &strategyir.ObjectStmt{
			Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
			Operation:  "constructor",
			TypeName:   definition.Name,
			ResultName: name,
			Arguments:  normalized,
			Mode:       mode,
		}, true, nil
	}
	objectType := s.objectTypes[strings.ToLower(receiver)]
	if objectType == "" {
		return nil, false, nil
	}
	methods := s.udtMethods[strings.ToLower(objectType)+"."+strings.ToLower(member)]
	if len(methods) == 0 {
		return nil, false, nil
	}
	normalized, err := s.normalizeObjectMethodArguments(line.number, args, methodParameters(methods))
	if err != nil {
		return nil, true, err
	}
	method, ok := selectExecutableMethod(methods, len(normalized))
	if !ok {
		return nil, true, fmt.Errorf("pine line %d: method %s has no overload for %d arguments", line.number, member, len(normalized))
	}
	return &strategyir.ObjectStmt{
		Range:      strategyir.SourceRange{StartLine: line.number, EndLine: line.number},
		Operation:  "method",
		TypeName:   objectType,
		Method:     method.Name,
		Target:     receiver,
		ResultName: name,
		Arguments:  normalized,
		Mode:       mode,
	}, true, nil
}

func objectFieldDefinition(definition strategyir.TypeDefinition, name string) (strategyir.ObjectField, bool) {
	for _, field := range definition.Fields {
		if strings.EqualFold(field.Name, name) {
			return field, true
		}
	}
	return strategyir.ObjectField{}, false
}

func (s *parseState) normalizeObjectConstructorArguments(lineNumber int, args []string, fields []strategyir.ObjectField) ([]string, error) {
	if !hasNamedArgument(args) {
		return s.normalizeCollectionArguments(lineNumber, args)
	}
	values := make([]string, len(fields))
	fieldIndexes := map[string]int{}
	for index, field := range fields {
		fieldIndexes[strings.ToLower(field.Name)] = index
	}
	nextPositional := 0
	for _, raw := range args {
		key, value, named := splitNamedArg(raw)
		target := nextPositional
		if named {
			index, ok := fieldIndexes[strings.ToLower(key)]
			if !ok {
				return nil, fmt.Errorf("pine line %d: unknown constructor field %s", lineNumber, key)
			}
			target = index
		} else {
			value = raw
			for target < len(values) && values[target] != "" {
				target++
			}
			nextPositional = target + 1
		}
		if target >= len(values) {
			return nil, fmt.Errorf("pine line %d: constructor received too many arguments", lineNumber)
		}
		if values[target] != "" {
			return nil, fmt.Errorf("pine line %d: duplicate constructor argument %s", lineNumber, fields[target].Name)
		}
		normalized := s.normalizeExpression(value)
		if err := s.takeNormalizationErr(lineNumber); err != nil {
			return nil, err
		}
		if err := validateExpression(lineNumber, "constructor argument", normalized); err != nil {
			return nil, err
		}
		values[target] = normalized
	}
	last := -1
	for index := range values {
		if values[index] != "" {
			last = index
		}
	}
	if last < 0 {
		return nil, nil
	}
	for index := 0; index <= last; index++ {
		if values[index] == "" {
			values[index] = fields[index].Default
		}
	}
	return values[:last+1], nil
}

func (s *parseState) normalizeObjectMethodArguments(lineNumber int, args []string, parameters []strategyir.ObjectParameter) ([]string, error) {
	if !hasNamedArgument(args) {
		return s.normalizeCollectionArguments(lineNumber, args)
	}
	values := make([]string, len(parameters))
	parameterIndexes := map[string]int{}
	for index, parameter := range parameters {
		parameterIndexes[strings.ToLower(parameter.Name)] = index
	}
	nextPositional := 0
	for _, raw := range args {
		key, value, named := splitNamedArg(raw)
		target := nextPositional
		if named {
			index, ok := parameterIndexes[strings.ToLower(key)]
			if !ok {
				return nil, fmt.Errorf("pine line %d: unknown method parameter %s", lineNumber, key)
			}
			target = index
		} else {
			value = raw
			for target < len(values) && values[target] != "" {
				target++
			}
			nextPositional = target + 1
		}
		if target >= len(values) {
			return nil, fmt.Errorf("pine line %d: method received too many arguments", lineNumber)
		}
		if values[target] != "" {
			return nil, fmt.Errorf("pine line %d: duplicate method argument %s", lineNumber, parameters[target].Name)
		}
		normalized := s.normalizeExpression(value)
		if err := s.takeNormalizationErr(lineNumber); err != nil {
			return nil, err
		}
		if err := validateExpression(lineNumber, "method argument", normalized); err != nil {
			return nil, err
		}
		values[target] = normalized
	}
	last := -1
	for index := range values {
		if values[index] != "" {
			last = index
		}
	}
	if last < 0 {
		return nil, nil
	}
	for index := 0; index <= last; index++ {
		if values[index] == "" {
			if strings.TrimSpace(parameters[index].Default) == "" {
				return nil, fmt.Errorf("pine line %d: method argument %s is required", lineNumber, parameters[index].Name)
			}
			values[index] = parameters[index].Default
		}
	}
	return values[:last+1], nil
}

func hasNamedArgument(args []string) bool {
	for _, arg := range args {
		if _, _, ok := splitNamedArg(arg); ok {
			return true
		}
	}
	return false
}

func methodParameters(methods []strategyir.MethodDefinition) []strategyir.ObjectParameter {
	maxParams := []strategyir.ObjectParameter(nil)
	for _, method := range methods {
		if len(method.Parameters) > len(maxParams) {
			maxParams = method.Parameters
		}
	}
	return maxParams
}

func (s *parseState) lowerObjectMethodCalls(expression string) (string, error) {
	result := expression
	for {
		start, typeName, call, args, close, ok := s.nextObjectHistoryMethodCall(result)
		expressionReceiver := ok
		if !ok {
			start, call, args, close, ok = s.nextObjectMethodCall(result)
			expressionReceiver = false
		}
		if !ok {
			start, typeName, call, args, close, ok = s.nextObjectMethodExpressionReceiverCall(result)
			expressionReceiver = ok
		}
		if !ok {
			return result, nil
		}
		dot := strings.LastIndex(call, ".")
		target := strings.TrimSpace(call[:dot])
		methodName := strings.TrimSpace(call[dot+1:])
		if !expressionReceiver {
			typeName = s.objectTypes[strings.ToLower(target)]
		}
		methods := s.udtMethods[strings.ToLower(typeName)+"."+strings.ToLower(methodName)]
		normalizedArgs, err := reorderObjectMethodCallArguments(args, methodParameters(methods))
		if err != nil {
			return result, err
		}
		method, ok := selectExecutableMethod(methods, len(normalizedArgs))
		if !ok {
			return result, fmt.Errorf("method %s has no overload for %d arguments", methodName, len(normalizedArgs))
		}
		replacementArgs := []string{strconvQuote(typeName), strconvQuote(method.Name), target}
		replacementArgs = append(replacementArgs, normalizedArgs...)
		replacement := fmt.Sprintf("object_method(%s)", strings.Join(replacementArgs, ", "))
		result = result[:start] + replacement + result[close+1:]
	}
}

func (s *parseState) nextObjectHistoryMethodCall(expression string) (int, string, string, []string, int, bool) {
	bestStart := -1
	bestType := ""
	bestCall := ""
	bestArgs := []string(nil)
	bestClose := -1
	bestOpen := -1
	for _, match := range objectHistoryMethodPattern.FindAllStringSubmatchIndex(expression, -1) {
		if len(match) < 8 {
			continue
		}
		target := strings.TrimSpace(expression[match[2]:match[3]])
		typeName := s.objectTypes[strings.ToLower(target)]
		if typeName == "" {
			continue
		}
		lookback := strings.TrimSpace(expression[match[4]:match[5]])
		methodName := strings.TrimSpace(expression[match[6]:match[7]])
		if len(s.udtMethods[strings.ToLower(typeName)+"."+strings.ToLower(methodName)]) == 0 {
			continue
		}
		open := match[1] - 1
		close := matchingParen(expression, open)
		if close < 0 || open <= bestOpen {
			continue
		}
		bestStart = match[0]
		bestType = typeName
		bestCall = fmt.Sprintf("history(%s, %s).%s", target, lookback, methodName)
		bestArgs = splitArguments(expression[open+1 : close])
		bestClose = close
		bestOpen = open
	}
	return bestStart, bestType, bestCall, bestArgs, bestClose, bestStart >= 0
}

func (s *parseState) nextObjectMethodExpressionReceiverCall(expression string) (int, string, string, []string, int, bool) {
	search := 0
	for search < len(expression) {
		start := strings.Index(strings.ToLower(expression[search:]), "object_method(")
		if start < 0 {
			return -1, "", "", nil, -1, false
		}
		start += search
		open := start + len("object_method")
		close := matchingParen(expression, open)
		if close < 0 {
			return -1, "", "", nil, -1, false
		}
		after := strings.TrimLeft(expression[close+1:], " \t")
		skipped := len(expression[close+1:]) - len(after)
		if !strings.HasPrefix(after, ".") {
			search = close + 1
			continue
		}
		methodStart := close + 1 + skipped + 1
		methodEnd := methodStart
		for methodEnd < len(expression) {
			ch := expression[methodEnd]
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' {
				methodEnd++
				continue
			}
			break
		}
		if methodEnd == methodStart || methodEnd >= len(expression) || expression[methodEnd] != '(' {
			search = methodEnd
			continue
		}
		callClose := matchingParen(expression, methodEnd)
		if callClose < 0 {
			return -1, "", "", nil, -1, false
		}
		receiver := strings.TrimSpace(expression[start : close+1])
		typeName := objectMethodExpressionReceiverType(receiver)
		methodName := strings.TrimSpace(expression[methodStart:methodEnd])
		if typeName == "" || len(s.udtMethods[strings.ToLower(typeName)+"."+strings.ToLower(methodName)]) == 0 {
			search = callClose + 1
			continue
		}
		return start, typeName, receiver + "." + methodName, splitArguments(expression[methodEnd+1 : callClose]), callClose, true
	}
	return -1, "", "", nil, -1, false
}

func objectMethodExpressionReceiverType(expression string) string {
	call, args, ok := parseFunctionCallText(strings.TrimSpace(expression))
	if !ok || !strings.EqualFold(call, "object_method") || len(args) < 1 {
		return ""
	}
	return unquote(strings.TrimSpace(args[0]))
}

func reorderObjectMethodCallArguments(args []string, parameters []strategyir.ObjectParameter) ([]string, error) {
	if !hasNamedArgument(args) {
		return append([]string(nil), args...), nil
	}
	values := make([]string, len(parameters))
	provided := make([]bool, len(parameters))
	last := -1
	for index, raw := range args {
		name, value, named := splitNamedArg(raw)
		if !named {
			if index >= len(parameters) {
				return nil, fmt.Errorf("method received too many arguments")
			}
			values[index] = strings.TrimSpace(raw)
			provided[index] = true
			last = index
			continue
		}
		found := -1
		for parameterIndex, parameter := range parameters {
			if strings.EqualFold(parameter.Name, strings.TrimSpace(name)) {
				found = parameterIndex
				break
			}
		}
		if found < 0 {
			return nil, fmt.Errorf("unknown method argument %s", strings.TrimSpace(name))
		}
		values[found] = strings.TrimSpace(value)
		provided[found] = true
		if found > last {
			last = found
		}
	}
	if last < 0 {
		return []string{}, nil
	}
	for index := 0; index <= last; index++ {
		if !provided[index] {
			if strings.TrimSpace(parameters[index].Default) == "" {
				return nil, fmt.Errorf("method argument %s is required", parameters[index].Name)
			}
			values[index] = parameters[index].Default
		}
	}
	return values[:last+1], nil
}

func (s *parseState) nextObjectMethodCall(expression string) (int, string, []string, int, bool) {
	bestStart := -1
	bestCall := ""
	bestArgs := []string(nil)
	bestClose := -1
	bestOpen := -1
	for _, match := range objectCallPattern.FindAllStringSubmatchIndex(expression, -1) {
		if len(match) < 6 {
			continue
		}
		target := strings.TrimSpace(expression[match[2]:match[3]])
		typeName := s.objectTypes[strings.ToLower(target)]
		if typeName == "" {
			continue
		}
		methodName := strings.TrimSpace(expression[match[4]:match[5]])
		if len(s.udtMethods[strings.ToLower(typeName)+"."+strings.ToLower(methodName)]) == 0 {
			continue
		}
		open := match[1] - 1
		close := matchingParen(expression, open)
		if close < 0 || open <= bestOpen {
			continue
		}
		bestStart = match[0]
		bestCall = target + "." + methodName
		bestArgs = splitArguments(expression[open+1 : close])
		bestClose = close
		bestOpen = open
	}
	return bestStart, bestCall, bestArgs, bestClose, bestStart >= 0
}

func strconvQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func selectExecutableMethod(methods []strategyir.MethodDefinition, argCount int) (strategyir.MethodDefinition, bool) {
	for _, method := range methods {
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
