<script setup lang="ts">
import type { Ref } from "vue";

import type PineSourceCodePaneType from "@/components/strategy-design/PineSourceCodePane.vue";
import PineSourceCodePane from "@/components/strategy-design/PineSourceCodePane.vue";
import SplitPane from "../shared/SplitPane.vue";
import SplitPaneItem from "../shared/SplitPaneItem.vue";
import StrategyDesignHeader from "./StrategyDesignHeader.vue";
import StrategyDesignInstructionWorkspace from "./StrategyDesignInstructionWorkspace.vue";
import StrategyDesignMetadataPane from "./StrategyDesignMetadataPane.vue";
import { useStrategyDesignContext } from "./strategyDesignContext";
import type { StrategyDisplayMode, StrategyMobileSection } from "./useStrategySidePanelLayout";

interface WorkbenchContext {
  strategyMobileSection: Ref<StrategyMobileSection>;
  strategyDisplayMode: Ref<StrategyDisplayMode>;
  metadataPaneOpen: Ref<boolean>;
  isMediumWorkbench: Ref<boolean>;
  error: Readonly<Ref<string>>;
  errorExpanded: Ref<boolean>;
  sourceEditorRef: Ref<InstanceType<typeof PineSourceCodePaneType> | null>;
  activeScript: Readonly<Ref<string>>;
  useSourceOverride: Ref<boolean>;
  pineDiagnosticMarkers: Readonly<Ref<Array<{
    severity: "error" | "warning" | "info";
    message: string;
    line: number;
    column: number;
    endLine: number;
    endColumn: number;
  }>>>;
  closeMetadataPane: () => void;
  commitSourceChange: (source: string) => void;
}

const context = useStrategyDesignContext<WorkbenchContext>();
</script>

<template>
  <div
    class="strategy-native-page"
    :class="[
      `strategy-native-page--mobile-${context.strategyMobileSection.value}`,
      `strategy-native-page--mode-${context.strategyDisplayMode.value}`,
      context.metadataPaneOpen.value
        ? 'strategy-native-page--metadata-open'
        : 'strategy-native-page--metadata-closed',
      { 'strategy-native-page--medium': context.isMediumWorkbench.value },
    ]"
    data-testid="strategy-design-stage"
  >
    <StrategyDesignHeader />

    <button
      v-if="context.error.value"
      type="button"
      class="strategy-native-banner strategy-native-banner--error"
      :class="{ 'is-expanded': context.errorExpanded.value }"
      :aria-expanded="context.errorExpanded.value"
      :title="context.error.value"
      @click="context.errorExpanded.value = !context.errorExpanded.value"
    >
      <v-icon size="13">fa-solid fa-triangle-exclamation</v-icon>
      <span>{{ context.error.value }}</span>
    </button>

    <button
      v-if="context.isMediumWorkbench.value && context.metadataPaneOpen.value"
      type="button"
      class="strategy-native-drawer-backdrop"
      aria-label="关闭策略信息"
      data-testid="strategy-metadata-backdrop"
      @click="context.closeMetadataPane"
    />

    <SplitPane class="strategy-native-shell" :pane-min-size="18">
      <SplitPaneItem
        :size="context.strategyDisplayMode.value === 'instruction'
          ? 100
          : context.strategyDisplayMode.value === 'split'
            ? (context.isMediumWorkbench.value ? 58 : 65)
            : 22"
        :min-size="context.strategyDisplayMode.value === 'instruction'
          ? 100
          : context.strategyDisplayMode.value === 'code' ? 18 : 32"
        :max-size="context.strategyDisplayMode.value === 'instruction'
          ? 100
          : context.strategyDisplayMode.value === 'code' ? 36 : 78"
      >
        <SplitPane class="strategy-native-instruction" :pane-min-size="16">
          <SplitPaneItem
            :size="context.strategyDisplayMode.value === 'code'
              ? 100
              : context.strategyDisplayMode.value === 'instruction' ? 22 : 34"
            :min-size="context.strategyDisplayMode.value === 'code' ? 100 : 18"
            :max-size="context.strategyDisplayMode.value === 'code' ? 100 : 42"
          >
            <StrategyDesignMetadataPane />
          </SplitPaneItem>

          <SplitPaneItem
            v-if="context.strategyDisplayMode.value !== 'code'"
            :size="context.strategyDisplayMode.value === 'instruction' ? 78 : 66"
            :min-size="38"
            :max-size="78"
          >
            <StrategyDesignInstructionWorkspace />
          </SplitPaneItem>
        </SplitPane>
      </SplitPaneItem>

      <SplitPaneItem
        v-if="context.strategyDisplayMode.value !== 'instruction'"
        :size="context.strategyDisplayMode.value === 'split'
          ? (context.isMediumWorkbench.value ? 42 : 35)
          : 78"
        :min-size="context.strategyDisplayMode.value === 'split' ? 30 : 64"
        :max-size="100"
      >
        <PineSourceCodePane
          :ref="context.sourceEditorRef"
          :model-value="context.activeScript.value"
          :source-editing-enabled="context.useSourceOverride.value"
          :diagnostic-markers="context.pineDiagnosticMarkers.value"
          @update:model-value="context.commitSourceChange"
          @update:source-editing-enabled="context.useSourceOverride.value = $event"
        />
      </SplitPaneItem>
    </SplitPane>
  </div>
