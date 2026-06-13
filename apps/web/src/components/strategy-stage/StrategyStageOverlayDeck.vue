<script setup lang="ts">
import type {
  StrategyDefinitionDocument,
  StrategyVisualNodeDocument,
} from "@/contracts";
import { computed, type Ref } from "vue";

import type {
  StrategyAuthoringTemplate,
  StrategyBlockKind,
} from "../../features/strategyVisualBuilder";
import {
  GET_TECHNICAL_INDICATOR_OPTIONS,
  INDICATOR_PERIOD_UNIT_OPTIONS,
  getPatternOptions,
  getTechnicalIndicatorConditionModeOptions,
  MOVING_AVERAGE_INDICATOR_OPTIONS,
  normalizeTechnicalIndicatorType,
  TECHNICAL_INDICATOR_OPTIONS,
} from "../../features/strategyVisualBuilderIndicatorBlock";
import {
  entryPositionPolicyLabel,
} from "../../features/strategyVisualBuilderScriptSupport";
import {
  STOP_LOSS_MODE_OPTIONS,
  STOP_LOSS_DIRECTION_OPTIONS,
  STOP_LOSS_TIME_UNIT_OPTIONS,
  STOP_LOSS_WINDOW_POLICY_OPTIONS,
} from "../../features/strategyVisualBuilderCatalog";

import "./strategyStageShared.css";

interface StrategyOverlayDeckBindings {
  definitionForm: Ref<StrategyDefinitionDocument>;
  selectedVisualNodeText: Ref<string>;
  selectedVisualNodeMessage: Ref<string>;
  selectedVisualNodeCode: Ref<string>;
  selectedVisualNodePeriod: Ref<string>;
  selectedIndicatorVariableName: Ref<string>;
  selectedIndicatorType: Ref<string>;
  selectedMovingAverageType: Ref<string>;
  selectedIndicatorPeriodUnit: Ref<string>;
  selectedIndicatorConditionMode: Ref<string>;
  selectedIndicatorOperator: Ref<string>;
  selectedIndicatorPatternType: Ref<string>;
  selectedIndicatorLookback: Ref<string>;
  selectedIndicatorPrimaryInputNodeId: Ref<string>;
  selectedIndicatorFastInputNodeId: Ref<string>;
  selectedIndicatorSlowInputNodeId: Ref<string>;
  selectedStopLossMode: Ref<string>;
  selectedStopLossDirection: Ref<string>;
  selectedStopLossTimeUnit: Ref<string>;
  selectedStopLossWindowPolicy: Ref<string>;
  selectedMacdFastPeriod: Ref<string>;
  selectedMacdSlowPeriod: Ref<string>;
  selectedMacdSignalPeriod: Ref<string>;
  selectedBollingerMultiplier: Ref<string>;
  selectedVisualNodeThreshold: Ref<string>;
  selectedPlaceOrderSide: Ref<string>;
  selectedPlaceOrderType: Ref<string>;
  selectedPlaceOrderEntryPositionPolicy: Ref<string>;
  selectedPlaceOrderQuantityMode: Ref<string>;
  selectedPlaceOrderQuantityValue: Ref<string>;
  selectedPlaceOrderLimitPrice: Ref<string>;
}

interface StrategyIndicatorGetterOption {
  value: string;
  label: string;
}

