package pine

import (
	"fmt"
	"strings"
)

func semanticCollectionDeclaration(line ASTLine) (SemanticDeclaration, bool) {
	namespace, call, typeArgs := collectionCallInfo(line.Text)
	annotationNamespace, annotationTypeArgs := collectionTypeAnnotationInfo(line.Type)
	if namespace == "" {
		namespace = annotationNamespace
	}
	if typeArgs == "" {
		typeArgs = annotationTypeArgs
	}
	operation := strings.TrimPrefix(call, namespace+".")
	if namespace == "" || (call != "" && !strings.HasPrefix(operation, "new")) {
		return SemanticDeclaration{}, false
	}
	executable := false
	if operation != "" {
		executable = collectionOperationExecutable(namespace, operation)
	}
	reason := "parse-only; runtime collection execution is not enabled"
	if executable {
		reason = ""
	}
	signature := call
	if signature == "" {
		signature = namespace
	}
	if typeArgs != "" {
		signature += "<" + typeArgs + ">"
	}
	return SemanticDeclaration{
		Line:              line.Line,
		Kind:              "collection",
		Name:              line.Name,
		Namespace:         namespace,
		Call:              call,
		TypeArgs:          typeArgs,
		Signature:         signature,
		Executable:        executable,
		Reason:            reason,
		UnsupportedReason: reason,
	}, true
}

func collectionTypeAnnotationInfo(annotation string) (string, string) {
	trimmed := normalizeTypeAnnotation(annotation)
	if trimmed == "" {
		return "", ""
	}
	lower := strings.ToLower(trimmed)
	for _, namespace := range []string{"array", "map", "matrix"} {
		if lower == namespace {
			return namespace, ""
		}
		prefix := namespace + "<"
		if strings.HasPrefix(lower, prefix) && strings.HasSuffix(trimmed, ">") {
			return namespace, strings.TrimSpace(trimmed[len(namespace)+1 : len(trimmed)-1])
		}
	}
	return "", ""
}

func semanticCollectionDeclarationDiagnostics(line ASTLine, declaration SemanticDeclaration, operations []SemanticCollectionOperation) []Diagnostic {
	annotationNamespace, annotationTypeArgs := collectionTypeAnnotationInfo(line.Type)
	constructor, hasConstructor := assignedCollectionConstructor(operations, line.Name)
	diagnostics := collectionTypeArgumentDiagnostics(line, "type annotation", annotationNamespace, annotationTypeArgs)
	if !hasConstructor {
		return diagnostics
	}
	if annotationNamespace != "" && constructor.Namespace != "" && annotationNamespace != constructor.Namespace {
		diagnostics = append(diagnostics, semanticDiagnostic(
			line,
			"PINE_SEMANTIC_COLLECTION_TYPE",
			fmt.Sprintf("%s declaration cannot be initialized with %s", annotationNamespace, constructor.Call),
		))
		return diagnostics
	}
	annotationArgs := collectionTypeArguments(annotationTypeArgs)
	constructorArgs := collectionConstructorTypeArguments(constructor)
	if collectionTypeArgumentsHaveExpectedArity(annotationNamespace, annotationArgs) &&
		collectionTypeArgumentsHaveExpectedArity(constructor.Namespace, constructorArgs) &&
		!equalCollectionTypeArguments(annotationArgs, constructorArgs) {
		diagnostics = append(diagnostics, semanticDiagnostic(
			line,
			"PINE_SEMANTIC_COLLECTION_TYPE",
			fmt.Sprintf("%s type arguments <%s> do not match %s element types <%s>", declaration.Name, strings.Join(annotationArgs, ", "), constructor.Call, strings.Join(constructorArgs, ", ")),
		))
	}
	return diagnostics
}

func semanticCollectionOperations(line ASTLine, collectionNamespaces map[string]string) []SemanticCollectionOperation {
	matches := collectionCallPattern.FindAllStringSubmatchIndex(line.Text, -1)
	operations := make([]SemanticCollectionOperation, 0, len(matches))
	for _, match := range matches {
		if len(match) < 8 {
			continue
		}
		namespace := strings.ToLower(line.Text[match[2]:match[3]])
		operation := strings.ToLower(line.Text[match[4]:match[5]])
		typeArgs := ""
		if match[6] >= 0 && match[7] >= 0 {
			typeArgs = strings.TrimSpace(strings.Trim(line.Text[match[6]:match[7]], "<>"))
		}
		open := match[1] - 1
		close := matchingParen(line.Text, open)
		args := []string{}
		if close > open {
			args = splitArguments(line.Text[open+1 : close])
		}
		operations = append(operations, SemanticCollectionOperation{
			Line:       line.Line,
			Namespace:  namespace,
			Operation:  operation,
			Call:       namespace + "." + operation,
			TypeArgs:   typeArgs,
			Signature:  collectionOperationSignatureText(namespace + "." + operation),
			Target:     collectionOperationTarget(operation, args, line.Name),
			Arguments:  args,
			Mutates:    collectionOperationMutates(operation),
			Supported:  collectionOperationSupported(namespace + "." + operation),
			Executable: collectionOperationExecutable(namespace, operation),
			Reason:     collectionOperationReason(namespace, operation),
		})
	}
	operations = append(operations, semanticCollectionMethodOperations(line, collectionNamespaces)...)
	return operations
}

