<script setup lang="ts">
import type { Ref } from "vue";

import type { PineV6WorkflowDocument, StrategyDefinitionDocument, StrategyInstanceItem } from "@/types";
import InstrumentIdentity from "../domain/market-data/InstrumentIdentity.vue";
import {
  formatStrategyInterval,
  formatStrategyRuntimeRiskSummary,
  formatStrategySymbols,
  readStrategyBinding,
} from "../strategy-runtime/strategyRuntimeInstanceBinding";
import StrategyDesignHistoryPanel from "./StrategyDesignHistoryPanel.vue";
import { useStrategyDesignContext } from "./strategyDesignContext";
import type { StrategySidePanelId } from "./useStrategySidePanelLayout";

interface Diagnostic {
  severity: "error" | "warning" | "info";
  code?: string;
  message: string;
  blockId?: string;
  line?: number;
  column?: number;
}

interface MetadataContext {
  definitionName: Ref<string>;
  definitionVersion: Ref<string>;
  definitionDescription: Ref<string>;
  selectedDefinitionId: Ref<string>;
  strategyDefinitions: Ref<StrategyDefinitionDocument[]>;
  selectedDefinition: Readonly<Ref<StrategyDefinitionDocument | null>>;
  isLoadingDefinitions: Ref<boolean>;
  workflow: Ref<PineV6WorkflowDocument>;
  workflowDiagnostics: Readonly<Ref<Diagnostic[]>>;
  analyzerDiagnostics: Readonly<Ref<Diagnostic[]>>;
  workflowErrorCount: Readonly<Ref<number>>;
  analyzerErrorCount: Readonly<Ref<number>>;
  totalDiagnosticCount: Readonly<Ref<number>>;
  totalErrorCount: Readonly<Ref<number>>;
  readonlyStrategies: Readonly<Ref<StrategyInstanceItem[]>>;
  isLoadingStrategies: Ref<boolean>;
  expandedStrategySidePanels: Ref<string[]>;
  expandedStrategySidePanelCount: Readonly<Ref<number>>;
  isWideWorkbench: Ref<boolean>;
  closeMetadataPane: () => void;
  finishStrategySidePanelDrag: () => void;
  strategySidePanelClasses: (id: StrategySidePanelId) => Record<string, boolean>;
  strategySidePanelPosition: (id: StrategySidePanelId) => number;
  handleStrategySidePanelDragOver: (event: DragEvent, id: StrategySidePanelId) => void;
  handleStrategySidePanelDragStart: (event: DragEvent, id: StrategySidePanelId) => void;
  handleStrategySidePanelDrop: (event: DragEvent) => void;
  applyDefinition: (definition: StrategyDefinitionDocument) => void;
  createNewWorkflow: () => void;
  updateDeclaration: (
    key: keyof PineV6WorkflowDocument["declaration"],
    value: PineV6WorkflowDocument["declaration"][keyof PineV6WorkflowDocument["declaration"]],
  ) => void;
  diagnosticClass: (diagnostic: Diagnostic) => string;
  loadStrategies: () => Promise<void>;
  statusLabel: (status: string) => string;
  statusClass: (status: string) => string;
  strategyInstrumentIds: (strategy: StrategyInstanceItem) => string[];
}

const context = useStrategyDesignContext<MetadataContext>();
</script>

