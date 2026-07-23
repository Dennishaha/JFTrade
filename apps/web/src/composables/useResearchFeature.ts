import { computed, isRef, ref, watch, type Ref } from "vue";

import {
  fetchProductFeature,
  type ProductFeatureProvider,
  type ProductFeatureResult,
} from "./productFeatures";
import { withBrokerProvider } from "./brokerProviderSelection";

export type ResearchFeaturePathSource = Ref<string> | (() => string);

export interface ResearchFeatureOptions {
  /** Logical CN is a UI scope. OpenD must receive concrete SH and SZ markets. */
  expandCN?: boolean;
  brokerId?: string | Ref<string> | (() => string);
  mergeComparator?: (
    left: Record<string, unknown>,
    right: Record<string, unknown>,
  ) => number;
}

export interface ResearchPartialError {
  scope: string;
  code: string;
  message: string;
}

export interface UseResearchFeatureResult {
  entries: Ref<Record<string, unknown>[]>;
  asOf: Ref<string>;
  provider: Ref<ProductFeatureProvider | null>;
  metadata: Ref<Record<string, unknown>>;
  total: Ref<number>;
  nextCursor: Ref<string>;
  hasMore: Ref<boolean>;
  warnings: Ref<string[]>;
  partialErrors: Ref<ResearchPartialError[]>;
  loading: Ref<boolean>;
  loadingMore: Ref<boolean>;
  error: Ref<string>;
  refresh: () => Promise<void>;
  loadMore: () => Promise<void>;
}

interface BranchState {
  market: string;
  path: string;
  result: ProductFeatureResult;
}

function pathValue(source: ResearchFeaturePathSource): string {
  return isRef(source) ? source.value : source();
}

function brokerValue(source: ResearchFeatureOptions["brokerId"]): string {
  if (typeof source === "function") return source().trim();
  if (isRef(source)) return source.value.trim();
  return String(source ?? "").trim();
}

function updateQuery(path: string, key: string, value: string): string {
  const [resource, hash = ""] = path.split("#", 2);
  const queryIndex = resource!.indexOf("?");
  const pathname = queryIndex >= 0 ? resource!.slice(0, queryIndex) : resource!;
  const params = new URLSearchParams(
    queryIndex >= 0 ? resource!.slice(queryIndex + 1) : "",
  );
  params.set(key, value);
  const query = params.toString();
  return `${pathname}${query ? `?${query}` : ""}${hash ? `#${hash}` : ""}`;
}

function marketFromPath(path: string): string {
  const queryIndex = path.indexOf("?");
  if (queryIndex < 0) return "";
  return (
    new URLSearchParams(path.slice(queryIndex + 1).split("#", 1)[0]).get(
      "market",
    ) ?? ""
  )
    .trim()
    .toUpperCase();
}

export function researchFeaturePaths(
  path: string,
  options: ResearchFeatureOptions = {},
): Array<{ market: string; path: string }> {
  const market = marketFromPath(path);
  if (options.expandCN !== false && market === "CN") {
    return ["SH", "SZ"].map((branchMarket) => ({
      market: branchMarket,
      path: updateQuery(path, "market", branchMarket),
    }));
  }
  return [{ market, path }];
}

function entryKey(entry: Record<string, unknown>, index: number): string {
  for (const key of ["instrumentId", "plateId", "institutionId", "eventId"]) {
    const value = entry[key];
    if (typeof value === "string" && value.trim()) {
      return `${key}:${value.trim().toUpperCase()}`;
    }
    if (typeof value === "number" && Number.isFinite(value)) {
      return `${key}:${value}`;
    }
  }
  const market = String(entry.market ?? "").trim().toUpperCase();
  const symbol = String(entry.symbol ?? entry.code ?? "").trim().toUpperCase();
  if (market && symbol) return `instrument:${market}.${symbol}`;
  // Do not collapse unrelated calendar rows which legitimately lack identifiers.
  return `row:${index}:${JSON.stringify(entry)}`;
}

function dedupePartialErrors(
  errors: ResearchPartialError[],
): ResearchPartialError[] {
  const keys = new Set<string>();
  return errors.filter((item) => {
    const key = `${item.scope}\u0000${item.code}\u0000${item.message}`;
    if (keys.has(key)) return false;
    keys.add(key);
    return true;
  });
}

function mergedTotal(
  branches: BranchState[],
  entries: Record<string, unknown>[],
  rawEntryCount: number,
): number {
  if (branches.length === 1) {
    return branches[0]!.result.total ?? entries.length;
  }
  // Backend totals are only additive when the concrete market branches do not
  // overlap. Once an exact identifier is duplicated, the only sound total is
  // the number of deduplicated entries we actually hold.
  if (rawEntryCount !== entries.length) return entries.length;
  if (branches.every((branch) => branch.result.total != null)) {
    return branches.reduce((sum, branch) => sum + branch.result.total!, 0);
  }
  return entries.length;
}

