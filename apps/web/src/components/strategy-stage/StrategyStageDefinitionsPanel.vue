<script setup lang="ts">
import type { StrategyDefinitionDocument } from "@/contracts";

import "./strategyStageShared.css";

const props = defineProps<{
  isLoadingDefinitions: boolean;
  strategyDefinitions: StrategyDefinitionDocument[];
  selectedDefinitionId: string;
  deletingDefinitionId?: string;
}>();

const emit = defineEmits<{
  "drag-start": [event: MouseEvent];
  "create-new": [];
  "close": [];
  "select-definition": [definition: StrategyDefinitionDocument];
  "delete-definition": [definition: StrategyDefinitionDocument];
}>();
</script>

<template>
  <div class="strategy-stage__panel-head strategy-stage__drag-handle" @mousedown="emit('drag-start', $event)">
    <div class="strategy-stage__section-title">策略定义</div>
    <div class="flex items-center gap-1.5">
      <button
        class="strategy-btn strategy-btn--ghost"
        data-testid="new-strategy-definition"
        type="button"
        @mousedown.stop
        @click="emit('create-new')"
      >
        新建
      </button>
      <button
        class="strategy-icon-btn"
        data-testid="toggle-strategy-definitions"
        title="收起定义列表"
        type="button"
        @mousedown.stop
        @click="emit('close')"
      >
        ‹
      </button>
    </div>
  </div>

  <div class="strategy-stage__panel-body">
    <div v-if="props.isLoadingDefinitions" class="strategy-empty-state">
      正在加载策略定义…
    </div>

    <div v-else-if="props.strategyDefinitions.length === 0" class="strategy-empty-state">
      暂无已保存的策略定义，可以直接从画布上方工具栏打开模板面板创建新草稿。
    </div>

    <div v-else class="grid gap-3">
      <div
        v-for="definition in props.strategyDefinitions"
        :key="definition.id"
        class="strategy-list-card"
        :class="{ 'is-active': definition.id === props.selectedDefinitionId }"
      >
        <div class="flex items-start justify-between gap-3">
          <button
            :data-testid="`strategy-definition-${definition.id}`"
            class="min-w-0 flex-1 text-left"
            type="button"
            @click="emit('select-definition', definition)"
          >
            <div class="text-base font-semibold">{{ definition.name }}</div>
            <div class="mt-2 text-sm text-slate-500">版本 {{ definition.version }}</div>
            <div class="mt-2 text-xs uppercase tracking-[0.2em] text-slate-500">{{ definition.runtime }}</div>
          </button>
          <button
            :data-testid="`delete-strategy-definition-${definition.id}`"
            class="strategy-definition-delete-btn"
            :disabled="props.deletingDefinitionId === definition.id"
            type="button"
            @click="emit('delete-definition', definition)"
          >
            {{ props.deletingDefinitionId === definition.id ? '删除中' : '删除' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
