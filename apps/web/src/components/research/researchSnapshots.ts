import { computed, isRef, ref, watch, type Ref } from "vue";

import { fetchEnvelopeWithInit } from "../../composables/apiClient";
import { withBrokerProvider } from "../../composables/brokerProviderSelection";
import type { ProductFeatureResult } from "../../composables/productFeatures";

export type ResearchInstrumentIdsSource = Ref<string[]> | (() => string[]);

export interface ResearchSnapshotState {
  entries: Ref<Record<string, unknown>[]>;
  byInstrumentId: Ref<Record<string, Record<string, unknown>>>;
  loading: Ref<boolean>;
  error: Ref<string>;
  refresh: () => Promise<void>;
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

export async function fetchResearchSnapshots(
  instrumentIds: string[],
  brokerId: string,
  refresh = false,
): Promise<Record<string, unknown>[]> {
  const ids = [
    ...new Set(
      instrumentIds
        .map((value) => value.trim().toUpperCase())
        .filter((value) => value.includes(".")),
    ),
  ];
  if (ids.length === 0) return [];
  let path = withBrokerProvider("/api/v1/market-data/snapshots", brokerId.trim());
  if (refresh) path += `${path.includes("?") ? "&" : "?"}refresh=true`;
  if (ids.length <= RESEARCH_SNAPSHOT_BATCH_SIZE) {
    const response = await fetchEnvelopeWithInit<ProductFeatureResult>(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ instrumentIds: ids }),
    });
    return response.entries ?? [];
  }
  const batches: string[][] = [];
  for (let index = 0; index < ids.length; index += RESEARCH_SNAPSHOT_BATCH_SIZE) {
    batches.push(ids.slice(index, index + RESEARCH_SNAPSHOT_BATCH_SIZE));
  }
  const results: Record<string, unknown>[][] = new Array(batches.length);
  let nextBatch = 0;

  async function worker(): Promise<void> {
    while (nextBatch < batches.length) {
      const batchIndex = nextBatch++;
      const instrumentIds = batches[batchIndex]!;
      const response = await fetchEnvelopeWithInit<ProductFeatureResult>(path, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ instrumentIds }),
      });
      results[batchIndex] = response.entries ?? [];
    }
  }

  await Promise.all(
    Array.from(
      { length: Math.min(RESEARCH_SNAPSHOT_CONCURRENCY, batches.length) },
      () => worker(),
    ),
  );
  return results.flat();
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
      entries.value = [];
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
  const previousClose = Number(snapshot.previousClose);
  const lastPrice = Number(snapshot.lastPrice);
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
    assetClass: entry.assetClass ?? fund?.assetClass,
    changeAmount: entry.changeAmount ?? changeAmount,
    changeRate: entry.changeRate ?? changeRate,
  };
}