function mergeBranchResults(branches: BranchState[]): ProductFeatureResult | null {
  if (branches.length === 0) return null;
  const first = branches[0]!.result;
  const entries: Record<string, unknown>[] = [];
  const keys = new Set<string>();
  let rowIndex = 0;
  let rawEntryCount = 0;
  for (const branch of branches) {
    for (const entry of branch.result.entries ?? []) {
      rawEntryCount++;
      const key = entryKey(entry, rowIndex++);
      if (keys.has(key)) continue;
      keys.add(key);
      entries.push(entry);
    }
  }
  const warnings = branches.flatMap((branch) => branch.result.warnings ?? []);
  const partialErrors = dedupePartialErrors(
    branches.flatMap((branch) => branch.result.partialErrors ?? []),
  );
  const asOf = branches
    .map((branch) => branch.result.asOf)
    .filter(Boolean)
    .sort()
    .at(-1) ?? first.asOf;
  return {
    ...first,
    asOf,
    entries,
    total: mergedTotal(branches, entries, rawEntryCount),
    ...(branches.length > 1 && branches.some((branch) => branch.result.nextCursor)
      ? { nextCursor: "multi-market" }
      : {}),
    hasMore: branches.some(
      (branch) => branch.result.hasMore ?? Boolean(branch.result.nextCursor),
    ),
    warnings: [...new Set(warnings)],
    partialErrors,
    metadata: {
      ...(first.metadata ?? {}),
      logicalMarket:
        branches.length > 1 ? "CN" : branches[0]?.market || undefined,
      sourceMarkets: branches.map((branch) => branch.market).filter(Boolean),
      ...(branches.length > 1
        ? {
            byMarket: Object.fromEntries(
              branches.map((branch) => [
                branch.market,
                branch.result.metadata ?? {},
              ]),
            ),
          }
        : {}),
    },
  };
}

function appendBranchPage(
  branch: BranchState,
  page: ProductFeatureResult,
): BranchState {
  const warnings = [
    ...(branch.result.warnings ?? []),
    ...(page.warnings ?? []),
  ];
  const partialErrors = dedupePartialErrors([
    ...(branch.result.partialErrors ?? []),
    ...(page.partialErrors ?? []),
  ]);
  return {
    ...branch,
    result: {
      ...page,
      entries: [...branch.result.entries, ...page.entries],
      ...(page.total != null || branch.result.total != null
        ? { total: page.total ?? branch.result.total! }
        : {}),
      ...(warnings.length > 0 ? { warnings: [...new Set(warnings)] } : {}),
      ...(partialErrors.length > 0 ? { partialErrors } : {}),
      ...(branch.result.metadata != null || page.metadata != null
        ? {
            metadata: {
              ...(branch.result.metadata ?? {}),
              ...(page.metadata ?? {}),
            },
          }
        : {}),
    },
  };
}

function numericField(
  entry: Record<string, unknown>,
  fields: readonly string[],
): number | null {
  for (const field of fields) {
    const value = Number(entry[field]);
    if (Number.isFinite(value)) return value;
  }
  return null;
}

function sortMergedEntries(
  entries: Record<string, unknown>[],
  path: string,
  comparator?: ResearchFeatureOptions["mergeComparator"],
): Record<string, unknown>[] {
  if (comparator != null) return [...entries].sort(comparator);
  const queryIndex = path.indexOf("?");
  const params = new URLSearchParams(
    queryIndex >= 0 ? path.slice(queryIndex + 1).split("#", 1)[0] : "",
  );
  const operation = params.get("operation") ?? "";
  const fields =
    operation === "top_movers"
      ? ["changeRate", "changeRatio"]
      : operation === "hot"
        ? ["averageHeat"]
        : operation === "high_dividend_state"
          ? ["dividendYieldTTM"]
          : [];
  if (fields.length === 0) return entries;
  const ascending = params.get("direction") === "down";
  return entries
    .map((entry, index) => ({ entry, index }))
    .sort((left, right) => {
      const leftValue = numericField(left.entry, fields);
      const rightValue = numericField(right.entry, fields);
      if (leftValue == null && rightValue == null) return left.index - right.index;
      if (leftValue == null) return 1;
      if (rightValue == null) return -1;
      const compared = ascending
        ? leftValue - rightValue
        : rightValue - leftValue;
      return compared || left.index - right.index;
    })
    .map((item) => item.entry);
}

function mergedResult(
  branches: BranchState[],
  path: string,
  options: ResearchFeatureOptions,
): ProductFeatureResult | null {
  const merged = mergeBranchResults(branches);
  if (merged != null && branches.length > 1) {
    merged.entries = sortMergedEntries(
      merged.entries,
      path,
      options.mergeComparator,
    );
  }
  return merged;
}

/**
 * Research feature loader with refresh, cursor pagination and last-request-wins
 * semantics. Logical CN paths are queried as concrete SH/SZ branches and merged
 * by canonical instrumentId.
 */
