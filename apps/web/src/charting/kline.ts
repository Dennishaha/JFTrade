export interface KlineCandle {
  at: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

export interface KlineChartPalette {
  bg: string;
  text: string;
  grid: string;
  border: string;
  up: string;
  down: string;
  volumeUp: string;
  volumeDown: string;
}

export interface KlineChartAdapter {
  setCandles(candles: readonly KlineCandle[]): void;
  applyPalette(palette: KlineChartPalette): void;
  setLoadMoreHandler(handler: (() => void) | null): void;
  resize(width: number, height: number): void;
  fitContent(): void;
  remove(): void;
}

export interface CreateKlineChartOptions {
  palette: KlineChartPalette;
}

export interface KlineChartFactory {
  readonly id: string;
  create(
    host: HTMLElement,
    options: CreateKlineChartOptions,
  ): KlineChartAdapter;
}

export const KLINE_PERIODS = [
  { value: "tick", label: "Tick" },
  { value: "1m", label: "1M" },
  { value: "5m", label: "5M" },
  { value: "15m", label: "15M" },
  { value: "30m", label: "30M" },
  { value: "1h", label: "1H" },
  { value: "1d", label: "1D" },
  { value: "1w", label: "1W" },
] as const;

const KLINE_PERIOD_ALIASES: Record<string, string> = {
  K_1M: "1m",
  K_3M: "3m",
  K_5M: "5m",
  K_10M: "10m",
  K_15M: "15m",
  K_30M: "30m",
  K_60M: "1h",
  K_TICK: "tick",
  TICK: "tick",
  TICKER: "tick",
  K_DAY: "1d",
  K_WEEK: "1w",
  "60M": "1h",
  "60MIN": "1h",
  "1H": "1h",
  "1D": "1d",
  "1W": "1w",
};

export function normalizeKlinePeriod(period: string): string {
  const normalized = period.trim();
  const alias = KLINE_PERIOD_ALIASES[normalized.toUpperCase()];
  if (alias != null) {
    return alias;
  }

  const lower = normalized.toLowerCase();
  switch (lower) {
    case "tick":
    case "1m":
    case "3m":
    case "5m":
    case "10m":
    case "15m":
    case "30m":
    case "1h":
    case "2h":
    case "3h":
    case "4h":
    case "1d":
    case "1w":
      return lower;
    default:
      throw new Error(`Unsupported K-line period '${period}'.`);
  }
}

export function formatKlinePeriodLabel(period: string): string {
  const normalized = normalizeKlinePeriod(period);
  return (
    KLINE_PERIODS.find((item) => item.value === normalized)?.label ??
    normalized.toUpperCase()
  );
}
