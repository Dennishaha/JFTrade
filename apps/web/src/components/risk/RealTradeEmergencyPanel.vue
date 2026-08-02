<script setup lang="ts">
import type { RealTradeKillSwitchStateResponse } from "@/contracts";

defineProps<{
  killSwitch: RealTradeKillSwitchStateResponse;
  loadingAction: string;
}>();

const emit = defineEmits<{
  activate: [];
  release: [];
}>();
</script>

<template>
  <section class="emergency-panel jf-panel" aria-label="紧急熔断">
    <header class="emergency-panel__head jf-panel__header">
      <span class="emergency-panel__title jf-panel__title">紧急熔断</span>
      <span
        class="emergency-panel__state"
        :class="killSwitch.killSwitchActive ? 'tv-status--error' : 'tv-status--success'"
      >
        <i class="tv-state-dot"></i>
        {{ killSwitch.killSwitchActive ? "正在阻断" : "未阻断" }}
      </span>
    </header>

    <div class="emergency-panel__body jf-panel__body">
      <div
        class="emergency-panel__summary"
        :class="killSwitch.killSwitchActive ? 'tv-status--error' : 'tv-status--success'"
      >
        <b>{{ killSwitch.killSwitchActive ? "下单与改单已被阻断" : "下单与改单未被熔断阻断" }}</b>
        <span>
          撤单{{ killSwitch.allowsCancel ? "允许" : "阻断" }} / 阻断
          {{ killSwitch.blockedOperations.join(" / ") }}
        </span>
      </div>

      <div class="emergency-panel__actions">
        <button
          type="button"
          class="tv-btn tv-btn-ghost emergency-panel__danger"
          :disabled="loadingAction === 'kill-switch.activate'"
          @click="emit('activate')"
        >
          {{ loadingAction === 'kill-switch.activate' ? "激活中..." : "激活熔断" }}
        </button>
        <button
          type="button"
          class="tv-btn tv-btn-ghost"
          :disabled="loadingAction === 'kill-switch.release'"
          @click="emit('release')"
        >
          {{ loadingAction === 'kill-switch.release' ? "解除中..." : "解除熔断" }}
        </button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.emergency-panel__head {
  justify-content: space-between;
  padding: 9px 12px;
}

.emergency-panel__state {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--tv-status-fg, var(--tv-text-muted));
  font-size: var(--jf-text-4);
}

.emergency-panel__body {
  gap: 10px;
}

.emergency-panel__summary {
  display: grid;
  gap: 3px;
  padding: 9px 11px;
  border: 1px solid var(--tv-status-border-color, var(--tv-border));
  border-radius: 6px;
  background: color-mix(
    in srgb,
    var(--tv-status-bg, var(--tv-bg-surface-2)) 45%,
    var(--tv-bg-surface)
  );
}

.emergency-panel__summary b {
  color: var(--tv-status-fg, var(--tv-text));
  font-size: var(--jf-text-6);
  font-weight: 600;
}

.emergency-panel__summary span {
  color: var(--tv-text-dim);
  font-size: var(--jf-text-4);
}

.emergency-panel__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.emergency-panel__actions .tv-btn {
  height: 28px;
  font-size: var(--jf-text-6);
}

.emergency-panel__danger:not(:disabled) {
  border-color: var(--tv-status-error-border);
  color: var(--tv-status-error-fg);
}

.emergency-panel__actions .tv-btn:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}
</style>
