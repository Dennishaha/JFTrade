<script setup lang="ts">
import { computed, ref } from "vue";

import { useResearchFeature } from "@/composables/research/useResearchFeature";
import { isProviderCapabilityMessage } from "@/composables/research/providerCapabilityFallback";
import ProviderUnsupportedState from "./ProviderUnsupportedState.vue";
import RankListPanel from "./RankListPanel.vue";
import SectorHeatmap from "./SectorHeatmap.vue";
import {
  directionClass,
  formatPrice,
  formatSigned,
  pickNumber,
  pickString,
} from "./researchEntry";
import {
  mergeResearchSnapshot,
  useResearchSnapshots,
} from "./researchSnapshots";
import EmptyState from "@/components/shared/EmptyState.vue";

const props = withDefaults(
  defineProps<{ market?: string; brokerId?: string }>(),
  { market: "US", brokerId: "" },
);

const emit = defineEmits<{
  select: [entry: Record<string, unknown>];
  more: [operation: string];
}>();

function rankingsRequest(operation: string, direction?: string, plateType?: string) {
  return {
    scope: "research" as const,
    family: "rankings" as const,
    market: props.market,
    operation,
    ...(direction ? { direction } : {}),
    ...(plateType ? { plateType } : {}),
    pageSize: 50,
  };
}

const gainersFeature = useResearchFeature(() =>
  props.market === "CN" ? null : rankingsRequest("top_movers", "up"),
  { brokerId: () => props.brokerId },
);
const losersFeature = useResearchFeature(() =>
  props.market === "CN" ? null : rankingsRequest("top_movers", "down"),
  { brokerId: () => props.brokerId },
);
const hot = useResearchFeature(() => props.market === "CN" ? null : rankingsRequest("hot"), {
  brokerId: () => props.brokerId,
});
const highDividend = useResearchFeature(() =>
  props.market === "HK" ? rankingsRequest("high_dividend_state") : null,
  { brokerId: () => props.brokerId },
);
const heatmapType = ref<"industry" | "concept" | "theme">("industry");
const heatmap = useResearchFeature(() =>
  rankingsRequest("heatmap", undefined, heatmapType.value),
  { brokerId: () => props.brokerId },
);

const PANEL_SIZE = 10;
const supportsMovers = computed(() => props.market === "US" || props.market === "HK");
const supportsBenchmarkSnapshots = computed(() => props.market !== "US");

const BENCHMARKS: Record<string, Array<{ instrumentId: string; name: string }>> = {
  US: [],
  HK: [
    { instrumentId: "HK.800000", name: "恒生指数" },
    { instrumentId: "HK.800100", name: "国企指数" },
    { instrumentId: "HK.800700", name: "恒生科技" },
  ],
  CN: [
    { instrumentId: "SH.000001", name: "上证指数" },
    { instrumentId: "SZ.399001", name: "深证成指" },
    { instrumentId: "SZ.399006", name: "创业板指" },
  ],
};

const benchmarkDefinitions = computed(
  () => BENCHMARKS[props.market] ?? [],
);
const benchmarkSnapshots = useResearchSnapshots(
  () => benchmarkDefinitions.value.map((item) => item.instrumentId),
  () => props.brokerId,
);

function canonicalInstrumentId(entry: Record<string, unknown>): string {
  return String(entry.instrumentId ?? "").trim().toUpperCase();
}

const enrichmentSnapshots = useResearchSnapshots(
  () =>
    [...hot.entries.value, ...highDividend.entries.value]
      .map(canonicalInstrumentId)
      .filter(Boolean),
  () => props.brokerId,
);

interface IndexCardModel {
  entry: Record<string, unknown>;
  name: string;
  value: number | null;
  changeAmount: number | null;
  changeRate: number | null;
}

const indexCards = computed<IndexCardModel[]>(() =>
  benchmarkDefinitions.value.map((definition) => {
    const snapshot =
      benchmarkSnapshots.byInstrumentId.value[definition.instrumentId];
    const entry = mergeResearchSnapshot(
      {
        instrumentId: definition.instrumentId,
        market: definition.instrumentId.split(".")[0],
        symbol: definition.instrumentId.split(".").slice(1).join("."),
        name: definition.name,
        productClass: "index",
      },
      snapshot,
    );
    const changeAmount = pickNumber(entry, ["changeAmount"]);
    const changeRate = pickNumber(entry, ["changeRate"]);
    return {
      entry,
      name: pickString(entry, ["name"]) || definition.name,
      value: pickNumber(entry, ["price", "lastPrice"]),
      changeAmount,
      changeRate,
    };
  }),
);

