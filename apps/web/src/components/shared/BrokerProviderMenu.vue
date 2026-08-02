<script setup lang="ts">
import type { BrokerProviderOption } from "@/composables/trading/brokerProviderSelection";

defineProps<{
  options: BrokerProviderOption[];
  selectedOption: BrokerProviderOption | null;
  switching: boolean;
  error: string;
  loading: boolean;
  loadError: string;
}>();

const emit = defineEmits<{
  select: [option: BrokerProviderOption];
}>();
</script>

<template>
  <div
    class="broker-provider-tag__menu"
    role="listbox"
    aria-label="行情提供者"
  >
    <div class="broker-provider-tag__heading">
      <strong>行情提供者</strong>
      <small>选择会应用到研究与产品行情</small>
    </div>
    <div
      v-if="switching"
      class="broker-provider-tag__empty"
      aria-live="polite"
    >
      正在启动内置行情提供者，首次启动可能需要几十秒…
    </div>
    <button
      v-for="option in options"
      :key="option.id"
      type="button"
      role="option"
      :aria-selected="option.id === selectedOption?.id"
      :disabled="!option.selectable || switching"
      :class="[
        `is-${option.displayState ?? option.state}`,
        { 'is-selected': option.id === selectedOption?.id },
      ]"
      @click="emit('select', option)"
    >
      <span class="broker-provider-tag__option-dot" />
      <span>
        <strong>{{ option.label }}</strong>
        <small v-if="option.securityFirm">{{ option.securityFirm }}</small>
        <small v-if="option.reason">{{ option.reason }}</small>
      </span>
      <span v-if="option.id === selectedOption?.id" aria-hidden="true">✓</span>
    </button>
    <div v-if="error" class="broker-provider-tag__empty">
      {{ error }}
    </div>
    <div v-if="options.length === 0" class="broker-provider-tag__empty">
      {{ loading ? "正在读取券商能力…" : loadError || "暂无可用提供者" }}
    </div>
  </div>
</template>

<style scoped>
.broker-provider-tag__option-dot {
  width: 8px;
  height: 8px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--tv-text-dim);
}
.broker-provider-tag__menu button.is-available .broker-provider-tag__option-dot {
  background: var(--tv-status-success-fg);
}
.broker-provider-tag__menu button.is-degraded .broker-provider-tag__option-dot {
  background: var(--tv-status-warning-fg);
}
.broker-provider-tag__menu button.is-unavailable .broker-provider-tag__option-dot {
  background: var(--tv-status-error-fg);
}
.broker-provider-tag__menu {
  width: min(280px, calc(100vw - 24px));
  padding: 6px;
  border: 1px solid var(--tv-border-strong);
  border-radius: 7px;
  background: var(--tv-bg-surface);
  box-shadow: 0 12px 30px rgb(0 0 0 / 28%);
}
.broker-provider-tag__heading {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  padding: 5px 7px 7px;
}
.broker-provider-tag__heading strong {
  color: var(--tv-text);
  font-size: 10px;
}
.broker-provider-tag__heading small {
  color: var(--tv-text-dim);
  font-size: 8px;
}
.broker-provider-tag__menu button {
  display: grid;
  width: 100%;
  grid-template-columns: 7px minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  padding: 7px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  text-align: left;
}
.broker-provider-tag__menu button:hover,
.broker-provider-tag__menu button.is-selected {
  background: var(--tv-bg-surface-2);
  color: var(--tv-text);
}
.broker-provider-tag__menu button:disabled {
  cursor: not-allowed;
  opacity: 0.48;
}
.broker-provider-tag__menu button > span:nth-child(2) {
  display: flex;
  min-width: 0;
  flex-direction: column;
}
.broker-provider-tag__menu button strong {
  overflow: hidden;
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.broker-provider-tag__menu button small,
.broker-provider-tag__empty {
  overflow: hidden;
  color: var(--tv-text-dim);
  font-size: 8px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.broker-provider-tag__empty {
  padding: 10px 7px;
}
</style>
