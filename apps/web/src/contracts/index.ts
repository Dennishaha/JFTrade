export interface ArchitectureCard {
  title: string;
  owner: string;
  status: string;
  summary: string;
  bullets: string[];
}

export interface RoadmapPhase {
  key: string;
  title: string;
  target: string;
  summary: string;
}

export interface ConsolePanel {
  name: string;
  state: string;
  description: string;
}

export interface ApiSuccessEnvelope<T> {
  ok: true;
  data: T;
  timestamp: string;
}

export interface ApiErrorEnvelope {
  ok: false;
  error: {
    code: string;
    message: string;
    details?: unknown;
  };
  timestamp: string;
}

export interface WatchlistGroup {
  id: string;
  name: string;
  isDefault?: boolean;
  protected?: boolean;
  revision: number;
  itemCount?: number;
  createdAt?: string;
  updatedAt?: string;
}

export interface WatchlistItemSource {
  sourceId: string;
  sourceName?: string;
  remoteGroupName?: string;
  importedAt?: string;
}

export interface WatchlistItem {
  instrumentId: string;
  market: string;
  symbol: string;
  name?: string;
  securityType?: string;
  groupIds?: string[];
  groupNames?: string[];
  sources?: WatchlistItemSource[];
  createdAt?: string;
  updatedAt?: string;
}

export interface WatchlistItemsPage {
  items: WatchlistItem[];
  nextCursor?: string | null;
  total?: number;
}

export interface WatchlistMembership {
  instrumentId: string;
  groupIds: string[];
  groups?: Array<{ id: string; name: string }>;
  revision: number;
}

export interface WatchlistMembershipUpdate {
  groupIds: string[];
  newGroupNames: string[];
  expectedRevision: number;
}

export interface WatchlistExtendedQuote {
  price?: number;
  change?: number;
  changePercent?: number;
  observedAt?: string;
}

export interface WatchlistQuote {
  instrumentId: string;
  name?: string;
  securityType?: string;
  price?: number;
  previousClose?: number;
  change?: number;
  changePercent?: number;
  session?: string;
  observedAt?: string;
  updateTime?: string;
  source?: string;
  preMarket?: WatchlistExtendedQuote;
  afterHours?: WatchlistExtendedQuote;
  overnight?: WatchlistExtendedQuote;
}

export interface WatchlistQuoteError {
  instrumentId: string;
  code?: string;
  message: string;
}

export interface WatchlistQuoteBatch {
  quotes: WatchlistQuote[];
  errors: WatchlistQuoteError[];
  observedAt: string;
}

export interface WatchlistSource {
  id: string;
  broker?: string;
  displayName: string;
  available: boolean;
  status?: string;
  message?: string;
}

export interface WatchlistRemoteGroup {
  remoteGroupId: string;
  name: string;
  type?: string;
  system?: boolean;
  ambiguous?: boolean;
  memberCount?: number;
}

export interface WatchlistBinding {
  id: string;
  sourceId: string;
  remoteGroupId: string;
  remoteGroupName: string;
  localGroupId: string;
  localGroupName?: string;
  lastImportedAt?: string;
}

export interface WatchlistImportDiffItem {
  instrumentId: string;
  name?: string;
  selected?: boolean;
}

export interface WatchlistImportPreviewRequest {
  sourceId: string;
  remoteGroupId: string;
  remoteGroupName?: string;
  localGroupId?: string;
  newGroupName?: string;
}

export interface WatchlistImportPreview {
  id: string;
  sourceId: string;
  remoteGroupName: string;
  localGroupId?: string;
  localGroupName?: string;
  added: WatchlistImportDiffItem[];
  unchanged: WatchlistImportDiffItem[];
  localOnly: WatchlistImportDiffItem[];
  expiresAt?: string;
  remoteHash?: string;
  localRevision?: number;
}

export interface WatchlistImportCommitRequest {
  deleteLocalOnlyInstrumentIds: string[];
}

export interface WatchlistImportRun {
  id: string;
  previewId?: string;
  sourceId: string;
  remoteGroupName?: string;
  localGroupId?: string;
  localGroupName?: string;
  addedCount?: number;
  deletedCount?: number;
  unchangedCount?: number;
  status: string;
  createdAt?: string;
  completedAt?: string;
}

export interface MarketTradingWindowDto {
  startMinute: number;
  endMinute: number;
  label: string;
}

export interface MarketPrecisionDto {
  price: number;
  quote: number;
}

export interface MarketProfileDto {
  code: string;
  resolvedMarket: string;
  preferredPrefix: string;
  displayName: string;
  quoteCurrency: string;
  timezone: string;
  supportsExtendedHours: boolean;
  requiresExchangePrefix: boolean;
  aliases: string[];
  regularSessions: MarketTradingWindowDto[];
  precision: MarketPrecisionDto;
  tickSize: number;
}

export interface MarketProfilesResponse {
  markets: MarketProfileDto[];
  defaultMarket: string;
  updatedAt: string;
}

export interface NormalizeInstrumentRequest {
  market?: string;
  symbol?: string;
  code?: string;
  instrumentId?: string;
}

export interface NormalizeInstrumentResponse {
  market: string;
  prefix: string;
  code: string;
  symbol: string;
  instrumentId: string;
  resolvedMarket: string;
}

export type ADKPermissionMode = "approval" | "less_approval" | "all";
export type ADKWorkMode = "chat" | "loop";

