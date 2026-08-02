<script setup lang="ts">
import type { Ref } from "vue";

import type {
  StrategyDefinitionVersionDocument,
  StrategyDefinitionVersionSummary,
} from "@/composables/strategy/strategyDefinitionVersions";
import { useStrategyDesignContext } from "./strategyDesignContext";
import type { StrategySidePanelId } from "./useStrategySidePanelLayout";

interface HistoryContext {
  definitionName: Ref<string>;
  selectedDefinitionId: Ref<string>;
  definitionVersions: Ref<StrategyDefinitionVersionSummary[]>;
  isLoadingDefinitionVersions: Ref<boolean>;
  definitionVersionsError: Ref<string>;
  selectedVersionSnapshot: Ref<StrategyDefinitionVersionDocument | null>;
  isLoadingVersionSnapshot: Ref<boolean>;
  versionSnapshotError: Ref<string>;
  selectedComparisonVersions: Readonly<Ref<StrategyDefinitionVersionSummary[]>>;
  canOpenVersionComparison: Readonly<Ref<boolean>>;
  isWideWorkbench: Ref<boolean>;
  strategySidePanelClasses: (id: StrategySidePanelId) => Record<string, boolean>;
  strategySidePanelPosition: (id: StrategySidePanelId) => number;
  handleStrategySidePanelDragOver: (event: DragEvent, id: StrategySidePanelId) => void;
  handleStrategySidePanelDragStart: (event: DragEvent, id: StrategySidePanelId) => void;
  handleStrategySidePanelDrop: (event: DragEvent) => void;
  loadDefinitionVersions: () => Promise<void>;
  isVersionSelectedForComparison: (version: string) => boolean;
  versionSelectionDisabled: (version: string) => boolean;
  toggleVersionForComparison: (version: string) => void;
  formatVersionSavedAt: (value: string) => string;
  showVersionSnapshot: (version: string) => Promise<void>;
  openVersionComparison: () => void;
}

const context = useStrategyDesignContext<HistoryContext>();
</script>

<template>
  <v-expansion-panel
    value="history"
    class="strategy-native-panel strategy-native-side-panel"
    :class="context.strategySidePanelClasses('history')"
    :style="{ order: context.strategySidePanelPosition('history') }"
    data-testid="strategy-side-panel-history"
  >
    <v-expansion-panel-title
      collapse-icon="fa-solid fa-chevron-right"
      data-testid="strategy-side-panel-history-title"
      :draggable="context.isWideWorkbench.value"
      expand-icon="fa-solid fa-chevron-right"
      title="拖动调整位置"
      @dragover.prevent="context.handleStrategySidePanelDragOver($event, 'history')"
      @dragstart="context.handleStrategySidePanelDragStart($event, 'history')"
      @drop.prevent="context.handleStrategySidePanelDrop"
    >
      <div class="strategy-native-side-panel__heading">
        <div class="strategy-native-panel__title">版本历史</div>
        <span class="strategy-native-panel-count">{{ context.definitionVersions.value.length }}</span>
      </div>
    </v-expansion-panel-title>
    <v-expansion-panel-text>
      <div class="strategy-native-panel__content">
        <div class="strategy-native-panel-tools">
          <span>不可变版本</span>
          <button
            type="button"
            class="strategy-native-icon-button"
            :disabled="context.selectedDefinitionId.value === '' || context.isLoadingDefinitionVersions.value"
            title="刷新版本历史"
            aria-label="刷新版本历史"
            @click.stop="void context.loadDefinitionVersions()"
          >
            <v-icon size="13">fa-solid fa-arrow-rotate-right</v-icon>
          </button>
        </div>
        <div v-if="context.selectedDefinitionId.value === ''" class="strategy-native-meta">
          保存策略后会生成首个不可变版本。
        </div>
        <div v-else-if="context.isLoadingDefinitionVersions.value" class="strategy-native-meta">
          正在加载版本历史…
        </div>
        <div v-else-if="context.definitionVersionsError.value" class="strategy-native-version-notice">
          版本历史暂不可用：{{ context.definitionVersionsError.value }}
        </div>
        <div v-else-if="context.definitionVersions.value.length === 0" class="strategy-native-meta">
          暂无已保存版本。
        </div>
        <template v-else>
          <div class="strategy-native-version-compare-note">
            选择两个版本后可在回测页比较其已完成回测；左侧为较早基线，右侧为较新候选。
          </div>
          <section
            v-for="version in context.definitionVersions.value"
            :key="version.version"
            class="strategy-native-version-entry"
            :class="{ 'is-selected': context.isVersionSelectedForComparison(version.version) }"
            :data-testid="`strategy-version-entry-${version.version}`"
          >
            <label class="strategy-native-version-entry__select">
              <input
                :checked="context.isVersionSelectedForComparison(version.version)"
                :disabled="context.versionSelectionDisabled(version.version)"
                type="checkbox"
                @change="context.toggleVersionForComparison(version.version)"
              >
              <span>
                <strong>v{{ version.version }}</strong>
                <em v-if="version.isCurrent">当前</em>
              </span>
            </label>
            <div class="strategy-native-version-entry__meta">
              <span>{{ version.name || context.definitionName.value || "未命名策略" }}</span>
              <span>{{ context.formatVersionSavedAt(version.savedAt) }}</span>
            </div>
            <button
              type="button"
              class="strategy-native-version-entry__view"
              :disabled="context.isLoadingVersionSnapshot.value"
              @click="void context.showVersionSnapshot(version.version)"
            >
              查看源码
            </button>
          </section>
          <button
            type="button"
            class="strategy-native-version-compare-button"
            data-testid="strategy-open-version-comparison"
            :disabled="!context.canOpenVersionComparison.value"
            @click="context.openVersionComparison"
          >
            比较选中版本（{{ context.selectedComparisonVersions.value.length }}/2）
          </button>
        </template>
        <div v-if="context.isLoadingVersionSnapshot.value" class="strategy-native-meta">
          正在加载历史源码…
        </div>
        <div v-else-if="context.versionSnapshotError.value" class="strategy-native-version-notice">
          历史源码不可用：{{ context.versionSnapshotError.value }}
        </div>
        <details v-else-if="context.selectedVersionSnapshot.value" class="strategy-native-version-source" open>
          <summary>v{{ context.selectedVersionSnapshot.value.version }} 只读源码</summary>
          <pre>{{ context.selectedVersionSnapshot.value.script || "该版本没有可用源码快照。" }}</pre>
        </details>
      </div>
    </v-expansion-panel-text>
  </v-expansion-panel>
