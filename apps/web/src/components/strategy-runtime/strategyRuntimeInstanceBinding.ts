import type {
    StrategyBindingInstrumentDocument,
    StrategyBrokerAccountBinding,
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
    StrategyRuntimeRiskMode,
    StrategyRuntimeRiskSettings,
} from "@/contracts";

import {
    buildBrokerAccountSelectionKey,
    type BrokerAccountSelectionOption,
} from "../../composables/consoleDataBrokerAccountSelection";
import { formatUserMarketLabel } from "../../composables/instrumentPresentation";

export function normalizeText(value: unknown): string {
    return typeof value === "string" ? value.trim() : "";
}

function normalizeInstrumentId(value: string): string {
    const normalized = normalizeText(value).toUpperCase();
    if (normalized.includes(":")) {
        const [market, code] = normalized.split(":", 2);
        if ((market ?? "") !== "" && (code ?? "") !== "") {
            return `${market}.${code}`;
        }
    }
    return normalized;
}

export function normalizeStrategyInstrumentIds(
    values: readonly string[] | null | undefined,
): string[] {
    if (!Array.isArray(values)) {
        return [];
    }
    const seen = new Set<string>();
    const result: string[] = [];
    for (const value of values) {
        const normalized = normalizeInstrumentId(value);
        if (normalized === "" || seen.has(normalized)) {
            continue;
        }
        seen.add(normalized);
        result.push(normalized);
    }
    return result;
}

function instrumentIdFromBindingInstrument(value: StrategyBindingInstrumentDocument): string {
    const market = normalizeText(value.market).toUpperCase();
    const code = normalizeText(value.code).toUpperCase();
    return market === "" || code === "" ? "" : `${market}.${code}`;
}

export function normalizeBindingInstruments(values: StrategyBindingInstrumentDocument[]): StrategyBindingInstrumentDocument[] {
    const seen = new Set<string>();
    const result: StrategyBindingInstrumentDocument[] = [];
    for (const value of values) {
        const instrumentId = instrumentIdFromBindingInstrument(value);
        if (instrumentId === "" || seen.has(instrumentId)) {
            continue;
        }
        seen.add(instrumentId);
        const [market, code] = instrumentId.split(".", 2);
        const resolvedMarket = market ?? "";
        const resolvedCode = code ?? "";
        if (resolvedMarket === "" || resolvedCode === "") {
            continue;
        }
        result.push({ market: resolvedMarket, code: resolvedCode });
    }
    return result;
}

export function bindingInstrumentsToSymbols(values: StrategyBindingInstrumentDocument[]): string[] {
    return normalizeStrategyInstrumentIds(values.map((value) => `${value.market}.${value.code}`));
}

export function splitSymbolsText(value: string): string[] {
    return value
    .split(/[\n\r\t,，;；]+/)
        .map((segment) => segment.trim())
        .filter((segment) => segment !== "");
}

export function parseStrategyInstrumentIdsText(value: string): string[] {
    return normalizeStrategyInstrumentIds(splitSymbolsText(value)).filter(
        (symbol) => bindingInstrumentFromSymbol(symbol) !== null,
    );
}

function formatTradingEnvironment(value: unknown): string {
    switch (normalizeText(value).toUpperCase()) {
        case "SIMULATE":
            return "模拟盘";
        case "REAL":
            return "实盘";
        default:
            return normalizeText(value) || "未设置";
    }
}

function asRecord(value: unknown): Record<string, unknown> | null {
    if (value === null || typeof value !== "object" || Array.isArray(value)) {
        return null;
    }
    return value as Record<string, unknown>;
}

function normalizeBrokerAccountBinding(
    value: StrategyBrokerAccountBinding | null | undefined,
): StrategyBrokerAccountBinding | null {
    if (value == null) {
        return null;
    }

    const brokerId = normalizeText(value.brokerId).toLowerCase();
    const accountId = normalizeText(value.accountId);
    const tradingEnvironment = normalizeText(value.tradingEnvironment).toUpperCase();
    const market = normalizeText(value.market).toUpperCase();

    if (brokerId === "" && accountId === "" && tradingEnvironment === "" && market === "") {
        return null;
    }

    return {
        brokerId,
        accountId,
        tradingEnvironment,
        market,
    };
}

function readStrategySymbolsFromParams(params: Record<string, unknown> | null): string[] {
    if (params === null) {
        return [];
    }
    const instruments = readStrategyInstrumentsFromParams(params);
    if (instruments.length > 0) {
        return bindingInstrumentsToSymbols(instruments);
    }
    if (Array.isArray(params.symbols)) {
        return normalizeStrategyInstrumentIds(
            params.symbols.filter((entry): entry is string => typeof entry === "string"),
        );
    }
    const symbol = normalizeInstrumentId(normalizeText(params.symbol));
    return symbol === "" ? [] : [symbol];
}

function readStrategyInstrumentsFromParams(params: Record<string, unknown> | null): StrategyBindingInstrumentDocument[] {
    if (params === null || !Array.isArray(params.instruments)) {
        return [];
    }
    return normalizeBindingInstruments(
        params.instruments
            .map((entry) => asRecord(entry))
            .filter((entry): entry is Record<string, unknown> => entry !== null)
            .map((entry) => ({
                market: normalizeText(entry.market),
                code: normalizeText(entry.code),
            })),
    );
}

