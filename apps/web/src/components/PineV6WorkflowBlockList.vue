<script setup lang="ts">
import { computed, ref } from "vue";

import type { PineV6WorkflowBlock, PineV6WorkflowBlockKind } from "@/contracts";

import {
  PINE_V6_BLOCK_KINDS,
  createPineV6WorkflowBlock,
} from "../features/pineV6Workflow";

const props = withDefaults(defineProps<{
  blocks: PineV6WorkflowBlock[];
  depth?: number;
}>(), {
  depth: 0,
});

const emit = defineEmits<{
  "update:blocks": [blocks: PineV6WorkflowBlock[]];
  "select-block": [blockId: string];
}>();

const blockKindOptions = computed(() => PINE_V6_BLOCK_KINDS);
const canNestFurther = computed(() => props.depth < 3);
const expandedBlockId = ref<string | null>(null);
const expandedBranchIds = ref<string[]>(["then", "else"]);

function replaceBlocks(nextBlocks: PineV6WorkflowBlock[]): void {
  emit("update:blocks", nextBlocks);
}

function updateBlock(index: number, patch: Partial<PineV6WorkflowBlock>): void {
  replaceBlocks(props.blocks.map((block, blockIndex) =>
    blockIndex === index ? { ...block, ...patch } : block,
  ));
}

function updateBlockParam(index: number, key: string, value: unknown): void {
  const block = props.blocks[index];
  if (block === undefined) {
    return;
  }
  updateBlock(index, {
    params: {
      ...block.params,
      [key]: value,
    },
  });
}

function updateNestedBlocks(
  index: number,
  branch: "thenBlocks" | "elseBlocks",
  value: PineV6WorkflowBlock[],
): void {
  updateBlock(index, { [branch]: value });
}

function addBlock(kind: PineV6WorkflowBlockKind): void {
  const block = createPineV6WorkflowBlock(kind);
  expandedBlockId.value = block.id;
  replaceBlocks([...props.blocks, block]);
}

function duplicateBlock(index: number): void {
  const source = props.blocks[index];
  if (source === undefined) {
    return;
  }
  const duplicate = cloneBlock(source);
  expandedBlockId.value = duplicate.id;
  replaceBlocks([
    ...props.blocks.slice(0, index + 1),
    duplicate,
    ...props.blocks.slice(index + 1),
  ]);
}

function deleteBlock(index: number): void {
  const block = props.blocks[index];
  if (block?.id === expandedBlockId.value) {
    expandedBlockId.value = null;
  }
  replaceBlocks(props.blocks.filter((_, blockIndex) => blockIndex !== index));
}

function moveBlock(index: number, direction: -1 | 1): void {
  const targetIndex = index + direction;
  if (targetIndex < 0 || targetIndex >= props.blocks.length) {
    return;
  }
  const next = [...props.blocks];
  const [item] = next.splice(index, 1);
  if (item === undefined) {
    return;
  }
  next.splice(targetIndex, 0, item);
  replaceBlocks(next);
}

function changeBlockKind(index: number, kind: string): void {
  const block = props.blocks[index];
  if (block === undefined) {
    return;
  }
  const next = createPineV6WorkflowBlock(kind as PineV6WorkflowBlockKind);
  updateBlock(index, {
    kind: next.kind,
    title: next.title,
    params: next.params,
    ...(next.thenBlocks === undefined ? {} : { thenBlocks: next.thenBlocks }),
    ...(next.elseBlocks === undefined ? {} : { elseBlocks: next.elseBlocks }),
  });
}

function setExpandedBlockId(value: unknown): void {
  expandedBlockId.value = typeof value === "string" ? value : null;
  if (expandedBlockId.value !== null) {
    emit("select-block", expandedBlockId.value);
  }
}

function toggleBlockExpansion(blockId: string): void {
  setExpandedBlockId(expandedBlockId.value === blockId ? null : blockId);
}

function isBranchExpanded(branch: "then" | "else"): boolean {
  return expandedBranchIds.value.includes(branch);
}

function cloneBlock(block: PineV6WorkflowBlock): PineV6WorkflowBlock {
  const cloned = createPineV6WorkflowBlock(block.kind);
  return {
    ...block,
    id: cloned.id,
    params: { ...block.params },
    ...(block.thenBlocks === undefined ? {} : { thenBlocks: block.thenBlocks.map(cloneBlock) }),
    ...(block.elseBlocks === undefined ? {} : { elseBlocks: block.elseBlocks.map(cloneBlock) }),
  };
}

