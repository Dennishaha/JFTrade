import type { MCPServerStatus } from "../../contracts/wire/settings";

export type ADKPermissionMode = "approval" | "less_approval" | "all";
export type ADKWorkMode = "chat" | "loop";
export type ADKProviderAPIProtocol = "chat_completions" | "responses";

export interface ADKProvider {
  id: string;
  displayName: string;
  baseUrl: string;
  model: string;
  apiProtocol: ADKProviderAPIProtocol;
  contextWindowTokens?: number;
  requestTimeoutMs: number;
  defaultHeaders?: Record<string, string>;
  enabled: boolean;
  default: boolean;
  hasApiKey: boolean;
  capabilities?: Record<string, boolean>;
  createdAt: string;
  updatedAt: string;
}

export type MCPServerAuthMode = "token" | "none";

export interface MCPServerSettings {
  enabled: boolean;
  port: number;
  authMode: MCPServerAuthMode;
  tokenConfigured: boolean;
}

export interface MCPServerSettingsSnapshot {
  settings: MCPServerSettings;
  status: MCPServerStatus;
}

export interface MCPServerTokenResetResult extends MCPServerSettingsSnapshot {
  token: string;
}

export type RuntimeDependencyStatus =
  | "ok"
  | "missing"
  | "outdated"
  | "error"
  | string;

export interface RuntimeDependencyItem {
  id: string;
  displayName: string;
  required: boolean;
  configurable: boolean;
  status: RuntimeDependencyStatus;
  minimumVersion: string;
  detectedVersion: string;
  configuredPath: string;
  effectivePath: string;
  resolvedPath: string;
  source: string;
  homepageUrl: string;
  message: string;
}

export interface RuntimeDependenciesResponse {
  checkedAt: string;
  allRequiredSatisfied: boolean;
  dependencies: RuntimeDependencyItem[];
}

export interface ADKAgent {
  id: string;
  name: string;
  instruction: string;
  providerId: string;
  model: string;
  tools: string[];
  skills: string[];
  permissionMode: ADKPermissionMode;
  memoryEnabled: boolean;
  recentUserWindow: number;
  workMode: ADKWorkMode;
  loopMaxIterations: number;
  status: "ENABLED" | "DISABLED" | string;
  builtin?: boolean;
  createdAt: string;
  updatedAt: string;
  deletedAt?: string;
}

export interface ADKToolDescriptor {
  name: string;
  displayName: string;
  description: string;
  category: string;
  permission: string;
  allowedModes: ADKPermissionMode[];
  requiresApprovalIn: ADKPermissionMode[];
  inputSchema?: Record<string, unknown>;
  outputSummary?: string;
  riskLevel?: "low" | "medium" | "high" | "critical" | string;
  requiredSkill?: string;
  requiredSkills?: string[];
}

