<script setup lang="ts">
import type { Ref } from "vue";

import { useStrategyDesignContext } from "./strategyDesignContext";
import type { StrategyDisplayMode, StrategyMobileSection } from "./useStrategySidePanelLayout";

interface HeaderContext {
  definitionName: Ref<string>;
  definitionVersion: Ref<string>;
  totalErrorCount: Readonly<Ref<number>>;
  totalDiagnosticCount: Readonly<Ref<number>>;
  metadataPaneOpen: Ref<boolean>;
  canUndoSourceChange: Readonly<Ref<boolean>>;
  canRedoSourceChange: Readonly<Ref<boolean>>;
  strategyDisplayMode: Ref<StrategyDisplayMode>;
  strategyMobileSection: Ref<StrategyMobileSection>;
  isAnalyzing: Ref<boolean>;
  isSavingDefinition: Ref<boolean>;
  actionFeedback: Ref<"analyze" | "save" | "">;
  toggleMetadataPane: () => void;
  undoSourceChange: () => void;
  redoSourceChange: () => void;
  setStrategyDisplayMode: (mode: StrategyDisplayMode) => void;
  setStrategyMobileSection: (section: StrategyMobileSection) => void;
  createNewWorkflow: () => void;
  analyzeCurrentScript: () => Promise<boolean>;
  saveDefinition: () => Promise<unknown>;
}

const {
  definitionName, definitionVersion, totalErrorCount, totalDiagnosticCount,
  metadataPaneOpen, canUndoSourceChange, canRedoSourceChange, strategyDisplayMode,
  strategyMobileSection, isAnalyzing, isSavingDefinition, actionFeedback,
  toggleMetadataPane, undoSourceChange, redoSourceChange, setStrategyDisplayMode,
  setStrategyMobileSection, createNewWorkflow, analyzeCurrentScript, saveDefinition,
} = useStrategyDesignContext<HeaderContext>();
</script>

<template>
  <header class="strategy-native-header">
    <div class="strategy-native-header__identity">
      <button
        type="button"
        class="strategy-native-metadata-toggle"
        :class="{ 'is-active': metadataPaneOpen }"
        :aria-expanded="metadataPaneOpen"
        aria-controls="strategy-design-metadata-pane"
        data-testid="strategy-metadata-toggle"
        title="显示或隐藏策略信息"
        @click="toggleMetadataPane"
      >
        <v-icon size="14">fa-solid fa-table-columns</v-icon>
        <span>策略信息</span>
      </button>
      <div class="strategy-native-title-block">
        <h1>策略快捷指令工作台</h1>
        <span class="strategy-native-active-definition" :title="definitionName || '新建草稿'">
          {{ definitionName || "新建草稿" }}
        </span>
      </div>
      <span class="strategy-native-chip">v{{ definitionVersion || "0.1.0" }}</span>
      <span
        class="strategy-native-chip strategy-native-chip--diagnostic"
        :class="{ 'has-error': totalErrorCount > 0 }"
        :title="`诊断 ${totalDiagnosticCount} 项，其中错误 ${totalErrorCount} 项`"
      >
        <v-icon size="11">fa-solid fa-stethoscope</v-icon>
        {{ totalDiagnosticCount }}
      </span>
    </div>
    <div class="strategy-native-header__actions">
      <div class="strategy-native-history-actions" aria-label="源码历史">
        <button
          type="button"
          class="strategy-native-history-button"
          :disabled="!canUndoSourceChange"
          data-testid="strategy-source-undo"
          title="撤回"
          aria-label="撤回"
          @click="undoSourceChange"
        >
          <v-icon size="13">fa-solid fa-arrow-rotate-left</v-icon>
        </button>
        <button
          type="button"
          class="strategy-native-history-button"
          :disabled="!canRedoSourceChange"
          data-testid="strategy-source-redo"
          title="重做"
          aria-label="重做"
          @click="redoSourceChange"
        >
          <v-icon size="13">fa-solid fa-arrow-rotate-right</v-icon>
        </button>
      </div>
      <div class="strategy-native-view-switch" aria-label="策略工作区视图">
        <button
          v-for="mode in (['instruction', 'split', 'code'] as const)"
          :key="mode"
          class="strategy-native-view-switch__button"
          :class="{ 'is-active': strategyDisplayMode === mode }"
          :data-testid="`strategy-display-mode-${mode}`"
          type="button"
          @click="setStrategyDisplayMode(mode)"
        >
          {{ mode === "instruction" ? "指令" : mode === "split" ? "双栏" : "代码" }}
        </button>
      </div>
      <button type="button" class="strategy-native-action-button" @click="createNewWorkflow">
        新建 Pine v6
      </button>
      <button
        type="button"
        class="strategy-native-action-button"
        :disabled="isAnalyzing"
        @click="void analyzeCurrentScript()"
      >
        {{ isAnalyzing ? "分析中" : actionFeedback === "analyze" ? "已分析" : "分析" }}
      </button>
      <button
        type="button"
        class="strategy-native-action-button strategy-native-action-button--primary"
        :disabled="isSavingDefinition"
        @click="void saveDefinition()"
      >
        {{ isSavingDefinition ? "保存中" : actionFeedback === "save" ? "已保存" : "保存" }}
      </button>
    </div>
  </header>

  <nav class="strategy-native-mobile-switch" aria-label="策略移动端工作区">
    <button
      v-for="section in (['definition', 'instruction', 'code'] as const)"
      :key="section"
      type="button"
      class="strategy-native-mobile-switch__button"
      :class="{ 'is-active': strategyMobileSection === section }"
      :data-testid="`strategy-mobile-section-${section}`"
      @click="setStrategyMobileSection(section)"
    >
      {{ section === "definition" ? "策略定义" : section === "instruction" ? "结构指令" : "Pine 代码" }}
    </button>
  </nav>
