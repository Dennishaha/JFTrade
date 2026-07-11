import { QueryClient } from "@tanstack/vue-query";

export const queryKeys = {
  settings: (section: string) => ["settings", section] as const,
  strategyDefinitions: () => ["strategyDefinitions"] as const,
  strategyDefinition: (definitionId: string, params: Record<string, string | boolean | undefined> = {}) =>
    ["strategyDefinitions", definitionId, params] as const,
  backtestRuns: (filters: Record<string, unknown> = {}) => ["backtestRuns", filters] as const,
  backtestRun: (runId: string) => ["backtestRuns", "detail", runId] as const,
  adk: (section: string) => ["adk", section] as const,
  marketData: (market: string, symbol: string, period: string) =>
    ["marketData", market, symbol, period] as const,
  watchlistGroups: () => ["watchlist", "groups"] as const,
  watchlistItemsRoot: () => ["watchlist", "items"] as const,
  watchlistItems: (filters: Record<string, unknown> = {}) =>
    ["watchlist", "items", filters] as const,
  watchlistMembershipsRoot: () => ["watchlist", "membership"] as const,
  watchlistMembership: (market: string, symbol: string) =>
    ["watchlist", "membership", market, symbol] as const,
  watchlistQuotes: (instrumentIds: string[]) =>
    ["watchlist", "quotes", [...instrumentIds].sort()] as const,
  watchlistSources: () => ["watchlist", "sources"] as const,
  watchlistSourceGroups: (sourceId: string) =>
    ["watchlist", "source-groups", sourceId] as const,
  watchlistBindings: () => ["watchlist", "bindings"] as const,
  watchlistImportRuns: () => ["watchlist", "import-runs"] as const,
};

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      gcTime: 5 * 60 * 1000,
      refetchOnMount: false,
      refetchOnWindowFocus: false,
      retry: 1,
      staleTime: 30 * 1000,
    },
  },
});