export function useResearchFeature(
  pathSource: ResearchFeaturePathSource,
  options: ResearchFeatureOptions = {},
): UseResearchFeatureResult {
  const result = ref<ProductFeatureResult | null>(null);
  const loading = ref(false);
  const loadingMore = ref(false);
  const error = ref("");
  let branches: BranchState[] = [];
  let requestToken = 0;

  const entries = computed(() => result.value?.entries ?? []);
  const asOf = computed(() => result.value?.asOf ?? "");
  const provider = computed(() => result.value?.provider ?? null);
  const metadata = computed(() => result.value?.metadata ?? {});
  const total = computed(() => result.value?.total ?? entries.value.length);
  const nextCursor = computed(() => result.value?.nextCursor ?? "");
  const hasMore = computed(
    () => result.value?.hasMore ?? Boolean(result.value?.nextCursor),
  );
  const warnings = computed(() => result.value?.warnings ?? []);
  const partialErrors = computed(() => result.value?.partialErrors ?? []);

  async function load(refresh = false): Promise<void> {
    const path = withBrokerProvider(
      pathValue(pathSource),
      brokerValue(options.brokerId),
    );
    const token = ++requestToken;
    if (!path) {
      branches = [];
      result.value = null;
      error.value = "";
      loading.value = false;
      return;
    }
    loading.value = true;
    error.value = "";
    const targets = researchFeaturePaths(path, options);
    const settled = await Promise.allSettled(
      targets.map(async (target) => {
        const requestPath = refresh
          ? updateQuery(target.path, "refresh", "true")
          : target.path;
        return {
          ...target,
          result: await fetchProductFeature(requestPath),
        } satisfies BranchState;
      }),
    );
    if (token !== requestToken) return;
    const nextBranches: BranchState[] = [];
    const branchErrors: ResearchPartialError[] = [];
    settled.forEach((item, index) => {
      const target = targets[index]!;
      if (item.status === "fulfilled") {
        nextBranches.push(item.value);
        return;
      }
      branchErrors.push({
        scope: target.market || "research",
        code: "QUERY_FAILED",
        message:
          item.reason instanceof Error
            ? item.reason.message
            : String(item.reason),
      });
    });
    if (nextBranches.length === 0) {
      branches = [];
      result.value = null;
      error.value = branchErrors[0]?.message ?? "研究数据加载失败";
    } else {
      branches = nextBranches;
      result.value = mergedResult(nextBranches, path, options);
      if (branchErrors.length > 0 && result.value != null) {
        result.value.partialErrors = [
          ...(result.value.partialErrors ?? []),
          ...branchErrors,
        ];
      }
    }
    loading.value = false;
  }

  watch(
    () => [pathValue(pathSource), brokerValue(options.brokerId)] as const,
    (current, previous) => {
      if (current[0] !== previous?.[0] || current[1] !== previous?.[1]) {
        requestToken++;
        branches = [];
        result.value = null;
        error.value = "";
        loadingMore.value = false;
      }
      void load();
    },
    { immediate: true },
  );

  async function refresh(): Promise<void> {
    await load(true);
  }

  async function loadMore(): Promise<void> {
    if (loading.value || loadingMore.value || !hasMore.value) return;
    const token = ++requestToken;
    loadingMore.value = true;
    error.value = "";
    const currentBranches = [...branches];
    try {
      const settled = await Promise.allSettled(
        currentBranches.map(async (branch) => {
          const cursor = branch.result.nextCursor ?? "";
          if (!cursor) return branch;
          const page = await fetchProductFeature(
            updateQuery(branch.path, "cursor", cursor),
          );
          return appendBranchPage(branch, page);
        }),
      );
      if (token !== requestToken) return;
      const branchErrors: ResearchPartialError[] = [];
      branches = settled.map((item, index) => {
        if (item.status === "fulfilled") return item.value;
        const branch = currentBranches[index]!;
        branchErrors.push({
          scope: branch.market || "research",
          code: "QUERY_FAILED",
          message:
            item.reason instanceof Error
              ? item.reason.message
              : String(item.reason),
        });
        // A failed concrete market keeps its previously loaded page and cursor;
        // the successful sibling can still advance independently.
        return branch;
      });
      const merged = mergedResult(
        branches,
        withBrokerProvider(
          pathValue(pathSource),
          brokerValue(options.brokerId),
        ),
        options,
      );
      if (merged != null) {
        merged.partialErrors = dedupePartialErrors([
          ...(result.value?.partialErrors ?? []),
          ...(merged.partialErrors ?? []),
          ...branchErrors,
        ]);
      }
      result.value = merged;
    } finally {
      if (token === requestToken) loadingMore.value = false;
    }
  }

  return {
    entries: entries as Ref<Record<string, unknown>[]>,
    asOf: asOf as Ref<string>,
    provider: provider as Ref<ProductFeatureProvider | null>,
    metadata: metadata as Ref<Record<string, unknown>>,
    total: total as Ref<number>,
    nextCursor: nextCursor as Ref<string>,
    hasMore: hasMore as Ref<boolean>,
    warnings: warnings as Ref<string[]>,
    partialErrors: partialErrors as Ref<ResearchPartialError[]>,
    loading,
    loadingMore,
    error,
    refresh,
    loadMore,
  };
}
