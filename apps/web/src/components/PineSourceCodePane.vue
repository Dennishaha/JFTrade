<script setup lang="ts">
import { ref } from "vue";

import {
  strategyPineEditorCompletions,
  strategyPineEditorExtraLibs,
  strategyPineEditorHoverItems,
} from "../features/strategyPineEditorIntelliSense";
import MonacoCodeEditor from "./MonacoCodeEditor.vue";

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
        <div>
          <div class="strategy-native-panel__title">Pine v6 源码</div>
          <div class="strategy-native-meta">
            源码是运行权威；左侧结构块由当前源码解析生成。
          </div>
        </div>
        <label class="strategy-native-toggle">
          <input
            :checked="sourceEditingEnabled"
            data-testid="strategy-source-override-toggle"
            type="checkbox"
            @change="emit('update:sourceEditingEnabled', ($event.target as HTMLInputElement).checked)"
          >
          <span>源码编辑</span>
        </label>
      </div>
      <MonacoCodeEditor
        ref="sourceEditorRef"
        :model-value="modelValue"
        language="pine-v6"
        height="100%"
        min-height="0"
        test-id="strategy-script-editor"
        :read-only="!sourceEditingEnabled"
        :extra-libs="strategyPineEditorExtraLibs"
        :completion-items="strategyPineEditorCompletions"
        :hover-items="strategyPineEditorHoverItems"
        :diagnostic-markers="diagnosticMarkers"
        @update:model-value="emit('update:modelValue', $event)"
      />
      <button type="button" @click="advancedSourceOpen = !advancedSourceOpen">
        {{ advancedSourceOpen ? "收起说明" : "Pine v6 支持边界" }}
      </button>
      <div v-if="advancedSourceOpen" class="strategy-native-meta">
        当前按闭合 K 线执行；订单按下一根 K 线成交；OCA、部分成交、tick 级重算是明确边界。开启源码编辑后，
        保存的 script 以源码为准，visualModel 保留当前指令快照。
      </div>
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
  gap: 0.75rem;
  padding: 0.75rem;
}

.strategy-native-panel {
  display: grid;
  gap: 0.75rem;
  border: 1px solid var(--tv-border);
  border-radius: 0.5rem;
  background: color-mix(in srgb, var(--tv-bg-surface) 96%, transparent);
  padding: 0.85rem;
}

.strategy-native-code {
  min-height: 0;
  height: 100%;
  grid-template-rows: auto minmax(0, 1fr) auto auto;
  border: 0;
  border-radius: 0;
  background: transparent;
  padding: 0;
}

.strategy-native-workspace-bar {
  min-width: 0;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5rem;
  justify-content: space-between;
}

.strategy-native-panel__title {
  color: var(--tv-text-muted);
  font-size: 0.72rem;
  font-weight: 800;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.strategy-native-meta {
  color: var(--tv-text-muted);
  font-size: 0.82rem;
  line-height: 1.45;
}

.strategy-native-toggle {
  display: inline-flex !important;
  grid-template-columns: auto 1fr;
  align-items: center;
  gap: 0.5rem;
  color: var(--tv-text-muted);
  font-size: 0.72rem;
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

.strategy-native-code :deep(.monaco-code-editor-shell) {
  min-height: 0;
  height: 100%;
  border-radius: 0.5rem;
  border-color: var(--tv-border);
}
</style>