function readParam(block: PineV6WorkflowBlock, key: string): string {
  const value = block.params[key];
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return typeof value === "string" ? value : "";
}

function blockKindLabel(kind: PineV6WorkflowBlockKind): string {
  return blockKindOptions.value.find((option) => option.kind === kind)?.label ?? kind;
}

function addBlockLabel(kind: PineV6WorkflowBlockKind, label: string): string {
  return kind === "series_assign" ? "+ 新增序列" : `+ 新增${label}`;
}

function filledParam(block: PineV6WorkflowBlock, key: string): string {
  return readParam(block, key).trim();
}

function describeBlock(block: PineV6WorkflowBlock): string {
  switch (block.kind) {
    case "series_assign":
      return `${filledParam(block, "name") || "变量"} 设为 ${filledParam(block, "expression") || "表达式"}`;
    case "var_state":
      return `持久变量 ${filledParam(block, "name") || "变量"} 初始为 ${filledParam(block, "initial") || "空值"}`;
    case "if":
      return `当 ${filledParam(block, "condition") || "条件"} 成立时执行分支`;
    case "request_security":
      return `读取 ${filledParam(block, "symbol") || "标的"} ${filledParam(block, "timeframe") || "周期"} 的 ${filledParam(block, "expression") || "表达式"}`;
    case "array_op":
      return `${filledParam(block, "name") || "数组"} 执行 ${filledParam(block, "mode") || "数组操作"}`;
    case "strategy_entry":
      return `按 ${filledParam(block, "direction") || "方向"} 开仓，订单 ${filledParam(block, "id") || "未命名"}`;
    case "strategy_order":
      return `提交 ${filledParam(block, "direction") || "方向"} 订单 ${filledParam(block, "id") || "未命名"}`;
    case "strategy_exit":
      return `从 ${filledParam(block, "from_entry") || "入场单"} 退出，退出单 ${filledParam(block, "id") || "未命名"}`;
    case "strategy_close":
      return `平仓 ${filledParam(block, "id") || "入场单"}${filledParam(block, "when") ? `，条件 ${filledParam(block, "when")}` : ""}`;
    case "plot":
      return `绘制 ${filledParam(block, "series") || "序列"}${filledParam(block, "title") ? `，标题 ${filledParam(block, "title")}` : ""}`;
    case "alertcondition":
      return `当 ${filledParam(block, "condition") || "条件"} 成立时触发提醒`;
    case "log":
      return `记录日志：${filledParam(block, "message") || "消息"}`;
    default:
      return block.title || block.kind;
  }
}
</script>

