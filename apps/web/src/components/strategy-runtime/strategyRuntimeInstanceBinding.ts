import type {
    StrategyBrokerAccountBinding,
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
} from "@jftrade/ui-contracts";

import {
    buildBrokerAccountSelectionKey,
    type BrokerAccountSelectionOption,
} from "../../composables/consoleDataBrokerAccountSelection";

export function normalizeText(value: unknown): string {
    return typeof value === "string" ? value.trim() : "";
}

function normalizeInstrumentId(value: string): string {
    const normalized = normalizeText(value).toUpperCase();
    if (normalized === "") {
        return "";
    }
    if (normalized.includes(":")) {
        const [market, symbol] = normalized.split(":", 2);
        if ((market ?? "") !== "" && (symbol ?? "") !== "") {
            return `${market}.${symbol}`;
        }
    }
    return normalized;
}

function normalizeSymbols(values: string[]): string[] {
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

export function splitSymbolsText(value: string): string[] {
    return value
        .split(/[\s,，;；]+/)
        .map((segment) => segment.trim())
        .filter((segment) => segment !== "");
}

export function parseSymbolsText(value: string): string[] {
    return normalizeSymbols(splitSymbolsText(value));
}

function isValidNormalizedInstrumentId(value: string): boolean {
    return /^[A-Z0-9_-]+\.[A-Z0-9._-]+$/.test(value);
}

export function parseValidatedSymbolsText(value: string): string[] {
    return normalizeSymbols(
        splitSymbolsText(value)
            .map((segment) => normalizeInstrumentId(segment))
            .filter((segment) => isValidNormalizedInstrumentId(segment)),
    );
}

export function invalidSymbolsFromText(value: string): string[] {
    const seen = new Set<string>();
    const invalidSymbols: string[] = [];
    for (const segment of splitSymbolsText(value)) {
        const normalized = normalizeInstrumentId(segment);
        if (normalized === "" || isValidNormalizedInstrumentId(normalized) || seen.has(normalized)) {
            continue;
        }
        seen.add(normalized);
        invalidSymbols.push(normalized);
    }
    return invalidSymbols;
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
    if (Array.isArray(params.symbols)) {
        return normalizeSymbols(
            params.symbols.filter((entry): entry is string => typeof entry === "string"),
        );
    }
    const symbol = normalizeInstrumentId(normalizeText(params.symbol));
    return symbol === "" ? [] : [symbol];
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
    const bindingSymbols = Array.isArray(strategy.binding?.symbols)
        ? normalizeSymbols(strategy.binding.symbols)
        : readStrategySymbolsFromParams(params);
    const executionModeSource =
        normalizeText(strategy.binding?.executionMode)
        || normalizeText(params?.executionMode);
    return {
        symbols: bindingSymbols,
        interval: normalizeText(strategy.binding?.interval) || normalizeText(params?.interval) || "5m",
        executionMode: executionModeSource === "notify_only" ? "notify_only" : "live",
        brokerAccount: normalizeBrokerAccountBinding(strategy.binding?.brokerAccount)
            ?? readStrategyBrokerAccount(params),
    };
}

export function formatBrokerAccountSummary(
    brokerAccount: StrategyBrokerAccountBinding | null | undefined,
): string {
    const normalized = normalizeBrokerAccountBinding(brokerAccount);
    if (normalized == null) {
        return "未绑定账号";
    }
    return `${normalized.brokerId.toUpperCase()} / ${formatTradingEnvironment(normalized.tradingEnvironment)} / ${normalized.accountId} / ${normalized.market}`;
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
    const normalized = normalizeSymbols(symbols);
    return normalized.length > 0 ? normalized.join(", ") : "暂无";
}

function formatBrokerAccountOption(option: BrokerAccountSelectionOption): string {
    return `${option.brokerId.toUpperCase()} / ${formatTradingEnvironment(option.tradingEnvironment)} / ${option.accountId} / ${option.market}`;
}

export function brokerAccountOptionSubtitle(option: BrokerAccountSelectionOption): string {
    return `${option.brokerId.toUpperCase()} / ${formatTradingEnvironment(option.tradingEnvironment)} / ${option.accountId} / ${option.market}`;
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
    symbolsText: string;
    interval: string;
    executionMode: StrategyExecutionMode;
    brokerAccountKey: string;
    fallbackBrokerAccount?: StrategyBrokerAccountBinding | null;
}): StrategyInstanceBindingDocument {
    const selectedAccount = resolveBrokerAccountOption(input.brokerAccountOptions, input.brokerAccountKey);
    return {
        symbols: parseValidatedSymbolsText(input.symbolsText),
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
    };
}