</template>

<style scoped>
.strategy-native-header {
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

.strategy-native-header__identity,
.strategy-native-title-block {
  display: flex;
  min-width: 0;
  align-items: center;
}

.strategy-native-header__identity {
  flex: 1 1 auto;
  gap: 6px;
  overflow: hidden;
}

.strategy-native-title-block {
  flex: 0 1 auto;
  gap: 7px;
  overflow: hidden;
}

.strategy-native-header h1 {
  flex: 0 0 auto;
  margin: 0;
  font-size: 0.92rem;
  line-height: 1;
  white-space: nowrap;
}

.strategy-native-active-definition {
  min-width: 0;
  max-width: 15rem;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.8rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-native-chip {
  display: inline-flex;
  min-height: 20px;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  padding: 1px 6px;
  color: var(--tv-text-muted);
  font-size: 0.7rem;
  font-weight: 700;
  line-height: 1;
  white-space: nowrap;
}

.strategy-native-chip--diagnostic.has-error {
  border-color: color-mix(in srgb, var(--jf-accent-red) 52%, var(--tv-border));
  background: color-mix(in srgb, var(--jf-accent-red) 10%, transparent);
  color: color-mix(in srgb, var(--jf-accent-red-text) 76%, var(--tv-text));
}

.strategy-native-metadata-toggle,
.strategy-native-action-button,
.strategy-native-history-button {
  min-height: 30px;
  height: 30px;
  border-radius: 5px;
}

.strategy-native-metadata-toggle {
  display: inline-flex;
  min-width: 30px;
  align-items: center;
  justify-content: center;
  gap: 5px;
  border-color: transparent;
  background: transparent;
  padding: 0 6px;
  color: var(--tv-text-muted);
  font-size: 0.77rem;
}

.strategy-native-metadata-toggle:is(:hover, .is-active) {
  border-color: var(--tv-border);
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.strategy-native-header__actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: nowrap;
  gap: 4px;
}

.strategy-native-history-actions {
  display: inline-flex;
  align-items: center;
}

.strategy-native-history-button {
  display: inline-grid;
  width: 28px;
  place-items: center;
  border-color: transparent;
  background: transparent;
  color: var(--tv-text-muted);
  padding: 0;
}

.strategy-native-history-button:hover:not(:disabled) {
  color: var(--tv-text);
}

.strategy-native-view-switch {
  display: inline-grid;
  grid-template-columns: repeat(3, minmax(2.55rem, 1fr));
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 72%, transparent);
  padding: 2px;
}

.strategy-native-view-switch__button {
  display: inline-grid;
  min-width: 2.55rem;
  min-height: 26px;
  place-items: center;
  border: 0;
  border-radius: 4px;
  background: transparent;
  padding: 0 7px;
  color: var(--tv-text-muted);
  font-size: 0.77rem;
  font-weight: 800;
  line-height: 1;
  white-space: nowrap;
}

.strategy-native-view-switch__button.is-active {
  background: color-mix(in srgb, var(--tv-accent) 22%, var(--tv-bg-surface));
  color: var(--tv-text);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tv-accent) 36%, transparent);
}

.strategy-native-mobile-switch {
  display: none;
}

button {
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font-weight: 700;
}

button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.strategy-native-action-button {
  padding: 0 9px;
  font-size: 0.8rem;
}

.strategy-native-action-button--primary {
  border-color: color-mix(in srgb, var(--tv-accent) 55%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 14%, var(--tv-bg-surface));
  color: var(--tv-accent);
}

button:focus-visible {
  outline: 2px solid var(--tv-accent);
  outline-offset: -2px;
}

@media (max-width: 920px) and (min-width: 769px) {
  .strategy-native-metadata-toggle span,
  .strategy-native-active-definition,
  .strategy-native-chip--diagnostic {
    display: none;
  }

  .strategy-native-title-block h1 {
    font-size: 0.86rem;
  }
}

@media (max-width: 768px) {
  .strategy-native-header {
    min-height: 44px;
    height: auto;
    flex: 0 0 auto;
    flex-flow: row wrap;
    gap: 4px 8px;
    padding: 5px 6px;
  }

  .strategy-native-header__identity {
    flex-basis: 100%;
  }

  .strategy-native-metadata-toggle,
  .strategy-native-active-definition {
    display: none;
  }

  .strategy-native-title-block {
    flex: 1 1 auto;
  }

  .strategy-native-header__actions {
    width: 100%;
    justify-content: flex-start;
  }

  .strategy-native-history-button,
  .strategy-native-action-button {
    min-height: 40px;
    height: 40px;
  }

  .strategy-native-history-button {
    width: 36px;
  }

  .strategy-native-view-switch {
    display: none;
  }

  .strategy-native-mobile-switch {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    min-height: 40px;
    flex: 0 0 40px;
    gap: 1px;
    border-width: 0 0 1px;
    border-radius: 0;
    border-color: var(--tv-border);
    background: var(--tv-bg-surface);
    padding: 3px 6px;
  }

  .strategy-native-mobile-switch__button {
    min-width: 0;
    min-height: 34px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: var(--tv-text-muted);
    padding: 0 6px;
    font-size: 0.77rem;
    font-weight: 800;
    white-space: nowrap;
  }

  .strategy-native-mobile-switch__button.is-active {
    background: color-mix(in srgb, var(--tv-accent) 18%, var(--tv-bg-surface));
    color: var(--tv-text);
  }
}
</style>
