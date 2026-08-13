<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { productFeaturePath, type ProductFeatureRequest } from "@/composables/product/productFeatureApi";

import { productCompactMenuProps } from "@/composables/product/productControlDensity";
import AppTabs from "@/components/shared/AppTabs.vue";
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
const request = computed<ProductFeatureRequest | null>(() => {
  if (!props.instrumentId.trim()) return null;
  const base = {
    instrumentId: props.instrumentId,
    operation: operation.value,
    pageSize: 50,
  };
  switch (section.value) {
    case "financials":
      return { scope: "research", family: "financials", ...base };
    case "valuation":
      return { scope: "research", family: "valuation", ...base };
    case "analyst":
      return { scope: "research", family: "analyst", ...base };
    case "ownership":
      return { scope: "research", family: "ownership", ...base };
    case "actions":
      return { scope: "research", family: "corporate-actions", ...base };
    case "short":
      return { scope: "research", family: "short-interest", ...base };
    case "news":
      return { scope: "market-feature", resource: "news", market: props.market, code: props.instrumentId, operation: operation.value, pageSize: 30 };
    default:
      return { scope: "research", family: "instrument", ...base };
  }
});
const path = computed(() => request.value == null ? "" : productFeaturePath(request.value));

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
    <AppTabs v-model="section" :items="sections" label="公司研究视图" />
    <ProductFeaturePanel
      :key="JSON.stringify(request)"
      :title="activeSection.label"
      :request="request"
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
.company-workspace > :first-child {
  min-height: 36px;
  flex: 0 0 36px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}
.company-workspace :deep(.app-tabs__tab) {
  min-width: 68px;
  height: 35px;
  padding: 0 9px;
  font-size: var(--jf-text-3);
}
.company-workspace :deep(.app-tabs__tab.is-active) {
  color: var(--tv-text);
}
.company-workspace__operation {
  width: 168px;
  max-width: 168px;
  flex: 0 0 168px;
}
</style>