<template>
  <div class="pine-block-list" :class="`pine-block-list--depth-${depth}`">
    <div v-if="blocks.length === 0" class="pine-block-list__empty">
      当前分支还没有 Pine v6 块。
    </div>

    <v-expansion-panels
      v-else
      :model-value="expandedBlockId"
      class="pine-block-panels"
      variant="default"
    >
      <v-expansion-panel
        v-for="(block, index) in blocks"
        :key="block.id"
        :value="block.id"
        class="pine-block"
        :class="{ 'pine-block--disabled': !block.enabled }"
        :data-testid="`pine-v6-workflow-block-${block.kind}`"
      >
        <v-expansion-panel-title class="pine-block__summary" @click="toggleBlockExpansion(block.id)">
          <div class="pine-block__rail" />
          <div class="pine-block__summary-text">
            <div class="pine-block__summary-heading">
              <span class="pine-block__kind-badge">{{ blockKindLabel(block.kind) }}</span>
              <strong>{{ block.title || blockKindLabel(block.kind) }}</strong>
              <span v-if="!block.enabled" class="pine-block__disabled-label">已停用</span>
            </div>
            <div class="pine-block__summary-description">
              {{ describeBlock(block) }}
            </div>
          </div>
        </v-expansion-panel-title>

        <v-expansion-panel-text v-if="expandedBlockId === block.id">
          <div class="pine-block__body">
            <div class="pine-block__header">
              <label class="pine-block__enabled">
                <input
                  :checked="block.enabled"
                  type="checkbox"
                  @change="updateBlock(index, { enabled: ($event.target as HTMLInputElement).checked })"
                >
                <span>启用</span>
              </label>

              <select
                class="pine-block__kind"
                :value="block.kind"
                @change="changeBlockKind(index, ($event.target as HTMLSelectElement).value)"
              >
                <option
                  v-for="option in blockKindOptions"
                  :key="option.kind"
                  :value="option.kind"
                >
                  {{ option.label }}
                </option>
              </select>

              <input
                class="pine-block__title"
                :value="block.title"
                placeholder="块标题"
                @input="updateBlock(index, { title: ($event.target as HTMLInputElement).value })"
              >

              <div class="pine-block__actions">
                <button type="button" :disabled="index === 0" title="上移" @click.stop="moveBlock(index, -1)">↑</button>
                <button type="button" :disabled="index === blocks.length - 1" title="下移" @click.stop="moveBlock(index, 1)">↓</button>
                <button type="button" title="复制" @click.stop="duplicateBlock(index)">⧉</button>
                <button type="button" title="删除" @click.stop="deleteBlock(index)">×</button>
              </div>
            </div>

            <div class="pine-block__params">
              <template v-if="block.kind === 'series_assign'">
                <label>
                  <span>变量名</span>
                  <input :value="readParam(block, 'name')" @input="updateBlockParam(index, 'name', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>表达式</span>
                  <input :value="readParam(block, 'expression')" @input="updateBlockParam(index, 'expression', ($event.target as HTMLInputElement).value)">
                </label>
              </template>

              <template v-else-if="block.kind === 'var_state'">
                <label>
                  <span>变量名</span>
                  <input :value="readParam(block, 'name')" @input="updateBlockParam(index, 'name', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>初始值</span>
                  <input :value="readParam(block, 'initial')" @input="updateBlockParam(index, 'initial', ($event.target as HTMLInputElement).value)">
                </label>
              </template>

              <template v-else-if="block.kind === 'if'">
                <label class="pine-block__wide">
                  <span>条件</span>
                  <input :value="readParam(block, 'condition')" @input="updateBlockParam(index, 'condition', ($event.target as HTMLInputElement).value)">
                </label>
              </template>

              <template v-else-if="block.kind === 'request_security'">
                <label>
                  <span>变量名</span>
                  <input :value="readParam(block, 'name')" @input="updateBlockParam(index, 'name', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>标的</span>
                  <input :value="readParam(block, 'symbol')" @input="updateBlockParam(index, 'symbol', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>周期</span>
                  <input :value="readParam(block, 'timeframe')" @input="updateBlockParam(index, 'timeframe', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>表达式</span>
                  <input :value="readParam(block, 'expression')" @input="updateBlockParam(index, 'expression', ($event.target as HTMLInputElement).value)">
                </label>
              </template>

              <template v-else-if="block.kind === 'array_op'">
                <label>
                  <span>数组名</span>
                  <input :value="readParam(block, 'name')" @input="updateBlockParam(index, 'name', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>模式</span>
                  <select :value="readParam(block, 'mode')" @change="updateBlockParam(index, 'mode', ($event.target as HTMLSelectElement).value)">
                    <option value="new_float">array.new_float</option>
                    <option value="push">array.push</option>
                    <option value="median">array.median</option>
                  </select>
                </label>
                <label>
                  <span>数值/输出</span>
                  <input :value="readParam(block, 'value') || readParam(block, 'output')" @input="updateBlockParam(index, readParam(block, 'mode') === 'median' ? 'output' : 'value', ($event.target as HTMLInputElement).value)">
                </label>
              </template>

              <template v-else-if="block.kind === 'strategy_entry' || block.kind === 'strategy_order'">
                <label>
                  <span>订单 ID</span>
                  <input :value="readParam(block, 'id')" @input="updateBlockParam(index, 'id', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>方向</span>
                  <select :value="readParam(block, 'direction')" @change="updateBlockParam(index, 'direction', ($event.target as HTMLSelectElement).value)">
                    <option value="strategy.long">strategy.long</option>
                    <option value="strategy.short">strategy.short</option>
                  </select>
                </label>
                <label>
                  <span>数量</span>
                  <input :value="readParam(block, 'qty')" placeholder="可选" @input="updateBlockParam(index, 'qty', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>限价</span>
                  <input :value="readParam(block, 'limit')" placeholder="可选" @input="updateBlockParam(index, 'limit', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>止损/触发价</span>
                  <input :value="readParam(block, 'stop')" placeholder="可选" @input="updateBlockParam(index, 'stop', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>触发条件</span>
                  <input :value="readParam(block, 'when')" placeholder="可选" @input="updateBlockParam(index, 'when', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>OCA 名称</span>
                  <input :value="readParam(block, 'oca_name')" placeholder="暂不支持" @input="updateBlockParam(index, 'oca_name', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>OCA 类型</span>
                  <input :value="readParam(block, 'oca_type')" placeholder="暂不支持" @input="updateBlockParam(index, 'oca_type', ($event.target as HTMLInputElement).value)">
                </label>
              </template>

              <template v-else-if="block.kind === 'strategy_exit'">
                <label>
                  <span>退出 ID</span>
                  <input :value="readParam(block, 'id')" @input="updateBlockParam(index, 'id', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>来源入场 ID</span>
                  <input :value="readParam(block, 'from_entry')" @input="updateBlockParam(index, 'from_entry', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>限价</span>
                  <input :value="readParam(block, 'limit')" @input="updateBlockParam(index, 'limit', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>止损/触发价</span>
                  <input :value="readParam(block, 'stop')" @input="updateBlockParam(index, 'stop', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>止盈点数</span>
                  <input :value="readParam(block, 'profit')" @input="updateBlockParam(index, 'profit', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>止损点数</span>
                  <input :value="readParam(block, 'loss')" @input="updateBlockParam(index, 'loss', ($event.target as HTMLInputElement).value)">
                </label>
              </template>

              <template v-else-if="block.kind === 'strategy_close'">
                <label>
                  <span>入场 ID</span>
                  <input :value="readParam(block, 'id')" @input="updateBlockParam(index, 'id', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>触发条件</span>
                  <input :value="readParam(block, 'when')" @input="updateBlockParam(index, 'when', ($event.target as HTMLInputElement).value)">
                </label>
              </template>

              <template v-else-if="block.kind === 'plot'">
                <label>
                  <span>序列</span>
                  <input :value="readParam(block, 'series')" @input="updateBlockParam(index, 'series', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>标题</span>
                  <input :value="readParam(block, 'title')" @input="updateBlockParam(index, 'title', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>颜色</span>
                  <input :value="readParam(block, 'color')" @input="updateBlockParam(index, 'color', ($event.target as HTMLInputElement).value)">
                </label>
              </template>

              <template v-else-if="block.kind === 'alertcondition'">
                <label>
                  <span>条件</span>
                  <input :value="readParam(block, 'condition')" @input="updateBlockParam(index, 'condition', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>标题</span>
                  <input :value="readParam(block, 'title')" @input="updateBlockParam(index, 'title', ($event.target as HTMLInputElement).value)">
                </label>
                <label>
                  <span>消息</span>
                  <input :value="readParam(block, 'message')" @input="updateBlockParam(index, 'message', ($event.target as HTMLInputElement).value)">
                </label>
              </template>

              <template v-else-if="block.kind === 'log'">
                <label class="pine-block__wide">
                  <span>消息</span>
                  <input :value="readParam(block, 'message')" @input="updateBlockParam(index, 'message', ($event.target as HTMLInputElement).value)">
                </label>
              </template>
            </div>

            <div v-if="block.kind === 'strategy_entry' || block.kind === 'strategy_order'" class="pine-block__boundary">
              下一根 K 线成交；OCA 当前为明确不支持边界，填写 oca_* 会产生诊断。
            </div>
            <div v-if="block.kind === 'request_security'" class="pine-block__boundary">
              当前运行时只支持静态周期和纯表达式 request.security。
            </div>

            <v-expansion-panels
              v-if="block.kind === 'if' && canNestFurther"
              v-model="expandedBranchIds"
              multiple
              class="pine-block__branches"
              variant="default"
            >
              <v-expansion-panel value="then" class="pine-block__branch-panel">
                <v-expansion-panel-title class="pine-block__branch-title">
                  <div>
                    <strong>是</strong>
                    <span>条件成立时执行</span>
                  </div>
                </v-expansion-panel-title>
                <v-expansion-panel-text v-if="isBranchExpanded('then')">
                  <PineV6WorkflowBlockList
                    :blocks="block.thenBlocks ?? []"
                    :depth="depth + 1"
                    @select-block="emit('select-block', $event)"
                    @update:blocks="updateNestedBlocks(index, 'thenBlocks', $event)"
                  />
                </v-expansion-panel-text>
              </v-expansion-panel>
              <v-expansion-panel value="else" class="pine-block__branch-panel">
                <v-expansion-panel-title class="pine-block__branch-title">
                  <div>
                    <strong>否</strong>
                    <span>条件不成立时执行</span>
                  </div>
                </v-expansion-panel-title>
                <v-expansion-panel-text v-if="isBranchExpanded('else')">
                  <PineV6WorkflowBlockList
                    :blocks="block.elseBlocks ?? []"
                    :depth="depth + 1"
                    @select-block="emit('select-block', $event)"
                    @update:blocks="updateNestedBlocks(index, 'elseBlocks', $event)"
                  />
                </v-expansion-panel-text>
              </v-expansion-panel>
            </v-expansion-panels>
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>
    </v-expansion-panels>

    <div class="pine-block-list__add">
      <select
        :value="blockKindOptions[0]?.kind ?? 'series_assign'"
        @change="addBlock(($event.target as HTMLSelectElement).value as PineV6WorkflowBlockKind)"
      >
        <option disabled value="">添加 Pine v6 块</option>
        <option v-for="option in blockKindOptions" :key="option.kind" :value="option.kind">
          {{ addBlockLabel(option.kind, option.label) }}
        </option>
      </select>
    </div>
  </div>
