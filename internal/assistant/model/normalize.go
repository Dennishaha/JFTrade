package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrBuiltinAgentProtected        = errors.New("builtin agent is protected")
	ErrCleanupCandidatesChanged     = errors.New("cleanup candidates changed")
	ErrInvalidTaskStatus            = errors.New("invalid task status")
	ErrInvalidProviderReasoning     = errors.New("invalid provider reasoning configuration")
	ErrProviderReasoningUnsupported = errors.New("provider does not support reasoning effort")
	ErrProviderInUse                = errors.New("provider is used by agent")
	ErrInputRequestNotFound         = errors.New("input request not found")
	ErrInputRequestInvalid          = errors.New("input response is invalid")
	ErrInputRequestConflict         = errors.New("input request conflict")
	ErrInputRequestAlreadyAnswered  = errors.New("input request already answered")
)

// sortableTimestampLayout retains nanosecond precision instead of trimming
// trailing zeroes like time.RFC3339Nano. Values in timestamp columns are
// ordered as text by SQLite, so a fixed-width fraction is required for their
// lexical and chronological order to match.
const sortableTimestampLayout = "2006-01-02T15:04:05.000000000Z07:00"

var (
	nowStringMu   sync.Mutex
	lastNowString time.Time
)

// NowString returns the canonical monotonic sortable timestamp used across
// the ADK persistence and projection layers.
func NowString() string {
	nowStringMu.Lock()
	defer nowStringMu.Unlock()

	now := time.Now().UTC()
	if !lastNowString.IsZero() && !now.After(lastNowString) {
		now = lastNowString.Add(time.Nanosecond)
	}
	lastNowString = now
	return now.Format(sortableTimestampLayout)
}

// NewContextRevisionID returns a fresh context revision identifier.
func NewContextRevisionID() string {
	return "ctxrev-" + uuid.NewString()
}

// NormalizeID converts arbitrary identifiers into the stable lowercase
// dash-separated form used by the ADK store.
func NormalizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if ok {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-_")
}

// NormalizeBaseURL strips the trailing slash from provider endpoints.
func NormalizeBaseURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

// NormalizeProvider applies the shared provider field normalization rules.
func NormalizeProvider(provider Provider) Provider {
	provider.ReasoningConfig = NormalizeProviderReasoningConfig(provider.ReasoningConfig)
	provider.RequestTimeoutMs = NormalizeProviderRequestTimeoutMs(provider.RequestTimeoutMs)
	provider.ContextWindowTokens = NormalizeContextWindowTokens(provider.ContextWindowTokens)
	return provider
}

// NormalizeContextWindowTokens clamps the provider context window into the
// supported range.
func NormalizeContextWindowTokens(value int) int {
	if value <= 0 {
		return 0
	}
	if value < 1_024 {
		return 1_024
	}
	if value > 10_000_000 {
		return 10_000_000
	}
	return value
}

// NormalizeRecentUserWindow clamps the recent user window into the supported
// range.
func NormalizeRecentUserWindow(value int) int {
	switch {
	case value <= 0:
		return 6
	case value < 2:
		return 2
	case value > 100:
		return 100
	default:
		return value
	}
}

// NormalizeProviderRequestTimeoutMs clamps provider request timeouts into the
// supported range and applies the shared default.
func NormalizeProviderRequestTimeoutMs(value int) int {
	const (
		minProviderRequestTimeoutMs = 15_000
		maxProviderRequestTimeoutMs = 600_000
	)
	if value <= 0 {
		return int(DefaultProviderRequestTimeout / time.Millisecond)
	}
	if value < minProviderRequestTimeoutMs {
		return minProviderRequestTimeoutMs
	}
	if value > maxProviderRequestTimeoutMs {
		return maxProviderRequestTimeoutMs
	}
	return value
}

// NormalizeHeaders trims and drops blank provider default headers.
func NormalizeHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			normalized[key] = value
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

// NormalizeStringSlice trims, deduplicates and sorts a string list.
func NormalizeStringSlice(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// OptionalTypeAssertion returns the value as T when the dynamic type matches,
// otherwise the zero value.
func OptionalTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		var zero T
		return zero
	}
	return typed
}

