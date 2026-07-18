<script setup lang="ts">
import { computed, ref, watch } from "vue";

import { productCompactMenuProps } from "../../composables/productControlDensity";
import ProductFeaturePanel from "./ProductFeaturePanel.vue";

type CompanySection =
  | "overview"
  | "financials"
  | "valuation"
  | "analyst"
  | "ownership"
  | "actions"
  | "short"
  | "news";

const props = defineProps<{ instrumentId: string; market: string }>();
const emit = defineEmits<{ openInstrument: [instrumentId: string] }>();
const section = ref<CompanySection>("overview");
const operation = ref("");
const sections: Array<{
  value: CompanySection;
  label: string;
  operations: Array<{ title: string; value: string }>;
}> = [
  {
    value: "overview",
    label: "概览与管理层",
    operations: [
      { title: "公司资料", value: "profile" },
      { title: "管理层", value: "executives" },
      { title: "管理层背景", value: "executive_background" },
      { title: "运营效率", value: "operational_efficiency" },
      { title: "Top Broker", value: "top_brokers" },
    ],
  },
  {
    value: "financials",
    label: "财务与收入",
    operations: [
      { title: "财务报表", value: "statements" },
      { title: "收入拆分", value: "revenue_breakdown" },
      { title: "业绩价格反应", value: "earnings_price_move" },
      { title: "业绩价格历史", value: "earnings_price_history" },
    ],
  },
  {
    value: "valuation",
    label: "估值",
    operations: [
      { title: "估值详情", value: "detail" },
      { title: "板块成分估值", value: "constituents" },
    ],
  },
  {
    value: "analyst",
    label: "评级与 Morningstar",
    operations: [
      { title: "分析师共识", value: "consensus" },
      { title: "评级汇总", value: "ratings" },
      { title: "Morningstar", value: "morningstar" },
      { title: "评级变化", value: "changes" },
    ],
  },
  {
    value: "ownership",
    label: "股东与机构",
    operations: [
      { title: "股东概览", value: "overview" },
      { title: "持股变化", value: "changes" },
      { title: "主要股东", value: "holders" },
      { title: "机构持仓", value: "institutional" },
      { title: "内部人持股", value: "insider_holders" },
      { title: "内部人交易", value: "insider_transactions" },
      { title: "管理层持股变化", value: "management_changes" },
    ],
  },
  {
    value: "actions",
    label: "派息/回购/拆股",
    operations: [
      { title: "派息", value: "dividends" },
      { title: "回购", value: "buybacks" },
      { title: "拆股/合股", value: "splits" },
      { title: "代码变更", value: "code_changes" },
    ],
  },
  {
    value: "short",
    label: "沽空",
    operations: [
      { title: "每日沽空量", value: "daily_volume" },
      { title: "沽空权益", value: "short_interest" },
    ],
  },
  {
    value: "news",
    label: "资讯",
    operations: [{ title: "新闻搜索", value: "search" }],
  },
];
const activeSection = computed(
  () => sections.find((item) => item.value === section.value) ?? sections[0]!,
);
const encodedInstrument = computed(() =>
  encodeURIComponent(props.instrumentId),
);
const path = computed(() => {
  if (!props.instrumentId.trim()) return "";
  const operationQuery = `operation=${encodeURIComponent(operation.value)}&pageSize=50`;
  switch (section.value) {
    case "financials":
      return `/api/v1/research/financials/${encodedInstrument.value}?${operationQuery}`;
    case "valuation":
      return `/api/v1/research/valuation/${encodedInstrument.value}?${operationQuery}`;
    case "analyst":
      return `/api/v1/research/analyst/${encodedInstrument.value}?${operationQuery}`;
    case "ownership":
      return `/api/v1/research/ownership/${encodedInstrument.value}?${operationQuery}`;
    case "actions":
      return `/api/v1/research/corporate-actions/${encodedInstrument.value}?${operationQuery}`;
    case "short":
      return `/api/v1/research/short-interest/${encodedInstrument.value}?${operationQuery}`;
    case "news":
      return `/api/v1/market-data/news?market=${props.market}&code=${encodedInstrument.value}&operation=${operation.value}&pageSize=30`;
    default:
      return `/api/v1/research/instruments/${encodedInstrument.value}?${operationQuery}`;
  }
});

watch(
  section,
  () => {
    operation.value = activeSection.value.operations[0]?.value ?? "";
  },
  { immediate: true },
);
</script>

<template>
  <section class="company-workspace">
    <v-tabs v-model="section" density="compact" show-arrows>
      <v-tab v-for="item in sections" :key="item.value" :value="item.value">
        {{ item.label }}
      </v-tab>
    </v-tabs>
    <ProductFeaturePanel
      :key="path"
      :title="activeSection.label"
      :path="path"
      @open-instrument="emit('openInstrument', $event)"
    >
      <template #controls>
        <v-select
          v-model="operation"
          class="company-workspace__operation product-compact-control"
          :items="activeSection.operations"
          :menu-props="productCompactMenuProps"
          density="compact"
          variant="outlined"
          hide-details
          aria-label="数据视图"
          title="数据视图"
        />
      </template>
    </ProductFeaturePanel>
  </section>
</template>

<style scoped>
.company-workspace {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
}
.company-workspace > :last-child {
  min-height: 0;
  flex: 1;
}
.company-workspace :deep(.v-tabs) {
  min-height: 36px;
  flex: 0 0 36px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}
.company-workspace :deep(.v-slide-group__content) {
  padding: 0 7px;
}
.company-workspace :deep(.v-tab) {
  min-width: 68px;
  height: 35px;
  padding: 0 9px;
  color: var(--tv-text-muted);
  font-size: 9px;
  letter-spacing: 0;
  text-transform: none;
}
.company-workspace :deep(.v-tab--selected) {
  color: var(--tv-text);
}
.company-workspace :deep(.v-tab__slider) {
  height: 2px;
}
.company-workspace__operation {
  width: 168px;
  max-width: 168px;
  flex: 0 0 168px;
}
</style>
