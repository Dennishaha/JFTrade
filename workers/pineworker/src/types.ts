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

export type RunScriptResponse = {
  jobId: string;
  outputs: SeriesOutput[];
  plots: PlotOutput[];
  orderIntents: OrderIntent[];
  logs: string[];
  warnings: string[];
  diagnostics: Diagnostic[];
  metadata: WorkerMetadata;
  error?: string;
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
  logs?: unknown[];
  warnings?: unknown[];
  diagnostics?: Diagnostic[];
  orderIntents?: unknown[];
};

export type PineTSExecutor = {
  run(request: RunScriptRequest): Promise<PineTSRunResult>;
  version(): string;
};
