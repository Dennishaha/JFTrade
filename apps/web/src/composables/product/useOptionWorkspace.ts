import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

import { apiPostPath } from "@/composables/shared/apiClient";
import {
  useBrokerProviderSelection,
  withBrokerProvider,
} from "@/composables/trading/brokerProviderSelection";
import { productCompactMenuProps } from "@/composables/product/productControlDensity";
import {
  buildOptionChainRows,
  formatOptionMetric,
  normalizeOptionExpirations,
  type OptionChainSideModel,
  type OptionExpirationModel,
} from "@/composables/product/optionChainModel";
import {
  type OptionComboSide,
  type OptionContractChoice,
  useOptionComboDraftStore,
} from "@/composables/product/optionComboDraft";
import {
  fetchProductFeature,
  prepareProductFeature,
  type ProductFeatureResult,
} from "@/composables/product/productFeatures";
import { usePolling } from "@/composables/shared/usePolling";
import { productFeaturePath } from "@/composables/product/productFeatureApi";

type Entry = Record<string, unknown>;
type OptionSection = "chain" | "analysis" | "events" | "strategy";
type StrikeRange = "all" | "near_atm";

export interface OptionWorkspaceProps {
  instrumentId: string;
  displayInstrumentId?: string;
  underlyingPending?: boolean;
  market: string;
  underlyingProductClass?: string;
}