// NormalizePermissionMode maps unknown modes to the approval default.
func NormalizePermissionMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PermissionModeLessApproval:
		return PermissionModeLessApproval
	case PermissionModeAll:
		return PermissionModeAll
	default:
		return PermissionModeApproval
	}
}

// ValidPermissionMode reports whether value names a supported permission mode.
func ValidPermissionMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll:
		return true
	default:
		return false
	}
}

// NormalizeToolAccessMode canonicalizes the explicit Agent tool access mode.
// An omitted mode preserves the legacy meaning: an empty tool list exposes all
// registered tools, while a non-empty list is an explicit allowlist.
func NormalizeToolAccessMode(value string, tools []string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ToolAccessModeSelected:
		return ToolAccessModeSelected
	case ToolAccessModeNone:
		return ToolAccessModeNone
	case ToolAccessModeAll:
		return ToolAccessModeAll
	default:
		if len(NormalizeStringSlice(tools)) > 0 {
			return ToolAccessModeSelected
		}
		return ToolAccessModeAll
	}
}

// ValidToolAccessMode reports whether value names a supported explicit tool
// access mode. The empty value remains valid for backwards-compatible payloads.
func ValidToolAccessMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", ToolAccessModeAll, ToolAccessModeSelected, ToolAccessModeNone:
		return true
	default:
		return false
	}
}

// NormalizeWorkMode maps unknown modes to the chat default.
func NormalizeWorkMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case WorkModeLoop:
		return WorkModeLoop
	default:
		return WorkModeChat
	}
}

// NormalizeAgentDefaultWorkMode normalizes an agent's work mode while
// defaulting anything that is not a loop mode to chat.
func NormalizeAgentDefaultWorkMode(value string) string {
	switch NormalizeWorkMode(value) {
	case WorkModeLoop:
		return WorkModeLoop
	default:
		return WorkModeChat
	}
}

// normalizeWorkflowPlan canonicalizes the workflow plan projection slices.
func normalizeWorkflowPlan(plan []WorkflowStepState) []WorkflowStepState {
	if len(plan) == 0 {
		return []WorkflowStepState{}
	}
	normalized := make([]WorkflowStepState, 0, len(plan))
	for _, step := range plan {
		if len(step.DependsOn) == 0 {
			step.DependsOn = []string{}
		} else {
			step.DependsOn = NormalizeStringSlice(step.DependsOn)
		}
		if len(step.Routes) == 0 {
			step.Routes = []string{}
		} else {
			step.Routes = NormalizeStringSlice(step.Routes)
		}
		normalized = append(normalized, step)
	}
	return normalized
}

// NormalizeRun returns a canonical copy of a run projection.
func NormalizeRun(run Run) Run {
	run.WorkMode = NormalizeWorkMode(run.WorkMode)
	run.ProviderID = strings.TrimSpace(run.ProviderID)
	run.ProviderName = strings.TrimSpace(run.ProviderName)
	run.Model = strings.TrimSpace(run.Model)
	run.ReasoningEffort = NormalizeReasoningEffort(run.ReasoningEffort)
	run.ReasoningEffortField = strings.TrimSpace(run.ReasoningEffortField)
	run.ReasoningEffortValue = strings.TrimSpace(run.ReasoningEffortValue)
	run.WorkflowEngine = strings.TrimSpace(run.WorkflowEngine)
	if len(run.ChildRunIDs) == 0 {
		run.ChildRunIDs = []string{}
	} else {
		run.ChildRunIDs = NormalizeStringSlice(run.ChildRunIDs)
	}
	run.ToolCalls = NormalizeToolCalls(run.ToolCalls)
	run.PendingApprovals = NormalizeApprovals(run.PendingApprovals)
	run.InputRequests = NormalizeInputRequests(run.InputRequests)
	run.InputRequest = NormalizeInputRequest(run.InputRequest)
	if run.InputRequest != nil {
		run.InputRequests = AppendInputRequestIfMissing(run.InputRequests, *run.InputRequest)
	}
	if latest := LatestInputRequest(run.InputRequests); latest != nil {
		run.InputRequest = latest
	}
	run.WorkflowPlan = normalizeWorkflowPlan(run.WorkflowPlan)
	if len(run.ToolSummaries) == 0 {
		run.ToolSummaries = []string{}
	} else {
		run.ToolSummaries = append([]string(nil), run.ToolSummaries...)
	}
	return run
}

