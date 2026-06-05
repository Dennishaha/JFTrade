<script setup lang="ts">
import type { SplitpanesResizedPayload } from "splitpanes";

import LightweightChart from "../components/workspace/LightweightChart.vue";
import OrderBookPanel from "../components/workspace/OrderBookPanel.vue";
import OrderEntryPanel from "../components/workspace/OrderEntryPanel.vue";
import PositionsPanel from "../components/workspace/PositionsPanel.vue";
import SplitPane from "../components/shared/SplitPane.vue";
import SplitPaneItem from "../components/shared/SplitPaneItem.vue";
import WatchlistPanel from "../components/workspace/WatchlistPanel.vue";
import {
  useWorkspaceLayout,
  type WorkspacePaneSizeKey,
} from "../composables/useWorkspaceLayout";

const { prefs, update } = useWorkspaceLayout();

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
</script>

<template>
  <div class="tv-workspace">
    <!-- Left-right split: left (chart + positions|order) | right (watchlist + orderbook) -->
    <SplitPane :pane-min-size="10" @resized="handlePaneResized('main', $event)">
      <!-- Left column: chart on top, positions | order on bottom -->
      <SplitPaneItem :size="prefs.paneSizes.main[0]">
        <SplitPane horizontal :pane-min-size="10" @resized="handlePaneResized('leftColumn', $event)">
          <SplitPaneItem :size="prefs.paneSizes.leftColumn[0]">
            <LightweightChart />
          </SplitPaneItem>
          <SplitPaneItem :size="prefs.paneSizes.leftColumn[1]" :min-size="15">
            <!-- Positions | Order side by side -->
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
</template>
