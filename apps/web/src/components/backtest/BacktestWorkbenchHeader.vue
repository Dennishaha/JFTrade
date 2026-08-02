<script setup lang="ts">
import { useBacktestPageContext } from "@/composables/backtest/useBacktestPage";

const {
  activateComparisonMode,
  activateSingleReportMode,
  backtestMobileSection,
  backtestSidebarOpen,
  error,
  errorExpanded,
  focusedRun,
  isTerminalBacktestStatus,
  openNewBacktestForm,
  reportMode,
  requestDeleteRun,
  selectBacktestMobileSection,
  toggleBacktestSidebar,
} = useBacktestPageContext();
</script>

<template>
  <header class="backtest-workbench-header">
    <div class="backtest-workbench-header__identity">
      <button
        type="button"
        class="backtest-sidebar-toggle"
        :class="{ 'is-active': backtestSidebarOpen || backtestMobileSection === 'setup' }"
        :aria-expanded="backtestSidebarOpen"
        aria-controls="backtest-sidebar"
        data-testid="backtest-sidebar-toggle"
        title="显示或隐藏回测配置与历史"
        @click="toggleBacktestSidebar"
      >
        <v-icon size="14">fa-solid fa-table-columns</v-icon>
        <span>配置与历史</span>
      </button>
      <div class="backtest-workbench-title">
        <h1>回测工作台</h1>
      </div>
    </div>
    <div class="backtest-workbench-header__actions">
      <div class="backtest-report-mode-switch" aria-label="回测报告视图">
        <button
          type="button"
          class="backtest-report-mode-switch__button"
          :class="{ 'is-active': reportMode === 'single' }"
          data-testid="backtest-report-mode-single"
          @click="activateSingleReportMode"
        >
          单次报告
        </button>
        <button
          type="button"
          class="backtest-report-mode-switch__button"
          :class="{ 'is-active': reportMode === 'compare' }"
          data-testid="backtest-open-version-comparison"
          @click="activateComparisonMode"
        >
          版本对比
        </button>
      </div>
      <button
        v-if="reportMode === 'single' && focusedRun && isTerminalBacktestStatus(focusedRun.status)"
        type="button"
        class="backtest-header-icon-button backtest-header-icon-button--danger"
        title="删除回测结果"
        aria-label="删除当前回测结果"
        @click="requestDeleteRun(focusedRun.id)"
      >
        <v-icon size="13">fa-solid fa-trash</v-icon>
      </button>
      <button
        type="button"
        class="backtest-header-action backtest-header-action--primary"
        data-testid="backtest-open-new-form"
        @click="openNewBacktestForm"
      >
        <v-icon size="13">fa-solid fa-plus</v-icon>
        新建回测
      </button>
    </div>
  </header>

  <div
    v-if="error"
    class="backtest-error-banner"
    :class="{ 'is-expanded': errorExpanded }"
    :title="error"
  >
    <button
      type="button"
      class="backtest-error-banner__content"
      :aria-expanded="errorExpanded"
      @click="errorExpanded = !errorExpanded"
    >
      <v-icon size="13">fa-solid fa-triangle-exclamation</v-icon>
      <span>{{ error }}</span>
      <v-icon size="12">
        {{ errorExpanded ? "fa-solid fa-chevron-up" : "fa-solid fa-chevron-down" }}
      </v-icon>
    </button>
    <button
      type="button"
      class="backtest-error-banner__close"
      aria-label="关闭错误提示"
      @click="error = ''"
    >
      <v-icon size="12">fa-solid fa-xmark</v-icon>
    </button>
  </div>

  <nav class="backtest-page__mobile-switch" aria-label="回测移动端工作区">
    <button
      class="backtest-page__mobile-switch-button"
      :class="{ 'is-active': backtestMobileSection === 'setup' }"
      data-testid="backtest-mobile-section-setup"
      type="button"
      @click="selectBacktestMobileSection('setup')"
    >
      配置与历史
    </button>
    <button
      class="backtest-page__mobile-switch-button"
      :class="{ 'is-active': backtestMobileSection === 'report' }"
      data-testid="backtest-mobile-section-report"
      :disabled="focusedRun == null && reportMode !== 'compare'"
      type="button"
      @click="selectBacktestMobileSection('report')"
    >
      报告
    </button>
  </nav>
</template>

<style scoped>
.backtest-workbench-header {
  display: flex;
  min-width: 0;
  min-height: 44px;
  flex: 0 0 44px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.backtest-workbench-header__identity,
.backtest-workbench-title,
.backtest-workbench-header__actions {
  display: flex;
  min-width: 0;
  align-items: center;
}

.backtest-workbench-header__identity {
  flex: 1 1 auto;
  gap: 6px;
  overflow: hidden;
}

.backtest-workbench-title {
  flex: 0 1 auto;
  gap: 7px;
  overflow: hidden;
}

.backtest-workbench-title h1 {
  flex: 0 0 auto;
  margin: 0;
  font-size: 0.92rem;
  line-height: 1;
  white-space: nowrap;
}

.backtest-workbench-header__actions {
  flex: 0 0 auto;
  justify-content: flex-end;
  gap: 4px;
}

.backtest-sidebar-toggle,
.backtest-header-action,
.backtest-header-icon-button,
.backtest-report-mode-switch__button {
  min-height: 30px;
  height: 30px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font-size: 0.77rem;
  font-weight: 750;
}

.backtest-sidebar-toggle,
.backtest-header-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  padding: 0 8px;
}