function bindingInstrumentFromSymbol(symbol: string): StrategyBindingInstrumentDocument | null {
    const normalized = normalizeInstrumentId(symbol);
    const [market, code] = normalized.split(".", 2);
    if ((market ?? "") === "" || (code ?? "") === "") {
        return null;
    }
    return {
        market: market ?? "",
        code: code ?? "",
    };
}

export function defaultStrategyRuntimeRiskSettings(): StrategyRuntimeRiskSettings {
    return {
        mode: "off",
        closeOnly: false,
        maxOrderQuantity: null,
        maxOrderNotional: null,
        dailyMaxOrders: null,
        pauseOnReject: false,
    };
}

function normalizePositiveNumber(value: unknown): number | null {
    const numeric = typeof value === "number"
        ? value
        : typeof value === "string" && value.trim() !== ""
            ? Number(value)
            : NaN;
    return Number.isFinite(numeric) && numeric > 0 ? numeric : null;
}

function normalizePositiveInteger(value: unknown): number | null {
    const numeric = normalizePositiveNumber(value);
    if (numeric === null) {
        return null;
    }
    return Math.max(1, Math.floor(numeric));
}

export function normalizeStrategyRuntimeRiskSettings(value: unknown): StrategyRuntimeRiskSettings {
    const record = asRecord(value);
    if (record === null) {
        return defaultStrategyRuntimeRiskSettings();
    }
    const modeText = normalizeText(record.mode).toLowerCase();
    const mode: StrategyRuntimeRiskMode =
        modeText === "monitor" || modeText === "enforce" ? modeText : "off";
    if (mode === "off") {
        return defaultStrategyRuntimeRiskSettings();
    }
    return {
        mode,
        closeOnly: record.closeOnly === true,
        maxOrderQuantity: normalizePositiveNumber(record.maxOrderQuantity),
        maxOrderNotional: normalizePositiveNumber(record.maxOrderNotional),
        dailyMaxOrders: normalizePositiveInteger(record.dailyMaxOrders),
        pauseOnReject: record.pauseOnReject === true,
    };
}

export function formatStrategyRuntimeRiskMode(mode: StrategyRuntimeRiskMode | string | null | undefined): string {
    switch (mode) {
        case "monitor":
            return "观察";
        case "enforce":
            return "执行";
        default:
            return "关闭";
    }
}

export function formatStrategyRuntimeRiskSummary(settings: StrategyRuntimeRiskSettings | null | undefined): string {
    const normalized = normalizeStrategyRuntimeRiskSettings(settings);
    if (normalized.mode === "off") {
        return "动态风控关闭";
    }
    const parts = [formatStrategyRuntimeRiskMode(normalized.mode)];
    if (normalized.closeOnly) parts.push("仅平仓");
    if (normalized.maxOrderQuantity !== null) parts.push(`单笔数量 <= ${normalized.maxOrderQuantity}`);
    if (normalized.maxOrderNotional !== null) parts.push(`单笔金额 <= ${normalized.maxOrderNotional}`);
    if (normalized.dailyMaxOrders !== null) parts.push(`日订单 <= ${normalized.dailyMaxOrders}`);
    if (normalized.pauseOnReject) parts.push("拒单后暂停");
    return parts.join(" / ");
}

function symbolsToBindingInstruments(symbols: string[]): StrategyBindingInstrumentDocument[] {
    return normalizeBindingInstruments(
        symbols
            .map((symbol) => bindingInstrumentFromSymbol(symbol))
            .filter((entry): entry is StrategyBindingInstrumentDocument => entry !== null),
    );
}

function readStrategyBrokerAccount(
    params: Record<string, unknown> | null,
): StrategyBrokerAccountBinding | null {
    if (params === null) {
        return null;
    }
    const brokerAccount = asRecord(params.brokerAccount);
    if (brokerAccount === null) {
        return null;
    }
    return normalizeBrokerAccountBinding({
        brokerId: normalizeText(brokerAccount.brokerId),
        accountId: normalizeText(brokerAccount.accountId),
        tradingEnvironment: normalizeText(brokerAccount.tradingEnvironment),
        market: normalizeText(brokerAccount.market),
    });
}

export function readStrategyBinding(strategy: StrategyInstanceItem): StrategyInstanceBindingDocument {
    const params = asRecord(strategy.params);
    const bindingInstruments = Array.isArray(strategy.binding?.instruments)
        ? normalizeBindingInstruments(strategy.binding.instruments)
        : readStrategyInstrumentsFromParams(params);
    const bindingSymbols = bindingInstruments.length > 0
        ? bindingInstrumentsToSymbols(bindingInstruments)
        : Array.isArray(strategy.binding?.symbols)
            ? normalizeStrategyInstrumentIds(strategy.binding.symbols)
            : readStrategySymbolsFromParams(params);
    const executionModeSource =
        normalizeText(strategy.binding?.executionMode)
        || normalizeText(params?.executionMode);
    return {
        instruments: bindingInstruments.length > 0 ? bindingInstruments : symbolsToBindingInstruments(bindingSymbols),
        symbols: bindingSymbols,
        interval: normalizeText(strategy.binding?.interval) || normalizeText(params?.interval) || "5m",
        executionMode: executionModeSource === "notify_only" ? "notify_only" : "live",
        brokerAccount: normalizeBrokerAccountBinding(strategy.binding?.brokerAccount)
            ?? readStrategyBrokerAccount(params),
        runtimeRisk: normalizeStrategyRuntimeRiskSettings(strategy.binding?.runtimeRisk ?? params?.runtimeRisk),
    };
}

