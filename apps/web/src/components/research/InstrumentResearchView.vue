<script setup lang="ts">
import { computed } from "vue";

import CompactInstrumentNews from "../domain/market-data/CompactInstrumentNews.vue";
import InstrumentSearchBox from "../domain/market-data/InstrumentSearchBox.vue";
import ResearchDataTable from "./ResearchDataTable.vue";
import { formatPrice, pickNumber, pickString } from "./researchEntry";
import {
  useInstrumentResearchController,
  type InstrumentResearchOperation,
} from "./useInstrumentResearchController";

const props = withDefaults(
  defineProps<{
    instrumentId: string;
    brokerId?: string;
    operation?: InstrumentResearchOperation;
  }>(),
  { brokerId: "", operation: "profile" },
);
const emit = defineEmits<{
  "update:instrumentId": [instrumentId: string];
  select: [entry: Record<string, unknown>];
  open: [entry: Record<string, unknown>];
}>();

const {
  searchValue,
  selectCandidate,
  feature,
  newsTarget,
  profileGroups,
  financialRows,
  financialColumns,
  valuationTrend,
  valuationHistory,
  valuationHistoryColumns,
  marketDistribution,
  marketDistributionSections,
  marketDistributionColumns,
  plateDistribution,
  plateStocks,
  plateStockColumns,
  profitGrowthRate,
  profitGrowthRows,
  profitGrowthColumns,
  hasValuationData,
  valuationTypeLabel,
  trendMetrics,
  marketMetrics,
  plateMetrics,
  profitMetrics,
  analyst,
  analystCount,
  analystRating,
  ownershipRows,
  ownershipColumns,
  actionColumns,
  shortRows,
  shortColumns,
  shortSummary,
  status,
} = useInstrumentResearchController(props, emit);

const analystRatings = computed(() =>
  [
    {
      key: "strongBuy",
      label: "强力推荐",
      className: "tv-up",
      value: pickNumber(analyst.value, ["strongBuy"]),
    },
    {
      key: "buy",
      label: "买入",
      className: "tv-up",
      value: pickNumber(analyst.value, ["buy"]),
    },
    {
      key: "hold",
      label: "持有",
      className: "",
      value: pickNumber(analyst.value, ["hold"]),
    },
    {
      key: "underperform",
      label: "跑输大盘",
      className: "tv-down",
      value: pickNumber(analyst.value, ["underperform"]),
    },
    {
      key: "sell",
      label: "卖出",
      className: "tv-down",
      value: pickNumber(analyst.value, ["sell"]),
    },
  ].filter((item) => item.value != null),
);
</script>

