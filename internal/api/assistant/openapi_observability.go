//nolint:unused // Annotation-only stubs are consumed by swag during contract generation.
package assistant

// documentADKAudit godoc
// @Summary 分页读取 ADK 审计事件
// @Tags adk
// @Produce json
// @Param kind query string false "事件类型"
// @Param subjectId query string false "Subject ID"
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Success 200 {object} httpserver.Envelope{data=ADKAuditData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/audit [get]
func documentADKAudit() {}

// documentADKMetrics godoc
// @Summary 读取 ADK 运行指标
// @Tags adk
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=ADKMetricsData}
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/metrics [get]
func documentADKMetrics() {}

// documentADKOptimizationTasks godoc
// @Summary 分页读取 ADK 优化任务
// @Tags adk
// @Produce json
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Success 200 {object} httpserver.Envelope{data=ADKOptimizationTasksData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/optimization-tasks [get]
func documentADKOptimizationTasks() {}

// documentADKOptimizationTask godoc
// @Summary 读取 ADK 优化任务
// @Tags adk
// @Produce json
// @Param taskId path string true "Task ID"
// @Success 200 {object} httpserver.Envelope{data=ADKOptimizationTaskData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/optimization-tasks/{taskId} [get]
func documentADKOptimizationTask() {}

// documentADKCancelOptimizationTask godoc
// @Summary 取消 ADK 优化任务
// @Tags adk
// @Produce json
// @Param taskId path string true "Task ID"
// @Success 200 {object} httpserver.Envelope{data=ADKOptimizationTaskData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/optimization-tasks/{taskId}/cancel [post]
func documentADKCancelOptimizationTask() {}
