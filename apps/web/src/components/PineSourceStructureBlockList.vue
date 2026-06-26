<script setup lang="ts">
import type { PineV6WorkflowDocument } from "@/contracts";

import {
  readSourceBlockField,
  sourceBlockEditableFields,
  type PineSourceBlock,
} from "../features/pineSourceStructureIndex";
import { PINE_V6_BLOCK_KINDS } from "../features/pineV6Workflow";

const props = defineProps<{
  nodes: PineSourceBlock[];
  selectedId: string;
  expandedId: string | null;
}>();

const emit = defineEmits<{
  "toggle-block": [block: PineSourceBlock];
  "add-block": [kind: string];
  "change-kind": [block: PineSourceBlock, kind: string];
  "delete-block": [block: PineSourceBlock];
  "duplicate-block": [block: PineSourceBlock];
  "move-block": [block: PineSourceBlock, direction: -1 | 1];
  "update-field": [block: PineSourceBlock, key: string, value: string];
}>();

function sourceBlockBadge(block: PineSourceBlock): string {
  switch (block.match.type) {
    case "strategy":
      return "策略";
    case "input":
      return "参数";
    case "instruction":
      return "指令";
    default:
      return "raw";
  }
}

function sourceBlockIsEditable(block: PineSourceBlock): boolean {
  return sourceBlockEditableFields(block).length > 0;
}

function sourceBlockFieldValue(block: PineSourceBlock, key: string): string {
  return readSourceBlockField(block, key);
}

function sourceBlockActionKind(block: PineSourceBlock): string {
  return block.match.type === "instruction" ? block.match.block.kind : "series_assign";
}

function sourceBlockDescription(block: PineSourceBlock): string {
  if (block.match.type === "strategy") {
    const title = sourceBlockFieldValue(block, "title").trim() || "Pine v6 策略";
    const initialCapital = sourceBlockFieldValue(block, "initialCapital").trim();
    const pyramiding = sourceBlockFieldValue(block, "pyramiding").trim();
    const pieces = [`声明策略 ${title}`];
    if (initialCapital !== "") {
      pieces.push(`初始资金 ${initialCapital}`);
    }
    if (pyramiding !== "") {
      pieces.push(`允许加仓 ${pyramiding} 次`);
    }
    return pieces.join("，");
  }
  if (block.match.type === "input") {
    const title = sourceBlockFieldValue(block, "title").trim() || block.match.input.name;
    const defaultValue = sourceBlockFieldValue(block, "defaultValue").trim() || "空值";
    return `输入参数 ${block.match.input.name} 使用 ${block.match.input.type} 类型，默认 ${defaultValue}，标题 ${title}`;
  }
  if (block.match.type === "instruction") {
    return describeInstructionSourceBlock(block.match.block);
  }
  if (block.kind === "version") {
    return `使用 ${block.detail || "Pine 版本声明"}`;
  }
  if (block.kind === "comment") {
    return `注释：${block.detail || block.raw.trim()}`;
  }
  if (block.match.type === "raw" && block.kind !== "raw") {
    return `${block.label}：${block.detail || block.raw.trim()}`;
  }
  return `保留原始 Pine：${block.raw.trim() || block.detail}`;
}

