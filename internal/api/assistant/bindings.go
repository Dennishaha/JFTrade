package assistant

import "github.com/jftrade/jftrade-main/internal/api/httpserver"

type taskURI struct {
	TaskID string `uri:"taskId" binding:"required"`
}

type memoryURI struct {
	MemoryID string `uri:"memoryId" binding:"required"`
}

type providerURI struct {
	ProviderID string `uri:"providerId" binding:"required"`
}

type agentURI struct {
	AgentID string `uri:"agentId" binding:"required"`
}

type skillURI struct {
	SkillID string `uri:"skillId" binding:"required"`
}

type sessionURI struct {
	SessionID string `uri:"sessionId" binding:"required"`
}

type runURI struct {
	RunID string `uri:"runId" binding:"required"`
}

type approvalURI struct {
	ApprovalID string `uri:"approvalId" binding:"required"`
}

type adkPageQuery struct {
	Limit  httpserver.OptionalIntValue `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset httpserver.OptionalIntValue `form:"offset,parser=encoding.TextUnmarshaler"`
}

type adkSessionsQuery struct {
	Limit   httpserver.OptionalIntValue `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset  httpserver.OptionalIntValue `form:"offset,parser=encoding.TextUnmarshaler"`
	AgentID string                      `form:"agentId"`
	Query   string                      `form:"query"`
}

type adkRunsQuery struct {
	Limit     httpserver.OptionalIntValue `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset    httpserver.OptionalIntValue `form:"offset,parser=encoding.TextUnmarshaler"`
	Status    string                      `form:"status"`
	AgentID   string                      `form:"agentId"`
	SessionID string                      `form:"sessionId"`
}

type adkApprovalsQuery struct {
	Limit   httpserver.OptionalIntValue `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset  httpserver.OptionalIntValue `form:"offset,parser=encoding.TextUnmarshaler"`
	Status  string                      `form:"status"`
	AgentID string                      `form:"agentId"`
}

type adkAgentsQuery struct {
	Status string `form:"status"`
}

type adkAuditQuery struct {
	Kind      string `form:"kind"`
	SubjectID string `form:"subjectId"`
}

type adkTasksQuery struct {
	Limit   httpserver.OptionalIntValue `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset  httpserver.OptionalIntValue `form:"offset,parser=encoding.TextUnmarshaler"`
	Status  string                      `form:"status"`
	AgentID string                      `form:"agentId"`
	RunID   string                      `form:"runId"`
}

type adkMemoryQuery struct {
	Scope   string `form:"scope"`
	AgentID string `form:"agentId"`
	Key     string `form:"key"`
}