func semanticCollectionMethodOperations(line ASTLine, collectionNamespaces map[string]string) []SemanticCollectionOperation {
	if len(collectionNamespaces) == 0 {
		return nil
	}
	matches := objectCallPattern.FindAllStringSubmatchIndex(line.Text, -1)
	operations := make([]SemanticCollectionOperation, 0, len(matches))
	for _, match := range matches {
		if len(match) < 6 {
			continue
		}
		target := strings.TrimSpace(line.Text[match[2]:match[3]])
		namespace, ok := collectionNamespaces[strings.ToLower(target)]
		if !ok {
			continue
		}
		operation := strings.ToLower(strings.TrimSpace(line.Text[match[4]:match[5]]))
		if strings.HasPrefix(operation, "new") {
			continue
		}
		open := match[1] - 1
		close := matchingParen(line.Text, open)
		if close < open {
			continue
		}
		args := []string{target}
		args = append(args, splitArguments(line.Text[open+1:close])...)
		call := namespace + "." + operation
		operations = append(operations, SemanticCollectionOperation{
			Line:       line.Line,
			Namespace:  namespace,
			Operation:  operation,
			Call:       call,
			Signature:  collectionOperationSignatureText(call),
			Target:     target,
			Arguments:  args,
			Mutates:    collectionOperationMutates(operation),
			Supported:  collectionOperationSupported(call),
			Executable: collectionOperationExecutable(namespace, operation),
			Reason:     collectionOperationReason(namespace, operation),
		})
	}
	return operations
}

func semanticObjectOperations(line ASTLine, typeDeclarations map[string]SemanticDeclaration, methodDeclarations map[string][]SemanticDeclaration, objectTypes map[string]string) []SemanticObjectOperation {
	matches := objectCallPattern.FindAllStringSubmatchIndex(line.Text, -1)
	operations := make([]SemanticObjectOperation, 0, len(matches))
	for _, match := range matches {
		if len(match) < 6 {
			continue
		}
		receiver := strings.TrimSpace(line.Text[match[2]:match[3]])
		method := strings.TrimSpace(line.Text[match[4]:match[5]])
		open := match[1] - 1
		close := matchingParen(line.Text, open)
		if close < open {
			continue
		}
		args := splitArguments(line.Text[open+1 : close])
		if strings.EqualFold(method, "new") {
			declaration, ok := typeDeclarations[strings.ToLower(receiver)]
			if !ok {
				continue
			}
			operations = append(operations, SemanticObjectOperation{
				Line:       line.Line,
				Kind:       "constructor",
				Type:       declaration.Name,
				Call:       declaration.Name + ".new",
				Signature:  objectConstructorSignature(declaration),
				Target:     line.Name,
				Arguments:  args,
				Supported:  true,
				Executable: false,
				Reason:     "parse-only; runtime object execution is not enabled",
			})
			continue
		}
		objectType, ok := objectTypes[strings.ToLower(receiver)]
		if !ok {
			continue
		}
		declarations := methodDeclarations[semanticMethodKey(objectType, method)]
		declaration, ok := resolveSemanticMethodDeclaration(declarations, len(args))
		if !ok {
			continue
		}
		operations = append(operations, SemanticObjectOperation{
			Line:       line.Line,
			Kind:       "method",
			Type:       objectType,
			Method:     declaration.Name,
			Call:       receiver + "." + declaration.Name,
			Signature:  objectMethodSignature(declaration),
			Target:     receiver,
			Arguments:  args,
			Supported:  true,
			Executable: false,
			Reason:     "parse-only; runtime object execution is not enabled",
		})
	}
	return operations
}

func assignedObjectConstructor(operations []SemanticObjectOperation, name string) (SemanticObjectOperation, bool) {
	for _, operation := range operations {
		if operation.Kind == "constructor" && operation.Target == name && operation.Type != "" {
			return operation, true
		}
	}
	return SemanticObjectOperation{}, false
}

func semanticMethodKey(receiverType string, method string) string {
	return normalizeSemanticType(receiverType) + "." + strings.ToLower(strings.TrimSpace(method))
}

