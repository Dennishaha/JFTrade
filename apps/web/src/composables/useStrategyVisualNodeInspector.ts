import type { StrategyVisualNodeDocument } from "@jftrade/ui-contracts";
import { computed, type ComputedRef } from "vue";

import {
  getStrategyBlockDefinition,
  getStrategyBlockKind,
  type StrategyBlockKind,
} from "../features/strategyVisualBuilder";
import {
  isDivergencePattern,
  nextTechnicalIndicatorNodeText,
  normalizeTechnicalIndicatorProperties,
  type TechnicalIndicatorBlockProperties,
} from "../features/strategyVisualBuilderIndicatorBlock";

interface UseStrategyVisualNodeInspectorOptions {
  selectedVisualNode: ComputedRef<StrategyVisualNodeDocument | null>;
  mutateSelectedVisualNode: (
    mutator: (node: StrategyVisualNodeDocument) => StrategyVisualNodeDocument,
  ) => void;
}

export function useStrategyVisualNodeInspector(
  { selectedVisualNode, mutateSelectedVisualNode }: UseStrategyVisualNodeInspectorOptions,
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

  const showsCodeInput = computed(() => selectedVisualKind.value === "codeBlock");

  const selectedVisualNodeCode = computed({
    get: () => {
      const code = selectedVisualNode.value?.properties.code;
      return typeof code === "string" ? code : "";
    },
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        text: value.trim() === "" ? "代码块" : node.text,
        properties: {
          ...node.properties,
          code: value,
        },
      }));
    },
  });

  const technicalIndicator = computed<TechnicalIndicatorBlockProperties | null>(() => {
    if (selectedVisualKind.value !== "technicalIndicator") {
      return null;
    }
    return normalizeTechnicalIndicatorProperties(selectedVisualNode.value?.properties ?? {});
  });

  const showsIndicatorTypeInput = computed(
    () => selectedVisualKind.value === "technicalIndicator",
  );

  const showsConditionModeInput = computed(
    () => selectedVisualKind.value === "technicalIndicator",
  );

  const showsThresholdInput = computed(() => {
    if (selectedVisualKind.value === "technicalIndicator") {
      return technicalIndicator.value?.conditionMode === "numeric";
    }
    return selectedVisualKind.value === "ifCloseAbove" || selectedVisualKind.value === "ifCloseBelow";
  });

  const showsPeriodInput = computed(() => {
    if (selectedVisualKind.value !== "technicalIndicator") {
      return false;
    }
    const indicatorType = technicalIndicator.value?.indicatorType;
    return indicatorType !== "movingAverage" && indicatorType !== "macd";
  });

  const showsMacdInputs = computed(() => false);
  const showsTechnicalIndicatorMacdInputs = computed(() => {
    if (selectedVisualKind.value !== "technicalIndicator") {
      return false;
    }
    const indicatorType = technicalIndicator.value?.indicatorType;
    return indicatorType === "movingAverage" || indicatorType === "macd" || indicatorType === "kdj";
  });

  const showsMultiplierInput = computed(
    () => technicalIndicator.value?.indicatorType === "bollinger",
  );

  const showsPatternTypeInput = computed(
    () => technicalIndicator.value?.conditionMode === "pattern",
  );

  const showsLookbackInput = computed(() =>
    isDivergencePattern(technicalIndicator.value?.patternType),
  );

  const selectedIndicatorType = computed({
    get: () => technicalIndicator.value?.indicatorType ?? "rsi",
    set: (value: string) => {
      updateTechnicalIndicator((properties) => ({
        ...properties,
        indicatorType: value,
      }));
    },
  });

  const selectedIndicatorConditionMode = computed({
    get: () => technicalIndicator.value?.conditionMode ?? "numeric",
    set: (value: string) => {
      updateTechnicalIndicator((properties) => ({
        ...properties,
        conditionMode: value,
      }));
    },
  });

  const selectedIndicatorOperator = computed({
    get: () => technicalIndicator.value?.operator ?? ">",
    set: (value: string) => {
      updateTechnicalIndicator((properties) => ({
        ...properties,
        operator: value,
      }));
    },
  });

  const selectedIndicatorPatternType = computed({
    get: () => technicalIndicator.value?.patternType ?? "goldenCross",
    set: (value: string) => {
      updateTechnicalIndicator((properties) => ({
        ...properties,
        patternType: value,
      }));
    },
  });

  const selectedIndicatorLookback = computed({
    get: () => readNumberString(technicalIndicator.value?.lookback),
    set: (value: string) => {
      updateTechnicalIndicator((properties) => ({
        ...properties,
        lookback: normalizeInteger(value, 5),
      }));
    },
  });

  const selectedVisualNodeThreshold = computed({
    get: () => {
      const threshold = selectedVisualNode.value?.properties.threshold;
      return readNumberString(threshold);
    },
    set: (value: string) => {
      const threshold = normalizeDecimal(value, 0);
      if (selectedVisualKind.value === "technicalIndicator") {
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
      if (selectedVisualKind.value !== "technicalIndicator") {
        return "";
      }
      const indicator = technicalIndicator.value;
      if (indicator === null) {
        return "";
      }
      return readNumberString(indicator.period);
    },
    set: (value: string) => {
      if (selectedVisualKind.value !== "technicalIndicator") {
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
      const indicator = technicalIndicator.value;
      if (indicator === null) {
        return "";
      }
      if (indicator.indicatorType === "movingAverage" || indicator.indicatorType === "macd") {
        return readNumberString(indicator.fastPeriod);
      }
      return "";
    },
    set: (value: string) => {
      updateTechnicalIndicator((properties) => ({
        ...properties,
        fastPeriod: normalizeInteger(value, properties.indicatorType === "movingAverage" ? 5 : 12),
      }));
    },
  });

  const selectedMacdSlowPeriod = computed({
    get: () => {
      const indicator = technicalIndicator.value;
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
      const indicator = technicalIndicator.value;
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
    get: () => readNumberString(technicalIndicator.value?.multiplier),
    set: (value: string) => {
      updateTechnicalIndicator((properties) => ({
        ...properties,
        multiplier: normalizeDecimal(value, 2),
      }));
    },
  });

  const showsPlaceOrderInputs = computed(
    () => selectedVisualKind.value === "placeOrder",
  );

  const selectedPlaceOrderSide = computed({
    get: () => readString(selectedVisualNode.value?.properties.side, "BUY"),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          side: value,
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

  const selectedPlaceOrderQuantityMode = computed({
    get: () => readString(selectedVisualNode.value?.properties.quantityMode, "shares"),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          quantityMode: value,
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
    selectedMacdFastPeriod,
    selectedMacdSlowPeriod,
    selectedMacdSignalPeriod,
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
    selectedPlaceOrderQuantityMode,
    selectedPlaceOrderQuantityValue,
    selectedPlaceOrderLimitPrice,
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
