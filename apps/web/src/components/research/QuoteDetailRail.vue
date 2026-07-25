<script setup lang="ts">
import { computed } from "vue";

import VerticalQuoteWorkbench from "../domain/market-data/VerticalQuoteWorkbench.vue";
import type {
  QuoteWorkbenchPeriod,
  QuoteWorkbenchTab,
} from "../domain/market-data/quoteWorkbench";
import {
  normalizeResearchQuoteTarget,
  type QuoteSeed,
  type ResearchQuoteTarget,
} from "./researchQuote";

const props = withDefaults(
  defineProps<{
    /** Preferred, normalized selection contract. */
    target?: ResearchQuoteTarget | null;
    seed?: QuoteSeed | null;
    brokerId?: string;
    visible?: boolean;
    drawer?: boolean;
    period?: QuoteWorkbenchPeriod;
    tab?: QuoteWorkbenchTab;
  }>(),
  {
    target: null,
    seed: null,
    brokerId: "",
    visible: true,
    drawer: false,
    period: "1d",
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
  () => normalizeResearchQuoteTarget(props.target),
);
const emptyText = computed(() =>
  props.target == null && props.seed == null
    ? "点击左侧榜单查看行情详情"
    : "该条目缺少精确的 OpenD 标的代码",
);
</script>

<template>
  <VerticalQuoteWorkbench
    :target="resolvedTarget"
    :seed="seed"
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
