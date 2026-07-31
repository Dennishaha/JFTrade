import { computed, isRef, ref, watch, type Ref } from "vue";

import { apiPostPath } from "@/composables/shared/apiClient";

export type ResearchInstrumentIdsSource = Ref<string[]> | (() => string[]);

export interface ResearchSnapshotState {
  entries: Ref<Record<string, unknown>[]>;
  byInstrumentId: Ref<Record<string, Record<string, unknown>>>;
  loading: Ref<boolean>;
  error: Ref<string>;
  refresh: () => Promise<void>;
}

export interface ResearchSnapshotErrorDetail {
  instrumentId: string;
  code?: string;
  message: string;
}

/** A batch can return usable quotes alongside per-instrument failures. */
export class ResearchSnapshotBatchError extends Error {
  readonly quotes: Record<string, unknown>[];
  readonly errors: ResearchSnapshotErrorDetail[];

  constructor(
    quotes: Record<string, unknown>[],
    errors: ResearchSnapshotErrorDetail[],
  ) {
    const details = errors
      .map((error) => `${error.instrumentId}: ${error.message}`)
      .join("；");
    super(`部分行情加载失败：${details}`);
    this.name = "ResearchSnapshotBatchError";
    this.quotes = quotes;
    this.errors = errors;
  }
}

const RESEARCH_SNAPSHOT_BATCH_SIZE = 200;
const RESEARCH_SNAPSHOT_CONCURRENCY = 3;

function normalizedIds(source: ResearchInstrumentIdsSource): string[] {
  const values = isRef(source) ? source.value : source();
  return [
    ...new Set(
      values
        .map((value) => value.trim().toUpperCase())
        .filter((value) => value.includes(".")),
    ),
  ];
}

function snapshotInstrumentId(entry: Record<string, unknown>): string {
  return String(entry.instrumentId ?? entry.symbol ?? "")
    .trim()
    .toUpperCase();
}

function normalizeSnapshotErrors(value: unknown): ResearchSnapshotErrorDetail[] {
  if (!Array.isArray(value)) return [];
  return value
    .map((item): ResearchSnapshotErrorDetail | null => {
      if (item == null || typeof item !== "object") return null;
      const error = item as Record<string, unknown>;
      const instrumentId = String(error.instrumentId ?? "").trim().toUpperCase();
      const message = String(error.message ?? "行情快照不可用").trim();
      if (!instrumentId || !message) return null;
      const code = String(error.code ?? "").trim();
      return code ? { instrumentId, code, message } : { instrumentId, message };
    })
    .filter((error): error is ResearchSnapshotErrorDetail => error != null);
}

export async function fetchResearchSnapshots(
  instrumentIds: string[],
  _brokerId: string,
  _refresh = false,
): Promise<Record<string, unknown>[]> {
  const ids = [
    ...new Set(
      instrumentIds
        .map((value) => value.trim().toUpperCase())
        .filter((value) => value.includes(".")),
    ),
  ];
  if (ids.length === 0) return [];
  const path = "/api/v1/watchlist/quotes/batch";
  if (ids.length <= RESEARCH_SNAPSHOT_BATCH_SIZE) {
    const response = await apiPostPath(
      "/api/v1/watchlist/quotes/batch",
      path,
      { instrumentIds: ids },
    );
    const quotes = response.quotes ?? [];
    const errors = normalizeSnapshotErrors(response.errors);
    if (errors.length > 0) {
      throw new ResearchSnapshotBatchError(quotes, errors);
    }
    return quotes;
  }
  const batches: string[][] = [];
  for (let index = 0; index < ids.length; index += RESEARCH_SNAPSHOT_BATCH_SIZE) {
    batches.push(ids.slice(index, index + RESEARCH_SNAPSHOT_BATCH_SIZE));
  }
  const results: Record<string, unknown>[][] = new Array(batches.length);
  const batchErrors: ResearchSnapshotErrorDetail[][] = new Array(batches.length);
  let nextBatch = 0;

  async function worker(): Promise<void> {
    while (nextBatch < batches.length) {
      const batchIndex = nextBatch++;
      const instrumentIds = batches[batchIndex]!;
      const response = await apiPostPath(
        "/api/v1/watchlist/quotes/batch",
        path,
        { instrumentIds },
      );
      results[batchIndex] = response.quotes ?? [];
      batchErrors[batchIndex] = normalizeSnapshotErrors(response.errors);
    }
  }

  await Promise.all(
    Array.from(
      { length: Math.min(RESEARCH_SNAPSHOT_CONCURRENCY, batches.length) },
      () => worker(),
    ),
  );
  const quotes = results.flat();
  const errors = batchErrors.flat();
  if (errors.length > 0) {
    throw new ResearchSnapshotBatchError(quotes, errors);
  }
  return quotes;
}

