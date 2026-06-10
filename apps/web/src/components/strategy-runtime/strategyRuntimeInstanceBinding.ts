import type {
    StrategyBindingInstrumentDocument,
    StrategyBrokerAccountBinding,
    StrategyExecutionMode,
    StrategyInstanceBindingDocument,
    StrategyInstanceItem,
} from "@/contracts";

import {
    buildBrokerAccountSelectionKey,
    type BrokerAccountSelectionOption,
} from "../../composables/consoleDataBrokerAccountSelection";
import { normalizeInstrumentId, resolveInstrumentRef } from "../../composables/instrumentRef";

export function normalizeText(value: unknown): string {
    return typeof value === "string" ? value.trim() : "";
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

function normalizeDraftSymbol(value: string, fallbackMarket?: string): string {
    const resolved = resolveInstrumentRef(
        { instrumentId: normalizeText(value) },
        normalizeText(fallbackMarket),
    );
    return resolved?.instrumentId ?? normalizeInstrumentId(value);
}

function instrumentIdFromBindingInstrument(value: StrategyBindingInstrumentDocument): string {
    const resolved = resolveInstrumentRef({
        market: normalizeText(value.market),
        code: normalizeText(value.code),
    }, normalizeText(value.market));
    return resolved?.instrumentId ?? "";
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
    return normalizeSymbols(values.map((value) => `${value.market}.${value.code}`));
}

export function splitSymbolsText(value: string): string[] {
    return value
    .split(/[\n\r\t,，;；]+/)
        .map((segment) => segment.trim())
        .filter((segment) => segment !== "");
}

export function parseSymbolsText(value: string): string[] {
    return parseSymbolsTextWithFallbackMarket(value);
}

export function parseSymbolsTextWithFallbackMarket(value: string, fallbackMarket?: string): string[] {
    return normalizeSymbols(
        splitSymbolsText(value).map((segment) => normalizeDraftSymbol(segment, fallbackMarket)),
    );
}

function isValidNormalizedInstrumentId(value: string): boolean {
    return /^[A-Z0-9_-]+\.[A-Z0-9._-]+$/.test(value);
}

export function parseValidatedSymbolsText(value: string): string[] {
    return parseValidatedSymbolsTextWithFallbackMarket(value);
}

export function parseValidatedSymbolsTextWithFallbackMarket(value: string, fallbackMarket?: string): string[] {
    return normalizeSymbols(
        splitSymbolsText(value)
            .map((segment) => normalizeDraftSymbol(segment, fallbackMarket))
            .filter((segment) => isValidNormalizedInstrumentId(segment)),
    );
}

export function parseBindingInstrumentsTextWithFallbackMarket(
    value: string,
    fallbackMarket?: string,
): StrategyBindingInstrumentDocument[] {
    return symbolsToBindingInstruments(parseValidatedSymbolsTextWithFallbackMarket(value, fallbackMarket));
}

export function invalidSymbolsFromText(value: string): string[] {
    return invalidSymbolsFromTextWithFallbackMarket(value);
}

export function invalidSymbolsFromTextWithFallbackMarket(value: string, fallbackMarket?: string): string[] {
    const seen = new Set<string>();
    const invalidSymbols: string[] = [];
    for (const segment of splitSymbolsText(value)) {
        const normalized = normalizeDraftSymbol(segment, fallbackMarket);
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
    const instruments = readStrategyInstrumentsFromParams(params);
    if (instruments.length > 0) {
        return bindingInstrumentsToSymbols(instruments);
    }
    if (Array.isArray(params.symbols)) {
        return normalizeSymbols(
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
    const resolved = resolveInstrumentRef({ instrumentId: symbol });
    if (resolved == null) {
        return null;
    }
    return {
        market: resolved.market,
        code: resolved.code,
    };
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
            ? normalizeSymbols(strategy.binding.symbols)
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
    symbolsText?: string;
    instruments?: StrategyBindingInstrumentDocument[];
    interval: string;
    executionMode: StrategyExecutionMode;
    brokerAccountKey: string;
    fallbackBrokerAccount?: StrategyBrokerAccountBinding | null;
}): StrategyInstanceBindingDocument {
    const selectedAccount = resolveBrokerAccountOption(input.brokerAccountOptions, input.brokerAccountKey);
    const instruments = input.instruments == null
        ? symbolsToBindingInstruments(parseValidatedSymbolsText(input.symbolsText ?? ""))
        : normalizeBindingInstruments(input.instruments);
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
    };
}