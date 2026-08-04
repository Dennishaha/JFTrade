import {
  usePythonMarketDataRuntimeWarmup,
  type PythonMarketDataRuntimeReadiness,
} from "@/composables/market-data/usePythonMarketDataRuntimeWarmup";

/** @deprecated Use the provider-neutral Python market-data warmup composable. */
export const useYFinanceRuntimeWarmup = usePythonMarketDataRuntimeWarmup;
export type YFinanceRuntimeReadiness = PythonMarketDataRuntimeReadiness;
