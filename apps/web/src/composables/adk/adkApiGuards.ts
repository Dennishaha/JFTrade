import type {
  ADKAgent,
  ADKApproval,
  ADKApprovalResolution,
  ADKAuditEvent,
  ADKChatResponse,
  ADKInputAnswer,
  ADKInputOption,
  ADKInputQuestion,
  ADKInputRequest,
  ADKInputResolution,
  ADKMemoryEntry,
  ADKMessage,
  ADKOptimizationRun,
  ADKOptimizationTask,
  ADKPermissionMode,
  ADKProvider,
  ADKRun,
  ADKSession,
  ADKSessionComposerState,
  ADKSessionContextSnapshot,
  ADKSkill,
  ADKTask,
  ADKTimelineEntry,
  ADKToolCall,
  ADKToolDescriptor,
  ADKWorkflowCanvasEdge,
  ADKWorkflowCanvasGraph,
  ADKWorkflowCanvasNode,
  ADKWorkflowDefinition,
  ADKWorkflowInvocationResult,
  ADKWorkflowNodeRun,
  ADKWorkflowResult,
  ADKWorkflowStepState,
  ADKWorkflowTrigger,
  ADKWorkflowTriggerLog,
  ADKWorkflowTriggerSaveResult,
  ADKWorkMode,
  MCPServerSettingsSnapshot,
  MCPServerTokenResetResult,
} from "@/types";
import type { ADKRuntimeSettings } from "@/contracts";

import type { ADKMetricsView, ADKPageEnvelope } from "./adkApiMapperModels";
import {
  isADKProviderReasoningConfig,
  isOptionalReasoningEffortWire,
} from "./adkApiReasoningGuards";

export {
  isADKProviderReasoningConfig,
  isADKProviderReasoningMapping,
  isADKProviderReasoningTestResponse,
  isADKProviderReasoningTestResult,
  isADKProviderTestResponse,
  isOptionalReasoningEffortWire,
  isReasoningEffort,
  normalizeADKAgentTemplateWire,
} from "./adkApiReasoningGuards";

type TypeGuard<T> = (value: unknown) => value is T;

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function isString(value: unknown): value is string {
  return typeof value === "string";
}

export function isNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

export function isBoolean(value: unknown): value is boolean {
  return typeof value === "boolean";
}

export function isNullableNumber(value: unknown): value is number | null {
  return value === null || isNumber(value);
}

export function isStringRecord(value: unknown): value is Record<string, string> {
  return isRecord(value) && Object.values(value).every(isString);
}

export function isNumberRecord(value: unknown): value is Record<string, number> {
  return isRecord(value) && Object.values(value).every(isNumber);
}

export function isArrayOf<T>(value: unknown, guard: TypeGuard<T>): value is T[] {
  return Array.isArray(value) && value.every(guard);
}

export function isOptional<T>(value: unknown, guard: TypeGuard<T>): boolean {
  return value === undefined || guard(value);
}

export function requireValue<T>(
  value: unknown,
  guard: TypeGuard<T>,
  label: string,
): T {
  if (!guard(value)) {
    throw new TypeError(`ADK API response is invalid: ${label}`);
  }
  return value;
}

export function isPermissionMode(value: unknown): value is ADKPermissionMode {
  return value === "approval" || value === "less_approval" || value === "all";
}

export function isWorkMode(value: unknown): value is ADKWorkMode {
  return value === "chat" || value === "loop";
}

export function isADKProvider(value: unknown): value is ADKProvider {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.displayName) &&
    isString(value.baseUrl) &&
    isString(value.model) &&
    isOptional(value.reasoningConfig, isADKProviderReasoningConfig) &&
    isNumber(value.requestTimeoutMs) &&
    isBoolean(value.enabled) &&
    isBoolean(value.default) &&
    isBoolean(value.hasApiKey) &&
    isString(value.createdAt) &&
    isString(value.updatedAt) &&
    isOptional(value.defaultHeaders, isStringRecord)
  );
}

