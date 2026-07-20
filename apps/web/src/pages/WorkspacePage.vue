<script setup lang="ts">
import type { SplitpanesResizedPayload } from "splitpanes";
import { computed, onMounted, onUnmounted, ref } from "vue";

import PredictionContractWorkspacePanel from "../components/product/PredictionContractWorkspacePanel.vue";
import OptionTradingDock from "../components/product/OptionTradingDock.vue";
import WorkspaceWatchlistSidebar from "../components/domain/watchlist/WorkspaceWatchlistSidebar.vue";
import WorkspaceProductPane from "../components/workspace/WorkspaceProductPane.vue";
import OrderBookPanel from "../components/workspace/OrderBookPanel.vue";
import OrderEntryPanel from "../components/workspace/OrderEntryPanel.vue";
import PositionsPanel from "../components/workspace/PositionsPanel.vue";
import SplitPane from "../components/shared/SplitPane.vue";
import SplitPaneItem from "../components/shared/SplitPaneItem.vue";
import InstrumentOverviewPanel from "../components/workspace/InstrumentOverviewPanel.vue";
import { useOptionComboDraftStore } from "../composables/optionComboDraft";
import {
  useWorkspaceTradingPrefs,
  useWorkspaceViewState,
  type WorkspacePaneSizeKey,
} from "../composables/useWorkspaceLayout";

const { prefs, update } = useWorkspaceViewState();
const { prefs: tradingPrefs } = useWorkspaceTradingPrefs();
const optionComboDraft = useOptionComboDraftStore();
const isPrediction = computed(
  () => tradingPrefs.value.marketSegment === "prediction",
);
const isOptionWorkspace = computed(
  () => optionComboDraft.workspaceActive.value && !isPrediction.value,
);
const predictionInstrumentId = computed(
  () => `${tradingPrefs.value.market}.${tradingPrefs.value.symbol}`,
);
const WORKSPACE_COMPACT_MEDIA_QUERY = "(max-width: 1180px)";
const isCompactWorkspace = ref(false);
const optionDockCollapsed = ref(false);
let compactWorkspaceMediaQuery: MediaQueryList | null = null;
let watchlistResizeStart: {
  pointerId: number;
  startX: number;
  startWidth: number;
} | null = null;

function syncCompactWorkspace(
  event: MediaQueryListEvent | MediaQueryList,
): void {
  isCompactWorkspace.value = event.matches;
}

function handlePaneResized(
  key: WorkspacePaneSizeKey,
  payload: SplitpanesResizedPayload,
): void {
  const sizes = payload.panes?.map((pane) => pane.size);
  if (
    sizes == null ||
    sizes.length !== 2 ||
    !sizes.every((size) => Number.isFinite(size) && size > 0 && size <= 100)
  ) {
    return;
  }

  update({
    paneSizes: {
      [key]: [sizes[0]!, sizes[1]!],
    },
  });
}

function setWatchlistSidebarOpen(open: boolean): void {
  update({ watchlistSidebarOpen: open });
}

function clampWatchlistSidebarWidth(width: number): number {
  return Math.min(420, Math.max(220, Math.round(width)));
}

function startWatchlistResize(event: PointerEvent): void {
  if (isCompactWorkspace.value || !prefs.value.watchlistSidebarOpen) return;
  watchlistResizeStart = {
    pointerId: event.pointerId,
    startX: event.clientX,
    startWidth: prefs.value.watchlistSidebarWidth,
  };
  window.addEventListener("pointermove", handleWatchlistResizeMove);
  window.addEventListener("pointerup", stopWatchlistResize);
  window.addEventListener("pointercancel", stopWatchlistResize);
  event.preventDefault();
}

function handleWatchlistResizeMove(event: PointerEvent): void {
  if (watchlistResizeStart == null) return;
  update({
    watchlistSidebarWidth: clampWatchlistSidebarWidth(
      watchlistResizeStart.startWidth + event.clientX - watchlistResizeStart.startX,
    ),
  });
}

function handleWatchlistResizeKeydown(event: KeyboardEvent): void {
  const step = event.shiftKey ? 25 : 10;
  let width = prefs.value.watchlistSidebarWidth;
  switch (event.key) {
    case "ArrowLeft":
      width -= step;
      break;
    case "ArrowRight":
      width += step;
      break;
    case "Home":
      width = 220;
      break;
    case "End":
      width = 420;
      break;
    default:
      return;
  }
  event.preventDefault();
  update({ watchlistSidebarWidth: clampWatchlistSidebarWidth(width) });
}

function stopWatchlistResize(event?: PointerEvent): void {
  if (
    event != null &&
    watchlistResizeStart != null &&
    event.pointerId !== watchlistResizeStart.pointerId
  ) {
    return;
  }
  watchlistResizeStart = null;
  window.removeEventListener("pointermove", handleWatchlistResizeMove);
  window.removeEventListener("pointerup", stopWatchlistResize);
  window.removeEventListener("pointercancel", stopWatchlistResize);
}

function handleWatchlistItemSelected(): void {
  if (isCompactWorkspace.value) {
    setWatchlistSidebarOpen(false);
  }
}