</template>

<style scoped>
.pine-block-list {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  justify-items: stretch;
  gap: 0.65rem;
  width: 100%;
  min-width: 0;
}

.pine-block-list__empty {
  border: 1px dashed var(--tv-border);
  border-radius: 0.5rem;
  padding: 0.85rem;
  color: var(--tv-text-muted);
  font-size: 0.85rem;
}

.pine-block-panels {
  display: grid !important;
  grid-template-columns: minmax(0, 1fr);
  justify-items: stretch;
  gap: 0.65rem;
  width: 100% !important;
  inline-size: 100% !important;
  min-width: 0;
  max-width: none !important;
}

.pine-block-panels :deep(.v-expansion-panel-title__overlay),
.pine-block-panels :deep(.v-expansion-panel__overlay) {
  display: none;
}

.pine-block-panels :deep(.v-expansion-panel-title),
.pine-block-panels :deep(.pine-block__summary.v-expansion-panel-title) {
  width: 100% !important;
  inline-size: 100% !important;
  max-width: none !important;
}

.pine-block-panels :deep(.v-expansion-panel) {
  flex: 1 1 100% !important;
  justify-self: stretch;
  width: 100% !important;
  inline-size: 100% !important;
  min-width: 0;
  max-width: none !important;
}

.pine-block-panels :deep(.v-expansion-panel-title__icon) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  align-self: center;
  margin-inline-start: 0.75rem;
  padding-right: 0.75rem;
  color: var(--tv-text);
}

