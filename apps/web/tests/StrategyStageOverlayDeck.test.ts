// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { ref } from "vue";

import StrategyStageOverlayDeck from "../src/components/strategy-stage/StrategyStageOverlayDeck.vue";

function createBindings() {
  return {
    definitionForm: ref({
      id: "def-1",
      name: "Overlay",
      version: "0.1.0",
      description: "",
      runtime: "pine-go-plan",
      sourceFormat: "pine-v6",
      symbol: "US.AAPL",
      interval: "1m",
      script: "",
      visualModel: null,
      createdAt: "",
      updatedAt: "",
    }),
    selectedVisualNodeText: ref("输出日志"),
    selectedVisualNodeMessage: ref(""),
    selectedVisualNodeCode: ref("plot(close)"),
    selectedVisualNodePeriod: ref("14"),
    selectedIndicatorVariableName: ref(""),
    selectedIndicatorType: ref("rsi"),
    selectedMovingAverageType: ref("EMA"),
    selectedIndicatorTimeframe: ref(""),
    selectedIndicatorConditionMode: ref("numeric"),
    selectedIndicatorOperator: ref("<"),
    selectedIndicatorPatternType: ref("goldenCross"),
    selectedIndicatorLookback: ref("5"),
    selectedSeriesConditionMode: ref("compare"),
    selectedSeriesConditionSource: ref("close"),
    selectedSeriesConditionOperator: ref(">"),
    selectedSeriesConditionThreshold: ref("520"),
    selectedSeriesConditionLength: ref("3"),
    selectedSeriesConditionEventSource: ref("close"),
    selectedSeriesConditionEventOperator: ref(">"),
    selectedSeriesConditionEventThreshold: ref("520"),
    selectedSeriesConditionValueSource: ref("close"),
    selectedSeriesConditionOccurrence: ref("0"),
    selectedIndicatorPrimaryInputNodeId: ref(""),
    selectedIndicatorFastInputNodeId: ref(""),
    selectedIndicatorSlowInputNodeId: ref(""),
    selectedStopLossMode: ref("stopLoss"),
    selectedStopLossDirection: ref("auto"),
    selectedStopLossTimeUnit: ref("bar"),
    selectedStopLossWindowPolicy: ref("continuous"),
    selectedStopLossTakeProfitPercentage: ref("4"),
    selectedMacdFastPeriod: ref("12"),
    selectedMacdSlowPeriod: ref("26"),
    selectedMacdSignalPeriod: ref("9"),
    selectedBollingerMultiplier: ref("2"),
    selectedIndicatorSource: ref("close"),
    selectedIndicatorAdxSmoothing: ref("14"),
    selectedIndicatorFactor: ref("3"),
    selectedIndicatorSarStart: ref("0.02"),
    selectedIndicatorSarIncrement: ref("0.02"),
    selectedIndicatorSarMaximum: ref("0.2"),
    selectedIndicatorOffset: ref("0"),
    selectedIndicatorSigma: ref("6"),
    selectedIndicatorLeftBars: ref("2"),
    selectedIndicatorRightBars: ref("2"),
    selectedVisualNodeThreshold: ref("30"),
    selectedPlaceOrderAction: ref("entry"),
    selectedPlaceOrderId: ref("Long"),
    selectedPlaceOrderSide: ref("BUY"),
    selectedPlaceOrderType: ref("MARKET"),
    selectedPlaceOrderEntryPositionPolicy: ref("sameDirection"),
    selectedPlaceOrderQuantityMode: ref("shares"),
    selectedPlaceOrderQuantityValue: ref("1"),
    selectedPlaceOrderLimitPrice: ref(""),
    selectedPlaceOrderStopPrice: ref(""),
    selectedPlaceOrderRiskAllowedDirection: ref("all"),
    selectedExpressionSlot: ref("primary"),
    selectedExpressionReference: ref(""),
    selectedExpressionField: ref(""),
    selectedExpressionOperator: ref(">"),
    selectedExpressionFunction: ref(""),
    selectedExpressionLiteral: ref(""),
    selectedExpressionHistoryOffset: ref(""),
    selectedAdvancedVariableName: ref("daily_close"),
    selectedAdvancedMode: ref("indicator"),
    selectedAdvancedDefaultValue: ref("20"),
    selectedAdvancedTimeframe: ref("D"),
    selectedAdvancedSource: ref("close"),
    selectedAdvancedSecondarySource: ref("open"),
    selectedAdvancedTertiarySource: ref("high"),
    selectedAdvancedNumber: ref("1"),
    selectedAdvancedExpression: ref("ta.supertrend(3, 10)"),
    selectedAdvancedOption: ref("direction"),
    selectedAdvancedReference: ref(""),
    selectedTimeFilterMode: ref("between"),
    selectedTimeFilterStartHour: ref("9"),
    selectedTimeFilterStartMinute: ref("30"),
    selectedTimeFilterEndHour: ref("16"),
    selectedTimeFilterEndMinute: ref("0"),
    selectedTimeFilterDayOfWeek: ref("2"),
    selectedSessionFilterScope: ref("market"),
  };
}

