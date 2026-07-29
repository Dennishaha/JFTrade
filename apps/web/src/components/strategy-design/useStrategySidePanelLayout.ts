import { computed, ref, type Ref } from "vue";

export type StrategyMobileSection = "definition" | "instruction" | "code";
export type StrategyDisplayMode = "instruction" | "split" | "code";
export type StrategySidePanelId =
  | "definition"
  | "history"
  | "declaration"
  | "diagnostics"
  | "instances";
type StrategySidePanelDropEdge = "before" | "after";

const STRATEGY_MEDIUM_WORKBENCH_QUERY = "(min-width: 769px) and (max-width: 1180px)";
const DEFAULT_STRATEGY_SIDE_PANEL_ORDER: StrategySidePanelId[] = [
  "definition",
  "history",
  "declaration",
  "diagnostics",
  "instances",
];

export function useStrategySidePanelLayout(
  strategyDisplayMode: Ref<StrategyDisplayMode>,
  strategyMobileSection: Ref<StrategyMobileSection>,
  metadataPaneOpen: Ref<boolean>,
) {
  const isMediumWorkbench = ref(false);
  const isWideWorkbench = ref(true);
  const expandedStrategySidePanels = ref<string[]>(["definition"]);
  const strategySidePanelOrder = ref<StrategySidePanelId[]>([
    ...DEFAULT_STRATEGY_SIDE_PANEL_ORDER,
  ]);
  const draggedStrategySidePanelId = ref<StrategySidePanelId | null>(null);
  const strategySidePanelDropTarget = ref<{
    id: StrategySidePanelId;
    edge: StrategySidePanelDropEdge;
  } | null>(null);
  let mediumWorkbenchMediaQuery: MediaQueryList | null = null;

  const fillStrategySidePanelId = computed<StrategySidePanelId | null>(() =>
    [...strategySidePanelOrder.value]
      .reverse()
      .find((panelId) => expandedStrategySidePanels.value.includes(panelId)) ?? null,
  );
  const expandedStrategySidePanelCount = computed(() =>
    strategySidePanelOrder.value.filter((panelId) =>
      expandedStrategySidePanels.value.includes(panelId),
    ).length,
  );

  function setStrategyDisplayMode(mode: StrategyDisplayMode): void {
    strategyDisplayMode.value = mode;
  }

  function toggleMetadataPane(): void {
    metadataPaneOpen.value = !metadataPaneOpen.value;
  }

  function closeMetadataPane(): void {
    metadataPaneOpen.value = false;
  }

  function strategySidePanelPosition(panelId: StrategySidePanelId): number {
    return strategySidePanelOrder.value.indexOf(panelId);
  }

  function strategySidePanelClasses(panelId: StrategySidePanelId): Record<string, boolean> {
    return {
      "is-fill-panel": fillStrategySidePanelId.value === panelId,
      "is-first-panel": strategySidePanelPosition(panelId) === 0,
      "is-dragging": draggedStrategySidePanelId.value === panelId,
      "is-drop-before": strategySidePanelDropTarget.value?.id === panelId
        && strategySidePanelDropTarget.value.edge === "before",
      "is-drop-after": strategySidePanelDropTarget.value?.id === panelId
        && strategySidePanelDropTarget.value.edge === "after",
    };
  }

  function moveStrategySidePanel(panelId: StrategySidePanelId, targetIndex: number): void {
    const currentIndex = strategySidePanelPosition(panelId);
    if (currentIndex < 0) return;
    const nextOrder = [...strategySidePanelOrder.value];
    nextOrder.splice(currentIndex, 1);
    const boundedTargetIndex = Math.max(0, Math.min(targetIndex, nextOrder.length));
    nextOrder.splice(boundedTargetIndex, 0, panelId);
    strategySidePanelOrder.value = nextOrder;
  }

  function handleStrategySidePanelDragStart(
    event: DragEvent,
    panelId: StrategySidePanelId,
  ): void {
    if (!isWideWorkbench.value) {
      event.preventDefault();
      return;
    }
    draggedStrategySidePanelId.value = panelId;
    strategySidePanelDropTarget.value = null;
    if (event.dataTransfer !== null) {
      event.dataTransfer.effectAllowed = "move";
      event.dataTransfer.setData("text/plain", panelId);
    }
  }

  function handleStrategySidePanelDragOver(
    event: DragEvent,
    panelId: StrategySidePanelId,
  ): void {
    if (draggedStrategySidePanelId.value === null || draggedStrategySidePanelId.value === panelId) {
      strategySidePanelDropTarget.value = null;
      return;
    }
    const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect();
    strategySidePanelDropTarget.value = {
      id: panelId,
      edge: event.clientY < bounds.top + bounds.height / 2 ? "before" : "after",
    };
    if (event.dataTransfer !== null) event.dataTransfer.dropEffect = "move";
  }

  function finishStrategySidePanelDrag(): void {
    draggedStrategySidePanelId.value = null;
    strategySidePanelDropTarget.value = null;
  }

  function handleStrategySidePanelDrop(event: DragEvent): void {
    const panelId = draggedStrategySidePanelId.value;
    const dropTarget = strategySidePanelDropTarget.value;
    if (panelId === null || dropTarget === null || panelId === dropTarget.id) {
      finishStrategySidePanelDrag();
      return;
    }
    const remaining = strategySidePanelOrder.value.filter((item) => item !== panelId);
    const targetIndex = remaining.indexOf(dropTarget.id);
    moveStrategySidePanel(panelId, targetIndex + (dropTarget.edge === "after" ? 1 : 0));
    finishStrategySidePanelDrag();
    event.preventDefault();
  }

  function syncMediumWorkbench(event: MediaQueryListEvent | MediaQueryList): void {
    const wasMediumWorkbench = isMediumWorkbench.value;
    isMediumWorkbench.value = event.matches;
    isWideWorkbench.value = typeof window === "undefined" || window.innerWidth > 1180;
    if (event.matches) {
      metadataPaneOpen.value = false;
    } else if (wasMediumWorkbench) {
      metadataPaneOpen.value = true;
    }
  }

  function handleWorkbenchKeydown(event: KeyboardEvent): void {
    if (event.key === "Escape" && isMediumWorkbench.value && metadataPaneOpen.value) {
      closeMetadataPane();
    }
  }

  function installStrategyWorkbenchMediaQuery(): void {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") return;
    mediumWorkbenchMediaQuery = window.matchMedia(STRATEGY_MEDIUM_WORKBENCH_QUERY);
    syncMediumWorkbench(mediumWorkbenchMediaQuery);
    if (typeof mediumWorkbenchMediaQuery.addEventListener === "function") {
      mediumWorkbenchMediaQuery.addEventListener("change", syncMediumWorkbench);
    } else {
      mediumWorkbenchMediaQuery.addListener(syncMediumWorkbench);
    }
  }

  function disposeStrategyWorkbenchMediaQuery(): void {
    if (typeof mediumWorkbenchMediaQuery?.removeEventListener === "function") {
      mediumWorkbenchMediaQuery.removeEventListener("change", syncMediumWorkbench);
    } else {
      mediumWorkbenchMediaQuery?.removeListener(syncMediumWorkbench);
    }
    mediumWorkbenchMediaQuery = null;
  }

  function setStrategyMobileSection(section: StrategyMobileSection): void {
    strategyMobileSection.value = section;
    setStrategyDisplayMode(section === "code" ? "code" : "instruction");
  }

  return {
    isMediumWorkbench,
    isWideWorkbench,
    expandedStrategySidePanels,
    strategySidePanelOrder,
    draggedStrategySidePanelId,
    strategySidePanelDropTarget,
    fillStrategySidePanelId,
    expandedStrategySidePanelCount,
    setStrategyDisplayMode,
    toggleMetadataPane,
    closeMetadataPane,
    strategySidePanelPosition,
    strategySidePanelClasses,
    moveStrategySidePanel,
    handleStrategySidePanelDragStart,
    handleStrategySidePanelDragOver,
    finishStrategySidePanelDrag,
    handleStrategySidePanelDrop,
    syncMediumWorkbench,
    handleWorkbenchKeydown,
    installStrategyWorkbenchMediaQuery,
    disposeStrategyWorkbenchMediaQuery,
    setStrategyMobileSection,
  };
}