function describeInstructionSourceBlock(block: PineV6WorkflowDocument["blocks"][number]): string {
  switch (block.kind) {
    case "series_assign":
      return `${filledSourceParam(block, "name") || "变量"} 设为 ${filledSourceParam(block, "expression") || "表达式"}`;
    case "var_state":
      return `持久变量 ${filledSourceParam(block, "name") || "变量"} 初始为 ${filledSourceParam(block, "initial") || "空值"}`;
    case "if":
      return `当 ${filledSourceParam(block, "condition") || "条件"} 成立时执行分支`;
    case "request_security":
      return `读取 ${filledSourceParam(block, "symbol") || "标的"} ${filledSourceParam(block, "timeframe") || "周期"} 的 ${filledSourceParam(block, "expression") || "表达式"}`;
    case "array_op":
      return `${filledSourceParam(block, "name") || "数组"} 执行 ${filledSourceParam(block, "mode") || "数组操作"}`;
    case "strategy_entry":
      return `按 ${filledSourceParam(block, "direction") || "方向"} 开仓，订单 ${filledSourceParam(block, "id") || "未命名"}`;
    case "strategy_order":
      return `提交 ${filledSourceParam(block, "direction") || "方向"} 订单 ${filledSourceParam(block, "id") || "未命名"}`;
    case "strategy_exit":
      return `从 ${filledSourceParam(block, "from_entry") || "入场单"} 退出，退出单 ${filledSourceParam(block, "id") || "未命名"}`;
    case "strategy_close":
      return `平仓 ${filledSourceParam(block, "id") || "入场单"}${filledSourceParam(block, "when") ? `，条件 ${filledSourceParam(block, "when")}` : ""}`;
    case "strategy_close_all":
      return `全部平仓${filledSourceParam(block, "immediately") ? `，立即执行 ${filledSourceParam(block, "immediately")}` : ""}`;
    case "strategy_cancel":
      return `撤销订单 ${filledSourceParam(block, "id") || "订单 ID"}`;
    case "strategy_cancel_all":
      return "撤销全部未成交订单";
    case "strategy_risk_allow_entry_in":
      return `允许入场方向 ${filledSourceParam(block, "direction") || "strategy.direction.all"}`;
    case "strategy_risk_max_drawdown":
      return `最大回撤 ${filledSourceParam(block, "value") || "10"}，类型 ${filledSourceParam(block, "type") || "strategy.percent_of_equity"}`;
    case "strategy_risk_max_intraday_loss":
      return `日内最大亏损 ${filledSourceParam(block, "value") || "10"}，类型 ${filledSourceParam(block, "type") || "strategy.percent_of_equity"}`;
    case "strategy_risk_max_intraday_filled_orders":
      return `日内最多成交 ${filledSourceParam(block, "count") || "10"} 笔`;
    case "strategy_risk_max_position_size":
      return `最大持仓 ${filledSourceParam(block, "contracts") || "1"}`;
    case "strategy_risk_max_cons_loss_days":
      return `连续亏损 ${filledSourceParam(block, "count") || "3"} 天后限制交易`;
    case "plot":
      return `绘制 ${filledSourceParam(block, "series") || "序列"}${filledSourceParam(block, "title") ? `，标题 ${filledSourceParam(block, "title")}` : ""}`;
    case "alertcondition":
      return `当 ${filledSourceParam(block, "condition") || "条件"} 成立时触发提醒`;
    case "log":
      return `记录日志：${filledSourceParam(block, "message") || "消息"}`;
    default:
      return block.title || block.kind;
  }
}

function filledSourceParam(block: PineV6WorkflowDocument["blocks"][number], key: string): string {
  const value = block.params[key];
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return typeof value === "string" ? value.trim() : "";
}
</script>

<template>
  <div class="strategy-native-source-index__list strategy-native-block-list pine-block-list" data-testid="pine-source-structure-index">
    <section
      v-for="node in props.nodes"
      :key="node.id"
      class="strategy-native-source-node pine-block"
      :class="{
        'is-selected': props.selectedId === node.id,
        'is-raw': node.match.type === 'raw',
      }"
      :style="{ marginLeft: `${node.depth * 1.35}rem` }"
      :data-testid="`pine-source-node-${node.kind}`"
    >
      <button class="strategy-native-source-node__summary pine-block__summary" type="button" @click="emit('toggle-block', node)">
        <div class="pine-block__rail" />
        <span class="strategy-native-source-node__main pine-block__summary-text">
          <span class="pine-block__summary-heading">
            <span class="strategy-native-source-node__line">{{ node.lineRange.start }}</span>
            <span class="pine-block__kind-badge">{{ sourceBlockBadge(node) }}</span>
            <strong>{{ node.label }}</strong>
          </span>
          <span class="pine-block__summary-description">{{ sourceBlockDescription(node) }}</span>
        </span>
      </button>
      <div v-if="props.expandedId === node.id" class="pine-block__body">
        <div class="pine-block__header">
          <select
            class="pine-block__kind"
            :value="sourceBlockActionKind(node)"
            @change="emit('change-kind', node, ($event.target as HTMLSelectElement).value)"
          >
            <option
              v-for="option in PINE_V6_BLOCK_KINDS"
              :key="option.kind"
              :value="option.kind"
            >
              {{ option.label }}
            </option>
          </select>
          <div class="pine-block__actions">
            <button type="button" title="上移" @click.stop="emit('move-block', node, -1)">↑</button>
            <button type="button" title="下移" @click.stop="emit('move-block', node, 1)">↓</button>
            <button type="button" title="复制" @click.stop="emit('duplicate-block', node)">⧉</button>
            <button type="button" title="删除" @click.stop="emit('delete-block', node)">×</button>
          </div>
        </div>
        <div v-if="sourceBlockIsEditable(node)" class="strategy-native-source-node__fields pine-block__params">
          <label
            v-for="field in sourceBlockEditableFields(node)"
            :key="`${node.id}-${field.key}`"
          >
            <span>{{ field.label }}</span>
            <select
              v-if="field.kind === 'select'"
              :value="sourceBlockFieldValue(node, field.key)"
              @change="emit('update-field', node, field.key, ($event.target as HTMLSelectElement).value)"
            >
              <option v-for="option in field.options ?? []" :key="option" :value="option">{{ option }}</option>
            </select>
            <input
              v-else
              :value="sourceBlockFieldValue(node, field.key)"
              @input="emit('update-field', node, field.key, ($event.target as HTMLInputElement).value)"
            >
          </label>
        </div>
        <pre v-else class="strategy-native-source-node__raw">{{ node.raw }}</pre>
      </div>
    </section>
    <div class="pine-block-list__add">
      <select
        :value="PINE_V6_BLOCK_KINDS[0]?.kind ?? 'series_assign'"
        @change="emit('add-block', ($event.target as HTMLSelectElement).value)"
      >
        <option disabled value="">添加 Pine v6 块</option>
        <option v-for="option in PINE_V6_BLOCK_KINDS" :key="option.kind" :value="option.kind">
          + 新增{{ option.label }}
        </option>
      </select>
    </div>
  </div>
