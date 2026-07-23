<script setup lang="ts">
import { computed } from "vue";

import VerticalQuoteWorkbench from "../domain/market-data/VerticalQuoteWorkbench.vue";
import type {
  QuoteWorkbenchPeriod,
  QuoteWorkbenchTab,
} from "../domain/market-data/quoteWorkbench";
import {
  normalizeResearchQuoteTarget,
  researchQuoteTargetFromEntry,
  type ResearchQuoteTarget,
} from "./researchQuote";

const props = withDefaults(
  defineProps<{
    /** Preferred, normalized selection contract. */
    target?: ResearchQuoteTarget | null;
    /** Backward-compatible input while research lists migrate to target. */
    entry?: Record<string, unknown> | null;
    market?: string;
    brokerId?: string;
    visible?: boolean;
    drawer?: boolean;
    period?: QuoteWorkbenchPeriod;
    tab?: QuoteWorkbenchTab;
  }>(),
  {
    target: null,
    entry: null,
    market: "",
    brokerId: "",
    visible: true,
    drawer: false,
    period: "day",
    tab: "quote",
  },
);

const emit = defineEmits<{
  "update:period": [period: QuoteWorkbenchPeriod];
  "update:tab": [tab: QuoteWorkbenchTab];
  close: [];
  select: [target: ResearchQuoteTarget];
  openWorkspace: [target: ResearchQuoteTarget];
}>();

const resolvedTarget = computed(
  () =>
    normalizeResearchQuoteTarget(props.target) ??
    researchQuoteTargetFromEntry(props.entry, props.market),
);
const emptyText = computed(() =>
  props.entry == null && props.target == null
    ? "点击左侧榜单查看行情详情"
    : "该条目缺少精确的 OpenD 标的代码",
);
</script>

<template>
  <VerticalQuoteWorkbench
    :target="resolvedTarget"
    :broker-id="brokerId"
    :visible="visible"
    :variant="drawer ? 'drawer' : 'rail'"
    :period="period"
    :tab="tab"
    :empty-text="emptyText"
    @update:period="emit('update:period', $event)"
    @update:tab="emit('update:tab', $event)"
    @select-target="emit('select', $event)"
    @open-workspace="emit('openWorkspace', $event)"
    @close="emit('close')"
  />
</template>
