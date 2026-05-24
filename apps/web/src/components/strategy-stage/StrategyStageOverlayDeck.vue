<script setup lang="ts">
import type {
  StrategyDefinitionDocument,
  StrategyVisualNodeDocument,
} from "@jftrade/ui-contracts";
import type { Ref } from "vue";

import type {
  StrategyAuthoringTemplate,
  StrategyBlockKind,
} from "../../features/strategyVisualBuilder";

import "./strategyStageShared.css";

interface StrategyOverlayDeckBindings {
  definitionForm: Ref<StrategyDefinitionDocument>;
  selectedVisualNodeText: Ref<string>;
  selectedVisualNodeMessage: Ref<string>;
  selectedVisualNodeCode: Ref<string>;
  selectedVisualNodePeriod: Ref<string>;
  selectedMacdFastPeriod: Ref<string>;
  selectedMacdSlowPeriod: Ref<string>;
  selectedMacdSignalPeriod: Ref<string>;
  selectedBollingerMultiplier: Ref<string>;
  selectedVisualNodeThreshold: Ref<string>;
  selectedPlaceOrderSide: Ref<string>;
  selectedPlaceOrderType: Ref<string>;
  selectedPlaceOrderQuantityMode: Ref<string>;
  selectedPlaceOrderQuantityValue: Ref<string>;
  selectedPlaceOrderLimitPrice: Ref<string>;
}

const props = defineProps<{
  bindings: StrategyOverlayDeckBindings;
  showTemplatesSection: boolean;
  showBasicInfoSection: boolean;
  showBlockDetailsSection: boolean;
  showMetadataSection: boolean;
  activeStrategyTemplateMode: StrategyAuthoringTemplate["mode"] | null;
  strategyTemplates: StrategyAuthoringTemplate[];
  selectedStrategyTemplateId: string;
  selectedVisualNode: StrategyVisualNodeDocument | null;
  selectedVisualKind: StrategyBlockKind | null;
  selectedVisualBlockLabel: string;
  selectedVisualBlockDescription: string;
  showsCodeInput: boolean;
  showsPeriodInput: boolean;
  showsMacdInputs: boolean;
  showsMultiplierInput: boolean;
  showsThresholdInput: boolean;
  showsPlaceOrderInputs: boolean;
  showsPlaceOrderLimitPriceInput: boolean;
  createdAtText: string;
  updatedAtText: string;
}>();

const emit = defineEmits<{
  "select-template": [templateId: string];
  "delete-selected-node": [];
  "close-block-details": [];
}>();

const definitionForm = props.bindings.definitionForm;
const selectedVisualNodeText = props.bindings.selectedVisualNodeText;
const selectedVisualNodeMessage = props.bindings.selectedVisualNodeMessage;
const selectedVisualNodeCode = props.bindings.selectedVisualNodeCode;
const selectedVisualNodePeriod = props.bindings.selectedVisualNodePeriod;
const selectedMacdFastPeriod = props.bindings.selectedMacdFastPeriod;
const selectedMacdSlowPeriod = props.bindings.selectedMacdSlowPeriod;
const selectedMacdSignalPeriod = props.bindings.selectedMacdSignalPeriod;
const selectedBollingerMultiplier = props.bindings.selectedBollingerMultiplier;
const selectedVisualNodeThreshold = props.bindings.selectedVisualNodeThreshold;
const selectedPlaceOrderSide = props.bindings.selectedPlaceOrderSide;
const selectedPlaceOrderType = props.bindings.selectedPlaceOrderType;
const selectedPlaceOrderQuantityMode = props.bindings.selectedPlaceOrderQuantityMode;
const selectedPlaceOrderQuantityValue = props.bindings.selectedPlaceOrderQuantityValue;
const selectedPlaceOrderLimitPrice = props.bindings.selectedPlaceOrderLimitPrice;

function toAuthoringModeLabel(mode: StrategyAuthoringTemplate["mode"] | null): string {
  return mode === "visual" ? "图优先" : "代码优先";
}

function toTemplateTypeLabel(mode: StrategyAuthoringTemplate["mode"]): string {
  return mode === "visual" ? "可视化" : "代码";
}
</script>

