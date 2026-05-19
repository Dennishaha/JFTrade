<script setup lang="ts">
import LightweightChart from "../components/workspace/LightweightChart.vue";
import OrderEntryPanel from "../components/workspace/OrderEntryPanel.vue";
import PositionsPanel from "../components/workspace/PositionsPanel.vue";
import WatchlistPanel from "../components/workspace/WatchlistPanel.vue";
</script>

<template>
  <div class="tv-workspace">
    <LightweightChart />
    <WatchlistPanel />
    <PositionsPanel />
    <OrderEntryPanel />
  </div>
</template>
