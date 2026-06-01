<script setup lang="ts">
import LightweightChart from "../components/workspace/LightweightChart.vue";
import OrderBookPanel from "../components/workspace/OrderBookPanel.vue";
import OrderEntryPanel from "../components/workspace/OrderEntryPanel.vue";
import PositionsPanel from "../components/workspace/PositionsPanel.vue";
import SplitPane from "../components/shared/SplitPane.vue";
import SplitPaneItem from "../components/shared/SplitPaneItem.vue";
import WatchlistPanel from "../components/workspace/WatchlistPanel.vue";
</script>

<template>
  <div class="tv-workspace">
    <!-- Left-right split: left (chart + positions|order) | right (watchlist + orderbook) -->
    <SplitPane :pane-min-size="10">
      <!-- Left column: chart on top, positions | order on bottom -->
      <SplitPaneItem :size="72">
        <SplitPane horizontal :pane-min-size="10">
          <SplitPaneItem :size="60">
            <LightweightChart />
          </SplitPaneItem>
          <SplitPaneItem :size="40" :min-size="15">
            <!-- Positions | Order side by side -->
            <SplitPane :pane-min-size="10">
              <SplitPaneItem :size="60" :min-size="15">
                <PositionsPanel />
              </SplitPaneItem>
              <SplitPaneItem :size="40" :min-size="18">
                <OrderEntryPanel />
              </SplitPaneItem>
            </SplitPane>
          </SplitPaneItem>
        </SplitPane>
      </SplitPaneItem>

      <!-- Right column: watchlist on top, orderbook below -->
      <SplitPaneItem :size="28" :min-size="15">
        <SplitPane horizontal :pane-min-size="10">
          <SplitPaneItem :size="60" :min-size="12">
            <WatchlistPanel />
          </SplitPaneItem>
          <SplitPaneItem :size="40" :min-size="15">
            <OrderBookPanel />
          </SplitPaneItem>
        </SplitPane>
      </SplitPaneItem>
    </SplitPane>
  </div>
</template>