<template>
  <section class="instrument-research">
    <header class="instrument-research__toolbar">
      <InstrumentSearchBox
        v-model="searchValue"
        class="instrument-research__search"
        variant="backtest"
        action-label="切换"
        placeholder="输入证券代码或名称"
        @select="selectCandidate"
      />
      <span class="instrument-research__identity">{{ instrumentId }}</span>
      <span class="instrument-research__spacer" />
      <small v-if="feature.asOf.value && operation !== 'news'">
        更新 {{ feature.asOf.value }}
      </small>
      <button
        v-if="operation !== 'news'"
        type="button"
        @click="feature.refresh"
      >
        刷新
      </button>
    </header>

    <div v-if="status" class="instrument-research__status">{{ status }}</div>

    <div
      v-else-if="operation === 'profile'"
      class="instrument-research__profile"
    >
      <section v-for="group in profileGroups" :key="group.title">
        <header>{{ group.title }}</header>
        <dl>
          <template v-for="item in group.entries" :key="`${group.title}:${item.name}`">
            <dt>{{ item.name }}</dt>
            <dd>
              <a
                v-if="item.link"
                :href="item.value"
                target="_blank"
                rel="noopener noreferrer"
              >{{ item.value }}</a>
              <span v-else>{{ item.value }}</span>
            </dd>
          </template>
        </dl>
      </section>
      <div v-if="profileGroups.length === 0" class="instrument-research__status">
        暂无公司资料
      </div>
    </div>

    <ResearchDataTable
      v-else-if="operation === 'financials'"
      :entries="financialRows"
      :columns="financialColumns"
      empty-label="暂无财务数据"
    />

    <div
      v-else-if="operation === 'valuation'"
      class="instrument-research__valuation"
    >
      <section v-if="Object.keys(valuationTrend).length > 0">
        <header>
          <span>估值趋势</span>
          <small>{{ valuationTypeLabel }}</small>
        </header>
        <div class="instrument-research__metric-grid">
          <div v-for="item in trendMetrics" :key="item.label">
            <span>{{ item.label }}</span>
            <strong>{{ item.value }}</strong>
          </div>
        </div>
        <ResearchDataTable
          v-if="valuationHistory.length > 0"
          class="instrument-research__nested-table"
          :entries="valuationHistory"
          :columns="valuationHistoryColumns"
          compact
        />
      </section>

      <section v-if="Object.keys(marketDistribution).length > 0">
        <header>市场分布</header>
        <div class="instrument-research__metric-grid">
          <div v-for="item in marketMetrics" :key="item.label">
            <span>{{ item.label }}</span>
            <strong>{{ item.value }}</strong>
          </div>
        </div>
        <ResearchDataTable
          v-if="marketDistributionSections.length > 0"
          class="instrument-research__nested-table"
          :entries="marketDistributionSections"
          :columns="marketDistributionColumns"
          compact
        />
      </section>

      <section v-if="Object.keys(plateDistribution).length > 0">
        <header>板块分布</header>
        <div class="instrument-research__metric-grid">
          <div v-for="item in plateMetrics" :key="item.label">
            <span>{{ item.label }}</span>
            <strong>{{ item.value }}</strong>
          </div>
        </div>
        <ResearchDataTable
          v-if="plateStocks.length > 0"
          class="instrument-research__nested-table"
          :entries="plateStocks"
          :columns="plateStockColumns"
          compact
        />
      </section>

      <section v-if="Object.keys(profitGrowthRate).length > 0">
        <header>盈利 / 营收增长</header>
        <div class="instrument-research__metric-grid">
          <div v-for="item in profitMetrics" :key="item.label">
            <span>{{ item.label }}</span>
            <strong>{{ item.value }}</strong>
          </div>
        </div>
        <p
          v-if="pickString(profitGrowthRate, ['conclusionDetailed'])"
          class="instrument-research__conclusion"
        >
          {{ pickString(profitGrowthRate, ["conclusionDetailed"]) }}
        </p>
        <ResearchDataTable
          v-if="profitGrowthRows.length > 0"
          class="instrument-research__nested-table"
          :entries="profitGrowthRows"
          :columns="profitGrowthColumns"
          compact
        />
      </section>
      <div v-if="!hasValuationData" class="instrument-research__status">
        暂无估值数据
      </div>
    </div>

    <div
      v-else-if="operation === 'analyst'"
      class="instrument-research__analyst"
    >
      <section class="instrument-research__target">
        <header>目标价区间</header>
        <div>
          <span>最低 <b>{{ formatPrice(pickNumber(analyst, ["lowest"])) }}</b></span>
          <span>平均 <b>{{ formatPrice(pickNumber(analyst, ["average"])) }}</b></span>
          <span>最高 <b>{{ formatPrice(pickNumber(analyst, ["highest"])) }}</b></span>
        </div>
      </section>
      <section class="instrument-research__ratings">
        <header>
          <span>评级分布</span>
          <span class="instrument-research__rating-meta">
            <small v-if="analystCount != null">{{ analystCount }} 位分析师</small>
            <strong>{{ analystRating }}</strong>
          </span>
        </header>
        <div
          v-for="item in analystRatings"
          :key="item.key"
          class="instrument-research__rating-row"
        >
          <span>{{ item.label }}</span>
          <i>
            <b
              :class="item.className"
              :style="{
                width: `${Math.min(100, Math.max(0, item.value ?? 0))}%`,
              }"
            />
          </i>
          <strong>{{ formatPrice(item.value) }}%</strong>
        </div>
        <small>{{ pickString(analyst, ["updateTimeStr", "updateTime"]) }}</small>
      </section>
    </div>

    <ResearchDataTable
      v-else-if="operation === 'ownership'"
      :entries="ownershipRows"
      :columns="ownershipColumns"
      empty-label="暂无股权数据"
      compact
    />

    <ResearchDataTable
      v-else-if="operation === 'corporate_actions'"
      :entries="feature.entries.value"
      :columns="actionColumns"
      empty-label="暂无公司行动"
      compact
      @select="emit('select', $event)"
      @open="emit('open', $event)"
    />

    <div
      v-else-if="operation === 'short_interest'"
      class="instrument-research__short"
    >
      <div
        v-if="shortSummary.length > 0"
        class="instrument-research__metric-grid instrument-research__short-summary"
      >
        <div v-for="item in shortSummary" :key="item.label">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
        </div>
      </div>
      <ResearchDataTable
        :entries="shortRows"
        :columns="shortColumns"
        empty-label="暂无卖空数据"
        compact
      />
    </div>

    <CompactInstrumentNews
      v-else
      class="instrument-research__news"
      :target="newsTarget"
      :query-instrument-id="instrumentId"
      :broker-id="brokerId"
      active
      @select-target="emit('update:instrumentId', $event.instrumentId)"
    />
  </section>
</template>

