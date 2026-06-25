import type {
  StrategyVisualModelDocument,
  StrategyVisualNodeDocument,
} from "@/contracts";
import { computed, ref, type ComputedRef } from "vue";

import {
  getStrategyBlockDefinition,
  getStrategyBlockKind,
  type StrategyBlockKind,
} from "../features/strategyVisualBuilder";
import {
  nextCollectionStatNodeText,
  nextDerivedSeriesNodeText,
  nextMtfSeriesNodeText,
  nextSessionFilterNodeText,
  nextSeriesConditionNodeText,
  nextStateUpdateNodeText,
  nextStateVariableNodeText,
  nextStopLossNodeText,
  nextStrategyInputNodeText,
  nextTimeFilterNodeText,
  normalizeCollectionStatBlockProperties,
  normalizeDerivedSeriesBlockProperties,
  normalizeMtfSeriesBlockProperties,
  normalizeSessionFilterBlockProperties,
  normalizeSeriesConditionBlockProperties,
  normalizeSeriesConditionMode,
  normalizeSeriesConditionOperator,
  normalizeSeriesSource,
  normalizeStateUpdateBlockProperties,
  normalizeStateVariableBlockProperties,
  normalizeStopLossBlockProperties,
  normalizeStrategyInputBlockProperties,
  normalizeTimeFilterBlockProperties,
} from "../features/strategyVisualBuilderCatalog";
import {
  getTechnicalIndicatorDefinition,
  isDivergencePattern,
  nextGetTechnicalIndicatorNodeText,
  nextTechnicalIndicatorConditionNodeText,
  normalizeIndicatorTimeframe,
  normalizeGetTechnicalIndicatorProperties,
  normalizeTechnicalIndicatorConditionProperties,
  type GetTechnicalIndicatorBlockProperties,
  type TechnicalIndicatorConditionBlockProperties,
} from "../features/strategyVisualBuilderIndicatorBlock";
import {
  listStrategyIndicatorGetterOptions,
  suggestStrategyIndicatorVariableName,
} from "../features/strategyVisualBuilderIndicatorReferences";
import {
  literalExpression,
  parsePineExpressionToVisualExpression,
  referenceExpression,
  renderVisualExpressionToPine,
  type VisualExpression,
  type VisualExpressionReference,
} from "../features/strategyVisualBuilderExpressions";
import {
  normalizeEntryPositionPolicy,
  normalizeOrderSide,
  normalizePineOrderAction,
  normalizePineRiskAllowEntryDirection,
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
  const selectedExpressionSlot = ref("primary");

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

  const showsCodeInput = computed(() => false);

  const selectedVisualNodeCode = computed({
    get: () => {
      const code = selectedVisualNode.value?.properties.code;
      return typeof code === "string" ? code : "";
    },
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        text: value.trim() === ""
          ? "代码片段"
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

  const seriesCondition = computed(() => {
    if (selectedVisualKind.value !== "seriesCondition") {
      return null;
    }
    return normalizeSeriesConditionBlockProperties(selectedVisualNode.value?.properties ?? {});
  });

  const strategyInput = computed(() => (
    selectedVisualKind.value === "strategyInput"
      ? normalizeStrategyInputBlockProperties(selectedVisualNode.value?.properties ?? {})
      : null
  ));

  const derivedSeries = computed(() => (
    selectedVisualKind.value === "derivedSeries"
      ? normalizeDerivedSeriesBlockProperties(selectedVisualNode.value?.properties ?? {})
      : null
  ));

  const mtfSeries = computed(() => (
    selectedVisualKind.value === "mtfSeries"
      ? normalizeMtfSeriesBlockProperties(selectedVisualNode.value?.properties ?? {})
      : null
  ));

  const stateVariable = computed(() => (
    selectedVisualKind.value === "stateVariable"
      ? normalizeStateVariableBlockProperties(selectedVisualNode.value?.properties ?? {})
      : null
  ));

  const stateUpdate = computed(() => (
    selectedVisualKind.value === "stateUpdate"
      ? normalizeStateUpdateBlockProperties(selectedVisualNode.value?.properties ?? {})
      : null
  ));

  const collectionStat = computed(() => (
    selectedVisualKind.value === "collectionStat"
      ? normalizeCollectionStatBlockProperties(selectedVisualNode.value?.properties ?? {})
      : null
  ));

  const timeFilter = computed(() => (
    selectedVisualKind.value === "timeFilter"
      ? normalizeTimeFilterBlockProperties(selectedVisualNode.value?.properties ?? {})
      : null
  ));

  const sessionFilter = computed(() => (
    selectedVisualKind.value === "sessionFilter"
      ? normalizeSessionFilterBlockProperties(selectedVisualNode.value?.properties ?? {})
      : null
  ));

  const showsAdvancedPineBlockInputs = computed(() =>
    selectedVisualKind.value === "strategyInput"
    || selectedVisualKind.value === "derivedSeries"
    || selectedVisualKind.value === "mtfSeries"
    || selectedVisualKind.value === "stateVariable"
    || selectedVisualKind.value === "stateUpdate"
    || selectedVisualKind.value === "collectionStat",
  );

  const showsTimeFilterInputs = computed(() => selectedVisualKind.value === "timeFilter");

  const showsSessionFilterInputs = computed(() => selectedVisualKind.value === "sessionFilter");

  const selectedIndicatorTypeValue = computed(() =>
    technicalIndicatorGetter.value?.indicatorType
    ?? technicalIndicatorCondition.value?.indicatorType
    ?? "rsi",
  );

  const selectedIndicatorDefinition = computed(() =>
    getTechnicalIndicatorDefinition(selectedIndicatorTypeValue.value),
  );

  const isTechnicalIndicatorGetter = computed(
    () => selectedVisualKind.value === "getTechnicalIndicator",
  );

  const isTechnicalIndicatorCondition = computed(
    () => selectedVisualKind.value === "technicalIndicatorCondition",
  );

  const isAnyTechnicalIndicator = computed(() =>
    isTechnicalIndicatorGetter.value
    || isTechnicalIndicatorCondition.value,
  );

  const showsIndicatorTypeInput = computed(
    () => isAnyTechnicalIndicator.value,
  );

  const showsConditionModeInput = computed(
    () => isTechnicalIndicatorCondition.value,
  );

  const showsSeriesConditionInputs = computed(
    () => selectedVisualKind.value === "seriesCondition",
  );

  const showsThresholdInput = computed(() => {
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
        || selectedIndicatorDefinition.value.parameterShape === "bollinger"
        || selectedIndicatorDefinition.value.parameterShape === "dmi"
        || selectedIndicatorDefinition.value.parameterShape === "supertrend"
        || selectedIndicatorDefinition.value.parameterShape === "linreg"
        || selectedIndicatorDefinition.value.parameterShape === "sourcePeriodMultiplier"
        || selectedIndicatorDefinition.value.parameterShape === "alma";
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
    return false;
  });

  const showsMovingAverageTypeInput = computed(() =>
    selectedIndicatorTypeValue.value === "movingAverage"
    && isTechnicalIndicatorGetter.value,
  );

  const showsIndicatorTimeframeInput = computed(() =>
    isTechnicalIndicatorGetter.value && [
      "movingAverage",
      "rsi",
      "macd",
      "atr",
      "cci",
      "bollinger",
      "stdev",
      "variance",
      "highest",
      "lowest",
      "sum",
      "mfi",
      "supertrend",
      "linreg",
      "obv",
      "pivotHigh",
      "pivotLow",
      "keltner",
      "alma",
    ].includes(selectedIndicatorTypeValue.value),
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

  const expressionReferenceOptions = computed<VisualExpressionReference[]>(() => {
    const options: VisualExpressionReference[] = [];
    for (const node of visualModel.value.nodes) {
      const kind = getStrategyBlockKind(node);
      const variableName = readVisualReferenceName(node);
      if (
        kind === null
        || variableName === null
        || (
          kind !== "getTechnicalIndicator"
          && kind !== "strategyInput"
          && kind !== "derivedSeries"
          && kind !== "mtfSeries"
          && kind !== "stateVariable"
          && kind !== "collectionStat"
        )
      ) {
        continue;
      }
      const fields = expressionFieldsForKind(kind, node.properties);
      options.push({
        value: variableName,
        label: `${node.text || variableName} · ${variableName}`,
        sourceBlockKind: kind,
        ...(fields.length === 0 ? {} : { fields }),
      });
    }
    return options;
  });

  const showsVisualExpressionInputs = computed(() =>
    selectedVisualKind.value === "seriesCondition"
    || selectedVisualKind.value === "derivedSeries"
    || selectedVisualKind.value === "stateUpdate"
    || selectedVisualKind.value === "mtfSeries"
    || selectedVisualKind.value === "collectionStat"
    || selectedVisualKind.value === "placeOrder"
    || selectedVisualKind.value === "stopLoss",
  );

  const showsMultiplierInput = computed(
    () => (
      selectedIndicatorDefinition.value.parameterShape === "bollinger"
      || selectedIndicatorDefinition.value.parameterShape === "sourcePeriodMultiplier"
    )
      && isTechnicalIndicatorGetter.value,
  );

  const showsIndicatorSourceInput = computed(() =>
    isTechnicalIndicatorGetter.value && [
      "movingAverage",
      "cci",
      "stdev",
      "variance",
      "highest",
      "lowest",
      "sum",
      "vwap",
      "mfi",
      "linreg",
      "obv",
      "pivotHigh",
      "pivotLow",
      "keltner",
      "alma",
    ].includes(selectedIndicatorTypeValue.value),
  );

  const showsIndicatorAdxSmoothingInput = computed(() =>
    isTechnicalIndicatorGetter.value && selectedIndicatorDefinition.value.parameterShape === "dmi",
  );

  const showsIndicatorFactorInput = computed(() =>
    isTechnicalIndicatorGetter.value && selectedIndicatorDefinition.value.parameterShape === "supertrend",
  );

  const showsIndicatorSarInputs = computed(() =>
    isTechnicalIndicatorGetter.value && selectedIndicatorDefinition.value.parameterShape === "sar",
  );

  const showsIndicatorOffsetInput = computed(() =>
    isTechnicalIndicatorGetter.value && (
      selectedIndicatorDefinition.value.parameterShape === "linreg"
      || selectedIndicatorDefinition.value.parameterShape === "alma"
    ),
  );

  const showsIndicatorSigmaInput = computed(() =>
    isTechnicalIndicatorGetter.value && selectedIndicatorDefinition.value.parameterShape === "alma",
  );

  const showsIndicatorPivotBarsInput = computed(() =>
    isTechnicalIndicatorGetter.value && selectedIndicatorDefinition.value.parameterShape === "pivot",
  );

  const showsPatternTypeInput = computed(
    () => technicalIndicatorCondition.value?.conditionMode === "pattern",
  );

  const showsLookbackInput = computed(() =>
    isDivergencePattern(
      technicalIndicatorCondition.value?.patternType,
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
      }
    },
  });

  const selectedIndicatorConditionMode = computed({
    get: () => technicalIndicatorCondition.value?.conditionMode ?? "numeric",
    set: (value: string) => {
      if (isTechnicalIndicatorCondition.value) {
        updateTechnicalIndicatorCondition((properties) => ({
          ...properties,
          conditionMode: value,
        }));
      }
    },
  });

  const selectedIndicatorOperator = computed({
    get: () => technicalIndicatorCondition.value?.operator ?? ">",
    set: (value: string) => {
      if (isTechnicalIndicatorCondition.value) {
        updateTechnicalIndicatorCondition((properties) => ({
          ...properties,
          operator: value,
        }));
      }
    },
  });

  const selectedIndicatorPatternType = computed({
    get: () => technicalIndicatorCondition.value?.patternType ?? "goldenCross",
    set: (value: string) => {
      if (isTechnicalIndicatorCondition.value) {
        updateTechnicalIndicatorCondition((properties) => ({
          ...properties,
          patternType: value,
        }));
      }
    },
  });

  const selectedIndicatorLookback = computed({
    get: () => readNumberString(technicalIndicatorCondition.value?.lookback),
    set: (value: string) => {
      if (isTechnicalIndicatorCondition.value) {
        updateTechnicalIndicatorCondition((properties) => ({
          ...properties,
          lookback: normalizeInteger(value, 5),
        }));
      }
    },
  });

  const selectedSeriesConditionMode = computed({
    get: () => seriesCondition.value?.mode ?? "compare",
    set: (value: string) => {
      updateSeriesCondition((properties) => ({
        ...properties,
        mode: normalizeSeriesConditionMode(value),
      }));
    },
  });

  const selectedSeriesConditionSource = computed({
    get: () => seriesCondition.value?.source ?? "close",
    set: (value: string) => {
      updateSeriesCondition((properties) => ({
        ...properties,
        source: normalizeSeriesSource(value),
      }));
    },
  });

  const selectedSeriesConditionOperator = computed({
    get: () => seriesCondition.value?.operator ?? ">",
    set: (value: string) => {
      updateSeriesCondition((properties) => ({
        ...properties,
        operator: normalizeSeriesConditionOperator(value),
      }));
    },
  });

  const selectedSeriesConditionThreshold = computed({
    get: () => readNumberString(seriesCondition.value?.threshold),
    set: (value: string) => {
      updateSeriesCondition((properties) => ({
        ...properties,
        threshold: normalizeDecimal(value, 0),
      }));
    },
  });

  const selectedSeriesConditionLength = computed({
    get: () => readNumberString(seriesCondition.value?.length),
    set: (value: string) => {
      updateSeriesCondition((properties) => ({
        ...properties,
        length: normalizeInteger(value, 3),
      }));
    },
  });

  const selectedSeriesConditionEventSource = computed({
    get: () => seriesCondition.value?.eventSource ?? "close",
    set: (value: string) => {
      updateSeriesCondition((properties) => ({
        ...properties,
        eventSource: normalizeSeriesSource(value),
      }));
    },
  });

  const selectedSeriesConditionEventOperator = computed({
    get: () => seriesCondition.value?.eventOperator ?? ">",
    set: (value: string) => {
      updateSeriesCondition((properties) => ({
        ...properties,
        eventOperator: normalizeSeriesConditionOperator(value),
      }));
    },
  });

  const selectedSeriesConditionEventThreshold = computed({
    get: () => readNumberString(seriesCondition.value?.eventThreshold),
    set: (value: string) => {
      updateSeriesCondition((properties) => ({
        ...properties,
        eventThreshold: normalizeDecimal(value, 520),
      }));
    },
  });

  const selectedSeriesConditionValueSource = computed({
    get: () => seriesCondition.value?.valueSource ?? "close",
    set: (value: string) => {
      updateSeriesCondition((properties) => ({
        ...properties,
        valueSource: normalizeSeriesSource(value),
      }));
    },
  });

  const selectedSeriesConditionOccurrence = computed({
    get: () => readNumberString(seriesCondition.value?.occurrence),
    set: (value: string) => {
      updateSeriesCondition((properties) => ({
        ...properties,
        occurrence: normalizeNonNegativeInteger(value, 0),
      }));
    },
  });

  const selectedAdvancedVariableName = computed({
    get: () =>
      strategyInput.value?.variableName
      ?? derivedSeries.value?.variableName
      ?? mtfSeries.value?.variableName
      ?? stateVariable.value?.variableName
      ?? stateUpdate.value?.variableName
      ?? collectionStat.value?.variableName
      ?? "",
    set: (value: string) => {
      updateAdvancedPineBlock({ variableName: normalizePineIdentifier(value, "signal") });
    },
  });

  const selectedAdvancedMode = computed({
    get: () =>
      strategyInput.value?.inputType
      ?? derivedSeries.value?.mode
      ?? mtfSeries.value?.expressionType
      ?? stateVariable.value?.valueType
      ?? collectionStat.value?.statFunction
      ?? "",
    set: (value: string) => {
      switch (selectedVisualKind.value) {
        case "strategyInput":
          updateAdvancedPineBlock({ inputType: value });
          break;
        case "derivedSeries":
          updateAdvancedPineBlock({ mode: value });
          break;
        case "mtfSeries":
          updateAdvancedPineBlock({ expressionType: value });
          break;
        case "stateVariable":
          updateAdvancedPineBlock({ valueType: value });
          break;
        case "collectionStat":
          updateAdvancedPineBlock({ statFunction: value });
          break;
      }
    },
  });

  const selectedAdvancedDefaultValue = computed({
    get: () =>
      readUnknownString(strategyInput.value?.defaultValue)
      || readUnknownString(stateVariable.value?.initialValue),
    set: (value: string) => {
      if (selectedVisualKind.value === "strategyInput") {
        updateAdvancedPineBlock({ defaultValue: value });
        return;
      }
      if (selectedVisualKind.value === "stateVariable") {
        updateAdvancedPineBlock({ initialValue: parseStateInitialValue(value, stateVariable.value?.valueType ?? "bool") });
      }
    },
  });

  const selectedAdvancedTimeframe = computed({
    get: () => mtfSeries.value?.timeframe ?? "",
    set: (value: string) => {
      updateAdvancedPineBlock({ timeframe: normalizePlainToken(value, "D") });
    },
  });

  const selectedAdvancedSource = computed({
    get: () =>
      derivedSeries.value?.source
      ?? mtfSeries.value?.source
      ?? collectionStat.value?.sourceA
      ?? "close",
    set: (value: string) => {
      if (selectedVisualKind.value === "collectionStat") {
        updateAdvancedPineBlock({ sourceA: normalizeSeriesSource(value) });
        return;
      }
      updateAdvancedPineBlock({ source: normalizeSeriesSource(value) });
    },
  });

  const selectedAdvancedSecondarySource = computed({
    get: () => collectionStat.value?.sourceB ?? "open",
    set: (value: string) => {
      updateAdvancedPineBlock({ sourceB: normalizeSeriesSource(value, "open") });
    },
  });

  const selectedAdvancedTertiarySource = computed({
    get: () => collectionStat.value?.sourceC ?? "high",
    set: (value: string) => {
      updateAdvancedPineBlock({ sourceC: normalizeSeriesSource(value, "high") });
    },
  });

  const selectedAdvancedNumber = computed({
    get: () => {
      if (collectionStat.value?.statFunction === "percentile") {
        return readNumberString(collectionStat.value.percentile);
      }
      if (derivedSeries.value?.mode === "nz") {
        return readNumberString(derivedSeries.value.fallbackValue);
      }
      if (derivedSeries.value !== null) {
        return readNumberString(derivedSeries.value.historyOffset);
      }
      if (mtfSeries.value !== null) {
        return readNumberString(mtfSeries.value.historyOffset);
      }
      return "";
    },
    set: (value: string) => {
      if (selectedVisualKind.value === "collectionStat") {
        updateAdvancedPineBlock({ percentile: normalizePercent(value, 50) });
        return;
      }
      if (selectedVisualKind.value === "derivedSeries" && derivedSeries.value?.mode === "nz") {
        updateAdvancedPineBlock({ fallbackValue: normalizeDecimal(value, 0) });
        return;
      }
      updateAdvancedPineBlock({ historyOffset: normalizeNonNegativeInteger(value, 1) });
    },
  });

  const selectedAdvancedExpression = computed({
    get: () =>
      stateUpdate.value?.expression
      ?? mtfSeries.value?.indicatorExpression
      ?? derivedSeries.value?.leftExpression
      ?? strategyInput.value?.title
      ?? "",
    set: (value: string) => {
      const expression = normalizeSafeInspectorExpression(value, "close");
      switch (selectedVisualKind.value) {
        case "stateUpdate":
          updateAdvancedPineBlock({ expression });
          break;
        case "mtfSeries":
          updateAdvancedPineBlock({ indicatorExpression: expression, expressionType: "indicator" });
          break;
        case "derivedSeries":
          updateAdvancedPineBlock({ leftExpression: expression });
          break;
        case "strategyInput":
          updateAdvancedPineBlock({ title: value.trim() || "Input" });
          break;
      }
    },
  });

  const selectedAdvancedOption = computed({
    get: () =>
      mtfSeries.value?.mtfField
      ?? derivedSeries.value?.mathFunction
      ?? derivedSeries.value?.crossFunction
      ?? derivedSeries.value?.operator
      ?? "",
    set: (value: string) => {
      if (selectedVisualKind.value === "mtfSeries") {
        updateAdvancedPineBlock({ mtfField: normalizePineField(value) });
        return;
      }
      if (selectedVisualKind.value === "derivedSeries") {
        if (derivedSeries.value?.mode === "math") {
          updateAdvancedPineBlock({ mathFunction: value });
          return;
        }
        if (derivedSeries.value?.mode === "cross") {
          updateAdvancedPineBlock({ crossFunction: value });
          return;
        }
        if (derivedSeries.value?.mode === "arithmetic") {
          updateAdvancedPineBlock({ operator: value });
        }
      }
    },
  });

  const selectedAdvancedReference = computed({
    get: () => "",
    set: (value: string) => {
      if (value.trim() === "") {
        return;
      }
      if (selectedVisualKind.value === "stateUpdate") {
        updateAdvancedPineBlock({ expression: value });
        return;
      }
      if (selectedVisualKind.value === "mtfSeries") {
        updateAdvancedPineBlock({ expressionType: "indicator", indicatorExpression: value });
        return;
      }
      if (selectedVisualKind.value === "derivedSeries") {
        updateAdvancedPineBlock({ leftExpression: value });
      }
    },
  });

  const selectedTimeFilterMode = computed({
    get: () => timeFilter.value?.mode ?? "between",
    set: (value: string) => {
      updateTimeFilter({ mode: value });
    },
  });

  const selectedTimeFilterStartHour = computed({
    get: () => readNumberString(timeFilter.value?.startHour),
    set: (value: string) => {
      updateTimeFilter({ startHour: normalizeBoundedInteger(value, 9, 0, 23) });
    },
  });

  const selectedTimeFilterStartMinute = computed({
    get: () => readNumberString(timeFilter.value?.startMinute),
    set: (value: string) => {
      updateTimeFilter({ startMinute: normalizeBoundedInteger(value, 30, 0, 59) });
    },
  });

  const selectedTimeFilterEndHour = computed({
    get: () => readNumberString(timeFilter.value?.endHour),
    set: (value: string) => {
      updateTimeFilter({ endHour: normalizeBoundedInteger(value, 16, 0, 23) });
    },
  });

  const selectedTimeFilterEndMinute = computed({
    get: () => readNumberString(timeFilter.value?.endMinute),
    set: (value: string) => {
      updateTimeFilter({ endMinute: normalizeBoundedInteger(value, 0, 0, 59) });
    },
  });

  const selectedTimeFilterDayOfWeek = computed({
    get: () => readNumberString(timeFilter.value?.dayOfWeek),
    set: (value: string) => {
      updateTimeFilter({ dayOfWeek: normalizeBoundedInteger(value, 2, 1, 7) });
    },
  });

  const selectedSessionFilterScope = computed({
    get: () => sessionFilter.value?.scope ?? "market",
    set: (value: string) => {
      updateSessionFilter({ scope: value });
    },
  });

  const expressionSlotOptions = computed(() => {
    switch (selectedVisualKind.value) {
      case "seriesCondition":
        return [
          { value: "primary", label: "左值 / Source" },
          { value: "secondary", label: "右值 / Threshold" },
          { value: "event", label: "事件表达式" },
          { value: "value", label: "Valuewhen 取值" },
        ];
      case "derivedSeries":
        return [
          { value: "primary", label: "左输入" },
          { value: "secondary", label: "右输入" },
          { value: "fallback", label: "NZ 兜底" },
        ];
      case "collectionStat":
        return [
          { value: "primary", label: "Source A" },
          { value: "secondary", label: "Source B" },
          { value: "tertiary", label: "Source C" },
        ];
      case "placeOrder":
        return [
          { value: "primary", label: "Limit 价格" },
          { value: "secondary", label: "Stop 价格" },
        ];
      case "stopLoss":
        return [
          { value: "primary", label: "Stop 价格" },
          { value: "secondary", label: "Take-profit 价格" },
          { value: "trailing", label: "Trailing 距离" },
        ];
      case "mtfSeries":
        return [{ value: "primary", label: "MTF 表达式" }];
      case "stateUpdate":
        return [{ value: "primary", label: "更新表达式" }];
      default:
        return [{ value: "primary", label: "表达式" }];
    }
  });

  const selectedExpressionReference = computed({
    get: () => "",
    set: (value: string) => {
      if (value.trim() === "") {
        return;
      }
      applyVisualExpressionToSelectedSlot(referenceExpression(value));
    },
  });

  const selectedExpressionField = computed({
    get: () => "",
    set: (value: string) => {
      if (value.trim() === "") {
        return;
      }
      applyVisualExpressionToSelectedSlot({
        kind: "field",
        target: currentVisualExpressionForSlot(selectedExpressionSlot.value),
        field: normalizePineIdentifier(value, "value"),
      });
    },
  });

  const selectedExpressionOperator = computed({
    get: () => {
      if (selectedVisualKind.value === "seriesCondition") {
        return seriesCondition.value?.operator ?? ">";
      }
      if (selectedVisualKind.value === "derivedSeries") {
        return derivedSeries.value?.operator ?? "-";
      }
      return ">";
    },
    set: (value: string) => {
      if (selectedVisualKind.value === "seriesCondition") {
        updateSeriesCondition((properties) => ({
          ...properties,
          operator: value === "<" ? "<" : ">",
        }));
        return;
      }
      if (selectedVisualKind.value === "derivedSeries") {
        updateAdvancedPineBlock({ operator: ["+", "-", "*", "/"].includes(value) ? value : "-" });
      }
    },
  });

  const selectedExpressionFunction = computed({
    get: () => "",
    set: (value: string) => {
      const functionName = normalizeExpressionFunction(value);
      const args = functionName.startsWith("math.") && functionName !== "math.min" && functionName !== "math.max"
        ? [currentVisualExpressionForSlot(selectedExpressionSlot.value)]
        : [currentPrimaryVisualExpression(), currentSecondaryVisualExpression()];
      applyVisualExpressionToSelectedSlot({ kind: "call", functionName, args });
    },
  });

  const selectedExpressionLiteral = computed({
    get: () => "",
    set: (value: string) => {
      const literal = parseExpressionLiteral(value);
      applyVisualExpressionToSelectedSlot(literal);
    },
  });

  const selectedExpressionHistoryOffset = computed({
    get: () => "",
    set: (value: string) => {
      const offset = normalizeNonNegativeInteger(value, 1);
      applyVisualExpressionToSelectedSlot({
        kind: "history",
        target: currentVisualExpressionForSlot(selectedExpressionSlot.value),
        offset,
      });
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

  const selectedStopLossTakeProfitPercentage = computed({
    get: () => readNumberString(stopLoss.value?.takeProfitPercentage),
    set: (value: string) => {
      if (selectedVisualKind.value === "stopLoss") {
        updateStopLoss((properties) => ({
          ...properties,
          takeProfitPercentage: normalizeDecimal(value, 4),
        }));
      }
    },
  });

  const showsStopLossTakeProfitPercentageInput = computed(
    () => selectedVisualKind.value === "stopLoss" && stopLoss.value?.mode === "bracketExit",
  );

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
    },
  });

  const selectedMacdFastPeriod = computed({
    get: () => {
      const indicator = technicalIndicatorGetter.value;
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
      }
    },
  });

  const selectedMacdSlowPeriod = computed({
    get: () => {
      const indicator = technicalIndicatorGetter.value;
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
      }
    },
  });

  const selectedMacdSignalPeriod = computed({
    get: () => {
      const indicator = technicalIndicatorGetter.value;
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
      }
    },
  });

  const selectedBollingerMultiplier = computed({
    get: () => readNumberString(technicalIndicatorGetter.value?.multiplier),
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          multiplier: normalizeDecimal(value, 2),
        }));
      }
    },
  });

  const selectedIndicatorSource = computed({
    get: () => technicalIndicatorGetter.value?.source ?? "close",
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          source: value,
        }));
      }
    },
  });

  const selectedIndicatorAdxSmoothing = computed({
    get: () => readNumberString(technicalIndicatorGetter.value?.adxSmoothing),
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          adxSmoothing: normalizeInteger(value, 14),
        }));
      }
    },
  });

  const selectedIndicatorFactor = computed({
    get: () => readNumberString(technicalIndicatorGetter.value?.factor),
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          factor: normalizeDecimal(value, 3),
        }));
      }
    },
  });

  const selectedIndicatorSarStart = computed({
    get: () => readNumberString(technicalIndicatorGetter.value?.start),
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          start: normalizeDecimal(value, 0.02),
        }));
      }
    },
  });

  const selectedIndicatorSarIncrement = computed({
    get: () => readNumberString(technicalIndicatorGetter.value?.increment),
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          increment: normalizeDecimal(value, 0.02),
        }));
      }
    },
  });

  const selectedIndicatorSarMaximum = computed({
    get: () => readNumberString(technicalIndicatorGetter.value?.maximum),
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          maximum: normalizeDecimal(value, 0.2),
        }));
      }
    },
  });

  const selectedIndicatorOffset = computed({
    get: () => readNumberString(technicalIndicatorGetter.value?.offset),
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          offset: normalizeDecimal(value, 0),
        }));
      }
    },
  });

  const selectedIndicatorSigma = computed({
    get: () => readNumberString(technicalIndicatorGetter.value?.sigma),
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          sigma: normalizeDecimal(value, 6),
        }));
      }
    },
  });

  const selectedIndicatorLeftBars = computed({
    get: () => readNumberString(technicalIndicatorGetter.value?.leftBars),
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          leftBars: normalizeInteger(value, 2),
        }));
      }
    },
  });

  const selectedIndicatorRightBars = computed({
    get: () => readNumberString(technicalIndicatorGetter.value?.rightBars),
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          rightBars: normalizeInteger(value, 2),
        }));
      }
    },
  });

  const selectedMovingAverageType = computed({
    get: () => technicalIndicatorGetter.value?.movingAverageType ?? "MA",
    set: (value: string) => {
      if (isTechnicalIndicatorGetter.value) {
        updateTechnicalIndicatorGetter((properties) => ({
          ...properties,
          movingAverageType: value,
        }));
      }
    },
  });

  const selectedIndicatorTimeframe = computed({
    get: () => technicalIndicatorGetter.value?.timeframe ?? "",
    set: (value: string) => {
      if (!isTechnicalIndicatorGetter.value) {
        return;
      }
      updateTechnicalIndicatorGetter((properties) => {
        const nextProperties = { ...properties };
        const timeframe = normalizeIndicatorTimeframe(value);
        if (timeframe === "") {
          delete nextProperties.timeframe;
        } else {
          nextProperties.timeframe = timeframe;
        }
        return nextProperties;
      });
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
    get: () => stopLoss.value?.timeUnit ?? "bar",
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

  const selectedPlaceOrderAction = computed({
    get: () => normalizePineOrderAction(selectedVisualNode.value?.properties.orderAction),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          orderAction: normalizePineOrderAction(value),
        },
      }));
    },
  });

  const selectedPlaceOrderId = computed({
    get: () => readString(selectedVisualNode.value?.properties.orderId, ""),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          orderId: value,
        },
      }));
    },
  });

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

  const selectedPlaceOrderStopPrice = computed({
    get: () => readNumberString(selectedVisualNode.value?.properties.stopPrice),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          stopPrice: normalizeDecimal(value, 0),
        },
      }));
    },
  });

  const selectedPlaceOrderRiskAllowedDirection = computed({
    get: () => normalizePineRiskAllowEntryDirection(
      selectedVisualNode.value?.properties.riskAllowedDirection,
    ),
    set: (value: string) => {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          riskAllowedDirection: normalizePineRiskAllowEntryDirection(value),
        },
      }));
    },
  });

  const showsPlaceOrderLimitPriceInput = computed(
    () => selectedPlaceOrderAction.value === "entry" || selectedPlaceOrderAction.value === "order" || selectedPlaceOrderAction.value === "close"
      ? selectedPlaceOrderType.value === "LIMIT"
      : false,
  );

  const showsPlaceOrderStopPriceInput = computed(
    () => selectedPlaceOrderAction.value === "entry" || selectedPlaceOrderAction.value === "order",
  );

  const showsPlaceOrderQuantityInputs = computed(
    () => selectedPlaceOrderAction.value === "entry"
      || selectedPlaceOrderAction.value === "order"
      || selectedPlaceOrderAction.value === "close",
  );

  const showsPlaceOrderSideInput = computed(
    () => selectedPlaceOrderAction.value === "entry"
      || selectedPlaceOrderAction.value === "order"
      || selectedPlaceOrderAction.value === "close",
  );

  const showsPlaceOrderTargetIdInput = computed(
    () => selectedPlaceOrderAction.value === "entry"
      || selectedPlaceOrderAction.value === "order"
      || selectedPlaceOrderAction.value === "close"
      || selectedPlaceOrderAction.value === "cancel",
  );

  const showsPlaceOrderRiskDirectionInput = computed(
    () => selectedPlaceOrderAction.value === "riskAllowEntryIn",
  );

  const showsPlaceOrderEntryPositionPolicyInput = computed(() => {
    const side = normalizeOrderSide(selectedPlaceOrderSide.value);
    return selectedPlaceOrderAction.value === "entry" && (side === "BUY" || side === "SELL_SHORT");
  });

  function currentPrimaryVisualExpression(): VisualExpression {
    if (selectedVisualKind.value === "seriesCondition") {
      return seriesCondition.value?.leftExpressionAst
        ?? seriesCondition.value?.sourceExpressionAst
        ?? parsePineExpressionToVisualExpression(seriesCondition.value?.source ?? "close")
        ?? { kind: "source", source: "close" };
    }
    if (selectedVisualKind.value === "derivedSeries") {
      return derivedSeries.value?.leftExpressionAst
        ?? derivedSeries.value?.sourceExpressionAst
        ?? parsePineExpressionToVisualExpression(derivedSeries.value?.leftExpression ?? "close")
        ?? { kind: "source", source: "close" };
    }
    if (selectedVisualKind.value === "stateUpdate") {
      return stateUpdate.value?.expressionAst
        ?? parsePineExpressionToVisualExpression(stateUpdate.value?.expression ?? "close > open")
        ?? { kind: "source", source: "close" };
    }
    if (selectedVisualKind.value === "mtfSeries") {
      return mtfSeries.value?.indicatorExpressionAst
        ?? parsePineExpressionToVisualExpression(mtfSeries.value?.indicatorExpression ?? "close")
        ?? { kind: "source", source: "close" };
    }
    if (selectedVisualKind.value === "collectionStat") {
      return collectionStat.value?.sourceAExpressionAst
        ?? parsePineExpressionToVisualExpression(collectionStat.value?.sourceA ?? "close")
        ?? { kind: "source", source: "close" };
    }
    if (selectedVisualKind.value === "placeOrder") {
      return (selectedVisualNode.value?.properties.limitPriceExpressionAst as VisualExpression | undefined)
        ?? literalExpression(Number(selectedVisualNode.value?.properties.limitPrice ?? 0));
    }
    if (selectedVisualKind.value === "stopLoss") {
      return stopLoss.value?.stopPriceExpressionAst
        ?? stopLoss.value?.trailingPriceExpressionAst
        ?? { kind: "source", source: "close" };
    }
    return { kind: "source", source: "close" };
  }

  function currentSecondaryVisualExpression(): VisualExpression {
    if (selectedVisualKind.value === "seriesCondition") {
      return seriesCondition.value?.rightExpressionAst
        ?? literalExpression(seriesCondition.value?.threshold ?? 0);
    }
    if (selectedVisualKind.value === "derivedSeries") {
      return derivedSeries.value?.rightExpressionAst
        ?? parsePineExpressionToVisualExpression(derivedSeries.value?.rightExpression ?? "open")
        ?? { kind: "source", source: "open" };
    }
    if (selectedVisualKind.value === "collectionStat") {
      return collectionStat.value?.sourceBExpressionAst
        ?? parsePineExpressionToVisualExpression(collectionStat.value?.sourceB ?? "open")
        ?? { kind: "source", source: "open" };
    }
    if (selectedVisualKind.value === "placeOrder") {
      return (selectedVisualNode.value?.properties.stopPriceExpressionAst as VisualExpression | undefined)
        ?? literalExpression(Number(selectedVisualNode.value?.properties.stopPrice ?? 0));
    }
    if (selectedVisualKind.value === "stopLoss") {
      return stopLoss.value?.takeProfitPriceExpressionAst
        ?? { kind: "source", source: "close" };
    }
    return literalExpression(0);
  }

  function currentVisualExpressionForSlot(slot: string): VisualExpression {
    if (slot === "secondary") {
      return currentSecondaryVisualExpression();
    }
    if (selectedVisualKind.value === "seriesCondition" && slot === "event") {
      return seriesCondition.value?.eventExpressionAst
        ?? { kind: "source", source: "close" };
    }
    if (selectedVisualKind.value === "seriesCondition" && slot === "value") {
      return seriesCondition.value?.valueExpressionAst
        ?? { kind: "source", source: "close" };
    }
    if (selectedVisualKind.value === "derivedSeries" && slot === "fallback") {
      return derivedSeries.value?.fallbackExpressionAst
        ?? literalExpression(derivedSeries.value?.fallbackValue ?? 0);
    }
    if (selectedVisualKind.value === "collectionStat" && slot === "tertiary") {
      return collectionStat.value?.sourceCExpressionAst
        ?? { kind: "source", source: "high" };
    }
    if (selectedVisualKind.value === "stopLoss" && slot === "trailing") {
      return stopLoss.value?.trailingPriceExpressionAst
        ?? { kind: "source", source: "close" };
    }
    return currentPrimaryVisualExpression();
  }

  function applyVisualExpressionToSelectedSlot(expression: VisualExpression): void {
    switch (selectedExpressionSlot.value) {
      case "secondary":
        applySecondaryVisualExpression(expression);
        return;
      case "event":
        if (selectedVisualKind.value === "seriesCondition") {
          updateSeriesCondition((properties) => ({
            ...properties,
            eventExpressionAst: expression,
          }));
        }
        return;
      case "value":
        if (selectedVisualKind.value === "seriesCondition") {
          updateSeriesCondition((properties) => ({
            ...properties,
            valueExpressionAst: expression,
          }));
        }
        return;
      case "fallback":
        if (selectedVisualKind.value === "derivedSeries") {
          updateAdvancedPineBlock({
            fallbackExpressionAst: expression,
          });
        }
        return;
      case "tertiary":
        if (selectedVisualKind.value === "collectionStat") {
          updateAdvancedPineBlock({
            sourceCExpressionAst: expression,
          });
        }
        return;
      case "trailing":
        if (selectedVisualKind.value === "stopLoss") {
          updateStopLoss((properties) => ({
            ...properties,
            trailingPriceExpressionAst: expression,
          }));
        }
        return;
      case "primary":
      default:
        applyPrimaryVisualExpression(expression);
    }
  }

  function applyPrimaryVisualExpression(expression: VisualExpression): void {
    if (selectedVisualKind.value === "seriesCondition") {
      const rendered = renderVisualExpressionToPine(expression);
      updateSeriesCondition((properties) => ({
        ...properties,
        sourceExpressionAst: expression,
        leftExpressionAst: expression,
        source: normalizeSeriesSource(rendered),
      }));
      return;
    }
    if (selectedVisualKind.value === "derivedSeries") {
      updateAdvancedPineBlock({
        sourceExpressionAst: expression,
        leftExpression: renderVisualExpressionToPine(expression),
        leftExpressionAst: expression,
      });
      return;
    }
    if (selectedVisualKind.value === "stateUpdate") {
      updateAdvancedPineBlock({
        expression: renderVisualExpressionToPine(expression),
        expressionAst: expression,
      });
      return;
    }
    if (selectedVisualKind.value === "mtfSeries") {
      updateAdvancedPineBlock({
        expressionType: "indicator",
        indicatorExpression: renderVisualExpressionToPine(expression),
        indicatorExpressionAst: expression,
      });
      return;
    }
    if (selectedVisualKind.value === "collectionStat") {
      updateAdvancedPineBlock({
        sourceAExpressionAst: expression,
      });
      return;
    }
    if (selectedVisualKind.value === "placeOrder") {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          orderType: "LIMIT",
          limitPriceExpressionAst: expression,
        },
      }));
      return;
    }
    if (selectedVisualKind.value === "stopLoss") {
      updateStopLoss((properties) => ({
        ...properties,
        ...(properties.mode === "trailingStop"
          ? { trailingPriceExpressionAst: expression }
          : { stopPriceExpressionAst: expression }),
      }));
    }
  }

  function applySecondaryVisualExpression(expression: VisualExpression): void {
    if (selectedVisualKind.value === "seriesCondition") {
      updateSeriesCondition((properties) => ({
        ...properties,
        rightExpressionAst: expression,
        threshold: expression.kind === "literal" && typeof expression.value === "number"
          ? expression.value
          : properties.threshold,
      }));
      return;
    }
    if (selectedVisualKind.value === "derivedSeries") {
      updateAdvancedPineBlock({
        rightExpression: renderVisualExpressionToPine(expression),
        rightExpressionAst: expression,
        fallbackExpressionAst: expression,
      });
      return;
    }
    if (selectedVisualKind.value === "collectionStat") {
      updateAdvancedPineBlock({
        sourceBExpressionAst: expression,
      });
      return;
    }
    if (selectedVisualKind.value === "placeOrder") {
      mutateSelectedVisualNode((node) => ({
        ...node,
        properties: {
          ...node.properties,
          stopPriceExpressionAst: expression,
        },
      }));
      return;
    }
    if (selectedVisualKind.value === "stopLoss") {
      updateStopLoss((properties) => ({
        ...properties,
        takeProfitPriceExpressionAst: expression,
      }));
    }
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

  function updateSeriesCondition(
    mutator: (properties: Record<string, unknown>) => Record<string, unknown>,
  ): void {
    if (selectedVisualKind.value !== "seriesCondition") {
      return;
    }

    mutateSelectedVisualNode((node) => {
      const nextProperties = normalizeSeriesConditionBlockProperties(mutator({ ...node.properties }));
      return {
        ...node,
        text: nextSeriesConditionNodeText(nextProperties as unknown as Record<string, unknown>),
        properties: nextProperties as unknown as Record<string, unknown>,
      };
    });
  }

  function updateAdvancedPineBlock(nextValues: Record<string, unknown>): void {
    const kind = selectedVisualKind.value;
    if (
      kind !== "strategyInput"
      && kind !== "derivedSeries"
      && kind !== "mtfSeries"
      && kind !== "stateVariable"
      && kind !== "stateUpdate"
      && kind !== "collectionStat"
    ) {
      return;
    }

    mutateSelectedVisualNode((node) => {
      const rawProperties = { ...node.properties, ...nextValues };
      const nextProperties = normalizeAdvancedPineBlockProperties(kind, rawProperties);
      return {
        ...node,
        text: nextAdvancedPineBlockText(kind, nextProperties),
        properties: nextProperties,
      };
    });
  }

  function updateTimeFilter(nextValues: Record<string, unknown>): void {
    if (selectedVisualKind.value !== "timeFilter") {
      return;
    }
    mutateSelectedVisualNode((node) => {
      const nextProperties = normalizeTimeFilterBlockProperties({ ...node.properties, ...nextValues });
      return {
        ...node,
        text: nextTimeFilterNodeText(nextProperties as unknown as Record<string, unknown>),
        properties: nextProperties as unknown as Record<string, unknown>,
      };
    });
  }

  function updateSessionFilter(nextValues: Record<string, unknown>): void {
    if (selectedVisualKind.value !== "sessionFilter") {
      return;
    }
    mutateSelectedVisualNode((node) => {
      const nextProperties = normalizeSessionFilterBlockProperties({ ...node.properties, ...nextValues });
      return {
        ...node,
        text: nextSessionFilterNodeText(nextProperties as unknown as Record<string, unknown>),
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
    selectedStopLossTakeProfitPercentage,
    showsStopLossTakeProfitPercentageInput,
    selectedMacdFastPeriod,
    selectedMacdSlowPeriod,
    selectedMacdSignalPeriod,
    selectedMovingAverageType,
    selectedIndicatorTimeframe,
    showsMultiplierInput,
    selectedBollingerMultiplier,
    showsIndicatorSourceInput,
    showsIndicatorTimeframeInput,
    selectedIndicatorSource,
    showsIndicatorAdxSmoothingInput,
    selectedIndicatorAdxSmoothing,
    showsIndicatorFactorInput,
    selectedIndicatorFactor,
    showsIndicatorSarInputs,
    selectedIndicatorSarStart,
    selectedIndicatorSarIncrement,
    selectedIndicatorSarMaximum,
    showsIndicatorOffsetInput,
    selectedIndicatorOffset,
    showsIndicatorSigmaInput,
    selectedIndicatorSigma,
    showsIndicatorPivotBarsInput,
    selectedIndicatorLeftBars,
    selectedIndicatorRightBars,
    showsConditionModeInput,
    showsSeriesConditionInputs,
    showsIndicatorTypeInput,
    showsPatternTypeInput,
    showsLookbackInput,
    selectedIndicatorType,
    selectedIndicatorConditionMode,
    selectedIndicatorOperator,
    selectedIndicatorPatternType,
    selectedIndicatorLookback,
    selectedSeriesConditionMode,
    selectedSeriesConditionSource,
    selectedSeriesConditionOperator,
    selectedSeriesConditionThreshold,
    selectedSeriesConditionLength,
    selectedSeriesConditionEventSource,
    selectedSeriesConditionEventOperator,
    selectedSeriesConditionEventThreshold,
    selectedSeriesConditionValueSource,
    selectedSeriesConditionOccurrence,
    showsVisualExpressionInputs,
    showsAdvancedPineBlockInputs,
    expressionReferenceOptions,
    expressionSlotOptions,
    selectedExpressionSlot,
    selectedExpressionReference,
    selectedExpressionField,
    selectedExpressionOperator,
    selectedExpressionFunction,
    selectedExpressionLiteral,
    selectedExpressionHistoryOffset,
    selectedAdvancedVariableName,
    selectedAdvancedMode,
    selectedAdvancedDefaultValue,
    selectedAdvancedTimeframe,
    selectedAdvancedSource,
    selectedAdvancedSecondarySource,
    selectedAdvancedTertiarySource,
    selectedAdvancedNumber,
    selectedAdvancedExpression,
    selectedAdvancedOption,
    selectedAdvancedReference,
    showsTimeFilterInputs,
    selectedTimeFilterMode,
    selectedTimeFilterStartHour,
    selectedTimeFilterStartMinute,
    selectedTimeFilterEndHour,
    selectedTimeFilterEndMinute,
    selectedTimeFilterDayOfWeek,
    showsSessionFilterInputs,
    selectedSessionFilterScope,
    showsPlaceOrderInputs,
    selectedPlaceOrderAction,
    selectedPlaceOrderId,
    selectedPlaceOrderSide,
    selectedPlaceOrderType,
    selectedPlaceOrderEntryPositionPolicy,
    selectedPlaceOrderQuantityMode,
    selectedPlaceOrderQuantityValue,
    selectedPlaceOrderLimitPrice,
    selectedPlaceOrderStopPrice,
    selectedPlaceOrderRiskAllowedDirection,
    showsPlaceOrderEntryPositionPolicyInput,
    showsPlaceOrderLimitPriceInput,
    showsPlaceOrderStopPriceInput,
    showsPlaceOrderQuantityInputs,
    showsPlaceOrderSideInput,
    showsPlaceOrderTargetIdInput,
    showsPlaceOrderRiskDirectionInput,
  };
}

function readString(value: unknown, fallback: string): string {
  return typeof value === "string" ? value : fallback;
}

function expressionFieldsForKind(
  kind: StrategyBlockKind,
  properties: Record<string, unknown>,
): NonNullable<VisualExpressionReference["fields"]> {
  if (kind === "getTechnicalIndicator") {
    switch (properties.indicatorType) {
      case "macd":
        return [
          { value: "diff", label: "diff" },
          { value: "signal", label: "signal" },
          { value: "histogram", label: "histogram" },
        ];
      case "kdj":
        return [
          { value: "k", label: "K" },
          { value: "d", label: "D" },
          { value: "j", label: "J" },
        ];
      case "bollinger":
      case "keltner":
        return [
          { value: "upper", label: "upper" },
          { value: "basis", label: "basis" },
          { value: "lower", label: "lower" },
        ];
      case "dmi":
        return [
          { value: "plusDI", label: "+DI" },
          { value: "minusDI", label: "-DI" },
          { value: "adx", label: "ADX" },
        ];
      case "supertrend":
        return [
          { value: "direction", label: "direction" },
          { value: "line", label: "line" },
        ];
      default:
        return [];
    }
  }
  return [];
}

function normalizeExpressionFunction(value: string): "math.min" | "math.max" | "math.abs" | "math.round" | "math.floor" | "math.ceil" | "nz" | "ta.crossover" | "ta.crossunder" | "ta.cross" | "ta.barssince" | "ta.valuewhen" {
  switch (value) {
    case "math.min":
    case "math.max":
    case "math.abs":
    case "math.round":
    case "math.floor":
    case "math.ceil":
    case "nz":
    case "ta.crossover":
    case "ta.crossunder":
    case "ta.cross":
    case "ta.barssince":
    case "ta.valuewhen":
      return value;
    case "barssince":
      return "ta.barssince";
    case "valuewhen":
      return "ta.valuewhen";
    default:
      return "math.max";
  }
}

function parseExpressionLiteral(value: string): VisualExpression {
  const trimmed = value.trim();
  if (trimmed === "true" || trimmed === "false") {
    return literalExpression(trimmed === "true");
  }
  const parsed = Number(trimmed);
  if (Number.isFinite(parsed)) {
    return literalExpression(parsed);
  }
  return literalExpression(trimmed);
}

function normalizeAdvancedPineBlockProperties(
  kind: StrategyBlockKind,
  properties: Record<string, unknown>,
): Record<string, unknown> {
  switch (kind) {
    case "strategyInput":
      return normalizeStrategyInputBlockProperties(properties) as unknown as Record<string, unknown>;
    case "derivedSeries":
      return normalizeDerivedSeriesBlockProperties(properties) as unknown as Record<string, unknown>;
    case "mtfSeries":
      return normalizeMtfSeriesBlockProperties(properties) as unknown as Record<string, unknown>;
    case "stateVariable":
      return normalizeStateVariableBlockProperties(properties) as unknown as Record<string, unknown>;
    case "stateUpdate":
      return normalizeStateUpdateBlockProperties(properties) as unknown as Record<string, unknown>;
    case "collectionStat":
      return normalizeCollectionStatBlockProperties(properties) as unknown as Record<string, unknown>;
    default:
      return properties;
  }
}

function nextAdvancedPineBlockText(
  kind: StrategyBlockKind,
  properties: Record<string, unknown>,
): string {
  switch (kind) {
    case "strategyInput":
      return nextStrategyInputNodeText(properties);
    case "derivedSeries":
      return nextDerivedSeriesNodeText(properties);
    case "mtfSeries":
      return nextMtfSeriesNodeText(properties);
    case "stateVariable":
      return nextStateVariableNodeText(properties);
    case "stateUpdate":
      return nextStateUpdateNodeText(properties);
    case "collectionStat":
      return nextCollectionStatNodeText(properties);
    default:
      return "图块";
  }
}

function readVisualReferenceName(node: StrategyVisualNodeDocument): string | null {
  const value = node.properties.variableName;
  if (typeof value !== "string") {
    return null;
  }
  const normalized = normalizePineIdentifier(value, "");
  return normalized === "" ? null : normalized;
}

function readUnknownString(value: unknown): string {
  if (value === undefined || value === null) {
    return "";
  }
  return String(value);
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

function normalizePineIdentifier(value: string, fallback: string): string {
  const trimmed = value.trim();
  if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(trimmed)) {
    return trimmed;
  }
  const sanitized = trimmed
    .replace(/[^A-Za-z0-9_]/g, "_")
    .replace(/^[^A-Za-z_]+/, "");
  return sanitized === "" ? fallback : sanitized;
}

function normalizePineField(value: string): string | undefined {
  const normalized = normalizePineIdentifier(value, "");
  return normalized === "" ? undefined : normalized;
}

function normalizePlainToken(value: string, fallback: string): string {
  const trimmed = value.trim();
  return /^[A-Za-z0-9_]+$/.test(trimmed) ? trimmed : fallback;
}

function normalizeSafeInspectorExpression(value: string, fallback: string): string {
  const trimmed = value.trim();
  if (trimmed === "") {
    return fallback;
  }
  return /^[A-Za-z0-9_.,()[\]\s+\-*/<>=!&|%:]+$/.test(trimmed) ? trimmed : fallback;
}

function parseStateInitialValue(value: string, valueType: string): number | boolean | string {
  if (valueType === "number") {
    return normalizeDecimal(value, 0);
  }
  if (valueType === "bool") {
    return value.trim().toLowerCase() === "true";
  }
  return value;
}

function normalizePercent(value: string, fallback: number): number {
  return Math.min(100, Math.max(0, normalizeDecimal(value, fallback)));
}

function normalizeInteger(value: string, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.max(1, Math.round(parsed)) : fallback;
}

function normalizeNonNegativeInteger(value: string, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.max(0, Math.round(parsed)) : fallback;
}

function normalizeBoundedInteger(value: string, fallback: number, min: number, max: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed)
    ? Math.min(max, Math.max(min, Math.round(parsed)))
    : fallback;
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
