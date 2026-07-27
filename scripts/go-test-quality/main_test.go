package main

import (
	"strings"
	"testing"
)

func TestAnalyzeSourcesRejectsCallsWithoutAssertions(t *testing.T) {
	t.Parallel()
	source := sourceFile{
		path: "sample/publisher_test.go",
		contents: []byte(`package sample
import "testing"
func TestPublishes(t *testing.T) {
	t.Helper()
	publisher.Publish(event)
}
`),
	}

	candidates, err := analyzeSources([]sourceFile{source})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Name != "TestPublishes" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestAnalyzeSourcesRecognizesStandardAndTestifyAssertions(t *testing.T) {
	t.Parallel()
	source := sourceFile{
		path: "sample/assertions_test.go",
		contents: []byte(`package sample
import (
	"testing"
	check "github.com/stretchr/testify/require"
)
func TestStandard(t *testing.T) {
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}
func TestTestify(t *testing.T) {
	check.NoError(t, err)
}
`),
	}

	candidates, err := analyzeSources([]sourceFile{source})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
}

func TestAnalyzeSourcesFollowsAssertionHelpersAcrossFiles(t *testing.T) {
	t.Parallel()
	sources := []sourceFile{
		{
			path: "sample/helper_test.go",
			contents: []byte(`package sample
import "testing"
func requireReady(t testing.TB, ready bool) {
	t.Helper()
	if !ready {
		t.Fatal("not ready")
	}
}
`),
		},
		{
			path: "sample/workflow_test.go",
			contents: []byte(`package sample
import "testing"
func TestWorkflow(t *testing.T) {
	requireReady(t, true)
}
`),
		},
	}

	candidates, err := analyzeSources(sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
}

func TestAnalyzeSourcesRecognizesAssertionsInsideNestedSubtests(t *testing.T) {
	t.Parallel()
	source := sourceFile{
		path: "sample/nested_test.go",
		contents: []byte(`package sample
import "testing"
func TestNested(t *testing.T) {
	t.Run("case", func(nested *testing.T) {
		if got != want {
			nested.Error("mismatch")
		}
	})
}
`),
	}

	candidates, err := analyzeSources([]sourceFile{source})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
}

func TestValidateExemptionsRejectsStaleEntries(t *testing.T) {
	t.Parallel()
	entry := exemption{
		Path:   "sample/effect_test.go",
		Test:   "TestEffect",
		Reason: "The process exit is the asserted effect.",
	}
	err := validateExemptions(
		map[string]exemption{candidateID(entry.Path, entry.Test): entry},
		map[string]struct{}{},
	)
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("error = %v", err)
	}
}
