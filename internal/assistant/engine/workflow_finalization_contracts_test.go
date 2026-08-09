package adk

import (
	"testing"
)

func TestRegisterWorkflowExecutionTracksParentAndChildRuns(t *testing.T) {
	runtime := newTestRuntime(t)
	execution := newBareGoogleADKExecution("root-run")
	execution.onDelta = func(ChatDelta) error { return nil }

	runtime.RegisterWorkflowExecution("parent", []string{"child-a", "child-b"}, execution)
	if execution.onDelta != nil ||
		runtime.adkRuns["parent"] != execution ||
		runtime.adkRuns["child-a"] != execution ||
		runtime.adkRuns["child-b"] != execution {
		t.Fatalf(
			"registered workflow execution = root:%p childA:%p childB:%p",
			runtime.adkRuns["parent"], runtime.adkRuns["child-a"], runtime.adkRuns["child-b"],
		)
	}
}
