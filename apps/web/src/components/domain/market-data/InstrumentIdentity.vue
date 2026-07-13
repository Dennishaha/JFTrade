<script setup lang="ts">
import { computed } from "vue";

import {
  formatUserMarketLabel,
  presentInstrument,
} from "@/composables/instrumentPresentation";

const props = withDefaults(
  defineProps<{
    market?: string | null | undefined;
    code?: string | null | undefined;
    instrumentId?: string | null | undefined;
    name?: string | null | undefined;
    compact?: boolean;
    layout?: "inline" | "stacked";
  }>(),
  {
    market: null,
    code: null,
    instrumentId: null,
    name: null,
    compact: false,
    layout: "inline",
  },
);

const presentation = computed(() =>
  presentInstrument({
    market: props.market,
    code: props.code,
    instrumentId: props.instrumentId,
  }),
);

const displayName = computed(() =>
  props.name?.trim() || presentation.value.displayCode || "未设置",
);

const stackedMarketTag = computed(() => {
  if (presentation.value.market === "") {
    return null;
  }
  return (
    presentation.value.exchangeTag ??
    formatUserMarketLabel(presentation.value.market)
  );
});

function copyCanonicalInstrumentId(event: ClipboardEvent): void {
  const instrumentId = presentation.value.instrumentId;
  if (instrumentId === "" || event.clipboardData == null) {
    return;
  }
  event.clipboardData.setData("text/plain", instrumentId);
  event.preventDefault();
}
</script>

<template>
  <span
    class="instrument-identity"
    :class="{
      'instrument-identity--compact': compact,
      'instrument-identity--stacked': layout === 'stacked',
      'instrument-identity--has-market-tag':
        layout === 'stacked' && stackedMarketTag != null,
    }"
    :data-market="presentation.market"
    :data-category-market="presentation.categoryMarket"
    :data-instrument-id="presentation.instrumentId"
    :data-copy-value="presentation.instrumentId"
    :title="presentation.instrumentId || undefined"
    @copy="copyCanonicalInstrumentId"
  >
    <template v-if="layout === 'stacked'">
      <span class="instrument-identity__primary">
        <span
          v-if="stackedMarketTag"
          class="instrument-identity__exchange-tag"
          :data-exchange="presentation.market"
        >
          {{ stackedMarketTag }}
        </span>
        <span class="instrument-identity__title">{{ displayName }}</span>
      </span>
      <span class="instrument-identity__secondary">
        {{ presentation.code || presentation.displayCode || "未设置" }}
      </span>
    </template>
    <template v-else>
      <span class="instrument-identity__code">
        {{ presentation.displayCode || "未设置" }}
      </span>
      <span
        v-if="presentation.exchangeTag"
        class="instrument-identity__exchange-tag"
        :data-exchange="presentation.market"
      >
        {{ presentation.exchangeTag }}
      </span>
      <span v-if="name" class="instrument-identity__name">{{ ` · ${name}` }}</span>
    </template>
  </span>
</template>

<style>
:root,
[data-theme="dark"] {
  --instrument-market-tag-cn-border: #7f1d1d;
  --instrument-market-tag-cn-background: #451a1a;
  --instrument-market-tag-cn-text: #fca5a5;
  --instrument-market-tag-hk-border: #6b21a8;
  --instrument-market-tag-hk-background: #2e1065;
  --instrument-market-tag-hk-text: #d8b4fe;
  --instrument-market-tag-us-border: #1e40af;
  --instrument-market-tag-us-background: #172554;
  --instrument-market-tag-us-text: #93c5fd;
}

[data-theme="light"] {
  --instrument-market-tag-cn-border: #fecaca;
  --instrument-market-tag-cn-background: #fee2e2;
  --instrument-market-tag-cn-text: #b91c1c;
  --instrument-market-tag-hk-border: #e9d5ff;
  --instrument-market-tag-hk-background: #f3e8ff;
  --instrument-market-tag-hk-text: #7e22ce;
  --instrument-market-tag-us-border: #bfdbfe;
  --instrument-market-tag-us-background: #dbeafe;
  --instrument-market-tag-us-text: #1d4ed8;
}
</style>