export function useResearchSnapshots(
  instrumentIdsSource: ResearchInstrumentIdsSource,
  brokerIdSource: Ref<string> | (() => string),
): ResearchSnapshotState {
  const entries = ref<Record<string, unknown>[]>([]);
  const loading = ref(false);
  const error = ref("");
  let requestToken = 0;

  const brokerId = (): string =>
    (isRef(brokerIdSource) ? brokerIdSource.value : brokerIdSource()).trim();

  async function load(refresh = false): Promise<void> {
    const ids = normalizedIds(instrumentIdsSource);
    const token = ++requestToken;
    if (ids.length === 0) {
      entries.value = [];
      loading.value = false;
      error.value = "";
      return;
    }
    loading.value = true;
    error.value = "";
    try {
      const response = await fetchResearchSnapshots(ids, brokerId(), refresh);
      if (token === requestToken) entries.value = response;
    } catch (cause) {
      if (token !== requestToken) return;
      error.value = cause instanceof Error ? cause.message : String(cause);
      entries.value =
        cause instanceof ResearchSnapshotBatchError ? cause.quotes : [];
    } finally {
      if (token === requestToken) loading.value = false;
    }
  }

  watch(
    () => `${normalizedIds(instrumentIdsSource).join("|")}|${brokerId()}`,
    () => {
      void load();
    },
    { immediate: true },
  );

  const byInstrumentId = computed(() => {
    const result: Record<string, Record<string, unknown>> = {};
    for (const entry of entries.value) {
      const instrumentId = snapshotInstrumentId(entry);
      if (instrumentId) result[instrumentId] = entry;
    }
    return result;
  });

  return {
    entries,
    byInstrumentId: byInstrumentId as Ref<
      Record<string, Record<string, unknown>>
    >,
    loading,
    error,
    refresh: () => load(true),
  };
}

export function mergeResearchSnapshot(
  entry: Record<string, unknown>,
  snapshot: Record<string, unknown> | undefined,
): Record<string, unknown> {
  if (snapshot == null) return entry;
  const previousClose = Number(snapshot.previousClose ?? snapshot.previousClosePrice);
  const lastPrice = Number(snapshot.lastPrice ?? snapshot.price);
  const hasPrices = Number.isFinite(previousClose) && Number.isFinite(lastPrice);
  const changeAmount = hasPrices ? lastPrice - previousClose : undefined;
  const changeRate =
    hasPrices && previousClose !== 0
      ? ((lastPrice - previousClose) / previousClose) * 100
      : undefined;
  const fund =
    snapshot.fund != null && typeof snapshot.fund === "object"
      ? (snapshot.fund as Record<string, unknown>)
      : null;
  return {
    ...entry,
    ...snapshot,
    instrumentId:
      String(entry.instrumentId ?? snapshot.instrumentId ?? snapshot.symbol ?? "")
        .trim()
        .toUpperCase(),
    name: entry.name ?? snapshot.name,
    price: Number.isFinite(lastPrice) ? lastPrice : entry.price,
    assetClass: entry.assetClass ?? snapshot.assetClass ?? fund?.assetClass,
    changeAmount: entry.changeAmount ?? snapshot.change ?? changeAmount,
    changeRate: entry.changeRate ?? snapshot.changePercent ?? changeRate,
  };
}
