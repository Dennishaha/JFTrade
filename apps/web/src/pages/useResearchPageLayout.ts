import type { SplitpanesResizedPayload } from "splitpanes";
import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
  type Ref,
} from "vue";

import type { ResearchQuoteTarget } from "../components/research/researchQuote";
import {
  clampResearchPaneSizesForWidth,
  readResearchViewState,
  researchPaneBoundsForWidth,
  writeResearchViewState,
} from "@/composables/research/useResearchViewState";

export function useResearchPageLayout(
  selectedQuoteTarget: Ref<ResearchQuoteTarget | null>,
) {
  const initialState = readResearchViewState();
  const marketRailCollapsed = ref(initialState.railCollapsed);
  const marketRailDrawer = ref(false);
  const marketPaneSizes = ref<[number, number]>(initialState.paneSizes);
  const researchPageRef = ref<HTMLElement | null>(null);
  const researchPageWidth = ref(0);
  const researchPaneBounds = computed(() =>
    researchPaneBoundsForWidth(researchPageWidth.value),
  );
  let railMediaQuery: MediaQueryList | null = null;
  let researchResizeObserver: ResizeObserver | null = null;
  let suppressRailPersistence = false;

  function persistResearchView(): void {
    writeResearchViewState({
      railCollapsed: marketRailCollapsed.value,
      paneSizes: marketPaneSizes.value,
    });
  }

  function syncRailMode(matches: boolean): void {
    const becameNarrow = matches && !marketRailDrawer.value;
    marketRailDrawer.value = matches;
    if (becameNarrow && selectedQuoteTarget.value == null) {
      suppressRailPersistence = true;
      marketRailCollapsed.value = true;
      queueMicrotask(() => {
        suppressRailPersistence = false;
      });
    }
  }

  function handleRailMediaChange(event: MediaQueryListEvent): void {
    syncRailMode(event.matches);
  }

  function syncResearchPageWidth(width: number): void {
    if (!Number.isFinite(width) || width <= 0) return;
    researchPageWidth.value = width;
    if (marketRailDrawer.value || marketRailCollapsed.value) return;
    const normalized = clampResearchPaneSizesForWidth(
      marketPaneSizes.value,
      width,
    );
    if (
      Math.abs(normalized[0] - marketPaneSizes.value[0]) < 0.01 &&
      Math.abs(normalized[1] - marketPaneSizes.value[1]) < 0.01
    ) {
      return;
    }
    marketPaneSizes.value = normalized;
    persistResearchView();
  }

  function handleMarketPaneResized(payload: SplitpanesResizedPayload): void {
    if (marketRailDrawer.value || marketRailCollapsed.value) return;
    const sizes = payload.panes?.map((pane) => pane.size);
    if (
      sizes == null ||
      sizes.length !== 2 ||
      !sizes.every((size) => Number.isFinite(size) && size > 0 && size <= 100)
    ) {
      return;
    }
    marketPaneSizes.value = clampResearchPaneSizesForWidth(
      [sizes[0]!, sizes[1]!],
      researchPageWidth.value,
    );
    persistResearchView();
  }

  watch(marketRailCollapsed, () => {
    if (!suppressRailPersistence) persistResearchView();
  });

  onMounted(() => {
    if (typeof window.matchMedia === "function") {
      railMediaQuery = window.matchMedia("(max-width: 1100px)");
      syncRailMode(railMediaQuery.matches);
      railMediaQuery.addEventListener("change", handleRailMediaChange);
    }
    const element = researchPageRef.value;
    if (element == null) return;
    syncResearchPageWidth(element.getBoundingClientRect().width);
    if (typeof ResizeObserver !== "undefined") {
      researchResizeObserver = new ResizeObserver((entries) => {
        const width =
          entries[0]?.contentRect.width ??
          researchPageRef.value?.getBoundingClientRect().width ??
          0;
        syncResearchPageWidth(width);
      });
      researchResizeObserver.observe(element);
    }
  });

  onBeforeUnmount(() => {
    railMediaQuery?.removeEventListener("change", handleRailMediaChange);
    railMediaQuery = null;
    researchResizeObserver?.disconnect();
    researchResizeObserver = null;
  });

  return {
    handleMarketPaneResized,
    handleRailMediaChange,
    marketPaneSizes,
    marketRailCollapsed,
    marketRailDrawer,
    persistResearchView,
    researchPageRef,
    researchPageWidth,
    researchPaneBounds,
    syncRailMode,
    syncResearchPageWidth,
  };
}