func semanticMethodSignatureKey(declaration SemanticDeclaration) string {
	types := make([]string, 0, len(declaration.Parameters))
	for _, parameter := range declaration.Parameters {
		parameterType := normalizeSemanticType(parameter.Type)
		if parameterType == "" {
			parameterType = "?"
		}
		types = append(types, parameterType)
	}
	receiverType := ""
	if declaration.Receiver != nil {
		receiverType = declaration.Receiver.Type
	}
	return semanticMethodKey(receiverType, declaration.Name) + "(" + strings.Join(types, ",") + ")"
}

func normalizeSemanticType(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), ""))
}

func resolveSemanticMethodDeclaration(declarations []SemanticDeclaration, argCount int) (SemanticDeclaration, bool) {
	for _, declaration := range declarations {
		parameters := declaration.Parameters
		if len(parameters) > 0 {
			parameters = parameters[1:]
		}
		if argCount >= requiredSemanticParameterCount(parameters) && argCount <= len(parameters) {
			return declaration, true
		}
	}
	if len(declarations) > 0 {
		return declarations[0], true
	}
	return SemanticDeclaration{}, false
}

func objectConstructorSignature(declaration SemanticDeclaration) string {
	fields := make([]string, 0, len(declaration.Fields))
	for _, field := range declaration.Fields {
		fields = append(fields, semanticParameterSignature(field))
	}
	return declaration.Name + ".new(" + strings.Join(fields, ", ") + ")"
}

func objectMethodSignature(declaration SemanticDeclaration) string {
	parameters := make([]string, 0, len(declaration.Parameters))
	for _, parameter := range declaration.Parameters {
		parameters = append(parameters, semanticParameterSignature(parameter))
	}
	return declaration.Name + "(" + strings.Join(parameters, ", ") + ")"
}

func semanticParameterSignature(parameter SemanticParameter) string {
	parts := make([]string, 0, 3)
	if strings.TrimSpace(parameter.Type) != "" {
		parts = append(parts, strings.TrimSpace(parameter.Type))
	}
	if strings.TrimSpace(parameter.Name) != "" {
		parts = append(parts, strings.TrimSpace(parameter.Name))
	}
	signature := strings.Join(parts, " ")
	if strings.TrimSpace(parameter.Default) != "" {
		if signature != "" {
			signature += " = "
		}
		signature += strings.TrimSpace(parameter.Default)
	}
	return signature
}

func objectOperationDiagnostics(line ASTLine, operations []SemanticObjectOperation, typeDeclarations map[string]SemanticDeclaration, methodDeclarations map[string][]SemanticDeclaration) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for _, operation := range operations {
		minArgs, maxArgs, ok := objectOperationArgBounds(operation, typeDeclarations, methodDeclarations)
		if !ok {
			continue
		}
		argCount := len(operation.Arguments)
		if argCount < minArgs || argCount > maxArgs {
			diagnostics = append(diagnostics, semanticDiagnostic(line, "PINE_SEMANTIC_OBJECT_SIGNATURE", fmt.Sprintf("%s expects %s", operation.Call, operation.Signature)))
		}
	}
	return diagnostics
}

func objectOperationArgBounds(operation SemanticObjectOperation, typeDeclarations map[string]SemanticDeclaration, methodDeclarations map[string][]SemanticDeclaration) (int, int, bool) {
	switch operation.Kind {
	case "constructor":
		declaration, ok := typeDeclarations[strings.ToLower(operation.Type)]
		if !ok {
			return 0, 0, false
		}
		return requiredSemanticParameterCount(declaration.Fields), len(declaration.Fields), true
	case "method":
		declarations := methodDeclarations[semanticMethodKey(operation.Type, operation.Method)]
		declaration, ok := semanticMethodDeclarationForOperation(declarations, operation)
		if !ok {
			return 0, 0, false
		}
		parameters := declaration.Parameters
		if len(parameters) > 0 {
			parameters = parameters[1:]
		}
		return requiredSemanticParameterCount(parameters), len(parameters), true
	default:
		return 0, 0, false
	}
}

func semanticMethodDeclarationForOperation(declarations []SemanticDeclaration, operation SemanticObjectOperation) (SemanticDeclaration, bool) {
	for _, declaration := range declarations {
		if objectMethodSignature(declaration) == operation.Signature {
			return declaration, true
		}
	}
	return resolveSemanticMethodDeclaration(declarations, len(operation.Arguments))
}

func requiredSemanticParameterCount(parameters []SemanticParameter) int {
	count := 0
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter.Default) == "" {
			count++
		}
	}
	return count
}