.pine-block-panels :deep(.v-expansion-panel-title__icon .v-icon) {
  opacity: 0.92;
}

.pine-block-panels :deep(.v-expansion-panel-text__wrapper) {
  padding: 0;
}

.pine-block {
  flex-basis: 100% !important;
  justify-self: stretch;
  width: 100% !important;
  inline-size: 100% !important;
  min-width: 0;
  max-width: none !important;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 0.5rem;
  background: color-mix(in srgb, var(--tv-bg-surface) 96%, transparent);
  color: var(--tv-text);
}

.pine-block + .pine-block {
  margin-top: 0;
}

.pine-block--disabled {
  opacity: 0.58;
}

.pine-block__rail {
  width: 0.35rem;
  align-self: stretch;
  background: color-mix(in srgb, var(--tv-accent) 70%, #14b8a6);
}

.pine-block__summary {
  width: 100% !important;
  inline-size: 100% !important;
  max-width: none !important;
  min-height: 4rem;
  display: grid !important;
  grid-template-columns: 0.35rem minmax(0, 1fr) auto;
  gap: 0.85rem;
  align-items: stretch;
  padding: 0;
  color: var(--tv-text);
}

.pine-block__summary-text {
  min-width: 0;
  display: grid;
  align-content: center;
  gap: 0.25rem;
  padding: 0.7rem 0;
}

.pine-block__summary-heading {
  min-width: 0;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.45rem;
}

.pine-block__summary-heading strong {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.9rem;
}

.pine-block__kind-badge,
.pine-block__disabled-label {
  flex-shrink: 0;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  padding: 0.12rem 0.45rem;
  color: var(--tv-text-muted);
  font-size: 0.7rem;
  font-weight: 800;
}

.pine-block__disabled-label {
  border-color: #fecaca;
  color: #991b1b;
}

.pine-block__summary-description {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--tv-text-muted);
  font-size: 0.84rem;
  line-height: 1.35;
}

