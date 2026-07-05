package pine

import "strings"

func semanticVisualMetadata(line ASTLine) []PineVisualMetadata {
	calls := semanticVisualCalls(line.Text)
	metadata := make([]PineVisualMetadata, 0, len(calls))
	for _, call := range calls {
		namedArgs := visualNamedArgs(call.args)
		metadata = append(metadata, PineVisualMetadata{
			Line:      line.Line,
			Kind:      visualMetadataKind(call.name),
			Call:      call.name,
			Variable:  visualMetadataVariable(line, call.name),
			Target:    visualMetadataTarget(call.name, call.args),
			Title:     visualMetadataTitle(call.name, call.args, namedArgs),
			Arguments: call.args,
			NamedArgs: namedArgs,
			Text:      line.Text,
		})
	}
	return metadata
}

type semanticVisualCall struct {
	name string
	args []string
}

func semanticVisualCalls(text string) []semanticVisualCall {
	calls := make([]semanticVisualCall, 0)
	index := 0
	for index < len(text) {
		open := strings.Index(text[index:], "(")
		if open < 0 {
			break
		}
		open += index
		nameStart := open
		for nameStart > 0 {
			ch := text[nameStart-1]
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '.' {
				nameStart--
				continue
			}
			break
		}
		name := strings.ToLower(strings.TrimSpace(text[nameStart:open]))
		close := matchingParen(text, open)
		if close < 0 {
			break
		}
		if isVisualCallName(name) {
			calls = append(calls, semanticVisualCall{name: name, args: splitArguments(text[open+1 : close])})
		}
		index = close + 1
	}
	return calls
}

func isVisualCallName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch lower {
	case "plot", "plotshape", "plotchar", "hline", "bgcolor", "barcolor", "fill", "alertcondition":
		return true
	default:
		return isDrawingVisualCallName(lower) ||
			strings.HasPrefix(lower, "table.")
	}
}

func isDrawingVisualCallName(name string) bool {
	for _, namespace := range []string{"label", "line", "box"} {
		prefix := namespace + "."
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		operation := strings.TrimPrefix(name, prefix)
		return operation == "new" ||
			operation == "delete" ||
			operation == "copy" ||
			strings.HasPrefix(operation, "set_") ||
			strings.HasPrefix(operation, "get_")
	}
	return false
}

func visualMetadataVariable(line ASTLine, call string) string {
	if line.Name == "" || !strings.Contains(strings.ToLower(line.Expression), strings.ToLower(call)+"(") {
		return ""
	}
	return line.Name
}

func visualNamedArgs(args []string) map[string]string {
	named := map[string]string{}
	for _, arg := range args {
		key, value, ok := splitNamedArg(arg)
		if !ok {
			continue
		}
		named[strings.ToLower(key)] = value
	}
	if len(named) == 0 {
		return nil
	}
	return named
}

func visualMetadataKind(call string) string {
	lower := strings.ToLower(strings.TrimSpace(call))
	switch {
	case lower == "alertcondition":
		return "alert"
	case strings.HasPrefix(lower, "label."), strings.HasPrefix(lower, "line."), strings.HasPrefix(lower, "box."):
		return "drawing"
	case strings.HasPrefix(lower, "table."):
		return "table"
	case lower == "bgcolor" || lower == "barcolor" || lower == "fill":
		return "color"
	default:
		return "plot"
	}
}

func visualMetadataTarget(call string, args []string) string {
	lower := strings.ToLower(strings.TrimSpace(call))
	switch {
	case len(args) == 0:
		return ""
	case lower == "label.new" && len(args) >= 3:
		return args[2]
	default:
		if _, value, ok := splitNamedArg(args[0]); ok {
			return value
		}
		return args[0]
	}
}

func visualMetadataTitle(call string, args []string, namedArgs map[string]string) string {
	for _, key := range []string{"title", "text", "message"} {
		if value := namedArgs[key]; value != "" {
			return unquote(value)
		}
	}
	lower := strings.ToLower(strings.TrimSpace(call))
	switch {
	case lower == "alertcondition" && len(args) >= 2:
		return unquote(args[1])
	case lower == "plot" && len(args) >= 2:
		return unquote(args[1])
	case lower == "hline" && len(args) >= 2:
		return unquote(args[1])
	case lower == "label.new" && len(args) >= 3:
		return unquote(args[2])
	default:
		return ""
	}
}
