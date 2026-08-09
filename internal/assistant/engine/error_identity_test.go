package adk

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	adktool "google.golang.org/adk/v2/tool"
	adkworkflow "google.golang.org/adk/v2/workflow"
)

func TestSerializedADKErrorRestoresKnownSentinelIdentity(t *testing.T) {
	for _, sentinel := range []error{
		adktool.ErrConfirmationRequired,
		adktool.ErrConfirmationRejected,
		adkworkflow.ErrNodeInterrupted,
		jfadkmodel.ErrUserGoalPauseRequested,
	} {
		t.Run(sentinel.Error(), func(t *testing.T) {
			text := "serialized tool failure: " + sentinel.Error()
			err := ErrorFromSerializedADKText(text)
			if !errors.Is(err, sentinel) {
				t.Fatalf("ErrorFromSerializedADKText(%q) = %v, want errors.Is(_, %v)", text, err, sentinel)
			}
			if err.Error() != text {
				t.Fatalf("classified error text = %q, want %q", err.Error(), text)
			}
		})
	}

	ordinary := errors.New("ordinary tool failure")
	if got := errorFromSerializedADKValue(ordinary); !errors.Is(got, ordinary) {
		t.Fatalf("ordinary error identity changed: got %v, want %v", got, ordinary)
	}
	wrapped := fmt.Errorf("tool execution: %w", adktool.ErrConfirmationRequired)
	if got := errorFromSerializedADKValue(wrapped); !errors.Is(got, wrapped) || !errors.Is(got, adktool.ErrConfirmationRequired) {
		t.Fatalf("wrapped sentinel changed: got %v", got)
	}
}

func TestGoogleADKRunnerErrorClassificationPreservesCause(t *testing.T) {
	upstream := errors.New("no function call event found for function responses ids: [approval-1]")
	classified := classifyGoogleADKRunnerError(upstream)
	if !errors.Is(classified, errGoogleADKFunctionCallEventMissing) {
		t.Fatalf("classified error = %v, want replay sentinel", classified)
	}
	if !errors.Is(classified, upstream) {
		t.Fatalf("classified error = %v, want original upstream cause", classified)
	}
	if classified.Error() != upstream.Error() {
		t.Fatalf("classified error text = %q, want %q", classified.Error(), upstream.Error())
	}

	ordinary := errors.New("different GO-ADK validation failure")
	if got := classifyGoogleADKRunnerError(ordinary); !errors.Is(got, ordinary) {
		t.Fatalf("ordinary runner error identity changed: got %v, want %v", got, ordinary)
	}
}

func TestADKProductionCodeUsesSentinelIdentityChecks(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}
	violations := make([]string, 0)
	files := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(files, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !isStringsContainsCall(call.Fun) || !callMatchesSentinelErrorText(call) {
				return true
			}
			position := files.Position(call.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d", filepath.ToSlash(name), position.Line))
			return true
		})
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("sentinel errors must use errors.Is, not strings.Contains(...Error()): %s", strings.Join(violations, ", "))
	}
}

func isStringsContainsCall(function ast.Expr) bool {
	selector, ok := function.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Contains" {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && identifier.Name == "strings"
}

func callMatchesSentinelErrorText(call *ast.CallExpr) bool {
	found := false
	for _, argument := range call.Args {
		ast.Inspect(argument, func(node ast.Node) bool {
			inner, ok := node.(*ast.CallExpr)
			if ok && isSentinelErrorMethod(inner.Fun) {
				found = true
				return false
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func isSentinelErrorMethod(function ast.Expr) bool {
	method, ok := function.(*ast.SelectorExpr)
	if !ok || method.Sel.Name != "Error" {
		return false
	}
	switch receiver := method.X.(type) {
	case *ast.Ident:
		return receiver.Name != "err" &&
			(strings.HasPrefix(receiver.Name, "Err") || strings.HasPrefix(receiver.Name, "err"))
	case *ast.SelectorExpr:
		return strings.HasPrefix(receiver.Sel.Name, "Err")
	default:
		return false
	}
}
