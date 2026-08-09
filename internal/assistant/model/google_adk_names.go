package model

import (
	"fmt"
	"strings"
)

// GoogleADKWorkflowRootName derives the stable ADK agent name for a workflow
// parent run.
func GoogleADKWorkflowRootName(parentRunID string) string {
	name := "workflow_" + strings.ReplaceAll(NormalizeID(parentRunID), "-", "_")
	if name == "workflow_" {
		return "workflow_root"
	}
	return name
}

// GoogleADKWorkflowChildName derives the stable ADK agent name for a workflow
// child node at the given plan index.
func GoogleADKWorkflowChildName(parentRunID string, index int) string {
	return fmt.Sprintf("%s_child_%d", GoogleADKWorkflowRootName(parentRunID), index+1)
}
