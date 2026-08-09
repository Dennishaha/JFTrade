package adk

import (
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

// WorkflowExecutionHandle is the opaque execution handle passed between
// engine-root Runtime services and the workflow executor.
type WorkflowExecutionHandle = jfadkmodel.WorkflowExecutionHandle

// WorkflowExecutorRuntime is the engine-root service surface the workflow
// executor depends on; Runtime implements it.
type WorkflowExecutorRuntime = jfadkmodel.WorkflowExecutorRuntime

var _ WorkflowExecutorRuntime = (*Runtime)(nil)
var _ WorkflowExecutionHandle = (*googleADKExecution)(nil)