<template>
  <aside id="strategy-design-metadata-pane" class="strategy-native-side">
    <div class="strategy-native-drawer-head">
      <div>
        <strong>策略信息</strong>
        <span>{{ context.definitionName.value || "新建草稿" }}</span>
      </div>
      <button type="button" aria-label="关闭策略信息" @click="context.closeMetadataPane">
        <v-icon size="14">fa-solid fa-xmark</v-icon>
      </button>
    </div>
    <v-expansion-panels
      v-model="context.expandedStrategySidePanels.value"
      multiple
      class="strategy-native-side-panels"
      :class="{
        'is-reorderable': context.isWideWorkbench.value,
        'is-space-constrained': context.expandedStrategySidePanelCount.value >= 3,
      }"
      variant="default"
      @dragend="context.finishStrategySidePanelDrag"
    >
      <v-expansion-panel
        value="definition"
        class="strategy-native-panel strategy-native-side-panel"
        :class="context.strategySidePanelClasses('definition')"
        :style="{ order: context.strategySidePanelPosition('definition') }"
        data-testid="strategy-side-panel-definition"
      >
        <v-expansion-panel-title
          collapse-icon="fa-solid fa-chevron-right"
          data-testid="strategy-side-panel-definition-title"
          :draggable="context.isWideWorkbench.value"
          expand-icon="fa-solid fa-chevron-right"
          title="拖动调整位置"
          @dragover.prevent="context.handleStrategySidePanelDragOver($event, 'definition')"
          @dragstart="context.handleStrategySidePanelDragStart($event, 'definition')"
          @drop.prevent="context.handleStrategySidePanelDrop"
        >
          <div class="strategy-native-side-panel__heading">
            <div class="strategy-native-panel__title">策略定义</div>
          </div>
        </v-expansion-panel-title>
        <v-expansion-panel-text>
          <div class="strategy-native-panel__content">
            <select
              v-model="context.selectedDefinitionId.value"
              :disabled="context.isLoadingDefinitions.value"
              @change="context.selectedDefinition.value
                ? context.applyDefinition(context.selectedDefinition.value)
                : context.createNewWorkflow()"
            >
              <option value="">新建草稿</option>
              <option
                v-for="definition in context.strategyDefinitions.value"
                :key="definition.id"
                :value="definition.id"
              >
                {{ definition.name }} / v{{ definition.version }}
              </option>
            </select>
            <label><span>名称</span><input v-model="context.definitionName.value"></label>
            <label>
              <span>版本（保存后自动生成）</span>
              <input v-model="context.definitionVersion.value" readonly aria-readonly="true">
            </label>
            <label>
              <span>说明</span>
              <textarea v-model="context.definitionDescription.value" rows="3" />
            </label>
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>

      <StrategyDesignHistoryPanel />

      <v-expansion-panel
        value="declaration"
        class="strategy-native-panel strategy-native-side-panel"
        :class="context.strategySidePanelClasses('declaration')"
        :style="{ order: context.strategySidePanelPosition('declaration') }"
        data-testid="strategy-side-panel-declaration"
      >
        <v-expansion-panel-title
          collapse-icon="fa-solid fa-chevron-right"
          data-testid="strategy-side-panel-declaration-title"
          :draggable="context.isWideWorkbench.value"
          expand-icon="fa-solid fa-chevron-right"
          title="拖动调整位置"
          @dragover.prevent="context.handleStrategySidePanelDragOver($event, 'declaration')"
          @dragstart="context.handleStrategySidePanelDragStart($event, 'declaration')"
          @drop.prevent="context.handleStrategySidePanelDrop"
        >
          <div class="strategy-native-side-panel__heading">
            <div class="strategy-native-panel__title">策略声明</div>
          </div>
        </v-expansion-panel-title>
        <v-expansion-panel-text>
          <div class="strategy-native-panel__content">
            <label>
              <span>标题</span>
              <input
                data-testid="strategy-declaration-title"
                :value="context.workflow.value.declaration.title"
                @input="context.updateDeclaration('title', ($event.target as HTMLInputElement).value)"
              >
            </label>
            <label class="strategy-native-toggle">
              <input
                :checked="context.workflow.value.declaration.overlay"
                type="checkbox"
                @change="context.updateDeclaration('overlay', ($event.target as HTMLInputElement).checked)"
              >
              <span>叠加到主图</span>
            </label>
            <label>
              <span>初始资金</span>
              <input
                :value="context.workflow.value.declaration.initialCapital ?? ''"
                type="number"
                @input="context.updateDeclaration('initialCapital', Number(($event.target as HTMLInputElement).value) || null)"
              >
            </label>
            <label>
              <span>币种</span>
              <input
                :value="context.workflow.value.declaration.currency ?? ''"
                @input="context.updateDeclaration('currency', ($event.target as HTMLInputElement).value)"
              >
            </label>
            <label>
              <span>允许加仓次数</span>
              <input
                :value="context.workflow.value.declaration.pyramiding ?? 0"
                type="number"
                @input="context.updateDeclaration('pyramiding', Number(($event.target as HTMLInputElement).value) || 0)"
              >
            </label>
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>

      <v-expansion-panel
        value="diagnostics"
        class="strategy-native-panel strategy-native-side-panel"
        :class="context.strategySidePanelClasses('diagnostics')"
        :style="{ order: context.strategySidePanelPosition('diagnostics') }"
        data-testid="strategy-side-panel-diagnostics"
      >
        <v-expansion-panel-title
          collapse-icon="fa-solid fa-chevron-right"
          data-testid="strategy-side-panel-diagnostics-title"
          :draggable="context.isWideWorkbench.value"
          expand-icon="fa-solid fa-chevron-right"
          title="拖动调整位置"
          @dragover.prevent="context.handleStrategySidePanelDragOver($event, 'diagnostics')"
          @dragstart="context.handleStrategySidePanelDragStart($event, 'diagnostics')"
          @drop.prevent="context.handleStrategySidePanelDrop"
        >
          <div class="strategy-native-side-panel__heading">
            <div class="strategy-native-panel__title">诊断</div>
            <span class="strategy-native-panel-count" :class="{ 'has-error': context.totalErrorCount.value > 0 }">
              {{ context.totalDiagnosticCount.value }}
            </span>
          </div>
        </v-expansion-panel-title>
        <v-expansion-panel-text>
          <div class="strategy-native-panel__content">
            <div
              v-if="context.workflowDiagnostics.value.length === 0 && context.analyzerDiagnostics.value.length === 0"
              class="strategy-native-meta"
            >
              暂无诊断。
            </div>
            <div
              v-for="diagnostic in context.workflowDiagnostics.value"
              :key="`${diagnostic.code}-${diagnostic.blockId ?? ''}`"
              class="strategy-native-diagnostic"
              :class="context.diagnosticClass(diagnostic)"
            >
              <strong>{{ diagnostic.code }}</strong><span>{{ diagnostic.message }}</span>
            </div>
            <div
              v-for="diagnostic in context.analyzerDiagnostics.value"
              :key="`${diagnostic.line}-${diagnostic.column}-${diagnostic.message}`"
              class="strategy-native-diagnostic"
              :class="context.diagnosticClass(diagnostic)"
            >
              <strong>{{ diagnostic.code ?? diagnostic.severity }}</strong>
              <span>第 {{ diagnostic.line }} 行：{{ diagnostic.message }}</span>
            </div>
            <div class="strategy-native-meta">
              工作流错误 {{ context.workflowErrorCount.value }} 个 /
              Pine 分析错误 {{ context.analyzerErrorCount.value }} 个
            </div>
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>

      <v-expansion-panel
        value="instances"
        class="strategy-native-panel strategy-native-side-panel"
        :class="context.strategySidePanelClasses('instances')"
        :style="{ order: context.strategySidePanelPosition('instances') }"
        data-testid="strategy-side-panel-instances"
      >
        <v-expansion-panel-title
          collapse-icon="fa-solid fa-chevron-right"
          data-testid="strategy-side-panel-instances-title"
          :draggable="context.isWideWorkbench.value"
          expand-icon="fa-solid fa-chevron-right"
          title="拖动调整位置"
          @dragover.prevent="context.handleStrategySidePanelDragOver($event, 'instances')"
          @dragstart="context.handleStrategySidePanelDragStart($event, 'instances')"
          @drop.prevent="context.handleStrategySidePanelDrop"
        >
          <div class="strategy-native-side-panel__heading">
            <div class="strategy-native-panel__title">策略实例</div>
            <span class="strategy-native-panel-count">{{ context.readonlyStrategies.value.length }}</span>
          </div>
        </v-expansion-panel-title>
        <v-expansion-panel-text>
          <div class="strategy-native-panel__content">
            <div class="strategy-native-panel-tools">
              <span>关联实例</span>
              <button
                type="button"
                class="strategy-native-icon-button"
                :disabled="context.isLoadingStrategies.value"
                title="刷新"
                aria-label="刷新策略实例"
                @click.stop="void context.loadStrategies()"
              >
                <v-icon size="13">fa-solid fa-arrow-rotate-right</v-icon>
              </button>
            </div>
            <div v-if="context.readonlyStrategies.value.length === 0" class="strategy-native-meta">
              暂无实例。
            </div>
            <section
              v-for="strategy in context.readonlyStrategies.value"
              :key="strategy.id"
              class="strategy-native-instance"
            >
              <div>
                <strong>{{ strategy.definition.name }}</strong>
                <span :class="['strategy-native-status', context.statusClass(strategy.status)]">
                  {{ context.statusLabel(strategy.status) }}
                </span>
              </div>
              <div
                class="flex flex-wrap items-center gap-1.5"
                :data-testid="`strategy-design-instance-symbols-${strategy.id}`"
              >
                <template v-if="context.strategyInstrumentIds(strategy).length > 0">
                  <InstrumentIdentity
                    v-for="symbol in context.strategyInstrumentIds(strategy)"
                    :key="symbol"
                    :instrument-id="symbol"
                    compact
                  />
                </template>
                <span v-else>{{ formatStrategySymbols(strategy) }}</span>
                <span>/ {{ formatStrategyInterval(strategy) }}</span>
              </div>
              <div>{{ formatStrategyRuntimeRiskSummary(readStrategyBinding(strategy).runtimeRisk) }}</div>
            </section>
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>
    </v-expansion-panels>
  </aside>
