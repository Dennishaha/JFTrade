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
import type { components } from "@/generated/openapi";

type ADKProviderWire = components["schemas"]["adk.Provider"];

export interface ADKPageEnvelope {
  limit: number;
  offset: number;
  total: number;
  returned: number;
  hasMore: boolean;
}

export interface ADKMetricsView {
  runs: {
    total: number;
    byStatus: Record<string, number>;
    byAgent: Record<string, number>;
    byProvider: Record<string, number>;
    lifecycle: {
      failed: number;
      timedOut: number;
      cancelled: number;
      resumed: number;
      orphaned: number;
    };
  };
  tools: {
    total: number;
    successful: number;
    averageDurationMs: number;
    byName: Record<string, number>;
    byStatus: Record<string, number>;
  };
  approvals: {
    pending: number;
    total: number;
    approved: number;
    denied: number;
    recoverablePending: number;
    pendingWaitMs: { average: number; max: number };
    resolutionWaitMs: { average: number; max: number; count: number };
  };
  usage: {
    samples: number;
    tokensInTotal: number | null;
    tokensOutTotal: number | null;
    tokensInAverage: number | null;
    tokensOutAverage: number | null;
  };
}

type TypeGuard<T> = (value: unknown) => value is T;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function isNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isBoolean(value: unknown): value is boolean {
  return typeof value === "boolean";
}

function isNullableNumber(value: unknown): value is number | null {
  return value === null || isNumber(value);
}

function isStringRecord(value: unknown): value is Record<string, string> {
  return isRecord(value) && Object.values(value).every(isString);
}

function isNumberRecord(value: unknown): value is Record<string, number> {
  return isRecord(value) && Object.values(value).every(isNumber);
}

function isArrayOf<T>(value: unknown, guard: TypeGuard<T>): value is T[] {
  return Array.isArray(value) && value.every(guard);
}

function isOptional<T>(value: unknown, guard: TypeGuard<T>): boolean {
  return value === undefined || guard(value);
}

function requireValue<T>(
  value: unknown,
  guard: TypeGuard<T>,
  label: string,
): T {
  if (!guard(value)) {
    throw new TypeError(`ADK API response is invalid: ${label}`);
  }
  return value;
}

function isPermissionMode(value: unknown): value is ADKPermissionMode {
  return value === "approval" || value === "less_approval" || value === "all";
}

function isWorkMode(value: unknown): value is ADKWorkMode {
  return value === "chat" || value === "loop";
}

function isADKProvider(value: unknown): value is ADKProvider {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.displayName) &&
    isString(value.baseUrl) &&
    isString(value.model) &&
    isNumber(value.requestTimeoutMs) &&
    isBoolean(value.enabled) &&
    isBoolean(value.default) &&
    isBoolean(value.hasApiKey) &&
    isString(value.createdAt) &&
    isString(value.updatedAt) &&
    isOptional(value.defaultHeaders, isStringRecord)
  );
}

function isADKAgent(value: unknown): value is ADKAgent {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.name) &&
    isString(value.instruction) &&
    isString(value.providerId) &&
    isString(value.model) &&
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

function isADKAgentTemplate(
  value: unknown,
): value is Omit<ADKAgent, "createdAt" | "updatedAt"> {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.name) &&
    isString(value.instruction) &&
    isString(value.providerId) &&
    isString(value.model) &&
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

function isADKToolDescriptor(value: unknown): value is ADKToolDescriptor {
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

function normalizeADKToolDescriptor(value: unknown): unknown {
  if (!isRecord(value) || value.requiresApprovalIn !== null) {
    return value;
  }
  return { ...value, requiresApprovalIn: [] };
}

function isADKSkill(value: unknown): value is ADKSkill {
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

function isADKToolCall(value: unknown): value is ADKToolCall {
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

function isADKApproval(value: unknown): value is ADKApproval {
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

function isADKInputOption(value: unknown): value is ADKInputOption {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.label) &&
    isOptional(value.description, isString) &&
    isOptional(value.recommended, isBoolean)
  );
}

function isADKInputQuestion(value: unknown): value is ADKInputQuestion {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.question) &&
    isArrayOf(value.options, isADKInputOption) &&
    isBoolean(value.allowOther)
  );
}

