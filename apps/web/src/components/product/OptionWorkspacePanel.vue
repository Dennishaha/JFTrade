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
  type OptionChainSideModel,
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
const loading = ref(false);
const chainError = ref("");
const snapshotError = ref("");
const result = ref<ProductFeatureResult | null>(null);
const snapshots = ref<Record<string, Entry>>({});
const selectedExpiry = ref("");
const strikeRange = ref<StrikeRange>("all");
const chainPage = ref(1);
const rowsPerPage = 20;
let chainRequestToken = 0;
let snapshotRequestToken = 0;
let snapshotRequestInFlight = false;
let snapshotRefreshPending = false;
let disposed = false;
const snapshotPolling = usePolling(
  () => loadVisibleSnapshots(),
  { intervalMs: 3_000 },
);

const today = new Date();
const endDate = new Date(today.getTime() + 30 * 86400_000);
const beginTime = ref(today.toISOString().slice(0, 10));
const endTime = ref(endDate.toISOString().slice(0, 10));
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
const expirations = computed(() => [
  ...new Set(
    (result.value?.entries ?? [])
      .map((entry) => String(entry.strikeTime ?? "").trim())
      .filter(Boolean),
  ),
]);
const activeChain = computed<Entry | null>(
  () =>
    (result.value?.entries ?? []).find(
      (entry) => String(entry.strikeTime ?? "") === selectedExpiry.value,
    ) ??
    result.value?.entries[0] ??
    null,
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
const selectedExpiryDays = computed(() => {
  if (!selectedExpiry.value) return null;
  const expiry = new Date(`${selectedExpiry.value}T00:00:00`);
  if (Number.isNaN(expiry.getTime())) return null;
  return Math.max(0, Math.ceil((expiry.getTime() - Date.now()) / 86_400_000));
});
const comboContracts = computed<OptionContractChoice[]>(() => {
  const choices: OptionContractChoice[] = [];
  const seen = new Set<string>();
  for (const chain of result.value?.entries ?? []) {
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
  chainPage.value = 1;
}

function formatExpiry(value: string): string {
  const date = new Date(`${value}T00:00:00`);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
  }).format(date);
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

async function loadChain(): Promise<void> {
  const token = ++chainRequestToken;
  if (section.value !== "chain") {
    loading.value = false;
    return;
  }
  if (!underlyingResolved.value) {
    result.value = null;
    snapshots.value = {};
    chainError.value = "";
    loading.value = false;
    return;
  }
  loading.value = true;
  chainError.value = "";
  try {
    const query = new URLSearchParams({
      beginTime: beginTime.value,
      endTime: endTime.value,
      pageSize: "200",
    });
    const response = await fetchProductFeature(
      withBrokerProvider(
        `/api/v1/market-data/options/chains/${encodedInstrument.value}?${query}`,
        selectedBrokerId.value,
      ),
    );
    if (token !== chainRequestToken) return;
    result.value = response;
    if (!expirations.value.includes(selectedExpiry.value)) {
      selectedExpiry.value = expirations.value[0] ?? "";
    }
    chainPage.value = 1;
  } catch (cause) {
    if (token !== chainRequestToken) return;
    chainError.value = cause instanceof Error ? cause.message : String(cause);
    result.value = null;
  } finally {
    if (token === chainRequestToken) loading.value = false;
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
  [normalizedUnderlying, beginTime, endTime, section, selectedBrokerId],
  () => void loadChain(),
  { immediate: true },
);
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
  chainRequestToken += 1;
  snapshotRequestToken += 1;
  snapshotRefreshPending = false;
  loading.value = false;
});
</script>

<template>
  <section class="option-workspace">
    <ProductPanelToolbar
      title="期权工作台"
    >
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
      v-if="chainError"
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
        <div class="option-workspace__expiry-list tv-scrollbar">
          <button
            v-for="expiry in expirations"
            :key="expiry"
            type="button"
            :class="{ 'is-active': expiry === selectedExpiry }"
            @click="selectExpiry(expiry)"
          >
            <strong>{{ formatExpiry(expiry) }}</strong>
            <span
              v-if="expiry === selectedExpiry && selectedExpiryDays != null"
            >
              {{ selectedExpiryDays }}天
            </span>
          </button>
        </div>
        <v-select
          v-model="selectedExpiry"
          class="option-workspace__expiry-select product-compact-control"
          :items="expirations"
          :menu-props="productCompactMenuProps"
          density="compact"
          variant="outlined"
          hide-details
          label="全部到期日"
        />
      </div>

      <div class="option-workspace__filters">
        <div class="option-workspace__date-range">
          <span>到期范围</span>
          <input v-model="beginTime" type="date" aria-label="期权开始到期日" />
          <span>至</span>
          <input v-model="endTime" type="date" aria-label="期权结束到期日" />
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
        v-if="optionRows.length === 0 && !loading"
        class="option-workspace__empty"
      >
        当前到期范围暂无期权合约，或账户没有相应行情权限。
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
