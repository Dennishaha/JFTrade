<script setup lang="ts">
import AppTabs from "../shared/AppTabs.vue";
import ProviderUnsupportedState from "./ProviderUnsupportedState.vue";
import { useStockScreenerControllerContext } from "./useStockScreenerController";

const {
  queryMarket,
  changeMarket,
  selectedPresetId,
  choosePreset,
  presets,
  presetName,
  newPreset,
  savingPreset,
  savePreset,
  selectedPreset,
  removePreset,
  screenStatus,
  screenStatusLabel,
  entries,
  exportCSV,
  loading,
  catalogLoading,
  retryAfterMs,
  execute,
  catalogError,
  presetError,
  queryError,
  providerUnsupported,
  warnings,
  partialErrors,
  mobilePane,
} = useStockScreenerControllerContext();

const mobileTabs = [
  { value: "builder", label: "条件" },
  { value: "results", label: "结果" },
] as const;

function selectMobilePane(value: string): void {
  if (value !== "builder" && value !== "results") return;
  mobilePane.value = value;
}
</script>

<template>
  <header class="stock-screener-view__toolbar">
    <div class="stock-screener-view__title">
      <strong>股票筛选</strong>
      <select
        :value="queryMarket"
        aria-label="筛选市场"
        @change="changeMarket"
      >
        <option value="US">美股</option>
        <option value="HK">港股</option>
        <option value="SH">沪市</option>
        <option value="SZ">深市</option>
      </select>
    </div>
    <span class="stock-screener-view__scope">范围：全市场</span>
    <label>
      <span>预设</span>
      <select
        class="stock-screener-view__preset-select"
        :value="selectedPresetId"
        @change="choosePreset"
      >
        <option value="">未保存</option>
        <option
          v-for="preset in presets"
          :key="preset.presetId"
          :value="preset.presetId"
        >
          {{ preset.name }}
        </option>
      </select>
    </label>
    <input
      v-model="presetName"
      class="stock-screener-view__preset-name"
      aria-label="预设名称"
      placeholder="预设名称"
    />
    <button type="button" @click="newPreset">新建</button>
    <button
      type="button"
      :disabled="!presetName.trim() || savingPreset"
      @click="savePreset"
    >
      {{ savingPreset ? "保存中…" : "保存" }}
    </button>
    <button type="button" :disabled="!selectedPreset" @click="removePreset">
      删除
    </button>
    <span
      class="stock-screener-view__status"
      :class="`is-${screenStatus}`"
      role="status"
    >
      {{ screenStatusLabel }}
    </span>
    <span class="stock-screener-view__spacer" />
    <button type="button" :disabled="!entries.length" @click="exportCSV">
      导出 CSV
    </button>
    <button
      class="stock-screener-view__run"
      type="button"
      :disabled="loading || catalogLoading || retryAfterMs > 0"
      @click="execute(0, false)"
    >
      {{
        loading
          ? "筛选中…"
          : retryAfterMs > 0
            ? `稍后重试 (${Math.ceil(retryAfterMs / 1000)}s)`
            : "执行筛选"
      }}
    </button>
  </header>

  <ProviderUnsupportedState
    v-if="providerUnsupported"
    class="stock-screener-view__notice"
    bordered
    :min-height="72"
  />
  <div
    v-else-if="catalogError || presetError || queryError"
    class="stock-screener-view__notice tv-status--error tv-status-surface"
  >
    {{ catalogError || presetError || queryError }}
  </div>
  <div
    v-for="warning in warnings"
    :key="warning"
    class="stock-screener-view__notice tv-status--warning tv-status-surface"
  >
    {{ warning }}
  </div>
  <div
    v-for="(partialError, index) in partialErrors"
    :key="`${partialError.code ?? 'partial'}-${index}`"
    class="stock-screener-view__notice tv-status--warning tv-status-surface"
  >
    {{ partialError.message || partialError.code || "部分结果不可用" }}
  </div>

  <AppTabs
    class="stock-screener-view__mobile-tabs"
    variant="compact"
    fill
    :model-value="mobilePane"
    :items="[mobileTabs[0], { ...mobileTabs[1], count: entries.length }]"
    label="选股器页面"
    @update:model-value="selectMobilePane"
  />
</template>

<style scoped>
.stock-screener-view__toolbar {
  display: flex;
  min-width: 0;
  min-height: 44px;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.stock-screener-view__toolbar label,
.stock-screener-view__title {
  display: flex;
  align-items: center;
  gap: 6px;
}

.stock-screener-view__title {
  padding-right: 8px;
  border-right: 1px solid var(--tv-border);
}

.stock-screener-view__title span,
.stock-screener-view__toolbar label > span {
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
}

.stock-screener-view__preset-select,
.stock-screener-view__preset-name {
  width: 120px;
}

.stock-screener-view__scope {
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
  white-space: nowrap;
}

.stock-screener-view__spacer {
  flex: 1;
}

.stock-screener-view__status {
  display: inline-flex;
  min-height: 22px;
  align-items: center;
  padding: 0 7px;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
  white-space: nowrap;
}

.stock-screener-view__status.is-已保存 {
  border-color: color-mix(in srgb, #28a879 50%, var(--tv-border));
  color: #28a879;
}

.stock-screener-view__status.is-有未保存修改,
.stock-screener-view__status.is-待更新 {
  border-color: color-mix(in srgb, var(--jf-accent-amber-muted) 60%, var(--tv-border));
  color: var(--jf-accent-amber-muted);
}

.stock-screener-view__status.is-error {
  border-color: color-mix(in srgb, var(--jf-accent-red-muted) 60%, var(--tv-border));
  color: var(--jf-accent-red-muted);
}

.stock-screener-view__run {
  border-color: var(--tv-accent) !important;
  background: color-mix(in srgb, var(--tv-accent) 14%, transparent) !important;
  color: var(--tv-accent) !important;
  font-weight: 600 !important;
}

.stock-screener-view__notice {
  min-height: 32px;
  padding: 7px 8px;
  border: 1px solid;
  border-radius: 6px;
}

.stock-screener-view__mobile-tabs {
  display: none;
}

@media (max-width: 900px) {
  .stock-screener-view__toolbar {
    flex-wrap: wrap;
  }

  .stock-screener-view__spacer {
    display: none;
  }
}

@media (max-width: 768px) {
  .stock-screener-view__toolbar {
    position: sticky;
    z-index: 4;
    top: 0;
    min-height: 42px;
  }

  .stock-screener-view__mobile-tabs {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 4px;
  }

  .stock-screener-view__mobile-tabs :deep(.app-tabs__tab.is-active) {
    border-color: var(--tv-accent);
    color: var(--tv-accent);
  }

  .stock-screener-view__run {
    position: fixed;
    z-index: 6;
    right: 8px;
    bottom: 8px;
    left: 8px;
    min-height: 40px;
  }
}
</style>
