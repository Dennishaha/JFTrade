import { computed, ref, watch, type Ref } from "vue";

import {
  KLINE_PERIODS,
  normalizeChartType,
  type ChartType,
} from "@/charting/kline";
import {
  backtestInstrumentTypeForSecurityType,
  categoryMarketForUser,
} from "@/composables/market-data/instrumentPresentation";
import type { InstrumentResolutionCandidate } from "@/types";
import type { BacktestFormState } from "./useBacktestRuns";
import type { BacktestStrategyDefinition } from "./useBacktestComparison";
import { formatStrategyVersion } from "./useBacktestComparison";
import type { BacktestProviderCapabilities } from "./backtestProviderSettings";
import {
  BACKTEST_BROKER_FEE_MODE_OPTIONS,
  BACKTEST_MARKET_FEE_MODE_OPTIONS,
  canonicalBacktestInstrumentInput,
  parseBacktestFeeRules,
  readStoredBacktestFormPreferences,
  supportsExtendedHoursForInterval as supportsInterval,
  writeStoredBacktestFormPreferences,
  type StoredBacktestFormPreferences,
} from "./backtestPagePreferences";

interface BacktestFormInput {
  definitions: Ref<BacktestStrategyDefinition[]>;
  quoteCurrencyForMarket: (market: string) => string;
  supportsExtendedHoursForMarket: (market: string) => boolean;
  providerCapabilities: Ref<BacktestProviderCapabilities | null>;
}

