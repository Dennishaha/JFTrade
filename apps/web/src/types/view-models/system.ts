import type { ExecutionSettingsResponse } from "../../contracts/wire/settings";
import type {
  RuntimeResourcesSummary,
  SystemStatusResponseDto,
} from "../../contracts/wire/system";
import type { StrategyRuntimeActiveInstanceSummary } from "./strategy";

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
  readFeatures?: Partial<
    Record<BrokerReadFeatureKey, BrokerReadFeatureCapability>
  >;
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

type SystemStatusWire = SystemStatusResponseDto;
type StrategyRuntimeWire = NonNullable<SystemStatusWire["strategyRuntime"]>;

export type SystemStatusResponse = Omit<
  SystemStatusWire,
  | "broker"
  | "observability"
  | "realTradeAccess"
  | "runtimeResources"
  | "strategyRuntime"
> & {
  broker: BrokerDescriptor;
  realTradeAccess?: SystemStatusWire["realTradeAccess"];
  strategyRuntime: Omit<StrategyRuntimeWire, "activeInstances"> & {
    activeInstances?: StrategyRuntimeActiveInstanceSummary[];
  };
  runtimeResources?: RuntimeResourcesSummary;
  observability: {
    requests: RequestObservabilitySummary;
  };
};

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

export const emptySystemStatus: SystemStatusResponse = {
  name: "JFTrade",
  apiPort: 3000,
  build: {
    version: "dev",
    commit: "unknown",
    buildTime: "dev",
    goos: "",
    goarch: "",
  },
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
    displayName: "Futu",
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
