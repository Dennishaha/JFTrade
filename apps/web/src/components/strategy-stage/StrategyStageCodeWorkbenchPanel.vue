<script setup lang="ts">
import type { StrategyDefinitionDocument } from "@/contracts";
import { ref, type Ref } from "vue";

import MonacoCodeEditor from "../MonacoCodeEditor.vue";
import type {
  MonacoCompletionDefinition,
  MonacoDiagnosticMarker,
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
  diagnosticMarkers?: MonacoDiagnosticMarker[];
  extraLibs: MonacoExtraLibDefinition[];
  hoverItems: MonacoHoverDefinition[];
  supportFeatureCount?: number;
}>();

const emit = defineEmits<{
  "drag-start": [event: MouseEvent];
  "script-blur": [];
  "cursor-offset": [offset: number];
}>();

const definitionForm = props.bindings.definitionForm;
const monacoEditorRef = ref<InstanceType<typeof MonacoCodeEditor> | null>(null);
const pineEditorPlaceholder = '//@version=6\nstrategy("My Strategy", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)\nfast = ta.ema(close, 8)';

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
    <div class="strategy-stage__section-title">Pine 策略工作台</div>
    <div v-if="(props.supportFeatureCount ?? 0) > 0" class="strategy-stage__section-meta">
      Pine v6 子集 · {{ props.supportFeatureCount }} 项能力
    </div>
  </div>

  <div class="strategy-stage__panel-body strategy-stage__panel-body--editor">
    <MonacoCodeEditor
      ref="monacoEditorRef"
      v-model="definitionForm.script"
      :completion-items="props.completionItems"
      :diagnostic-markers="props.diagnosticMarkers ?? []"
      :extra-libs="props.extraLibs"
      :hover-items="props.hoverItems"
      :resizable="false"
      class="flex-1 min-h-0"
      height="100%"
      language="pine-v6"
      min-height="280px"
      :placeholder="pineEditorPlaceholder"
      test-id="strategy-script-editor"
      @blur="emit('script-blur')"
      @cursor-offset="emit('cursor-offset', $event)"
    />
  </div>
</template>