.pine-block__body {
  min-width: 0;
  display: grid;
  gap: 0.75rem;
  padding: 0.85rem;
}

.pine-block__header {
  display: grid;
  grid-template-columns: auto minmax(9rem, 12rem) minmax(0, 1fr) auto;
  gap: 0.65rem;
  align-items: center;
}

.pine-block__enabled {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.78rem;
  color: var(--tv-text-muted);
}

.pine-block__kind,
.pine-block__title,
.pine-block__params input,
.pine-block__params select,
.pine-block-list__add select {
  width: 100%;
  min-width: 0;
  border: 1px solid var(--tv-border);
  border-radius: 0.45rem;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 0.45rem 0.55rem;
  font-size: 0.85rem;
  outline: none;
}

.pine-block__params {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr));
  gap: 0.65rem;
}

.pine-block__params label {
  min-width: 0;
  display: grid;
  gap: 0.25rem;
  color: var(--tv-text-muted);
  font-size: 0.72rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.pine-block__wide {
  grid-column: 1 / -1;
}

.pine-block__actions {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
}

.pine-block__actions button {
  width: 1.8rem;
  height: 1.8rem;
  border: 1px solid var(--tv-border);
  border-radius: 0.45rem;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
}

.pine-block__actions button:disabled {
  opacity: 0.35;
}

.pine-block__boundary {
  border: 1px solid color-mix(in srgb, #f59e0b 36%, var(--tv-border));
  border-radius: 0.45rem;
  background: color-mix(in srgb, #f59e0b 10%, transparent);
  color: color-mix(in srgb, #92400e 80%, var(--tv-text));
  padding: 0.55rem 0.65rem;
  font-size: 0.78rem;
}

.pine-block__branches {
  display: grid !important;
  grid-template-columns: minmax(0, 1fr);
  justify-items: stretch;
  gap: 0.75rem;
  width: 100% !important;
  inline-size: 100% !important;
  min-width: 0;
}

.pine-block__branches :deep(.v-expansion-panel-title__overlay),
.pine-block__branches :deep(.v-expansion-panel__overlay) {
  display: none;
}

.pine-block__branches :deep(.v-expansion-panel-title__icon) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  align-self: center;
  margin-inline-start: 0.75rem;
  padding-right: 0.75rem;
}

.pine-block__branches :deep(.v-expansion-panel) {
  flex: 1 1 100% !important;
  justify-self: stretch;
  width: 100% !important;
  inline-size: 100% !important;
  min-width: 0;
  max-width: none !important;
}

.pine-block__branches :deep(.v-expansion-panel-text__wrapper) {
  padding: 0.75rem 0.85rem 0.85rem;
}

.pine-block__branch-panel {
  width: 100% !important;
  inline-size: 100% !important;
  min-width: 0;
  max-width: none !important;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 0.45rem;
  background: color-mix(in srgb, var(--tv-bg-elevated) 58%, transparent);
  color: var(--tv-text);
}

.pine-block__branch-panel + .pine-block__branch-panel {
  margin-top: 0;
}

.pine-block__branch-title {
  min-height: 3.1rem;
  margin-bottom: 0;
  padding: 0.55rem 0.85rem;
  color: var(--tv-text-muted);
}

.pine-block__branch-title > div {
  display: grid;
  gap: 0.15rem;
}

.pine-block__branch-title strong {
  color: var(--tv-text);
  font-size: 0.86rem;
}

.pine-block__branch-title span {
  font-size: 0.76rem;
}

.pine-block-list__add select {
  appearance: none;
  -webkit-appearance: none;
  background-image: none;
  border-style: dashed;
  color: var(--tv-text-muted);
}

@media (max-width: 860px) {
  .pine-block__header {
    grid-template-columns: 1fr;
  }

  .pine-block__actions {
    justify-content: flex-end;
  }
}
</style>
