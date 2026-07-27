//nolint:unused // Annotation-only stubs are consumed by swag during contract generation.
package assistant

// documentADKSessions godoc
// @Summary 分页读取 ADK Session
// @Tags adk
// @Produce json
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Param agentId query string false "Agent ID"
// @Param query query string false "标题搜索关键字"
// @Success 200 {object} httpserver.Envelope{data=ADKSessionsData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/sessions [get]
func documentADKSessions() {}

// documentADKCreateSession godoc
// @Summary 创建 ADK Session
// @Tags adk
// @Accept json
// @Produce json
// @Param request body ADKCreateSessionRequest true "Session"
// @Success 200 {object} httpserver.Envelope{data=adk.Session}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/sessions [post]
func documentADKCreateSession() {}

// documentADKSession godoc
// @Summary 读取 ADK Session 详情
// @Tags adk
// @Produce json
// @Param sessionId path string true "Session ID"
// @Success 200 {object} httpserver.Envelope{data=adk.SessionsResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/sessions/{sessionId} [get]
func documentADKSession() {}

// documentADKRenameSession godoc
// @Summary 重命名 ADK Session
// @Tags adk
// @Accept json
// @Produce json
// @Param sessionId path string true "Session ID"
// @Param request body ADKRenameSessionRequest true "Session 标题"
// @Success 200 {object} httpserver.Envelope{data=adk.Session}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/sessions/{sessionId} [put]
func documentADKRenameSession() {}

// documentADKDeleteSession godoc
// @Summary 删除 ADK Session
// @Tags adk
// @Produce json
// @Param sessionId path string true "Session ID"
// @Success 200 {object} httpserver.Envelope{data=ADKDeletedIDData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/sessions/{sessionId} [delete]
func documentADKDeleteSession() {}

// documentADKSessionContext godoc
// @Summary 读取 ADK Session 上下文
// @Tags adk
// @Produce json
// @Param sessionId path string true "Session ID"
// @Success 200 {object} httpserver.Envelope{data=adk.SessionContextSnapshot}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/sessions/{sessionId}/context [get]
func documentADKSessionContext() {}

// documentADKCompactSessionContext godoc
// @Summary 压缩 ADK Session 上下文
// @Tags adk
// @Accept json
// @Produce json
// @Param sessionId path string true "Session ID"
// @Param request body ADKCompactContextRequest true "压缩参数"
// @Success 200 {object} httpserver.Envelope{data=adk.SessionContextSnapshot}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/sessions/{sessionId}/context/compact [post]
func documentADKCompactSessionContext() {}

// documentADKUpdateSessionComposerState godoc
// @Summary 更新 ADK Session 编辑器状态
// @Tags adk
// @Accept json
// @Produce json
// @Param sessionId path string true "Session ID"
// @Param request body ADKSessionComposerStatePatch true "编辑器状态变更"
// @Success 200 {object} httpserver.Envelope{data=adk.SessionComposerState}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/sessions/{sessionId}/composer-state [patch]
func documentADKUpdateSessionComposerState() {}

// documentADKChat godoc
// @Summary 执行 ADK 对话
// @Tags adk
// @Accept json
// @Produce json
// @Param request body ADKChatRequest true "对话请求"
// @Success 200 {object} httpserver.Envelope{data=adk.ChatResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/chat [post]
func documentADKChat() {}

// documentADKRuns godoc
// @Summary 分页读取 ADK Run
// @Tags adk
// @Produce json
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Param status query string false "Run 状态"
// @Param agentId query string false "Agent ID"
// @Param sessionId query string false "Session ID"
// @Success 200 {object} httpserver.Envelope{data=ADKRunsData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/runs [get]
func documentADKRuns() {}

// documentADKRun godoc
// @Summary 读取 ADK Run
// @Tags adk
// @Produce json
// @Param runId path string true "Run ID"
// @Success 200 {object} httpserver.Envelope{data=adk.Run}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/runs/{runId} [get]
func documentADKRun() {}

// documentADKCancelRun godoc
// @Summary 取消 ADK Run
// @Tags adk
// @Produce json
// @Param runId path string true "Run ID"
// @Success 200 {object} httpserver.Envelope{data=adk.Run}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/runs/{runId}/cancel [post]
func documentADKCancelRun() {}

// documentADKPauseRun godoc
// @Summary 暂停 ADK Goal Run
// @Tags adk
// @Produce json
// @Param runId path string true "Run ID"
// @Success 200 {object} httpserver.Envelope{data=adk.Run}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/runs/{runId}/pause [post]
func documentADKPauseRun() {}

// documentADKResumeRun godoc
// @Summary 恢复 ADK Goal Run
// @Tags adk
// @Produce json
// @Param runId path string true "Run ID"
// @Success 200 {object} httpserver.Envelope{data=adk.Run}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/runs/{runId}/resume [post]
func documentADKResumeRun() {}

// documentADKUpdateRunObjective godoc
// @Summary 更新 ADK Goal Run 目标
// @Tags adk
// @Accept json
// @Produce json
// @Param runId path string true "Run ID"
// @Param request body ADKUpdateRunObjectiveRequest true "新目标"
// @Success 200 {object} httpserver.Envelope{data=adk.Run}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/runs/{runId}/objective [patch]
func documentADKUpdateRunObjective() {}

// documentADKInputResponse godoc
// @Summary 回答 ADK Run 输入请求
// @Tags adk
// @Accept json
// @Produce json
// @Param runId path string true "Run ID"
// @Param request body ADKInputResponseRequest true "输入回答"
// @Success 200 {object} httpserver.Envelope{data=adk.InputResolution}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/runs/{runId}/input-response [post]
func documentADKInputResponse() {}

// documentADKApprovals godoc
// @Summary 分页读取 ADK 审批
// @Tags adk
// @Produce json
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Param status query string false "审批状态"
// @Param agentId query string false "Agent ID"
// @Success 200 {object} httpserver.Envelope{data=ADKApprovalsData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/approvals [get]
func documentADKApprovals() {}

// documentADKApprove godoc
// @Summary 批准 ADK 工具调用
// @Tags adk
// @Produce json
// @Param approvalId path string true "Approval ID"
// @Success 200 {object} httpserver.Envelope{data=adk.ApprovalResolution}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/approvals/{approvalId}/approve [post]
func documentADKApprove() {}

// documentADKDeny godoc
// @Summary 拒绝 ADK 工具调用
// @Tags adk
// @Produce json
// @Param approvalId path string true "Approval ID"
// @Success 200 {object} httpserver.Envelope{data=adk.ApprovalResolution}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/approvals/{approvalId}/deny [post]
func documentADKDeny() {}