function isADKInputAnswer(value: unknown): value is ADKInputAnswer {
  return (
    isRecord(value) &&
    isString(value.questionId) &&
    isOptional(value.optionId, isString) &&
    isOptional(value.otherText, isString)
  );
}

function isADKInputRequest(value: unknown): value is ADKInputRequest {
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

function isADKWorkflowStep(value: unknown): value is ADKWorkflowStepState {
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

function isADKRun(value: unknown): value is ADKRun {
  if (!isRecord(value)) return false;
  return (
    isString(value.id) &&
    isString(value.sessionId) &&
    isString(value.agentId) &&
    isString(value.status) &&
    isString(value.message) &&
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

function isADKSession(value: unknown): value is ADKSession {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.agentId) &&
    isString(value.title) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

function isADKSessionComposerState(
  value: unknown,
): value is ADKSessionComposerState {
  return (
    isRecord(value) &&
    isString(value.sessionId) &&
    isString(value.chatDraft) &&
    isString(value.providerIdOverride) &&
    isString(value.modelOverride) &&
    isString(value.workModeOverride) &&
    isString(value.permissionModeOverride) &&
    isString(value.goalObjectiveDraft) &&
    isBoolean(value.goalObjectiveTouched)
  );
}

function isContextBreakdown(
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

function isADKSessionContextSnapshot(
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

function isADKTimelineEntry(value: unknown): value is ADKTimelineEntry {
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

function isADKTask(value: unknown): value is ADKTask {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.title) &&
    isString(value.status) &&
    isString(value.createdAt) &&
    isString(value.updatedAt)
  );
}

function isADKMemoryEntry(value: unknown): value is ADKMemoryEntry {
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

function isADKOptimizationRun(value: unknown): value is ADKOptimizationRun {
  return (
    isRecord(value) &&
    isString(value.definitionId) &&
    isString(value.runId) &&
    isString(value.status)
  );
}

function isADKOptimizationTask(
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

function isADKAuditEvent(value: unknown): value is ADKAuditEvent {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.kind) &&
    isString(value.detail) &&
    isString(value.createdAt)
  );
}

function isCanvasNode(value: unknown): value is ADKWorkflowCanvasNode {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.type) &&
    isRecord(value.position) &&
    isNumber(value.position.x) &&
    isNumber(value.position.y)
  );
}

function isCanvasEdge(value: unknown): value is ADKWorkflowCanvasEdge {
  return (
    isRecord(value) &&
    isString(value.id) &&
    isString(value.source) &&
    isString(value.target)
  );
}

function isCanvasGraph(value: unknown): value is ADKWorkflowCanvasGraph {
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

function isADKWorkflowDefinition(
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

function isADKWorkflowTrigger(value: unknown): value is ADKWorkflowTrigger {
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

function isADKWorkflowNodeRun(value: unknown): value is ADKWorkflowNodeRun {
  return (
    isRecord(value) &&
    isString(value.nodeId) &&
    isString(value.nodeType) &&
    isString(value.status)
  );
}

function isADKMessage(value: unknown): value is ADKMessage {
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

function isADKChatResponse(value: unknown): value is ADKChatResponse {
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

function isADKWorkflowResult(value: unknown): value is ADKWorkflowResult {
  return (
    isRecord(value) &&
    isOptional(value.format, isString) &&
    isOptional(value.markdown, isString) &&
    isOptional(value.rawResponse, isADKChatResponse)
  );
}

function isADKWorkflowTriggerLog(
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

function isADKApprovalResolution(
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

function isADKInputResolution(value: unknown): value is ADKInputResolution {
  return (
    isRecord(value) &&
    isADKInputRequest(value.request) &&
    isOptional(value.run, isADKRun) &&
    isOptional(value.parentRun, isADKRun) &&
    isOptional(value.message, isADKMessage)
  );
}

function isADKWorkflowInvocation(
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

function isADKWorkflowTriggerSaveResult(
  value: unknown,
): value is ADKWorkflowTriggerSaveResult {
  return (
    isRecord(value) &&
    isADKWorkflowTrigger(value.trigger) &&
    isOptional(value.secret, isString)
  );
}

function isPageEnvelope(value: unknown): value is ADKPageEnvelope {
  return (
    isRecord(value) &&
    isNumber(value.limit) &&
    isNumber(value.offset) &&
    isNumber(value.total) &&
    isNumber(value.returned) &&
    isBoolean(value.hasMore)
  );
}

function isRuntimeSettings(value: unknown): value is ADKRuntimeSettings {
  return (
    isRecord(value) &&
    isNumber(value.runTimeoutMs) &&
    isNumber(value.streamIdleTimeoutMs)
  );
}

function isMCPSettingsSnapshot(
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

function isMCPTokenResetResult(
  value: unknown,
): value is MCPServerTokenResetResult {
  return (
    isRecord(value) && isMCPSettingsSnapshot(value) && isString(value.token)
  );
}

function isMetricsView(value: unknown): value is ADKMetricsView {
  if (
    !isRecord(value) ||
    !isRecord(value.runs) ||
    !isRecord(value.runs.lifecycle) ||
    !isRecord(value.tools) ||
    !isRecord(value.approvals) ||
    !isRecord(value.approvals.pendingWaitMs) ||
    !isRecord(value.approvals.resolutionWaitMs) ||
    !isRecord(value.usage)
  ) {
    return false;
  }
  const runs = value.runs;
  const lifecycle = runs.lifecycle;
  const approvals = value.approvals;
  if (!isRecord(lifecycle) || !isRecord(approvals)) {
    return false;
  }
  return (
    isNumber(runs.total) &&
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
    ["pending", "total", "approved", "denied", "recoverablePending"].every(
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
    isNullableNumber(value.usage.tokensOutAverage)
  );
}

function requireList<T>(
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

function normalizeRunWire(value: unknown): unknown {
  if (!isRecord(value)) return value;
  return {
    ...value,
    ...(value.toolCalls === null ? { toolCalls: [] } : {}),
    ...(value.pendingApprovals === null ? { pendingApprovals: [] } : {}),
  };
}

function normalizeTimelineWire(value: unknown): unknown {
  if (!isRecord(value)) return value;
  return {
    ...value,
    ...(value.toolCalls === null ? { toolCalls: [] } : {}),
    ...(value.approvals === null ? { approvals: [] } : {}),
  };
}

export const requireADKProvider = (value: unknown): ADKProvider =>
  requireValue(value, isADKProvider, "provider");
export const requireADKProviders = (value: unknown): ADKProvider[] =>
  requireList(value, isADKProvider, "providers");
export const requireADKAgent = (value: unknown): ADKAgent =>
  requireValue(value, isADKAgent, "agent");
export const requireADKAgents = (value: unknown): ADKAgent[] =>
  requireList(value, isADKAgent, "agents");
export const requireADKAgentTemplates = (
  value: unknown,
): Array<Omit<ADKAgent, "createdAt" | "updatedAt">> =>
  requireList(value, isADKAgentTemplate, "agent templates");
export const requireADKToolDescriptors = (
  value: unknown,
): ADKToolDescriptor[] =>
  requireList(
    Array.isArray(value) ? value.map(normalizeADKToolDescriptor) : value,
    isADKToolDescriptor,
    "tools",
  );
export const requireADKSkill = (value: unknown): ADKSkill =>
  requireValue(value, isADKSkill, "skill");
export const requireADKSkills = (value: unknown): ADKSkill[] =>
  requireList(value, isADKSkill, "skills");
export const requireADKRun = (value: unknown): ADKRun =>
  requireValue(normalizeRunWire(value), isADKRun, "run");
export const requireADKRuns = (value: unknown): ADKRun[] => {
  if (!Array.isArray(value)) {
    throw new TypeError("ADK API response is invalid: runs");
  }
  return value.map(requireADKRun);
};
export const requireADKApproval = (value: unknown): ADKApproval =>
  requireValue(value, isADKApproval, "approval");
export const requireADKApprovals = (value: unknown): ADKApproval[] =>
  requireList(value, isADKApproval, "approvals");
export const requireADKApprovalResolution = (
  value: unknown,
): ADKApprovalResolution =>
  requireValue(value, isADKApprovalResolution, "approval resolution");
export const requireADKInputResolution = (
  value: unknown,
): ADKInputResolution =>
  requireValue(value, isADKInputResolution, "input resolution");
export const requireADKSession = (value: unknown): ADKSession =>
  requireValue(value, isADKSession, "session");
export const requireADKSessions = (value: unknown): ADKSession[] =>
  requireList(value, isADKSession, "sessions");
export const requireADKComposerState = (
  value: unknown,
): ADKSessionComposerState =>
  requireValue(value, isADKSessionComposerState, "session composer state");
export const requireADKContextSnapshot = (
  value: unknown,
): ADKSessionContextSnapshot =>
  requireValue(value, isADKSessionContextSnapshot, "session context");
export const requireADKTimeline = (value: unknown): ADKTimelineEntry[] => {
  if (!Array.isArray(value)) {
    throw new TypeError("ADK API response is invalid: timeline");
  }
  return value.map((entry) =>
    requireValue(normalizeTimelineWire(entry), isADKTimelineEntry, "timeline"),
  );
};
export const requireADKTask = (value: unknown): ADKTask =>
  requireValue(value, isADKTask, "task");
export const requireADKTasks = (value: unknown): ADKTask[] =>
  requireList(value, isADKTask, "tasks");
export const requireADKMemoryEntry = (value: unknown): ADKMemoryEntry =>
  requireValue(value, isADKMemoryEntry, "memory entry");
export const requireADKMemoryEntries = (value: unknown): ADKMemoryEntry[] =>
  requireList(value, isADKMemoryEntry, "memory entries");
export const requireADKOptimizationTask = (
  value: unknown,
): ADKOptimizationTask =>
  requireValue(value, isADKOptimizationTask, "optimization task");
export const requireADKOptimizationTasks = (
  value: unknown,
): ADKOptimizationTask[] =>
  requireList(value, isADKOptimizationTask, "optimization tasks");
export const requireADKAuditEvents = (value: unknown): ADKAuditEvent[] =>
  requireList(value, isADKAuditEvent, "audit events");
export const requireADKPage = (value: unknown): ADKPageEnvelope =>
  requireValue(value, isPageEnvelope, "page");
export const requireADKRuntimeSettings = (
  value: unknown,
): ADKRuntimeSettings =>
  requireValue(value, isRuntimeSettings, "runtime settings");
export const requireMCPSettingsSnapshot = (
  value: unknown,
): MCPServerSettingsSnapshot =>
  requireValue(value, isMCPSettingsSnapshot, "MCP settings");
export const requireMCPTokenResetResult = (
  value: unknown,
): MCPServerTokenResetResult =>
  requireValue(value, isMCPTokenResetResult, "MCP token reset");
export const requireADKMetrics = (value: unknown): ADKMetricsView =>
  requireValue(value, isMetricsView, "metrics");
export const requireADKWorkflowDefinition = (
  value: unknown,
): ADKWorkflowDefinition =>
  requireValue(value, isADKWorkflowDefinition, "workflow");
export const requireADKWorkflowDefinitions = (
  value: unknown,
): ADKWorkflowDefinition[] =>
  requireList(value, isADKWorkflowDefinition, "workflows");
export const requireADKWorkflowTrigger = (
  value: unknown,
): ADKWorkflowTrigger =>
  requireValue(value, isADKWorkflowTrigger, "workflow trigger");
export const requireADKWorkflowTriggers = (
  value: unknown,
): ADKWorkflowTrigger[] =>
  requireList(value, isADKWorkflowTrigger, "workflow triggers");
export const requireADKWorkflowTriggerLogs = (
  value: unknown,
): ADKWorkflowTriggerLog[] =>
  requireList(value, isADKWorkflowTriggerLog, "workflow trigger logs");
export const requireADKWorkflowInvocation = (
  value: unknown,
): ADKWorkflowInvocationResult =>
  requireValue(value, isADKWorkflowInvocation, "workflow invocation");
export const requireADKWorkflowTriggerSave = (
  value: unknown,
): ADKWorkflowTriggerSaveResult =>
  requireValue(value, isADKWorkflowTriggerSaveResult, "workflow trigger save");
