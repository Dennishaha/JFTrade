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

export interface RealtimeKlineSnapshot {
  price: number;
  volume: number;
  at: string;
}

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

function mergeDisplayCandles(
  current: readonly KlineCandle[],
  next: readonly KlineCandle[],
): KlineCandle[] {
  const byTime = new Map<string, KlineCandle>();
  for (const candle of [...current, ...next]) {
    byTime.set(candle.at, candle);
  }

  return [...byTime.values()].sort(
    (left, right) => new Date(left.at).getTime() - new Date(right.at).getTime(),
  );
}

export function resolveKlinePeriodDurationMs(period: string): number | null {
  switch (period) {
    case "tick":
      return null;
    case "1m":
      return 60_000;
    case "3m":
      return 3 * 60_000;
    case "5m":
      return 5 * 60_000;
    case "10m":
      return 10 * 60_000;
    case "15m":
      return 15 * 60_000;
    case "30m":
      return 30 * 60_000;
    case "1h":
      return 60 * 60_000;
    case "1d":
      return 24 * 60 * 60_000;
    case "1w":
      return 7 * 24 * 60 * 60_000;
    default:
      return null;
  }
}

export function overlayRealtimeTickCandle(
  candles: readonly KlineCandle[],
  snapshot: RealtimeKlineSnapshot | null,
  period: string,
): KlineCandle[] {
  if (snapshot == null) {
    return [...candles];
  }

  const tickTime = new Date(snapshot.at).getTime();
  if (!Number.isFinite(tickTime)) {
    return [...candles];
  }

  if (period === "tick") {
    return mergeDisplayCandles(candles, [
      {
        at: snapshot.at,
        open: snapshot.price,
        high: snapshot.price,
        low: snapshot.price,
        close: snapshot.price,
        volume: snapshot.volume,
      },
    ]);
  }

  const durationMs = resolveKlinePeriodDurationMs(period);
  if (durationMs == null) {
    return [...candles];
  }

  const bucketStart = new Date(
    Math.floor(tickTime / durationMs) * durationMs,
  ).toISOString();
  const existing = candles.find((candle) => candle.at === bucketStart);
  const last = candles[candles.length - 1];
  const baseOpen = existing?.open ?? last?.close ?? snapshot.price;
  const baseVolume = existing?.volume ?? 0;

  return mergeDisplayCandles(candles, [
    {
      at: bucketStart,
      open: baseOpen,
      high: Math.max(existing?.high ?? baseOpen, snapshot.price),
      low: Math.min(existing?.low ?? baseOpen, snapshot.price),
      close: snapshot.price,
      volume: Math.max(baseVolume, snapshot.volume),
    },
  ]);
}