export interface ADKProvider {
  id: string;
  displayName: string;
  baseUrl: string;
  model: string;
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

export interface ADKRuntimeSettings {
  runTimeoutMs: number;
  streamIdleTimeoutMs: number;
}

export type MCPServerAuthMode = "token" | "none";

export interface MCPServerSettings {
  enabled: boolean;
  port: number;
  authMode: MCPServerAuthMode;
  tokenConfigured: boolean;
}

export interface MCPServerStatus {
  running: boolean;
  endpoint: string;
  lastError?: string;
}

export interface MCPServerSettingsSnapshot {
  settings: MCPServerSettings;
  status: MCPServerStatus;
}

export interface MCPServerTokenResetResult extends MCPServerSettingsSnapshot {
  token: string;
}

export interface PineWorkerSettingsResponse {
  backtestWorkerLimit: number;
  instanceWorkerLimit: number;
  nodeBinaryPath: string;
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

export interface HealthResponse {
  service: {
    service: string;
    status: string;
    checkedAt: string;
  };
  persistence: {
    engine: string;
    databasePath: string;
    status: string;
    migrated: boolean;
    pendingMigrations: string[];
    tables: string[];
    checkedAt: string;
  };
}

export type BrokerReadFeatureKey =
  | "funds"
  | "positions"
  | "orders"
  | "fills"
  | "cashFlows"
  | "orderFees"
  | "marginRatios"
  | "maxTradeQuantity"
  | "orderBook";

export interface BrokerReadFeatureCapability {
  supportedEnvironments: string[];
  supportsHistory?: boolean;
  requiresSymbols?: boolean;
  requiresClearingDate?: boolean;
  requiresPrice?: boolean;
  requiresOrderIdEx?: boolean;
  requiresSymbol?: boolean;
  requiresPassword?: boolean;
  // orderBook specific
  defaultNum?: number;
  minNum?: number;
  maxNum?: number;
  numPresets?: number[];
  supportsRealTimePush?: boolean;
}

export interface BrokerMarketCapability {
  market: string;
  supportsQuote: boolean;
  supportsTrade: boolean;
  readFeatures: Record<BrokerReadFeatureKey, BrokerReadFeatureCapability>;
}

export interface BrokerDescriptor {
  id: string;
  displayName: string;
  environments: string[];
  capabilities: BrokerMarketCapability[];
  notes: string[];
}

export type ObservabilityImportance = "low" | "normal" | "high" | "critical";

export interface ObservabilityEvent {
  at: string;
  level: string;
  importance: ObservabilityImportance;
  message: string;
  error?: string;
  method?: string;
  path?: string;
  operation?: string;
  status?: number;
  latencyMs?: number;
  requestId?: string;
  sessionId?: string;
  runId?: string;
  taskId?: string;
  brokerId?: string;
  accountId?: string;
  instrumentId?: string;
  providerId?: string;
  source?: string;
}

export interface RequestObservabilitySummary {
  recentErrors: ObservabilityEvent[];
  recentSlowRequests: ObservabilityEvent[];
  slowThresholdMs: number;
  minimumImportance: ObservabilityImportance;
  openD: {
    totalCalls: number;
    failedCalls: number;
    lastCallAt?: string;
    lastSuccessAt?: string;
    lastErrorAt?: string;
    lastError?: string;
    lastOperation?: string;
    lastRequestId?: string;
  };
}

export interface SystemStatusResponse {
  name: string;
  apiPort: number;
  defaultBroker: string;
  defaultTradingEnvironment: string;
  realTradingEnabled: boolean;
  realTradingKillSwitch: {
    active: boolean;
    runtimeActive: boolean;
    blockedOperations: string[];
    allowsCancel: boolean;
  };
  realTradingRisk: {
    enabled: boolean;
    maxOrderQuantity: number | null;
    maxOrderNotional: number | null;
    runtimeConfiguredMaxOrderQuantity: number | null;
    runtimeConfiguredMaxOrderNotional: number | null;
    runtimeRiskConfigured: boolean;
  };
  realTradeAccess?: {
    approverAllowlistEnabled: boolean;
    approverCount: number;
    adminAllowlistEnabled: boolean;
    adminCount: number;
  };
  broker: BrokerDescriptor;
  persistence: {
    engine: string;
    databasePath: string;
    status: string;
    migrated: boolean;
    pendingMigrations: string[];
    tables: string[];
    checkedAt: string;
  };
  strategyRuntime: {
    status: string;
    activeStrategies: number;
    supportsBacktestParity: boolean;
    activeInstances?: StrategyRuntimeActiveInstanceSummary[];
  };
  runtimeResources?: RuntimeResourcesSummary;
  observability: {
    requests: RequestObservabilitySummary;
  };
  message: string;
}

export interface RuntimeResourceDescriptor {
  id: string;
  owner: string;
  kind: string;
  path: string;
  initializedBy: string;
  schemaOwner: string;
  closeOwner: string;
  healthProvider: string;
  environmentOverride?: string;
  critical: boolean;
}

export interface RuntimeResourcesSummary {
  checkedAt: string;
  count: number;
  items: RuntimeResourceDescriptor[];
}

export interface StorageOverviewResponse {
  pendingOutbox: Array<{
    id: string;
    topic: string;
    status: string;
    availableAt: string;
    createdAt: string;
  }>;
  recentJobs: Array<{
    id: string;
    queue: string;
    kind: string;
    status: string;
    scheduledAt: string;
    updatedAt: string;
  }>;
  recentAuditLogs: Array<{
    id: string;
    action: string;
    targetType: string;
    targetId: string;
    createdAt: string;
  }>;
  recentExecutionCommands: Array<{
    id: string;
    brokerId: string;
    operation: string;
    idempotencyKey: string;
    actorType: string;
    actorId: string;
    internalOrderId: string | null;
    completedAt: string | null;
    createdAt: string;
  }>;
}

export interface FutuBrokerIntegrationConfig {
  type: "futu";
  host: string;
  apiPort: number;
  websocketPort: number;
  maxWebSocketConnections: number;
  useEncryption: boolean;
  websocketKey: string;
  tradeMarket: string;
  securityFirm: string;
}

export type BrokerIntegrationConfig = FutuBrokerIntegrationConfig;

export interface BrokerSettingsResponse {
  brokers: Array<{
    descriptor: BrokerDescriptor;
    integration: {
      brokerId: string;
      enabled: boolean;
      config: BrokerIntegrationConfig;
      updatedAt: string;
      createdAt: string;
    } | null;
    defaults: BrokerIntegrationConfig | null;
  }>;
  accounts: Array<{
    id: string;
    brokerId: string;
    accountId: string;
    displayName: string;
    tradingEnvironment: string;
    market: string;
    securityFirm: string | null;
    enabled: boolean;
    updatedAt: string;
    createdAt: string;
  }>;
}

export interface ExecutionSettingsResponse {
  defaultTradingEnvironment: string;
  brokerOrderHistoryLookbackDays: number;
  seenFillRetentionDays: number;
}

export interface OnboardingReason {
  code:
    | "BROKER_DISCONNECTED"
    | "QUOTE_NOT_LOGGED_IN"
    | "TRADE_NOT_LOGGED_IN"
    | "NO_MANAGED_ACCOUNTS"
    | string;
  severity: "info" | "warning" | "error";
  message: string;
}

export interface OnboardingStateResponse {
  state: {
    completed: boolean;
    completedAt?: string;
    dismissedAt?: string;
    lastBrokerId: string;
  };
  shouldShowOobe: boolean;
  reasons: OnboardingReason[];
  recommendedBrokerId: string;
  brokers: Array<{
    descriptor: BrokerDescriptor;
    enabled: boolean;
    available: boolean;
    configured: boolean;
  }>;
}

export interface UIColorPreferencesResponse {
  appearance: {
    upColor: string;
    downColor: string;
  };
}

export interface SecuritySettingsResponse {
  webAccessEnabled: boolean;
  publicAccessEnabled: boolean;
  webPort: number;
  passwordConfigured: boolean;
}

export type PluginInstallStatus =
  | "NOT_INSTALLED"
  | "INSTALLING"
  | "INSTALLED"
  | "FAILED";

export type PluginOperationStatus =
  | "QUEUED"
  | "RUNNING"
  | "SUCCEEDED"
  | "FAILED";

export interface PluginDescriptorDto {
  id: string;
  type: string;
  displayName: string;
  version: string;
  description: string;
  keywords: string[];
}

export interface PluginOperationDto {
  operationId: string;
  pluginId: string;
  status: PluginOperationStatus;
  phase: string;
  progress: number;
  message: string;
  targetDir: string;
  installPath: string;
  startedAt: string;
  updatedAt: string;
  completedAt: string | null;
  error: string | null;
}

export interface PluginUninstallGuidanceDto {
  pluginId: string;
  path: string;
  exists: boolean;
  commands: {
    posix: string;
    powershell: string;
  };
}

export interface PluginBuildTupleDto {
  jftradeVersion: string;
  goVersion: string;
  goos: string;
  goarch: string;
  buildMode: string;
  buildTags?: string[];
}

export interface PluginCompatibilityDto {
  mode: string;
  supported: boolean;
  requiresRebuild: boolean;
  reason?: string | null;
  host: PluginBuildTupleDto;
  artifact?: PluginBuildTupleDto | null;
}

export interface PluginInstallationDto {
  status: PluginInstallStatus;
  installed: boolean;
  installPath: string;
  targetDir: string;
  markerPath: string;
  currentOperation: PluginOperationDto | null;
  lastOperation: PluginOperationDto | null;
  uninstallGuidance: PluginUninstallGuidanceDto;
}

export interface PluginCatalogResponse {
  targetDir: string;
  plugins: Array<{
    descriptor: PluginDescriptorDto;
    installation: PluginInstallationDto;
    compatibility?: PluginCompatibilityDto;
  }>;
}

export interface StrategyVisualNodeDocument {
  id: string;
  type: string;
  x: number;
  y: number;
  text: string;
  properties: Record<string, unknown>;
}

export interface StrategyVisualEdgeDocument {
  id?: string | undefined;
  type: string;
  sourceNodeId: string;
  targetNodeId: string;
  text?: string | undefined;
  properties?: Record<string, unknown> | undefined;
}

export interface StrategyVisualModelDocument {
  engine: string;
  version: number;
  nodes: StrategyVisualNodeDocument[];
  edges: StrategyVisualEdgeDocument[];
}

export type PineV6WorkflowBlockKind =
  | "series_assign"
  | "var_state"
  | "if"
  | "request_security"
  | "array_op"
  | "strategy_entry"
  | "strategy_exit"
  | "strategy_order"
  | "strategy_close"
  | "strategy_close_all"
  | "strategy_cancel"
  | "strategy_cancel_all"
  | "strategy_risk_allow_entry_in"
  | "strategy_risk_max_drawdown"
  | "strategy_risk_max_intraday_loss"
  | "strategy_risk_max_intraday_filled_orders"
  | "strategy_risk_max_position_size"
  | "strategy_risk_max_cons_loss_days"
  | "plot"
  | "alertcondition"
  | "log";

export interface PineV6WorkflowDeclaration {
  title: string;
  overlay: boolean;
  initialCapital?: number | null;
  currency?: string | null;
  pyramiding?: number | null;
  defaultQtyType?: string | null;
  defaultQtyValue?: number | null;
  calcOnEveryTick?: boolean | null;
  processOrdersOnClose?: boolean | null;
}

export interface PineV6WorkflowInput {
  id: string;
  name: string;
  title: string;
  type: "int" | "float" | "bool" | "string" | "source" | "time" | "timeframe" | "color";
  defaultValue: string;
}

export interface PineV6WorkflowRuntimeBindingDraft {
  market: string;
  code: string;
  interval: string;
  executionMode: StrategyExecutionMode;
  useExtendedHours: boolean;
  brokerAccountKey?: string;
  runtimeRisk?: StrategyRuntimeRiskSettings;
}

export interface PineV6WorkflowBlock {
  id: string;
  kind: PineV6WorkflowBlockKind;
  enabled: boolean;
  title: string;
  params: Record<string, unknown>;
  thenBlocks?: PineV6WorkflowBlock[];
  elseBlocks?: PineV6WorkflowBlock[];
}

export interface PineV6WorkflowDocument {
  engine: "pine-v6-workflow";
  version: number;
  declaration: PineV6WorkflowDeclaration;
  inputs: PineV6WorkflowInput[];
  blocks: PineV6WorkflowBlock[];
  runtimeBindingDraft: PineV6WorkflowRuntimeBindingDraft;
}

export type StrategySourceFormat = "pine-v6";

export type StrategyInstanceStatus = "RUNNING" | "PAUSED" | "STOPPED";

export type StrategyExecutionMode = "live" | "notify_only";

export type StrategyRuntimeRiskMode = "off" | "monitor" | "enforce";

export interface StrategyDefinitionSummaryDocument {
  strategyId: string;
  name: string;
  version: string;
}

export interface StrategyBrokerAccountBinding {
  brokerId: string;
  accountId: string;
  tradingEnvironment: string;
  market: string;
}

export interface StrategyBindingInstrumentDocument {
  market: string;
  code: string;
}

export interface StrategyInstanceBindingDocument {
  instruments?: StrategyBindingInstrumentDocument[];
  symbols: string[];
  interval: string;
  executionMode: StrategyExecutionMode;
  brokerAccount?: StrategyBrokerAccountBinding | null;
  runtimeRisk: StrategyRuntimeRiskSettings;
}

export interface StrategyRuntimeRiskSettings {
  mode: StrategyRuntimeRiskMode;
  closeOnly: boolean;
  maxOrderQuantity?: number | null;
  maxOrderNotional?: number | null;
  dailyMaxOrders?: number | null;
  pauseOnReject: boolean;
}

export interface StrategyRuntimeObservation {
  actualStatus: StrategyInstanceStatus;
  activeSymbols: string[];
  lastClosedKlineAt?: string | null;
  lastSignalAt?: string | null;
  lastOrderAt?: string | null;
  lastErrorAt?: string | null;
  lastError?: string | null;
  updatedAt?: string | null;
}

export interface StrategyRuntimeActiveInstanceSummary extends StrategyRuntimeObservation {
  instanceId: string;
  definitionName: string;
}

export interface StrategyDefinitionSyncStatus {
  definitionId: string;
  appliedVersion: string;
  latestVersion: string;
  isLatest: boolean;
  canApplyLatest: boolean;
  blockedReason?: string | null;
}

export interface StrategyApplyLinkedInstancesResponse {
  definitionId: string;
  latestVersion: string;
  totalLinked: number;
  applied: string[];
  alreadyLatest: string[];
  skippedBusy: string[];
}

export interface StrategyActivityPage {
  limit: number;
  offset: number;
  total: number;
  returned: number;
  hasMore: boolean;
}

export interface StrategyLogListResponse {
  instanceId: string;
  logs: string[];
  page: StrategyActivityPage;
}

export interface StrategyAuditEntryDocument {
  instanceId: string;
  kind: string;
  detail?: string;
  at: string;
}

export interface StrategyAuditListResponse {
  instanceId: string;
  entries: StrategyAuditEntryDocument[];
  page: StrategyActivityPage;
}

export interface StrategyInstanceItem {
  id: string;
  pluginId?: string;
  definition: StrategyDefinitionSummaryDocument;
  runtime: string;
  sourceFormat: StrategySourceFormat;
  startable: boolean;
  binding?: StrategyInstanceBindingDocument;
  params: Record<string, unknown>;
  status: StrategyInstanceStatus;
  createdAt: string;
  logs: string[];
  definitionSync?: StrategyDefinitionSyncStatus | null;
  runtimeObservation?: StrategyRuntimeObservation | null;
}

export interface StrategyDefinitionDocument {
  id: string;
  name: string;
  version: string;
  description: string;
  runtime: string;
  sourceFormat?: StrategySourceFormat;
  symbol?: string;
  interval?: string;
  script: string;
  visualModel?: PineV6WorkflowDocument | StrategyVisualModelDocument | null;
  createdAt: string;
  updatedAt: string;
  derivedWarmupBars?: number;
  derivedWarmupInterval?: string;
}

export interface PluginInstallResponse {
  operation: PluginOperationDto;
}

export type FutuOpenDInstallOptionId = "gui" | "command-line";

export interface FutuOpenDInstallOptionDto {
  id: FutuOpenDInstallOptionId;
  label: string;
  description: string;
  url: string;
  recommended: boolean;
}

export interface FutuOpenDInstallGuideResponse {
  brokerId: "futu";
  title: string;
  description: string;
  options: FutuOpenDInstallOptionDto[];
  nextSteps: string[];
  settings: {
    host: string;
    apiPort: number;
    websocketPort: number;
    maxWebSocketConnections: number;
    useEncryption: boolean;
    websocketKeyRequired: boolean;
    minimumVersion: string;
  };
}

export type FutuOpenDIssueCode =
  | "NONE"
  | "LOGIN_TIMEOUT"
  | "CONNECTION_LIMIT"
  | "PROTOCOL_PARSE_ERROR"
  | "WS_POOL_EXHAUSTED"
  | "WEBSOCKET_AUTH"
  | "OPEND_VERSION_UNSUPPORTED"
  | "OPEND_API_CONNECTIVITY";

export interface FutuOpenDHealthResponse {
  checkedAt: string;
  status: "healthy" | "degraded" | "offline";
  runtime: {
    connectivity: "connected" | "degraded" | "disconnected";
    host: string;
    port: number;
    useEncryption: boolean;
    websocketKeyConfigured: boolean;
    quoteLoggedIn: boolean | null;
    tradeLoggedIn: boolean | null;
    programStatus: string | null;
    serverVersion: string | null;
    minimumVersion: string;
    lastError: string | null;
  };
  diagnosis: {
    code: FutuOpenDIssueCode;
    summary: string | null;
    manualRetryRequired: boolean;
    restartOpenDRecommended: boolean;
  };
  localSocketDiagnostics: {
    websocketEstablishedConnections: number;
    likelyConnectionSaturation: boolean;
    topClientProcesses: Array<{
      processName: string;
      pid: number;
      establishedConnections: number;
    }>;
  };
  localInstallation: {
    platform: string;
    installed: boolean;
    version: string | null;
    installPath: string | null;
    guiDetected: boolean;
    process: {
      running: boolean;
      pid: number | null;
      executablePath: string | null;
    };
  };
  latestVersion: {
    value: string | null;
    sourceUrl: string | null;
    checkedAt: string | null;
    status:
      | "unknown"
      | "not_installed"
      | "up_to_date"
      | "outdated"
      | "ahead_of_latest";
    error: string | null;
  };
  recommendations: string[];
}

export type WorkerBrokerOrderUpdateSubscriptionStatus =
  | "active"
  | "retrying"
  | "inactive";

export interface WorkerBrokerOrderUpdateErrorContext {
  summary: string;
  rawMessage: string | null;
  code: string | null;
  reason: string | null;
  category: "connection" | "broker" | "subscription" | "unknown";
}

export interface WorkerBrokerOrderUpdatesResponse {
  subscriptions: Array<{
    subscriptionKey: string;
    brokerId: string;
    tradingEnvironment: string | null;
    accountId: string | null;
    market: string | null;
    status: WorkerBrokerOrderUpdateSubscriptionStatus;
    lastAction: string;
    lastActionAt: string;
    lastError: string | null;
    lastErrorContext: WorkerBrokerOrderUpdateErrorContext | null;
    consecutiveFailures: number | null;
    retryDelayMs: number | null;
    backoffUntil: string | null;
  }>;
  recentInvalidations: Array<{
    subscriptionKey: string;
    brokerId: string;
    tradingEnvironment: string | null;
    accountId: string | null;
    market: string | null;
    kind: "DISCONNECTED" | "ERROR";
    message: string | null;
    errorContext: WorkerBrokerOrderUpdateErrorContext | null;
    consecutiveFailures: number | null;
    retryDelayMs: number | null;
    backoffUntil: string | null;
    createdAt: string;
  }>;
  brokers: Array<{
    brokerId: string;
    lastAction: string;
    lastActionAt: string;
    connectivity: string | null;
    lastError: string | null;
    accountsDiscovered: number | null;
    activeSubscriptions: number;
    retryingSubscriptions: number;
    inactiveSubscriptions: number;
    backoffSubscriptions: number;
    disconnectedBackoffSubscriptions: number;
    subscribeFailedBackoffSubscriptions: number;
    errorBackoffSubscriptions: number;
    dominantBackoffSource: "SUBSCRIBE_FAILED" | "DISCONNECTED" | "ERROR" | null;
    dominantBackoffCount: number;
    longestBackoffSource: "SUBSCRIBE_FAILED" | "DISCONNECTED" | "ERROR" | null;
    longestBackoffRemainingMs: number | null;
    longestBackoffSubscriptionKey: string | null;
    longestBackoffMarket: string | null;
    longestBackoffTradingEnvironment: string | null;
    longestBackoffAccountId: string | null;
    topBackoffHotspots: Array<{
      subscriptionKey: string;
      source: "SUBSCRIBE_FAILED" | "DISCONNECTED" | "ERROR";
      remainingMs: number;
      backoffUntil: string;
      lastActionAt: string;
      tradingEnvironment: string | null;
      accountId: string | null;
      market: string | null;
      reason: string | null;
      reasonContext: WorkerBrokerOrderUpdateErrorContext | null;
    }>;
    layeredBackoffSummaries: Array<{
      tradingEnvironment: string | null;
      accountId: string | null;
      activeSubscriptions: number;
      retryingSubscriptions: number;
      inactiveSubscriptions: number;
      backoffSubscriptions: number;
      dominantBackoffSource:
        | "SUBSCRIBE_FAILED"
        | "DISCONNECTED"
        | "ERROR"
        | null;
      dominantBackoffCount: number;
      longestBackoffRemainingMs: number | null;
      topBackoffMarket: string | null;
    }>;
    recentInvalidationCount: number;
    lastInvalidationKind: "DISCONNECTED" | "ERROR" | null;
    lastInvalidationAt: string | null;
    backoffActive: boolean;
    backoffSource: "SUBSCRIBE_FAILED" | "DISCONNECTED" | "ERROR" | null;
    backoffUntil: string | null;
    backoffRemainingMs: number | null;
  }>;
  runtime: {
    lastStoppedAt: string | null;
    stoppedSubscriptions: number | null;
  };
}

export type RealTradeApprovalDecision = "approved" | "rejected";

export interface RealTradeApprovalsResponse {
  realTradingEnabled: boolean;
  requiredConfirmationText: string;
  maxApprovalAgeMs: number;
  approvalWorkflowAvailable?: boolean;
  approvalWorkflowStatus?: "not_configured" | "not_implemented" | "available" | string;
  approvalWorkflowMessage?: string | null;
  approvalPolicy?: {
    approverAllowlistEnabled: boolean;
    approverCount: number;
    largeOrderNotional?: number | null;
    approvalWorkflowAvailable?: boolean;
    approvalMode?: string;
  };
  entries: Array<{
    id: string;
    decision: RealTradeApprovalDecision;
    action: string;
    brokerId: string;
    operation: string;
    tradingEnvironment: string | null;
    accountId: string | null;
    market: string | null;
    symbol: string | null;
    orderId: string | null;
    operatorId: string | null;
    ticketId: string | null;
    reason: string | null;
    approvedAt: string | null;
    gateReason: string | null;
    errorCode: string | null;
    createdAt: string;
  }>;
}

export interface RealTradeRiskEventsResponse {
  realTradingEnabled: boolean;
  riskEnabled: boolean;
  runtimeRiskConfigured: boolean;
  runtimeConfiguredMaxOrderQuantity: number | null;
  runtimeConfiguredMaxOrderNotional: number | null;
  effectiveMaxOrderQuantity: number | null;
  effectiveMaxOrderNotional: number | null;
  maxOrderQuantity: number | null;
  maxOrderNotional: number | null;
  entries: Array<{
    id: string;
    eventType: "activated" | "released" | "rejected" | "updated" | "disabled";
    action: string;
    brokerId: string;
    operation: string | null;
    tradingEnvironment: string | null;
    accountId: string | null;
    market: string | null;
    symbol: string | null;
    orderId: string | null;
    quantity: number | null;
    price: number | null;
    operatorId: string | null;
    reason: string | null;
    errorCode: string | null;
    realTradingEnabled?: boolean | null;
    configuredMaxOrderQuantity: number | null;
    configuredMaxOrderNotional: number | null;
    activatedAt: string | null;
    createdAt: string;
  }>;
}

export interface RealTradeRiskStateResponse {
  realTradingEnabled: boolean;
  riskEnabled: boolean;
  runtimeRiskConfigured: boolean;
  runtimeConfiguredMaxOrderQuantity: number | null;
  runtimeConfiguredMaxOrderNotional: number | null;
  effectiveMaxOrderQuantity: number | null;
  effectiveMaxOrderNotional: number | null;
  entry: {
    id: string;
    tradingEnvironment: string;
    realTradingEnabled: boolean;
    maxOrderQuantity: number | null;
    maxOrderNotional: number | null;
    operatorId: string;
    reason: string;
    activatedAt: string;
    updatedAt: string;
  } | null;
}

export interface RealTradeKillSwitchEventsResponse {
  realTradingEnabled: boolean;
  killSwitchActive: boolean;
  runtimeActive: boolean;
  blockedOperations: string[];
  allowsCancel: boolean;
  entries: Array<{
    id: string;
    eventType: "activated" | "released" | "rejected";
    action: string;
    brokerId: string;
    operation: string | null;
    tradingEnvironment: string | null;
    accountId: string | null;
    market: string | null;
    symbol: string | null;
    orderId: string | null;
    quantity: number | null;
    price: number | null;
    killSwitchSource: "RUNTIME" | null;
    operatorId: string | null;
    reason: string | null;
    errorCode: string | null;
    activatedAt: string | null;
    createdAt: string;
  }>;
}

export interface RealTradeKillSwitchStateResponse {
  realTradingEnabled: boolean;
  runtimeActive: boolean;
  killSwitchActive: boolean;
  killSwitchSource: "RUNTIME" | null;
  blockedOperations: string[];
  allowsCancel: boolean;
  entry: {
    id: string;
    tradingEnvironment: string;
    operatorId: string;
    reason: string;
    activatedAt: string;
    updatedAt: string;
  } | null;
}

export interface RealTradeHardStopsResponse {
  blockedOperations: string[];
  allowsCancel: boolean;
  entries: Array<{
    id: string;
    brokerId: string;
    tradingEnvironment: string;
    accountId: string;
    market: string | null;
    symbol: string | null;
    operatorId: string;
    reason: string;
    activatedAt: string;
    updatedAt: string;
  }>;
}

export interface RealTradeHardStopEventsResponse {
  realTradingEnabled: boolean;
  blockedOperations: string[];
  allowsCancel: boolean;
  entries: Array<{
    id: string;
    eventType: "activated" | "released" | "rejected";
    action: string;
    brokerId: string;
    operation: string | null;
    tradingEnvironment: string | null;
    accountId: string | null;
    market: string | null;
    symbol: string | null;
    orderId: string | null;
    quantity: number | null;
    price: number | null;
    hardStopScope: "ACCOUNT" | "MARKET" | "SYMBOL" | null;
    operatorId: string | null;
    reason: string | null;
    errorCode: string | null;
    hardStopId: string | null;
    activatedAt: string | null;
    createdAt: string;
  }>;
}

export interface BrokerRuntimeResponse {
  descriptor: BrokerDescriptor;
  session: {
    brokerId: string;
    displayName: string;
    connection: {
      host: string;
      apiPort: number;
      websocketPort: number;
      port: number;
      useEncryption: boolean;
    };
    connectivity: string;
    checkedAt: string;
    lastError: string | null;
    globalState: {
      quoteLoggedIn: boolean;
      tradeLoggedIn: boolean;
      serverVersion: string | null;
      programStatus: string | null;
      timestamp: string | null;
      markets: Array<{
        market: string;
        state: string;
      }>;
    } | null;
    accountsDiscovered: number;
  };
  accounts: Array<{
    accountId: string;
    tradingEnvironment: string;
    accountType: string;
    accountRole: string | null;
    securityFirm: string | null;
    marketAuthorities: string[];
    simulatedAccountType: string | null;
  }>;
}

export interface BrokerPositionsResponse {
  checkedAt: string;
  connectivity: string;
  lastError: string | null;
  positions: Array<{
    accountId: string;
    tradingEnvironment: string;
    market: string;
    symbol: string;
    symbolName: string | null;
    quantity: number;
    sellableQuantity: number;
    lastPrice: number;
    costPrice: number | null;
    averageCostPrice: number | null;
    marketValue: number;
    unrealizedPnl: number | null;
    realizedPnl: number | null;
    pnlRatio: number | null;
    currency: string | null;
  }>;
}

export interface BrokerFundsResponse {
  checkedAt: string;
  connectivity: string;
  lastError: string | null;
  summary: {
    accountId: string;
    tradingEnvironment: string;
    market: string;
    currency: string | null;
    totalAssets: number | null;
    securitiesAssets: number | null;
    fundAssets: number | null;
    bondAssets: number | null;
    cash: number | null;
    marketValue: number | null;
    longMarketValue: number | null;
    shortMarketValue: number | null;
    purchasingPower: number | null;
    shortSellingPower: number | null;
    netCashPower: number | null;
    availableWithdrawalCash: number | null;
    maxWithdrawal: number | null;
    availableFunds: number | null;
    frozenCash: number | null;
    pendingAsset: number | null;
    unrealizedPnl: number | null;
    realizedPnl: number | null;
    initialMargin: number | null;
    maintenanceMargin: number | null;
    marginCallMargin: number | null;
    riskStatus: string | null;
    // Margin & Financing 融资融券
    debtCash: number | null;
    isPdt: boolean | null;
    pdtSeq: string | null;
    beginningDTBP: number | null;
    remainingDTBP: number | null;
    dtCallAmount: number | null;
    dtStatus: string | null;
    exposureLevel: string | null;
    exposureLimit: number | null;
    usedLimit: number | null;
    remainingLimit: number | null;
  } | null;
  currencyBalances: Array<{
    accountId: string;
    tradingEnvironment: string;
    currency: string;
    cash: number | null;
    availableWithdrawalCash: number | null;
    netCashPower: number | null;
  }>;
  marketAssets: Array<{
    accountId: string;
    tradingEnvironment: string;
    market: string;
    assets: number | null;
  }>;
}

export interface BrokerCashFlowsResponse {
  checkedAt: string;
  connectivity: string;
  lastError: string | null;
  cashFlows: Array<{
    accountId: string;
    tradingEnvironment: string;
    market: string;
    cashFlowId: string | null;
    clearingDate: string | null;
    settlementDate: string | null;
    currency: string | null;
    cashFlowType: string | null;
    cashFlowDirection: string | null;
    cashFlowAmount: number | null;
    cashFlowRemark: string | null;
  }>;
}

export interface BrokerOrderFeesResponse {
  checkedAt: string;
  connectivity: string;
  lastError: string | null;
  fees: Array<{
    accountId: string;
    tradingEnvironment: string;
    market: string;
    brokerOrderIdEx: string;
    feeAmount: number | null;
    feeItems: Array<{
      title: string;
      value: number;
    }>;
  }>;
}

export interface BrokerFillsResponse {
  checkedAt: string;
  connectivity: string;
  lastError: string | null;
  fills: Array<{
    accountId: string;
    tradingEnvironment: string;
    market: string;
    brokerOrderId: string;
    brokerOrderIdEx: string | null;
    brokerFillId: string;
    brokerFillIdEx: string | null;
    symbol: string;
    symbolName: string | null;
    side: string;
    filledQuantity: number;
    fillPrice: number | null;
    filledAt: string;
    status: string | null;
  }>;
}

export interface BrokerMarginRatiosResponse {
  checkedAt: string;
  connectivity: string;
  lastError: string | null;
  marginRatios: Array<{
    accountId: string;
    tradingEnvironment: string;
    market: string;
    symbol: string;
    isLongPermit: boolean | null;
    isShortPermit: boolean | null;
    shortPoolRemain: number | null;
    shortFeeRate: number | null;
    alertLongRatio: number | null;
    alertShortRatio: number | null;
    initialMarginLongRatio: number | null;
    initialMarginShortRatio: number | null;
    marginCallLongRatio: number | null;
    marginCallShortRatio: number | null;
    maintenanceLongRatio: number | null;
    maintenanceShortRatio: number | null;
  }>;
}

export interface BrokerMaxTradeQuantityResponse {
  checkedAt: string;
  connectivity: string;
  lastError: string | null;
  maxTradeQuantity: {
    accountId: string;
    tradingEnvironment: string;
    market: string;
    symbol: string;
    orderType: string;
    price: number;
    maxCashBuy: number;
    maxCashAndMarginBuy: number | null;
    maxPositionSell: number;
    maxSellShort: number | null;
    maxBuyBack: number | null;
    longRequiredIM: number | null;
    shortRequiredIM: number | null;
    session: string | null;
  } | null;
}

export interface BrokerOrdersResponse {
  checkedAt: string;
  connectivity: string;
  lastError: string | null;
  orders: Array<{
    accountId: string;
    tradingEnvironment: string;
    market: string;
    brokerOrderId: string;
    brokerOrderIdEx: string | null;
    symbol: string;
    symbolName: string | null;
    side: string;
    orderType: string;
    status: string;
    quantity: number;
    filledQuantity: number | null;
    price: number | null;
    filledAveragePrice: number | null;
    submittedAt: string;
    updatedAt: string;
    remark: string | null;
    lastError: string | null;
    timeInForce: string | null;
    currency: string | null;
  }>;
}

export interface PortfolioPositionsResponse {
  positions: Array<{
    brokerId: string;
    tradingEnvironment: string;
    accountId: string;
    market: string;
    symbol: string;
    quantity: number;
    averagePrice: number;
    marketValue: number;
    updatedAt: string;
    createdAt: string;
  }>;
}

export interface PortfolioCashBalancesResponse {
  balances: Array<{
    brokerId: string;
    tradingEnvironment: string;
    accountId: string;
    currency: string;
    cashBalance: number;
    updatedAt: string;
    createdAt: string;
  }>;
}

export type PortfolioReconciliationStatus =
  | "matched"
  | "different"
  | "missing-in-projection"
  | "missing-at-broker";

export interface PortfolioReconciliationResponse {
  checkedAt: string;
  connectivity: string;
  lastError: string | null;
  positions: Array<{
    brokerId: string;
    tradingEnvironment: string;
    accountId: string;
    market: string;
    symbol: string;
    symbolName: string | null;
    status: PortfolioReconciliationStatus;
    projectedQuantity: number | null;
    brokerQuantity: number | null;
    quantityDelta: number;
    projectedAveragePrice: number | null;
    brokerAverageCostPrice: number | null;
    averagePriceDelta: number | null;
    projectedRealizedPnl: number | null;
    brokerRealizedPnl: number | null;
    realizedPnlDelta: number | null;
    projectedUpdatedAt: string | null;
  }>;
}

export interface PortfolioCashReconciliationResponse {
  checkedAt: string;
  connectivity: string;
  lastError: string | null;
  balances: Array<{
    brokerId: string;
    tradingEnvironment: string;
    accountId: string;
    currency: string;
    status: PortfolioReconciliationStatus;
    projectedCashBalance: number | null;
    brokerCash: number | null;
    cashDelta: number;
    brokerAvailableWithdrawalCash: number | null;
    brokerNetCashPower: number | null;
    projectedUpdatedAt: string | null;
  }>;
}

export interface BrokerPlaceOrderRequestPayload {
  tradingEnvironment: string;
  accountId: string;
  market: string;
  code?: string;
  symbol?: string;
  side: string;
  quantity: number;
  idempotencyKey?: string;
  price?: number;
  orderType?: string;
  remark?: string;
  timeInForce?: string;
}

export interface BacktestStartRequestPayload {
  definitionId: string;
  definitionVersion?: string;
  market?: string;
  code?: string;
  symbol?: string;
  instrumentType?: string;
  interval: string;
  startDate: string;
  endDate: string;
  startTime?: string;
  endTime?: string;
  initialBalance: number;
  rehabType?: string;
  useExtendedHours?: boolean;
  tradingCosts?: BacktestTradingCostsPayload;
  executionModel?: "conservative-bar-v1";
}

export interface BacktestFeeRulePayload {
  id: string;
  label?: string;
  category: "broker" | "exchange" | "clearing" | "regulatory" | "tax";
  side?: "buy" | "sell" | "both";
  basis: "notional" | "share" | "order";
  rate?: number;
  fixedAmount?: number;
  minAmount?: number;
  maxAmount?: number;
  maxRate?: number;
  rounding?: string;
  currency?: string;
  appliesTo?: string[];
  effectiveFrom?: string;
  effectiveTo?: string;
  sourceUrl?: string;
}

export interface BacktestFeeSchedulePayload {
  mode?: "market_preset" | "custom" | "script" | "none";
  presetId?: string;
  rules?: BacktestFeeRulePayload[];
}

export interface BacktestTradingCostsPayload {
  brokerFees?: BacktestFeeSchedulePayload;
  marketFees?: BacktestFeeSchedulePayload;
}

export interface BacktestSyncRequestPayload {
  market?: string;
  code?: string;
  symbol?: string;
  intervals: string[];
  startDate: string;
  endDate: string;
  since?: string;
  until?: string;
  rehabType?: string;
  sessionScope?: "legacy" | "regular" | "extended";
}

export interface BrokerCancelOrderRequestPayload {
  tradingEnvironment: string;
  accountId: string;
  market: string;
  orderId: string;
  idempotencyKey?: string;
  quantity?: number;
  price?: number;
}

export interface BrokerModifyOrderRequestPayload {
  tradingEnvironment: string;
  accountId: string;
  market: string;
  orderId: string;
  idempotencyKey?: string;
  quantity?: number;
  price?: number;
}

export interface BrokerOrderSyncRequestPayload {
  tradingEnvironment: string;
  accountId: string;
  market: string;
  symbol?: string;
  orderId?: string;
}

export interface BrokerOrderSyncResponse {
  brokerId: string;
  request: BrokerOrderSyncRequestPayload;
  snapshot: BrokerOrdersResponse;
  syncedOrders: number;
  auditLogId: string;
  auditAction: string;
  outboxEventId: string;
}

export interface BrokerOrderCommandResponse {
  accepted: boolean;
  operation: string;
  internalOrderId?: string;
  brokerOrderId: string | null;
  brokerOrderIdEx: string | null;
  orderStatus: string | null;
  brokerErrorCode: string | null;
  message: string;
  checkedAt: string;
}

export interface ExecutionOrderSummaryResponse {
  internalOrderId: string;
  brokerId: string;
  brokerOrderId: string | null;
  brokerOrderIdEx: string | null;
  source: ExecutionOrderSource;
  sourceDetail: ExecutionOrderSourceDetail;
  tradingEnvironment: string;
  accountId: string;
  market: string;
  symbol: string | null;
  side: string | null;
  orderType: string | null;
  status: string;
  rawBrokerStatus?: string | null;
  requestedQuantity: number | null;
  requestedPrice: number | null;
  filledQuantity: number | null;
  filledAveragePrice: number | null;
  remark: string | null;
  lastError: string | null;
  lastErrorCode: string | null;
  lastErrorSource: ExecutionOrderErrorSource | null;
  submittedAt: string | null;
  updatedAt: string;
  createdAt: string;
}

export type ExecutionOrderSource = "system" | "broker";

export type ExecutionOrderSourceDetail =
  | "command.place"
  | "broker.current"
  | "broker.history"
  | "broker.push"
  | "broker.fill";

export type ExecutionOrderErrorSource =
  | "command.place"
  | "command.cancel"
  | "command.modify"
  | "command.modify.local"
  | "command.modify.broker"
  | "command.modify.fallback"
  | "broker.sync"
  | "broker.push";

export interface ExecutionOrderEventResponse {
  id: string;
  internalOrderId: string;
  eventType: string;
  previousStatus: string | null;
  nextStatus: string;
  payloadJson: string;
  createdAt: string;
}

export interface ExecutionOrdersResponse {
  orders: ExecutionOrderSummaryResponse[];
}

export interface ExecutionOrderEventsResponse {
  internalOrderId: string;
  events: ExecutionOrderEventResponse[];
}

export interface ExecutionOrderDetailsResponse {
  order: ExecutionOrderSummaryResponse;
  recentEvents: ExecutionOrderEventResponse[];
  checkedAt: string;
}

// ---------------------------------------------------------------------------
// Market-data read/query response DTOs
// ---------------------------------------------------------------------------

export interface MarketDataQuoteSnapshotDto {
  lastPrice: number | null;
  openPrice: number | null;
  highPrice: number | null;
  lowPrice: number | null;
  previousClosePrice: number | null;
  volume: number | null;
  turnover: number | null;
  bidPrice: number | null;
  bidSize: number | null;
  askPrice: number | null;
  askSize: number | null;
  quoteCurrency: string | null;
  marketPhase: string;
}

export interface MarketDataCandleDto {
  interval: string;
  openTime: string;
  closeTime: string;
  openPrice: number;
  highPrice: number;
  lowPrice: number;
  closePrice: number;
  volume: number | null;
  turnover: number | null;
  closed: boolean;
}

export interface MarketDataTradeTickDto {
  price: number;
  size: number | null;
  turnover: number | null;
  side: string;
  tradeId: string | null;
}

export interface MarketDataQueryMetaDto {
  instrumentId: string;
  source: string | null;
  resolvedAt: string;
  fromCache: boolean;
}

export interface MarketDataExtendedQuote {
  price?: number | null;
  highPrice?: number | null;
  lowPrice?: number | null;
  volume?: number | null;
  turnover?: number | null;
  changeVal?: number | null;
  changeRate?: number | null;
  amplitude?: number | null;
  quoteTime?: string | null;
}

export interface MarketDataExtendedQuoteBlocks {
  preMarket?: MarketDataExtendedQuote | null;
  afterMarket?: MarketDataExtendedQuote | null;
  overnight?: MarketDataExtendedQuote | null;
}

export interface MarketSecurityRef {
  instrumentId: string;
  market: string;
  symbol: string;
}

export interface MarketSecurityEquityDetails {
  issuedShares: number;
  issuedMarketValue: number;
  netAsset: number;
  netProfit: number;
  earningsPerShare: number;
  outstandingShares: number;
  outstandingMarketVal: number;
  netAssetPerShare: number;
  earningsYieldRate: number;
  peRate: number;
  pbRate: number;
  peTTMRate: number;
  dividendTTM?: number | null;
  dividendRatioTTM?: number | null;
  dividendLFY?: number | null;
  dividendLFYRatio?: number | null;
}

export interface MarketSecurityWarrantDetails {
  conversionRate: number;
  warrantType: string;
  strikePrice: number;
  maturityTime: string;
  endTradeTime: string;
  owner?: MarketSecurityRef | null;
  recoveryPrice: number;
  streetVolume: number;
  issueVolume: number;
  streetRate: number;
  delta: number;
  impliedVolatility: number;
  premium: number;
  maturityTimestamp?: number | null;
  endTradeTimestamp?: number | null;
  leverage?: number | null;
  inOutPriceRatio?: number | null;
  breakEvenPoint?: number | null;
  conversionPrice?: number | null;
  priceRecoveryRatio?: number | null;
  score?: number | null;
  upperStrikePrice?: number | null;
  lowerStrikePrice?: number | null;
  inLinePriceStatus?: string | null;
  issuerCode?: string | null;
}

export interface MarketSecurityOptionDetails {
  optionType: string;
  owner?: MarketSecurityRef | null;
  strikeTime: string;
  strikePrice: number;
  contractSize: number;
  contractSizeFloat?: number | null;
  openInterest: number;
  impliedVolatility: number;
  premium: number;
  delta: number;
  gamma: number;
  vega: number;
  theta: number;
  rho: number;
  strikeTimestamp?: number | null;
  indexOptionType?: string | null;
  netOpenInterest?: number | null;
  expiryDateDistance?: number | null;
  contractNominalValue?: number | null;
  ownerLotMultiplier?: number | null;
  optionAreaType?: string | null;
  contractMultiplier?: number | null;
}

export interface MarketSecurityIndexDetails {
  raiseCount: number;
  fallCount: number;
  equalCount: number;
}

export interface MarketSecurityPlateDetails {
  raiseCount: number;
  fallCount: number;
  equalCount: number;
}

export interface MarketSecurityFutureDetails {
  lastSettlePrice: number;
  position: number;
  positionChange: number;
  lastTradeTime: string;
  lastTradeTimestamp?: number | null;
  isMainContract: boolean;
}

export interface MarketSecurityTrustDetails {
  dividendYield: number;
  aum: number;
  outstandingUnit: number;
  netAssetValue: number;
  premium: number;
  assetClass: string;
}

export interface MarketSecurityDetails {
  instrumentId: string;
  market: string;
  symbol: string;
  securityId?: number | null;
  name: string;
  securityType: string;
  exchangeType: string;
  listTime: string;
  listTimestamp?: number | null;
  delisting?: boolean | null;
  lotSize: number;
  isSuspend: boolean;
  priceSpread: number;
  updateTime: string;
  updateTimestamp?: number | null;
  highPrice: number;
  openPrice: number;
  lowPrice: number;
  lastClosePrice: number;
  currentPrice: number;
  volume: number;
  turnover: number;
  turnoverRate: number;
  askPrice?: number | null;
  bidPrice?: number | null;
  askVolume?: number | null;
  bidVolume?: number | null;
  amplitude?: number | null;
  averagePrice?: number | null;
  bidAskRatio?: number | null;
  volumeRatio?: number | null;
  highest52WeeksPrice?: number | null;
  lowest52WeeksPrice?: number | null;
  highestHistoryPrice?: number | null;
  lowestHistoryPrice?: number | null;
  sessionStatus?: string | null;
  closePrice5Minute?: number | null;
  highPrecisionVolume?: number | null;
  highPrecisionAskVol?: number | null;
  highPrecisionBidVol?: number | null;
  extended?: MarketDataExtendedQuoteBlocks | null;
  equity?: MarketSecurityEquityDetails | null;
  warrant?: MarketSecurityWarrantDetails | null;
  option?: MarketSecurityOptionDetails | null;
  index?: MarketSecurityIndexDetails | null;
  plate?: MarketSecurityPlateDetails | null;
  future?: MarketSecurityFutureDetails | null;
  trust?: MarketSecurityTrustDetails | null;
}

export interface MarketSecurityDetailsQueryResult {
  request: {
    market: string;
    symbol: string;
    instrumentId: string;
  };
  security: MarketSecurityDetails | null;
  meta: MarketDataQueryMetaDto;
}

export interface MarketDataSnapshotResponse {
  ok: boolean;
  instrumentId: string;
  snapshot: MarketDataQuoteSnapshotDto | null;
  meta: MarketDataQueryMetaDto;
  error: string | null;
}

export interface MarketDataCandlesResponse {
  ok: boolean;
  instrumentId: string;
  interval: string;
  fromTime: string | null;
  toTime: string | null;
  totalReturned: number;
  candles: MarketDataCandleDto[];
  meta: MarketDataQueryMetaDto;
  error: string | null;
}

export interface MarketDataTicksResponse {
  ok: boolean;
  instrumentId: string;
  fromTime: string;
  toTime: string;
  totalReturned: number;
  ticks: MarketDataTradeTickDto[];
  meta: MarketDataQueryMetaDto;
  error: string | null;
}

// --- Depth (Order Book) ---

export interface OrderBookDetailItemDto {
  orderId: number;
  volume: number;
}

export interface OrderBookLevelDto {
  price: number;
  volume: number;
  orderCount: number;
  detailList?: OrderBookDetailItemDto[] | null;
}

export interface OrderBookSnapshotDto {
  accountId: string;
  symbol: string;
  name?: string | null;
  svrRecvTimeBid?: string | null;
  svrRecvTimeAsk?: string | null;
  bids: OrderBookLevelDto[];
  asks: OrderBookLevelDto[];
}

export interface MarketDataDepthResponse {
  request: {
    market: string;
    symbol: string;
    instrumentId: string;
    num: number;
  };
  depth: OrderBookSnapshotDto;
  meta: MarketDataQueryMetaDto;
}

export interface OrderBookDepthPreset {
  num: number;
  label: string;
}

export interface BrokerOrderBookCapability {
  defaultNum: number;
  minNum: number;
  maxNum: number;
  numPresets: number[];
  supportsRealTimePush: boolean;
}

export interface MarketDataProviderCapabilities {
  snapshots: boolean;
  streamingQuotes: boolean;
  streamingDepth: boolean;
  historicalCandles: boolean;
  tickCandles: boolean;
  orderBookDepth: boolean;
  instrumentSearch: boolean;
  extendedHours: boolean;
  candleIntervals: string[];
  orderBookLevels: number[];
  sessions: string[];
}

export interface MarketDataProviderConstraints {
  requiresOpenD: boolean;
  requiresMarketDataRight: boolean;
  usesSubscriptionQuota: boolean;
}

export interface MarketDataProviderDescriptor {
  providerId: string;
  displayName: string;
  brokerId?: string;
  source: string;
  defaultMarket: string;
  supportedMarkets: string[];
  transports: string[];
  capabilities: MarketDataProviderCapabilities;
  constraints: MarketDataProviderConstraints;
  notes?: string[];
}

export interface MarketDataProviderHealth {
  connected: boolean;
  streamMode: string;
  activeCount: number;
}

export interface MarketDataRuntimeState {
  Connected: boolean;
  Generation: number;
  ActiveCount: number;
  LastRefreshAt: string;
  QuoteRetryAt: string;
  QuoteFailures: number;
  QuoteLastError: string;
  StreamRetryAt: string;
  StreamFailures: number;
  StreamLastError: string;
  Closed: boolean;
}

export interface MarketDataProviderStatusResponse {
  descriptor: MarketDataProviderDescriptor;
  health: MarketDataProviderHealth;
  runtime: MarketDataRuntimeState;
  subscriptions: MarketDataSubscriptionsResponse;
  checkedAt: string;
}

export interface MarketDataSubscriptionEntryDto {
  key: string;
  channel: string;
  market: string;
  symbol: string;
  instrumentId: string;
  interval: string | null;
  depthLevel: number | null;
  consumers: string[];
  refCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface MarketDataSubscriptionQuotaBucketDto {
  market: string;
  used: number;
  limit: number | null;
  remaining: number | null;
}

export interface MarketDataSubscriptionsResponse {
  totalActiveSubscriptions: number;
  quota: {
    totalUsed: number;
    totalLimit: number | null;
    totalRemaining: number | null;
    byMarket: MarketDataSubscriptionQuotaBucketDto[];
  };
  entries: MarketDataSubscriptionEntryDto[];
}

export const architectureCards: ArchitectureCard[] = [
  {
    title: "Broker Gateway",
    owner: "broker-core",
    status: "Phase 2 Active",
    summary:
      "统一承接 Futu / OpenD，并已具备最小 session probe 与账户发现能力。",
    bullets: [
      "显式区分 SIMULATE 与 REAL 环境",
      "中央化处理账户能力与市场能力",
      "原始券商 payload 不上浮到业务层",
    ],
  },
  {
    title: "Execution + Risk",
    owner: "execution / risk-engine",
    status: "Scaffolded",
    summary: "订单状态机与风控门禁会成为 live trading 的核心保护带。",
    bullets: [
      "策略只能产生下单意图，不直接触达券商",
      "所有下单路径先过风控",
      "拒单、撤单、成交都保留审计线索",
    ],
  },
  {
    title: "Data + Strategy Runtime",
    owner: "market-data / strategy-runtime",
    status: "Scaffolded",
    summary:
      "统一实时行情、回放、回测与策略运行接口，降低 live 和 backtest 偏差。",
    bullets: [
      "订阅额度与限频集中治理",
      "策略运行上下文可复用于回测",
      "市场日历与多市场规则进入统一模型",
    ],
  },
];

export const roadmapPhases: RoadmapPhase[] = [
  {
    key: "phase-0",
    title: "Phase 0 / Workspace Scaffold",
    target: "当前已完成",
    summary:
      "完成 monorepo 根配置、Express API、Worker、Vue 控制台和核心 packages 占位。",
  },
  {
    key: "phase-1",
    title: "Phase 1 / Persistence + Infra",
    target: "已完成",
    summary: "持久层、统一错误模型、日志与健康检查已经落地并完成运行验证。",
  },
  {
    key: "phase-2",
    title: "Phase 2 / Futu Minimum Loop",
    target: "当前进行中",
    summary:
      "已接入 OpenD session probe 与账户发现，下一步进入模拟账户下单、撤单与订单同步。",
  },
];

export const consolePanels: ConsolePanel[] = [
  {
    name: "策略台",
    state: "等待实现",
    description: "负责策略实例生命周期、参数管理、运行日志与人工干预。",
  },
  {
    name: "订单台",
    state: "等待实现",
    description: "显示订单状态机、撤改单通道、拒单原因与实时成交。",
  },
  {
    name: "风控看板",
    state: "等待实现",
    description: "展示 kill switch、规则命中、账户限制与环境门禁状态。",
  },
  {
    name: "组合面板",
    state: "等待实现",
    description: "负责持仓、资金、盈亏、多币种估值与对账视图。",
  },
];

export const emptySystemStatus: SystemStatusResponse = {
  name: "JFTrade",
  apiPort: 3000,
  defaultBroker: "futu",
  defaultTradingEnvironment: "SIMULATE",
  realTradingEnabled: false,
  realTradingKillSwitch: {
    active: false,
    runtimeActive: false,
    blockedOperations: ["PLACE", "MODIFY"],
    allowsCancel: true,
  },
  realTradingRisk: {
    enabled: false,
    maxOrderQuantity: null,
    maxOrderNotional: null,
    runtimeConfiguredMaxOrderQuantity: null,
    runtimeConfiguredMaxOrderNotional: null,
    runtimeRiskConfigured: false,
  },
  realTradeAccess: {
    approverAllowlistEnabled: false,
    approverCount: 0,
    adminAllowlistEnabled: false,
    adminCount: 0,
  },
  broker: {
    id: "futu",
    displayName: "Futu OpenAPI via OpenD",
    environments: ["SIMULATE", "REAL"],
    capabilities: [],
    notes: [],
  },
  persistence: {
    engine: "sqlite",
    databasePath: "./var/db/jftrade.sqlite",
    status: "warn",
    migrated: false,
    pendingMigrations: [],
    tables: [],
    checkedAt: new Date(0).toISOString(),
  },
  strategyRuntime: {
    status: "idle",
    activeStrategies: 0,
    supportsBacktestParity: true,
    activeInstances: [],
  },
  runtimeResources: {
    checkedAt: new Date(0).toISOString(),
    count: 0,
    items: [],
  },
  observability: {
    requests: {
      recentErrors: [],
      recentSlowRequests: [],
      slowThresholdMs: 750,
      minimumImportance: "low",
      openD: {
        totalCalls: 0,
        failedCalls: 0,
      },
    },
  },
  message: "Waiting for API connection.",
};

export const emptyStorageOverview: StorageOverviewResponse = {
  pendingOutbox: [],
  recentJobs: [],
  recentAuditLogs: [],
  recentExecutionCommands: [],
};

export const emptyBrokerSettings: BrokerSettingsResponse = {
  brokers: [],
  accounts: [],
};

export const emptyExecutionSettings: ExecutionSettingsResponse = {
  defaultTradingEnvironment: "SIMULATE",
  brokerOrderHistoryLookbackDays: 30,
  seenFillRetentionDays: 90,
};

export const emptyOnboardingState: OnboardingStateResponse = {
  state: {
    completed: true,
    lastBrokerId: "",
  },
  shouldShowOobe: false,
  reasons: [],
  recommendedBrokerId: "futu",
  brokers: [],
};

export const emptyPluginCatalog: PluginCatalogResponse = {
  targetDir: "",
  plugins: [],
};

export const emptyFutuOpenDInstallGuide: FutuOpenDInstallGuideResponse = {
  brokerId: "futu",
  title: "",
  description: "",
  options: [],
  nextSteps: [],
  settings: {
    host: "127.0.0.1",
    apiPort: 11110,
    websocketPort: 11111,
    maxWebSocketConnections: 20,
    useEncryption: false,
    websocketKeyRequired: false,
    minimumVersion: "10.8.6808",
  },
};

export const emptyFutuOpenDHealth: FutuOpenDHealthResponse = {
  checkedAt: "",
  status: "offline",
  runtime: {
    connectivity: "disconnected",
    host: "127.0.0.1",
    port: 11111,
    useEncryption: false,
    websocketKeyConfigured: false,
    quoteLoggedIn: null,
    tradeLoggedIn: null,
    programStatus: null,
    serverVersion: null,
    minimumVersion: "10.8.6808",
    lastError: null,
  },
  diagnosis: {
    code: "NONE",
    summary: null,
    manualRetryRequired: false,
    restartOpenDRecommended: false,
  },
  localSocketDiagnostics: {
    websocketEstablishedConnections: 0,
    likelyConnectionSaturation: false,
    topClientProcesses: [],
  },
  localInstallation: {
    platform: "",
    installed: false,
    version: null,
    installPath: null,
    guiDetected: false,
    process: {
      running: false,
      pid: null,
      executablePath: null,
    },
  },
  latestVersion: {
    value: null,
    sourceUrl: null,
    checkedAt: null,
    status: "unknown",
    error: null,
  },
  recommendations: [],
};

export const emptyWorkerBrokerOrderUpdates: WorkerBrokerOrderUpdatesResponse = {
  subscriptions: [],
  recentInvalidations: [],
  brokers: [],
  runtime: {
    lastStoppedAt: null,
    stoppedSubscriptions: null,
  },
};

export const emptyRealTradeApprovals: RealTradeApprovalsResponse = {
  realTradingEnabled: false,
  requiredConfirmationText: "ENABLE_REAL_TRADING",
  maxApprovalAgeMs: 5 * 60 * 1000,
  approvalWorkflowAvailable: false,
  approvalWorkflowStatus: "not_configured",
  approvalWorkflowMessage: null,
  approvalPolicy: {
    approverAllowlistEnabled: false,
    approverCount: 0,
    approvalWorkflowAvailable: false,
    approvalMode: "none",
  },
  entries: [],
};

export const emptyRealTradeRiskEvents: RealTradeRiskEventsResponse = {
  realTradingEnabled: false,
  riskEnabled: false,
  runtimeRiskConfigured: false,
  runtimeConfiguredMaxOrderQuantity: null,
  runtimeConfiguredMaxOrderNotional: null,
  effectiveMaxOrderQuantity: null,
  effectiveMaxOrderNotional: null,
  maxOrderQuantity: null,
  maxOrderNotional: null,
  entries: [],
};

export const emptyRealTradeRiskState: RealTradeRiskStateResponse = {
  realTradingEnabled: false,
  riskEnabled: false,
  runtimeRiskConfigured: false,
  runtimeConfiguredMaxOrderQuantity: null,
  runtimeConfiguredMaxOrderNotional: null,
  effectiveMaxOrderQuantity: null,
  effectiveMaxOrderNotional: null,
  entry: null,
};

export const emptyRealTradeKillSwitchEvents: RealTradeKillSwitchEventsResponse =
  {
    realTradingEnabled: false,
    killSwitchActive: false,
    runtimeActive: false,
    blockedOperations: ["PLACE", "MODIFY"],
    allowsCancel: true,
    entries: [],
  };

export const emptyRealTradeKillSwitchState: RealTradeKillSwitchStateResponse = {
  realTradingEnabled: false,
  runtimeActive: false,
  killSwitchActive: false,
  killSwitchSource: null,
  blockedOperations: ["PLACE", "MODIFY"],
  allowsCancel: true,
  entry: null,
};

export const emptyRealTradeHardStops: RealTradeHardStopsResponse = {
  blockedOperations: ["PLACE", "MODIFY"],
  allowsCancel: true,
  entries: [],
};

export const emptyRealTradeHardStopEvents: RealTradeHardStopEventsResponse = {
  realTradingEnabled: false,
  blockedOperations: ["PLACE", "MODIFY"],
  allowsCancel: true,
  entries: [],
};

export const emptyBrokerRuntime: BrokerRuntimeResponse = {
  descriptor: {
    id: "futu",
    displayName: "Futu OpenAPI via OpenD",
    environments: ["SIMULATE", "REAL"],
    capabilities: [],
    notes: [],
  },
  session: {
    brokerId: "futu",
    displayName: "Futu OpenAPI via OpenD",
    connection: {
      host: "127.0.0.1",
      apiPort: 11110,
      websocketPort: 11111,
      port: 11110,
      useEncryption: false,
    },
    connectivity: "disconnected",
    checkedAt: "",
    lastError: null,
    globalState: null,
    accountsDiscovered: 0,
  },
  accounts: [],
};

export const emptyBrokerPositions: BrokerPositionsResponse = {
  checkedAt: new Date(0).toISOString(),
  connectivity: "disconnected",
  lastError: null,
  positions: [],
};

export const emptyBrokerFunds: BrokerFundsResponse = {
  checkedAt: new Date(0).toISOString(),
  connectivity: "disconnected",
  lastError: null,
  summary: null,
  currencyBalances: [],
  marketAssets: [],
};

export const emptyBrokerCashFlows: BrokerCashFlowsResponse = {
  checkedAt: new Date(0).toISOString(),
  connectivity: "disconnected",
  lastError: null,
  cashFlows: [],
};

export const emptyBrokerOrderFees: BrokerOrderFeesResponse = {
  checkedAt: new Date(0).toISOString(),
  connectivity: "disconnected",
  lastError: null,
  fees: [],
};

export const emptyBrokerFills: BrokerFillsResponse = {
  checkedAt: new Date(0).toISOString(),
  connectivity: "disconnected",
  lastError: null,
  fills: [],
};

export const emptyBrokerMarginRatios: BrokerMarginRatiosResponse = {
  checkedAt: new Date(0).toISOString(),
  connectivity: "disconnected",
  lastError: null,
  marginRatios: [],
};

export const emptyBrokerMaxTradeQuantity: BrokerMaxTradeQuantityResponse = {
  checkedAt: new Date(0).toISOString(),
  connectivity: "disconnected",
  lastError: null,
  maxTradeQuantity: null,
};

export const emptyBrokerOrders: BrokerOrdersResponse = {
  checkedAt: new Date(0).toISOString(),
  connectivity: "disconnected",
  lastError: null,
  orders: [],
};

export const emptyPortfolioPositions: PortfolioPositionsResponse = {
  positions: [],
};

export const emptyPortfolioCashBalances: PortfolioCashBalancesResponse = {
  balances: [],
};

export const emptyPortfolioReconciliation: PortfolioReconciliationResponse = {
  checkedAt: new Date(0).toISOString(),
  connectivity: "disconnected",
  lastError: null,
  positions: [],
};

export const emptyPortfolioCashReconciliation: PortfolioCashReconciliationResponse =
  {
    checkedAt: new Date(0).toISOString(),
    connectivity: "disconnected",
    lastError: null,
    balances: [],
  };

export const emptyExecutionOrders: ExecutionOrdersResponse = {
  orders: [],
};

export const emptyExecutionOrderEvents: ExecutionOrderEventsResponse = {
  internalOrderId: "",
  events: [],
};

export const emptyMarketDataSubscriptions: MarketDataSubscriptionsResponse = {
  totalActiveSubscriptions: 0,
  quota: {
    totalUsed: 0,
    totalLimit: null,
    totalRemaining: null,
    byMarket: [],
  },
  entries: [],
};