function mountOverlay(overrides: Record<string, unknown> = {}) {
  return mount(StrategyStageOverlayDeck, {
    props: {
      bindings: createBindings(),
      showTemplatesSection: false,
      showBasicInfoSection: false,
      showBlockDetailsSection: true,
      activeStrategyTemplateMode: null,
      strategyTemplates: [],
      selectedStrategyTemplateId: "",
      selectedVisualNode: {
        id: "node-1",
        type: "rect",
        x: 0,
        y: 0,
        text: "输出日志",
        properties: { blockKind: "log", message: "hello" },
      },
      selectedVisualKind: "log",
      selectedVisualBlockLabel: "输出日志",
      selectedVisualBlockDescription: "",
      selectedVisualSupport: {
        status: "supported",
        label: "可运行",
        message: "该图块会生成 Pine v6 日志语句。",
      },
      showsCodeInput: false,
      showsVisualExpressionInputs: false,
      showsAdvancedPineBlockInputs: false,
      showsTimeFilterInputs: false,
      showsSessionFilterInputs: false,
      expressionSlotOptions: [{ value: "primary", label: "表达式" }],
      expressionReferenceOptions: [],
      showsPeriodInput: false,
      showsMacdInputs: false,
      showsTechnicalIndicatorMacdInputs: false,
      showsMovingAverageTypeInput: false,
      showsIndicatorVariableNameInput: false,
      indicatorVariableNamePlaceholder: "",
      showsIndicatorPrimaryInputSelect: false,
      showsIndicatorFastInputSelect: false,
      showsIndicatorSlowInputSelect: false,
      indicatorGetterOptions: [],
      showsMultiplierInput: false,
      showsIndicatorSourceInput: false,
      showsIndicatorTimeframeInput: false,
      showsIndicatorAdxSmoothingInput: false,
      showsIndicatorFactorInput: false,
      showsIndicatorSarInputs: false,
      showsIndicatorOffsetInput: false,
      showsIndicatorSigmaInput: false,
      showsIndicatorPivotBarsInput: false,
      showsThresholdInput: false,
      showsConditionModeInput: false,
      showsSeriesConditionInputs: false,
      showsIndicatorTypeInput: false,
      showsPatternTypeInput: false,
      showsLookbackInput: false,
      showsPlaceOrderInputs: false,
      showsPlaceOrderEntryPositionPolicyInput: false,
      showsPlaceOrderLimitPriceInput: false,
      showsPlaceOrderStopPriceInput: false,
      showsPlaceOrderQuantityInputs: false,
      showsPlaceOrderSideInput: false,
      showsPlaceOrderTargetIdInput: false,
      showsPlaceOrderRiskDirectionInput: false,
      showsStopLossTakeProfitPercentageInput: false,
      createdAtText: "",
      updatedAtText: "",
      ...overrides,
    },
  });
}