onMounted(() => {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return;
  }
  compactWorkspaceMediaQuery = window.matchMedia(WORKSPACE_COMPACT_MEDIA_QUERY);
  isCompactWorkspace.value = compactWorkspaceMediaQuery.matches;
  if (typeof compactWorkspaceMediaQuery.addEventListener === "function") {
    compactWorkspaceMediaQuery.addEventListener("change", syncCompactWorkspace);
  } else {
    compactWorkspaceMediaQuery.addListener(syncCompactWorkspace);
  }
});

onUnmounted(() => {
  stopWatchlistResize();
  if (compactWorkspaceMediaQuery == null) {
    return;
  }
  if (typeof compactWorkspaceMediaQuery.removeEventListener === "function") {
    compactWorkspaceMediaQuery.removeEventListener(
      "change",
      syncCompactWorkspace,
    );
  } else {
    compactWorkspaceMediaQuery.removeListener(syncCompactWorkspace);
  }
  compactWorkspaceMediaQuery = null;
});
</script>

<template>
  <div class="tv-workspace tv-workspace--scoped" data-capability-surface="workspace.root">
    <button
      v-if="!prefs.watchlistSidebarOpen"
      type="button"
      class="tv-workspace__watchlist-open"
      title="显示自选栏"
      aria-label="显示自选栏"
      @click="setWatchlistSidebarOpen(true)"
    >
      <v-icon>fa-regular fa-star</v-icon>
      <span>自选</span>
    </button>

    <template v-if="isCompactWorkspace && prefs.watchlistSidebarOpen">
      <button
        type="button"
        class="tv-workspace__watchlist-backdrop"
        aria-label="关闭自选栏"
        @click="setWatchlistSidebarOpen(false)"
      />
      <div
        class="tv-workspace__watchlist-drawer"
        :style="{ width: `${Math.min(prefs.watchlistSidebarWidth, 360)}px` }"
      >
        <WorkspaceWatchlistSidebar
          compact
          @close="setWatchlistSidebarOpen(false)"
          @selected="handleWatchlistItemSelected"
        />
      </div>
    </template>

    <div
      class="tv-workspace__desktop-shell"
      :class="{ 'is-compact': isCompactWorkspace }"
    >
      <div
        v-if="!isCompactWorkspace && prefs.watchlistSidebarOpen"
        class="tv-workspace__watchlist-slot"
        :style="{ width: `${prefs.watchlistSidebarWidth}px` }"
      >
        <WorkspaceWatchlistSidebar
          @close="setWatchlistSidebarOpen(false)"
          @selected="handleWatchlistItemSelected"
        />
        <div
          class="tv-workspace__watchlist-resizer tv-resizer--vertical"
          role="separator"
          tabindex="0"
          aria-orientation="vertical"
          aria-label="调整自选栏宽度"
          :aria-valuemin="220"
          :aria-valuemax="420"
          :aria-valuenow="prefs.watchlistSidebarWidth"
          title="拖拽调整自选栏宽度"
          @pointerdown="startWatchlistResize"
          @keydown="handleWatchlistResizeKeydown"
        />
      </div>
      <div class="tv-workspace__body">
        <SplitPane :pane-min-size="10" @resized="handlePaneResized('main', $event)">
          <SplitPaneItem :size="prefs.paneSizes.main[0]">
            <SplitPane
              horizontal
              class="tv-workspace__left-split"
              :class="{
                'is-option-dock-collapsed':
                  isOptionWorkspace && optionDockCollapsed,
              }"
              :pane-min-size="10"
              @resized="handlePaneResized('leftColumn', $event)"
            >
              <SplitPaneItem :size="prefs.paneSizes.leftColumn[0]">
                <section
                  class="tv-workspace__responsive-panel tv-workspace__responsive-panel--chart"
                >
                  <WorkspaceProductPane />
                </section>
              </SplitPaneItem>
              <SplitPaneItem :size="prefs.paneSizes.leftColumn[1]" :min-size="15">
                <section
                  v-if="isOptionWorkspace"
                  class="tv-workspace__responsive-panel tv-workspace__responsive-panel--option-dock"
                  :class="{ 'is-option-dock-collapsed': optionDockCollapsed }"
                  data-capability-surface="workspace.order"
                >
                  <OptionTradingDock v-model:collapsed="optionDockCollapsed" />
                </section>
                <SplitPane v-else :pane-min-size="10" @resized="handlePaneResized('bottom', $event)">
                  <SplitPaneItem :size="prefs.paneSizes.bottom[0]" :min-size="15">
                    <section class="tv-workspace__responsive-panel">
                      <PositionsPanel />
                    </section>
                  </SplitPaneItem>
                  <SplitPaneItem :size="prefs.paneSizes.bottom[1]" :min-size="18" data-capability-surface="workspace.order">
                    <section class="tv-workspace__responsive-panel">
                      <OrderEntryPanel />
                    </section>
                  </SplitPaneItem>
                </SplitPane>
              </SplitPaneItem>
            </SplitPane>
          </SplitPaneItem>

          <!-- Right column: instrument overview on top, orderbook below -->
          <SplitPaneItem :size="prefs.paneSizes.main[1]" :min-size="15">
            <SplitPane horizontal :pane-min-size="10" @resized="handlePaneResized('rightColumn', $event)">
              <SplitPaneItem :size="prefs.paneSizes.rightColumn[0]" :min-size="12">
                <section class="tv-workspace__responsive-panel">
                  <InstrumentOverviewPanel v-if="!isPrediction" />
                  <PredictionContractWorkspacePanel
                    v-else
                    :instrument-id="predictionInstrumentId"
                    view="contract"
                  />
                </section>
              </SplitPaneItem>
              <SplitPaneItem :size="prefs.paneSizes.rightColumn[1]" :min-size="15">
                <section class="tv-workspace__responsive-panel">
                  <OrderBookPanel v-if="!isPrediction" />
                  <PredictionContractWorkspacePanel
                    v-else
                    :instrument-id="predictionInstrumentId"
                    view="depth"
                  />
                </section>
              </SplitPaneItem>
            </SplitPane>
          </SplitPaneItem>
        </SplitPane>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tv-workspace.tv-workspace--scoped {
  position: relative;
}

