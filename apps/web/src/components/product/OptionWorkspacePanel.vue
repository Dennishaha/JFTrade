<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

import { fetchEnvelopeWithInit } from "../../composables/apiClient";
import {
  useBrokerProviderSelection,
  withBrokerProvider,
} from "../../composables/brokerProviderSelection";
import { productCompactMenuProps } from "../../composables/productControlDensity";
import {
  buildOptionChainRows,
  formatOptionMetric,
  normalizeOptionExpirations,
  type OptionChainSideModel,
  type OptionExpirationModel,
} from "../../composables/optionChainModel";
import {
  type OptionComboSide,
  type OptionContractChoice,
  useOptionComboDraftStore,
} from "../../composables/optionComboDraft";
import {
  fetchProductFeature,
  type ProductFeatureResult,
} from "../../composables/productFeatures";
import { usePolling } from "../../composables/usePolling";
import OptionChainTable from "./OptionChainTable.vue";
import OptionContractAnalysisDrawer from "./OptionContractAnalysisDrawer.vue";
import OptionResearchPanel from "./OptionResearchPanel.vue";
import ProductFeaturePanel from "./ProductFeaturePanel.vue";
import ProductPanelToolbar from "./ProductPanelToolbar.vue";

type Entry = Record<string, unknown>;
type OptionSection = "chain" | "analysis" | "events" | "strategy";
type StrikeRange = "all" | "near_atm";

const props = withDefaults(
  defineProps<{
    instrumentId: string;
    displayInstrumentId?: string;
    underlyingPending?: boolean;
    market: string;
    underlyingProductClass?: string;
  }>(),
  {
    displayInstrumentId: "",
    underlyingPending: false,
    underlyingProductClass: "equity",
  },
);
const emit = defineEmits<{ openInstrument: [instrumentId: string] }>();

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
const featurePath = computed(() => {
  let path = "";
  if (section.value === "events") {
    return "";
  }
  if (!underlyingResolved.value) return "";
  if (section.value === "analysis") {
    path = `/api/v1/market-data/options/analysis/${encodedInstrument.value}?operation=${analysisOperation.value}&pageSize=100`;
    return withBrokerProvider(path, selectedBrokerId.value);
  }
  path = `/api/v1/market-data/options/analysis/${encodedInstrument.value}?operation=strategy&option_strategy=${strategyType.value}&pageSize=100`;
  return withBrokerProvider(path, selectedBrokerId.value);
});

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

  const query = new URLSearchParams({
    beginTime: expiry,
    endTime: expiry,
  });
  let request!: Promise<Entry | null>;
  request = fetchProductFeature(
    withBrokerProvider(
      `/api/v1/market-data/options/chains/${encodedInstrument.value}?${query}`,
      selectedBrokerId.value,
    ),
  )
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
    const response = await fetchProductFeature(
      withBrokerProvider(
        `/api/v1/market-data/options/expirations/${encodedInstrument.value}`,
        selectedBrokerId.value,
      ),
    );
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
    const response = await fetchEnvelopeWithInit<ProductFeatureResult>(
      withBrokerProvider(
        `/api/v1/market-data/snapshots?market=${encodeURIComponent(props.market)}`,
        selectedBrokerId.value,
      ),
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ instrumentIds: unique }),
      },
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
</script>

