import type { SplitpanesResizedPayload } from "splitpanes";
import { computed, ref, type Ref } from "vue";

import type { StrategyInstanceItem } from "@/types";

export type StrategyRuntimeMobileSection = "instances" | "workbench";
export type StrategyRuntimeWorkbenchLayout = "desktop" | "compact" | "mobile";

const COMPACT_MEDIA_QUERY = "(max-width: 1180px)";
const MOBILE_MEDIA_QUERY = "(max-width: 768px)";

export function useStrategyRuntimeLayout(
  selectedStrategy: Readonly<Ref<StrategyInstanceItem | null>>,
) {
  const runtimePaneSizes = ref<[number, number]>([30, 70]);
  const isCompactStrategyRuntime = ref(false);
  const isMobileStrategyRuntime = ref(false);
  const strategyRuntimeMobileSection = ref<StrategyRuntimeMobileSection>("instances");
  let compactMediaQuery: MediaQueryList | null = null;
  let mobileMediaQuery: MediaQueryList | null = null;

  const strategyRuntimeWorkbenchLayout = computed<StrategyRuntimeWorkbenchLayout>(() => {
    if (isMobileStrategyRuntime.value) return "mobile";
    return isCompactStrategyRuntime.value ? "compact" : "desktop";
  });

  function syncCompactStrategyRuntime(event: MediaQueryListEvent | MediaQueryList): void {
    isCompactStrategyRuntime.value = event.matches;
  }

  function syncMobileStrategyRuntime(event: MediaQueryListEvent | MediaQueryList): void {
    isMobileStrategyRuntime.value = event.matches;
    if (!event.matches && strategyRuntimeMobileSection.value !== "instances") {
      strategyRuntimeMobileSection.value = "instances";
    }
  }

  function setupStrategyRuntimeMediaQueries(): void {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") return;
    compactMediaQuery = window.matchMedia(COMPACT_MEDIA_QUERY);
    mobileMediaQuery = window.matchMedia(MOBILE_MEDIA_QUERY);
    isCompactStrategyRuntime.value = compactMediaQuery.matches;
    isMobileStrategyRuntime.value = mobileMediaQuery.matches;
    if (typeof compactMediaQuery.addEventListener === "function") {
      compactMediaQuery.addEventListener("change", syncCompactStrategyRuntime);
      mobileMediaQuery.addEventListener("change", syncMobileStrategyRuntime);
    } else {
      compactMediaQuery.addListener(syncCompactStrategyRuntime);
      mobileMediaQuery.addListener(syncMobileStrategyRuntime);
    }
  }

  function teardownStrategyRuntimeMediaQueries(): void {
    if (compactMediaQuery !== null) {
      if (typeof compactMediaQuery.removeEventListener === "function") {
        compactMediaQuery.removeEventListener("change", syncCompactStrategyRuntime);
      } else {
        compactMediaQuery.removeListener(syncCompactStrategyRuntime);
      }
    }
    if (mobileMediaQuery !== null) {
      if (typeof mobileMediaQuery.removeEventListener === "function") {
        mobileMediaQuery.removeEventListener("change", syncMobileStrategyRuntime);
      } else {
        mobileMediaQuery.removeListener(syncMobileStrategyRuntime);
      }
    }
    compactMediaQuery = null;
    mobileMediaQuery = null;
  }

  function selectStrategyRuntimeMobileSection(section: StrategyRuntimeMobileSection): void {
    strategyRuntimeMobileSection.value =
      section === "workbench" && selectedStrategy.value === null ? "instances" : section;
  }

  function handleRuntimePaneResized(payload: SplitpanesResizedPayload): void {
    const sizes = payload.panes?.map((pane) => pane.size);
    if (
      sizes == null ||
      sizes.length !== 2 ||
      !sizes.every((size) => Number.isFinite(size) && size > 0 && size <= 100)
    ) {
      return;
    }
    runtimePaneSizes.value = [sizes[0]!, sizes[1]!];
  }

  return {
    runtimePaneSizes,
    isCompactStrategyRuntime,
    isMobileStrategyRuntime,
    strategyRuntimeMobileSection,
    strategyRuntimeWorkbenchLayout,
    syncCompactStrategyRuntime,
    syncMobileStrategyRuntime,
    setupStrategyRuntimeMediaQueries,
    teardownStrategyRuntimeMediaQueries,
    selectStrategyRuntimeMobileSection,
    handleRuntimePaneResized,
  };
}
