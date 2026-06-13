// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { ref } from "vue";

import StrategyStageOverlayDeck from "../src/components/strategy-stage/StrategyStageOverlayDeck.vue";

function mountOverlayForCodeKind(kind: "codeBlock" | "pineSnippet") {
  return mount(StrategyStageOverlayDeck, {
    props: {
      bindings: {
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
        selectedVisualNodeText: ref(kind === "codeBlock" ? "旧代码块" : "Pine 片段"),
        selectedVisualNodeMessage: ref(""),
        selectedVisualNodeCode: ref(kind === "codeBlock" ? "console.log('legacy')" : "plot(close)"),
        selectedVisualNodePeriod: ref("14"),
        selectedIndicatorVariableName: ref(""),
        selectedIndicatorType: ref("rsi"),
        selectedMovingAverageType: ref("EMA"),
        selectedIndicatorPeriodUnit: ref("bar"),
        selectedIndicatorConditionMode: ref("numeric"),
        selectedIndicatorOperator: ref("<"),
        selectedIndicatorPatternType: ref("goldenCross"),
        selectedIndicatorLookback: ref("5"),
        selectedIndicatorPrimaryInputNodeId: ref(""),
        selectedIndicatorFastInputNodeId: ref(""),
        selectedIndicatorSlowInputNodeId: ref(""),
        selectedStopLossMode: ref("stopLoss"),
        selectedStopLossDirection: ref("auto"),
        selectedStopLossTimeUnit: ref("bar"),
        selectedStopLossWindowPolicy: ref("continuous"),
        selectedMacdFastPeriod: ref("12"),
        selectedMacdSlowPeriod: ref("26"),
        selectedMacdSignalPeriod: ref("9"),
        selectedBollingerMultiplier: ref("2"),
        selectedVisualNodeThreshold: ref("30"),
        selectedPlaceOrderSide: ref("BUY"),
        selectedPlaceOrderType: ref("MARKET"),
        selectedPlaceOrderEntryPositionPolicy: ref("sameDirection"),
        selectedPlaceOrderQuantityMode: ref("shares"),
        selectedPlaceOrderQuantityValue: ref("1"),
        selectedPlaceOrderLimitPrice: ref(""),
      },
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
        text: kind === "codeBlock" ? "旧代码块" : "Pine 片段",
        properties: { blockKind: kind },
      },
      selectedVisualKind: kind,
      selectedVisualBlockLabel: kind === "codeBlock" ? "代码块" : "Pine 片段",
      selectedVisualBlockDescription: "",
      showsCodeInput: true,
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
      showsThresholdInput: false,
      showsConditionModeInput: false,
      showsIndicatorTypeInput: false,
      showsPatternTypeInput: false,
      showsLookbackInput: false,
      showsPlaceOrderInputs: false,
      showsPlaceOrderEntryPositionPolicyInput: false,
      showsPlaceOrderLimitPriceInput: false,
      createdAtText: "",
      updatedAtText: "",
    },
  });
}

describe("StrategyStageOverlayDeck", () => {
  it("shows legacy codeBlock input as read-only compatibility", () => {
    const wrapper = mountOverlayForCodeKind("codeBlock");

    expect(wrapper.text()).toContain("历史代码块（只读）");
    expect(wrapper.text()).toContain("旧 codeBlock 仅可读取");
    expect(wrapper.get("textarea").attributes("readonly")).toBeDefined();
  });

  it("keeps Pine snippet input editable", () => {
    const wrapper = mountOverlayForCodeKind("pineSnippet");

    expect(wrapper.text()).toContain("Pine 片段");
    expect(wrapper.get("textarea").attributes("readonly")).toBeUndefined();
  });
});
