import type { PluginOperationDto } from "./plugins";
import type {
  StrategyBindingInstrumentDocument,
  StrategyBrokerAccountBinding,
  StrategyDefinitionDto,
  StrategyDefinitionSummaryDocument,
  StrategyVisualModelDto,
} from "../../contracts/wire/strategy";

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

export interface StrategyInstanceBindingDocument {
  instruments?: StrategyBindingInstrumentDocument[];
  symbols: string[];
  interval: string;
  chartType?: "standard" | "heikinashi";
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

export type StrategyDefinitionDocument = Omit<
  StrategyDefinitionDto,
  "visualModel"
> & {
  visualModel?:
    | StrategyVisualModelDto
    | PineV6WorkflowDocument
    | StrategyVisualModelDocument
    | null;
  derivedWarmupBars?: number;
  derivedWarmupInterval?: string;
};

export interface PluginInstallResponse {
  operation: PluginOperationDto;
}