<template>
  <div class="strategy-stage__panel-body">
    <section v-if="props.showTemplatesSection" data-testid="strategy-templates-section" class="strategy-stack-card">
      <div class="strategy-stack-card__head">
        <div class="strategy-stage__section-title">样板策略</div>
        <span v-if="props.activeStrategyTemplateMode !== null" class="strategy-page__pill">
          {{ toAuthoringModeLabel(props.activeStrategyTemplateMode) }}
        </span>
      </div>
      <div class="mt-3 grid gap-3">
        <button
          v-for="template in props.strategyTemplates"
          :key="template.id"
          :data-testid="`strategy-template-${template.id}`"
          class="strategy-template-card"
          :class="{ 'is-active': template.id === props.selectedStrategyTemplateId }"
          type="button"
          @click="emit('select-template', template.id)"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="text-base font-semibold">{{ template.label }}</div>
            <span class="strategy-page__pill">{{ toTemplateTypeLabel(template.mode) }}</span>
          </div>
          <div class="mt-2 text-sm leading-6 text-slate-500">
            {{ template.description }}
          </div>
          <div class="mt-3 text-xs uppercase tracking-[0.18em] text-slate-400">
            {{ template.defaultSymbol }} / {{ template.defaultInterval }}
          </div>
        </button>
      </div>
    </section>

    <section v-if="props.showBasicInfoSection" data-testid="strategy-basic-info-section" class="strategy-stack-card">
      <div class="strategy-stack-card__head">
        <div class="strategy-stage__section-title">基本信息</div>
      </div>

      <div class="mt-3 grid gap-3">
        <label class="grid gap-1.5 text-sm text-slate-700">
          <span class="font-medium">定义 ID</span>
          <input
            v-model="definitionForm.id"
            class="rounded-2xl border border-slate-300 px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-slate-500"
            placeholder="js-mean-revert"
            type="text"
          />
        </label>
        <label class="grid gap-1.5 text-sm text-slate-700">
          <span class="font-medium">策略名称</span>
          <input
            v-model="definitionForm.name"
            class="rounded-2xl border border-slate-300 px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-slate-500"
            placeholder="例如：双均线观察策略"
            type="text"
          />
        </label>
        <div class="grid gap-3 md:grid-cols-2">
          <label class="grid gap-1.5 text-sm text-slate-700">
            <span class="font-medium">版本</span>
            <input
              v-model="definitionForm.version"
              class="rounded-2xl border border-slate-300 px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-slate-500"
              placeholder="0.1.0"
              type="text"
            />
          </label>
          <label class="grid gap-1.5 text-sm text-slate-700">
            <span class="font-medium">运行时</span>
            <input
              v-model="definitionForm.runtime"
              class="rounded-2xl border border-slate-300 bg-slate-50 px-3 py-2.5 text-sm text-slate-900 outline-none"
              readonly
              type="text"
            />
          </label>
        </div>
        <div class="grid gap-3 md:grid-cols-2">
          <label class="grid gap-1.5 text-sm text-slate-700">
            <span class="font-medium">标的</span>
            <input
              v-model="definitionForm.symbol"
              class="rounded-2xl border border-slate-300 px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-slate-500"
              placeholder="00700"
              type="text"
            />
          </label>
          <label class="grid gap-1.5 text-sm text-slate-700">
            <span class="font-medium">周期</span>
            <input
              v-model="definitionForm.interval"
              class="rounded-2xl border border-slate-300 px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-slate-500"
              placeholder="1m"
              type="text"
            />
          </label>
        </div>
        <label class="grid gap-1.5 text-sm text-slate-700">
          <span class="font-medium">说明</span>
          <textarea
            v-model="definitionForm.description"
            class="min-h-[88px] rounded-3xl border border-slate-300 px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-slate-500"
            placeholder="例如：说明这套策略的目标、触发条件和输出行为。"
          />
        </label>
      </div>
    </section>

    <section
      v-if="props.showBlockDetailsSection"
      data-testid="strategy-block-inspector-card"
      class="strategy-stack-card"
    >
      <div class="strategy-stack-card__head">
        <div class="strategy-stage__section-title">
          图块详情 · {{ props.selectedVisualBlockLabel }}
        </div>
        <button
          class="strategy-stack-card__close"
          data-testid="close-block-details"
          type="button"
          aria-label="关闭图块详情"
          @click="emit('close-block-details')"
        >
          <svg aria-hidden="true" fill="none" viewBox="0 0 16 16" class="strategy-stack-card__close-icon">
            <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-linecap="round" stroke-width="1.5" />
          </svg>
        </button>
      </div>

      <div class="mt-3 grid gap-4">
        <div class="text-sm text-slate-500">
          {{ props.selectedVisualBlockDescription }}
        </div>

        <label class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">块标题</span>
          <input
            v-model="selectedVisualNodeText"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            type="text"
          />
        </label>

        <label v-if="props.showsPeriodInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">周期 / 窗口</span>
          <input
            v-model="selectedVisualNodePeriod"
            data-testid="strategy-block-period-input"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            min="1"
            step="1"
            type="number"
          />
        </label>

        <div v-if="props.showsMacdInputs" class="grid gap-3 md:grid-cols-3">
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">快线</span>
            <input
              v-model="selectedMacdFastPeriod"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              min="1"
              step="1"
              type="number"
            />
          </label>
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">慢线</span>
            <input
              v-model="selectedMacdSlowPeriod"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              min="1"
              step="1"
              type="number"
            />
          </label>
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">信号线</span>
            <input
              v-model="selectedMacdSignalPeriod"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              min="1"
              step="1"
              type="number"
            />
          </label>
        </div>

        <label v-if="props.showsMultiplierInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">乘数</span>
          <input
            v-model="selectedBollingerMultiplier"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            min="0.1"
            step="0.1"
            type="number"
          />
        </label>

        <label v-if="props.showsThresholdInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">阈值</span>
          <input
            v-model="selectedVisualNodeThreshold"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            step="0.01"
            type="number"
          />
        </label>

        <!-- ── 下单图块专属属性 ── -->
        <template v-if="props.showsPlaceOrderInputs">
          <div class="grid gap-3 md:grid-cols-2">
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">方向</span>
              <select
                v-model="selectedPlaceOrderSide"
                class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              >
                <option value="BUY">买入开多</option>
                <option value="SELL">卖出平多</option>
                <option value="SELL_SHORT">卖出开空</option>
                <option value="BUY_COVER">买入平空</option>
              </select>
            </label>
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">订单类型</span>
              <select
                v-model="selectedPlaceOrderType"
                class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              >
                <option value="MARKET">市价单</option>
                <option value="LIMIT">限价单</option>
              </select>
            </label>
          </div>

          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">数量模式</span>
            <select
              v-model="selectedPlaceOrderQuantityMode"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option value="shares">固定股票数</option>
              <option value="amount">固定金额</option>
              <option value="positionPercent">账户仓位百分比</option>
              <option value="cashPercent">用户可用现金百分比</option>
            </select>
          </label>

          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">
              <template v-if="selectedPlaceOrderQuantityMode === 'shares'">股票数量（股）</template>
              <template v-else-if="selectedPlaceOrderQuantityMode === 'amount'">金额</template>
              <template v-else-if="selectedPlaceOrderQuantityMode === 'positionPercent'">仓位百分比（%）</template>
              <template v-else-if="selectedPlaceOrderQuantityMode === 'cashPercent'">现金百分比（%）</template>
            </span>
            <input
              v-model="selectedPlaceOrderQuantityValue"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              min="0"
              :step="selectedPlaceOrderQuantityMode === 'shares' ? '1' : '0.01'"
              type="number"
            />
            <span class="text-xs text-slate-400">
              <template v-if="selectedPlaceOrderQuantityMode === 'amount'">
                股票数量以实际为准，不超过输入金额
              </template>
              <template v-else-if="selectedPlaceOrderQuantityMode === 'positionPercent'">
                基于当前账户持仓市值计算目标股数
              </template>
              <template v-else-if="selectedPlaceOrderQuantityMode === 'cashPercent'">
                回测使用策略当时的现金，实盘使用账户可用资金
              </template>
            </span>
          </label>

          <label v-if="props.showsPlaceOrderLimitPriceInput" class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">限价</span>
            <input
              v-model="selectedPlaceOrderLimitPrice"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              min="0"
              step="0.01"
              type="number"
            />
          </label>
        </template>

        <label
          v-if="props.selectedVisualKind === 'log' || props.selectedVisualKind === 'notify'"
          class="grid gap-2 text-sm text-slate-700"
        >
          <span class="font-medium">消息内容</span>
          <textarea
            v-model="selectedVisualNodeMessage"
            class="min-h-[110px] rounded-3xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            placeholder="例如：收盘价更新: ${ctx.kline.close}"
          />
        </label>

        <label v-if="props.showsCodeInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">代码片段</span>
          <textarea
            v-model="selectedVisualNodeCode"
            class="min-h-[170px] rounded-3xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            placeholder="例如：const signal = ctx.kline.close > 520;"
          />
        </label>

        <button class="strategy-btn strategy-btn--ghost strategy-btn--danger" type="button" @click="emit('delete-selected-node')">
          删除当前图块
        </button>
      </div>
    </section>

    <section v-if="props.showMetadataSection" data-testid="strategy-metadata-section" class="strategy-stack-card">
      <div class="strategy-stack-card__head">
        <div class="strategy-stage__section-title">元信息</div>
      </div>

      <div class="mt-3 grid gap-3 text-sm text-slate-600">
        <div class="strategy-meta-card">
          <div class="text-xs uppercase tracking-[0.2em] text-slate-500">定义 ID</div>
          <div class="mt-2 font-medium text-slate-900">{{ definitionForm.id || '自动生成' }}</div>
        </div>
        <div class="strategy-meta-card">
          <div class="text-xs uppercase tracking-[0.2em] text-slate-500">创建时间</div>
          <div class="mt-2 font-medium text-slate-900">{{ props.createdAtText }}</div>
        </div>
        <div class="strategy-meta-card">
          <div class="text-xs uppercase tracking-[0.2em] text-slate-500">更新时间</div>
          <div class="mt-2 font-medium text-slate-900">{{ props.updatedAtText }}</div>
        </div>
      </div>
    </section>
  </div>
</template>