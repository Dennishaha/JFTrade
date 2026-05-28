<script setup lang="ts">
import type { StrategyDefinitionDocument } from "@jftrade/ui-contracts";
import { ref, type Ref } from "vue";

import MonacoCodeEditor from "../MonacoCodeEditor.vue";
import type {
  MonacoCompletionDefinition,
  MonacoExtraLibDefinition,
  MonacoHoverDefinition,
} from "../../features/strategyMonacoIntelliSenseTypes";

import "./strategyStageShared.css";

interface StrategyCodeWorkbenchBindings {
  definitionForm: Ref<StrategyDefinitionDocument>;
}

const props = defineProps<{
  bindings: StrategyCodeWorkbenchBindings;
  strategyDisplayMode: "canvas" | "split" | "code";
  completionItems: MonacoCompletionDefinition[];
  extraLibs: MonacoExtraLibDefinition[];
  hoverItems: MonacoHoverDefinition[];
}>();

const emit = defineEmits<{
  "drag-start": [event: MouseEvent];
  "script-blur": [];
  "cursor-offset": [offset: number];
}>();

const definitionForm = props.bindings.definitionForm;
const monacoEditorRef = ref<InstanceType<typeof MonacoCodeEditor> | null>(null);
const dslEditorPlaceholder = 'on kline_close:\n  log "kline closed"';

interface CodeOffsetRange {
  start: number;
  end: number;
}

function revealCodeRange(range: CodeOffsetRange): void {
  monacoEditorRef.value?.revealOffsetRange(range);
}

defineExpose({
  revealCodeRange,
});
</script>

<template>
  <div class="strategy-stage__panel-head strategy-stage__drag-handle" @mousedown="emit('drag-start', $event)">
    <div class="strategy-stage__section-title">DSL 策略工作台</div>
  </div>

  <div class="strategy-stage__panel-body strategy-stage__panel-body--editor">
    <MonacoCodeEditor
      ref="monacoEditorRef"
      v-model="definitionForm.script"
      :completion-items="props.completionItems"
      :extra-libs="props.extraLibs"
      :hover-items="props.hoverItems"
      :resizable="false"
      class="flex-1 min-h-0"
      height="100%"
      language="jftrade-dsl"
      min-height="280px"
      :placeholder="dslEditorPlaceholder"
      test-id="strategy-script-editor"
      @blur="emit('script-blur')"
      @cursor-offset="emit('cursor-offset', $event)"
    />
  </div>
</template>