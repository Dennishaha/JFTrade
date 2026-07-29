<script setup lang="ts">
import { ref } from "vue";

import {
  strategyPineEditorCompletions,
  strategyPineEditorExtraLibs,
  strategyPineEditorHoverItems,
} from "@/features/strategyPineEditorIntelliSense";
import MonacoCodeEditor from "@/components/shared/MonacoCodeEditor.vue";

interface DiagnosticMarker {
  severity: "error" | "warning" | "info";
  message: string;
  line: number;
  column: number;
  endLine: number;
  endColumn: number;
}

interface MonacoOffsetRange {
  start: number;
  end: number;
}

defineProps<{
  modelValue: string;
  sourceEditingEnabled: boolean;
  diagnosticMarkers: DiagnosticMarker[];
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
  "update:sourceEditingEnabled": [value: boolean];
}>();

const advancedSourceOpen = ref(false);
const sourceEditorRef = ref<InstanceType<typeof MonacoCodeEditor> | null>(null);

function revealOffsetRange(range: MonacoOffsetRange): void {
  sourceEditorRef.value?.revealOffsetRange(range);
}

defineExpose({
  revealOffsetRange,
});
</script>

<template>
  <aside class="strategy-native-code-pane">
    <section class="strategy-native-panel strategy-native-code">
      <div class="strategy-native-workspace-bar">
        <div class="strategy-native-code__identity">
          <div class="strategy-native-panel__title">Pine v6 源码</div>
          <span class="strategy-native-code__authority" title="运行时以源码为准；结构块由当前源码解析生成。">
            运行时以源码为准
          </span>
        </div>
        <div class="strategy-native-code__actions">
          <label class="strategy-native-toggle">
            <input
              :checked="sourceEditingEnabled"
              data-testid="strategy-source-override-toggle"
              type="checkbox"
              @change="emit('update:sourceEditingEnabled', ($event.target as HTMLInputElement).checked)"
            >
            <span>源码编辑</span>
          </label>
          <button
            type="button"
            class="strategy-native-code__help-button"
            :aria-expanded="advancedSourceOpen"
            title="Pine v6 支持边界"
            aria-label="Pine v6 支持边界"
            @click="advancedSourceOpen = !advancedSourceOpen"
          >
            <v-icon size="13">fa-regular fa-circle-question</v-icon>
          </button>
        </div>
        <div v-if="advancedSourceOpen" class="strategy-native-code__help strategy-native-meta">
          <strong>Pine v6 支持边界</strong>
          <span>
            当前按闭合 K 线执行；订单按下一根 K 线成交；OCA、部分成交、tick 级重算是明确边界。开启源码编辑后，
            保存的 script 以源码为准，visualModel 保留当前指令快照。
          </span>
        </div>
      </div>
      <MonacoCodeEditor
        ref="sourceEditorRef"
        :model-value="modelValue"
        language="pine-v6"
        height="100%"
        min-height="0"
        :font-size="12"
        test-id="strategy-script-editor"
        :read-only="!sourceEditingEnabled"
        :extra-libs="strategyPineEditorExtraLibs"
        :completion-items="strategyPineEditorCompletions"
        :hover-items="strategyPineEditorHoverItems"
        :diagnostic-markers="diagnosticMarkers"
        @update:model-value="emit('update:modelValue', $event)"
      />
    </section>
  </aside>
</template>

<style scoped>
.strategy-native-code-pane {
  width: 100%;
  min-width: 0;
  min-height: 0;
  height: 100%;
  overflow: hidden;
  display: grid;
  align-content: stretch;
  grid-template-rows: minmax(0, 1fr);
  gap: 0;
  padding: 0;
  background: var(--tv-bg-app);
}

.strategy-native-panel {
  display: grid;

  border: 1px solid var(--tv-border);
  border-radius: 0.5rem;
  background: color-mix(in srgb, var(--tv-bg-surface) 96%, transparent);
  padding: 0.85rem;
}

.strategy-native-code {
  min-height: 0;
  height: 100%;
  grid-template-rows: 36px minmax(0, 1fr);
  border: 0;
  border-radius: 0;
  background: transparent;
  padding: 0;
}

.strategy-native-workspace-bar {
  position: relative;
  min-width: 0;
  display: flex;
  align-items: center;
  min-height: 36px;
  flex-wrap: nowrap;
  gap: 0.4rem;
  justify-content: space-between;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.strategy-native-code__identity,
.strategy-native-code__actions {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
}

.strategy-native-code__identity {
  overflow: hidden;
}

.strategy-native-code__actions {
  flex: 0 0 auto;
}

.strategy-native-panel__title {
  color: var(--tv-text-muted);
  font-size: 0.72rem;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.strategy-native-meta {
  color: var(--tv-text-muted);
  font-size: 0.75rem;
  line-height: 1.4;
}

.strategy-native-code__authority {
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.73rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-native-toggle {
  display: inline-flex !important;
  grid-template-columns: auto 1fr;
  align-items: center;
  min-height: 28px;
  gap: 0.35rem;
  color: var(--tv-text-muted);
  font-size: 0.73rem;
  font-weight: 700;
  letter-spacing: 0 !important;
  text-transform: none !important;
}

.strategy-native-toggle input {
  width: auto;
}

button {
  border: 1px solid var(--tv-border);
  border-radius: 0.45rem;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 0.48rem 0.75rem;
  font-size: 0.85rem;
  font-weight: 700;
}

.strategy-native-code__help-button {
  display: inline-grid;
  width: 28px;
  height: 28px;
  place-items: center;
  border: 0;
  border-radius: 5px;
  background: transparent;
  padding: 0;
  color: var(--tv-text-muted);
}

.strategy-native-code__help-button:is(:hover, [aria-expanded="true"]) {
  background: var(--tv-bg-elevated);
  color: var(--tv-text);
}

.strategy-native-code__help-button:focus-visible {
  outline: 2px solid var(--tv-accent);
  outline-offset: -2px;
}

.strategy-native-code__help {
  position: absolute;
  z-index: 10;
  top: calc(100% + 5px);
  right: 6px;
  display: grid;
  width: min(22rem, calc(100vw - 32px));
  gap: 5px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-elevated);
  padding: 8px;
  color: var(--tv-text-muted);
  box-shadow: 0 12px 32px rgba(2, 6, 23, 0.32);
}

.strategy-native-code__help strong {
  color: var(--tv-text);
  font-size: 0.77rem;
}

.strategy-native-code :deep(.monaco-code-editor-shell) {
  min-height: 0;
  height: 100%;
  border: 0;
  border-radius: 0;
}

@media (max-width: 768px) {
  .strategy-native-code {
    grid-template-rows: 42px minmax(0, 1fr);
  }

  .strategy-native-workspace-bar {
    min-height: 42px;
  }

  .strategy-native-toggle,
  .strategy-native-code__help-button {
    min-height: 40px;
  }

  .strategy-native-code__help-button {
    width: 40px;
    height: 40px;
  }
}
</style>