<template>
  <section class="option-workspace">
    <ProductPanelToolbar title="期权工作台">
      <div class="option-workspace__stats">
        <span
          >标的价
          <strong>{{ formatOptionMetric(underlyingPrice) }}</strong></span
        >
        <span
          >到期 <strong>{{ expirations.length || "—" }}</strong></span
        >
        <span
          >合约 <strong>{{ optionRows.length || "—" }}</strong></span
        >
        <span
          >ATM <strong>{{ formatOptionMetric(atmStrike) }}</strong></span
        >
      </div>
    </ProductPanelToolbar>

    <nav class="option-workspace__sections" aria-label="期权工作台视图">
      <v-btn-toggle
        v-model="section"
        class="product-segmented-control tv-scrollbar"
        mandatory
        density="compact"
      >
        <v-btn
          v-for="item in sectionItems"
          :key="item.value"
          :value="item.value"
        >
          <span>{{ item.label }}</span>
        </v-btn>
      </v-btn-toggle>

      <v-select
        v-if="section === 'analysis'"
        v-model="analysisOperation"
        class="product-compact-control"
        :items="[
          { title: '标的概览', value: 'underlying_overview' },
          { title: 'Put / Call 与市场统计', value: 'market_statistics' },
          { title: '历史统计', value: 'historical_statistics' },
          { title: '历史波动率', value: 'historical_volatility' },
        ]"
        :menu-props="productCompactMenuProps"
        density="compact"
        variant="outlined"
        hide-details
        label="分析"
      />
      <v-select
        v-else-if="section === 'events'"
        v-model="eventOperation"
        class="product-compact-control"
        :items="eventItems"
        :menu-props="productCompactMenuProps"
        density="compact"
        variant="outlined"
        hide-details
        label="事件"
      />
      <v-select
        v-else-if="section === 'strategy'"
        v-model="strategyType"
        class="product-compact-control"
        :items="[
          { title: '跨式', value: '1' },
          { title: '宽跨', value: '2' },
          { title: '垂直价差', value: '3' },
          { title: '日历价差', value: '4' },
          { title: '蝶式', value: '5' },
        ]"
        :menu-props="productCompactMenuProps"
        density="compact"
        variant="outlined"
        hide-details
        label="策略"
      />
    </nav>

    <v-progress-linear
      v-if="loading"
      class="option-workspace__chain-progress"
      indeterminate
    />
    <v-alert
      v-if="section === 'chain' && expirationError"
      type="warning"
      variant="tonal"
      density="compact"
    >
      {{ expirationError }}
    </v-alert>
    <v-alert
      v-if="section === 'chain' && chainError"
      type="warning"
      variant="tonal"
      density="compact"
    >
      {{ chainError }}
    </v-alert>
    <v-alert
      v-if="section === 'chain' && snapshotError"
      type="warning"
      variant="tonal"
      density="compact"
    >
      {{ snapshotError }}
    </v-alert>

    <div
      v-if="needsUnderlying && !underlyingResolved"
      class="option-workspace__resolution"
    >
      <span class="option-workspace__resolution-icon">⌁</span>
      <strong>
        {{
          underlyingPending
            ? "正在识别当前期权合约的正股标的"
            : "当前产品没有可用的期权标的"
        }}
      </strong>
      <p>未解析成功前不会使用当前合约代码查询期权链或期权分析。</p>
    </div>

    <div
      v-else-if="section === 'chain'"
      class="option-workspace__chain tv-scrollbar"
    >
      <div class="option-workspace__expiry-bar">
        <div class="option-workspace__expiry-list">
          <button
            v-for="expiry in primaryExpirations"
            :key="expiry.date"
            type="button"
            :class="{ 'is-active': expiry.date === selectedExpiry }"
            @click="selectExpiry(expiry.date)"
          >
            <strong>{{ formatExpiry(expiry.date) }}</strong>
            <span>{{ expiry.daysToExpiry }}天{{ expiry.cycleLabel ? ` · ${expiry.cycleLabel}` : "" }}</span>
          </button>
        </div>
        <button
          v-if="remainingExpirations.length > 0"
          type="button"
          class="option-workspace__expiry-expand"
          :class="{ 'is-expanded': showAllExpirations }"
          :aria-expanded="showAllExpirations"
          :aria-label="
            showAllExpirations ? '收起全部到期日' : '展开全部到期日'
          "
          @click="toggleAllExpirations"
        >
          <span class="fa-solid fa-chevron-down" aria-hidden="true" />
        </button>
      </div>
      <div
        v-if="showAllExpirations && remainingExpirations.length > 0"
        class="option-workspace__expiry-more tv-scrollbar"
        role="group"
        aria-label="其余全部到期日"
      >
        <button
          v-for="expiry in remainingExpirations"
          :key="expiry.date"
          type="button"
          :class="{ 'is-active': expiry.date === selectedExpiry }"
          @click="selectExpiry(expiry.date)"
        >
          <strong>{{ formatExpiry(expiry.date) }}</strong>
          <span>{{ expiry.daysToExpiry }}天{{ expiry.cycleLabel ? ` · ${expiry.cycleLabel}` : "" }}</span>
        </button>
      </div>

      <div class="option-workspace__filters">
        <div class="option-workspace__expiry-coverage">
          <span>到期日范围：全部未到期</span>
          <strong v-if="furthestExpiry"
            >覆盖至 {{ formatExpiry(furthestExpiry) }}</strong
          >
        </div>
        <div class="option-workspace__range-toggle">
          <button
            type="button"
            :class="{ 'is-active': strikeRange === 'all' }"
            @click="strikeRange = 'all'"
          >
            全部行权价
          </button>
          <button
            type="button"
            :class="{ 'is-active': strikeRange === 'near_atm' }"
            @click="strikeRange = 'near_atm'"
          >
            ATM 附近
          </button>
        </div>
      </div>

      <div class="option-workspace__table tv-scrollbar">
        <OptionChainTable
          :rows="visibleChainRows"
          :underlying-instrument-id="normalizedUnderlying"
          :underlying-price="underlyingPrice"
          :selected-legs="comboDraft.legs.value"
          @open-contract="openContract"
          @select-leg="selectComboLeg"
        />
      </div>
      <v-pagination
        v-if="chainPageCount > 1"
        v-model="chainPage"
        :length="chainPageCount"
        :total-visible="7"
        density="compact"
      />
      <div
        v-if="expirations.length === 0 && !loading && !expirationError"
        class="option-workspace__empty"
      >
        当前标的暂无未到期期权合约。
      </div>
      <div
        v-else-if="activeChain != null && optionRows.length === 0 && !loading && !chainError"
        class="option-workspace__empty"
      >
        该到期日暂无期权合约。
      </div>
    </div>

    <OptionResearchPanel
      v-else-if="section === 'events'"
      :market="market"
      :operation="
        eventOperation as 'unusual' | 'zero_dte' | 'earnings' | 'seller'
      "
      scope="underlying"
      :underlying-instrument-id="normalizedUnderlying"
      :underlying-product-class="underlyingProductClass"
      @open-instrument="
        (instrumentId) => emit('openInstrument', instrumentId)
      "
    />

    <ProductFeaturePanel
      v-else
      :key="featurePath"
      :title="section === 'strategy' ? '合法价差与策略' : '期权研究'"
      :path="featurePath"
      :active="Boolean(featurePath)"
      @open-instrument="emit('openInstrument', $event)"
    />

    <OptionContractAnalysisDrawer
      :contract="selectedContract"
      :market="market"
      @close="selectedContract = null"
      @open-workspace="emit('openInstrument', $event)"
    />
  </section>
</template>

<style scoped src="./optionWorkspace.css"></style>