export function useOptionWorkspace(props: Readonly<OptionWorkspaceProps>) {
const section = ref<OptionSection>("chain");
const expirationLoading = ref(false);
const chainLoading = ref(false);
const expirationError = ref("");
const chainError = ref("");
const snapshotError = ref("");
const expirationResult = ref<ProductFeatureResult | null>(null);
const chainsByExpiry = ref<Record<string, Entry>>({});
const snapshots = ref<Record<string, Entry>>({});
const selectedExpiry = ref("");
const showAllExpirations = ref(false);
const strikeRange = ref<StrikeRange>("all");
const chainPage = ref(1);
const rowsPerPage = 20;
const primaryExpiryLimit = 4;
let expirationRequestToken = 0;
let chainRequestToken = 0;
let snapshotRequestToken = 0;
let snapshotRequestInFlight = false;
let snapshotRefreshPending = false;
let disposed = false;
const chainRequests = new Map<string, Promise<Entry | null>>();
const snapshotPolling = usePolling(
  () => loadVisibleSnapshots(),
  { intervalMs: 3_000 },
);

const analysisOperation = ref("underlying_overview");
const eventOperation = ref("unusual");
const strategyType = ref("1");
const selectedContract = ref<OptionChainSideModel | null>(null);
const { selectedBrokerId } = useBrokerProviderSelection();
const comboDraft = useOptionComboDraftStore();

const sectionItems: Array<{
  value: OptionSection;
  label: string;
}> = [
  { value: "chain", label: "期权链" },
  { value: "analysis", label: "波动率与统计" },
  { value: "events", label: "0DTE 与异动" },
  { value: "strategy", label: "策略生成器" },
];
const eventItems = computed(() => [
  { title: "异动", value: "unusual" },
  ...(props.market.trim().toUpperCase() === "US"
    ? [{ title: "0DTE 标的", value: "zero_dte" }]
    : []),
  { title: "财报期权", value: "earnings" },
  { title: "卖方筛选", value: "seller" },
]);

const normalizedUnderlying = computed(() =>
  props.instrumentId.trim().toUpperCase(),
);
const needsUnderlying = computed(() => true);
const underlyingResolved = computed(() => normalizedUnderlying.value !== "");
const loading = computed(
  () =>
    section.value === "chain" &&
    (expirationLoading.value || chainLoading.value),
);
const expirations = computed<OptionExpirationModel[]>(() =>
  normalizeOptionExpirations(expirationResult.value?.entries ?? []),
);
const primaryExpirations = computed<OptionExpirationModel[]>(() => {
  const primary = expirations.value.slice(0, primaryExpiryLimit);
  const selected = expirations.value.find(
    (expiry) => expiry.date === selectedExpiry.value,
  );
  if (
    selected == null ||
    primary.some((expiry) => expiry.date === selected.date)
  ) {
    return primary;
  }
  return [...primary.slice(0, primaryExpiryLimit - 1), selected];
});
const remainingExpirations = computed<OptionExpirationModel[]>(() => {
  const primaryDates = new Set(
    primaryExpirations.value.map((expiry) => expiry.date),
  );
  return expirations.value.filter((expiry) => !primaryDates.has(expiry.date));
});
const furthestExpiry = computed(
  () => expirations.value[expirations.value.length - 1]?.date ?? "",
);
const nextExpiry = computed(() => {
  const index = expirations.value.findIndex(
    (expiry) => expiry.date === selectedExpiry.value,
  );
  return index >= 0 ? (expirations.value[index + 1]?.date ?? "") : "";
});
const activeChain = computed<Entry | null>(
  () => chainsByExpiry.value[selectedExpiry.value] ?? null,
);
const optionRows = computed<Entry[]>(() => {
  const options = activeChain.value?.option;
  return Array.isArray(options) ? (options as Entry[]) : [];
});
const underlyingPrice = computed(() => {
  const value = Number(
    snapshotForInstrument(normalizedUnderlying.value).lastPrice,
  );
  return Number.isFinite(value) ? value : null;
});
const chainRows = computed(() =>
  buildOptionChainRows(
    optionRows.value,
    snapshots.value,
    props.market,
    underlyingPrice.value,
  ),
);
const rangedChainRows = computed(() => {
  if (strikeRange.value === "all" || chainRows.value.length <= rowsPerPage) {
    return chainRows.value;
  }
  const atmIndex = chainRows.value.findIndex((row) => row.isAtm);
  if (atmIndex < 0) return chainRows.value.slice(0, rowsPerPage);
  const start = Math.max(0, atmIndex - Math.floor(rowsPerPage / 2));
  return chainRows.value.slice(start, start + rowsPerPage);
});
const chainPageCount = computed(() =>
  Math.max(1, Math.ceil(rangedChainRows.value.length / rowsPerPage)),
);
const visibleChainRows = computed(() => {
  const start = (chainPage.value - 1) * rowsPerPage;
  return rangedChainRows.value.slice(start, start + rowsPerPage);
});
const visibleOptionRows = computed(() => {
  const visibleKeys = new Set(visibleChainRows.value.map((row) => row.key));
  return optionRows.value.filter((_, index) => {
    const strike = chainRows.value[index]?.strike;
    return visibleKeys.has(`${strike ?? "unknown"}-${index}`);
  });
});
const atmStrike = computed(
  () => chainRows.value.find((row) => row.isAtm)?.strike ?? null,
);
const comboContracts = computed<OptionContractChoice[]>(() => {
  const choices: OptionContractChoice[] = [];
  const seen = new Set<string>();
  const comboChains = [selectedExpiry.value, nextExpiry.value]
    .map((expiry) => chainsByExpiry.value[expiry])
    .filter((chain): chain is Entry => chain != null);
  for (const chain of comboChains) {
    const expiry = String(chain.strikeTime ?? "").trim();
    const options = Array.isArray(chain.option) ? (chain.option as Entry[]) : [];
    const rows = buildOptionChainRows(
      options,
      {},
      props.market,
      null,
    );
    for (const row of rows) {
      for (const side of [row.call, row.put]) {
        if (!side.code || row.strike == null || seen.has(side.code)) continue;
        seen.add(side.code);
        choices.push({
          instrumentId: side.instrumentId,
          code: side.code,
          name: side.name || side.code,
          label: `${expiry} · ${side === row.call ? "CALL" : "PUT"} ${row.strike} · ${side.name || side.code}`,
          optionType: side === row.call ? "call" : "put",
          strike: row.strike,
          multiplier: side.multiplier,
          expiry,
          bidPrice: side.bidPrice,
          askPrice: side.askPrice,
        });
      }
    }
  }
  return choices;
});
const snapshotDependencyKey = computed(() => {
  const visibleInstrumentIds = visibleChainRows.value.flatMap((row) => [
    row.call.instrumentId,
    row.put.instrumentId,
  ]);
  return [
    normalizedUnderlying.value,
    selectedExpiry.value,
    String(chainPage.value),
    strikeRange.value,
    selectedBrokerId.value,
    ...visibleInstrumentIds,
    ...comboDraft.selectedLegInstrumentIds.value,
  ]
    .map((value) => String(value ?? "").trim().toUpperCase())
    .join("|");
});
const encodedInstrument = computed(() =>
  encodeURIComponent(normalizedUnderlying.value),
);
const featureRequest = computed(() => {
  if (section.value === "events") {
    return null;
  }
  if (!underlyingResolved.value) return null;
  if (section.value === "analysis") {
    return {
      scope: "market-feature" as const,
      resource: "option-analysis" as const,
      brokerId: selectedBrokerId.value,
      instrumentId: normalizedUnderlying.value,
      operation: analysisOperation.value,
      pageSize: 100,
    };
  }
  return {
    scope: "market-feature" as const,
    resource: "option-analysis" as const,
    brokerId: selectedBrokerId.value,
    instrumentId: normalizedUnderlying.value,
    operation: "strategy",
    optionStrategy: strategyType.value,
    pageSize: 100,
  };
});
const featurePath = computed(() =>
  featureRequest.value == null ? "" : productFeaturePath(featureRequest.value),
);

function snapshotForInstrument(value: string): Entry {
  return snapshots.value[value.trim().toUpperCase()] ?? {};
}

function selectExpiry(value: string): void {
  selectedExpiry.value = value;
  showAllExpirations.value = false;
  chainPage.value = 1;
}

function toggleAllExpirations(): void {
  showAllExpirations.value = !showAllExpirations.value;
}

function formatExpiry(value: string): string {
  return /^\d{4}-\d{2}-\d{2}$/.test(value)
    ? value.replaceAll("-", "/")
    : value;
}

function openContract(contract: OptionChainSideModel): void {
  if (contract.instrumentId) selectedContract.value = contract;
}

function selectComboLeg(
  contract: OptionChainSideModel,
  side: OptionComboSide,
): void {
  const choice = comboContracts.value.find(
    (candidate) =>
      candidate.instrumentId.trim().toUpperCase() ===
      contract.instrumentId.trim().toUpperCase(),
  );
  if (choice != null) comboDraft.toggleLeg(choice, side);
}

function nextExpiryAfter(expiry: string): string {
  const index = expirations.value.findIndex((item) => item.date === expiry);
  return index >= 0 ? (expirations.value[index + 1]?.date ?? "") : "";
}

function requestExpiryChain(
  expiry: string,
  expirationToken: number,
): Promise<Entry | null> {
  const cached = chainsByExpiry.value[expiry];
  if (cached != null) return Promise.resolve(cached);
  const inFlight = chainRequests.get(expiry);
  if (inFlight != null) return inFlight;

  let request!: Promise<Entry | null>;
  request = fetchProductFeature(prepareProductFeature({
    scope: "market-feature",
    resource: "option-chains",
    brokerId: selectedBrokerId.value,
    instrumentId: normalizedUnderlying.value,
    beginTime: expiry,
    endTime: expiry,
  }))
    .then((response) => {
      if (disposed || expirationToken !== expirationRequestToken) return null;
      const chain =
        response.entries.find(
          (entry) => String(entry.strikeTime ?? "").trim() === expiry,
        ) ?? { strikeTime: expiry, option: [] };
      chainsByExpiry.value = {
        ...chainsByExpiry.value,
        [expiry]: chain,
      };
      return chain;
    })
    .finally(() => {
      if (chainRequests.get(expiry) === request) chainRequests.delete(expiry);
    });
  chainRequests.set(expiry, request);
  return request;
}

async function prefetchNextExpiry(
  expiry: string,
  expirationToken: number,
): Promise<void> {
  const followingExpiry = nextExpiryAfter(expiry);
  if (
    !followingExpiry ||
    disposed ||
    expirationToken !== expirationRequestToken
  ) {
    return;
  }
  try {
    await requestExpiryChain(followingExpiry, expirationToken);
  } catch {
    // Prefetch failures stay silent; a foreground selection retries the request.
  }
}

async function loadSelectedChain(): Promise<void> {
  const expiry = selectedExpiry.value;
  const token = ++chainRequestToken;
  if (!underlyingResolved.value || !expiry) {
    chainLoading.value = false;
    chainError.value = "";
    return;
  }
  const expirationToken = expirationRequestToken;
  chainLoading.value = true;
  chainError.value = "";
  try {
    await requestExpiryChain(expiry, expirationToken);
    if (
      token !== chainRequestToken ||
      expirationToken !== expirationRequestToken ||
      expiry !== selectedExpiry.value
    ) {
      return;
    }
    chainPage.value = 1;
    void prefetchNextExpiry(expiry, expirationToken);
  } catch (cause) {
    if (token !== chainRequestToken || expiry !== selectedExpiry.value) return;
    chainError.value = cause instanceof Error ? cause.message : String(cause);
  } finally {
    if (token === chainRequestToken) chainLoading.value = false;
  }
}

async function loadExpirationCatalog(): Promise<void> {
  const token = ++expirationRequestToken;
  chainRequestToken += 1;
  chainRequests.clear();
  expirationResult.value = null;
  chainsByExpiry.value = {};
  selectedExpiry.value = "";
  showAllExpirations.value = false;
  expirationError.value = "";
  chainError.value = "";
  chainLoading.value = false;
  if (!underlyingResolved.value) {
    snapshots.value = {};
    expirationLoading.value = false;
    return;
  }
  expirationLoading.value = true;
  try {
    const response = await fetchProductFeature(prepareProductFeature({
      scope: "market-feature",
      resource: "option-expirations",
      brokerId: selectedBrokerId.value,
      instrumentId: normalizedUnderlying.value,
    }));
    if (disposed || token !== expirationRequestToken) return;
    expirationResult.value = response;
    selectedExpiry.value =
      normalizeOptionExpirations(response.entries)[0]?.date ?? "";
  } catch (cause) {
    if (token !== expirationRequestToken) return;
    expirationError.value =
      cause instanceof Error ? cause.message : String(cause);
  } finally {
    if (token === expirationRequestToken) expirationLoading.value = false;
  }
}

async function loadVisibleSnapshots(): Promise<void> {
  if (
    disposed ||
    section.value !== "chain" ||
    !underlyingResolved.value ||
    (typeof document !== "undefined" && document.hidden)
  ) {
    snapshotRefreshPending = false;
    return;
  }
  if (snapshotRequestInFlight) {
    snapshotRefreshPending = true;
    return;
  }
  const token = ++snapshotRequestToken;
  const dependencyKey = snapshotDependencyKey.value;
  const instrumentIds = [
    normalizedUnderlying.value,
    ...visibleChainRows.value.flatMap((row) => [
      row.call.instrumentId,
      row.put.instrumentId,
    ]),
    ...comboDraft.selectedLegInstrumentIds.value,
  ];
  const unique = [
    ...new Set(
      instrumentIds.map((value) => value.trim().toUpperCase()).filter(Boolean),
    ),
  ];
  if (unique.length === 0) return;
  snapshotRequestInFlight = true;
  snapshotRefreshPending = false;
  try {
    const response = await apiPostPath(
      "/api/v1/market-data/snapshots",
      withBrokerProvider(
        `/api/v1/market-data/snapshots?market=${encodeURIComponent(props.market)}`,
        selectedBrokerId.value,
      ),
      { instrumentIds: unique },
    );
    if (
      token !== snapshotRequestToken ||
      dependencyKey !== snapshotDependencyKey.value
    ) {
      return;
    }
    const next: Record<string, Entry> = {};
    for (const entry of response.entries) {
      const value = String(entry.instrumentId ?? entry.symbol ?? "")
        .trim()
        .toUpperCase();
      const instrumentId =
        value && !value.includes(".")
          ? `${props.market.trim().toUpperCase()}.${value}`
          : value;
      if (instrumentId) next[instrumentId] = entry;
    }
    snapshots.value = { ...snapshots.value, ...next };
    comboDraft.updateQuotes(
      Object.entries(next).map(([instrumentId, entry]) => {
        const bidPrice = Number(entry.bidPrice);
        const askPrice = Number(entry.askPrice);
        return {
          instrumentId,
          bidPrice: Number.isFinite(bidPrice) ? bidPrice : null,
          askPrice: Number.isFinite(askPrice) ? askPrice : null,
        };
      }),
    );
    snapshotError.value = "";
  } catch {
    if (token === snapshotRequestToken) {
      snapshotError.value =
        "实时合约价格暂不可用，期权链与行权价仍可查看。";
    }
  } finally {
    snapshotRequestInFlight = false;
    if (snapshotRefreshPending && !disposed) {
      snapshotRefreshPending = false;
      void loadVisibleSnapshots();
    }
  }
}

watch(
  [normalizedUnderlying, selectedBrokerId],
  () => void loadExpirationCatalog(),
  { immediate: true },
);
watch(selectedExpiry, () => void loadSelectedChain());
watch(snapshotDependencyKey, () => void loadVisibleSnapshots());
watch(
  [normalizedUnderlying, () => props.market],
  ([instrumentId, market]) => {
    comboDraft.setContext(instrumentId ?? "", market ?? "");
  },
  { immediate: true },
);
watch(
  comboContracts,
  (contracts) => comboDraft.setContracts(contracts),
  { immediate: true },
);
watch(
  [normalizedUnderlying, () => props.market, selectedBrokerId],
  () => {
    selectedContract.value = null;
    snapshots.value = {};
    snapshotError.value = "";
  },
);
watch(
  () => props.market,
  (market) => {
    if (
      market.trim().toUpperCase() !== "US" &&
      eventOperation.value === "zero_dte"
    ) {
      eventOperation.value = "unusual";
    }
  },
);

onMounted(() => {
  comboDraft.setWorkspaceActive(true);
  snapshotPolling.start();
});
onBeforeUnmount(() => {
  comboDraft.setWorkspaceActive(false);
  disposed = true;
  expirationRequestToken += 1;
  chainRequestToken += 1;
  snapshotRequestToken += 1;
  chainRequests.clear();
  snapshotRefreshPending = false;
  expirationLoading.value = false;
  chainLoading.value = false;
});

  return {
    section,
    expirationLoading,
    chainLoading,
    expirationError,
    chainError,
    snapshotError,
    expirationResult,
    chainsByExpiry,
    snapshots,
    selectedExpiry,
    showAllExpirations,
    strikeRange,
    chainPage,
    rowsPerPage,
    primaryExpiryLimit,
    chainRequests,
    snapshotPolling,
    analysisOperation,
    eventOperation,
    strategyType,
    selectedContract,
    comboDraft,
    sectionItems,
    eventItems,
    normalizedUnderlying,
    needsUnderlying,
    underlyingResolved,
    loading,
    expirations,
    primaryExpirations,
    remainingExpirations,
    furthestExpiry,
    nextExpiry,
    activeChain,
    optionRows,
    underlyingPrice,
    chainRows,
    rangedChainRows,
    chainPageCount,
    visibleChainRows,
    visibleOptionRows,
    atmStrike,
    comboContracts,
    snapshotDependencyKey,
    encodedInstrument,
    featureRequest,
    featurePath,
    snapshotForInstrument,
    selectExpiry,
    toggleAllExpirations,
    formatExpiry,
    openContract,
    selectComboLeg,
    nextExpiryAfter,
    requestExpiryChain,
    prefetchNextExpiry,
    loadSelectedChain,
    loadExpirationCatalog,
    loadVisibleSnapshots,
    selectedBrokerId,
    formatOptionMetric,
    productCompactMenuProps,
  };
}
