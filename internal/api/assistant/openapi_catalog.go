//nolint:unused // Annotation-only stubs are consumed by swag during contract generation.
package assistant

// documentADKSnapshot godoc
// @Summary 读取 ADK 快照
// @Tags adk
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=ADKSnapshotData}
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk [get]
func documentADKSnapshot() {}

// documentADKTools godoc
// @Summary 读取 ADK 工具目录
// @Tags adk
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=ADKToolsData}
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/tools [get]
func documentADKTools() {}

// documentADKAgentTemplates godoc
// @Summary 读取内置 Agent 模板
// @Tags adk
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=ADKAgentTemplatesData}
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/agent-templates [get]
func documentADKAgentTemplates() {}

// documentADKTasks godoc
// @Summary 分页读取 ADK 任务
// @Tags adk
// @Produce json
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Param status query string false "任务状态"
// @Param agentId query string false "Agent ID"
// @Param runId query string false "Run ID"
// @Success 200 {object} httpserver.Envelope{data=ADKTasksData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/tasks [get]
func documentADKTasks() {}

// documentADKCreateTask godoc
// @Summary 创建 ADK 任务
// @Tags adk
// @Accept json
// @Produce json
// @Param request body ADKTaskWriteRequest true "任务"
// @Success 200 {object} httpserver.Envelope{data=adk.Task}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/tasks [post]
func documentADKCreateTask() {}

// documentADKTask godoc
// @Summary 读取 ADK 任务
// @Tags adk
// @Produce json
// @Param taskId path string true "Task ID"
// @Success 200 {object} httpserver.Envelope{data=adk.Task}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/tasks/{taskId} [get]
func documentADKTask() {}

// documentADKUpdateTask godoc
// @Summary 更新 ADK 任务
// @Tags adk
// @Accept json
// @Produce json
// @Param taskId path string true "Task ID"
// @Param request body ADKTaskPatchRequest true "任务变更"
// @Success 200 {object} httpserver.Envelope{data=adk.Task}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/tasks/{taskId} [put]
func documentADKUpdateTask() {}

// documentADKDeleteTask godoc
// @Summary 删除 ADK 任务
// @Tags adk
// @Produce json
// @Param taskId path string true "Task ID"
// @Success 200 {object} httpserver.Envelope{data=ADKDeletedIDData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/tasks/{taskId} [delete]
func documentADKDeleteTask() {}

// documentADKMemory godoc
// @Summary 查询 ADK 记忆
// @Tags adk
// @Produce json
// @Param scope query string false "记忆范围"
// @Param agentId query string false "Agent ID"
// @Param key query string false "记忆键"
// @Success 200 {object} httpserver.Envelope{data=ADKMemoryData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/memory [get]
func documentADKMemory() {}

// documentADKSaveMemory godoc
// @Summary 保存 ADK 记忆
// @Tags adk
// @Accept json
// @Produce json
// @Param request body ADKMemoryWriteRequest true "记忆"
// @Success 200 {object} httpserver.Envelope{data=adk.MemoryEntry}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/memory [post]
func documentADKSaveMemory() {}

// documentADKDeleteMemory godoc
// @Summary 删除 ADK 记忆
// @Tags adk
// @Produce json
// @Param memoryId path string true "Memory ID"
// @Success 200 {object} httpserver.Envelope{data=ADKDeletedIDData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/memory/{memoryId} [delete]
func documentADKDeleteMemory() {}

// documentADKProviders godoc
// @Summary 读取 ADK Provider 列表
// @Tags adk
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=ADKProvidersData}
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/providers [get]
func documentADKProviders() {}

// documentADKCreateProvider godoc
// @Summary 创建 ADK Provider
// @Tags adk
// @Accept json
// @Produce json
// @Param request body ADKProviderWriteRequest true "Provider"
// @Success 200 {object} httpserver.Envelope{data=adk.Provider}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/providers [post]
func documentADKCreateProvider() {}

// documentADKUpdateProvider godoc
// @Summary 更新 ADK Provider
// @Tags adk
// @Accept json
// @Produce json
// @Param providerId path string true "Provider ID"
// @Param request body ADKProviderWriteRequest true "Provider"
// @Success 200 {object} httpserver.Envelope{data=adk.Provider}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/providers/{providerId} [put]
func documentADKUpdateProvider() {}

// documentADKDeleteProvider godoc
// @Summary 删除 ADK Provider
// @Tags adk
// @Produce json
// @Param providerId path string true "Provider ID"
// @Success 200 {object} httpserver.Envelope{data=ADKDeletedIDData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/providers/{providerId} [delete]
func documentADKDeleteProvider() {}

// documentADKSetDefaultProvider godoc
// @Summary 设置默认 ADK Provider
// @Tags adk
// @Produce json
// @Param providerId path string true "Provider ID"
// @Success 200 {object} httpserver.Envelope{data=adk.Provider}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/providers/{providerId}/default [post]
func documentADKSetDefaultProvider() {}

// documentADKTestProvider godoc
// @Summary 测试 ADK Provider 连通性
// @Tags adk
// @Accept json
// @Produce json
// @Param providerId path string true "Provider ID"
// @Param request body ADKProviderTestRequest false "Provider test mode"
// @Success 200 {object} httpserver.Envelope{data=ADKProviderTestData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 502 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/providers/{providerId}/test [post]
func documentADKTestProvider() {}

// documentADKAgents godoc
// @Summary 分页读取 ADK Agent
// @Tags adk
// @Produce json
// @Param status query string false "Agent 状态"
// @Param limit query int false "分页大小"
// @Param offset query int false "分页偏移"
// @Success 200 {object} httpserver.Envelope{data=ADKAgentsData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/agents [get]
func documentADKAgents() {}

// documentADKCreateAgent godoc
// @Summary 创建 ADK Agent
// @Tags adk
// @Accept json
// @Produce json
// @Param request body ADKAgentWriteRequest true "Agent"
// @Success 200 {object} httpserver.Envelope{data=adk.Agent}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/agents [post]
func documentADKCreateAgent() {}

// documentADKUpdateAgent godoc
// @Summary 更新 ADK Agent
// @Tags adk
// @Accept json
// @Produce json
// @Param agentId path string true "Agent ID"
// @Param request body ADKAgentWriteRequest true "Agent"
// @Success 200 {object} httpserver.Envelope{data=adk.Agent}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/agents/{agentId} [put]
func documentADKUpdateAgent() {}

// documentADKDeleteAgent godoc
// @Summary 删除 ADK Agent
// @Tags adk
// @Produce json
// @Param agentId path string true "Agent ID"
// @Success 200 {object} httpserver.Envelope{data=ADKDeletedIDData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/agents/{agentId} [delete]
func documentADKDeleteAgent() {}

// documentADKSkills godoc
// @Summary 读取 ADK Skill 列表
// @Tags adk
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=ADKSkillsData}
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/skills [get]
func documentADKSkills() {}

// documentADKInstallSkill godoc
// @Summary 安装 ADK Skill
// @Tags adk
// @Accept json
// @Produce json
// @Param request body ADKInstallSkillRequest true "Skill URL"
// @Success 200 {object} httpserver.Envelope{data=adk.Skill}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/skills [post]
func documentADKInstallSkill() {}

// documentADKDeleteSkill godoc
// @Summary 删除 ADK Skill
// @Tags adk
// @Produce json
// @Param skillId path string true "Skill ID"
// @Success 200 {object} httpserver.Envelope{data=ADKDeletedIDData}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/adk/skills/{skillId} [delete]
func documentADKDeleteSkill() {}