const gainers = computed(() => gainersFeature.entries.value.slice(0, PANEL_SIZE));
const losers = computed(() => losersFeature.entries.value.slice(0, PANEL_SIZE));
const hottest = computed(() =>
  hot.entries.value.slice(0, PANEL_SIZE).map((entry) =>
    mergeResearchSnapshot(
      entry,
      enrichmentSnapshots.byInstrumentId.value[canonicalInstrumentId(entry)],
    ),
  ),
);
const dividendEntries = computed(() =>
  highDividend.entries.value.slice(0, PANEL_SIZE).map((entry) =>
    mergeResearchSnapshot(
      entry,
      enrichmentSnapshots.byInstrumentId.value[canonicalInstrumentId(entry)],
    ),
  ),
);

const anyLoading = computed(
  () =>
    benchmarkSnapshots.loading.value ||
    gainersFeature.loading.value ||
    losersFeature.loading.value ||
    hot.loading.value ||
    highDividend.loading.value ||
    heatmap.loading.value,
);
// Only a full capability miss replaces the whole body; a single unsupported
// branch (e.g. embedded providers have no US/HK board heatmap feed) degrades
// per-section so the ranking panels that did load stay visible. Features whose
// request is skipped for the market are excluded from the vote.
const activeFeatures = computed(() => {
  const features = [heatmap];
  if (props.market !== "CN") features.push(gainersFeature, losersFeature, hot);
  if (props.market === "HK") features.push(highDividend);
  return features;
});
const providerUnsupported = computed(() =>
  activeFeatures.value.every((feature) => feature.providerUnsupported.value),
);
// Provider-capability 409s degrade to an in-view empty state; the banner keeps
// only genuine failures.
const anyError = computed(
  () =>
    [
      benchmarkSnapshots.error.value,
      enrichmentSnapshots.error.value,
      gainersFeature.error.value,
      losersFeature.error.value,
      hot.error.value,
      heatmap.error.value,
      highDividend.error.value,
    ].find((message) => message !== "" && !isProviderCapabilityMessage(message)) ??
    "",
);
</script>

<template>
  <div class="market-home-view">
    <EmptyState
      :loading="anyLoading"
      class="market-home-view__status"
      :min-height="96"
    >
      <div v-if="anyError" class="market-home-view__warning">部分数据加载失败：{{ anyError }}</div>
      <div class="market-home-view__indices">
        <div
          v-if="!supportsBenchmarkSnapshots"
          class="market-home-view__benchmark-note"
        >
          <strong>美股指数快照暂不可用</strong>
          <span>当前 OpenD 10.9.6908 明确不支持美股指数证券，研究中心不会用 ETF 代理或伪造指数卡片。</span>
        </div>
        <div
          v-for="card in indexCards"
          :key="card.name"
          class="market-home-view__index-card"
          @click="emit('select', card.entry)"
        >
          <div class="market-home-view__index-head">
            <span class="market-home-view__index-name">{{ card.name }}</span>
          </div>
          <div class="market-home-view__index-value tv-num" :class="directionClass(card.changeRate)">
            {{ formatPrice(card.value) }}
          </div>
          <div class="market-home-view__index-change tv-num" :class="directionClass(card.changeRate)">
            {{ formatSigned(card.changeAmount) }}
            {{ formatSigned(card.changeRate, "%") }}
          </div>
        </div>
        <EmptyState
          v-if="supportsBenchmarkSnapshots && indexCards.length === 0"
          empty
          empty-label="暂无指数数据"
          class="market-home-view__status market-home-view__status--inline"
          :min-height="76"
          grow
        />
      </div>

      <ProviderUnsupportedState v-if="providerUnsupported" :min-height="200" />
      <div v-else class="market-home-view__body">
        <div class="market-home-view__ranks">
          <div v-if="supportsMovers" class="market-home-view__panel">
            <RankListPanel
              title="领涨榜"
              :entries="gainers"
              @select="emit('select', $event)"
            />
            <button type="button" class="market-home-view__more" @click="emit('more', 'top_gainers')">&gt;</button>
          </div>
          <div v-if="supportsMovers" class="market-home-view__panel">
            <RankListPanel
              title="领跌榜"
              :entries="losers"
              default-sort-order="asc"
              @select="emit('select', $event)"
            />
            <button type="button" class="market-home-view__more" @click="emit('more', 'top_losers')">&gt;</button>
          </div>
          <div v-if="supportsMovers" class="market-home-view__panel">
            <RankListPanel
              title="热度榜"
              :entries="hottest"
              value-field="averageHeat"
              value-label="综合热度"
              @select="emit('select', $event)"
            />
            <button type="button" class="market-home-view__more" @click="emit('more', 'hot')">&gt;</button>
          </div>
          <div v-if="market === 'HK'" class="market-home-view__panel">
            <RankListPanel
              title="高股息"
              :entries="dividendEntries"
              value-field="dividendYieldTTM"
              value-label="股息率"
              @select="emit('select', $event)"
            />
            <button type="button" class="market-home-view__more" @click="emit('more', 'high_dividend_state')">&gt;</button>
          </div>
          <div v-if="!supportsMovers" class="market-home-view__market-note">
            OpenD 涨跌榜与热门榜仅支持美股、港股；专用高股息协议不接受市场参数且实测仅返回港股，沪深不展示错配数据。
          </div>
        </div>

        <div class="market-home-view__heatmap">
          <header class="market-home-view__heatmap-head">
            <span class="market-home-view__heatmap-title">板块热力</span>
            <span class="tv-seg">
              <button
                type="button"
                :class="{ 'is-active': heatmapType === 'industry' }"
                @click="heatmapType = 'industry'"
              >行业</button>
              <button
                type="button"
                :class="{ 'is-active': heatmapType === 'concept' }"
                @click="heatmapType = 'concept'"
              >概念</button>
              <button
                type="button"
                :class="{ 'is-active': heatmapType === 'theme' }"
                @click="heatmapType = 'theme'"
              >主题</button>
            </span>
          </header>
          <ProviderUnsupportedState
            v-if="heatmap.providerUnsupported.value"
            bordered
            :min-height="120"
          />
          <SectorHeatmap
            v-else
            :entries="heatmap.entries.value"
            :height="420"
            @select="emit('select', $event)"
          />
        </div>
      </div>
    </EmptyState>
  </div>
