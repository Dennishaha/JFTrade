import type {
  BrokerCashFlowsResponse,
  BrokerFillsResponse,
  BrokerFundsResponse,
  BrokerMarginRatiosResponse,
  BrokerMaxTradeQuantityResponse,
  BrokerOrderFeesResponse,
  BrokerOrdersResponse,
  BrokerPositionsResponse,
  BrokerRuntimeResponse,
  ExecutionOrderEventResponse,
  PortfolioCashBalancesResponse,
  PortfolioPositionsResponse,
  RealTradeApprovalsResponse,
  RealTradeHardStopEventsResponse,
  RealTradeHardStopsResponse,
  RealTradeKillSwitchEventsResponse,
  RealTradeKillSwitchStateResponse,
  RealTradeRiskEventsResponse,
  RealTradeRiskStateResponse,
} from "../../contracts/generated/trading";

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
  chartType?: "standard" | "heikinashi";
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

export type BacktestSessionScope = "regular" | "extended";

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
  sessionScope?: BacktestSessionScope;
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
  orderKind?: "single" | "option_combo" | "event_single" | "event_parlay";
  productClass?: string;
  quantityMode?: "units" | "contracts" | "amount";
  clientOrderId?: string | null;
  previewId?: string | null;
  requestedAmount?: number | null;
  payout?: number | null;
  legs?: ExecutionOrderLegResponse[];
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

export interface ExecutionOrderLegResponse {
  id: string;
  internalOrderId: string;
  index: number;
  brokerLegId?: string | null;
  instrumentId: string;
  productClass: string;
  side: string;
  ratio: number;
  predictionSide?: string;
  requestedQuantity?: number | null;
  requestedAmount?: number | null;
  requestedPrice?: number | null;
  status: string;
  filledQuantity?: number | null;
  filledAmount?: number | null;
  averagePrice?: number | null;
  fees?: number | null;
  payout?: number | null;
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

export const emptyRealTradeApprovals: RealTradeApprovalsResponse = {
  realTradingEnabled: false,
  requiredConfirmationText: "ENABLE_REAL_TRADING",
  maxApprovalAgeMs: 5 * 60 * 1000,
  approvalWorkflowAvailable: false,
  approvalWorkflowStatus: "not_configured",
  approvalWorkflowMessage: "",
  approvalPolicy: {
    approverAllowlistEnabled: false,
    approverCount: 0,
    largeOrderNotional: null,
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
    displayName: "Futu",
    environments: ["SIMULATE", "REAL"],
    capabilities: [],
    notes: [],
  },
  session: {
    brokerId: "futu",
    displayName: "Futu",
    connection: {
      host: "127.0.0.1",
      apiPort: 11110,
      websocketPort: 11111,
      port: 11110,
      useEncryption: false,
      marketDataTransport: "bbgo-opend-tcp-api",
    },
    connectivity: "disconnected",
    checkedAt: "",
    lastError: null,
    globalState: null,
    accountsDiscovered: 0,
    liveWebSocketClients: {
      connected: 0,
      limit: 20,
      atLimit: false,
    },
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
  checkedAt: new Date(0).toISOString(),
  connectivity: "disconnected",
  lastError: null,
  positions: [],
};

export const emptyPortfolioCashBalances: PortfolioCashBalancesResponse = {
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
