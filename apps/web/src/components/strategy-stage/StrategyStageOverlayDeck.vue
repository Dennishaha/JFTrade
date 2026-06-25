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
import type { VisualExpressionReference } from "../../features/strategyVisualBuilderExpressions";
import {
  GET_TECHNICAL_INDICATOR_OPTIONS,
  INDICATOR_TIMEFRAME_OPTIONS,
  getPatternOptions,
  getTechnicalIndicatorConditionModeOptions,
  MOVING_AVERAGE_INDICATOR_OPTIONS,
  normalizeTechnicalIndicatorType,
  TECHNICAL_INDICATOR_OPTIONS,
} from "../../features/strategyVisualBuilderIndicatorBlock";
import {
  entryPositionPolicyLabel,
} from "../../features/strategyVisualBuilderScriptSupport";
import type { PineBlockSupportAssessment } from "../../features/strategyVisualBuilderSupport";
import {
  STOP_LOSS_MODE_OPTIONS,
  STOP_LOSS_DIRECTION_OPTIONS,
  STOP_LOSS_TIME_UNIT_OPTIONS,
  STOP_LOSS_WINDOW_POLICY_OPTIONS,
  COLLECTION_STAT_FUNCTION_OPTIONS,
  SERIES_CONDITION_MODE_OPTIONS,
  SERIES_SOURCE_OPTIONS,
  STRATEGY_INPUT_TYPE_OPTIONS,
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
  selectedIndicatorTimeframe: Ref<string>;
  selectedIndicatorConditionMode: Ref<string>;
  selectedIndicatorOperator: Ref<string>;
  selectedIndicatorPatternType: Ref<string>;
  selectedIndicatorLookback: Ref<string>;
  selectedSeriesConditionMode: Ref<string>;
  selectedSeriesConditionSource: Ref<string>;
  selectedSeriesConditionOperator: Ref<string>;
  selectedSeriesConditionThreshold: Ref<string>;
  selectedSeriesConditionLength: Ref<string>;
  selectedSeriesConditionEventSource: Ref<string>;
  selectedSeriesConditionEventOperator: Ref<string>;
  selectedSeriesConditionEventThreshold: Ref<string>;
  selectedSeriesConditionValueSource: Ref<string>;
  selectedSeriesConditionOccurrence: Ref<string>;
  selectedIndicatorPrimaryInputNodeId: Ref<string>;
  selectedIndicatorFastInputNodeId: Ref<string>;
  selectedIndicatorSlowInputNodeId: Ref<string>;
  selectedStopLossMode: Ref<string>;
  selectedStopLossDirection: Ref<string>;
  selectedStopLossTimeUnit: Ref<string>;
  selectedStopLossWindowPolicy: Ref<string>;
  selectedStopLossTakeProfitPercentage: Ref<string>;
  selectedMacdFastPeriod: Ref<string>;
  selectedMacdSlowPeriod: Ref<string>;
  selectedMacdSignalPeriod: Ref<string>;
  selectedBollingerMultiplier: Ref<string>;
  selectedIndicatorSource: Ref<string>;
  selectedIndicatorAdxSmoothing: Ref<string>;
  selectedIndicatorFactor: Ref<string>;
  selectedIndicatorSarStart: Ref<string>;
  selectedIndicatorSarIncrement: Ref<string>;
  selectedIndicatorSarMaximum: Ref<string>;
  selectedIndicatorOffset: Ref<string>;
  selectedIndicatorSigma: Ref<string>;
  selectedIndicatorLeftBars: Ref<string>;
  selectedIndicatorRightBars: Ref<string>;
  selectedVisualNodeThreshold: Ref<string>;
  selectedPlaceOrderAction: Ref<string>;
  selectedPlaceOrderId: Ref<string>;
  selectedPlaceOrderSide: Ref<string>;
  selectedPlaceOrderType: Ref<string>;
  selectedPlaceOrderEntryPositionPolicy: Ref<string>;
  selectedPlaceOrderQuantityMode: Ref<string>;
  selectedPlaceOrderQuantityValue: Ref<string>;
  selectedPlaceOrderLimitPrice: Ref<string>;
  selectedPlaceOrderStopPrice: Ref<string>;
  selectedPlaceOrderRiskAllowedDirection: Ref<string>;
  selectedExpressionSlot: Ref<string>;
  selectedExpressionReference: Ref<string>;
  selectedExpressionField: Ref<string>;
  selectedExpressionOperator: Ref<string>;
  selectedExpressionFunction: Ref<string>;
  selectedExpressionLiteral: Ref<string>;
  selectedExpressionHistoryOffset: Ref<string>;
  selectedAdvancedVariableName: Ref<string>;
  selectedAdvancedMode: Ref<string>;
  selectedAdvancedDefaultValue: Ref<string>;
  selectedAdvancedTimeframe: Ref<string>;
  selectedAdvancedSource: Ref<string>;
  selectedAdvancedSecondarySource: Ref<string>;
  selectedAdvancedTertiarySource: Ref<string>;
  selectedAdvancedNumber: Ref<string>;
  selectedAdvancedExpression: Ref<string>;
  selectedAdvancedOption: Ref<string>;
  selectedAdvancedReference: Ref<string>;
  selectedTimeFilterMode: Ref<string>;
  selectedTimeFilterStartHour: Ref<string>;
  selectedTimeFilterStartMinute: Ref<string>;
  selectedTimeFilterEndHour: Ref<string>;
  selectedTimeFilterEndMinute: Ref<string>;
  selectedTimeFilterDayOfWeek: Ref<string>;
  selectedSessionFilterScope: Ref<string>;
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
  selectedVisualSupport: PineBlockSupportAssessment | null;
  showsCodeInput: boolean;
  showsVisualExpressionInputs: boolean;
  showsAdvancedPineBlockInputs: boolean;
  showsTimeFilterInputs: boolean;
  showsSessionFilterInputs: boolean;
  expressionSlotOptions: Array<StrategyIndicatorGetterOption>;
  expressionReferenceOptions: Array<StrategyIndicatorGetterOption | VisualExpressionReference>;
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
  showsIndicatorSourceInput: boolean;
  showsIndicatorTimeframeInput: boolean;
  showsIndicatorAdxSmoothingInput: boolean;
  showsIndicatorFactorInput: boolean;
  showsIndicatorSarInputs: boolean;
  showsIndicatorOffsetInput: boolean;
  showsIndicatorSigmaInput: boolean;
  showsIndicatorPivotBarsInput: boolean;
  showsThresholdInput: boolean;
  showsConditionModeInput: boolean;
  showsSeriesConditionInputs: boolean;
  showsIndicatorTypeInput: boolean;
  showsPatternTypeInput: boolean;
  showsLookbackInput: boolean;
  showsPlaceOrderInputs: boolean;
  showsPlaceOrderEntryPositionPolicyInput: boolean;
  showsPlaceOrderLimitPriceInput: boolean;
  showsPlaceOrderStopPriceInput: boolean;
  showsPlaceOrderQuantityInputs: boolean;
  showsPlaceOrderSideInput: boolean;
  showsPlaceOrderTargetIdInput: boolean;
  showsPlaceOrderRiskDirectionInput: boolean;
  showsStopLossTakeProfitPercentageInput: boolean;
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
const selectedIndicatorTimeframe = props.bindings.selectedIndicatorTimeframe;
const selectedIndicatorConditionMode = props.bindings.selectedIndicatorConditionMode;
const selectedIndicatorOperator = props.bindings.selectedIndicatorOperator;
const selectedIndicatorPatternType = props.bindings.selectedIndicatorPatternType;
const selectedIndicatorLookback = props.bindings.selectedIndicatorLookback;
const selectedSeriesConditionMode = props.bindings.selectedSeriesConditionMode;
const selectedSeriesConditionSource = props.bindings.selectedSeriesConditionSource;
const selectedSeriesConditionOperator = props.bindings.selectedSeriesConditionOperator;
const selectedSeriesConditionThreshold = props.bindings.selectedSeriesConditionThreshold;
const selectedSeriesConditionLength = props.bindings.selectedSeriesConditionLength;
const selectedSeriesConditionEventSource = props.bindings.selectedSeriesConditionEventSource;
const selectedSeriesConditionEventOperator = props.bindings.selectedSeriesConditionEventOperator;
const selectedSeriesConditionEventThreshold = props.bindings.selectedSeriesConditionEventThreshold;
const selectedSeriesConditionValueSource = props.bindings.selectedSeriesConditionValueSource;
const selectedSeriesConditionOccurrence = props.bindings.selectedSeriesConditionOccurrence;
const selectedIndicatorPrimaryInputNodeId = props.bindings.selectedIndicatorPrimaryInputNodeId;
const selectedIndicatorFastInputNodeId = props.bindings.selectedIndicatorFastInputNodeId;
const selectedIndicatorSlowInputNodeId = props.bindings.selectedIndicatorSlowInputNodeId;
const selectedStopLossMode = props.bindings.selectedStopLossMode;
const selectedStopLossDirection = props.bindings.selectedStopLossDirection;
const selectedStopLossTimeUnit = props.bindings.selectedStopLossTimeUnit;
const selectedStopLossWindowPolicy = props.bindings.selectedStopLossWindowPolicy;
const selectedStopLossTakeProfitPercentage = props.bindings.selectedStopLossTakeProfitPercentage;
const selectedMacdFastPeriod = props.bindings.selectedMacdFastPeriod;
const selectedMacdSlowPeriod = props.bindings.selectedMacdSlowPeriod;
const selectedMacdSignalPeriod = props.bindings.selectedMacdSignalPeriod;
const selectedBollingerMultiplier = props.bindings.selectedBollingerMultiplier;
const selectedIndicatorSource = props.bindings.selectedIndicatorSource;
const selectedIndicatorAdxSmoothing = props.bindings.selectedIndicatorAdxSmoothing;
const selectedIndicatorFactor = props.bindings.selectedIndicatorFactor;
const selectedIndicatorSarStart = props.bindings.selectedIndicatorSarStart;
const selectedIndicatorSarIncrement = props.bindings.selectedIndicatorSarIncrement;
const selectedIndicatorSarMaximum = props.bindings.selectedIndicatorSarMaximum;
const selectedIndicatorOffset = props.bindings.selectedIndicatorOffset;
const selectedIndicatorSigma = props.bindings.selectedIndicatorSigma;
const selectedIndicatorLeftBars = props.bindings.selectedIndicatorLeftBars;
const selectedIndicatorRightBars = props.bindings.selectedIndicatorRightBars;
const selectedVisualNodeThreshold = props.bindings.selectedVisualNodeThreshold;
const selectedPlaceOrderAction = props.bindings.selectedPlaceOrderAction;
const selectedPlaceOrderId = props.bindings.selectedPlaceOrderId;
const selectedPlaceOrderSide = props.bindings.selectedPlaceOrderSide;
const selectedPlaceOrderType = props.bindings.selectedPlaceOrderType;
const selectedPlaceOrderEntryPositionPolicy = props.bindings.selectedPlaceOrderEntryPositionPolicy;
const selectedPlaceOrderQuantityMode = props.bindings.selectedPlaceOrderQuantityMode;
const selectedPlaceOrderQuantityValue = props.bindings.selectedPlaceOrderQuantityValue;
const selectedPlaceOrderLimitPrice = props.bindings.selectedPlaceOrderLimitPrice;
const selectedPlaceOrderStopPrice = props.bindings.selectedPlaceOrderStopPrice;
const selectedPlaceOrderRiskAllowedDirection = props.bindings.selectedPlaceOrderRiskAllowedDirection;
const selectedExpressionSlot = props.bindings.selectedExpressionSlot;
const selectedExpressionReference = props.bindings.selectedExpressionReference;
const selectedExpressionField = props.bindings.selectedExpressionField;
const selectedExpressionOperator = props.bindings.selectedExpressionOperator;
const selectedExpressionFunction = props.bindings.selectedExpressionFunction;
const selectedExpressionLiteral = props.bindings.selectedExpressionLiteral;
const selectedExpressionHistoryOffset = props.bindings.selectedExpressionHistoryOffset;
const selectedAdvancedVariableName = props.bindings.selectedAdvancedVariableName;
const selectedAdvancedMode = props.bindings.selectedAdvancedMode;
const selectedAdvancedDefaultValue = props.bindings.selectedAdvancedDefaultValue;
const selectedAdvancedTimeframe = props.bindings.selectedAdvancedTimeframe;
const selectedAdvancedSource = props.bindings.selectedAdvancedSource;
const selectedAdvancedSecondarySource = props.bindings.selectedAdvancedSecondarySource;
const selectedAdvancedTertiarySource = props.bindings.selectedAdvancedTertiarySource;
const selectedAdvancedNumber = props.bindings.selectedAdvancedNumber;
const selectedAdvancedExpression = props.bindings.selectedAdvancedExpression;
const selectedAdvancedOption = props.bindings.selectedAdvancedOption;
const selectedAdvancedReference = props.bindings.selectedAdvancedReference;
const selectedTimeFilterMode = props.bindings.selectedTimeFilterMode;
const selectedTimeFilterStartHour = props.bindings.selectedTimeFilterStartHour;
const selectedTimeFilterStartMinute = props.bindings.selectedTimeFilterStartMinute;
const selectedTimeFilterEndHour = props.bindings.selectedTimeFilterEndHour;
const selectedTimeFilterEndMinute = props.bindings.selectedTimeFilterEndMinute;
const selectedTimeFilterDayOfWeek = props.bindings.selectedTimeFilterDayOfWeek;
const selectedSessionFilterScope = props.bindings.selectedSessionFilterScope;

const placeOrderEntryPositionPolicyOptions = [
  { value: "sameDirection", label: entryPositionPolicyLabel("sameDirection") },
  { value: "flatOnly", label: entryPositionPolicyLabel("flatOnly") },
  { value: "allow", label: entryPositionPolicyLabel("allow") },
] as const;
const placeOrderActionOptions = [
  { value: "entry", label: "开仓 entry" },
  { value: "order", label: "净额 order" },
  { value: "close", label: "平仓 close" },
  { value: "closeAll", label: "全部平仓 close_all" },
  { value: "cancel", label: "撤销指定订单" },
  { value: "cancelAll", label: "撤销全部订单" },
  { value: "riskAllowEntryIn", label: "允许开仓方向" },
] as const;
const riskAllowedDirectionOptions = [
  { value: "all", label: "允许多空" },
  { value: "long", label: "仅允许多头" },
  { value: "short", label: "仅允许空头" },
] as const;

const indicatorOptions = computed(() => (
  props.selectedVisualKind === "getTechnicalIndicator"
    ? GET_TECHNICAL_INDICATOR_OPTIONS
    : TECHNICAL_INDICATOR_OPTIONS
));
const movingAverageOptions = MOVING_AVERAGE_INDICATOR_OPTIONS;
const indicatorTimeframeOptions = INDICATOR_TIMEFRAME_OPTIONS;
const seriesConditionModeOptions = SERIES_CONDITION_MODE_OPTIONS;
const seriesSourceOptions = SERIES_SOURCE_OPTIONS;
const indicatorSourceOptions = [
  { value: "open", label: "Open" },
  { value: "high", label: "High" },
  { value: "low", label: "Low" },
  { value: "close", label: "Close" },
  { value: "volume", label: "Volume" },
  { value: "hl2", label: "HL2" },
  { value: "hlc3", label: "HLC3" },
  { value: "ohlc4", label: "OHLC4" },
] as const;
const stopLossModeOptions = STOP_LOSS_MODE_OPTIONS;
const stopLossDirectionOptions = STOP_LOSS_DIRECTION_OPTIONS;
const stopLossTimeUnitOptions = STOP_LOSS_TIME_UNIT_OPTIONS;
const stopLossWindowPolicyOptions = STOP_LOSS_WINDOW_POLICY_OPTIONS;
const strategyInputTypeOptions = STRATEGY_INPUT_TYPE_OPTIONS;
const collectionStatFunctionOptions = COLLECTION_STAT_FUNCTION_OPTIONS;
const derivedSeriesModeOptions = [
  { value: "history", label: "历史引用" },
  { value: "nz", label: "NZ 兜底" },
  { value: "math", label: "Math 函数" },
  { value: "arithmetic", label: "四则表达式" },
  { value: "cross", label: "交叉判断" },
] as const;
const mtfExpressionTypeOptions = [
  { value: "source", label: "Source" },
  { value: "history", label: "历史引用" },
  { value: "indicator", label: "指标表达式" },
] as const;
const stateValueTypeOptions = [
  { value: "number", label: "数字" },
  { value: "bool", label: "布尔" },
  { value: "string", label: "字符串" },
] as const;
const derivedOptionOptions = [
  { value: "min", label: "math.min" },
  { value: "max", label: "math.max" },
  { value: "abs", label: "math.abs" },
  { value: "round", label: "math.round" },
  { value: "floor", label: "math.floor" },
  { value: "ceil", label: "math.ceil" },
  { value: "crossover", label: "ta.crossover" },
  { value: "crossunder", label: "ta.crossunder" },
  { value: "cross", label: "ta.cross" },
  { value: "+", label: "+" },
  { value: "-", label: "-" },
  { value: "*", label: "*" },
  { value: "/", label: "/" },
] as const;
const mtfFieldOptions = [
  { value: "", label: "不选择字段" },
  { value: "histogram", label: "MACD histogram" },
  { value: "direction", label: "Supertrend direction" },
  { value: "upper", label: "Upper" },
  { value: "lower", label: "Lower" },
] as const;
const expressionOperatorOptions = [
  { value: ">", label: ">" },
  { value: "<", label: "<" },
  { value: "+", label: "+" },
  { value: "-", label: "-" },
  { value: "*", label: "*" },
  { value: "/", label: "/" },
] as const;
const expressionFunctionOptions = [
  { value: "math.max", label: "math.max" },
  { value: "math.min", label: "math.min" },
  { value: "math.abs", label: "math.abs" },
  { value: "math.round", label: "math.round" },
  { value: "nz", label: "nz" },
  { value: "ta.crossover", label: "ta.crossover" },
  { value: "ta.crossunder", label: "ta.crossunder" },
  { value: "ta.cross", label: "ta.cross" },
] as const;
const timeFilterModeOptions = [
  { value: "between", label: "时间区间" },
  { value: "after", label: "不早于" },
  { value: "before", label: "早于" },
  { value: "dayOfWeek", label: "星期" },
] as const;
const dayOfWeekOptions = [
  { value: "1", label: "周日" },
  { value: "2", label: "周一" },
  { value: "3", label: "周二" },
  { value: "4", label: "周三" },
  { value: "5", label: "周四" },
  { value: "6", label: "周五" },
  { value: "7", label: "周六" },
] as const;
const sessionScopeOptions = [
  { value: "market", label: "常规交易时段" },
  { value: "premarket", label: "盘前" },
  { value: "postmarket", label: "盘后" },
] as const;

const expressionFieldOptions = computed(() => {
  const selected = props.expressionReferenceOptions.find((option) => option.value === selectedExpressionReference.value);
  return "fields" in (selected ?? {}) ? (selected as VisualExpressionReference).fields ?? [] : [];
});

const normalizedSelectedIndicatorType = computed(() =>
  normalizeTechnicalIndicatorType(selectedIndicatorType.value),
);

const indicatorConditionModeOptions = computed(() =>
  getTechnicalIndicatorConditionModeOptions(
    normalizedSelectedIndicatorType.value,
    false,
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
    case "bracketExit":
      return "止损幅度（%）";
    default:
      return "止损幅度（%）";
  }
});