</template>

<style scoped>
.market-home-view {
  display: flex;
  width: 100%;
  min-width: 0;
  max-width: 100%;
  min-height: 0;
  flex-direction: column;
  gap: 8px;
  color: var(--tv-text);
  font-size: var(--jf-text-6);
}

.market-home-view__warning {
  padding: 7px 10px;
  border: 1px solid color-mix(in srgb, var(--tv-warn) 40%, var(--tv-border));
  border-radius: 4px;
  color: var(--tv-warn);
}

.market-home-view__indices {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 8px;
}

.market-home-view__index-card {
  padding: 10px 12px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  cursor: pointer;
}

.market-home-view__benchmark-note {
  display: flex;
  min-height: 76px;
  grid-column: 1 / -1;
  flex-direction: column;
  justify-content: center;
  gap: 5px;
  padding: 10px 12px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-muted);
}

.market-home-view__benchmark-note span {
  color: var(--tv-text-dim);
  font-size: var(--jf-text-4);
}

.market-home-view__index-card:hover {
  border-color: var(--tv-border-strong);
}

.market-home-view__index-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.market-home-view__index-name {
  color: var(--tv-text-muted);
}

.market-home-view__index-value {
  margin-top: 6px;
  font-size: var(--jf-text-13);
  font-weight: 650;
  line-height: 1.1;
}

.market-home-view__index-change {
  margin-top: 2px;
}

.market-home-view__body {
  display: flex;
  width: 100%;
  min-width: 0;
  flex-wrap: wrap;
  gap: 8px;
}

.market-home-view__ranks {
  display: grid;
  min-width: 0;
  max-width: 100%;
  flex: 2 1 480px;
  grid-template-columns: repeat(
    auto-fit,
    minmax(min(320px, 100%), 1fr)
  );
  gap: 8px;
  align-content: start;
}

.market-home-view__panel {
  position: relative;
  min-width: 0;
}

.market-home-view__market-note {
  display: grid;
  min-height: 96px;
  place-items: center;
  padding: 16px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  color: var(--tv-text-dim);
  text-align: center;
}

.market-home-view__more {
  position: absolute;
  top: 0;
  right: 10px;
  height: 32px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--tv-text-dim);
  cursor: pointer;
  font-size: var(--jf-text-6);
}

.market-home-view__more:hover {
  color: var(--tv-text);
}

.market-home-view__heatmap {
  display: flex;
  min-width: 260px;
  flex: 1 1 260px;
  flex-direction: column;
  gap: 6px;
}

.market-home-view__heatmap-head {
  display: flex;
  height: 32px;
  align-items: center;
  justify-content: space-between;
  padding: 0 2px;
}

.market-home-view__heatmap-title {
  font-weight: 600;
}

</style>