</template>

<style scoped>
.strategy-native-side {
  display: flex;
  min-width: 0;
  min-height: 0;
  height: 100%;
  flex-direction: column;
  overflow: hidden;
  border-right: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.strategy-native-drawer-head {
  display: none;
  min-height: 44px;
  flex: 0 0 44px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  border-bottom: 1px solid var(--tv-border);
  padding: 0 8px;
}

.strategy-native-drawer-head > div {
  display: grid;
  min-width: 0;
  gap: 1px;
}

.strategy-native-drawer-head strong {
  font-size: 0.82rem;
}

.strategy-native-drawer-head span {
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.7rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-native-drawer-head button {
  display: inline-grid;
  width: 32px;
  min-height: 32px;
  place-items: center;
  padding: 0;
}

.strategy-native-side-panels {
  display: flex !important;
  height: 100%;
  width: 100% !important;
  min-width: 0;
  min-height: 0;
  flex: 1 1 auto;
  flex-direction: column;
  flex-wrap: nowrap !important;
  justify-content: flex-start !important;
  gap: 0;
  overflow-x: hidden;
  overflow-y: auto;
  background: transparent;
}

.strategy-native-side-panels :deep(.v-expansion-panel-title__overlay),
.strategy-native-side-panels :deep(.v-expansion-panel__overlay) {
  display: none;
}

.strategy-native-side-panels :deep(.v-expansion-panel::after) {
  border: 0 !important;
  content: none !important;
}

.strategy-native-side-panels :deep(.v-expansion-panel-title) {
  width: 100% !important;
  min-height: 34px;
  flex: 0 0 34px;
  margin-inline: 0;
  border: 0 !important;
  border-radius: 0 !important;
  padding: 0 8px;
  background: var(--tv-bg-surface);
}

.strategy-native-side-panels :deep(.v-expansion-panel-title:hover) {
  background: var(--tv-bg-elevated);
}

.strategy-native-side-panels :deep(.v-expansion-panel) {
  width: 100% !important;
  min-width: 0;
  flex: 0 0 auto !important;
  margin: 0 !important;
  border-radius: 0 !important;
}

.strategy-native-side-panels :deep(.v-expansion-panel--active) {
  display: flex;
  min-height: 34px;
  flex: 0 1 auto !important;
  flex-direction: column;
}

.strategy-native-side-panels :deep(.v-expansion-panel--active.is-fill-panel) {
  flex: 1 1 0 !important;
}

.strategy-native-side-panels.is-space-constrained :deep(.v-expansion-panel--active) {
  min-height: 96px;
  flex: 1 1 0 !important;
}

.strategy-native-side-panels :deep(.v-expansion-panel-text) {
  min-height: 0;
  flex: 1 1 auto;
  overflow: hidden;
}

.strategy-native-side-panels :deep(.v-expansion-panel-text__wrapper) {
  width: 100%;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 0;
  scrollbar-gutter: auto;
}

.strategy-native-side-panels :deep(.v-expansion-panel-title__icon) {
  order: -1;
  margin-inline: 0 7px;
  font-size: 0.72rem;
  transform: rotate(0deg) !important;
}

.strategy-native-side-panels :deep(.v-expansion-panel-title--active > .v-expansion-panel-title__icon) {
  transform: rotate(90deg) !important;
}

.strategy-native-side-panels.is-reorderable :deep(.v-expansion-panel-title) {
  cursor: grab;
}

.strategy-native-side-panels.is-reorderable :deep(.v-expansion-panel-title:active) {
  cursor: grabbing;
}

.strategy-native-side-panels :deep(.strategy-native-side-panel) {
  position: relative;
  display: block;
  width: 100% !important;
  min-width: 0;
  overflow: hidden;
  border: 0;
  border-radius: 0;
  background: var(--tv-bg-surface);
}

.strategy-native-side-panels :deep(.strategy-native-side-panel:not(.is-first-panel) .v-expansion-panel-title) {
  box-shadow: inset 0 1px 0 var(--tv-border);
}

.strategy-native-side-panels :deep(.strategy-native-side-panel.is-dragging) {
  opacity: 0.48;
}

.strategy-native-side-panels :deep(.strategy-native-side-panel:is(.is-drop-before, .is-drop-after)::before) {
  position: absolute;
  z-index: 3;
  right: 0;
  left: 0;
  height: 1px;
  background: var(--tv-accent);
  content: "";
  pointer-events: none;
}

.strategy-native-side-panels :deep(.strategy-native-side-panel.is-drop-before::before) {
  top: 0;
}

.strategy-native-side-panels :deep(.strategy-native-side-panel.is-drop-after::before) {
  bottom: 0;
}

.strategy-native-side-panels :deep(.strategy-native-side-panel__heading) {
  display: flex;
  min-width: 0;
  flex: 1 1 auto;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.strategy-native-side-panels :deep(.strategy-native-panel__title) {
  color: var(--tv-text-muted);
  font-size: 0.78rem;
  font-weight: 750;
  letter-spacing: 0.04em;
}

.strategy-native-side-panels :deep(.strategy-native-panel-count) {
  display: inline-flex;
  min-height: 18px;
  align-items: center;
  border: 1px solid transparent;
  border-radius: 999px;
  background: var(--tv-bg-elevated);
  padding-inline: 5px;
  color: var(--tv-text-muted);
  font-size: 0.68rem;
  font-weight: 700;
}

.strategy-native-side-panels :deep(.strategy-native-panel-count.has-error) {
  border-color: color-mix(in srgb, var(--jf-accent-red) 52%, var(--tv-border));
  color: color-mix(in srgb, var(--jf-accent-red-text) 76%, var(--tv-text));
}

.strategy-native-side-panels :deep(.strategy-native-icon-button) {
  display: inline-grid;
  width: 28px;
  min-height: 30px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 0;
}

.strategy-native-side-panels :deep(.strategy-native-panel-tools) {
  display: flex;
  min-width: 0;
  min-height: 30px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  color: var(--tv-text-muted);
  font-size: 0.71rem;
  font-weight: 700;
}

.strategy-native-side-panels :deep(.strategy-native-panel__content) {
  display: grid;
  gap: 8px;
  padding: 8px;
  background: color-mix(in srgb, var(--tv-bg-app) 35%, var(--tv-bg-surface));
}

.strategy-native-side-panels :deep(.strategy-native-panel label) {
  display: grid;
  gap: 3px;
  color: var(--tv-text-muted);
  font-size: 0.7rem;
  letter-spacing: 0.06em;
  font-weight: 700;
  text-transform: uppercase;
}

.strategy-native-side-panels :deep(.strategy-native-panel input),
.strategy-native-side-panels :deep(.strategy-native-panel select),
.strategy-native-side-panels :deep(.strategy-native-panel textarea) {
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

.strategy-native-side-panels :deep(.strategy-native-panel textarea) {
  min-height: 56px;
  resize: vertical;
}

.strategy-native-side-panels :deep(.strategy-native-toggle) {
  display: inline-flex !important;
  min-height: 28px;
  grid-template-columns: auto 1fr;
  align-items: center;
  gap: 5px;
  letter-spacing: 0 !important;
  text-transform: none !important;
}

.strategy-native-side-panels :deep(.strategy-native-toggle input) {
  width: auto;
}

.strategy-native-side-panels :deep(.strategy-native-diagnostic) {
  display: grid;
  gap: 2px;
  border: 1px solid var(--tv-border);
  padding: 5px 6px;
  border-radius: 5px;
  font-size: 0.77rem;
  overflow-wrap: anywhere;
}

.strategy-native-side-panels :deep(.strategy-native-diagnostic--error) {
  border-color: color-mix(in srgb, var(--jf-accent-red) 48%, var(--tv-border));
  background: color-mix(in srgb, var(--jf-accent-red) 12%, var(--tv-bg-surface));
  color: color-mix(in srgb, var(--jf-accent-red-text) 72%, var(--tv-text));
}

.strategy-native-side-panels :deep(.strategy-native-diagnostic--warning) {
  border-color: color-mix(in srgb, var(--jf-accent-amber) 48%, var(--tv-border));
  background: color-mix(in srgb, var(--jf-accent-amber) 12%, var(--tv-bg-surface));
  color: color-mix(in srgb, var(--jf-accent-amber-text) 70%, var(--tv-text));
}

.strategy-native-side-panels :deep(.strategy-native-diagnostic--info) {
  border-color: color-mix(in srgb, var(--tv-accent) 44%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-accent) 10%, var(--tv-bg-surface));
  color: color-mix(in srgb, var(--tv-accent) 74%, var(--tv-text));
}

.strategy-native-side-panels :deep(.strategy-native-instance) {
  display: grid;
  width: 100%;
  gap: 3px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--tv-border);
  font-size: 0.77rem;
  text-align: left;
}

.strategy-native-side-panels :deep(.strategy-native-instance > div:first-child) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 5px;
}

.strategy-native-side-panels :deep(.strategy-native-status) {
  border-radius: 999px;
  padding: 2px 5px;
  font-size: 0.68rem;
}

.strategy-native-side-panels :deep(.strategy-native-status--running) {
  background: color-mix(in srgb, var(--jf-accent-green) 18%, var(--tv-bg-surface));
  color: color-mix(in srgb, #86efac 72%, var(--tv-text));
}

.strategy-native-side-panels :deep(.strategy-native-status--paused) {
  background: color-mix(in srgb, var(--jf-accent-amber) 18%, var(--tv-bg-surface));
  color: color-mix(in srgb, var(--jf-accent-amber-text) 72%, var(--tv-text));
}

.strategy-native-side-panels :deep(.strategy-native-status--stopped) {
  background: color-mix(in srgb, var(--tv-text-muted) 18%, var(--tv-bg-surface));
  color: var(--tv-text-muted);
}

.strategy-native-side-panels :deep(.strategy-native-meta) {
  color: var(--tv-text-muted);
  font-size: 0.77rem;
  line-height: 1.35;
}

@media (min-width: 769px) and (max-width: 1180px) {
  .strategy-native-drawer-head {
    display: flex;
  }
}

@media (max-width: 768px) {
  .strategy-native-side-panels :deep(.strategy-native-panel input),
  .strategy-native-side-panels :deep(.strategy-native-panel select),
  .strategy-native-side-panels :deep(.strategy-native-panel textarea),
  .strategy-native-side-panels :deep(.strategy-native-panel-tools),
  .strategy-native-side-panels :deep(.strategy-native-icon-button) {
    min-height: 40px;
  }

  .strategy-native-side-panels :deep(.strategy-native-icon-button) {
    width: 40px;
    height: 40px;
  }
}
</style>
