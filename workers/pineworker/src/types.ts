export const runtimeId = "pine-pinets";

export const workerVersion = "0.1.0";

export type RunMode = "backtest" | "live" | "analyze";

export type Candle = {
  openTime: number;
  closeTime?: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
};

export type RunScriptRequest = {
  jobId: string;
  scriptId?: string;
  source: string;
  symbol: string;
  timeframe: string;
  mode?: RunMode | string;
  candles: Candle[];
  params?: Record<string, string>;
};

export type Diagnostic = {
  severity: "error" | "warning" | "info";
  code: string;
  message: string;
  line?: number;
  column?: number;
};

export type SeriesOutput = {
  name: string;
  kind: string;
  values: number[];
};

export type PlotOutput = {
  name: string;
  values: number[];
};

export type AlertEvent = {
  type: string;
  id: string;
  message: string;
  title?: string;
  frequency?: string;
  barIndex: number;
  time: number;
};

export type VisualOutput = {
  kind: string;
  name: string;
  payloadJson: string;
};

export type OrderIntent = {
  kind: string;
  id?: string;
  fromEntry?: string;
  direction?: "long" | "short" | "flat" | string;
  quantity?: number;
  quantityPct?: number;
  limitPrice?: number;
  stopPrice?: number;
  comment?: string;
  alertMessage?: string;
  disableAlert?: boolean;
  barIndex: number;
  time: number;
  hasQuantity?: boolean;
  hasQuantityPct?: boolean;
  hasLimitPrice?: boolean;
  hasStopPrice?: boolean;
};

export type WorkerMetadata = {
  workerId: string;
  version: string;
  pineTSVersion: string;
  scriptHash: string;
  dataHash: string;
  durationMs: number;
  requestBytes: number;
  responseBytes: number;
  peakRSSBytes: number;
};

export type StrategyMetrics = {
  buyAndHoldPnl: number;
  buyAndHoldPerGain: number;
  strategyOutperformance: number;
  hasBuyAndHoldPnl: boolean;
  hasBuyAndHoldPerGain: boolean;
  hasStrategyOutperformance: boolean;
};

export type RunScriptResponse = {
  jobId: string;
  outputs: SeriesOutput[];
  plots: PlotOutput[];
  orderIntents: OrderIntent[];
  alerts: AlertEvent[];
  visualOutputs: VisualOutput[];
  logs: string[];
  warnings: string[];
  diagnostics: Diagnostic[];
  metadata: WorkerMetadata;
  error?: string;
  strategyMetrics?: StrategyMetrics;
};

export type PineTSPlotDataPoint = {
  time?: number;
  value?: number | null;
};

export type PineTSPlot = {
  data?: PineTSPlotDataPoint[] | number[];
};

export type PineTSRunResult = {
  plots?: Record<string, PineTSPlot | number[]>;
  alerts?: unknown[];
  visualOutputs?: unknown[];
  drawings?: unknown;
  logs?: unknown[];
  warnings?: unknown[];
  diagnostics?: Diagnostic[];
  orderIntents?: unknown[];
  strategy?: unknown;
};

export type PineTSExecutor = {
  run(request: RunScriptRequest): Promise<PineTSRunResult>;
  version(): string;
};

export type HealthStatus = {
  ok: boolean;
  workerId: string;
  version: string;
  pineTSVersion: string;
  capabilities: string[];
};
