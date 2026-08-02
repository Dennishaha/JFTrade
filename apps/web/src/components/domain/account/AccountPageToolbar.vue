<script setup lang="ts">
import { ACCOUNT_TABS, type AccountTab } from "@/features/accountPage";

defineProps<{
  activeTab: AccountTab;
  pendingOrderCount: number;
  refreshing: boolean;
  error: string;
}>();

const emit = defineEmits<{
  select: [tab: AccountTab];
  refresh: [];
}>();
</script>

<template>
  <div class="account-page__tabs-row">
    <div class="account-page__tabs" role="tablist" aria-label="账户视图">
      <button
        v-for="tab in ACCOUNT_TABS"
        :key="tab.value"
        type="button"
        role="tab"
        :aria-selected="activeTab === tab.value"
        :class="{ 'is-active': activeTab === tab.value }"
        @click="emit('select', tab.value)"
      >
        {{ tab.label }}
        <span v-if="tab.value === 'orders' && pendingOrderCount">
          {{ pendingOrderCount }}
        </span>
      </button>
    </div>
    <button
      type="button"
      class="account-page__refresh"
      :disabled="refreshing"
      aria-label="刷新账户数据"
      title="刷新账户数据"
      @click="emit('refresh')"
    >
      <span
        class="account-page__refresh-icon"
        :class="{ 'is-refreshing': refreshing }"
        aria-hidden="true"
      >&#x21BB;</span>
      刷新
    </button>
  </div>
  <v-alert
    v-if="error"
    type="warning"
    :closable="false"
    density="compact"
    data-testid="account-live-data-error"
    class="account-page__data-error"
  >
    {{ error }}
  </v-alert>
</template>

<style scoped>
.account-page__tabs-row {
  display: flex;
  flex: 0 0 auto;
  align-items: stretch;
  min-width: 0;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}
.account-page__data-error {
  flex: 0 0 auto;
  margin: 8px 12px 0;
}
.account-page__tabs {
  display: flex;
  min-width: 0;
  flex: 1;
  gap: 2px;
  padding: 5px 7px 0;
  overflow-x: auto;
  scrollbar-width: thin;
}
.account-page__tabs button {
  position: relative;
  flex: 0 0 auto;
  padding: 8px 14px 9px;
  border: 0;
  border-radius: 6px 6px 0 0;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: var(--jf-text-5);
}
.account-page__tabs button span {
  margin-left: 4px;
  color: var(--tv-text-dim);
  font-size: var(--jf-text-3);
}
.account-page__tabs button.is-active {
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font-weight: 650;
}
.account-page__tabs button.is-active::after {
  position: absolute;
  right: 8px;
  bottom: -1px;
  left: 8px;
  height: 2px;
  background: var(--tv-accent);
  content: "";
}
.account-page__refresh {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  margin: 4px 8px 4px 4px;
  padding: 4px 10px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-muted);
  cursor: pointer;
  font-size: var(--jf-text-5);
}
.account-page__refresh:hover:not(:disabled) {
  border-color: var(--tv-accent);
  color: var(--tv-text);
}
.account-page__refresh:disabled {
  cursor: default;
  opacity: 0.6;
}
.account-page__refresh-icon {
  display: inline-block;
  font-size: var(--jf-text-6);
  line-height: 1;
}
.account-page__refresh-icon.is-refreshing {
  animation: account-page-refresh-spin 0.8s linear infinite;
}
@keyframes account-page-refresh-spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