</template>

<style scoped>
.strategy-native-page {
  position: relative;
  display: flex;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  overflow: hidden;
  background: var(--tv-bg-app);
  color: var(--tv-text);
}

.strategy-native-page :deep(button) {
  border: 1px solid var(--tv-border);
  border-radius: 5px;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
  font-weight: 700;
}

.strategy-native-page :deep(button:disabled) {
  cursor: not-allowed;
  opacity: 0.45;
}

.strategy-native-shell {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  border: 0;
  border-radius: 0;
  background: var(--tv-bg-app);
}

.strategy-native-shell :deep(.splitpanes__pane),
.strategy-native-instruction :deep(.splitpanes__pane) {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.strategy-native-instruction {
  position: relative;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.strategy-native-banner {
  display: flex;
  width: 100%;
  min-height: 30px;
  flex: 0 0 30px;
  align-items: center;
  gap: 6px;
  overflow: hidden;
  border-width: 0 0 1px;
  border-radius: 0;
  padding: 0 8px;
  text-align: left;
}

.strategy-native-banner--error {
  border-color: color-mix(in srgb, var(--jf-accent-red) 52%, var(--tv-border));
  background: color-mix(in srgb, var(--jf-accent-red) 12%, var(--tv-bg-surface));
  color: color-mix(in srgb, var(--jf-accent-red-text) 72%, var(--tv-text));
}

.strategy-native-banner span {
  min-width: 0;
  overflow: hidden;
  font-size: 0.8rem;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-native-banner.is-expanded {
  height: auto;
  min-height: 30px;
  flex-basis: auto;
  padding-block: 6px;
}

.strategy-native-banner.is-expanded span {
  overflow: visible;
  white-space: normal;
}

.strategy-native-banner:focus-visible {
  outline: 2px solid var(--tv-accent);
  outline-offset: -2px;
}

.strategy-native-drawer-backdrop {
  position: absolute;
  z-index: 20;
  inset: 44px 0 0;
  border: 0;
  border-radius: 0;
  background: rgba(2, 6, 23, 0.38);
  padding: 0;
}

@media (min-width: 1181px) {
  .strategy-native-page--metadata-closed .strategy-native-instruction > :deep(.splitpanes__pane:first-of-type),
  .strategy-native-page--metadata-closed .strategy-native-instruction > :deep(.splitpanes__splitter:first-of-type) {
    display: none;
  }

  .strategy-native-page--metadata-closed .strategy-native-instruction > :deep(.splitpanes__pane:last-of-type),
  .strategy-native-page--mode-code.strategy-native-page--metadata-closed .strategy-native-shell > :deep(.splitpanes__pane:last-of-type) {
    width: 100% !important;
    max-width: 100% !important;
    flex: 0 0 100% !important;
  }

  .strategy-native-page--mode-code.strategy-native-page--metadata-closed .strategy-native-shell > :deep(.splitpanes__pane:first-of-type),
  .strategy-native-page--mode-code.strategy-native-page--metadata-closed .strategy-native-shell > :deep(.splitpanes__splitter:first-of-type) {
    display: none;
  }
}

@media (min-width: 769px) and (max-width: 1180px) {
  .strategy-native-page--mode-instruction .strategy-native-instruction > :deep(.splitpanes__pane:first-of-type),
  .strategy-native-page--mode-split .strategy-native-instruction > :deep(.splitpanes__pane:first-of-type),
  .strategy-native-page--mode-code .strategy-native-shell > :deep(.splitpanes__pane:first-of-type) {
    position: absolute !important;
    z-index: 30;
    inset: 0 auto 0 0;
    width: min(360px, calc(100% - 48px)) !important;
    max-width: min(360px, calc(100% - 48px)) !important;
    min-width: min(280px, calc(100% - 48px)) !important;
    flex: 0 0 min(360px, calc(100% - 48px)) !important;
    transform: translateX(0);
    transition: transform 160ms ease;
    box-shadow: 16px 0 36px rgba(2, 6, 23, 0.3);
  }

  .strategy-native-page--mode-instruction .strategy-native-instruction > :deep(.splitpanes__splitter),
  .strategy-native-page--mode-split .strategy-native-instruction > :deep(.splitpanes__splitter),
  .strategy-native-page--mode-code .strategy-native-shell > :deep(.splitpanes__splitter) {
    display: none;
  }

  .strategy-native-page--mode-instruction .strategy-native-instruction > :deep(.splitpanes__pane:last-of-type),
  .strategy-native-page--mode-split .strategy-native-instruction > :deep(.splitpanes__pane:last-of-type),
  .strategy-native-page--mode-code .strategy-native-shell > :deep(.splitpanes__pane:last-of-type) {
    width: 100% !important;
    max-width: 100% !important;
    flex: 0 0 100% !important;
  }

  .strategy-native-page--metadata-closed.strategy-native-page--mode-instruction .strategy-native-instruction > :deep(.splitpanes__pane:first-of-type),
  .strategy-native-page--metadata-closed.strategy-native-page--mode-split .strategy-native-instruction > :deep(.splitpanes__pane:first-of-type),
  .strategy-native-page--metadata-closed.strategy-native-page--mode-code .strategy-native-shell > :deep(.splitpanes__pane:first-of-type) {
    pointer-events: none;
    transform: translateX(-105%);
    box-shadow: none;
  }
}

@media (max-width: 768px) {
  .strategy-native-page {
    gap: 0;
    padding: 0;
  }

  .strategy-native-shell,
  .strategy-native-instruction {
    display: block !important;
    width: 100%;
    height: 100%;
    min-height: 0;
  }

  .strategy-native-shell :deep(.splitpanes__splitter),
  .strategy-native-instruction :deep(.splitpanes__splitter) {
    display: none !important;
  }

  .strategy-native-shell :deep(.splitpanes__pane),
  .strategy-native-instruction :deep(.splitpanes__pane) {
    width: 100% !important;
    max-width: 100% !important;
    min-width: 0 !important;
    height: 100% !important;
    max-height: 100% !important;
    min-height: 0 !important;
    flex: none !important;
    transform: none !important;
  }

  .strategy-native-page--mobile-definition .strategy-native-shell :deep(.splitpanes__pane:has(.strategy-native-code-pane)),
  .strategy-native-page--mobile-definition .strategy-native-instruction :deep(.splitpanes__pane:has(.strategy-native-main)),
  .strategy-native-page--mobile-instruction .strategy-native-shell :deep(.splitpanes__pane:has(.strategy-native-code-pane)),
  .strategy-native-page--mobile-instruction .strategy-native-instruction :deep(.splitpanes__pane:has(.strategy-native-side)),
  .strategy-native-page--mobile-code .strategy-native-shell :deep(.splitpanes__pane:has(.strategy-native-instruction)) {
    display: none !important;
  }
}
</style>
