package persistence

import (
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestApplyTaskPatchAcceptsNilTask(t *testing.T) {
	if err := applyTaskPatch(nil, "task", jfadkmodel.TaskPatchRequest{}); err != nil {
		t.Fatalf("applyTaskPatch(nil): %v", err)
	}
}