// NormalizeChatResponse canonicalizes the public chat projection.
func NormalizeChatResponse(response ChatResponse) ChatResponse {
	response.Run = NormalizeRun(response.Run)
	response.PendingApprovals = NormalizeApprovals(response.PendingApprovals)
	response.InputRequest = NormalizeInputRequest(response.InputRequest)
	response.Timeline = NormalizeTimelineEntries(response.Timeline)
	return response
}

// NormalizeApprovalResolution canonicalizes embedded run projections.
func NormalizeApprovalResolution(resolution ApprovalResolution) ApprovalResolution {
	if resolution.Run != nil {
		resolution.Run = new(NormalizeRun(*resolution.Run))
	}
	if resolution.ParentRun != nil {
		resolution.ParentRun = new(NormalizeRun(*resolution.ParentRun))
	}
	return resolution
}

// NormalizeSessionComposerState canonicalizes persisted composer overrides.
func NormalizeSessionComposerState(sessionID string, state SessionComposerState) SessionComposerState {
	state.SessionID = strings.TrimSpace(DefaultString(state.SessionID, sessionID))
	state.ChatDraft = limitComposerText(state.ChatDraft)
	state.ProviderIDOverride = strings.TrimSpace(state.ProviderIDOverride)
	state.ModelOverride = strings.TrimSpace(state.ModelOverride)
	state.ReasoningEffortOverride = string(NormalizeOptionalReasoningEffort(ReasoningEffort(state.ReasoningEffortOverride)))
	workMode := strings.TrimSpace(state.WorkModeOverride)
	if workMode != "" && ValidWorkMode(workMode) {
		state.WorkModeOverride = NormalizeWorkMode(workMode)
	} else {
		state.WorkModeOverride = ""
	}
	permissionMode := strings.ToLower(strings.TrimSpace(state.PermissionModeOverride))
	if permissionMode != "" && ValidPermissionMode(permissionMode) {
		state.PermissionModeOverride = NormalizePermissionMode(permissionMode)
	} else {
		state.PermissionModeOverride = ""
	}
	state.GoalObjectiveDraft = limitComposerText(state.GoalObjectiveDraft)
	return state
}

// NormalizeSessionsResponse canonicalizes a session aggregate projection.
func NormalizeSessionsResponse(response SessionsResponse) SessionsResponse {
	response.Timeline = NormalizeTimelineEntries(response.Timeline)
	if len(response.Runs) == 0 {
		response.Runs = []Run{}
	} else {
		for index := range response.Runs {
			response.Runs[index] = NormalizeRun(response.Runs[index])
		}
	}
	response.ComposerState = NormalizeSessionComposerState(response.Session.ID, response.ComposerState)
	return response
}

func limitComposerText(value string) string {
	if len([]rune(value)) <= MaxMessageLength {
		return value
	}
	return string([]rune(value)[:MaxMessageLength])
}

// NormalizeInputRequests normalizes every input request in the slice.
func NormalizeInputRequests(requests []InputRequest) []InputRequest {
	if len(requests) == 0 {
		return []InputRequest{}
	}
	result := make([]InputRequest, 0, len(requests))
	for index := range requests {
		result = append(result, *NormalizeInputRequest(&requests[index]))
	}
	return result
}

// NormalizeInputRequest returns a shallow copy with canonical slice shapes.
func NormalizeInputRequest(request *InputRequest) *InputRequest {
	if request == nil {
		return nil
	}
	result := *request
	result.Questions = append([]InputQuestion(nil), request.Questions...)
	for index := range result.Questions {
		result.Questions[index].Options = append([]InputOption(nil), result.Questions[index].Options...)
	}
	result.Answers = append([]InputAnswer(nil), request.Answers...)
	return &result
}

