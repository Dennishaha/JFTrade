import type { SplitpanesResizedPayload } from "splitpanes";
import {
  onBeforeUnmount,
  onMounted,
  ref,
} from "vue";

import type { BacktestReportTab } from "@/components/backtest/backtestRunPresentation";
export type BacktestReportMode = "single" | "compare";
export type BacktestMobileSection = "setup" | "report";
export type BacktestSidePanelId = "setup" | "history";

export const BACKTEST_MEDIUM_WORKBENCH_QUERY =
  "(min-width: 769px) and (max-width: 1180px)";

export function useBacktestPageLayout(input: {
  canShowReport: () => boolean;
}) {
  const activeReportTab = ref<BacktestReportTab>("chart");
  const selectedRunId = ref("");
  const showNewBacktestForm = ref(false);
  const newBacktestFormTouched = ref(false);
  const backtestPaneSizes = ref<[number, number]>([30, 70]);
  const backtestMobileSection = ref<BacktestMobileSection>("setup");
  const backtestSidebarOpen = ref(true);
  const isMediumBacktestWorkbench = ref(false);
  const errorExpanded = ref(false);
  const expandedBacktestPanels = ref<BacktestSidePanelId[]>(["history"]);
  let mediumWorkbenchMediaQuery: MediaQueryList | null = null;

  function setBacktestSetupPanelOpen(open: boolean): void {
    const nextPanels: BacktestSidePanelId[] = expandedBacktestPanels.value.filter(
      (panel) => panel !== "setup",
    );
    if (open) nextPanels.unshift("setup");
    if (!nextPanels.includes("history")) nextPanels.push("history");
    expandedBacktestPanels.value = nextPanels;
    showNewBacktestForm.value = open;
  }

  function handleBacktestPanelsUpdate(value: unknown): void {
    const panels = Array.isArray(value)
      ? value.filter(
          (panel): panel is BacktestSidePanelId =>
            panel === "setup" || panel === "history",
        )
      : [];
    expandedBacktestPanels.value = panels;
    const setupOpen = panels.includes("setup");
    if (setupOpen !== showNewBacktestForm.value) {
      newBacktestFormTouched.value = true;
      showNewBacktestForm.value = setupOpen;
    }
  }

  function toggleBacktestPanel(panel: BacktestSidePanelId): void {
    const nextPanels = expandedBacktestPanels.value.includes(panel)
      ? expandedBacktestPanels.value.filter((item) => item !== panel)
      : [...expandedBacktestPanels.value, panel];
    handleBacktestPanelsUpdate(nextPanels);
  }

  function openNewBacktestForm(): void {
    newBacktestFormTouched.value = true;
    setBacktestSetupPanelOpen(true);
    backtestSidebarOpen.value = true;
    backtestMobileSection.value = "setup";
  }

  function toggleNewBacktestForm(): void {
    newBacktestFormTouched.value = true;
    setBacktestSetupPanelOpen(!showNewBacktestForm.value);
    backtestSidebarOpen.value = true;
    backtestMobileSection.value = "setup";
  }

  function toggleBacktestSidebar(): void {
    if (typeof window !== "undefined" && window.innerWidth <= 768) {
      backtestMobileSection.value = "setup";
      return;
    }
    backtestSidebarOpen.value = !backtestSidebarOpen.value;
  }

  function closeBacktestSidebar(): void {
    backtestSidebarOpen.value = false;
  }

  function syncMediumBacktestWorkbench(
    event: MediaQueryListEvent | MediaQueryList,
  ): void {
    const wasMedium = isMediumBacktestWorkbench.value;
    isMediumBacktestWorkbench.value = event.matches;
    if (event.matches) {
      backtestSidebarOpen.value = false;
      return;
    }
    if (wasMedium) backtestSidebarOpen.value = true;
  }

  function handleBacktestWorkbenchKeydown(event: KeyboardEvent): void {
    if (
      event.key === "Escape" &&
      isMediumBacktestWorkbench.value &&
      backtestSidebarOpen.value
    ) {
      closeBacktestSidebar();
    }
  }

  function installBacktestWorkbenchMediaQuery(): void {
    if (typeof window === "undefined") return;
    window.addEventListener("keydown", handleBacktestWorkbenchKeydown);
    if (typeof window.matchMedia !== "function") return;
    mediumWorkbenchMediaQuery = window.matchMedia(
      BACKTEST_MEDIUM_WORKBENCH_QUERY,
    );
    syncMediumBacktestWorkbench(mediumWorkbenchMediaQuery);
    if (typeof mediumWorkbenchMediaQuery.addEventListener === "function") {
      mediumWorkbenchMediaQuery.addEventListener(
        "change",
        syncMediumBacktestWorkbench,
      );
    } else {
      mediumWorkbenchMediaQuery.addListener(syncMediumBacktestWorkbench);
    }
  }

  function disposeBacktestWorkbenchMediaQuery(): void {
    if (typeof window !== "undefined") {
      window.removeEventListener("keydown", handleBacktestWorkbenchKeydown);
    }
    if (typeof mediumWorkbenchMediaQuery?.removeEventListener === "function") {
      mediumWorkbenchMediaQuery.removeEventListener(
        "change",
        syncMediumBacktestWorkbench,
      );
    } else {
      mediumWorkbenchMediaQuery?.removeListener(syncMediumBacktestWorkbench);
    }
    mediumWorkbenchMediaQuery = null;
  }

  function selectBacktestMobileSection(section: BacktestMobileSection): void {
    if (
      section === "report" &&
      !input.canShowReport()
    ) {
      backtestMobileSection.value = "setup";
      return;
    }
    backtestMobileSection.value = section;
  }

  function handleBacktestPaneResized(payload: SplitpanesResizedPayload): void {
    const sizes = payload.panes?.map((pane) => pane.size);
    if (
      sizes == null ||
      sizes.length !== 2 ||
      !sizes.every((size) => Number.isFinite(size) && size > 0 && size <= 100)
    ) {
      return;
    }
    backtestPaneSizes.value = [sizes[0]!, sizes[1]!];
  }

  onMounted(installBacktestWorkbenchMediaQuery);
  onBeforeUnmount(disposeBacktestWorkbenchMediaQuery);

  return {
    activeReportTab,
    backtestMobileSection,
    backtestPaneSizes,
    backtestSidebarOpen,
    closeBacktestSidebar,
    disposeBacktestWorkbenchMediaQuery,
    errorExpanded,
    expandedBacktestPanels,
    handleBacktestPaneResized,
    handleBacktestPanelsUpdate,
    handleBacktestWorkbenchKeydown,
    installBacktestWorkbenchMediaQuery,
    isMediumBacktestWorkbench,
    newBacktestFormTouched,
    openNewBacktestForm,
    selectedRunId,
    selectBacktestMobileSection,
    setBacktestSetupPanelOpen,
    showNewBacktestForm,
    syncMediumBacktestWorkbench,
    toggleBacktestPanel,
    toggleBacktestSidebar,
    toggleNewBacktestForm,
  };
}
