package assistant

type Page[T any] struct {
	Items  []T
	Total  int
	Limit  int
	Offset int
}

type TaskQuery struct {
	Status  string
	AgentID string
	RunID   string
	Limit   int
	Offset  int
}

type MemoryQuery struct {
	Scope   string
	AgentID string
	Key     string
}

type AgentQuery struct {
	Status string
}

type SessionQuery struct {
	AgentID string
	Query   string
	Limit   int
	Offset  int
}

type CreateSessionRequest struct {
	AgentID string
	Title   string
}

type RunQuery struct {
	Status    string
	AgentID   string
	SessionID string
	Limit     int
	Offset    int
}

type ApprovalQuery struct {
	Status  string
	AgentID string
	Limit   int
	Offset  int
}

type AuditQuery struct {
	Kind      string
	SubjectID string
}

type OptimizationTasks struct {
	Tasks []map[string]any `json:"tasks"`
}