const codeInputLabel = computed(() =>
  props.selectedVisualKind === "pineSnippet"
    ? "Pine 片段"
    : "代码片段",
);

const codeInputPlaceholder = computed(() =>
  props.selectedVisualKind === "pineSnippet"
    ? "例如：plot(close)"
    : "例如：const signal = ctx.kline.close > 520;",
);

const advancedModeOptions = computed(() => {
  switch (props.selectedVisualKind) {
    case "strategyInput":
      return strategyInputTypeOptions;
    case "derivedSeries":
      return derivedSeriesModeOptions;
    case "mtfSeries":
      return mtfExpressionTypeOptions;
    case "stateVariable":
      return stateValueTypeOptions;
    case "collectionStat":
      return collectionStatFunctionOptions;
    default:
      return [];
  }
});

const advancedModeLabel = computed(() => {
  switch (props.selectedVisualKind) {
    case "strategyInput":
      return "参数类型";
    case "derivedSeries":
      return "派生模式";
    case "mtfSeries":
      return "MTF 表达式";
    case "stateVariable":
      return "状态类型";
    case "collectionStat":
      return "统计函数";
    default:
      return "模式";
  }
});

const advancedExpressionLabel = computed(() => {
  switch (props.selectedVisualKind) {
    case "strategyInput":
      return "参数标题";
    case "stateUpdate":
      return "更新表达式";
    case "mtfSeries":
      return "指标表达式";
    case "derivedSeries":
      return "左侧表达式";
    default:
      return "表达式";
  }
});

