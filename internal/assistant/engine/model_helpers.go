package adk

import (
	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

var (
	ErrBuiltinAgentProtected       = jfadkmodel.ErrBuiltinAgentProtected
	ErrCleanupCandidatesChanged    = jfadkmodel.ErrCleanupCandidatesChanged
	ErrInvalidTaskStatus           = jfadkmodel.ErrInvalidTaskStatus
	ErrProviderInUse               = jfadkmodel.ErrProviderInUse
	errInputRequestNotFound        = jfadkmodel.ErrInputRequestNotFound
	errInputRequestInvalid         = jfadkmodel.ErrInputRequestInvalid
	errInputRequestConflict        = jfadkmodel.ErrInputRequestConflict
	errInputRequestAlreadyAnswered = jfadkmodel.ErrInputRequestAlreadyAnswered
)

func nowString() string {
	return jfadkmodel.NowString()
}

func newContextRevisionID() string {
	return jfadkmodel.NewContextRevisionID()
}

func ensureSessionContextRevision(state jfadkmodel.SessionContextState, sessionID string) jfadkmodel.SessionContextState {
	return enginepersistence.EnsureSessionContextRevision(state, sessionID)
}

func normalizeID(value string) string {
	return jfadkmodel.NormalizeID(value)
}

func defaultString(value string, defaultValue string) string {
	return jfadkmodel.DefaultString(value, defaultValue)
}

func normalizeContextWindowTokens(value int) int {
	return jfadkmodel.NormalizeContextWindowTokens(value)
}

func normalizeRecentUserWindow(value int) int {
	return jfadkmodel.NormalizeRecentUserWindow(value)
}

func normalizeProviderRequestTimeoutMs(value int) int {
	return jfadkmodel.NormalizeProviderRequestTimeoutMs(value)
}

func normalizeHeaders(headers map[string]string) map[string]string {
	return jfadkmodel.NormalizeHeaders(headers)
}

func normalizePermissionMode(value string) string {
	return jfadkmodel.NormalizePermissionMode(value)
}

func validPermissionMode(value string) bool {
	return jfadkmodel.ValidPermissionMode(value)
}

func normalizeAgentDefaultWorkMode(value string) string {
	return jfadkmodel.NormalizeAgentDefaultWorkMode(value)
}

func defaultAgentInstruction() string {
	return jfadkmodel.DefaultAgentInstruction()
}

func runHasPendingApproval(approvals []Approval) bool {
	return jfadkmodel.RunHasPendingApproval(approvals)
}

func finishToolCall(call *ToolCall) {
	jfadkmodel.FinishToolCall(call)
}

func validateProviderBaseURL(rawURL string) error {
	return enginepersistence.ValidateProviderBaseURL(rawURL)
}

func currentErrOrNotFound(err error, ok bool) error {
	return enginepersistence.CurrentErrOrNotFound(err, ok)
}

func validateInputAnswers(request jfadkmodel.InputRequest, submitted []jfadkmodel.InputAnswer) ([]jfadkmodel.InputAnswer, error) {
	return jfadkmodel.ValidateInputAnswers(request, submitted)
}