export function isADKAgent(value: unknown): value is ADKAgent {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.name) &&
    isString(value.instruction) &&
    isString(value.providerId) &&
    isString(value.model) &&
    isOptionalReasoningEffortWire(value.reasoningEffort) &&
    isArrayOf(value.tools, isString) &&
    isArrayOf(value.skills, isString) &&
    isPermissionMode(value.permissionMode) &&
    isBoolean(value.memoryEnabled) &&
    isNumber(value.recentUserWindow) &&
    isWorkMode(value.workMode) &&
    isNumber(value.loopMaxIterations) &&
    isString(value.status) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKAgentTemplate(
  value: unknown,
): value is Omit<ADKAgent, "createdAt" | "updatedAt"> {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.name) &&
    isString(value.instruction) &&
    isString(value.providerId) &&
    isString(value.model) &&
    isOptionalReasoningEffortWire(value.reasoningEffort) &&
    isArrayOf(value.tools, isString) &&
    isArrayOf(value.skills, isString) &&
    isPermissionMode(value.permissionMode) &&
    isBoolean(value.memoryEnabled) &&
    isNumber(value.recentUserWindow) &&
    isWorkMode(value.workMode) &&
    isNumber(value.loopMaxIterations) &&
    isString(value.status)
  );
}

export function isADKToolDescriptor(value: unknown): value is ADKToolDescriptor {
  if (!isRecord(value)) return false;
  return (
    isString(value.name) &&
    isString(value.displayName) &&
    isString(value.description) &&
    isString(value.category) &&
    isString(value.permission) &&
    isArrayOf(value.allowedModes, isPermissionMode) &&
    isArrayOf(value.requiresApprovalIn, isPermissionMode)
  );
}

export function normalizeADKToolDescriptor(value: unknown): unknown {
  if (!isRecord(value) || value.requiresApprovalIn !== null) {
    return value;
  }
  return { ...value, requiresApprovalIn: [] };
}

export function isADKSkill(value: unknown): value is ADKSkill {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.displayName) &&
    isString(value.description) &&
    isString(value.source) &&
    isString(value.installPath) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKToolCall(value: unknown): value is ADKToolCall {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.runId) &&
    isString(value.toolName) &&
    isString(value.permission) &&
    isString(value.status) &&
    isBoolean(value.requiresUser) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKApproval(value: unknown): value is ADKApproval {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.runId) &&
    isString(value.agentId) &&
    isString(value.toolName) &&
    isString(value.status) &&
    isString(value.reason) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKInputOption(value: unknown): value is ADKInputOption {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.label) &&
    isOptional(value.description, isString) &&
    isOptional(value.recommended, isBoolean)
  );
}

export function isADKInputQuestion(value: unknown): value is ADKInputQuestion {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.question) &&
    isArrayOf(value.options, isADKInputOption) &&
    isBoolean(value.allowOther)
  );
}

export function isADKInputAnswer(value: unknown): value is ADKInputAnswer {
  return (
    isRecord(value) &&
    isString(value.questionId) &&
    isOptional(value.optionId, isString) &&
    isOptional(value.otherText, isString)
  );
}

export function isADKInputRequest(value: unknown): value is ADKInputRequest {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.runId) &&
    isString(value.agentId) &&
    isString(value.functionCallId) &&
    isString(value.status) &&
    isArrayOf(value.questions, isADKInputQuestion) &&
    isOptional(value.answers, (answers): answers is ADKInputAnswer[] =>
      isArrayOf(answers, isADKInputAnswer),
    ) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKWorkflowStep(value: unknown): value is ADKWorkflowStepState {
  return (
    isRecord(value) &&
    isString(value.title) &&
    isString(value.status) &&
    isOptional(value.dependsOn, (items): items is string[] =>
      isArrayOf(items, isString),
    ) &&
    isOptional(value.routes, (items): items is string[] =>
      isArrayOf(items, isString),
    )
  );
}

export function isADKRun(value: unknown): value is ADKRun {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.sessionId) &&
    isString(value.agentId) &&
    isString(value.status) &&
    isString(value.message) &&
    isOptionalReasoningEffortWire(value.reasoningEffort) &&
    isArrayOf(value.toolCalls, isADKToolCall) &&
    isArrayOf(value.pendingApprovals, isADKApproval) &&
    isOptional(value.inputRequest, isADKInputRequest) &&
    isOptional(value.inputRequests, (requests): requests is ADKInputRequest[] =>
      isArrayOf(requests, isADKInputRequest),
    ) &&
    isOptional(value.workflowPlan, (steps): steps is ADKWorkflowStepState[] =>
      isArrayOf(steps, isADKWorkflowStep),
    ) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKSession(value: unknown): value is ADKSession {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.agentId) &&
    isString(value.title) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKSessionComposerState(
  value: unknown,
): value is ADKSessionComposerState {
  return (
    isRecord(value) &&
    isString(value.sessionId) &&
    isString(value.chatDraft) &&
    isString(value.providerIdOverride) &&
    isString(value.modelOverride) &&
    isString(value.reasoningEffortOverride) &&
    isString(value.workModeOverride) &&
    isString(value.permissionModeOverride) &&
    isString(value.goalObjectiveDraft) &&
    isBoolean(value.goalObjectiveTouched)
  );
}