const advancedNumberLabel = computed(() =>
  props.selectedVisualKind === "collectionStat" ? "百分位" : "历史偏移 / 数值",
);

function toAuthoringModeLabel(mode: StrategyAuthoringTemplate["mode"] | null): string {
  return mode === "visual" ? "图优先" : "代码优先";
}

function toTemplateTypeLabel(mode: StrategyAuthoringTemplate["mode"]): string {
  return mode === "visual" ? "可视化" : "代码";
}

function supportToneClass(status: PineBlockSupportAssessment["status"]): string {
  return `strategy-block-support--${status}`;
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
        <div
          v-if="props.selectedVisualSupport !== null"
          class="strategy-block-support"
          :class="supportToneClass(props.selectedVisualSupport.status)"
          data-testid="strategy-block-support-status"
        >
          <span class="strategy-block-support__label">{{ props.selectedVisualSupport.label }}</span>
          <span class="strategy-block-support__message">{{ props.selectedVisualSupport.message }}</span>
        </div>

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

        <template v-if="props.showsSeriesConditionInputs">
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">序列条件类型</span>
            <select
              v-model="selectedSeriesConditionMode"
              data-testid="strategy-series-condition-mode"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option v-for="option in seriesConditionModeOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>

          <div v-if="selectedSeriesConditionMode === 'compare' || selectedSeriesConditionMode === 'rising' || selectedSeriesConditionMode === 'falling'" class="grid gap-3 md:grid-cols-3">
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">数据源</span>
              <select v-model="selectedSeriesConditionSource" data-testid="strategy-series-condition-source" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500">
                <option v-for="option in seriesSourceOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>
            <label v-if="selectedSeriesConditionMode === 'compare'" class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">比较</span>
              <select v-model="selectedSeriesConditionOperator" data-testid="strategy-series-condition-operator" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500">
                <option value=">">&gt;</option>
                <option value="<">&lt;</option>
              </select>
            </label>
            <label v-if="selectedSeriesConditionMode === 'compare'" class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">阈值</span>
              <input v-model="selectedSeriesConditionThreshold" data-testid="strategy-series-condition-threshold" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" step="0.01" type="number" />
            </label>
            <label v-if="selectedSeriesConditionMode === 'rising' || selectedSeriesConditionMode === 'falling'" class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">窗口长度</span>
              <input v-model="selectedSeriesConditionLength" data-testid="strategy-series-condition-length" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" min="1" step="1" type="number" />
            </label>
          </div>

          <div v-if="selectedSeriesConditionMode === 'barssince' || selectedSeriesConditionMode === 'valuewhen'" class="grid gap-3 md:grid-cols-3">
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">事件源</span>
              <select v-model="selectedSeriesConditionEventSource" data-testid="strategy-series-condition-event-source" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500">
                <option v-for="option in seriesSourceOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">事件比较</span>
              <select v-model="selectedSeriesConditionEventOperator" data-testid="strategy-series-condition-event-operator" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500">
                <option value=">">&gt;</option>
                <option value="<">&lt;</option>
              </select>
            </label>
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">事件阈值</span>
              <input v-model="selectedSeriesConditionEventThreshold" data-testid="strategy-series-condition-event-threshold" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" step="0.01" type="number" />
            </label>
          </div>

          <div v-if="selectedSeriesConditionMode === 'barssince'" class="grid gap-3 md:grid-cols-2">
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">最近 N 柱内</span>
              <input v-model="selectedSeriesConditionLength" data-testid="strategy-series-condition-bars-length" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" min="1" step="1" type="number" />
            </label>
          </div>

          <div v-if="selectedSeriesConditionMode === 'valuewhen'" class="grid gap-3 md:grid-cols-4">
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">取值源</span>
              <select v-model="selectedSeriesConditionValueSource" data-testid="strategy-series-condition-value-source" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500">
                <option v-for="option in seriesSourceOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">Occurrence</span>
              <input v-model="selectedSeriesConditionOccurrence" data-testid="strategy-series-condition-occurrence" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" min="0" step="1" type="number" />
            </label>
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">比较</span>
              <select v-model="selectedSeriesConditionOperator" data-testid="strategy-series-condition-value-operator" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500">
                <option value=">">&gt;</option>
                <option value="<">&lt;</option>
              </select>
            </label>
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">比较阈值</span>
              <input v-model="selectedSeriesConditionThreshold" data-testid="strategy-series-condition-value-threshold" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" step="0.01" type="number" />
            </label>
          </div>
        </template>

        <template v-if="props.showsTimeFilterInputs">
          <div class="grid gap-3 md:grid-cols-3">
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">过滤模式</span>
              <select v-model="selectedTimeFilterMode" data-testid="strategy-time-filter-mode" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500">
                <option v-for="option in timeFilterModeOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">开始小时</span>
              <input v-model="selectedTimeFilterStartHour" data-testid="strategy-time-filter-start-hour" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" max="23" min="0" step="1" type="number" />
            </label>
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">开始分钟</span>
              <input v-model="selectedTimeFilterStartMinute" data-testid="strategy-time-filter-start-minute" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" max="59" min="0" step="1" type="number" />
            </label>
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">结束小时</span>
              <input v-model="selectedTimeFilterEndHour" data-testid="strategy-time-filter-end-hour" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" max="23" min="0" step="1" type="number" />
            </label>
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">结束分钟</span>
              <input v-model="selectedTimeFilterEndMinute" data-testid="strategy-time-filter-end-minute" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" max="59" min="0" step="1" type="number" />
            </label>
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">星期</span>
              <select v-model="selectedTimeFilterDayOfWeek" data-testid="strategy-time-filter-day-of-week" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500">
                <option v-for="option in dayOfWeekOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>
          </div>
        </template>

        <template v-if="props.showsSessionFilterInputs">
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">交易时段</span>
            <select v-model="selectedSessionFilterScope" data-testid="strategy-session-filter-scope" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500">
              <option v-for="option in sessionScopeOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>
        </template>

        <template v-if="props.showsVisualExpressionInputs">
          <div class="grid gap-3 rounded-2xl border border-slate-200 bg-slate-50/70 p-3">
            <div class="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">结构化表达式</div>
            <div class="grid gap-3 md:grid-cols-2">
              <label class="grid gap-2 text-sm text-slate-700">
                <span class="font-medium">编辑目标</span>
                <select
                  v-model="selectedExpressionSlot"
                  data-testid="strategy-expression-slot"
                  class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
                >
                  <option v-for="option in props.expressionSlotOptions" :key="option.value" :value="option.value">
                    {{ option.label }}
                  </option>
                </select>
              </label>

              <label v-if="props.expressionReferenceOptions.length > 0" class="grid gap-2 text-sm text-slate-700">
                <span class="font-medium">引用</span>
                <select
                  v-model="selectedExpressionReference"
                  data-testid="strategy-expression-reference"
                  class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
                >
                  <option value="">选择变量</option>
                  <option v-for="option in props.expressionReferenceOptions" :key="option.value" :value="option.value">
                    {{ option.label }}
                  </option>
                </select>
              </label>

              <label v-if="expressionFieldOptions.length > 0" class="grid gap-2 text-sm text-slate-700">
                <span class="font-medium">字段</span>
                <select
                  v-model="selectedExpressionField"
                  data-testid="strategy-expression-field"
                  class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
                >
                  <option value="">选择字段</option>
                  <option v-for="option in expressionFieldOptions" :key="option.value" :value="option.value">
                    {{ option.label }}
                  </option>
                </select>
              </label>

              <label class="grid gap-2 text-sm text-slate-700">
                <span class="font-medium">操作符</span>
                <select
                  v-model="selectedExpressionOperator"
                  data-testid="strategy-expression-operator"
                  class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
                >
                  <option v-for="option in expressionOperatorOptions" :key="option.value" :value="option.value">
                    {{ option.label }}
                  </option>
                </select>
              </label>

              <label class="grid gap-2 text-sm text-slate-700">
                <span class="font-medium">函数</span>
                <select
                  v-model="selectedExpressionFunction"
                  data-testid="strategy-expression-function"
                  class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
                >
                  <option value="">选择函数</option>
                  <option v-for="option in expressionFunctionOptions" :key="option.value" :value="option.value">
                    {{ option.label }}
                  </option>
                </select>
              </label>

              <label class="grid gap-2 text-sm text-slate-700">
                <span class="font-medium">Literal / 右值</span>
                <input
                  v-model="selectedExpressionLiteral"
                  data-testid="strategy-expression-literal"
                  class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
                  placeholder="30"
                />
              </label>

              <label class="grid gap-2 text-sm text-slate-700">
                <span class="font-medium">历史引用</span>
                <input
                  v-model="selectedExpressionHistoryOffset"
                  data-testid="strategy-expression-history-offset"
                  class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
                  min="0"
                  step="1"
                  type="number"
                />
              </label>
            </div>
          </div>
        </template>

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

        <label v-if="props.showsIndicatorTimeframeInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">固定周期</span>
          <select
            v-model="selectedIndicatorTimeframe"
            data-testid="strategy-block-indicator-timeframe-select"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
          >
            <option v-for="option in indicatorTimeframeOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </label>

        <label v-if="props.showsIndicatorSourceInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">数据源</span>
          <select
            v-model="selectedIndicatorSource"
            data-testid="strategy-block-indicator-source-select"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
          >
            <option v-for="option in indicatorSourceOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
        </label>

        <label v-if="props.showsIndicatorAdxSmoothingInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">ADX 平滑周期</span>
          <input
            v-model="selectedIndicatorAdxSmoothing"
            data-testid="strategy-block-indicator-adx-smoothing-input"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            min="1"
            step="1"
            type="number"
          />
        </label>

        <label v-if="props.showsIndicatorFactorInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">因子</span>
          <input
            v-model="selectedIndicatorFactor"
            data-testid="strategy-block-indicator-factor-input"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            min="0.01"
            step="0.01"
            type="number"
          />
        </label>

        <div v-if="props.showsIndicatorSarInputs" class="grid gap-3 md:grid-cols-3">
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">起始</span>
            <input v-model="selectedIndicatorSarStart" data-testid="strategy-block-indicator-sar-start-input" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" min="0.01" step="0.01" type="number" />
          </label>
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">增量</span>
            <input v-model="selectedIndicatorSarIncrement" data-testid="strategy-block-indicator-sar-increment-input" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" min="0.01" step="0.01" type="number" />
          </label>
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">最大值</span>
            <input v-model="selectedIndicatorSarMaximum" data-testid="strategy-block-indicator-sar-maximum-input" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" min="0.01" step="0.01" type="number" />
          </label>
        </div>

        <div v-if="props.showsIndicatorPivotBarsInput" class="grid gap-3 md:grid-cols-2">
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">左侧柱数</span>
            <input v-model="selectedIndicatorLeftBars" data-testid="strategy-block-indicator-left-bars-input" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" min="1" step="1" type="number" />
          </label>
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">右侧柱数</span>
            <input v-model="selectedIndicatorRightBars" data-testid="strategy-block-indicator-right-bars-input" class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500" min="1" step="1" type="number" />
          </label>
        </div>

        <label v-if="props.showsIndicatorOffsetInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">Offset</span>
          <input
            v-model="selectedIndicatorOffset"
            data-testid="strategy-block-indicator-offset-input"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            step="0.01"
            type="number"
          />
        </label>

        <label v-if="props.showsIndicatorSigmaInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">Sigma</span>
          <input
            v-model="selectedIndicatorSigma"
            data-testid="strategy-block-indicator-sigma-input"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            min="0.01"
            step="0.01"
            type="number"
          />
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

        <label v-if="props.showsStopLossTakeProfitPercentageInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">止盈幅度（%）</span>
          <input
            v-model="selectedStopLossTakeProfitPercentage"
            data-testid="strategy-stop-loss-take-profit-percentage"
            class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            min="0"
            step="0.01"
            type="number"
          />
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
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">Pine 订单动作</span>
            <select
              v-model="selectedPlaceOrderAction"
              data-testid="strategy-place-order-action"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option v-for="option in placeOrderActionOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>

          <label v-if="props.showsPlaceOrderTargetIdInput" class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">订单 / 持仓 ID</span>
            <input
              v-model="selectedPlaceOrderId"
              data-testid="strategy-place-order-id"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              placeholder="Long"
              type="text"
            />
          </label>

          <label v-if="props.showsPlaceOrderRiskDirectionInput" class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">允许方向</span>
            <select
              v-model="selectedPlaceOrderRiskAllowedDirection"
              data-testid="strategy-place-order-risk-direction"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option v-for="option in riskAllowedDirectionOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>

          <div v-if="props.showsPlaceOrderSideInput" class="grid gap-3 md:grid-cols-2">
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

          <label v-if="props.showsPlaceOrderQuantityInputs" class="grid gap-2 text-sm text-slate-700">
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

          <label v-if="props.showsPlaceOrderStopPriceInput" class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">触发价 / Stop</span>
            <input
              v-model="selectedPlaceOrderStopPrice"
              data-testid="strategy-place-order-stop-price"
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

        <template v-if="props.showsAdvancedPineBlockInputs">
          <label class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">变量名</span>
            <input
              v-model="selectedAdvancedVariableName"
              data-testid="strategy-advanced-variable-name"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              placeholder="signal_name"
            />
          </label>

          <label v-if="advancedModeOptions.length > 0" class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">{{ advancedModeLabel }}</span>
            <select
              v-model="selectedAdvancedMode"
              data-testid="strategy-advanced-mode"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option v-for="option in advancedModeOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>

          <label
            v-if="props.selectedVisualKind === 'strategyInput' || props.selectedVisualKind === 'stateVariable'"
            class="grid gap-2 text-sm text-slate-700"
          >
            <span class="font-medium">默认值</span>
            <input
              v-model="selectedAdvancedDefaultValue"
              data-testid="strategy-advanced-default-value"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              placeholder="20"
            />
          </label>

          <label v-if="props.selectedVisualKind === 'mtfSeries'" class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">Timeframe</span>
            <input
              v-model="selectedAdvancedTimeframe"
              data-testid="strategy-advanced-timeframe"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              placeholder="D"
            />
          </label>

          <label
            v-if="props.selectedVisualKind === 'derivedSeries' || props.selectedVisualKind === 'mtfSeries' || props.selectedVisualKind === 'collectionStat'"
            class="grid gap-2 text-sm text-slate-700"
          >
            <span class="font-medium">Source</span>
            <select
              v-model="selectedAdvancedSource"
              data-testid="strategy-advanced-source"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option v-for="option in seriesSourceOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>

          <template v-if="props.selectedVisualKind === 'collectionStat'">
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">Source B</span>
              <select
                v-model="selectedAdvancedSecondarySource"
                data-testid="strategy-advanced-secondary-source"
                class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              >
                <option v-for="option in seriesSourceOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>
            <label class="grid gap-2 text-sm text-slate-700">
              <span class="font-medium">Source C</span>
              <select
                v-model="selectedAdvancedTertiarySource"
                data-testid="strategy-advanced-tertiary-source"
                class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              >
                <option v-for="option in seriesSourceOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </label>
          </template>

          <label
            v-if="props.selectedVisualKind === 'derivedSeries' || props.selectedVisualKind === 'mtfSeries' || props.selectedVisualKind === 'collectionStat'"
            class="grid gap-2 text-sm text-slate-700"
          >
            <span class="font-medium">{{ advancedNumberLabel }}</span>
            <input
              v-model="selectedAdvancedNumber"
              data-testid="strategy-advanced-number"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              min="0"
              step="1"
              type="number"
            />
          </label>

          <label
            v-if="props.selectedVisualKind === 'strategyInput' || props.selectedVisualKind === 'derivedSeries' || props.selectedVisualKind === 'mtfSeries' || props.selectedVisualKind === 'stateUpdate'"
            class="grid gap-2 text-sm text-slate-700"
          >
            <span class="font-medium">{{ advancedExpressionLabel }}</span>
            <textarea
              v-model="selectedAdvancedExpression"
              data-testid="strategy-advanced-expression"
              class="min-h-[86px] rounded-3xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
              placeholder="close > open"
            />
          </label>

          <label
            v-if="props.selectedVisualKind === 'derivedSeries' || props.selectedVisualKind === 'mtfSeries'"
            class="grid gap-2 text-sm text-slate-700"
          >
            <span class="font-medium">函数 / 字段</span>
            <select
              v-model="selectedAdvancedOption"
              data-testid="strategy-advanced-option"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option
                v-for="option in props.selectedVisualKind === 'mtfSeries' ? mtfFieldOptions : derivedOptionOptions"
                :key="option.value"
                :value="option.value"
              >
                {{ option.label }}
              </option>
            </select>
          </label>

          <label v-if="props.expressionReferenceOptions.length > 0" class="grid gap-2 text-sm text-slate-700">
            <span class="font-medium">引用变量</span>
            <select
              v-model="selectedAdvancedReference"
              data-testid="strategy-advanced-reference"
              class="rounded-2xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            >
              <option value="">选择变量</option>
              <option v-for="option in props.expressionReferenceOptions" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          </label>
        </template>

        <label v-if="props.showsCodeInput" class="grid gap-2 text-sm text-slate-700">
          <span class="font-medium">{{ codeInputLabel }}</span>
          <textarea
            v-model="selectedVisualNodeCode"
            class="min-h-[170px] rounded-3xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 outline-none transition focus:border-amber-500"
            :placeholder="codeInputPlaceholder"
          />
        </label>

        <button class="strategy-btn strategy-btn--ghost strategy-btn--danger" type="button" @click="emit('delete-selected-node')">
          删除当前图块
        </button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.strategy-block-support {
  display: grid;
  gap: 0.35rem;
  padding: 0.7rem 0.8rem;
  border: 1px solid color-mix(in srgb, var(--tv-border) 78%, transparent);
  border-radius: 0.75rem;
  background: color-mix(in srgb, var(--tv-bg-elevated) 48%, transparent);
}

.strategy-block-support__label {
  font-size: 0.72rem;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--tv-accent-strong);
}

.strategy-block-support__message {
  color: var(--tv-text-muted);
  font-size: 0.82rem;
  line-height: 1.5;
}

.strategy-block-support--unsupportedConfig {
  border-color: color-mix(in srgb, var(--card-red-border) 72%, var(--tv-border));
  background: color-mix(in srgb, var(--card-red-surface) 52%, transparent);
}

.strategy-block-support--unsupportedConfig .strategy-block-support__label {
  color: var(--card-red-text);
}

.strategy-block-support--warning,
.strategy-block-support--snippetOnly {
  border-color: color-mix(in srgb, var(--tv-accent) 42%, var(--tv-border));
}
</style>