// AppendInputRequestIfMissing appends a normalized input request unless one
// with the same ID or function-call ID already exists.
func AppendInputRequestIfMissing(requests []InputRequest, request InputRequest) []InputRequest {
	for index := range requests {
		if requests[index].ID == request.ID || (request.FunctionCallID != "" && requests[index].FunctionCallID == request.FunctionCallID) {
			requests[index] = *NormalizeInputRequest(&request)
			return requests
		}
	}
	return append(requests, *NormalizeInputRequest(&request))
}

// LatestInputRequest returns the normalized trailing input request.
func LatestInputRequest(requests []InputRequest) *InputRequest {
	if len(requests) == 0 {
		return nil
	}
	return NormalizeInputRequest(&requests[len(requests)-1])
}

// NormalizeToolCalls returns a non-nil copy of the tool call list.
func NormalizeToolCalls(toolCalls []ToolCall) []ToolCall {
	if len(toolCalls) == 0 {
		return []ToolCall{}
	}
	return append([]ToolCall(nil), toolCalls...)
}

// NormalizeApprovals returns a non-nil copy of the approval list.
func NormalizeApprovals(approvals []Approval) []Approval {
	if len(approvals) == 0 {
		return []Approval{}
	}
	return append([]Approval(nil), approvals...)
}

// NormalizeTimelineEntries normalizes every timeline entry in the slice.
func NormalizeTimelineEntries(entries []TimelineEntry) []TimelineEntry {
	if len(entries) == 0 {
		return []TimelineEntry{}
	}
	normalized := make([]TimelineEntry, 0, len(entries))
	for _, entry := range entries {
		normalized = append(normalized, NormalizeTimelineEntry(entry))
	}
	return normalized
}

// NormalizeTimelineEntry normalizes one timeline entry's nested lists.
func NormalizeTimelineEntry(entry TimelineEntry) TimelineEntry {
	entry.ToolCalls = NormalizeToolCalls(entry.ToolCalls)
	entry.Approvals = NormalizeApprovals(entry.Approvals)
	entry.InputRequest = NormalizeInputRequest(entry.InputRequest)
	return entry
}

// ValidateInputAnswers normalizes and validates submitted input answers
// against the request's questions.
func ValidateInputAnswers(request InputRequest, submitted []InputAnswer) ([]InputAnswer, error) {
	if len(submitted) != len(request.Questions) {
		return nil, fmt.Errorf("%w: every question must be answered", ErrInputRequestInvalid)
	}
	byQuestion := make(map[string]InputAnswer, len(submitted))
	for _, answer := range submitted {
		questionID := strings.TrimSpace(answer.QuestionID)
		if questionID == "" {
			return nil, fmt.Errorf("%w: questionId is required", ErrInputRequestInvalid)
		}
		if _, exists := byQuestion[questionID]; exists {
			return nil, fmt.Errorf("%w: duplicate answer for %s", ErrInputRequestInvalid, questionID)
		}
		answer.QuestionID = questionID
		answer.OptionID = strings.TrimSpace(answer.OptionID)
		answer.OtherText = strings.TrimSpace(answer.OtherText)
		byQuestion[questionID] = answer
	}
	canonical := make([]InputAnswer, 0, len(request.Questions))
	for _, question := range request.Questions {
		answer, ok := byQuestion[question.ID]
		if !ok {
			return nil, fmt.Errorf("%w: missing answer for %s", ErrInputRequestInvalid, question.ID)
		}
		usesOption := answer.OptionID != ""
		usesOther := answer.OtherText != ""
		if usesOption == usesOther {
			return nil, fmt.Errorf("%w: %s must use exactly one answer type", ErrInputRequestInvalid, question.ID)
		}
		if usesOther {
			if !question.AllowOther {
				return nil, fmt.Errorf("%w: %s does not allow other text", ErrInputRequestInvalid, question.ID)
			}
			canonical = append(canonical, InputAnswer{QuestionID: question.ID, OtherText: answer.OtherText})
			continue
		}
		validOption := false
		for _, option := range question.Options {
			if option.ID == answer.OptionID {
				validOption = true
				break
			}
		}
		if !validOption {
			return nil, fmt.Errorf("%w: invalid option for %s", ErrInputRequestInvalid, question.ID)
		}
		canonical = append(canonical, InputAnswer{QuestionID: question.ID, OptionID: answer.OptionID})
	}
	return canonical, nil
}

