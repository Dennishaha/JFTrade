package persistence

import (
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"testing"
)

func TestApplyTaskPatchAcceptsNilTask(t *testing.T) {
	if err := applyTaskPatch(nil, "task", assistantmodel.TaskPatchRequest{}); err != nil {
		t.Fatalf("applyTaskPatch(nil): %v", err)
	}
}
