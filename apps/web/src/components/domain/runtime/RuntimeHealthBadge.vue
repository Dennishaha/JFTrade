<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(defineProps<{
  status: string;
  label?: string;
  disabled?: boolean;
}>(), {
  label: "",
  disabled: false,
});

const normalizedStatus = computed(() => props.status.trim().toUpperCase());
const tone = computed(() => {
  if (props.disabled) return "disabled";
  switch (normalizedStatus.value) {
    case "RUNNING":
    case "HEALTHY":
    case "CONNECTED":
      return "healthy";
    case "PAUSED":
    case "DEGRADED":
    case "WARNING":
      return "warning";
    case "FAILED":
    case "ERROR":
    case "DISCONNECTED":
      return "error";
    default:
      return "idle";
  }
});

const defaultLabel = computed(() => {
  switch (normalizedStatus.value) {
    case "RUNNING": return "运行中";
    case "PAUSED": return "已暂停";
    case "STOPPED": return "已停止";
    case "FAILED": return "执行失败";
    default: return normalizedStatus.value || "未知";
  }
});
</script>

<template>
  <span
    class="runtime-health-badge"
    :class="`runtime-health-badge--${tone}`"
    :data-status="normalizedStatus"
    :data-tone="tone"
    :aria-disabled="disabled ? 'true' : undefined"
  >
    <span class="runtime-health-badge__dot" aria-hidden="true"></span>
    {{ label || defaultLabel }}
  </span>
</template>

<style scoped>
.runtime-health-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid rgb(226 232 240);
  border-radius: 999px;
  padding: 0.2rem 0.65rem;
  background: rgb(248 250 252);
  color: rgb(71 85 105);
  font-size: 0.72rem;
  font-weight: 700;
  letter-spacing: 0;
  text-transform: uppercase;
  white-space: nowrap;
}

.runtime-health-badge__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.runtime-health-badge--healthy {
  border-color: rgb(167 243 208);
  background: rgb(236 253 245);
  color: rgb(4 120 87);
}

.runtime-health-badge--warning {
  border-color: rgb(253 230 138);
  background: rgb(254 249 195);
  color: rgb(161 98 7);
}

.runtime-health-badge--error {
  border-color: rgb(254 202 202);
  background: rgb(254 242 242);
  color: rgb(185 28 28);
}

.runtime-health-badge--disabled {
  opacity: 0.55;
}
</style>
