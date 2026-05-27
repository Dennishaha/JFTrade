<script setup lang="ts">
import { computed } from "vue";

import { formatTradingEnvironment } from "../composables/consoleDataFormatting";

const props = defineProps<{ env: string }>();

interface BadgeStyle {
  background: string;
  label: string;
}

function resolveBadgeStyle(env: string): BadgeStyle {
  switch (env) {
    case "REAL":
      return { background: "#dc2626", label: formatTradingEnvironment(env) };
    case "PAPER":
      return { background: "#16a34a", label: formatTradingEnvironment(env) };
    case "SIMULATE":
    case "SIM":
      return { background: "#2563eb", label: formatTradingEnvironment(env) };
    default:
      return { background: "#6b7280", label: formatTradingEnvironment(env) };
  }
}

const badge = computed(() => resolveBadgeStyle(props.env));
</script>

<template>
  <span
    :style="{ background: badge.background, color: '#ffffff' }"
    class="inline-flex items-center rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-[0.2em]"
  >
    {{ badge.label }}
  </span>
</template>