</template>

<style scoped>
.strategy-native-version-compare-note,
.strategy-native-version-notice {
  border: 1px solid var(--tv-border);
  padding: 5px 7px;
  border-radius: 5px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 72%, transparent);
  color: var(--tv-text-muted);
  font-size: 0.73rem;
  line-height: 1.35;
}

.strategy-native-version-notice {
  border-color: color-mix(in srgb, var(--jf-accent-amber) 44%, var(--tv-border));
  background: color-mix(in srgb, var(--jf-accent-amber) 10%, var(--tv-bg-surface));
  color: color-mix(in srgb, var(--jf-accent-amber-text) 72%, var(--tv-text));
  overflow-wrap: anywhere;
}

.strategy-native-version-entry {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 3px 6px;
  align-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  padding: 5px 6px;
}

.strategy-native-version-entry.is-selected {
  border-color: color-mix(in srgb, var(--tv-accent) 58%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 9%, var(--tv-bg-surface));
}

.strategy-native-version-entry__select {
  display: inline-flex !important;
  min-width: 0;
  align-items: center;
  gap: 5px;
  color: var(--tv-text) !important;
  font-size: 0.8rem !important;
  letter-spacing: 0 !important;
  text-transform: none !important;
}

.strategy-native-version-entry__select input {
  width: auto !important;
}

.strategy-native-version-entry__select span {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.strategy-native-version-entry__select em {
  border-radius: 999px;
  background: color-mix(in srgb, var(--tv-accent) 18%, var(--tv-bg-surface));
  color: var(--tv-accent);
  padding: 1px 4px;
  font-size: 0.65rem;
  font-style: normal;
  font-weight: 800;
}

.strategy-native-version-entry__meta {
  grid-column: 1;
  display: flex;
  gap: 5px;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.69rem;
  white-space: nowrap;
}

.strategy-native-version-entry__meta span {
  overflow: hidden;
  text-overflow: ellipsis;
}

.strategy-native-version-entry__view {
  grid-column: 2;
  grid-row: 1 / span 2;
  align-self: center;
  min-height: 28px;
  padding: 0 6px;
  font-size: 0.71rem;
}

.strategy-native-version-compare-button {
  width: 100%;
  min-height: 32px;
  padding: 0 8px;
  font-size: 0.77rem;
}

.strategy-native-version-source {
  display: grid;
  gap: 5px;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-elevated);
  padding: 6px;
}

.strategy-native-version-source summary {
  cursor: pointer;
  color: var(--tv-text);
  font-size: 0.77rem;
  font-weight: 800;
}

.strategy-native-version-source pre {
  max-height: 14rem;
  margin: 0;
  overflow: auto;
  color: var(--tv-text);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.73rem;
  line-height: 1.35;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