<style scoped>
.instrument-identity {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 0.4rem;
  color: inherit;
  vertical-align: middle;
}

.instrument-identity--compact {
  gap: 0.3rem;
  font-size: 0.875em;
}

.instrument-identity--stacked {
  --instrument-identity-tag-width: 2rem;
  --instrument-identity-primary-gap: 0.4rem;

  display: flex;
  width: 100%;
  align-items: flex-start;
  flex-direction: column;
  gap: 0.15rem;
  line-height: 1.2;
}

.instrument-identity--stacked.instrument-identity--compact {
  --instrument-identity-primary-gap: 0.3rem;
}

.instrument-identity__primary {
  display: flex;
  width: 100%;
  min-width: 0;
  align-items: center;
  gap: 0.4rem;
}

.instrument-identity--stacked.instrument-identity--has-market-tag
  .instrument-identity__primary {
  display: grid;
  grid-template-columns:
    var(--instrument-identity-tag-width)
    minmax(0, 1fr);
  column-gap: var(--instrument-identity-primary-gap);
}

.instrument-identity__title {
  min-width: 0;
  overflow: hidden;
  font-size: 0.875rem;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.instrument-identity__secondary {
  max-width: 100%;
  overflow: hidden;
  color: var(--tv-text-dim, #64748b);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.6875rem;
  font-variant-numeric: tabular-nums;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.instrument-identity--stacked.instrument-identity--has-market-tag
  .instrument-identity__secondary {
  max-width: calc(
    100% - var(--instrument-identity-tag-width) -
      var(--instrument-identity-primary-gap)
  );
  margin-inline-start: calc(
    var(--instrument-identity-tag-width) +
      var(--instrument-identity-primary-gap)
  );
}

.instrument-identity--compact .instrument-identity__primary {
  gap: var(--instrument-identity-primary-gap);
}

.instrument-identity--compact .instrument-identity__title {
  font-size: 0.75rem;
}

.instrument-identity--compact .instrument-identity__secondary {
  font-size: 0.625rem;
}

.instrument-identity__name {
  min-width: 0;
  overflow: hidden;
  color: inherit;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.instrument-identity__code {
  flex: 0 0 auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-variant-numeric: tabular-nums;
}

.instrument-identity__exchange-tag {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  min-height: 1.25rem;
  padding: 0 0.4rem;
  border: 1px solid color-mix(in srgb, currentColor 24%, transparent);
  border-radius: 999px;
  background: color-mix(in srgb, currentColor 8%, transparent);
  font-size: 0.7rem;
  font-weight: 600;
  line-height: 1.2;
}

.instrument-identity--compact .instrument-identity__exchange-tag {
  min-height: 1rem;
  padding: 0 0.3rem;
  font-size: 0.625rem;
}

.instrument-identity--stacked .instrument-identity__exchange-tag {
  box-sizing: border-box;
  width: var(--instrument-identity-tag-width);
  justify-content: center;
  min-height: 0.95rem;
  padding: 0 0.2rem;
  border-radius: 0.2rem;
  font-size: 0.5625rem;
}

.instrument-identity[data-category-market="CN"]
  .instrument-identity__exchange-tag {
  border-color: var(--instrument-market-tag-cn-border);
  background: var(--instrument-market-tag-cn-background);
  color: var(--instrument-market-tag-cn-text);
}

.instrument-identity[data-market="HK"]
  .instrument-identity__exchange-tag {
  border-color: var(--instrument-market-tag-hk-border);
  background: var(--instrument-market-tag-hk-background);
  color: var(--instrument-market-tag-hk-text);
}

.instrument-identity[data-market="US"]
  .instrument-identity__exchange-tag {
  border-color: var(--instrument-market-tag-us-border);
  background: var(--instrument-market-tag-us-background);
  color: var(--instrument-market-tag-us-text);
}
</style>
