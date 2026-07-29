<script setup lang="ts">
import type { Ref } from "vue";

import type { PineSourceBlock } from "../../features/pineSourceStructureIndex";
import PineSourceStructureBlockList from "../PineSourceStructureBlockList.vue";
import { useStrategyDesignContext } from "./strategyDesignContext";

interface InstructionContext {
  sourceStructureNodes: Readonly<Ref<PineSourceBlock[]>>;
  rawSourceNodeCount: Readonly<Ref<number>>;
  selectedSourceNodeSummary: Readonly<Ref<string>>;
  selectedSourceNodeId: Ref<string>;
  expandedSourceNodeId: Ref<string | null>;
  toggleSourceBlockExpansion: (block: PineSourceBlock) => void;
  addSourceBlock: (kind: string) => void;
  changeSourceBlockKind: (block: PineSourceBlock, kind: string) => void;
  deleteSourceStructureBlock: (block: PineSourceBlock) => void;
  duplicateSourceStructureBlock: (block: PineSourceBlock) => void;
  moveSourceStructureBlock: (block: PineSourceBlock, direction: -1 | 1) => void;
  updateSourceBlockField: (block: PineSourceBlock, key: string, value: unknown) => void;
}

const context = useStrategyDesignContext<InstructionContext>();
</script>

<template>
  <main class="strategy-native-main">
    <section class="strategy-native-panel strategy-native-panel--workspace">
      <div class="strategy-native-workspace-bar">
        <div class="strategy-native-workspace-bar__identity">
          <div class="strategy-native-panel__title">结构指令</div>
          <span class="strategy-native-chip">{{ context.sourceStructureNodes.value.length }} 节点</span>
          <span class="strategy-native-chip">{{ context.rawSourceNodeCount.value }} raw</span>
          <span class="strategy-native-execution-hint">收盘确认 / 下一根 K 线成交</span>
        </div>
        <div
          class="strategy-native-selected-block"
          :title="context.selectedSourceNodeSummary.value"
        >
          {{ context.selectedSourceNodeSummary.value }}
        </div>
      </div>
      <div class="strategy-native-block-scroll" data-testid="strategy-instruction-scroll">
        <PineSourceStructureBlockList
          :nodes="context.sourceStructureNodes.value"
          :selected-id="context.selectedSourceNodeId.value"
          :expanded-id="context.expandedSourceNodeId.value"
          @toggle-block="context.toggleSourceBlockExpansion"
          @add-block="context.addSourceBlock"
          @change-kind="context.changeSourceBlockKind"
          @delete-block="context.deleteSourceStructureBlock"
          @duplicate-block="context.duplicateSourceStructureBlock"
          @move-block="context.moveSourceStructureBlock"
          @update-field="context.updateSourceBlockField"
        />
      </div>
    </section>
  </main>
</template>

<style scoped>
.strategy-native-main {
  min-width: 0;
  min-height: 0;
  height: 100%;
  overflow: hidden;
  background: var(--tv-bg-app);
}

.strategy-native-workspace-bar {
  display: flex;
  min-width: 0;
  min-height: 36px;
  flex: 0 0 36px;
  align-items: center;
  justify-content: space-between;
  flex-wrap: nowrap;
  gap: 6px;
  padding: 0 8px;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.strategy-native-workspace-bar__identity {
  display: flex;
  min-width: 0;
  flex: 1 1 auto;
  align-items: center;
  gap: 5px;
  overflow: hidden;
}

.strategy-native-panel__title {
  color: var(--tv-text-muted);
  font-size: 0.78rem;
  font-weight: 750;
  letter-spacing: 0.04em;
}

.strategy-native-chip {
  display: inline-flex;
  min-height: 20px;
  flex: 0 0 auto;
  align-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  padding: 1px 6px;
  color: var(--tv-text-muted);
  font-size: 0.7rem;
  font-weight: 700;
  white-space: nowrap;
}

.strategy-native-execution-hint {
  min-width: 0;
  overflow: hidden;
  border-left: 1px solid var(--tv-border);
  padding-left: 6px;
  color: var(--tv-text-muted);
  font-size: 0.7rem;
  line-height: 1;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-native-selected-block {
  max-width: 42%;
  overflow: hidden;
  color: var(--tv-text-muted);
  font-size: 0.73rem;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-native-panel--workspace {
  display: grid;
  min-width: 0;
  min-height: 0;
  height: 100%;
  grid-template-rows: auto minmax(0, 1fr);
  overflow: hidden;
  border: 0;
  background: transparent;
}

.strategy-native-block-scroll {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  width: 100%;
  min-width: 0;
  min-height: 0;
  align-content: start;
  overflow: auto;
  padding: 6px;
}

@media (max-width: 768px) {
  .strategy-native-workspace-bar {
    min-height: 42px;
    height: auto;
    flex-basis: auto;
  }

  .strategy-native-selected-block,
  .strategy-native-execution-hint {
    display: none;
  }
}
</style>
