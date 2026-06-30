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
