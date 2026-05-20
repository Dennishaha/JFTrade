export interface KlineCandle {
  period?: string;
  at: string;
  displayAt?: string | null;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  session?: string | null;
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
  indicatorA: string;
  indicatorB: string;
  indicatorC: string;
  macdPositive: string;
  macdNegative: string;
}

export interface KlineChartAdapter {
  setCandles(candles: readonly KlineCandle[]): void;
  setIndicators(indicators: readonly KlineIndicatorKey[]): void;
  applyPalette(palette: KlineChartPalette): void;
  setLoadMoreHandler(handler: (() => void) | null): void;
  resize(width: number, height: number): void;
  fitContent(): void;
  remove(): void;
}

export interface CreateKlineChartOptions {
  palette: KlineChartPalette;
  indicators?: readonly KlineIndicatorKey[];
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

const MOVING_AVERAGE_PERIODS = [5, 10, 20, 30, 60, 120, 180, 250] as const;
type MovingAveragePeriod = (typeof MOVING_AVERAGE_PERIODS)[number];

export type KlineIndicatorKey =
  | "volume"
  | "macd"
  | "kdj"
  | `ma${MovingAveragePeriod}`
  | `ema${MovingAveragePeriod}`;

export interface KlineIndicatorDefinition {
  value: KlineIndicatorKey;
  label: string;
  kind: "pane" | "overlay";
  family: "volume" | "macd" | "kdj" | "ma" | "ema";
  period?: MovingAveragePeriod;
}

export const KLINE_INDICATORS = [
  { value: "volume", label: "VOL", kind: "pane", family: "volume" },
  { value: "macd", label: "MACD", kind: "pane", family: "macd" },
  { value: "kdj", label: "KDJ", kind: "pane", family: "kdj" },
  { value: "ma5", label: "MA5", kind: "overlay", family: "ma", period: 5 },
  { value: "ma10", label: "MA10", kind: "overlay", family: "ma", period: 10 },
  { value: "ma20", label: "MA20", kind: "overlay", family: "ma", period: 20 },
  { value: "ma30", label: "MA30", kind: "overlay", family: "ma", period: 30 },
  { value: "ma60", label: "MA60", kind: "overlay", family: "ma", period: 60 },
  { value: "ma120", label: "MA120", kind: "overlay", family: "ma", period: 120 },
  { value: "ma180", label: "MA180", kind: "overlay", family: "ma", period: 180 },
  { value: "ma250", label: "MA250", kind: "overlay", family: "ma", period: 250 },
  { value: "ema5", label: "EMA5", kind: "overlay", family: "ema", period: 5 },
  { value: "ema10", label: "EMA10", kind: "overlay", family: "ema", period: 10 },
  { value: "ema20", label: "EMA20", kind: "overlay", family: "ema", period: 20 },
  { value: "ema30", label: "EMA30", kind: "overlay", family: "ema", period: 30 },
  { value: "ema60", label: "EMA60", kind: "overlay", family: "ema", period: 60 },
  { value: "ema120", label: "EMA120", kind: "overlay", family: "ema", period: 120 },
  { value: "ema180", label: "EMA180", kind: "overlay", family: "ema", period: 180 },
  { value: "ema250", label: "EMA250", kind: "overlay", family: "ema", period: 250 },
] as const satisfies readonly KlineIndicatorDefinition[];

const KLINE_INDICATOR_SET = new Set<KlineIndicatorKey>(
  KLINE_INDICATORS.map((indicator) => indicator.value),
);

export function getKlineIndicatorDefinition(
  indicator: KlineIndicatorKey,
): KlineIndicatorDefinition | undefined {
  return KLINE_INDICATORS.find((definition) => definition.value === indicator);
}

export function isKlinePaneIndicator(indicator: KlineIndicatorKey): boolean {
  return getKlineIndicatorDefinition(indicator)?.kind === "pane";
}

export function isKlineOverlayIndicator(indicator: KlineIndicatorKey): boolean {
  return getKlineIndicatorDefinition(indicator)?.kind === "overlay";
}

export interface RealtimeKlineSnapshot {
  price: number;
  volume: number;
  at: string;
  observedAt?: string | null;
  barVolume?: number | null;
  barOpen?: number | null;
  barHigh?: number | null;
  barLow?: number | null;
  session?: string | null;
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

export function normalizeKlineIndicators(
  indicators: readonly string[],
): KlineIndicatorKey[] {
  const normalized = indicators.filter(
    (indicator): indicator is KlineIndicatorKey =>
      KLINE_INDICATOR_SET.has(indicator as KlineIndicatorKey),
  );

  if (normalized.length === 0) {
    return ["volume"];
  }

  return KLINE_INDICATORS.map((indicator) => indicator.value).filter((value) =>
    normalized.includes(value),
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

function parseCandleTime(at: string | null | undefined): number | null {
  if (at == null || at === "") {
    return null;
  }

  const timestamp = new Date(at).getTime();
  return Number.isFinite(timestamp) ? timestamp : null;
}

export function resolveKlineCandleDisplayAt(candle: KlineCandle): string {
  const explicitDisplayAt = candle.displayAt?.trim();
  if (explicitDisplayAt != null && explicitDisplayAt !== "") {
    return explicitDisplayAt;
  }

  return resolveKlineBucketDisplayAt(candle.period, candle.at) ?? candle.at;
}

function shouldDisplayBucketEnd(period: string | null | undefined): boolean {
  switch (period) {
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
      return true;
    default:
      return false;
  }
}

export function resolveKlineBucketDisplayAt(
  period: string | null | undefined,
  bucketAt: string,
): string | null {
  if (period == null) {
    return null;
  }

  if (!shouldDisplayBucketEnd(period)) {
    return null;
  }

  const durationMs = resolveKlinePeriodDurationMs(period);
  const bucketTime = parseCandleTime(bucketAt);
  if (durationMs == null || bucketTime == null) {
    return null;
  }

  return new Date(bucketTime + durationMs).toISOString();
}

function shiftMinuteBucket(date: Date, shift: number): Date {
  const minute = date.getUTCMinutes() - (date.getUTCMinutes() % shift);
  return new Date(
    Date.UTC(
      date.getUTCFullYear(),
      date.getUTCMonth(),
      date.getUTCDate(),
      date.getUTCHours(),
      minute,
      0,
      0,
    ),
  );
}

function truncateSnapshotTimeToPeriod(
  timestampMs: number,
  period: string,
): string | null {
  const date = new Date(timestampMs);
  if (!Number.isFinite(date.getTime())) {
    return null;
  }

  switch (period) {
    case "1m":
      date.setUTCSeconds(0, 0);
      return date.toISOString();
    case "3m":
      return shiftMinuteBucket(date, 3).toISOString();
    case "5m":
      return shiftMinuteBucket(date, 5).toISOString();
    case "10m":
      return shiftMinuteBucket(date, 10).toISOString();
    case "15m":
      return shiftMinuteBucket(date, 15).toISOString();
    case "30m":
      return shiftMinuteBucket(date, 30).toISOString();
    case "1h":
      date.setUTCMinutes(0, 0, 0);
      return date.toISOString();
    case "1d":
      return new Date(
        Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()),
      ).toISOString();
    case "1w": {
      const weekday = date.getUTCDay();
      const distanceFromMonday = weekday === 0 ? 6 : weekday - 1;
      return new Date(
        Date.UTC(
          date.getUTCFullYear(),
          date.getUTCMonth(),
          date.getUTCDate() - distanceFromMonday,
        ),
      ).toISOString();
    }
    default:
      return null;
  }
}

function resolveSnapshotTimelineAt(snapshot: RealtimeKlineSnapshot): string {
  return snapshot.observedAt?.trim() || snapshot.at;
}

function findCandleInPeriod(
  candles: readonly KlineCandle[],
  bucketStart: string,
  period: string,
): KlineCandle | null {
  for (let index = candles.length - 1; index >= 0; index--) {
    const candle = candles[index];
    const candleTime = parseCandleTime(candle?.at);
    if (candleTime == null) {
      continue;
    }
    if (truncateSnapshotTimeToPeriod(candleTime, period) === bucketStart) {
      return candle ?? null;
    }
  }
  return null;
}

function maxRealtimeOverlayGapMs(
  period: string,
  durationMs: number,
): number | null {
  if (period === "1d" || period === "1w") {
    return null;
  }
  return Math.max(durationMs * 3, 15 * 60_000);
}

function resolveRealtimeOverlayDisplayAt(
  period: string,
  bucketStart: string,
): string | null {
  return resolveKlineBucketDisplayAt(period, bucketStart);
}

export function resolveRealtimeBucketStart(
  candles: readonly KlineCandle[],
  snapshot: RealtimeKlineSnapshot,
  period: string,
): string | null {
  const durationMs = resolveKlinePeriodDurationMs(period);
  if (durationMs == null) {
    return null;
  }

  const snapshotTime = parseCandleTime(resolveSnapshotTimelineAt(snapshot));
  if (snapshotTime == null) {
    return null;
  }

  const snapshotBucketStart = truncateSnapshotTimeToPeriod(snapshotTime, period);
  const snapshotBucketTime = parseCandleTime(snapshotBucketStart);
  if (snapshotBucketStart == null || snapshotBucketTime == null) {
    return null;
  }

  const existingBucket = findCandleInPeriod(candles, snapshotBucketStart, period);
  if (existingBucket != null) {
    return existingBucket.at;
  }

  const latestHistoricalBucket = candles[candles.length - 1];
  const latestHistoricalBucketTime = parseCandleTime(latestHistoricalBucket?.at);
  if (latestHistoricalBucketTime != null) {
    const latestBucketStart = truncateSnapshotTimeToPeriod(
      latestHistoricalBucketTime,
      period,
    );
    const latestBucketTime = parseCandleTime(latestBucketStart);
    if (latestBucketTime == null || snapshotBucketTime <= latestBucketTime) {
      return null;
    }

    const maxGap = maxRealtimeOverlayGapMs(period, durationMs);
    if (maxGap != null && snapshotBucketTime - latestBucketTime > maxGap) {
      return null;
    }
  }

  return snapshotBucketStart;
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

  const timelineAt = resolveSnapshotTimelineAt(snapshot);
  const tickTime = new Date(timelineAt).getTime();
  if (!Number.isFinite(tickTime)) {
    return [...candles];
  }

  if (period === "tick") {
    const tickCandle: KlineCandle = {
      period: "tick",
      at: timelineAt,
      open: snapshot.price,
      high: snapshot.price,
      low: snapshot.price,
      close: snapshot.price,
      volume: snapshot.barVolume ?? snapshot.volume,
    };
    if (snapshot.session !== undefined) {
      tickCandle.session = snapshot.session;
    }
    return mergeDisplayCandles(candles, [
      tickCandle,
    ]);
  }

  const durationMs = resolveKlinePeriodDurationMs(period);
  if (durationMs == null) {
    return [...candles];
  }

  const bucketStart = resolveRealtimeBucketStart(candles, snapshot, period);
  if (bucketStart == null) {
    return [...candles];
  }
  const existing = candles.find((candle) => candle.at === bucketStart);
  const displayAt = resolveRealtimeOverlayDisplayAt(period, bucketStart);
  const last = candles[candles.length - 1];
  const baseOpen =
    snapshot.barOpen ?? existing?.open ?? last?.close ?? snapshot.price;
  const baseVolume = existing?.volume ?? 0;
  const overlayHigh = snapshot.barHigh ?? snapshot.price;
  const overlayLow = snapshot.barLow ?? snapshot.price;

  const session = snapshot.session ?? existing?.session;
  const mergedCandle: KlineCandle = {
    period,
    at: bucketStart,
    open: baseOpen,
    high: Math.max(existing?.high ?? baseOpen, overlayHigh),
    low: Math.min(existing?.low ?? baseOpen, overlayLow),
    close: snapshot.price,
    volume: snapshot.barVolume ?? Math.max(baseVolume, snapshot.volume),
  };
  if (displayAt != null) {
    mergedCandle.displayAt = displayAt;
  }
  if (session !== undefined) {
    mergedCandle.session = session;
  }

  return mergeDisplayCandles(candles, [mergedCandle]);
}
