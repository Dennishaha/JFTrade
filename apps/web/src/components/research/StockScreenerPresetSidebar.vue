<script setup lang="ts">
import { useStockScreenerControllerContext } from "./useStockScreenerController";

const {
  presets,
  newPreset,
  selectedPresetId,
  choosePresetFromSidebar,
} = useStockScreenerControllerContext();
</script>

<template>
  <aside class="stock-screener-view__preset-sidebar" aria-label="筛选策略">
    <div class="stock-screener-view__preset-sidebar-head">
      <strong>我的策略</strong>
      <span>{{ presets.length }}</span>
    </div>
    <button
      type="button"
      class="stock-screener-view__new-preset"
      @click="newPreset"
    >
      ＋ 新建策略
    </button>
    <div v-if="presets.length" class="stock-screener-view__preset-list">
      <button
        v-for="preset in presets"
        :key="preset.presetId"
        type="button"
        :class="{ 'is-active': selectedPresetId === preset.presetId }"
        @click="choosePresetFromSidebar(preset)"
      >
        <span>{{ preset.name }}</span>
      </button>
    </div>
    <div v-else class="stock-screener-view__preset-empty">
      还没有保存策略
    </div>
  </aside>
</template>

<style scoped>
.stock-screener-view__preset-sidebar {
  box-sizing: border-box;
  display: grid;
  width: 100%;
  max-width: 100%;
  height: 100%;
  align-content: start;
  min-width: 0;
  min-height: 0;
  overflow: auto;
  gap: 8px;
  padding: 8px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

.stock-screener-view__preset-sidebar-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--tv-text);
}

.stock-screener-view__preset-sidebar-head span {
  color: var(--tv-text-muted);
  font-size: 11px;
}

.stock-screener-view__new-preset {
  min-height: 34px !important;
  border-color: color-mix(in srgb, var(--tv-accent) 35%, var(--tv-border)) !important;
  background: color-mix(in srgb, var(--tv-accent) 9%, transparent) !important;
  color: var(--tv-accent) !important;
}

.stock-screener-view__preset-list {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.stock-screener-view__preset-list button {
  display: flex;
  min-width: 0;
  min-height: 34px;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  border-color: transparent;
  background: transparent;
  text-align: left;
}

.stock-screener-view__preset-list button span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stock-screener-view__preset-list button.is-active {
  border-color: var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.stock-screener-view__preset-empty {
  padding: 12px 4px;
  color: var(--tv-text-dim);
  font-size: 11px;
  text-align: center;
}

@container (max-width: 880px) {
  .stock-screener-view__preset-sidebar {
    grid-template-columns: auto minmax(0, 1fr);
    align-items: center;
  }

  .stock-screener-view__preset-sidebar-head {
    grid-column: 1;
  }

  .stock-screener-view__new-preset {
    grid-column: 2;
  }

  .stock-screener-view__preset-list,
  .stock-screener-view__preset-empty {
    grid-column: 1 / -1;
  }
}

@media (max-width: 900px) {
  .stock-screener-view__preset-sidebar {
    grid-template-columns: 1fr;
  }

  .stock-screener-view__preset-sidebar-head,
  .stock-screener-view__new-preset,
  .stock-screener-view__preset-list,
  .stock-screener-view__preset-empty {
    grid-column: auto;
  }
}
</style>
