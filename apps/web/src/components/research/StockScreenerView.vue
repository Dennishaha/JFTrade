<script setup lang="ts">
import SplitPane from "../shared/SplitPane.vue";
import SplitPaneItem from "../shared/SplitPaneItem.vue";
import StockScreenerBuilder from "./StockScreenerBuilder.vue";
import StockScreenerDialogs from "./StockScreenerDialogs.vue";
import StockScreenerPresetSidebar from "./StockScreenerPresetSidebar.vue";
import StockScreenerResults from "./StockScreenerResults.vue";
import StockScreenerToolbar from "./StockScreenerToolbar.vue";
import type { StockScreenEntry } from "./stockScreenTypes";
import {
  provideStockScreenerController,
  useStockScreenerController,
} from "./useStockScreenerController";

const props = withDefaults(
  defineProps<{
    market: string;
    brokerId?: string;
    initialPresetId?: string;
    active?: boolean;
  }>(),
  {
    brokerId: "",
    initialPresetId: "",
    active: true,
  },
);

const emit = defineEmits<{
  select: [entry: StockScreenEntry];
  open: [entry: StockScreenEntry];
  presetChange: [presetId: string];
  contextChange: [context: { market: string; brokerId?: string }];
}>();

const controller = useStockScreenerController(props, emit);
provideStockScreenerController(controller);
defineExpose(controller);

const {
  screenerOuterPaneSizes,
  screenerInnerPaneSizes,
  screenerOuterPaneMinSizes,
  screenerInnerPaneMinSizes,
  handleScreenerOuterPaneResized,
  handleScreenerInnerPaneResized,
} = controller;
</script>

<template>
  <section class="stock-screener-view">
    <StockScreenerToolbar />

    <SplitPane
      class="stock-screener-view__workspace"
      :pane-min-size="10"
      :push-other-panes="false"
      @resized="handleScreenerOuterPaneResized"
    >
      <SplitPaneItem
        :size="screenerOuterPaneSizes[0]"
        :min-size="screenerOuterPaneMinSizes[0]"
        :max-size="30"
      >
        <StockScreenerPresetSidebar />
      </SplitPaneItem>

      <SplitPaneItem
        :size="screenerOuterPaneSizes[1]"
        :min-size="screenerOuterPaneMinSizes[1]"
        :max-size="88"
      >
        <SplitPane
          class="stock-screener-view__layout"
          :pane-min-size="20"
          :push-other-panes="false"
          @resized="handleScreenerInnerPaneResized"
        >
          <SplitPaneItem
            :size="screenerInnerPaneSizes[0]"
            :min-size="screenerInnerPaneMinSizes[0]"
            :max-size="55"
          >
            <StockScreenerBuilder />
          </SplitPaneItem>

          <SplitPaneItem
            :size="screenerInnerPaneSizes[1]"
            :min-size="screenerInnerPaneMinSizes[1]"
            :max-size="72"
          >
            <StockScreenerResults />
          </SplitPaneItem>
        </SplitPane>
      </SplitPaneItem>
    </SplitPane>

    <StockScreenerDialogs />
  </section>
</template>

<style scoped>
.stock-screener-view {
  box-sizing: border-box;
  display: grid;
  width: 100%;
  max-width: 100%;
  container-type: inline-size;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  gap: 8px;
  padding: 8px;
  color: var(--tv-text);
  font-size: 12px;
}

.stock-screener-view :deep(button),
.stock-screener-view :deep(input),
.stock-screener-view :deep(select) {
  min-height: 28px;
  border: 1px solid var(--tv-border);
  border-radius: 4px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font: inherit;
}

.stock-screener-view :deep(button) {
  padding: 0 8px;
  cursor: pointer;
}

.stock-screener-view :deep(button:hover:not(:disabled)) {
  border-color: var(--tv-accent);
  background: var(--tv-bg-elevated);
}

.stock-screener-view :deep(button:disabled) {
  cursor: not-allowed;
  opacity: 0.45;
}

.stock-screener-view :deep(input),
.stock-screener-view :deep(select) {
  min-width: 0;
  padding: 0 6px;
}

.stock-screener-view__workspace,
.stock-screener-view__layout {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.stock-screener-view__workspace :deep(.splitpanes__pane) {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.stock-screener-view__workspace :deep(.splitpanes__splitter) {
  z-index: 3;
}

.stock-screener-view :deep(.stock-screener-view__builder),
.stock-screener-view :deep(.stock-screener-view__results) {
  box-sizing: border-box;
  width: 100%;
  max-width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
}

@container (max-width: 880px) {
  .stock-screener-view__workspace,
  .stock-screener-view__layout {
    display: block !important;
    height: auto;
    overflow: visible;
  }

  .stock-screener-view__workspace :deep(.splitpanes__splitter) {
    display: none !important;
  }

  .stock-screener-view__workspace :deep(.splitpanes__pane) {
    display: block;
    width: 100% !important;
    max-width: 100% !important;
    height: auto !important;
    min-width: 0 !important;
    min-height: 0 !important;
    overflow: visible;
    transform: none !important;
  }

  .stock-screener-view__workspace
    :deep(.splitpanes__pane + .splitpanes__pane) {
    margin-top: 8px;
  }
}

@media (max-width: 768px) {
  .stock-screener-view {
    padding: 4px;
    padding-bottom: 56px;
  }

  .stock-screener-view :deep(.is-mobile-hidden) {
    display: none;
  }

  .stock-screener-view__layout
    :deep(.splitpanes__pane:has(> .is-mobile-hidden)) {
    display: none !important;
  }

  .stock-screener-view :deep(.stock-screener-view__builder),
  .stock-screener-view :deep(.stock-screener-view__results) {
    max-height: none;
  }
}
</style>