// InputAnswersEqual compares answer sets by canonical JSON shape.
func InputAnswersEqual(left []InputAnswer, right []InputAnswer) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

// ValidWorkMode reports whether value names a supported work mode.
func ValidWorkMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", WorkModeChat, WorkModeLoop:
		return true
	default:
		return false
	}
}

// NormalizeLoopMaxIterations clamps the loop iteration budget.
func NormalizeLoopMaxIterations(value int) int {
	switch {
	case value <= 0:
		return DefaultLoopMaxIterations
	case value > MaxLoopIterations:
		return MaxLoopIterations
	default:
		return value
	}
}

// NormalizeTaskStatus normalizes the ADK task status vocabulary.
func NormalizeTaskStatus(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "TODO", "IN_PROGRESS", "BLOCKED", "DONE", "CANCELLED":
		return strings.ToUpper(strings.TrimSpace(value)), nil
	case "":
		return "TODO", nil
	default:
		return "", fmt.Errorf("%w %q", ErrInvalidTaskStatus, value)
	}
}

// NormalizeTaskDependsOn normalizes task dependencies and rejects self
// references.
func NormalizeTaskDependsOn(taskID string, values []string) ([]string, error) {
	taskID = NormalizeID(taskID)
	normalized := NormalizeStringSlice(values)
	for _, value := range normalized {
		if NormalizeID(value) == taskID {
			return nil, fmt.Errorf("task cannot depend on itself")
		}
	}
	return normalized, nil
}

// NormalizeMemoryKey converts memory keys into the stable normalized form.
func NormalizeMemoryKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if ok {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-_")
}

// DefaultAgentInstruction returns the canonical JFTrade agent system
// instruction.
func DefaultAgentInstruction() string {
	return "你是 JFTrade 投资分析 agent。优先使用内部行情、账户、策略和回测工具；涉及安装 skill、保存策略、运行优化或改变自动化状态时遵守当前审批等级。输出必须说明使用了哪些数据来源，不提供保证收益承诺。\n\n" +
		"对目标明确的任务，要在当前运行中连续完成诊断、结论以及直接相关的可执行方案。安全、只读且能从现有上下文合理推断的下一步，必须直接完成；不得用‘你想先做哪项’、‘你更想看哪部分’、‘是否继续’或‘如果需要我可以继续’把它留给用户。多个安全分支都直接服务原始意图时，采用推荐默认值或合并覆盖，不得仅为减少工作量要求用户选择。\n\n" +
		"只有三类真正阻塞情况可以调用 interaction.request_user：缺少只有用户才能提供的必要信息、存在无法合并的重大取舍，或继续会越过权限/任务范围边界。提问时必须如实填写 decisionKind 和 blockingReason。实际写操作仍走审批流程，不得用提问工具替代授权。"
}

// RunHasPendingApproval reports whether any approval is still pending.
func RunHasPendingApproval(approvals []Approval) bool {
	for _, approval := range approvals {
		if approval.Status == ApprovalStatusPending {
			return true
		}
	}
	return false
}

// FinishToolCall stamps a tool call as completed and records its duration.
func FinishToolCall(call *ToolCall) {
	if call == nil {
		return
	}
	completedAt := NowString()
	call.CompletedAt = &completedAt
	call.UpdatedAt = completedAt
	startedAt, startErr := time.Parse(time.RFC3339Nano, call.StartedAt)
	completed, completedErr := time.Parse(time.RFC3339Nano, completedAt)
	if startErr == nil && completedErr == nil {
		call.DurationMs = completed.Sub(startedAt).Milliseconds()
	}
}
