<script setup lang="ts">
import {
  PROVIDER_UNSUPPORTED_HINT,
  PROVIDER_UNSUPPORTED_LABEL,
} from "@/composables/research/providerCapabilityFallback";

withDefaults(
  defineProps<{ bordered?: boolean; minHeight?: number }>(),
  { bordered: false, minHeight: 120 },
);
</script>

<template>
  <div
    class="provider-unsupported-state"
    :class="{ 'provider-unsupported-state--bordered': bordered }"
    :style="{ minHeight: `${minHeight}px` }"
  >
    <strong>{{ PROVIDER_UNSUPPORTED_LABEL }}</strong>
    <span>{{ PROVIDER_UNSUPPORTED_HINT }}</span>
  </div>
</template>

<style scoped>
.provider-unsupported-state {
  display: grid;
  place-content: center;
  gap: 4px;
  color: var(--tv-text-dim);
  text-align: center;
}

.provider-unsupported-state span {
  font-size: var(--jf-text-4);
}

.provider-unsupported-state--bordered {
  border: 1px solid var(--tv-border);
  border-radius: var(--jf-radius-md);
  background: var(--tv-bg-surface);
}
</style>
