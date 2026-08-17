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

<!-- 样式在 styles/components.css（共享空态样式，与 .jf-empty-panel 同处），
     避免给 scoped 样式总量预算带来增量。 -->