.tv-workspace__desktop-shell {
  display: flex;
  min-width: 0;
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.tv-workspace__responsive-panel {
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.tv-workspace__watchlist-slot {
  position: relative;
  min-width: 220px;
  max-width: 420px;
  flex: 0 0 auto;
}

.tv-workspace__watchlist-resizer {
  position: absolute;
  top: 0;
  right: -3px;
  bottom: 0;
  z-index: 4;
  cursor: col-resize;
}

.tv-workspace__watchlist-open {
  position: absolute;
  top: 44px;
  left: 8px;
  z-index: 12;
  display: inline-flex;
  height: 31px;
  align-items: center;
  gap: 6px;
  padding: 0 9px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: color-mix(in srgb, var(--tv-bg-elevated) 92%, transparent);
  color: var(--tv-text-muted);
  box-shadow: 0 5px 18px rgba(2, 6, 23, .14);
  font-size: 10px;
}

.tv-workspace__watchlist-open:hover {
  border-color: var(--tv-accent);
  color: var(--tv-accent);
}

.tv-workspace__watchlist-backdrop {
  position: absolute;
  inset: 0;
  z-index: 34;
  border: 0;
  background: rgba(2, 6, 23, .42);
}

.tv-workspace__watchlist-drawer {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  z-index: 35;
  max-width: calc(100vw - 28px);
}

.tv-workspace__left-split.is-option-dock-collapsed
  > :deep(.splitpanes__pane:first-child) {
  height: calc(100% - 36px) !important;
}

.tv-workspace__left-split.is-option-dock-collapsed
  > :deep(.splitpanes__splitter) {
  display: none;
}

.tv-workspace__left-split.is-option-dock-collapsed
  > :deep(.splitpanes__pane:last-child) {
  height: 36px !important;
}

.tv-workspace__desktop-shell.is-compact {
  display: block;
  padding: 6px;
  overflow: auto;
  scrollbar-gutter: stable both-edges;
}

.tv-workspace__desktop-shell.is-compact .tv-workspace__body {
  display: grid;
  height: auto;
  gap: 8px;
  overflow: visible;
}

.tv-workspace__desktop-shell.is-compact
  .tv-workspace__body
  :deep(.tv-splitpanes),
.tv-workspace__desktop-shell.is-compact
  .tv-workspace__body
  :deep(.tv-pane) {
  display: contents !important;
}

.tv-workspace__desktop-shell.is-compact
  .tv-workspace__body
  :deep(.splitpanes__splitter) {
  display: none;
}

.tv-workspace__desktop-shell.is-compact .tv-workspace__responsive-panel {
  height: auto;
  min-height: 300px;
  border: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.tv-workspace__desktop-shell.is-compact
  .tv-workspace__responsive-panel--chart {
  height: clamp(380px, 58vh, 640px);
  min-height: 380px;
}

.tv-workspace__desktop-shell.is-compact
  .tv-workspace__responsive-panel--option-dock {
  min-height: 420px;
}

.tv-workspace__desktop-shell.is-compact
  .tv-workspace__responsive-panel--option-dock.is-option-dock-collapsed {
  height: 36px;
  min-height: 36px;
}

@media (max-width: 768px) {
  .tv-workspace.tv-workspace--scoped {
    scrollbar-gutter: auto;
  }

  .tv-workspace__desktop-shell.is-compact {
    padding: 2px;
    scrollbar-gutter: auto;
  }

  .tv-workspace__desktop-shell.is-compact .tv-workspace__body {
    gap: 6px;
  }

  .tv-workspace__desktop-shell.is-compact .tv-workspace__responsive-panel {
    min-height: 280px;
  }

  .tv-workspace__desktop-shell.is-compact
    .tv-workspace__responsive-panel--chart {
    height: clamp(340px, 58vh, 560px);
    min-height: 340px;
  }

  .tv-workspace__desktop-shell.is-compact
    .tv-workspace__responsive-panel--option-dock.is-option-dock-collapsed {
    height: 36px;
    min-height: 36px;
  }
}
</style>