<style scoped>
.instrument-research {
  display: flex;
  min-height: 0;
  height: 100%;
  flex-direction: column;
  gap: 8px;
  color: var(--tv-text);
  font-size: var(--jf-text-6);
}

.instrument-research__toolbar {
  display: flex;
  min-height: 34px;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
}

.instrument-research__search {
  width: min(360px, 48%);
}

.instrument-research__identity {
  color: var(--tv-text-muted);
  font-family: var(--tv-font-mono, monospace);
}

.instrument-research__spacer {
  flex: 1;
}

.instrument-research__toolbar small {
  color: var(--tv-text-dim);
}

.instrument-research__toolbar > button {
  height: 28px;
  padding: 0 9px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
  cursor: pointer;
  font: inherit;
}

.instrument-research__status {
  display: grid;
  min-height: 140px;
  flex: 1;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-dim);
}

.instrument-research__profile,
.instrument-research__valuation {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 8px;
  overflow: auto;
}

.instrument-research__profile > section,
.instrument-research__valuation > section,
.instrument-research__target,
.instrument-research__ratings {
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.instrument-research__profile section > header,
.instrument-research__valuation section > header,
.instrument-research__target > header,
.instrument-research__ratings > header {
  display: flex;
  min-height: 32px;
  align-items: center;
  justify-content: space-between;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  font-weight: 600;
}

.instrument-research__valuation section > header small {
  color: var(--tv-text-dim);
  font-size: var(--jf-text-4);
  font-weight: 500;
}

.instrument-research__profile dl {
  display: grid;
  margin: 0;
  grid-template-columns: minmax(90px, 0.35fr) minmax(0, 1fr);
}

.instrument-research__profile dt,
.instrument-research__profile dd {
  min-height: 32px;
  margin: 0;
  padding: 7px 8px;
  overflow-wrap: anywhere;
  border-bottom: 1px solid var(--tv-border);
}

.instrument-research__profile dt {
  color: var(--tv-text-muted);
}

.instrument-research__profile a {
  color: var(--tv-accent);
}

.instrument-research__metric-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(110px, 1fr));
  gap: 1px;
  background: var(--tv-border);
}

.instrument-research__metric-grid > div {
  display: flex;
  min-height: 52px;
  flex-direction: column;
  justify-content: center;
  padding: 6px 8px;
  background: var(--tv-bg-surface);
}

.instrument-research__metric-grid span {
  color: var(--tv-text-dim);
  font-size: var(--jf-text-4);
}

.instrument-research__nested-table {
  border: 0;
  border-top: 1px solid var(--tv-border);
  border-radius: 0;
}

.instrument-research__conclusion {
  margin: 0;
  padding: 7px 8px;
  border-top: 1px solid var(--tv-border);
  color: var(--tv-text-muted);
  line-height: 1.5;
}

.instrument-research__analyst {
  display: grid;
  grid-template-columns: minmax(280px, 1fr) minmax(320px, 1.2fr);
  gap: 8px;
}

.instrument-research__target > div {
  display: grid;
  min-height: 110px;
  grid-template-columns: repeat(3, 1fr);
  align-items: center;
}

.instrument-research__target span {
  display: flex;
  flex-direction: column;
  gap: 5px;
  color: var(--tv-text-dim);
  text-align: center;
}

.instrument-research__target b {
  color: var(--tv-text);
  font-size: var(--jf-text-12);
}

.instrument-research__ratings {
  padding-bottom: 6px;
}

.instrument-research__rating-meta {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.instrument-research__rating-meta small {
  color: var(--tv-text-dim);
  font-weight: 500;
}

.instrument-research__rating-row {
  display: grid;
  min-height: 30px;
  grid-template-columns: 58px 1fr 58px;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
}

.instrument-research__rating-row i {
  height: 7px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--tv-bg-elevated);
}

.instrument-research__rating-row i b {
  display: block;
  height: 100%;
  background: var(--tv-accent);
}

.instrument-research__rating-row i b.tv-up {
  background: var(--tv-price-up);
}

.instrument-research__rating-row i b.tv-down {
  background: var(--tv-price-down);
}

.instrument-research__rating-row > strong {
  text-align: right;
}

.instrument-research__ratings > small {
  display: block;
  padding: 4px 8px;
  color: var(--tv-text-dim);
  text-align: right;
}

.instrument-research__short {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  gap: 8px;
}

.instrument-research__short-summary {
  flex: 0 0 auto;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
}

.instrument-research__short > :deep(.research-data-table) {
  min-height: 0;
  flex: 1;
}

.instrument-research__news {
  min-height: 0;
  flex: 1;
}

@media (max-width: 820px) {
  .instrument-research__toolbar {
    flex-wrap: wrap;
  }

  .instrument-research__search {
    width: 100%;
  }

  .instrument-research__analyst {
    grid-template-columns: 1fr;
  }
}
</style>