export function isContextBreakdown(
  value: unknown,
): value is ADKSessionContextSnapshot["breakdown"] {
  return (
    isRecord(value) &&
    isNumber(value.instructionTokens) &&
    isNumber(value.handoffTokens) &&
    isNumber(value.recentUserTokens) &&
    isNumber(value.protectedTailTokens) &&
    isNumber(value.otherVisibleTokens) &&
    isNumber(value.pendingUserTokens) &&
    isNumber(value.toolDeclarationTokens)
  );
}

export function isADKSessionContextSnapshot(
  value: unknown,
): value is ADKSessionContextSnapshot {
  if (!isRecord(value)) return false;
  return (
    isString(value.sessionId) &&
    isNumber(value.currentInputTokens) &&
    isNumber(value.projectedNextTurnTokens) &&
    isNumber(value.contextWindowTokens) &&
    isNumber(value.usageRatio) &&
    isString(value.status) &&
    isNumber(value.recentUserWindow) &&
    isNumber(value.retainedRecentUserCount) &&
    isNumber(value.activeHandoffCount) &&
    isContextBreakdown(value.breakdown) &&
    isOptional(value.rawBreakdown, isContextBreakdown) &&
    isBoolean(value.autoCompacted) &&
    isBoolean(value.degradedSummary)
  );
}

export function isADKTimelineEntry(value: unknown): value is ADKTimelineEntry {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.sessionId) &&
    isString(value.kind) &&
    isString(value.createdAt) &&
    isNumber(value.sequence) &&
    isOptional(value.toolCalls, (calls): calls is ADKToolCall[] =>
      isArrayOf(calls, isADKToolCall),
    ) &&
    isOptional(value.approvals, (approvals): approvals is ADKApproval[] =>
      isArrayOf(approvals, isADKApproval),
    ) &&
    isOptional(value.inputRequest, isADKInputRequest)
  );
}

export function isADKTask(value: unknown): value is ADKTask {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.title) &&
    isString(value.status) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKMemoryEntry(value: unknown): value is ADKMemoryEntry {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.key) &&
    isString(value.value) &&
    isString(value.scope) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKOptimizationRun(value: unknown): value is ADKOptimizationRun {
  return (
    isRecord(value) &&
    isString(value.definitionId) &&
    isString(value.runId) &&
    isString(value.status)
  );
}

export function isADKOptimizationTask(
  value: unknown,
): value is ADKOptimizationTask {
  if (!isRecord(value) || !isRecord(value.progress)) return false;
  return (
    isString(value.id) &&
    isString(value.status) &&
    isString(value.objective) &&
    isArrayOf(value.runs, isADKOptimizationRun) &&
    isNumber(value.progress.total) &&
    isNumber(value.progress.running) &&
    isNumber(value.progress.completed) &&
    isNumber(value.progress.failed) &&
    isNumber(value.progress.cancelled) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKAuditEvent(value: unknown): value is ADKAuditEvent {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.kind) &&
    isString(value.detail) &&
    isString(value.createdAt)
  );
}

export function isCanvasNode(value: unknown): value is ADKWorkflowCanvasNode {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.type) &&
    isRecord(value.position) &&
    isNumber(value.position.x) &&
    isNumber(value.position.y)
  );
}

export function isCanvasEdge(value: unknown): value is ADKWorkflowCanvasEdge {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.source) &&
    isString(value.target)
  );
}

export function isCanvasGraph(value: unknown): value is ADKWorkflowCanvasGraph {
  return (
    isRecord(value) &&
    isOptional(value.nodes, (nodes): nodes is ADKWorkflowCanvasNode[] =>
      isArrayOf(nodes, isCanvasNode),
    ) &&
    isOptional(value.edges, (edges): edges is ADKWorkflowCanvasEdge[] =>
      isArrayOf(edges, isCanvasEdge),
    )
  );
}