const props = defineProps<{
  bindings: StrategyOverlayDeckBindings;
  showTemplatesSection: boolean;
  showBasicInfoSection: boolean;
  showBlockDetailsSection: boolean;
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
  showsTechnicalIndicatorMacdInputs: boolean;
  showsMovingAverageTypeInput: boolean;
  showsIndicatorVariableNameInput: boolean;
  indicatorVariableNamePlaceholder: string;
  showsIndicatorPrimaryInputSelect: boolean;
  showsIndicatorFastInputSelect: boolean;
  showsIndicatorSlowInputSelect: boolean;
  indicatorGetterOptions: StrategyIndicatorGetterOption[];
  showsMultiplierInput: boolean;
  showsThresholdInput: boolean;
  showsConditionModeInput: boolean;
  showsIndicatorTypeInput: boolean;
  showsPatternTypeInput: boolean;
  showsLookbackInput: boolean;
  showsPlaceOrderInputs: boolean;
  showsPlaceOrderEntryPositionPolicyInput: boolean;
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
const selectedIndicatorVariableName = props.bindings.selectedIndicatorVariableName;
const selectedIndicatorType = props.bindings.selectedIndicatorType;
const selectedMovingAverageType = props.bindings.selectedMovingAverageType;
const selectedIndicatorPeriodUnit = props.bindings.selectedIndicatorPeriodUnit;
const selectedIndicatorConditionMode = props.bindings.selectedIndicatorConditionMode;
const selectedIndicatorOperator = props.bindings.selectedIndicatorOperator;
const selectedIndicatorPatternType = props.bindings.selectedIndicatorPatternType;
const selectedIndicatorLookback = props.bindings.selectedIndicatorLookback;
const selectedIndicatorPrimaryInputNodeId = props.bindings.selectedIndicatorPrimaryInputNodeId;
const selectedIndicatorFastInputNodeId = props.bindings.selectedIndicatorFastInputNodeId;
const selectedIndicatorSlowInputNodeId = props.bindings.selectedIndicatorSlowInputNodeId;
const selectedStopLossMode = props.bindings.selectedStopLossMode;
const selectedStopLossDirection = props.bindings.selectedStopLossDirection;
const selectedStopLossTimeUnit = props.bindings.selectedStopLossTimeUnit;
const selectedStopLossWindowPolicy = props.bindings.selectedStopLossWindowPolicy;
const selectedMacdFastPeriod = props.bindings.selectedMacdFastPeriod;
const selectedMacdSlowPeriod = props.bindings.selectedMacdSlowPeriod;
const selectedMacdSignalPeriod = props.bindings.selectedMacdSignalPeriod;
const selectedBollingerMultiplier = props.bindings.selectedBollingerMultiplier;
const selectedVisualNodeThreshold = props.bindings.selectedVisualNodeThreshold;
const selectedPlaceOrderSide = props.bindings.selectedPlaceOrderSide;
const selectedPlaceOrderType = props.bindings.selectedPlaceOrderType;
const selectedPlaceOrderEntryPositionPolicy = props.bindings.selectedPlaceOrderEntryPositionPolicy;
const selectedPlaceOrderQuantityMode = props.bindings.selectedPlaceOrderQuantityMode;
const selectedPlaceOrderQuantityValue = props.bindings.selectedPlaceOrderQuantityValue;
const selectedPlaceOrderLimitPrice = props.bindings.selectedPlaceOrderLimitPrice;

const placeOrderEntryPositionPolicyOptions = [
  { value: "sameDirection", label: entryPositionPolicyLabel("sameDirection") },
  { value: "flatOnly", label: entryPositionPolicyLabel("flatOnly") },
  { value: "allow", label: entryPositionPolicyLabel("allow") },
] as const;

const indicatorOptions = computed(() => (
  props.selectedVisualKind === "getTechnicalIndicator"
    ? GET_TECHNICAL_INDICATOR_OPTIONS
    : TECHNICAL_INDICATOR_OPTIONS
));
const movingAverageOptions = MOVING_AVERAGE_INDICATOR_OPTIONS;
const indicatorPeriodUnitOptions = INDICATOR_PERIOD_UNIT_OPTIONS;
const stopLossModeOptions = STOP_LOSS_MODE_OPTIONS;
const stopLossDirectionOptions = STOP_LOSS_DIRECTION_OPTIONS;
const stopLossTimeUnitOptions = STOP_LOSS_TIME_UNIT_OPTIONS;
const stopLossWindowPolicyOptions = STOP_LOSS_WINDOW_POLICY_OPTIONS;

const normalizedSelectedIndicatorType = computed(() =>
  normalizeTechnicalIndicatorType(selectedIndicatorType.value),
);

const indicatorConditionModeOptions = computed(() =>
  getTechnicalIndicatorConditionModeOptions(
    normalizedSelectedIndicatorType.value,
    props.selectedVisualKind === "technicalIndicator",
  ),
);

const indicatorPatternOptions = computed(() =>
  getPatternOptions(normalizedSelectedIndicatorType.value),
);

const stopLossThresholdLabel = computed(() => {
  switch (selectedStopLossMode.value) {
    case "takeProfit":
      return "止盈幅度（%）";
    case "trailingStop":
      return "追踪幅度（%）";
    default:
      return "止损幅度（%）";
  }
});

const isLegacyCodeBlockSelected = computed(() => props.selectedVisualKind === "codeBlock");

const codeInputLabel = computed(() =>
  props.selectedVisualKind === "pineSnippet"
    ? "Pine 片段"
    : isLegacyCodeBlockSelected.value
      ? "历史代码块（只读）"
      : "代码片段",
);

const codeInputPlaceholder = computed(() =>
  props.selectedVisualKind === "pineSnippet"
    ? "例如：plot(close)"
    : isLegacyCodeBlockSelected.value
      ? "旧 codeBlock 仅保留历史兼容；请改用 Pine 片段或标准 Pine 图块。"
      : "例如：const signal = ctx.kline.close > 520;",
);

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
            标的 / 周期在运行实例中绑定
          </div>
        </button>
      </div>
    </section>

    <section v-if="props.showBasicInfoSection" data-testid="strategy-basic-info-section" class="strategy-stack-card">
      <div class="strategy-stack-card__head">
        <div class="strategy-stage__section-title">基本信息</div>
      </div>

      <div class="mt-3 grid gap-3">
        <div class="rounded-3xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm leading-6 text-slate-600">
          标的、运行周期和券商账号会在创建运行实例时绑定。设计阶段只保存策略逻辑与元信息。
        </div>
        <label class="grid gap-1.5 text-sm text-slate-700">
          <span class="font-medium">定义 ID</span>
          <input
            :value="definitionForm.id || '自动生成 GUID'"
            class="rounded-2xl border border-slate-300 bg-slate-50 px-3 py-2.5 text-sm text-slate-900 outline-none"
            readonly
            type="text"
          />
          <span class="text-xs text-slate-500">新建策略时自动生成 GUID，保存后定义 ID 不可修改。</span>
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
          <div class="grid gap-1.5 text-sm text-slate-700">
            <span class="font-medium">版本</span>
            <div class="rounded-2xl border border-slate-300 bg-slate-50 px-3 py-2.5 text-sm text-slate-900">
              v{{ definitionForm.version || '0.1.0' }}
            </div>
            <span class="text-xs text-slate-500">版本由系统在保存时自动编号并强制递增。</span>
          </div>
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
        <label class="grid gap-1.5 text-sm text-slate-700">
          <span class="font-medium">说明</span>
          <textarea
            v-model="definitionForm.description"
            class="min-h-[88px] rounded-3xl border border-slate-300 px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-slate-500"
            placeholder="例如：说明这套策略的目标、触发条件和输出行为。"
          />
        </label>

        <div class="grid gap-2 pt-2">
          <div class="text-xs font-semibold uppercase tracking-[0.2em] text-slate-500">元信息</div>
          <div class="grid gap-3 text-sm text-slate-600 md:grid-cols-3">
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
        </div>
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

        <label v-if="props.showsIndicatorVariableNameInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">变量名</span>
          <input
            v-model="selectedIndicatorVariableName"
            data-testid="strategy-block-variable-name-input"
            :placeholder="props.indicatorVariableNamePlaceholder"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            type="text"
          />
        </label>

        <label v-if="props.showsIndicatorTypeInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">技术指标类型</span>
          <select
            v-model="selectedIndicatorType"
            data-testid="strategy-block-indicator-type-select"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
          >
            <option v-for="option in indicatorOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </label>

        <label v-if="props.showsMovingAverageTypeInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">均线指标类型</span>
          <select
            v-model="selectedMovingAverageType"
            data-testid="strategy-block-moving-average-type-select"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
          >
            <option v-for="option in movingAverageOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </label>

        <label v-if="props.showsConditionModeInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">判断类型</span>
          <select
            v-model="selectedIndicatorConditionMode"
            data-testid="strategy-block-indicator-condition-mode-select"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
          >
            <option v-for="option in indicatorConditionModeOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </label>

        <label v-if="props.showsIndicatorPrimaryInputSelect" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">引用变量</span>
          <select
            v-model="selectedIndicatorPrimaryInputNodeId"
            data-testid="strategy-block-indicator-input-primary-select"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
          >
            <option value="">请选择指标数据</option>
            <option v-for="option in props.indicatorGetterOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </label>

        <div v-if="props.showsIndicatorFastInputSelect || props.showsIndicatorSlowInputSelect" class="grid gap-3 md:grid-cols-2">
          <label v-if="props.showsIndicatorFastInputSelect" class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">快线变量</span>
            <select
              v-model="selectedIndicatorFastInputNodeId"
              data-testid="strategy-block-indicator-input-fast-select"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option value="">请选择快线数据</option>
              <option v-for="option in props.indicatorGetterOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>

          <label v-if="props.showsIndicatorSlowInputSelect" class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">慢线变量</span>
            <select
              v-model="selectedIndicatorSlowInputNodeId"
              data-testid="strategy-block-indicator-input-slow-select"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option value="">请选择慢线数据</option>
              <option v-for="option in props.indicatorGetterOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>
        </div>

        <label v-if="props.showsPeriodInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">{{ props.selectedVisualKind === 'stopLoss' ? '观察窗口' : '周期 / 窗口' }}</span>
          <input
            v-model="selectedVisualNodePeriod"
            data-testid="strategy-block-period-input"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            min="1"
            step="1"
            type="number"
          />
        </label>

        <label v-if="props.selectedVisualKind === 'getTechnicalIndicator' && normalizedSelectedIndicatorType === 'movingAverage'" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">时间单位</span>
          <select
            v-model="selectedIndicatorPeriodUnit"
            data-testid="strategy-block-indicator-period-unit-select"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
          >
            <option v-for="option in indicatorPeriodUnitOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </label>

        <div v-if="props.selectedVisualKind === 'stopLoss'" class="grid gap-3 md:grid-cols-2">
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">风险模式</span>
            <select
              v-model="selectedStopLossMode"
              data-testid="strategy-stop-loss-mode"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option v-for="option in stopLossModeOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>

          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">监控方向</span>
            <select
              v-model="selectedStopLossDirection"
              data-testid="strategy-stop-loss-direction"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option v-for="option in stopLossDirectionOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>

          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">时间单位</span>
            <select
              v-model="selectedStopLossTimeUnit"
              data-testid="strategy-stop-loss-time-unit"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option v-for="option in stopLossTimeUnitOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>

          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">窗口模式</span>
            <select
              v-model="selectedStopLossWindowPolicy"
              data-testid="strategy-stop-loss-window-policy"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option v-for="option in stopLossWindowPolicyOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>
        </div>

        <div v-if="props.showsMacdInputs || props.showsTechnicalIndicatorMacdInputs" class="grid gap-3 md:grid-cols-3">
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">{{ selectedIndicatorType === 'movingAverage' ? '快线' : '快线' }}</span>
            <input
              v-model="selectedMacdFastPeriod"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              min="1"
              step="1"
              type="number"
            />
          </label>
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">{{ selectedIndicatorType === 'kdj' ? 'M1' : '慢线' }}</span>
            <input
              v-model="selectedMacdSlowPeriod"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              min="1"
              step="1"
              type="number"
            />
          </label>
          <label v-if="selectedIndicatorType !== 'movingAverage'" class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">{{ selectedIndicatorType === 'kdj' ? 'M2' : '信号线' }}</span>
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

        <label v-if="props.showsPatternTypeInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">形态条件</span>
          <select
            v-model="selectedIndicatorPatternType"
            data-testid="strategy-block-indicator-pattern-select"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
          >
            <option v-for="option in indicatorPatternOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </label>

        <label v-if="props.showsThresholdInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">{{ props.selectedVisualKind === 'stopLoss' ? stopLossThresholdLabel : '阈值' }}</span>
          <input
            v-model="selectedVisualNodeThreshold"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            step="0.01"
            type="number"
          />
        </label>

        <label v-if="props.showsThresholdInput && props.selectedVisualKind === 'technicalIndicator'" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">比较方向</span>
          <select
            v-model="selectedIndicatorOperator"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
          >
            <option value=">">大于</option>
            <option value="<">小于</option>
          </select>
        </label>

        <label v-if="props.showsLookbackInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">背离观察窗口</span>
          <input
            v-model="selectedIndicatorLookback"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            min="1"
            step="1"
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
                data-testid="strategy-place-order-side"
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
              data-testid="strategy-place-order-quantity-mode"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option value="shares">固定股票数</option>
              <option value="amount">固定金额</option>
              <option value="equityPercent">账户权益百分比</option>
            </select>
          </label>

          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">
              <template v-if="selectedPlaceOrderQuantityMode === 'shares'">股票数量（股）</template>
              <template v-else-if="selectedPlaceOrderQuantityMode === 'amount'">金额</template>
              <template v-else-if="selectedPlaceOrderQuantityMode === 'equityPercent'">账户权益百分比（%）</template>
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
                生成 qty = 金额 / close，最终股数按运行时向下取整
              </template>
              <template v-else-if="selectedPlaceOrderQuantityMode === 'equityPercent'">
                对齐 Pine 的 strategy.percent_of_equity 语义，按账户权益估算 qty
              </template>
            </span>
          </label>

          <label v-if="props.showsPlaceOrderEntryPositionPolicyInput" class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">开仓持仓策略</span>
            <select
              v-model="selectedPlaceOrderEntryPositionPolicy"
              data-testid="strategy-place-order-entry-position-policy"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option v-for="option in placeOrderEntryPositionPolicyOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
            <span class="text-xs text-slate-400">
              仅对买入开多 / 卖出开空生效，用于控制已有仓位时是否继续同向开仓。
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
          <span class="font-medium">{{ codeInputLabel }}</span>
          <textarea
            v-model="selectedVisualNodeCode"
            class="min-h-[170px] rounded-3xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            :placeholder="codeInputPlaceholder"
            :readonly="isLegacyCodeBlockSelected"
          />
          <span v-if="isLegacyCodeBlockSelected" class="text-xs leading-5 text-slate-500">
            旧 codeBlock 仅可读取；新的无法标准化 Pine 会保留为 Pine 片段。
          </span>
        </label>

        <button class="strategy-btn strategy-btn--ghost strategy-btn--danger" type="button" @click="emit('delete-selected-node')">
          删除当前图块
        </button>
      </div>
    </section>
  </div>
</template>