export function formatBrokerAccountSummary(
    brokerAccount: StrategyBrokerAccountBinding | null | undefined,
): string {
    const normalized = normalizeBrokerAccountBinding(brokerAccount);
    if (normalized == null) {
        return "未绑定账号";
    }
    return `${normalized.brokerId.toUpperCase()} / ${formatTradingEnvironment(normalized.tradingEnvironment)} / ${normalized.accountId} / ${formatUserMarketLabel(normalized.market)}`;
}

export function formatStrategySymbols(strategy: StrategyInstanceItem): string {
    const symbols = readStrategyBinding(strategy).symbols;
    return symbols.length > 0 ? symbols.join(", ") : "未绑定交易代码";
}

export function formatStrategyInterval(strategy: StrategyInstanceItem): string {
    return readStrategyBinding(strategy).interval || "5m";
}

export function formatRuntimeObservationSymbols(symbols: string[] | null | undefined): string {
    if (!Array.isArray(symbols)) {
        return "暂无";
    }
    const normalized = normalizeStrategyInstrumentIds(symbols);
    return normalized.length > 0 ? normalized.join(", ") : "暂无";
}

function formatBrokerAccountOption(option: BrokerAccountSelectionOption): string {
    return `${option.brokerId.toUpperCase()} / ${formatTradingEnvironment(option.tradingEnvironment)} / ${option.accountId} / ${formatUserMarketLabel(option.market)}`;
}

export function brokerAccountOptionSubtitle(option: BrokerAccountSelectionOption): string {
    return `${option.brokerId.toUpperCase()} / ${formatTradingEnvironment(option.tradingEnvironment)} / ${option.accountId} / ${formatUserMarketLabel(option.market)}`;
}

export function filterBrokerAccountOptions(
    options: BrokerAccountSelectionOption[],
    query: string,
): BrokerAccountSelectionOption[] {
    const normalizedQuery = normalizeText(query).toLowerCase();
    if (normalizedQuery === "") {
        return options;
    }
    return options.filter((option) =>
        [
            option.displayName,
            option.accountId,
            option.market,
            option.brokerId,
            option.tradingEnvironment,
            formatBrokerAccountOption(option),
        ]
            .filter((value): value is string => typeof value === "string")
            .some((value) => value.toLowerCase().includes(normalizedQuery)),
    );
}

export function resolveBrokerAccountSelectionKey(
    options: BrokerAccountSelectionOption[],
    brokerAccount: StrategyBrokerAccountBinding | null | undefined,
): string {
    const normalized = normalizeBrokerAccountBinding(brokerAccount);
    if (normalized == null) {
        return "";
    }

    const selectionKey = buildBrokerAccountSelectionKey({
        brokerId: normalized.brokerId,
        tradingEnvironment: normalized.tradingEnvironment,
        accountId: normalized.accountId,
        market: normalized.market,
    });

    return options.find((option) => option.selectionKey === selectionKey)?.selectionKey ?? "";
}

export function resolveBrokerAccountOption(
    options: BrokerAccountSelectionOption[],
    selectionKey: string,
): BrokerAccountSelectionOption | null {
    return options.find((option) => option.selectionKey === selectionKey) ?? null;
}

export function buildStrategyBindingPayload(input: {
    brokerAccountOptions: BrokerAccountSelectionOption[];
    instruments: StrategyBindingInstrumentDocument[];
    interval: string;
    executionMode: StrategyExecutionMode;
    brokerAccountKey: string;
    fallbackBrokerAccount?: StrategyBrokerAccountBinding | null;
    runtimeRisk?: StrategyRuntimeRiskSettings | null;
}): StrategyInstanceBindingDocument {
    const selectedAccount = resolveBrokerAccountOption(input.brokerAccountOptions, input.brokerAccountKey);
    const instruments = normalizeBindingInstruments(input.instruments);
    const symbols = bindingInstrumentsToSymbols(instruments);
    return {
        instruments,
        symbols,
        interval: normalizeText(input.interval) || "5m",
        executionMode: input.executionMode === "notify_only" ? "notify_only" : "live",
        brokerAccount: selectedAccount == null
            ? input.fallbackBrokerAccount ?? null
            : {
                brokerId: selectedAccount.brokerId,
                accountId: selectedAccount.accountId,
                tradingEnvironment: selectedAccount.tradingEnvironment,
                market: selectedAccount.market,
            },
        runtimeRisk: normalizeStrategyRuntimeRiskSettings(input.runtimeRisk),
    };
}
