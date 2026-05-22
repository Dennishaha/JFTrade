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

export interface SystemStatusResponse {
  name: string;
  apiPort: number;
  defaultBroker: string;
  defaultTradingEnvironment: string;
  realTradingEnabled: boolean;
  realTradingKillSwitch: {
    active: boolean;
    envConfiguredActive: boolean;
    controlPlaneActive: boolean;
    blockedOperations: string[];
    allowsCancel: boolean;
  };
  realTradingRisk: {
    enabled: boolean;
    maxOrderQuantity: number | null;
    maxOrderNotional: number | null;
    envConfiguredMaxOrderQuantity: number | null;
    envConfiguredMaxOrderNotional: number | null;
    controlPlaneActive: boolean;
    controlPlaneMaxOrderQuantity: number | null;
    controlPlaneMaxOrderNotional: number | null;
    riskConfigSource: "ENV" | "CONTROL_PLANE" | "MERGED" | null;
  };
  realTradeAccess?: {
    approverAllowlistEnabled: boolean;
    approverCount: number;
    adminAllowlistEnabled: boolean;
    adminCount: number;
  };
  broker: {
    id: string;
    displayName: string;
    environments: string[];
    capabilities: Array<{
      market: string;
      supportsQuote: boolean;
      supportsTrade: boolean;
    }>;
    notes: string[];
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
  strategyRuntime: {
    status: string;
    activeStrategies: number;
    supportsBacktestParity: boolean;
  };
  message: string;
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
    descriptor: {
      id: string;
      displayName: string;
      environments: string[];
      capabilities: Array<{
        market: string;
        supportsQuote: boolean;
        supportsTrade: boolean;
      }>;
      notes: string[];
    };
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
  }>;
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
  };
}

export type FutuOpenDIssueCode =
  | "NONE"
  | "LOGIN_TIMEOUT"
  | "CONNECTION_LIMIT"
  | "PROTOCOL_PARSE_ERROR"
  | "WS_POOL_EXHAUSTED"
  | "WEBSOCKET_AUTH";

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
  approvalPolicy?: {
    approverAllowlistEnabled: boolean;
    approverCount: number;
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
  riskConfigSource: "ENV" | "CONTROL_PLANE" | "MERGED" | null;
  envConfiguredMaxOrderQuantity: number | null;
  envConfiguredMaxOrderNotional: number | null;
  controlPlaneActive: boolean;
  controlPlaneMaxOrderQuantity: number | null;
  controlPlaneMaxOrderNotional: number | null;
  effectiveMaxOrderQuantity: number | null;
  effectiveMaxOrderNotional: number | null;
  maxOrderQuantity: number | null;
  maxOrderNotional: number | null;
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
    riskConfigSource: "ENV" | "CONTROL_PLANE" | "MERGED" | null;
    operatorId: string | null;
    reason: string | null;
    errorCode: string | null;
    configuredMaxOrderQuantity: number | null;
    configuredMaxOrderNotional: number | null;
    envConfiguredMaxOrderQuantity: number | null;
    envConfiguredMaxOrderNotional: number | null;
    controlPlaneMaxOrderQuantity: number | null;
    controlPlaneMaxOrderNotional: number | null;
    activatedAt: string | null;
    createdAt: string;
  }>;
}

export interface RealTradeRiskStateResponse {
  realTradingEnabled: boolean;
  riskEnabled: boolean;
  riskConfigSource: "ENV" | "CONTROL_PLANE" | "MERGED" | null;
  envConfiguredMaxOrderQuantity: number | null;
  envConfiguredMaxOrderNotional: number | null;
  controlPlaneActive: boolean;
  controlPlaneMaxOrderQuantity: number | null;
  controlPlaneMaxOrderNotional: number | null;
  effectiveMaxOrderQuantity: number | null;
  effectiveMaxOrderNotional: number | null;
  entry: {
    id: string;
    tradingEnvironment: string;
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
  envConfiguredActive: boolean;
  controlPlaneActive: boolean;
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
    killSwitchSource: "ENV" | "CONTROL_PLANE" | null;
    operatorId: string | null;
    reason: string | null;
    errorCode: string | null;
    activatedAt: string | null;
    createdAt: string;
  }>;
}

