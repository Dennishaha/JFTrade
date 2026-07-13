import {
  computed,
  getCurrentScope,
  onScopeDispose,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
} from "vue";

import type {
  InstrumentResolutionCandidate,
  InstrumentResolutionFailure,
  InstrumentResolutionResponse,
  InstrumentResolutionStatus,
} from "@/contracts";

import { fetchEnvelopeWithInit } from "./apiClient";
import {
  categoryMarketForUser,
  normalizeInstrumentMarket,
  parseInstrumentId,
} from "./instrumentPresentation";

export type {
  InstrumentResolutionCandidate,
  InstrumentResolutionFailure,
  InstrumentResolutionResponse,
  InstrumentResolutionStatus,
} from "@/contracts";

export interface ResolveMarketInstrumentInput {
  market: string;
  query: string;
  signal?: AbortSignal;
}

export interface UseInstrumentResolverOptions {
  market: MaybeRefOrGetter<string>;
  query: MaybeRefOrGetter<string>;
  onResolved: (candidate: InstrumentResolutionCandidate) => void;
  onError?: (error: Error) => void;
}

interface RawInstrumentResolutionCandidate {
  market?: string | null;
  resolvedMarket?: string | null;
  instrumentId?: string | null;
  code?: string | null;
  symbol?: string | null;
  name?: string | null;
  securityType?: string | null;
  lotSize?: number | null;
  source?: string | null;
}

interface RawInstrumentResolutionResponse {
  requestedMarket?: string | null;
  query?: string | null;
  resolutionStatus?: string | null;
  totalReturned?: number | null;
  entries?: RawInstrumentResolutionCandidate[] | null;
  failures?: Array<{
    market?: string | null;
    code?: string | null;
    message?: string | null;
  }> | null;
}

const RESOLUTION_STATUSES = new Set<InstrumentResolutionStatus>([
  "resolved",
  "ambiguous",
  "not_found",
  "incomplete",
]);

const MARKET_SUBSET_CHILDREN: Readonly<Record<string, readonly string[]>> = {
  CN: ["SH", "SZ"],
};

const CANONICAL_LEAF_MARKETS: Readonly<Record<string, string>> = {
  US: "US",
  HK: "HK",
  SH: "SH",
  SZ: "SZ",
  CNSH: "SH",
  CNSZ: "SZ",
  SG: "SG",
  JP: "JP",
  AU: "AU",
  MY: "MY",
  CA: "CA",
};

function normalizedText(value: string | null | undefined): string {
  return (value ?? "").trim();
}

function isMarketSubset(market: string): boolean {
  return MARKET_SUBSET_CHILDREN[normalizeInstrumentMarket(market)] != null;
}

function canonicalLeafMarket(market: string): string | null {
  return CANONICAL_LEAF_MARKETS[normalizeInstrumentMarket(market)] ?? null;
}

function requestedMarketMatchesLeaf(
  requestedMarket: string,
  leafMarket: string,
): boolean {
  const normalizedRequestedMarket = normalizeInstrumentMarket(requestedMarket);
  if (normalizedRequestedMarket === "") {
    return true;
  }
  const subsetChildren = MARKET_SUBSET_CHILDREN[normalizedRequestedMarket];
  if (subsetChildren != null) {
    return subsetChildren.includes(leafMarket);
  }
  return canonicalLeafMarket(normalizedRequestedMarket) === leafMarket;
}

export function resolveDirectInstrumentCandidate(
  market: string,
  query: string,
): InstrumentResolutionCandidate | null {
  const requestedMarket = normalizeInstrumentMarket(market);
  const normalizedQuery = query.trim().toUpperCase().replace(":", ".");
  const qualified = parseInstrumentId(normalizedQuery);
  const querySeparator = normalizedQuery.indexOf(".");
  const queryPrefix =
    querySeparator < 0 ? "" : normalizedQuery.slice(0, querySeparator);
  if (
    qualified == null &&
    (canonicalLeafMarket(queryPrefix) != null ||
      isMarketSubset(queryPrefix))
  ) {
    return null;
  }
  const qualifiedLeafMarket =
    qualified == null ? null : canonicalLeafMarket(qualified.market);
  const qualifiedUsesRecognizedMarket =
    qualifiedLeafMarket != null ||
    (qualified != null && isMarketSubset(qualified.market));

  if (
    qualifiedUsesRecognizedMarket &&
    (qualifiedLeafMarket == null ||
      !requestedMarketMatchesLeaf(requestedMarket, qualifiedLeafMarket))
  ) {
    return null;
  }

  const actualMarket =
    qualifiedLeafMarket ?? canonicalLeafMarket(requestedMarket) ?? "";
  const code = normalizedText(
    qualifiedLeafMarket == null ? normalizedQuery : qualified?.code,
  ).toUpperCase();
  if (
    actualMarket === "" ||
    code === "" ||
    (!qualifiedUsesRecognizedMarket && isMarketSubset(requestedMarket))
  ) {
    return null;
  }

  return {
    market: actualMarket,
    resolvedMarket: categoryMarketForUser(actualMarket),
    instrumentId: `${actualMarket}.${code}`,
    code,
    symbol: code,
    name: null,
    securityType: null,
    lotSize: null,
    source: "direct-input",
  };
}