describe("StrategyStageOverlayDeck", () => {
  it("does not expose a Pine snippet editor", () => {
    const wrapper = mountOverlay();

    expect(wrapper.text()).not.toContain("Pine 片段");
    expect(wrapper.text()).not.toContain("代码片段");
  });

  it("renders advanced Pine indicator controls when requested by the capability surface", () => {
    const wrapper = mountOverlay({
      selectedVisualKind: "getTechnicalIndicator",
      selectedVisualBlockLabel: "指标数据",
      selectedVisualNode: {
        id: "trend-node",
        type: "rect",
        x: 0,
        y: 0,
        text: "Supertrend",
        properties: { blockKind: "getTechnicalIndicator", indicatorType: "supertrend" },
      },
      showsCodeInput: false,
      showsPeriodInput: true,
      showsIndicatorTypeInput: true,
      showsIndicatorSourceInput: true,
      showsIndicatorTimeframeInput: true,
      showsIndicatorFactorInput: true,
      selectedVisualSupport: {
        status: "supported",
        label: "指标可运行",
        message: "生成 Supertrend 的 Pine v6 表达式。",
      },
    });

    expect(wrapper.get('[data-testid="strategy-block-indicator-source-select"]').exists()).toBe(true);
    const timeframeSelect = wrapper.get('[data-testid="strategy-block-indicator-timeframe-select"]');
    expect(timeframeSelect.text()).toContain("当前周期");
    expect(timeframeSelect.text()).toContain("日线");
    expect(wrapper.get('[data-testid="strategy-block-indicator-factor-input"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-block-period-input"]').exists()).toBe(true);
  });

  it("renders editable controls for advanced Pine visual blocks", () => {
    const wrapper = mountOverlay({
      selectedVisualKind: "mtfSeries",
      selectedVisualBlockLabel: "高周期序列",
      selectedVisualNode: {
        id: "mtf-node",
        type: "rect",
        x: 0,
        y: 0,
        text: "MTF trend",
        properties: { blockKind: "mtfSeries", variableName: "daily_trend" },
      },
      showsCodeInput: false,
      showsAdvancedPineBlockInputs: true,
      expressionReferenceOptions: [
        { value: "trend_fast", label: "EMA 快线 · trend_fast" },
      ],
      selectedVisualSupport: {
        status: "supported",
        label: "MTF 可运行",
        message: "生成静态 timeframe 的 request.security。",
      },
    });

    expect(wrapper.get('[data-testid="strategy-advanced-variable-name"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-advanced-mode"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-advanced-timeframe"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-advanced-expression"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-advanced-reference"]').text()).toContain("trend_fast");
  });

  it("renders structured expression editor controls with reference fields", () => {
    const wrapper = mountOverlay({
      selectedVisualKind: "seriesCondition",
      selectedVisualBlockLabel: "序列条件判断",
      selectedVisualNode: {
        id: "condition-node",
        type: "diamond",
        x: 0,
        y: 0,
        text: "结构化条件",
        properties: { blockKind: "seriesCondition", mode: "compare" },
      },
      showsCodeInput: false,
      showsSeriesConditionInputs: true,
      showsVisualExpressionInputs: true,
      expressionReferenceOptions: [
        {
          value: "trend",
          label: "趋势 · trend",
          sourceBlockKind: "getTechnicalIndicator",
          fields: [{ value: "histogram", label: "histogram" }],
        },
      ],
      selectedVisualSupport: {
        status: "supported",
        label: "表达式可运行",
        message: "结构化表达式会生成安全 Pine。",
      },
    });

    expect(wrapper.get('[data-testid="strategy-expression-reference"]').text()).toContain("trend");
    expect(wrapper.get('[data-testid="strategy-expression-slot"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-expression-operator"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-expression-function"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-expression-literal"]').exists()).toBe(true);
    expect(wrapper.get('[data-testid="strategy-expression-history-offset"]').exists()).toBe(true);
  });

  it("renders time and session filter inspector controls", () => {
    const timeWrapper = mountOverlay({
      selectedVisualKind: "timeFilter",
      selectedVisualBlockLabel: "时间过滤",
      selectedVisualNode: {
        id: "time-filter",
        type: "diamond",
        x: 0,
        y: 0,
        text: "时间过滤",
        properties: { blockKind: "timeFilter" },
      },
      showsCodeInput: false,
      showsTimeFilterInputs: true,
      selectedVisualSupport: {
        status: "supported",
        label: "时间过滤可运行",
        message: "生成 hour/minute 条件。",
      },
    });
    expect(timeWrapper.get('[data-testid="strategy-time-filter-mode"]').exists()).toBe(true);
    expect(timeWrapper.get('[data-testid="strategy-time-filter-start-hour"]').exists()).toBe(true);
    expect(timeWrapper.get('[data-testid="strategy-time-filter-day-of-week"]').exists()).toBe(true);

    const sessionWrapper = mountOverlay({
      selectedVisualKind: "sessionFilter",
      selectedVisualBlockLabel: "交易时段过滤",
      selectedVisualNode: {
        id: "session-filter",
        type: "diamond",
        x: 0,
        y: 0,
        text: "交易时段过滤",
        properties: { blockKind: "sessionFilter" },
      },
      showsCodeInput: false,
      showsSessionFilterInputs: true,
      selectedVisualSupport: {
        status: "supported",
        label: "交易时段可运行",
        message: "生成 session 状态条件。",
      },
    });
    expect(sessionWrapper.get('[data-testid="strategy-session-filter-scope"]').text()).toContain("常规交易时段");
  });
});