export function isADKWorkflowDefinition(
  value: unknown,
): value is ADKWorkflowDefinition {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.name) &&
    isString(value.status) &&
    isString(value.agentId) &&
    isString(value.workMode) &&
    isString(value.promptTemplate) &&
    isOptional(value.canvasGraph, isCanvasGraph) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKWorkflowTrigger(value: unknown): value is ADKWorkflowTrigger {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.workflowId) &&
    isString(value.type) &&
    isString(value.title) &&
    isString(value.status) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKWorkflowNodeRun(value: unknown): value is ADKWorkflowNodeRun {
  return (
    isRecord(value) &&
    isString(value.nodeId) &&
    isString(value.nodeType) &&
    isString(value.status)
  );
}

export function isADKMessage(value: unknown): value is ADKMessage {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.sessionId) &&
    isString(value.role) &&
    isString(value.kind) &&
    isString(value.content) &&
    isString(value.createdAt)
  );
}

export function isADKChatResponse(value: unknown): value is ADKChatResponse {
  return (
    isRecord(value) &&
    isString(value.reply) &&
    isADKSession(value.session) &&
    isADKRun(value.run) &&
    isArrayOf(value.pendingApprovals, isADKApproval) &&
    isArrayOf(value.timeline, isADKTimelineEntry) &&
    isOptional(value.inputRequest, isADKInputRequest) &&
    isOptional(value.context, isADKSessionContextSnapshot)
  );
}

export function isADKWorkflowResult(value: unknown): value is ADKWorkflowResult {
  return (
    isRecord(value) &&
    isOptional(value.format, isString) &&
    isOptional(value.markdown, isString) &&
    isOptional(value.rawResponse, isADKChatResponse)
  );
}

export function isADKWorkflowTriggerLog(
  value: unknown,
): value is ADKWorkflowTriggerLog {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.workflowId) &&
    isString(value.triggerType) &&
    isString(value.status) &&
    isOptional(value.result, isADKWorkflowResult) &&
    isOptional(value.nodeRuns, (runs): runs is ADKWorkflowNodeRun[] =>
      isArrayOf(runs, isADKWorkflowNodeRun),
    ) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

export function isADKApprovalResolution(
  value: unknown,
): value is ADKApprovalResolution {
  return (
    isRecord(value) &&
    isADKApproval(value.approval) &&
    isOptional(value.run, isADKRun) &&
    isOptional(value.parentRun, isADKRun) &&
    isOptional(value.message, isADKMessage)
  );
}

export function isADKInputResolution(value: unknown): value is ADKInputResolution {
  return (
    isRecord(value) &&
    isADKInputRequest(value.request) &&
    isOptional(value.run, isADKRun) &&
    isOptional(value.parentRun, isADKRun) &&
    isOptional(value.message, isADKMessage)
  );
}

export function isADKWorkflowInvocation(
  value: unknown,
): value is ADKWorkflowInvocationResult {
  return (
    isRecord(value) &&
    isADKWorkflowDefinition(value.workflow) &&
    isADKWorkflowTriggerLog(value.log) &&
    isOptional(value.trigger, isADKWorkflowTrigger) &&
    isOptional(value.response, isADKChatResponse)
  );
}

export function isADKWorkflowTriggerSaveResult(
  value: unknown,
): value is ADKWorkflowTriggerSaveResult {
  return (
    isRecord(value) &&
    isADKWorkflowTrigger(value.trigger) &&
    isOptional(value.secret, isString)
  );
}

export function isPageEnvelope(value: unknown): value is ADKPageEnvelope {
  return (
    isRecord(value) &&
    isNumber(value.limit) &&
    isNumber(value.offset) &&
    isNumber(value.total) &&
    isNumber(value.returned) &&
    isBoolean(value.hasMore)
  );
}

export function isRuntimeSettings(value: unknown): value is ADKRuntimeSettings {
  return (
    isRecord(value) &&
    isNumber(value.runTimeoutMs) &&
    isNumber(value.streamIdleTimeoutMs)
  );
}