function normalizeCandidate(
  entry: RawInstrumentResolutionCandidate,
): InstrumentResolutionCandidate | null {
  const rawInstrumentId = normalizedText(entry.instrumentId).toUpperCase();
  const parsedInstrumentId = parseInstrumentId(rawInstrumentId);
  const rawCode = normalizedText(entry.code ?? entry.symbol).toUpperCase();
  const parsedCode = parseInstrumentId(rawCode);
  const market = normalizeInstrumentMarket(
    parsedInstrumentId?.market ?? parsedCode?.market ?? entry.market,
  );
  const code = normalizedText(
    parsedInstrumentId?.code ?? parsedCode?.code ?? rawCode,
  ).toUpperCase();
  const instrumentId =
    market !== "" && code !== "" ? `${market}.${code}` : "";
  if (market === "" || code === "" || instrumentId === "") {
    return null;
  }

  return {
    market,
    resolvedMarket:
      normalizeInstrumentMarket(entry.resolvedMarket) ||
      categoryMarketForUser(market),
    instrumentId,
    code,
    symbol: code,
    name: normalizedText(entry.name) || null,
    securityType: normalizedText(entry.securityType) || null,
    lotSize:
      typeof entry.lotSize === "number" && Number.isFinite(entry.lotSize)
        ? entry.lotSize
        : null,
    source: normalizedText(entry.source),
  };
}

function normalizeFailures(
  failures: RawInstrumentResolutionResponse["failures"],
): InstrumentResolutionFailure[] {
  if (!Array.isArray(failures)) {
    return [];
  }
  return failures.map((failure) => ({
    market: normalizeInstrumentMarket(failure.market),
    code: normalizedText(failure.code).toUpperCase(),
    message: normalizedText(failure.message) || "查询失败",
  }));
}

function inferResolutionStatus(
  rawStatus: string | null | undefined,
  entries: InstrumentResolutionCandidate[],
  failures: InstrumentResolutionFailure[],
): InstrumentResolutionStatus {
  const normalizedStatus = normalizedText(rawStatus).toLowerCase();
  if (
    RESOLUTION_STATUSES.has(normalizedStatus as InstrumentResolutionStatus)
  ) {
    return normalizedStatus as InstrumentResolutionStatus;
  }
  if (entries.length > 1) {
    return "ambiguous";
  }
  if (failures.length > 0) {
    return "incomplete";
  }
  return entries.length === 1 ? "resolved" : "not_found";
}

export async function resolveMarketInstrumentCandidates(
  input: ResolveMarketInstrumentInput,
): Promise<InstrumentResolutionResponse> {
  const requestedMarket = normalizeInstrumentMarket(input.market);
  const query = input.query.trim();
  const params = new URLSearchParams({ market: requestedMarket, query });
  const init: RequestInit = { method: "GET" };
  if (input.signal != null) {
    init.signal = input.signal;
  }
  const raw = await fetchEnvelopeWithInit<RawInstrumentResolutionResponse>(
    `/api/v1/market-data/instruments?${params.toString()}`,
    init,
  );
  const byInstrumentId = new Map<string, InstrumentResolutionCandidate>();
  for (const rawEntry of Array.isArray(raw.entries) ? raw.entries : []) {
    const entry = normalizeCandidate(rawEntry);
    if (entry != null && !byInstrumentId.has(entry.instrumentId)) {
      byInstrumentId.set(entry.instrumentId, entry);
    }
  }
  const entries = [...byInstrumentId.values()];
  const failures = normalizeFailures(raw.failures);

  return {
    requestedMarket:
      normalizeInstrumentMarket(raw.requestedMarket) || requestedMarket,
    query: normalizedText(raw.query) || query,
    resolutionStatus: inferResolutionStatus(
      raw.resolutionStatus,
      entries,
      failures,
    ),
    totalReturned: entries.length,
    entries,
    failures,
  };
}

