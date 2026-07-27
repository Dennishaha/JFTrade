//nolint:unused // Annotation-only stubs are consumed by swag during contract generation.
package assistant

// documentADKWorkflows godoc
// @Summary 分页读取 ADK Workflow
// @Tags adk
// @Produce json
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Param status query string false "Workflow 状态"
// @Success 200 {object} httpserver.Envelope{data=ADKWorkflowsData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflows [get]
func documentADKWorkflows() {}

// documentADKCreateWorkflow godoc
// @Summary 创建 ADK Workflow
// @Tags adk
// @Accept json
// @Produce json
// @Param request body ADKWorkflowDefinitionWriteRequest true "Workflow"
// @Success 200 {object} httpserver.Envelope{data=adk.WorkflowDefinition}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflows [post]
func documentADKCreateWorkflow() {}

// documentADKWorkflow godoc
// @Summary 读取 ADK Workflow
// @Tags adk
// @Produce json
// @Param workflowId path string true "Workflow ID"
// @Success 200 {object} httpserver.Envelope{data=adk.WorkflowDefinition}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflows/{workflowId} [get]
func documentADKWorkflow() {}

// documentADKUpdateWorkflow godoc
// @Summary 更新 ADK Workflow
// @Tags adk
// @Accept json
// @Produce json
// @Param workflowId path string true "Workflow ID"
// @Param request body ADKWorkflowDefinitionWriteRequest true "Workflow"
// @Success 200 {object} httpserver.Envelope{data=adk.WorkflowDefinition}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflows/{workflowId} [put]
func documentADKUpdateWorkflow() {}

// documentADKDeleteWorkflow godoc
// @Summary 删除 ADK Workflow
// @Tags adk
// @Produce json
// @Param workflowId path string true "Workflow ID"
// @Success 200 {object} httpserver.Envelope{data=ADKWorkflowDeleteData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflows/{workflowId} [delete]
func documentADKDeleteWorkflow() {}

// documentADKRunWorkflow godoc
// @Summary 执行 ADK Workflow
// @Tags adk
// @Accept json
// @Produce json
// @Param workflowId path string true "Workflow ID"
// @Param request body ADKWorkflowInputsRequest false "Workflow inputs"
// @Success 200 {object} httpserver.Envelope{data=ADKWorkflowInvocationData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflows/{workflowId}/run [post]
func documentADKRunWorkflow() {}

// documentADKWorkflowTriggers godoc
// @Summary 读取 ADK Workflow Trigger
// @Tags adk
// @Produce json
// @Param workflowId path string true "Workflow ID"
// @Success 200 {object} httpserver.Envelope{data=ADKWorkflowTriggersData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflows/{workflowId}/triggers [get]
func documentADKWorkflowTriggers() {}

// documentADKCreateWorkflowTrigger godoc
// @Summary 创建 ADK Workflow Trigger
// @Tags adk
// @Accept json
// @Produce json
// @Param workflowId path string true "Workflow ID"
// @Param request body ADKWorkflowTriggerWriteRequest true "Trigger"
// @Success 200 {object} httpserver.Envelope{data=ADKWorkflowTriggerSaveData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflows/{workflowId}/triggers [post]
func documentADKCreateWorkflowTrigger() {}

// documentADKUpdateWorkflowTrigger godoc
// @Summary 更新 ADK Workflow Trigger
// @Tags adk
// @Accept json
// @Produce json
// @Param workflowId path string true "Workflow ID"
// @Param triggerId path string true "Trigger ID"
// @Param request body ADKWorkflowTriggerWriteRequest true "Trigger"
// @Success 200 {object} httpserver.Envelope{data=ADKWorkflowTriggerSaveData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflows/{workflowId}/triggers/{triggerId} [put]
func documentADKUpdateWorkflowTrigger() {}

// documentADKDeleteWorkflowTrigger godoc
// @Summary 删除 ADK Workflow Trigger
// @Tags adk
// @Produce json
// @Param workflowId path string true "Workflow ID"
// @Param triggerId path string true "Trigger ID"
// @Success 200 {object} httpserver.Envelope{data=ADKWorkflowTriggerDeleteData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflows/{workflowId}/triggers/{triggerId} [delete]
func documentADKDeleteWorkflowTrigger() {}

// documentADKRunWorkflowTrigger godoc
// @Summary 手动执行 ADK Workflow Trigger
// @Tags adk
// @Accept json
// @Produce json
// @Param triggerId path string true "Trigger ID"
// @Param request body ADKWorkflowInputsRequest false "Workflow inputs"
// @Success 200 {object} httpserver.Envelope{data=ADKWorkflowInvocationData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflow-triggers/{triggerId}/run [post]
func documentADKRunWorkflowTrigger() {}

// documentADKWorkflowTriggerLogs godoc
// @Summary 分页读取 ADK Workflow Trigger 日志
// @Tags adk
// @Produce json
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Param workflowId query string false "Workflow ID"
// @Param triggerId query string false "Trigger ID"
// @Param status query string false "日志状态"
// @Success 200 {object} httpserver.Envelope{data=ADKWorkflowTriggerLogsData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflow-trigger-logs [get]
func documentADKWorkflowTriggerLogs() {}

// documentADKWorkflowWebhook godoc
// @Summary 通过 Webhook 执行 ADK Workflow Trigger
// @Tags adk
// @Accept json
// @Produce json
// @Param triggerId path string true "Trigger ID"
// @Param Authorization header string false "Bearer trigger secret"
// @Param X-JFTrade-Workflow-Secret header string false "Trigger secret"
// @Param request body ADKWorkflowInputsRequest false "Workflow inputs"
// @Success 200 {object} httpserver.Envelope{data=ADKWorkflowInvocationData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/workflow-webhooks/{triggerId} [post]
func documentADKWorkflowWebhook() {}