func collectionOperationDiagnostics(line ASTLine, operations []SemanticCollectionOperation) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for _, operation := range operations {
		signature, ok := collectionOperationSignatures[operation.Call]
		if !ok {
			continue
		}
		argCount := len(operation.Arguments)
		if argCount < signature.minArgs || argCount > signature.maxArgs {
			diagnostics = append(diagnostics, semanticDiagnostic(line, "PINE_SEMANTIC_COLLECTION_SIGNATURE", fmt.Sprintf("%s expects %s", operation.Call, signature.signature)))
		}
		if strings.HasPrefix(operation.Operation, "new") {
			diagnostics = append(diagnostics, collectionTypeArgumentDiagnostics(line, operation.Call, operation.Namespace, operation.TypeArgs)...)
		}
	}
	return diagnostics
}

func collectionTypeArgumentDiagnostics(line ASTLine, source string, namespace string, typeArgs string) []Diagnostic {
	if strings.TrimSpace(typeArgs) == "" {
		return nil
	}
	expected, ok := collectionTypeArgumentCount(namespace)
	if !ok {
		return nil
	}
	actual := len(collectionTypeArguments(typeArgs))
	if actual == expected {
		return nil
	}
	return []Diagnostic{semanticDiagnostic(
		line,
		"PINE_SEMANTIC_COLLECTION_TYPE",
		fmt.Sprintf("%s requires %d type argument(s), got %d", source, expected, actual),
	)}
}

func collectionTypeArgumentCount(namespace string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(namespace)) {
	case "array", "matrix":
		return 1, true
	case "map":
		return 2, true
	default:
		return 0, false
	}
}

func collectionTypeArguments(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := make([]string, 0, 2)
	start := 0
	angleDepth := 0
	otherDepth := 0
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '(', '[':
			otherDepth++
		case ')', ']':
			if otherDepth > 0 {
				otherDepth--
			}
		case ',':
			if angleDepth == 0 && otherDepth == 0 {
				parts = append(parts, strings.TrimSpace(value[start:index]))
				start = index + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(value[start:]))
	return parts
}

func collectionConstructorTypeArguments(operation SemanticCollectionOperation) []string {
	if strings.TrimSpace(operation.TypeArgs) != "" {
		return collectionTypeArguments(operation.TypeArgs)
	}
	if operation.Namespace != "array" || !strings.HasPrefix(operation.Operation, "new_") {
		return nil
	}
	elementType := strings.TrimPrefix(operation.Operation, "new_")
	switch elementType {
	case "bool", "float", "int", "string":
		return []string{elementType}
	default:
		return nil
	}
}

func equalCollectionTypeArguments(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if normalizeCollectionTypeArgument(left[index]) != normalizeCollectionTypeArgument(right[index]) {
			return false
		}
	}
	return true
}

func collectionTypeArgumentsHaveExpectedArity(namespace string, args []string) bool {
	if len(args) == 0 {
		return false
	}
	expected, ok := collectionTypeArgumentCount(namespace)
	return ok && len(args) == expected
}

func normalizeCollectionTypeArgument(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), ""))
}

func collectionOperationSignatureText(call string) string {
	if signature, ok := collectionOperationSignatures[call]; ok {
		return signature.signature
	}
	return ""
}

func collectionOperationSupported(call string) bool {
	_, ok := collectionOperationSignatures[call]
	return ok
}

func collectionOperationExecutable(namespace string, operation string) bool {
	return executableCollectionOperations[namespace][operation]
}

func collectionOperationReason(namespace string, operation string) string {
	if collectionOperationExecutable(namespace, operation) {
		return ""
	}
	return "parse-only; runtime collection execution is not enabled"
}

func assignedCollectionConstructor(operations []SemanticCollectionOperation, name string) (SemanticCollectionOperation, bool) {
	for _, operation := range operations {
		if strings.HasPrefix(operation.Operation, "new") && operation.Target == name && operation.Namespace != "" {
			return operation, true
		}
	}
	return SemanticCollectionOperation{}, false
}

func collectionCallInfo(text string) (string, string, string) {
	match := collectionCallPattern.FindStringSubmatch(text)
	if match == nil {
		return "", "", ""
	}
	namespace := strings.ToLower(match[1])
	method := strings.ToLower(match[2])
	typeArgs := strings.Trim(match[3], "<>")
	return namespace, namespace + "." + method, strings.TrimSpace(typeArgs)
}

func inferCollectionValueKind(text string) SemanticValueKind {
	for _, operation := range semanticCollectionOperations(ASTLine{Text: text}, nil) {
		if strings.HasPrefix(operation.Operation, "new") {
			return SemanticValueObject
		}
	}
	return SemanticValueUnknown
}

func collectionOperationTarget(operation string, args []string, assignedName string) string {
	if strings.HasPrefix(operation, "new") {
		return assignedName
	}
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func collectionOperationMutates(operation string) bool {
	switch operation {
	case "push", "pop", "shift", "unshift", "insert", "remove", "clear", "set", "fill", "put":
		return true
	default:
		return strings.HasPrefix(operation, "new")
	}
}
