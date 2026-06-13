import type {
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@/contracts";
import { computed, type ComputedRef } from "vue";

import {
  getStrategyBlockDefinition,
  getStrategyBlockKind,
  type StrategyBlockKind,
} from "../features/strategyVisualBuilder";
import {
  nextStopLossNodeText,
  normalizeStopLossBlockProperties,
} from "../features/strategyVisualBuilderCatalog";
import {
  getTechnicalIndicatorDefinition,
  isDivergencePattern,
  nextGetTechnicalIndicatorNodeText,
  nextTechnicalIndicatorConditionNodeText,
  nextTechnicalIndicatorNodeText,
  normalizeIndicatorPeriodUnit,
  normalizeGetTechnicalIndicatorProperties,
  normalizeTechnicalIndicatorConditionProperties,
  normalizeTechnicalIndicatorProperties,
  type GetTechnicalIndicatorBlockProperties,
  type TechnicalIndicatorBlockProperties,
  type TechnicalIndicatorConditionBlockProperties,
} from "../features/strategyVisualBuilderIndicatorBlock";
import {
  listStrategyIndicatorGetterOptions,
  suggestStrategyIndicatorVariableName,
} from "../features/strategyVisualBuilderIndicatorReferences";
import {
  normalizeEntryPositionPolicy,
  normalizeOrderSide,
  normalizeQuantityModeForSide,
} from "../features/strategyVisualBuilderScriptSupport";

interface UseStrategyVisualNodeInspectorOptions {
  visualModel: ComputedRef<StrategyVisualModelDocument>;
  selectedVisualNode: ComputedRef<StrategyVisualNodeDocument | null>;
  mutateSelectedVisualNode: (
    mutator: (node: StrategyVisualNodeDocument) => StrategyVisualNodeDocument,
  ) => void;
}

export function useStrategyVisualNodeInspector(
  { visualModel, selectedVisualNode, mutateSelectedVisualNode }: UseStrategyVisualNodeInspectorOptions,
) {
  const selectedVisualKind = computed<StrategyBlockKind | null>(() =>
    getStrategyBlockKind(selectedVisualNode.value),
  );

  const selectedVisualBlock = computed(() =>
    getStrategyBlockDefinition(selectedVisualKind.value),
  );

  const selectedVisualNodeText = computed({
    get: () => selectedVisualNode.value?.text ?? "",
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        text: value,
      }));
    },
  });

  const selectedVisualNodeMessage = computed({
    get: () => {
      const message = selectedVisualNode.value?.properties.message;
      return typeof message === "string" ? message : "";
    },
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          message: value,
        },
      }));
    },
  });

  const showsCodeInput = computed(() =>
    selectedVisualKind.value === "codeBlock" || selectedVisualKind.value === "pineSnippet",
  );

  const selectedVisualNodeCode = computed({
    get: () => {
      const code = selectedVisualNode.value?.properties.code;
      return typeof code === "string" ? code : "";
    },
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        text: value.trim() === ""
          ? selectedVisualKind.value === "pineSnippet" ? "Pine 片段" : "代码块"
          : node.text,
        properties: {
          ...node.properties,
          code: value,
        },
      }));
    },
  });

  const stopLoss = computed(() => {
    if (selectedVisualKind.value !== "stopLoss") {
      return null;
    }
    return normalizeStopLossBlockProperties(selectedVisualNode.value?.properties ?? {});
  });

  const technicalIndicator = computed<TechnicalIndicatorBlockProperties | null>(() => {
    if (selectedVisualKind.value !== "technicalIndicator") {
      return null;
    }
    return normalizeTechnicalIndicatorProperties(selectedVisualNode.value?.properties ?? {});
  });

  const technicalIndicatorGetter = computed<GetTechnicalIndicatorBlockProperties | null>(() => {
    if (selectedVisualKind.value !== "getTechnicalIndicator") {
      return null;
    }
    return normalizeGetTechnicalIndicatorProperties(selectedVisualNode.value?.properties ?? {});
  });

  const technicalIndicatorCondition = computed<TechnicalIndicatorConditionBlockProperties | null>(() => {
    if (selectedVisualKind.value !== "technicalIndicatorCondition") {
      return null;
    }
    return normalizeTechnicalIndicatorConditionProperties(selectedVisualNode.value?.properties ?? {});
  });

  const selectedIndicatorTypeValue = computed(() =>
    technicalIndicatorGetter.value?.indicatorType
    ?? technicalIndicatorCondition.value?.indicatorType
    ?? technicalIndicator.value?.indicatorType
    ?? "rsi",
  );

  const selectedIndicatorDefinition = computed(() =>
    getTechnicalIndicatorDefinition(selectedIndicatorTypeValue.value),
  );

  const isLegacyTechnicalIndicator = computed(
    () => selectedVisualKind.value === "technicalIndicator",
  );

  const isTechnicalIndicatorGetter = computed(
    () => selectedVisualKind.value === "getTechnicalIndicator",
  );

  const isTechnicalIndicatorCondition = computed(
    () => selectedVisualKind.value === "technicalIndicatorCondition",
  );

  const isAnyTechnicalIndicator = computed(() =>
    isLegacyTechnicalIndicator.value
    || isTechnicalIndicatorGetter.value
    || isTechnicalIndicatorCondition.value,
  );

  const showsIndicatorTypeInput = computed(
    () => isAnyTechnicalIndicator.value,
  );

  const showsConditionModeInput = computed(
    () => isLegacyTechnicalIndicator.value || isTechnicalIndicatorCondition.value,
  );

  const showsThresholdInput = computed(() => {
    if (isLegacyTechnicalIndicator.value) {
      return technicalIndicator.value?.conditionMode === "numeric";
    }
    if (isTechnicalIndicatorCondition.value) {
      return technicalIndicatorCondition.value?.conditionMode === "numeric";
    }
    if (selectedVisualKind.value === "stopLoss") {
      return true;
    }
    return selectedVisualKind.value === "ifCloseAbove" || selectedVisualKind.value === "ifCloseBelow";
  });

  const showsPeriodInput = computed(() => {
    if (isTechnicalIndicatorGetter.value) {
      return selectedIndicatorDefinition.value.parameterShape === "period"
        || selectedIndicatorDefinition.value.parameterShape === "windowSize"
        || selectedIndicatorDefinition.value.parameterShape === "bollinger";
    }
    if (isLegacyTechnicalIndicator.value) {
      const indicatorType = technicalIndicator.value?.indicatorType;
      return indicatorType !== "movingAverage" && indicatorType !== "macd";
    }
    if (selectedVisualKind.value === "stopLoss") {
      return true;
    }
    return false;
  });

  const showsMacdInputs = computed(() => false);
  const showsTechnicalIndicatorMacdInputs = computed(() => {
    if (isTechnicalIndicatorGetter.value) {
      return selectedIndicatorDefinition.value.parameterShape === "macd"
        || selectedIndicatorDefinition.value.parameterShape === "kdj";
    }
    if (isLegacyTechnicalIndicator.value) {
      const indicatorType = technicalIndicator.value?.indicatorType;
      return indicatorType === "movingAverage" || indicatorType === "macd" || indicatorType === "kdj";
    }
    return false;
  });

  const showsMovingAverageTypeInput = computed(() =>
    selectedIndicatorTypeValue.value === "movingAverage"
    && (isLegacyTechnicalIndicator.value || isTechnicalIndicatorGetter.value),
  );

  const showsIndicatorVariableNameInput = computed(
    () => isTechnicalIndicatorGetter.value,
  );

  const showsIndicatorPrimaryInputSelect = computed(
    () => isTechnicalIndicatorCondition.value && selectedIndicatorTypeValue.value !== "movingAverage",
  );

  const showsIndicatorFastInputSelect = computed(
    () => isTechnicalIndicatorCondition.value && selectedIndicatorTypeValue.value === "movingAverage",
  );

  const showsIndicatorSlowInputSelect = computed(
    () => isTechnicalIndicatorCondition.value && selectedIndicatorTypeValue.value === "movingAverage",
  );

  const indicatorGetterOptions = computed(() =>
    listStrategyIndicatorGetterOptions(
      visualModel.value,
      isTechnicalIndicatorCondition.value
        ? technicalIndicatorCondition.value?.indicatorType
        : undefined,
    ),
  );

  const showsMultiplierInput = computed(
    () => selectedIndicatorTypeValue.value === "bollinger"
      && (isLegacyTechnicalIndicator.value || isTechnicalIndicatorGetter.value),
  );

  const showsPatternTypeInput = computed(
    () => technicalIndicatorCondition.value?.conditionMode === "pattern"
      || technicalIndicator.value?.conditionMode === "pattern",
  );

  const showsLookbackInput = computed(() =>
    isDivergencePattern(
      technicalIndicatorCondition.value?.patternType ?? technicalIndicator.value?.patternType,
    ),
  );

  const selectedIndicatorType = computed({
    get: () => selectedIndicatorTypeValue.value,
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          indicatorType: value,
        }));
        return;
      }
      if (isTechnicalIndicatorCondition.value) {
        updateTechnicalIndicatorCondition((properties) => ({
          ...properties,
          indicatorType: value,
        }));
        return;
      }
      updateTechnicalIndicator((properties) => ({
        ...properties,
        indicatorType: value,
      }));
    },
  });

  const selectedIndicatorConditionMode = computed({
    get: () => technicalIndicatorCondition.value?.conditionMode ?? technicalIndicator.value?.conditionMode ?? "numeric",
    set: (value: string) => {
      if (isTechnicalIndicatorCondition.value) {
        updateTechnicalIndicatorCondition((properties) => ({
          ...properties,
          conditionMode: value,
        }));
        return;
      }
      updateTechnicalIndicator((properties) => ({
        ...properties,
        conditionMode: value,
      }));
    },
  });

  const selectedIndicatorOperator = computed({
    get: () => technicalIndicatorCondition.value?.operator ?? technicalIndicator.value?.operator ?? ">",
    set: (value: string) => {
      if (isTechnicalIndicatorCondition.value) {
        updateTechnicalIndicatorCondition((properties) => ({
          ...properties,
          operator: value,
        }));
        return;
      }
      updateTechnicalIndicator((properties) => ({
        ...properties,
        operator: value,
      }));
    },
  });

  const selectedIndicatorPatternType = computed({
    get: () => technicalIndicatorCondition.value?.patternType ?? technicalIndicator.value?.patternType ?? "goldenCross",
    set: (value: string) => {
      if (isTechnicalIndicatorCondition.value) {
        updateTechnicalIndicatorCondition((properties) => ({
          ...properties,
          patternType: value,
        }));
        return;
      }
      updateTechnicalIndicator((properties) => ({
        ...properties,
        patternType: value,
      }));
    },
  });

  const selectedIndicatorLookback = computed({
    get: () => readNumberString(technicalIndicatorCondition.value?.lookback ?? technicalIndicator.value?.lookback),
    set: (value: string) => {
      if (isTechnicalIndicatorCondition.value) {
        updateTechnicalIndicatorCondition((properties) => ({
          ...properties,
          lookback: normalizeInteger(value, 5),
        }));
        return;
      }
      updateTechnicalIndicator((properties) => ({
        ...properties,
        lookback: normalizeInteger(value, 5),
      }));
    },
  });

  const selectedVisualNodeThreshold = computed({
    get: () => {
      if (selectedVisualKind.value === "stopLoss") {
        return readNumberString(stopLoss.value?.percentage);
      }
      const threshold = selectedVisualNode.value?.properties.threshold;
      return readNumberString(threshold);
    },
    set: (value: string) => {
      if (selectedVisualKind.value === "stopLoss") {
        updateStopLoss((properties) => ({
          ...properties,
          percentage: normalizeDecimal(value, 2),
        }));
        return;
      }
      const threshold = normalizeDecimal(value, 0);
      if (isTechnicalIndicatorCondition.value) {
        updateTechnicalIndicatorCondition((properties) => ({
          ...properties,
          threshold,
        }));
        return;
      }
      if (isLegacyTechnicalIndicator.value) {
        updateTechnicalIndicator((properties) => ({
          ...properties,
          threshold,
        }));
        return;
      }
      mutateSelectedVisualNode((node) => ({
        ...node,
        text: nextPriceConditionNodeText(selectedVisualKind.value, threshold),
        properties: {
          ...node.properties,
          threshold,
        },
      }));
    },
  });

  const selectedVisualNodePeriod = computed({
    get: () => {
      if (selectedVisualKind.value === "stopLoss") {
        return readNumberString(stopLoss.value?.timeValue);
      }
      if (isTechnicalIndicatorGetter.value) {
        if (selectedIndicatorDefinition.value.parameterShape === "windowSize") {
          return readNumberString(technicalIndicatorGetter.value?.windowSize);
        }
        return readNumberString(technicalIndicatorGetter.value?.period);
      }
      if (isLegacyTechnicalIndicator.value) {
        const indicator = technicalIndicator.value;
        if (indicator === null) {
          return "";
        }
        return readNumberString(indicator.period);
      }
      return "";
    },
    set: (value: string) => {
      if (selectedVisualKind.value === "stopLoss") {
        updateStopLoss((properties) => ({
          ...properties,
          timeValue: normalizeInteger(value, 1),
        }));
        return;
      }
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => {
          if (selectedIndicatorDefinition.value.parameterShape === "windowSize") {
            return {
              ...properties,
              windowSize: normalizeInteger(value, 20),
            };
          }
          return {
            ...properties,
            period: normalizeInteger(value, 14),
          };
        });
        return;
      }
      if (!isLegacyTechnicalIndicator.value) {
        return;
      }
      updateTechnicalIndicator((properties) => ({
        ...properties,
        period: normalizeInteger(value, 14),
      }));
    },
  });

  const selectedMacdFastPeriod = computed({
    get: () => {
      const indicator = technicalIndicatorGetter.value ?? technicalIndicator.value;
      if (indicator === null) {
        return "";
      }
      if (indicator.indicatorType === "movingAverage" || indicator.indicatorType === "macd") {
        return readNumberString(indicator.fastPeriod);
      }
      return "";
    },
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          fastPeriod: normalizeInteger(value, 12),
        }));
        return;
      }
      updateTechnicalIndicator((properties) => ({
        ...properties,
        fastPeriod: normalizeInteger(value, properties.indicatorType === "movingAverage" ? 5 : 12),
      }));
    },
  });

  const selectedMacdSlowPeriod = computed({
    get: () => {
      const indicator = technicalIndicatorGetter.value ?? technicalIndicator.value;
      if (indicator === null) {
        return "";
      }
      if (indicator.indicatorType === "movingAverage" || indicator.indicatorType === "macd") {
        return readNumberString(indicator.slowPeriod);
      }
      if (indicator.indicatorType === "kdj") {
        return readNumberString(indicator.m1);
      }
      return "";
    },
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => {
          if (properties.indicatorType === "kdj") {
            return {
              ...properties,
              m1: normalizeInteger(value, 3),
            };
          }
          return {
            ...properties,
            slowPeriod: normalizeInteger(value, 26),
          };
        });
        return;
      }
      updateTechnicalIndicator((properties) => {
        if (properties.indicatorType === "kdj") {
          return {
            ...properties,
            m1: normalizeInteger(value, 3),
          };
        }
        return {
          ...properties,
          slowPeriod: normalizeInteger(value, properties.indicatorType === "movingAverage" ? 20 : 26),
        };
      });
    },
  });

  const selectedMacdSignalPeriod = computed({
    get: () => {
      const indicator = technicalIndicatorGetter.value ?? technicalIndicator.value;
      if (indicator === null) {
        return "";
      }
      if (indicator.indicatorType === "macd") {
        return readNumberString(indicator.signalPeriod);
      }
      if (indicator.indicatorType === "kdj") {
        return readNumberString(indicator.m2);
      }
      return "";
    },
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => {
          if (properties.indicatorType === "kdj") {
            return {
              ...properties,
              m2: normalizeInteger(value, 3),
            };
          }
          return {
            ...properties,
            signalPeriod: normalizeInteger(value, 9),
          };
        });
        return;
      }
      updateTechnicalIndicator((properties) => {
        if (properties.indicatorType === "kdj") {
          return {
            ...properties,
            m2: normalizeInteger(value, 3),
          };
        }
        return {
          ...properties,
          signalPeriod: normalizeInteger(value, 9),
        };
      });
    },
  });

  const selectedBollingerMultiplier = computed({
    get: () => readNumberString(technicalIndicatorGetter.value?.multiplier ?? technicalIndicator.value?.multiplier),
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          multiplier: normalizeDecimal(value, 2),
        }));
        return;
      }
      updateTechnicalIndicator((properties) => ({
        ...properties,
        multiplier: normalizeDecimal(value, 2),
      }));
    },
  });

  const selectedMovingAverageType = computed({
    get: () => technicalIndicatorGetter.value?.movingAverageType ?? technicalIndicator.value?.movingAverageType ?? "MA",
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          movingAverageType: value,
        }));
        return;
      }
      updateTechnicalIndicator((properties) => ({
        ...properties,
        movingAverageType: value,
      }));
    },
  });

  const selectedIndicatorPeriodUnit = computed({
    get: () => technicalIndicatorGetter.value?.periodUnit ?? "bar",
    set: (value: string) => {
      if (!isTechnicalIndicatorGetter.value) {
        return;
      }
      updateTechnicalIndicatorGetter((properties) => ({
        ...properties,
        periodUnit: normalizeIndicatorPeriodUnit(value),
      }));
    },
  });

  const indicatorVariableNamePlaceholder = computed(() => {
    if (!isTechnicalIndicatorGetter.value) {
      return "";
    }
    return suggestStrategyIndicatorVariableName(selectedVisualNode.value?.properties ?? {});
  });

  const selectedIndicatorVariableName = computed({
    get: () => technicalIndicatorGetter.value?.variableName ?? "",
    set: (value: string) => {
      if (!isTechnicalIndicatorGetter.value) {
        return;
      }

      updateTechnicalIndicatorGetter((properties) => {
        const nextProperties = { ...properties };
        const normalized = value.trim();
        if (normalized === "") {
          delete nextProperties.variableName;
        } else {
          nextProperties.variableName = normalized;
        }
        return nextProperties;
      });
    },
  });

  const selectedIndicatorPrimaryInputNodeId = computed({
    get: () => technicalIndicatorCondition.value?.inputPrimaryNodeId ?? "",
    set: (value: string) => {
      updateTechnicalIndicatorConditionInput("inputPrimaryNodeId", value);
    },
  });

  const selectedIndicatorFastInputNodeId = computed({
    get: () => technicalIndicatorCondition.value?.inputFastNodeId ?? "",
    set: (value: string) => {
      updateTechnicalIndicatorConditionInput("inputFastNodeId", value);
    },
  });

  const selectedIndicatorSlowInputNodeId = computed({
    get: () => technicalIndicatorCondition.value?.inputSlowNodeId ?? "",
    set: (value: string) => {
      updateTechnicalIndicatorConditionInput("inputSlowNodeId", value);
    },
  });

  const selectedStopLossDirection = computed({
    get: () => stopLoss.value?.direction ?? "auto",
    set: (value: string) => {
      updateStopLoss((properties) => ({
        ...properties,
        direction: value,
      }));
    },
  });

  const selectedStopLossMode = computed({
    get: () => stopLoss.value?.mode ?? "stopLoss",
    set: (value: string) => {
      updateStopLoss((properties) => ({
        ...properties,
        mode: value,
      }));
    },
  });

  const selectedStopLossTimeUnit = computed({
    get: () => stopLoss.value?.timeUnit ?? "day",
    set: (value: string) => {
      updateStopLoss((properties) => ({
        ...properties,
        timeUnit: value,
      }));
    },
  });

  const selectedStopLossWindowPolicy = computed({
    get: () => stopLoss.value?.windowPolicy ?? "continuous",
    set: (value: string) => {
      updateStopLoss((properties) => ({
        ...properties,
        windowPolicy: value,
      }));
    },
  });

  const showsPlaceOrderInputs = computed(
    () => selectedVisualKind.value === "placeOrder",
  );

  const selectedPlaceOrderSide = computed({
    get: () => readString(selectedVisualNode.value?.properties.side, "BUY"),
    set: (value: string) => {
      const normalizedSide = normalizeOrderSide(value);
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          side: normalizedSide,
          quantityMode: normalizeQuantityModeForSide(
            node.properties.quantityMode,
            normalizedSide,
          ),
        },
      }));
    },
  });

  const selectedPlaceOrderType = computed({
    get: () => readString(selectedVisualNode.value?.properties.orderType, "MARKET"),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          orderType: value,
        },
      }));
    },
  });

  const selectedPlaceOrderEntryPositionPolicy = computed({
    get: () => readString(selectedVisualNode.value?.properties.entryPositionPolicy, "sameDirection"),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          entryPositionPolicy: normalizeEntryPositionPolicy(value),
        },
      }));
    },
  });

  const selectedPlaceOrderQuantityMode = computed({
    get: () => normalizeQuantityModeForSide(
      selectedVisualNode.value?.properties.quantityMode,
      normalizeOrderSide(selectedVisualNode.value?.properties.side),
    ),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          quantityMode: normalizeQuantityModeForSide(
            value,
            normalizeOrderSide(node.properties.side),
          ),
        },
      }));
    },
  });

  const selectedPlaceOrderQuantityValue = computed({
    get: () => readNumberString(selectedVisualNode.value?.properties.quantityValue),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          quantityValue: normalizeDecimal(value, 100),
        },
      }));
    },
  });

  const selectedPlaceOrderLimitPrice = computed({
    get: () => readNumberString(selectedVisualNode.value?.properties.limitPrice),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          limitPrice: normalizeDecimal(value, 0),
        },
      }));
    },
  });

  const showsPlaceOrderLimitPriceInput = computed(
    () => selectedPlaceOrderType.value === "LIMIT",
  );

  const showsPlaceOrderEntryPositionPolicyInput = computed(() => {
    const side = normalizeOrderSide(selectedPlaceOrderSide.value);
    return side === "BUY" || side === "SELL_SHORT";
  });

  function updateTechnicalIndicator(
    mutator: (properties: Record<string, unknown>) => Record<string, unknown>,
  ): void {
    if (selectedVisualKind.value !== "technicalIndicator") {
      return;
    }

    mutateSelectedVisualNode((node) => {
      const nextProperties = normalizeTechnicalIndicatorProperties(mutator({ ...node.properties }));
      return {
        ...node,
        text: nextTechnicalIndicatorNodeText(nextProperties as unknown as Record<string, unknown>),
        properties: nextProperties as unknown as Record<string, unknown>,
      };
    });
  }

  function updateTechnicalIndicatorGetter(
    mutator: (properties: Record<string, unknown>) => Record<string, unknown>,
  ): void {
    if (selectedVisualKind.value !== "getTechnicalIndicator") {
      return;
    }

    mutateSelectedVisualNode((node) => {
      const nextProperties = normalizeGetTechnicalIndicatorProperties(mutator({ ...node.properties }));
      return {
        ...node,
        text: nextGetTechnicalIndicatorNodeText(nextProperties as unknown as Record<string, unknown>),
        properties: nextProperties as unknown as Record<string, unknown>,
      };
    });
  }

  function updateTechnicalIndicatorCondition(
    mutator: (properties: Record<string, unknown>) => Record<string, unknown>,
  ): void {
    if (selectedVisualKind.value !== "technicalIndicatorCondition") {
      return;
    }

    mutateSelectedVisualNode((node) => {
      const nextProperties = normalizeTechnicalIndicatorConditionProperties(mutator({ ...node.properties }));
      return {
        ...node,
        text: nextTechnicalIndicatorConditionNodeText(nextProperties as unknown as Record<string, unknown>),
        properties: nextProperties as unknown as Record<string, unknown>,
      };
    });
  }

  function updateTechnicalIndicatorConditionInput(
    propertyName: "inputPrimaryNodeId" | "inputFastNodeId" | "inputSlowNodeId",
    value: string,
  ): void {
    if (!isTechnicalIndicatorCondition.value) {
      return;
    }

    updateTechnicalIndicatorCondition((properties) => {
      const nextProperties = { ...properties };
      const normalized = value.trim();
      if (normalized === "") {
        delete nextProperties[propertyName];
      } else {
        nextProperties[propertyName] = normalized;
      }
      return nextProperties;
    });
  }

  function updateStopLoss(
    mutator: (properties: Record<string, unknown>) => Record<string, unknown>,
  ): void {
    if (selectedVisualKind.value !== "stopLoss") {
      return;
    }

    mutateSelectedVisualNode((node) => {
      const nextProperties = normalizeStopLossBlockProperties(mutator({ ...node.properties }));
      return {
        ...node,
        text: nextStopLossNodeText(nextProperties as unknown as Record<string, unknown>),
        properties: nextProperties as unknown as Record<string, unknown>,
      };
    });
  }

  return {
    selectedVisualKind,
    selectedVisualBlock,
    selectedVisualNodeText,
    selectedVisualNodeMessage,
    showsCodeInput,
    selectedVisualNodeCode,
    showsThresholdInput,
    selectedVisualNodeThreshold,
    showsPeriodInput,
    selectedVisualNodePeriod,
    showsMacdInputs,
    showsTechnicalIndicatorMacdInputs,
    showsMovingAverageTypeInput,
    showsIndicatorVariableNameInput,
    indicatorVariableNamePlaceholder,
    selectedIndicatorVariableName,
    showsIndicatorPrimaryInputSelect,
    showsIndicatorFastInputSelect,
    showsIndicatorSlowInputSelect,
    indicatorGetterOptions,
    selectedIndicatorPrimaryInputNodeId,
    selectedIndicatorFastInputNodeId,
    selectedIndicatorSlowInputNodeId,
    selectedStopLossMode,
    selectedStopLossDirection,
    selectedStopLossTimeUnit,
    selectedStopLossWindowPolicy,
    selectedMacdFastPeriod,
    selectedMacdSlowPeriod,
    selectedMacdSignalPeriod,
    selectedMovingAverageType,
    selectedIndicatorPeriodUnit,
    showsMultiplierInput,
    selectedBollingerMultiplier,
    showsConditionModeInput,
    showsIndicatorTypeInput,
    showsPatternTypeInput,
    showsLookbackInput,
    selectedIndicatorType,
    selectedIndicatorConditionMode,
    selectedIndicatorOperator,
    selectedIndicatorPatternType,
    selectedIndicatorLookback,
    showsPlaceOrderInputs,
    selectedPlaceOrderSide,
    selectedPlaceOrderType,
    selectedPlaceOrderEntryPositionPolicy,
    selectedPlaceOrderQuantityMode,
    selectedPlaceOrderQuantityValue,
    selectedPlaceOrderLimitPrice,
    showsPlaceOrderEntryPositionPolicyInput,
    showsPlaceOrderLimitPriceInput,
  };
}

function readString(value: unknown, fallback: string): string {
  return typeof value === "string" ? value : fallback;
}

function readNumberString(value: unknown): string {
  if (typeof value === "number" && Number.isFinite(value)) {
    return String(value);
  }
  if (typeof value === "string") {
    return value;
  }
  return "";
}

function normalizeInteger(value: string, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.max(1, Math.round(parsed)) : fallback;
}

function normalizeDecimal(value: string, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function nextPriceConditionNodeText(
  kind: StrategyBlockKind | null,
  threshold: number,
): string {
  if (kind === "ifCloseAbove") {
    return `收盘价 > ${threshold}`;
  }
  if (kind === "ifCloseBelow") {
    return `收盘价 < ${threshold}`;
  }
  return "条件";
}