</template>

<style scoped>
.strategy-native-source-index__list {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  justify-items: stretch;
  gap: 0.65rem;
  width: 100%;
  min-width: 0;
}

.strategy-native-block-list {
  width: 100%;
  min-width: 0;
  justify-self: stretch;
}

.strategy-native-source-node {
  container-type: inline-size;
  flex-basis: 100% !important;
  justify-self: stretch;
  width: auto !important;
  inline-size: auto !important;
  min-width: 0;
  max-width: none !important;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  border-radius: 0.5rem;
  background: color-mix(in srgb, var(--tv-bg-surface) 96%, transparent);
  color: var(--tv-text);
}

.strategy-native-source-node:hover,
.strategy-native-source-node.is-selected {
  border-color: color-mix(in srgb, var(--tv-accent) 48%, var(--tv-border));
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--tv-accent) 16%, transparent);
}

.strategy-native-source-node__summary {
  width: 100% !important;
  inline-size: 100% !important;
  max-width: none !important;
  min-height: 2rem;
  display: grid !important;
  grid-template-columns: 0.35rem minmax(0, 1fr);
  gap: 0.65rem;
  align-items: stretch;
  border: 0;
  background: transparent;
  color: inherit;
  padding: 0;
  text-align: left;
  cursor: pointer;
}

.strategy-native-source-node.is-raw {
  border-style: dashed;
}

.pine-block__rail {
  width: 0.35rem;
  align-self: stretch;
  background: color-mix(in srgb, var(--tv-accent) 70%, #14b8a6);
}

.strategy-native-source-node__line {
  flex-shrink: 0;
  color: var(--tv-text-muted);
  font-family: "SFMono-Regular", Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  font-size: 0.8rem;
  font-weight: 800;
}

.strategy-native-source-node__main {
  min-width: 0;
  display: grid;
  grid-template-columns: minmax(11rem, max-content) minmax(12rem, 1fr);
  align-content: center;
  align-items: center;
  column-gap: 1rem;
  row-gap: 0.08rem;
  padding: 0.3rem 0.55rem 0.3rem 0;
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

.pine-block__kind-badge {
  flex-shrink: 0;
  border: 1px solid var(--tv-border);
  border-radius: 999px;
  padding: 0.12rem 0.45rem;
  color: var(--tv-text-muted);
  font-size: 0.7rem;
  font-weight: 800;
}

.pine-block__summary-description {
  text-align: right;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--tv-text-muted);
  font-size: 0.75rem;
  line-height: 1.35;
}

.pine-block__body {
  min-width: 0;
  display: grid;
  gap: 0.42rem;
  padding: 0.45rem 0.6rem;
}

.pine-block__header {
  display: grid;
  grid-template-columns: minmax(9rem, 12rem) auto;
  gap: 0.5rem;
  align-items: center;
  justify-content: space-between;
}

.pine-block__kind,
.pine-block-list__add select {
  width: 100%;
  min-width: 0;
  border: 1px solid var(--tv-border);
  border-radius: 0.45rem;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 0.32rem 0.45rem;
  font-size: 0.82rem;
  outline: none;
}

.pine-block__actions {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
}

.pine-block__actions button {
  width: 1.65rem;
  height: 1.65rem;
  border: 1px solid var(--tv-border);
  border-radius: 0.45rem;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 0;
  font-size: 0.85rem;
  font-weight: 700;
}

.pine-block-list__add select {
  justify-self: stretch;
  appearance: none;
  padding-right: 0.45rem;
}

.strategy-native-source-node__fields {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr));
  gap: 0.4rem;
}

.strategy-native-source-node__fields label {
  min-width: 0;
  display: grid;
  gap: 0.16rem;
  color: var(--tv-text-muted);
  font-size: 0.68rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.strategy-native-source-node__fields input,
.strategy-native-source-node__fields select {
  width: 100%;
  min-width: 0;
  border: 1px solid var(--tv-border);
  border-radius: 0.45rem;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  padding: 0.32rem 0.45rem;
  font-size: 0.82rem;
  line-height: 1.35;
  outline: none;
}

.strategy-native-source-node__raw {
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--tv-muted);
  font-family: "SFMono-Regular", Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  font-size: 0.72rem;
}

@container (max-width: 34rem) {
  .strategy-native-source-node__main {
    grid-template-columns: minmax(0, 1fr);
  }

  .pine-block__summary-description {
    text-align: left;
  }
}
</style>