export interface ADKSkill {
  id: string;
  displayName: string;
  description: string;
  source: string;
  installPath: string;
  version?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ADKToolCall {
  id: string;
  runId: string;
  toolName: string;
  permission: string;
  status: string;
  input?: Record<string, unknown>;
  output?: unknown;
  error?: string | null;
  requiresUser: boolean;
  idempotencyKey?: string;
  createdAt: string;
  startedAt?: string;
  updatedAt: string;
  completedAt?: string;
  durationMs?: number;
}

export interface ADKArtifactRef {
  name: string;
  version: number;
  uri: string;
  mimeType: string;
  truncated: true;
}

export interface ADKApproval {
  id: string;
  runId: string;
  agentId: string;
  toolName: string;
  input?: Record<string, unknown>;
  status: "PENDING" | "APPROVED" | "DENIED" | string;
  reason: string;
  functionCallId?: string;
  confirmationCallId?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ADKInputOption {
  id: string;
  label: string;
  description?: string;
  recommended?: boolean;
}

export interface ADKInputQuestion {
  id: string;
  question: string;
  options: ADKInputOption[];
  allowOther: boolean;
}

export interface ADKInputAnswer {
  questionId: string;
  optionId?: string;
  otherText?: string;
}

export interface ADKInputRequest {
  id: string;
  runId: string;
  agentId: string;
  functionCallId: string;
  title?: string;
  status: "PENDING" | "ANSWERED" | "CANCELLED" | string;
  questions: ADKInputQuestion[];
  answers?: ADKInputAnswer[];
  createdAt: string;
  updatedAt: string;
  answeredAt?: string;
}

export interface ADKSession {
  id: string;
  agentId: string;
  title: string;
  workflowId?: string;
  workflowName?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ADKSessionComposerState {
  sessionId: string;
  chatDraft: string;
  providerIdOverride: string;
  modelOverride: string;
  workModeOverride: ADKWorkMode | "" | string;
  permissionModeOverride: ADKPermissionMode | "" | string;
  goalObjectiveDraft: string;
  goalObjectiveTouched: boolean;
  updatedAt?: string;
}

export interface ADKSessionContextSnapshot {
  sessionId: string;
  contextRevisionId?: string;
  previousContextRevisionId?: string;
  contextRevisionCreatedAt?: string;
  currentInputTokens: number;
  projectedNextTurnTokens: number;
  estimatedInputTokens?: number;
  rawCurrentInputTokens?: number;
  rawProjectedNextTurnTokens?: number;
  contextWindowTokens: number;
  usageRatio: number;
  status:
    | "unknown"
    | "healthy"
    | "warning"
    | "near_limit"
    | "critical"
    | string;
  recentUserWindow: number;
  retainedRecentUserCount: number;
  protectedRecentCount?: number;
  activeHandoffCount: number;
  latestHandoffPreview?: string;
  summaryPreview?: string;
  rawEventCount?: number;
  compactedEventCount?: number;
  summaryBoundaryEventIndex?: number;
  breakdown: {
    instructionTokens: number;
    handoffTokens: number;
    recentUserTokens: number;
    protectedTailTokens: number;
    otherVisibleTokens: number;
    pendingUserTokens: number;
    toolDeclarationTokens: number;
  };
  rawBreakdown?: {
    instructionTokens: number;
    handoffTokens: number;
    recentUserTokens: number;
    protectedTailTokens: number;
    otherVisibleTokens: number;
    pendingUserTokens: number;
    toolDeclarationTokens: number;
  };
  trimmedToolResponseCount?: number;
  lastCompactedAt?: string;
  lastCompactionMode?: "manual" | "auto" | "aggressive" | string;
  lastCompactionReason?: string;
  autoCompacted: boolean;
  degradedSummary: boolean;
}

export interface ADKTranscriptEntry {
  id: string;
  sessionId: string;
  runId?: string;
  role: "user" | "assistant" | string;
  kind: string;
  content: string;
  reasoningContent?: string;
  createdAt: string;
}

export type ADKMessage = ADKTranscriptEntry;

export type ADKTimelineEntryKind =
  | "user_message"
  | "assistant_reasoning"
  | "tool_group"
  | "approval_group"
  | "input_request"
  | "context_notice"
  | "assistant_message";

export type ADKTimelineEntryStatus = "streaming" | "final" | string;

export interface ADKTimelineEntry {
  id: string;
  sessionId: string;
  runId?: string;
  kind: ADKTimelineEntryKind | string;
  createdAt: string;
  updatedAt?: string;
  sequence: number;
  status?: ADKTimelineEntryStatus;
  text?: string;
  originalText?: string;
  processedText?: string;
  toolCalls?: ADKToolCall[];
  approvals?: ADKApproval[];
  inputRequest?: ADKInputRequest;
}

export interface ADKRunUsage {
  modelCalls?: number;
  toolCallsTotal?: number;
  durationMs?: number;
  tokensIn?: number;
  tokensOut?: number;
}

export interface ADKRun {
  id: string;
  sessionId: string;
  agentId: string;
  providerId?: string;
  providerName?: string;
  model?: string;
  maxDurationMs?: number;
  status: string;
  message: string;
  userMessage?: string;
  preToolContent?: string;
  preToolReasoning?: string;
  toolSummaries?: string[];
  failureReason?: string;
  errorCode?: string;
  degraded?: boolean;
  optimizationTaskId?: string;
  workMode?: ADKWorkMode | string;
  permissionMode?: ADKPermissionMode | string;
  objective?: string;
  parentRunId?: string;
  childRunIds?: string[];
  iteration?: number;
  workflowStatus?: string;
  workflowEngine?: string;
  workflowCursor?: number;
  workflowPlan?: ADKWorkflowStepState[];
  toolCalls: ADKToolCall[];
  pendingApprovals: ADKApproval[];
  inputRequest?: ADKInputRequest;
  inputRequests?: ADKInputRequest[];
  resumeState?: string;
  pauseRequestedAt?: string;
  pausedAt?: string;
  pausedReason?: string;
  finalMessageId?: string;
  usage?: ADKRunUsage;
  createdAt: string;
  startedAt?: string;
  updatedAt: string;
  completedAt?: string;
  cancelledAt?: string;
}

export interface ADKWorkflowStepState {
  taskId?: string;
  title: string;
  description?: string;
  message?: string;
  status: string;
  childRunId?: string;
  childProviderId?: string;
  childModel?: string;
  dependsOn?: string[];
  iteration?: number;
  order?: number;
  modeHint?: string;
  agentRole?: string;
  plannerStepId?: string;
  planSource?: string;
  workflowMode?: string;
  objective?: string;
  executor?: string;
  resultSummary?: string;
  plannerWarnings?: string[];
  nodeName?: string;
  nodeStatus?: string;
  routes?: string[];
  outputSummary?: string;
}

export type ADKWorkflowStatus = "ENABLED" | "DISABLED" | string;
export type ADKWorkflowTriggerType =
  | "manual"
  | "schedule"
  | "webhook"
  | "event"
  | "market_threshold"
  | string;
export type ADKWorkflowTriggerStatus = "ENABLED" | "DISABLED" | "ERROR" | string;
export type ADKWorkflowTriggerLogStatus =
  | "QUEUED"
  | "RUNNING"
  | "SUCCEEDED"
  | "PENDING_APPROVAL"
  | "FAILED"
  | "CANCELLED"
  | "SKIPPED"
  | string;

export interface ADKWorkflowCanvasPoint {
  x: number;
  y: number;
}

export interface ADKWorkflowCanvasNode {
  id: string;
  type: string;
  position: ADKWorkflowCanvasPoint;
  data?: Record<string, unknown>;
}

export interface ADKWorkflowCanvasEdge {
  id: string;
  source: string;
  target: string;
  sourceHandle?: string;
  targetHandle?: string;
  type?: string;
  data?: Record<string, unknown>;
}

export interface ADKWorkflowCanvasGraph {
  version?: string;
  nodes?: ADKWorkflowCanvasNode[];
  edges?: ADKWorkflowCanvasEdge[];
  viewport?: Record<string, unknown>;
}

export interface ADKWorkflowDefinition {
  id: string;
  name: string;
  description?: string;
  status: ADKWorkflowStatus;
  agentId: string;
  workMode: ADKWorkMode | string;
  providerId?: string;
  model?: string;
  permissionMode?: ADKPermissionMode | string;
  promptTemplate: string;
  objectiveTemplate?: string;
  defaultInputs?: Record<string, unknown>;
  canvasGraph?: ADKWorkflowCanvasGraph;
  tags?: string[];
  builtinTemplate?: boolean;
  createdAt: string;
  updatedAt: string;
  deletedAt?: string;
}

export interface ADKWorkflowDefinitionWriteRequest {
  id?: string;
  name: string;
  description?: string;
  status?: ADKWorkflowStatus;
  agentId: string;
  workMode?: ADKWorkMode | string;
  providerId?: string;
  model?: string;
  permissionMode?: ADKPermissionMode | string;
  promptTemplate: string;
  objectiveTemplate?: string;
  defaultInputs?: Record<string, unknown>;
  canvasGraph?: ADKWorkflowCanvasGraph;
  tags?: string[];
}

export interface ADKWorkflowTrigger {
  id: string;
  workflowId: string;
  type: ADKWorkflowTriggerType;
  title: string;
  status: ADKWorkflowTriggerStatus;
  config?: Record<string, unknown>;
  hasSecret?: boolean;
  nextRunAt?: string;
  lastRunAt?: string;
  lastRunId?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
  deletedAt?: string;
}

export interface ADKWorkflowTriggerWriteRequest {
  id?: string;
  type: ADKWorkflowTriggerType;
  title?: string;
  status?: ADKWorkflowTriggerStatus;
  config?: Record<string, unknown>;
  resetSecret?: boolean;
}

export interface ADKWorkflowTriggerLog {
  id: string;
  workflowId: string;
  triggerId?: string;
  triggerType: ADKWorkflowTriggerType;
  status: ADKWorkflowTriggerLogStatus;
  runId?: string;
  sessionId?: string;
  inputs?: Record<string, unknown>;
  matchedEvent?: Record<string, unknown>;
  result?: ADKWorkflowResult;
  nodeRuns?: ADKWorkflowNodeRun[];
  error?: string;
  startedAt?: string;
  finishedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ADKWorkflowResult {
  format?: string;
  markdown?: string;
  json?: Record<string, unknown>;
  rawResponse?: ADKChatResponse;
}

export interface ADKWorkflowNodeRun {
  nodeId: string;
  nodeType: string;
  title?: string;
  status: ADKWorkflowTriggerLogStatus;
  startedAt?: string;
  finishedAt?: string;
  inputs?: Record<string, unknown>;
  outputs?: Record<string, unknown>;
  error?: string;
}

export interface ADKChatResponse {
  reply: string;
  reasoningContent?: string;
  session: ADKSession;
  run: ADKRun;
  pendingApprovals: ADKApproval[];
  inputRequest?: ADKInputRequest;
  timeline: ADKTimelineEntry[];
  context?: ADKSessionContextSnapshot;
}

export interface ADKWorkflowTriggerSaveResult {
  trigger: ADKWorkflowTrigger;
  secret?: string;
}

export interface ADKWorkflowInvocationResult {
  workflow: ADKWorkflowDefinition;
  trigger?: ADKWorkflowTrigger;
  log: ADKWorkflowTriggerLog;
  response?: ADKChatResponse;
}

export interface ADKApprovalResolution {
  approval: ADKApproval;
  run?: ADKRun;
  parentRun?: ADKRun;
  message?: ADKMessage;
}

export interface ADKInputResolution {
  request: ADKInputRequest;
  run?: ADKRun;
  parentRun?: ADKRun;
  message?: ADKMessage;
}

export interface ADKAuditEvent {
  id: string;
  kind: string;
  subjectId?: string;
  detail: string;
  metadata?: Record<string, unknown>;
  createdAt: string;
}

export interface ADKOptimizationRun {
  definitionId: string;
  runId: string;
  status: string;
  result?: unknown;
}

export interface ADKOptimizationTask {
  id: string;
  status: string;
  objective: string;
  runs: ADKOptimizationRun[];
  progress: {
    total: number;
    running: number;
    completed: number;
    failed: number;
    cancelled: number;
  };
  createdAt: string;
  updatedAt: string;
}

export interface ADKTask {
  id: string;
  title: string;
  description?: string;
  status: string;
  agentId?: string;
  runId?: string;
  dependsOn?: string[];
  order?: number;
  modeHint?: string;
  agentRole?: string;
  plannerStepId?: string;
  planSource?: string;
  workflowMode?: string;
  objective?: string;
  message?: string;
  executor?: string;
  childProviderId?: string;
  childModel?: string;
  resultSummary?: string;
  plannerWarnings?: string[];
  createdAt: string;
  updatedAt: string;
}

export interface ADKTaskFilters {
  status?: string;
  agentId?: string;
  runId?: string;
  limit?: number;
  offset?: number;
}

export interface ADKTaskPatch {
  title?: string;
  description?: string;
  status?: string;
  agentId?: string;
  runId?: string;
  dependsOn?: string[];
  order?: number;
  modeHint?: string;
  agentRole?: string;
  plannerStepId?: string;
  planSource?: string;
  workflowMode?: string;
  objective?: string;
  message?: string;
  executor?: string;
  childProviderId?: string;
  childModel?: string;
  resultSummary?: string;
  plannerWarnings?: string[];
}

export interface ADKMemoryEntry {
  id: string;
  agentId?: string;
  key: string;
  value: string;
  scope: string;
  createdAt: string;
  updatedAt: string;
}

export interface ADKMemoryFilters {
  scope?: string;
  agentId?: string;
  key?: string;
}
