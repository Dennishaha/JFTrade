<script setup lang="ts">
import { useBacktestPageContext } from "@/composables/backtest/useBacktestPage";
import BacktestHistoryPanel from "./BacktestHistoryPanel.vue";
import BacktestSetupPanel from "./BacktestSetupPanel.vue";

const { closeBacktestSidebar, resultsPageSummary } = useBacktestPageContext();
</script>

<template>
  <aside id="backtest-sidebar" class="backtest-page__pane backtest-page__pane--sidebar">
    <div class="bt-sidebar-shell">
      <div class="bt-sidebar-drawer-head">
        <div>
          <strong>配置与历史</strong>
          <span>{{ resultsPageSummary || "回测结果由服务端提供。" }}</span>
        </div>
        <button type="button" aria-label="关闭回测配置与历史" @click="closeBacktestSidebar">
          <v-icon size="14">fa-solid fa-xmark</v-icon>
        </button>
      </div>

      <div class="bt-sidebar-panels">
        <BacktestSetupPanel />
        <BacktestHistoryPanel />
      </div>
    </div>
  </aside>
</template>

<style scoped>
.backtest-page__pane {
  display: flex;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.backtest-page__pane > * {
  min-width: 0;
}

.backtest-page__pane--sidebar {
  container-type: inline-size;
  background: var(--tv-bg-surface);
}

.bt-sidebar-shell,
.bt-sidebar-panels {
  display: flex;
  width: 100%;
  min-width: 0;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
  overflow: hidden;
}

.bt-sidebar-drawer-head {
  display: none;
  min-height: 40px;
  flex: 0 0 40px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
}

.bt-sidebar-drawer-head > div {
  display: grid;
  min-width: 0;
  gap: 1px;
}

.bt-sidebar-drawer-head strong {
  color: var(--tv-text);
  font-size: 0.8rem;
}

.bt-sidebar-drawer-head span {
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.68rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bt-sidebar-drawer-head button {
  display: inline-grid;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-muted);
}

.bt-sidebar-panels :deep(.bt-sidebar-panel) {
  display: flex;
  min-width: 0;
  min-height: 34px;
  flex: 0 0 34px;
  flex-direction: column;
  overflow: hidden;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.bt-sidebar-panels :deep(.bt-sidebar-panel.is-expanded) {
  min-height: 96px;
  flex: 1 1 0;
}

.bt-sidebar-panels :deep(.bt-sidebar-panel__title) {
  display: grid;
  width: 100%;
  min-width: 0;
  min-height: 34px;
  flex: 0 0 34px;
  grid-template-columns: 12px minmax(0, 1fr) auto;
  align-items: center;
  gap: 6px;
  border: 0;
  border-radius: 0;
  background: var(--tv-bg-surface);
  padding: 0 8px;
  color: var(--tv-text-muted);
  text-align: left;
}

.bt-sidebar-panels :deep(.bt-sidebar-panel__title:hover) {
  background: var(--tv-bg-elevated);
}

.bt-sidebar-panels :deep(.bt-sidebar-panel__title > .v-icon) {
  transform: rotate(0deg);
  transition: transform 120ms ease;
}

.bt-sidebar-panels :deep(.bt-sidebar-panel.is-expanded > .bt-sidebar-panel__title > .v-icon) {
  transform: rotate(90deg);
}

.bt-sidebar-panels :deep(.bt-sidebar-panel__title span) {
  font-size: 0.78rem;
  font-weight: 750;
  letter-spacing: var(--jf-tracking-1);
}

.bt-sidebar-panels :deep(.bt-sidebar-panel__title em) {
  max-width: 12rem;
  overflow: hidden;
  font-size: 0.67rem;
  font-style: normal;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bt-sidebar-panels :deep(.bt-sidebar-panel__body) {
  min-width: 0;
  min-height: 0;
  flex: 1 1 auto;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.bt-sidebar-panels :deep(.bt-sidebar-panel__body--setup) {
  overflow: hidden;
}

.bt-sidebar-panels :deep(.bt-native-input),
.bt-sidebar-panels :deep(.bt-native-select),
.bt-sidebar-panels :deep(.bt-native-textarea) {
  width: 100%;
  min-width: 0;
  min-height: 32px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 5px 8px;
  font-size: 0.83rem;
  line-height: 1.25;
  outline: none;
}

.bt-sidebar-panels :deep(.bt-native-input:focus),
.bt-sidebar-panels :deep(.bt-native-select:focus),
.bt-sidebar-panels :deep(.bt-native-textarea:focus) {
  border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
}

.bt-sidebar-panels :deep(.bt-native-input:disabled),
.bt-sidebar-panels :deep(.bt-native-select:disabled),
.bt-sidebar-panels :deep(.bt-native-textarea:disabled) {
  cursor: not-allowed;
  opacity: 0.55;
}

.bt-sidebar-panels :deep(.bt-native-textarea) {
  min-height: 56px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  resize: vertical;
}

.bt-sidebar-panels :deep(.bt-sidebar-panel--setup.is-expanded + .bt-sidebar-panel--history.is-expanded) {
  flex-grow: 1.25;
}

@container (max-width: 360px) {
  .bt-sidebar-panels :deep(.grid-cols-2) {
    grid-template-columns: minmax(0, 1fr) !important;
  }
}

@container (max-width: 320px) {
  .bt-sidebar-panels :deep(.bt-sidebar-pagination .v-btn) {
    height: 28px;
    min-width: 28px;
    width: 28px;
  }
}

@media (min-width: 769px) and (max-width: 1180px) {
  .bt-sidebar-drawer-head {
    display: flex;
  }
}

@media (max-width: 768px) {
  .bt-sidebar-drawer-head {
    display: none;
  }
}
</style>