export function isMCPSettingsSnapshot(
  value: unknown,
): value is MCPServerSettingsSnapshot {
  if (!isRecord(value) || !isRecord(value.settings) || !isRecord(value.status)) {
    return false;
  }
  return (
    isBoolean(value.settings.enabled) &&
    isNumber(value.settings.port) &&
    (value.settings.authMode === "token" || value.settings.authMode === "none") &&
    isBoolean(value.settings.tokenConfigured) &&
    isBoolean(value.status.running) &&
    isString(value.status.endpoint) &&
    isOptional(value.status.lastError, isString)
  );
}

export function isMCPTokenResetResult(
  value: unknown,
): value is MCPServerTokenResetResult {
  return (
    isRecord(value) && isMCPSettingsSnapshot(value) && isString(value.token)
  );
}

export function isMetricsView(value: unknown): value is ADKMetricsView {
  if (
    !isRecord(value) ||
    !isRecord(value.runs) ||
    !isRecord(value.runs.lifecycle) ||
    !isRecord(value.tools) ||
    !isRecord(value.approvals) ||
    !isRecord(value.approvals.pendingWaitMs) ||
    !isRecord(value.approvals.resolutionWaitMs) ||
    !isRecord(value.usage) ||
    !isRecord(value.sessions) ||
    !isRecord(value.workflows) ||
    !isRecord(value.measurementWindow)
  ) {
    return false;
  }
  const runs = value.runs;
  const lifecycle = runs.lifecycle;
  const approvals = value.approvals;
  const workflows = value.workflows;
  if (!isRecord(lifecycle) || !isRecord(approvals) || !isRecord(workflows)) {
    return false;
  }
  return (
    isNumber(runs.total) &&
    isNumber(runs.last7Days) &&
    isNumberRecord(runs.byStatus) &&
    isNumberRecord(runs.byAgent) &&
    isNumberRecord(runs.byProvider) &&
    ["failed", "timedOut", "cancelled", "resumed", "orphaned"].every(
      (key) => isNumber(lifecycle[key]),
    ) &&
    isNumber(value.tools.total) &&
    isNumber(value.tools.successful) &&
    isNumber(value.tools.averageDurationMs) &&
    isNumberRecord(value.tools.byName) &&
    isNumberRecord(value.tools.byStatus) &&
    ["pending", "total", "last7Days", "approved", "denied", "recoverablePending"].every(
      (key) => isNumber(approvals[key]),
    ) &&
    isNumber(value.approvals.pendingWaitMs.average) &&
    isNumber(value.approvals.pendingWaitMs.max) &&
    isNumber(value.approvals.resolutionWaitMs.average) &&
    isNumber(value.approvals.resolutionWaitMs.max) &&
    isNumber(value.approvals.resolutionWaitMs.count) &&
    isNumber(value.usage.samples) &&
    isNullableNumber(value.usage.tokensInTotal) &&
    isNullableNumber(value.usage.tokensOutTotal) &&
    isNullableNumber(value.usage.tokensInAverage) &&
    isNullableNumber(value.usage.tokensOutAverage) &&
    isNumber(value.sessions.total) &&
    isNumber(value.sessions.last7Days) &&
    [
      "definitions",
      "enabledDefinitions",
      "triggers",
      "enabledTriggers",
      "invocations",
      "invocationsLast7Days",
    ].every((key) => isNumber(workflows[key])) &&
    isNumberRecord(workflows.byStatus) &&
    isNumberRecord(workflows.byTriggerType) &&
    isNumber(value.measurementWindow.days) &&
    isString(value.measurementWindow.since)
  );
}

export function requireList<T>(
  value: unknown,
  guard: TypeGuard<T>,
  label: string,
): T[] {
  return requireValue(
    value,
    (candidate): candidate is T[] => isArrayOf(candidate, guard),
    label,
  );
}

export function normalizeRunWire(value: unknown): unknown {
  if (!isRecord(value)) return value;
  return {
    ...value,
    ...(value.toolCalls === null ? { toolCalls: [] } : {}),
    ...(value.pendingApprovals === null ? { pendingApprovals: [] } : {}),
  };
}

export function normalizeTimelineWire(value: unknown): unknown {
  if (!isRecord(value)) return value;
  return {
    ...value,
    ...(value.toolCalls === null ? { toolCalls: [] } : {}),
    ...(value.approvals === null ? { approvals: [] } : {}),
  };
}