.backtest-sidebar-toggle {
  border-color: transparent;
  background: transparent;
  color: var(--tv-text-muted);
}

.backtest-sidebar-toggle:is(:hover, .is-active) {
  border-color: var(--tv-border);
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.backtest-header-action--primary {
  border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 14%, var(--tv-bg-surface));
  color: var(--tv-accent);
}

.backtest-header-icon-button {
  display: inline-grid;
  width: 30px;
  place-items: center;
  padding: 0;
}

.backtest-header-icon-button--danger {
  border-color: transparent;
  background: transparent;
  color: var(--tv-status-error-fg);
}

.backtest-report-mode-switch {
  display: inline-grid;
  grid-template-columns: repeat(2, minmax(4rem, 1fr));
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 72%, transparent);
  padding: 2px;
}

.backtest-report-mode-switch__button {
  min-width: 4rem;
  min-height: 26px;
  height: 26px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  padding: 0 7px;
  color: var(--tv-text-muted);
}

.backtest-report-mode-switch__button.is-active {
  background: color-mix(in srgb, var(--tv-accent) 22%, var(--tv-bg-surface));
  color: var(--tv-text);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tv-accent) 36%, transparent);
}

.backtest-error-banner {
  display: flex;
  min-width: 0;
  min-height: 30px;
  flex: 0 0 30px;
  align-items: center;
  gap: 7px;
  border: 0;
  border-bottom: 1px solid color-mix(in srgb, var(--jf-accent-red) 42%, var(--tv-border));
  border-radius: 0;
  background: color-mix(in srgb, var(--jf-accent-red) 9%, var(--tv-bg-surface));
  padding: 0 4px 0 8px;
  color: color-mix(in srgb, var(--jf-accent-red-text) 78%, var(--tv-text));
  font-size: 0.76rem;
  text-align: left;
}

.backtest-error-banner__content {
  display: flex;
  min-width: 0;
  flex: 1 1 auto;
  align-items: center;
  gap: 7px;
  align-self: stretch;
  border: 0;
  background: transparent;
  color: inherit;
  padding: 5px 0;
  font-size: inherit;
  text-align: left;
}

.backtest-error-banner__content > span {
  min-width: 0;
  flex: 1 1 auto;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.backtest-error-banner.is-expanded {
  min-height: 30px;
  flex-basis: auto;
}

.backtest-error-banner.is-expanded .backtest-error-banner__content > span {
  overflow: visible;
  white-space: normal;
}

.backtest-error-banner__close {
  display: inline-grid;
  width: 24px;
  min-height: 24px;
  flex: 0 0 24px;
  place-items: center;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: inherit;
}

.backtest-page__mobile-switch {
  display: none;
}

@media (min-width: 769px) and (max-width: 1180px) {
  .backtest-workbench-header {
    gap: 4px;
    padding-inline: 6px;
  }
}

@media (max-width: 920px) and (min-width: 769px) {
  .backtest-sidebar-toggle span {
    display: none;
  }
}

@media (max-width: 768px) {
  .backtest-workbench-header {
    min-height: 44px;
    height: auto;
    flex: 0 0 auto;
    flex-flow: row wrap;
    gap: 4px 8px;
    padding: 5px 6px;
  }

  .backtest-workbench-header__identity {
    flex-basis: 100%;
  }

  .backtest-sidebar-toggle span {
    display: none;
  }

  .backtest-workbench-title {
    flex: 1 1 auto;
  }

  .backtest-workbench-header__actions {
    width: 100%;
    justify-content: flex-start;
  }

  .backtest-page__mobile-switch {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    min-height: 40px;
    flex: 0 0 40px;
    gap: 1px;
    min-width: 0;
    border-bottom: 1px solid var(--tv-border);
    background: var(--tv-bg-surface);
    padding: 3px 6px;
  }

  .backtest-page__mobile-switch-button {
    min-width: 0;
    min-height: 34px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: var(--tv-text-muted);
    font-size: 0.77rem;
    font-weight: 800;
    line-height: 1.2;
  }

  .backtest-page__mobile-switch-button.is-active {
    background: color-mix(in srgb, var(--tv-accent) 18%, var(--tv-bg-surface));
    color: var(--tv-text);
  }

  .backtest-page__mobile-switch-button:disabled {
    cursor: not-allowed;
    opacity: 0.55;
  }
}
</style>
