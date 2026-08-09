package model

// WorkflowRequest is the executor-facing input contract for a workflow run.
// It is shared by the engine-root composition seam and the workflow runtime.
type WorkflowRequest struct {
	Agent              Agent
	Session            Session
	Message            string
	Mode               string
	Objective          string
	RunOptions         RunOptions
	OnDelta            func(ChatDelta) error
	EmitRun            bool
	ClientRequestID    string
	RequestFingerprint string

	GoalDecision *WorkflowGoalDecision
}

// ReusedChatRequestError reports that a chat request was already claimed by an
// existing run, so the caller can return that run instead of starting a new one.
type ReusedChatRequestError struct {
	Run Run
}

func (e *ReusedChatRequestError) Error() string {
	return "chat request already belongs to run " + e.Run.ID
}
