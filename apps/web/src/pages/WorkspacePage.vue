<script setup lang="ts">
import type { SplitpanesResizedPayload } from "splitpanes";
import { onMounted, onUnmounted, ref } from "vue";

import LightweightChart from "../components/workspace/LightweightChart.vue";
import OrderBookPanel from "../components/workspace/OrderBookPanel.vue";
import OrderEntryPanel from "../components/workspace/OrderEntryPanel.vue";
import PositionsPanel from "../components/workspace/PositionsPanel.vue";
import SplitPane from "../components/shared/SplitPane.vue";
import SplitPaneItem from "../components/shared/SplitPaneItem.vue";
import WatchlistPanel from "../components/workspace/WatchlistPanel.vue";
import {
  useWorkspaceViewState,
  type WorkspacePaneSizeKey,
} from "../composables/useWorkspaceLayout";

const { prefs, update } = useWorkspaceViewState();
const WORKSPACE_COMPACT_MEDIA_QUERY = "(max-width: 1180px)";
const isCompactWorkspace = ref(false);
let compactWorkspaceMediaQuery: MediaQueryList | null = null;

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
  <div class="tv-workspace tv-workspace--scoped">
    <div v-if="isCompactWorkspace" class="tv-workspace__compact-stack">
      <section class="tv-workspace__compact-panel tv-workspace__compact-panel--chart">
        <LightweightChart />
      </section>
      <section class="tv-workspace__compact-panel">
        <PositionsPanel />
      </section>
      <section class="tv-workspace__compact-panel">
        <OrderEntryPanel />
      </section>
      <section class="tv-workspace__compact-panel">
        <WatchlistPanel />
      </section>
      <section class="tv-workspace__compact-panel">
        <OrderBookPanel />
      </section>
    </div>

    <div v-else class="tv-workspace__body">
      <SplitPane :pane-min-size="10" @resized="handlePaneResized('main', $event)">
        <SplitPaneItem :size="prefs.paneSizes.main[0]">
          <SplitPane horizontal :pane-min-size="10" @resized="handlePaneResized('leftColumn', $event)">
            <SplitPaneItem :size="prefs.paneSizes.leftColumn[0]">
              <LightweightChart />
            </SplitPaneItem>
            <SplitPaneItem :size="prefs.paneSizes.leftColumn[1]" :min-size="15">
              <SplitPane :pane-min-size="10" @resized="handlePaneResized('bottom', $event)">
                <SplitPaneItem :size="prefs.paneSizes.bottom[0]" :min-size="15">
                  <PositionsPanel />
                </SplitPaneItem>
                <SplitPaneItem :size="prefs.paneSizes.bottom[1]" :min-size="18">
                  <OrderEntryPanel />
                </SplitPaneItem>
              </SplitPane>
            </SplitPaneItem>
          </SplitPane>
        </SplitPaneItem>

        <!-- Right column: watchlist on top, orderbook below -->
        <SplitPaneItem :size="prefs.paneSizes.main[1]" :min-size="15">
          <SplitPane horizontal :pane-min-size="10" @resized="handlePaneResized('rightColumn', $event)">
            <SplitPaneItem :size="prefs.paneSizes.rightColumn[0]" :min-size="12">
              <WatchlistPanel />
            </SplitPaneItem>
            <SplitPaneItem :size="prefs.paneSizes.rightColumn[1]" :min-size="15">
              <OrderBookPanel />
            </SplitPaneItem>
          </SplitPane>
        </SplitPaneItem>
      </SplitPane>
    </div>
  </div>
</template>

<style scoped>
.tv-workspace__compact-stack {
  display: grid;
  gap: 8px;
  min-width: 0;
  min-height: 100%;
  padding: 6px;
  overflow: auto;
  scrollbar-gutter: stable both-edges;
}

.tv-workspace__compact-panel {
  min-width: 0;
  min-height: 300px;
  overflow: hidden;
  border: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.tv-workspace__compact-panel--chart {
  min-height: 380px;
}

@media (max-width: 768px) {
  .tv-workspace.tv-workspace--scoped {
    scrollbar-gutter: auto;
  }

  .tv-workspace__compact-stack {
    gap: 6px;
    padding: 2px;
    scrollbar-gutter: auto;
  }

  .tv-workspace__compact-panel {
    min-height: 280px;
  }

  .tv-workspace__compact-panel--chart {
    min-height: 340px;
  }
}
</style>
