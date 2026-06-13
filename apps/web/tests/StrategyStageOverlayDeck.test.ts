// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { ref } from "vue";

import StrategyStageOverlayDeck from "../src/components/strategy-stage/StrategyStageOverlayDeck.vue";

function mountOverlayForPineSnippet() {
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
        selectedVisualNodeText: ref("Pine 片段"),
        selectedVisualNodeMessage: ref(""),
        selectedVisualNodeCode: ref("plot(close)"),
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
        text: "Pine 片段",
        properties: { blockKind: "pineSnippet" },
      },
      selectedVisualKind: "pineSnippet",
      selectedVisualBlockLabel: "Pine 片段",
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
  it("keeps Pine snippet input editable", () => {
    const wrapper = mountOverlayForPineSnippet();

    expect(wrapper.text()).toContain("Pine 片段");
    expect(wrapper.get("textarea").attributes("readonly")).toBeUndefined();
  });
});