export function useInstrumentResolver(options: UseInstrumentResolverOptions) {
  const loading = ref(false);
  const panelOpen = ref(false);
  const candidates = ref<InstrumentResolutionCandidate[]>([]);
  const failures = ref<InstrumentResolutionFailure[]>([]);
  const resolutionStatus = ref<InstrumentResolutionStatus | null>(null);
  const resolutionError = ref("");
  const activeCandidateIndex = ref(-1);
  let activeController: AbortController | null = null;
  let requestVersion = 0;

  const statusMessage = computed(() => {
    if (resolutionError.value !== "") {
      return resolutionError.value;
    }
    switch (resolutionStatus.value) {
      case "ambiguous":
        return "找到多个匹配标的，请选择一个。";
      case "incomplete":
        return "部分市场查询失败，无法安全确认唯一标的。请确认候选、重试或输入完整市场前缀。";
      case "not_found":
        return "未找到匹配标的，请检查代码或输入完整市场前缀。";
      default:
        return "";
    }
  });

  function cancelActiveRequest(): void {
    requestVersion += 1;
    activeController?.abort();
    activeController = null;
    loading.value = false;
  }

  function clearResolutionState(): void {
    panelOpen.value = false;
    candidates.value = [];
    failures.value = [];
    resolutionStatus.value = null;
    resolutionError.value = "";
    activeCandidateIndex.value = -1;
  }

  function reset(): void {
    cancelActiveRequest();
    clearResolutionState();
  }

  function closePanel(): void {
    panelOpen.value = false;
    activeCandidateIndex.value = -1;
  }

  function selectCandidate(candidate: InstrumentResolutionCandidate): void {
    closePanel();
    options.onResolved(candidate);
  }

  function selectActiveCandidate(): boolean {
    const candidate = candidates.value[activeCandidateIndex.value];
    if (candidate == null) {
      return false;
    }
    selectCandidate(candidate);
    return true;
  }

  function reportError(error: Error): void {
    resolutionError.value = error.message;
    resolutionStatus.value = null;
    candidates.value = [];
    failures.value = [];
    activeCandidateIndex.value = -1;
    panelOpen.value = true;
    options.onError?.(error);
  }

  function isAbortError(cause: unknown): boolean {
    return (
      (typeof DOMException !== "undefined" &&
        cause instanceof DOMException &&
        cause.name === "AbortError") ||
      (cause instanceof Error && cause.name === "AbortError")
    );
  }

  async function resolve(): Promise<InstrumentResolutionCandidate | null> {
    const market = toValue(options.market).trim();
    const query = toValue(options.query).trim();
    if (query === "") {
      reportError(new Error("请输入标的代码。"));
      return null;
    }

    activeController?.abort();
    const currentRequest = ++requestVersion;
    clearResolutionState();

    const directCandidate = resolveDirectInstrumentCandidate(market, query);
    if (directCandidate != null) {
      candidates.value = [directCandidate];
      resolutionStatus.value = "resolved";
      selectCandidate(directCandidate);
      return directCandidate;
    }

    const controller = new AbortController();
    activeController = controller;
    loading.value = true;

    try {
      const response = await resolveMarketInstrumentCandidates({
        market,
        query,
        signal: controller.signal,
      });
      if (currentRequest !== requestVersion) {
        return null;
      }

      candidates.value = response.entries;
      failures.value = response.failures;
      resolutionStatus.value = response.resolutionStatus;

      if (
        response.resolutionStatus === "resolved" &&
        response.entries.length === 1
      ) {
        const candidate = response.entries[0] ?? null;
        if (candidate != null) {
          selectCandidate(candidate);
          return candidate;
        }
      }
      if (response.resolutionStatus === "resolved") {
        reportError(new Error("标的解析响应缺少唯一候选。"));
        return null;
      }

      panelOpen.value = true;
      activeCandidateIndex.value = response.entries.length > 0 ? 0 : -1;
      return null;
    } catch (cause) {
      if (currentRequest !== requestVersion || isAbortError(cause)) {
        return null;
      }
      reportError(
        cause instanceof Error
          ? cause
          : new Error("标的查询失败，请稍后重试。"),
      );
      return null;
    } finally {
      if (currentRequest === requestVersion) {
        loading.value = false;
        activeController = null;
      }
    }
  }

  function moveActiveCandidate(offset: -1 | 1): void {
    if (!panelOpen.value || candidates.value.length === 0) {
      return;
    }
    activeCandidateIndex.value =
      (activeCandidateIndex.value + offset + candidates.value.length) %
      candidates.value.length;
  }

  function handleKeydown(event: KeyboardEvent): boolean {
    if (event.isComposing) {
      return false;
    }
    if (event.key === "Escape" && panelOpen.value) {
      event.preventDefault();
      closePanel();
      return true;
    }
    if (event.key === "ArrowDown" && panelOpen.value) {
      event.preventDefault();
      moveActiveCandidate(1);
      return true;
    }
    if (event.key === "ArrowUp" && panelOpen.value) {
      event.preventDefault();
      moveActiveCandidate(-1);
      return true;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      if (panelOpen.value && selectActiveCandidate()) {
        return true;
      }
      void resolve();
      return true;
    }
    return false;
  }

  watch(
    [
      () => normalizeInstrumentMarket(toValue(options.market)),
      () => toValue(options.query),
    ],
    reset,
  );
  if (getCurrentScope() != null) {
    onScopeDispose(cancelActiveRequest);
  }

  return {
    loading,
    panelOpen,
    candidates,
    failures,
    resolutionStatus,
    resolutionError,
    statusMessage,
    activeCandidateIndex,
    resolve,
    reset,
    closePanel,
    selectCandidate,
    selectActiveCandidate,
    moveActiveCandidate,
    handleKeydown,
  };
}