export function useBacktestForm(input: BacktestFormInput) {
  const storedBacktestFormPreferences = readStoredBacktestFormPreferences();
  const selectedDefinitionId = ref(
    storedBacktestFormPreferences.selectedDefinitionId,
  );
  const selectedMarket = ref(storedBacktestFormPreferences.selectedMarket);
  const codeInput = ref(storedBacktestFormPreferences.codeInput);
  const instrumentSearchQuery = ref(
    canonicalBacktestInstrumentInput(
      storedBacktestFormPreferences.selectedMarket,
      storedBacktestFormPreferences.codeInput,
    ),
  );
  const interval = ref(storedBacktestFormPreferences.interval);
  const chartType = ref<ChartType>(storedBacktestFormPreferences.chartType);
  const startDate = ref(storedBacktestFormPreferences.startDate);
  const endDate = ref(storedBacktestFormPreferences.endDate);
  const initialBalance = ref(storedBacktestFormPreferences.initialBalance);
  const instrumentType = ref(storedBacktestFormPreferences.instrumentType);
  const rehabType = ref(storedBacktestFormPreferences.rehabType);
  const useExtendedHours = ref(storedBacktestFormPreferences.useExtendedHours);
  const brokerFeeMode = ref(storedBacktestFormPreferences.brokerFeeMode);
  const marketFeeMode = ref(storedBacktestFormPreferences.marketFeeMode);
  const brokerFeeRulesText = ref(
    storedBacktestFormPreferences.brokerFeeRulesText,
  );
  const marketFeeRulesText = ref(
    storedBacktestFormPreferences.marketFeeRulesText,
  );

  const selectedDefinition = computed(() =>
    input.definitions.value.find(
      (definition) => definition.id === selectedDefinitionId.value,
    ),
  );
  const displayInstrumentId = computed(() =>
    canonicalBacktestInstrumentInput(selectedMarket.value, codeInput.value),
  );
  const instrumentSelectionResolved = computed(() => {
    const draft = instrumentSearchQuery.value
      .trim()
      .toUpperCase()
      .replace(":", ".");
    return draft !== "" && draft === displayInstrumentId.value;
  });
  const periodLabel = computed(
    () =>
      KLINE_PERIODS.find((period) => period.value === interval.value)?.label ??
      interval.value,
  );
  const supportsExtendedHoursForInterval = (
    market: string,
    intervalValue: string,
  ) =>
    supportsInterval(
      market,
      intervalValue,
      input.supportsExtendedHoursForMarket,
    );
  const extendedHoursSupported = computed(() =>
    input.providerCapabilities.value?.extendedHours === true &&
    supportsExtendedHoursForInterval(selectedMarket.value, interval.value),
  );
  const availableKlinePeriods = computed(() => {
    const supported = input.providerCapabilities.value?.candleIntervals ?? [];
    if (supported.length === 0) return KLINE_PERIODS;
    return KLINE_PERIODS.filter((period) => supported.includes(period.value));
  });
  const availableRehabTypes = computed(() => {
    const options = [
      { value: "forward", label: "前复权" },
      { value: "backward", label: "后复权" },
      { value: "none", label: "不复权" },
    ];
    const supported = input.providerCapabilities.value?.priceAdjustments;
    // 能力未知（未加载或供应商未声明）时只保守提供不复权。
    if (!Array.isArray(supported) || supported.length === 0) {
      return options.filter((option) => option.value === "none");
    }
    return options.filter((option) => supported.includes(option.value));
  });
  const extendedHoursHint = computed(() => {
    if (!extendedHoursSupported.value) {
      return "当前市场或周期不支持扩展交易时段回放与对应同步版本。";
    }
    return useExtendedHours.value
      ? "US 盘前、盘后与夜盘数据会写入 extended 版本，并参与本次回测回放/高周期合成。"
      : "仅使用 US regular session 数据；同步会写入 regular-only 版本，回测不会混入扩展时段 bar。";
  });
  const quoteCurrency = computed(() =>
    input.quoteCurrencyForMarket(selectedMarket.value),
  );
  const brokerFeeRules = computed(() =>
    parseBacktestFeeRules(brokerFeeRulesText.value),
  );
  const marketFeeRules = computed(() =>
    parseBacktestFeeRules(marketFeeRulesText.value),
  );
  const costModeSummary = computed(() => {
    const broker =
      BACKTEST_BROKER_FEE_MODE_OPTIONS.find(
        (item) => item.value === brokerFeeMode.value,
      )?.title ?? brokerFeeMode.value;
    const market =
      BACKTEST_MARKET_FEE_MODE_OPTIONS.find(
        (item) => item.value === marketFeeMode.value,
      )?.title ?? marketFeeMode.value;
    return `券商 ${broker} / 市场 ${market}`;
  });
  const backtestFormState = computed<BacktestFormState>(() => ({
    definitionId: selectedDefinitionId.value,
    definitionVersion: selectedDefinition.value?.version?.trim() ?? "",
    market: selectedMarket.value.trim().toUpperCase(),
    code: instrumentSelectionResolved.value
      ? codeInput.value.trim().toUpperCase()
      : "",
    instrumentId:
      instrumentSelectionResolved.value &&
      (codeInput.value.includes(".") || codeInput.value.includes(":"))
        ? codeInput.value.trim().toUpperCase()
        : "",
    instrumentType: instrumentType.value,
    interval: interval.value,
    chartType: interval.value === "tick" ? "standard" : chartType.value,
    startDate: startDate.value,
    endDate: endDate.value,
    initialBalance: initialBalance.value,
    rehabType: rehabType.value,
    useExtendedHours: useExtendedHours.value,
    brokerFeeMode: brokerFeeMode.value,
    marketFeeMode: marketFeeMode.value,
    brokerFeeRules: brokerFeeRules.value,
    marketFeeRules: marketFeeRules.value,
  }));

  function handleResolvedBacktestInstrument(
    candidate: InstrumentResolutionCandidate,
  ): void {
    selectedMarket.value = categoryMarketForUser(candidate.market);
    codeInput.value = candidate.instrumentId;
    instrumentSearchQuery.value = candidate.instrumentId;
    instrumentType.value = backtestInstrumentTypeForSecurityType(
      candidate.securityType,
    );
  }

  function quoteCurrencyFromInstrumentId(instrumentId: string | undefined) {
    const market = (instrumentId ?? "").trim().toUpperCase().split(".")[0] ?? "";
    return input.quoteCurrencyForMarket(market);
  }

  function resolveRunQuoteCurrency(run: {
    request: { symbol: string };
    result?: { quoteCurrency?: string | undefined } | undefined;
  }) {
    return (
      run.result?.quoteCurrency?.trim() ||
      quoteCurrencyFromInstrumentId(run.request.symbol)
    );
  }

  function resolveRunSessionMode(run: {
    request: {
      symbol: string;
      interval: string;
      useExtendedHours?: boolean | undefined;
    };
  }) {
    const market = run.request.symbol.trim().toUpperCase().split(".")[0] ?? "";
    if (!supportsExtendedHoursForInterval(market, run.request.interval)) {
      return "常规时段";
    }
    return run.request.useExtendedHours ? "含扩展时段" : "仅常规时段";
  }

  function resolveStrategyDefinition(definitionId: string | undefined) {
    if (!definitionId) return null;
    return (
      input.definitions.value.find(
        (definition) => definition.id === definitionId,
      ) ?? null
    );
  }

  function resolveStrategyName(definitionId: string | undefined) {
    return resolveStrategyDefinition(definitionId)?.name ?? definitionId ?? "未命名策略";
  }

  function resolveBacktestStrategyVersionNotice(run: {
    request: { definitionId: string; definitionVersion?: string | undefined };
  }) {
    const recordedVersion = (run.request.definitionVersion ?? "").trim();
    if (recordedVersion === "") return "";
    const currentDefinition = resolveStrategyDefinition(run.request.definitionId);
    if (currentDefinition == null) {
      return `历史策略回测结果：当前策略定义已不存在；该结果基于策略 ${formatStrategyVersion(recordedVersion)}。`;
    }
    const currentVersion = currentDefinition.version.trim();
    if (currentVersion === "" || currentVersion === recordedVersion) return "";
    return `旧版本策略回测结果：当时策略 ${formatStrategyVersion(recordedVersion)}，当前已更新到 ${formatStrategyVersion(currentVersion)}。`;
  }

  watch(
    [extendedHoursSupported, input.providerCapabilities],
    ([supported, capabilities]) => {
      const marketAndIntervalSupported = supportsExtendedHoursForInterval(
        selectedMarket.value,
        interval.value,
      );
      if (!supported && (capabilities != null || !marketAndIntervalSupported)) {
        useExtendedHours.value = false;
      }
    },
    { immediate: true },
  );
  watch(interval, (value) => {
    if (value === "tick") chartType.value = "standard";
  }, { immediate: true });
  watch(
    availableKlinePeriods,
    (periods) => {
      if (
        periods.length > 0 &&
        !periods.some((period) => period.value === interval.value)
      ) {
        interval.value = periods[0]!.value;
      }
    },
    { immediate: true },
  );
  watch(
    [availableRehabTypes, input.providerCapabilities],
    ([options, capabilities]) => {
      // 供应商能力尚未加载时不要清空本地保存的选择。
      if (capabilities == null) return;
      if (
        options.length > 0 &&
        !options.some((option) => option.value === rehabType.value)
      ) {
        rehabType.value = options.some((option) => option.value === "none")
          ? "none"
          : options[0]!.value;
      }
    },
    { immediate: true },
  );
  watch(
    [
      selectedDefinitionId,
      selectedMarket,
      codeInput,
      interval,
      chartType,
      startDate,
      endDate,
      initialBalance,
      instrumentType,
      rehabType,
      useExtendedHours,
      brokerFeeMode,
      marketFeeMode,
      brokerFeeRulesText,
      marketFeeRulesText,
    ],
    (values) => {
      const [
        definitionId,
        market,
        code,
        selectedInterval,
        selectedChartType,
        selectedStartDate,
        selectedEndDate,
        balance,
        selectedInstrumentType,
        selectedRehabType,
        extendedHours,
        selectedBrokerFeeMode,
        selectedMarketFeeMode,
        brokerRules,
        marketRules,
      ] = values;
      const preferences: StoredBacktestFormPreferences = {
        selectedDefinitionId: definitionId.trim(),
        selectedMarket: market.trim().toUpperCase(),
        codeInput: code.trim().toUpperCase(),
        interval: selectedInterval.trim(),
        chartType: normalizeChartType(selectedChartType),
        startDate: selectedStartDate,
        endDate: selectedEndDate,
        initialBalance: balance,
        instrumentType: selectedInstrumentType,
        rehabType: selectedRehabType,
        useExtendedHours: extendedHours,
        brokerFeeMode: selectedBrokerFeeMode,
        marketFeeMode: selectedMarketFeeMode,
        brokerFeeRulesText: brokerRules,
        marketFeeRulesText: marketRules,
      };
      writeStoredBacktestFormPreferences(preferences);
    },
    { immediate: true },
  );

  return {
    backtestFormState,
    availableKlinePeriods,
    availableRehabTypes,
    brokerFeeMode,
    brokerFeeRules,
    brokerFeeRulesText,
    chartType,
    codeInput,
    costModeSummary,
    displayInstrumentId,
    endDate,
    extendedHoursHint,
    extendedHoursSupported,
    handleResolvedBacktestInstrument,
    initialBalance,
    instrumentSearchQuery,
    instrumentSelectionResolved,
    instrumentType,
    interval,
    marketFeeMode,
    marketFeeRules,
    marketFeeRulesText,
    periodLabel,
    quoteCurrency,
    quoteCurrencyFromInstrumentId,
    rehabType,
    resolveBacktestStrategyVersionNotice,
    resolveRunQuoteCurrency,
    resolveRunSessionMode,
    resolveStrategyDefinition,
    resolveStrategyName,
    selectedDefinition,
    selectedDefinitionId,
    selectedMarket,
    startDate,
    storedBacktestFormPreferences,
    supportsExtendedHoursForInterval,
    useExtendedHours,
  };
}