export interface RealTradeKillSwitchStateResponse {
  realTradingEnabled: boolean;
  envConfiguredActive: boolean;
  controlPlaneActive: boolean;
  killSwitchActive: boolean;
  killSwitchSource: "ENV" | "CONTROL_PLANE" | null;
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
  descriptor: {
    id: string;
    displayName: string;
    environments: string[];
    capabilities: Array<{
      market: string;
      supportsQuote: boolean;
      supportsTrade: boolean;
    }>;
    notes: string[];
  };
  session: {
    brokerId: string;
    displayName: string;
    connection: {
      host: string;
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
    cashFlowId: string;
    clearingDate: string | null;
    settlementDate: string | null;
    currency: string | null;
    type: string | null;
    direction: string;
    amount: number | null;
    remark: string | null;
  }>;
}

export interface BrokerOrderFeesResponse {
  checkedAt: string;
  connectivity: string;
  lastError: string | null;
  fees: Array<{
    brokerOrderId: string;
    totalFee: number | null;
    currency: string | null;
    details: Array<{
      title: string;
      amount: number | null;
    }>;
  }>;
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
  symbol: string;
  side: string;
  quantity: number;
  idempotencyKey?: string;
  price?: number;
  orderType?: string;
  remark?: string;
  timeInForce?: string;
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
  tradingEnvironment: string;
  accountId: string;
  market: string;
  symbol: string | null;
  side: string | null;
  orderType: string | null;
  status: string;
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
    envConfiguredActive: false,
    controlPlaneActive: false,
    blockedOperations: ["PLACE", "MODIFY"],
    allowsCancel: true,
  },
  realTradingRisk: {
    enabled: false,
    maxOrderQuantity: null,
    maxOrderNotional: null,
    envConfiguredMaxOrderQuantity: null,
    envConfiguredMaxOrderNotional: null,
    controlPlaneActive: false,
    controlPlaneMaxOrderQuantity: null,
    controlPlaneMaxOrderNotional: null,
    riskConfigSource: null,
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
  },
};

export const emptyFutuOpenDHealth: FutuOpenDHealthResponse = {
  checkedAt: new Date(0).toISOString(),
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
  approvalPolicy: {
    approverAllowlistEnabled: false,
    approverCount: 0,
  },
  entries: [],
};

export const emptyRealTradeRiskEvents: RealTradeRiskEventsResponse = {
  realTradingEnabled: false,
  riskEnabled: false,
  riskConfigSource: null,
  envConfiguredMaxOrderQuantity: null,
  envConfiguredMaxOrderNotional: null,
  controlPlaneActive: false,
  controlPlaneMaxOrderQuantity: null,
  controlPlaneMaxOrderNotional: null,
  effectiveMaxOrderQuantity: null,
  effectiveMaxOrderNotional: null,
  maxOrderQuantity: null,
  maxOrderNotional: null,
  entries: [],
};

export const emptyRealTradeRiskState: RealTradeRiskStateResponse = {
  realTradingEnabled: false,
  riskEnabled: false,
  riskConfigSource: null,
  envConfiguredMaxOrderQuantity: null,
  envConfiguredMaxOrderNotional: null,
  controlPlaneActive: false,
  controlPlaneMaxOrderQuantity: null,
  controlPlaneMaxOrderNotional: null,
  effectiveMaxOrderQuantity: null,
  effectiveMaxOrderNotional: null,
  entry: null,
};

export const emptyRealTradeKillSwitchEvents: RealTradeKillSwitchEventsResponse =
  {
    realTradingEnabled: false,
    killSwitchActive: false,
    envConfiguredActive: false,
    controlPlaneActive: false,
    blockedOperations: ["PLACE", "MODIFY"],
    allowsCancel: true,
    entries: [],
  };

export const emptyRealTradeKillSwitchState: RealTradeKillSwitchStateResponse = {
  realTradingEnabled: false,
  envConfiguredActive: false,
  controlPlaneActive: false,
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
      port: 11111,
      useEncryption: false,
    },
    connectivity: "disconnected",
    checkedAt: new Date(0).toISOString(),
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
