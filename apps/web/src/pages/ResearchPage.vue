<script setup lang="ts">
import OptionResearchPanel from "../components/product/OptionResearchPanel.vue";
import PredictionResearchPanel from "../components/product/PredictionResearchPanel.vue";
import ArkResearchView from "../components/research/ArkResearchView.vue";
import ConceptSectorView from "../components/research/ConceptSectorView.vue";
import DerivativeScreenView from "../components/research/DerivativeScreenView.vue";
import DividendCalendarView from "../components/research/DividendCalendarView.vue";
import EarningsCalendarView from "../components/research/EarningsCalendarView.vue";
import EconCalendarView from "../components/research/EconCalendarView.vue";
import IndustryChainView from "../components/research/IndustryChainView.vue";
import InstrumentResearchView from "../components/research/InstrumentResearchView.vue";
import InstitutionGridView from "../components/research/InstitutionGridView.vue";
import IpoCenterView from "../components/research/IpoCenterView.vue";
import MacroResearchView from "../components/research/MacroResearchView.vue";
import MarketHomeView from "../components/research/MarketHomeView.vue";
import MarketRankingsView from "../components/research/MarketRankingsView.vue";
import QuoteDetailRail from "../components/research/QuoteDetailRail.vue";
import StockScreenerView from "../components/research/StockScreenerView.vue";
import BrokerProviderTag from "../components/shared/BrokerProviderTag.vue";
import SplitPane from "../components/shared/SplitPane.vue";
import SplitPaneItem from "../components/shared/SplitPaneItem.vue";
import { useResearchPageController } from "./useResearchPageController";

const {
  sections,
  MARKET_VIEWS,
  MARKET_CODE_OPTIONS,
  selectedBrokerAccount,
  systemStatus,
  selectedBrokerId,
  configFor,
  operationFor,
  firstQueryValue,
  quotePeriodFromQuery,
  predictionContractViewFromQuery,
  activeSection,
  activeConfig,
  activeOperation,
  activeMarketView,
  activeMarketCode,
  workspaceInstrumentId,
  activeInstrumentId,
  activeIndicatorId,
  activeChainId,
  activePlateId,
  activePresetId,
  activeInstitutionId,
  activePredictionSeriesCode,
  activePredictionEventCode,
  activePredictionContractCode,
  activePredictionContractView,
  activeScreenMarket,
  selectedQuoteTarget,
  selectedQuoteSeed,
  selectedQuotePeriod,
  selectedQuoteTab,
  marketRailCollapsed,
  marketRailDrawer,
  marketPaneSizes,
  researchPageRef,
  researchPageWidth,
  researchPaneBounds,
  rankingInitialOperation,
  syncRailMode,
  handleRailMediaChange,
  syncResearchPageWidth,
  queryWith,
  quoteQuery,
  clearQuoteSelection,
  persistResearchView,
  normalizeInvalidResearchRoute,
  selectSection,
  selectOperation,
  selectMarket,
  updateResearchContext,
  selectIndicatorContext,
  selectIndustryChain,
  selectIndustryPlate,
  selectResearchInstrument,
  selectScreenPreset,
  selectInstitutionContext,
  selectPredictionSeries,
  selectPredictionEvent,
  selectPredictionContract,
  selectPredictionContractView,
  selectScreenContext,
  queryMarket,
  sectionMarketOptions,
  showSectionMarketSwitch,
  visibleOperations,
  handleMarketPaneResized,
  optionResearchOperation,
  macroResearchOperation,
  arkResearchOperation,
  derivativeScreenOperation,
  instrumentResearchOperation,
  activeFeatureIDs,
  handleMarketSelect,
  selectRailTarget,
  selectQuotePeriod,
  selectQuoteTab,
  handleMarketMore,
  openWorkspaceInstrument,
  openQuoteTargetInWorkspace,
  openOptionResearchInstrument,
  researchEntry,
  selectResearchEntry,
  openResearchEntry,
} = useResearchPageController();
</script>

<template>
  <div ref="researchPageRef" class="research-page" :data-capability-surface="activeConfig.surfaceId">
    <SplitPane class="research-page__shell" :class="{
      'is-drawer': marketRailDrawer,
      'is-rail-collapsed': marketRailCollapsed,
    }" :pane-min-size="8" @resized="handleMarketPaneResized">
      <SplitPaneItem :size="marketRailCollapsed ? 100 : marketPaneSizes[0]"
        :min-size="marketRailCollapsed ? 100 : researchPaneBounds.leftMinSize"
        :max-size="marketRailCollapsed ? 100 : researchPaneBounds.leftMaxSize">
        <section class="research-page__center">
          <div class="research-page__navigation">
            <v-tabs class="research-page__tabs" :model-value="activeSection" density="compact" show-arrows
              @update:model-value="selectSection">
              <v-tab v-for="section in sections" :key="section.value" :value="section.value"
                :data-capability-surface="section.surfaceId">
                {{ section.label }}
              </v-tab>
            </v-tabs>
            <div class="research-page__navigation-actions">
              <BrokerProviderTag :feature-id="activeFeatureIDs[0]" :feature-ids="activeFeatureIDs" :market="queryMarket"
                :preferred-broker-id="selectedBrokerAccount?.brokerId"
                :default-broker-id="systemStatus.defaultBroker" />
              <button
                type="button"
                class="research-page__rail-toggle"
                :title="marketRailCollapsed ? '展开行情详情' : '收起行情详情'"
                :aria-label="marketRailCollapsed ? '展开行情详情' : '收起行情详情'"
                @click="marketRailCollapsed = !marketRailCollapsed"
              >
                <svg
                  class="research-page__rail-toggle-icon"
                  viewBox="0 0 20 20"
                  :data-direction="marketRailCollapsed ? 'left' : 'right'"
                  aria-hidden="true"
                >
                  <path
                    :d="
                      marketRailCollapsed
                        ? 'm12.5 4.5-5.5 5.5 5.5 5.5'
                        : 'm7.5 4.5 5.5 5.5-5.5 5.5'
                    "
                  />
                </svg>
              </button>
            </div>
          </div>

          <div v-if="activeSection === 'market'" class="research-page__market-nav">
            <span class="tv-seg research-page__market-views">
              <button v-for="view in MARKET_VIEWS" :key="view.value" type="button"
                :class="{ 'is-active': activeMarketView === view.value }" @click="activeMarketView = view.value">
                {{ view.label }}
              </button>
            </span>
            <span class="tv-seg research-page__market-codes">
              <button v-for="option in MARKET_CODE_OPTIONS" :key="option.value" type="button"
                :class="{ 'is-active': activeMarketCode === option.value }" @click="selectMarket(option.value)">
                {{ option.label }}
              </button>
            </span>
          </div>
          <div v-else class="research-page__section-nav">
            <span class="tv-seg research-page__section-operations">
              <button v-for="operation in visibleOperations" :key="operation.value" type="button"
                :class="{ 'is-active': activeOperation === operation.value }" @click="selectOperation(operation.value)">
                {{ operation.label }}
              </button>
            </span>
            <span class="research-page__section-spacer" />
            <span v-if="showSectionMarketSwitch" class="tv-seg research-page__section-markets">
              <button v-for="option in sectionMarketOptions" :key="option.value" type="button"
                :class="{ 'is-active': activeMarketCode === option.value }" @click="selectMarket(option.value)">{{
                option.label }}</button>
            </span>
          </div>

          <main class="research-page__body">
            <div class="research-page__content">
              <MarketHomeView v-if="activeSection === 'market' && activeMarketView === 'home'"
                :market="activeMarketCode" :broker-id="selectedBrokerId" @select="handleMarketSelect"
                @more="handleMarketMore" />
              <MarketRankingsView v-else-if="activeSection === 'market' && activeMarketView === 'rankings'"
                :market="activeMarketCode" :broker-id="selectedBrokerId" :initial-operation="rankingInitialOperation"
                @select="handleMarketSelect" />
              <ConceptSectorView v-else-if="activeSection === 'market'" :market="activeMarketCode"
                :broker-id="selectedBrokerId" @select="handleMarketSelect" />
              <StockScreenerView v-else-if="activeSection === 'screens'" :market="activeScreenMarket"
                :broker-id="selectedBrokerId" :initial-preset-id="activePresetId" @select="selectResearchEntry"
                @open="openResearchEntry" @preset-change="selectScreenPreset" @context-change="selectScreenContext" />
              <EarningsCalendarView v-else-if="activeSection === 'calendar' && activeOperation === 'earnings'"
                :market="queryMarket" :broker-id="selectedBrokerId" @select="handleMarketSelect" />
              <EconCalendarView v-else-if="activeSection === 'calendar' && activeOperation === 'economic'"
                :market="queryMarket" :broker-id="selectedBrokerId" />
              <IpoCenterView v-else-if="activeSection === 'calendar' && activeOperation === 'ipos'"
                :market="queryMarket" :broker-id="selectedBrokerId" @select="handleMarketSelect" />
              <DividendCalendarView v-else-if="activeSection === 'calendar' && activeOperation === 'dividends'"
                :market="queryMarket" :broker-id="selectedBrokerId" @select="selectResearchEntry"
                @open="openResearchEntry" />
              <MacroResearchView v-else-if="activeSection === 'macro'" :broker-id="selectedBrokerId"
                :operation="macroResearchOperation" :indicator-id="activeIndicatorId"
                @update:indicator-id="selectIndicatorContext" />
              <InstitutionGridView
                v-else-if="activeSection === 'institutions' && ['list', 'holding_changes'].includes(activeOperation)"
                :market="queryMarket" :broker-id="selectedBrokerId"
                :operation="activeOperation === 'holding_changes' ? 'holding_changes' : 'list'"
                :institution-id="activeInstitutionId" @update:institution-id="selectInstitutionContext"
                @select="handleMarketSelect" />
              <ArkResearchView v-else-if="activeSection === 'institutions'" :market="queryMarket"
                :broker-id="selectedBrokerId" :operation="arkResearchOperation" @select="selectResearchEntry"
                @open="openResearchEntry" />
              <IndustryChainView v-else-if="activeSection === 'industries'" :market="queryMarket"
                :broker-id="selectedBrokerId" :chain-id="activeChainId" :plate-id="activePlateId"
                @update:chain-id="selectIndustryChain" @update:plate-id="selectIndustryPlate"
                @select="selectResearchEntry" @open="openResearchEntry" />
              <InstrumentResearchView v-else-if="activeSection === 'instrument'" :instrument-id="activeInstrumentId"
                :broker-id="selectedBrokerId" :operation="instrumentResearchOperation"
                @update:instrument-id="selectResearchInstrument" @select="selectResearchEntry"
                @open="openResearchEntry" />
              <PredictionResearchPanel v-else-if="activeSection === 'prediction'" presentation="research"
                :series-code="activePredictionSeriesCode" :event-code="activePredictionEventCode"
                :contract-code="activePredictionContractCode" :contract-view="activePredictionContractView"
                @update:series-code="selectPredictionSeries" @update:event-code="selectPredictionEvent"
                @update:contract-code="selectPredictionContract" @update:contract-view="selectPredictionContractView"
                @open-instrument="openWorkspaceInstrument" />
              <OptionResearchPanel
                v-else-if="activeSection === 'derivatives' && !['option_screen', 'warrant'].includes(activeOperation)"
                market="US" :operation="optionResearchOperation" scope="market" presentation="research"
                @open-instrument="openOptionResearchInstrument" />
              <DerivativeScreenView v-else-if="activeSection === 'derivatives'" :operation="derivativeScreenOperation"
                :broker-id="selectedBrokerId" @select="selectResearchEntry" @open="
                  openResearchEntry(
                    $event,
                    derivativeScreenOperation === 'option_screen'
                      ? 'option'
                      : 'unknown',
                  )
                  " />
              <div v-else class="research-page__empty">
                当前研究视图尚不可用
              </div>
            </div>
          </main>
        </section>
        <button v-if="marketRailDrawer && !marketRailCollapsed" type="button" class="research-page__rail-backdrop"
          aria-label="关闭行情详情" @click="marketRailCollapsed = true" />
      </SplitPaneItem>
      <SplitPaneItem v-if="!marketRailCollapsed" :size="marketPaneSizes[1]" :min-size="researchPaneBounds.railMinSize"
        :max-size="researchPaneBounds.railMaxSize">
        <aside class="research-page__market-rail">
          <QuoteDetailRail :target="selectedQuoteTarget" :seed="selectedQuoteSeed" :broker-id="selectedBrokerId"
            :visible="!marketRailCollapsed" :drawer="marketRailDrawer" :period="selectedQuotePeriod"
            :tab="selectedQuoteTab" @update:period="selectQuotePeriod" @update:tab="selectQuoteTab"
            @select="selectRailTarget" @open-workspace="openQuoteTargetInWorkspace"
            @close="marketRailCollapsed = true" />
        </aside>
      </SplitPaneItem>
    </SplitPane>
  </div>
</template>

<style scoped>
.research-page {
  min-height: 0;
  height: 100%;
  overflow: hidden;
  background: var(--tv-bg-app);
  color: var(--tv-text);
}

.research-page__shell {
  position: relative;
  min-height: 0;
  height: 100%;
}

.research-page__shell :deep(.splitpanes__pane) {
  position: relative;
  min-width: 0;
  min-height: 0;
  overflow: visible;
}

.research-page__shell :deep(.splitpanes__splitter) {
  z-index: 4;
}

.research-page__center {
  display: flex;
  min-width: 0;
  min-height: 0;
  height: 100%;
  flex-direction: column;
  overflow: hidden;
}

.research-page__navigation {
  display: flex;
  min-width: 0;
  min-height: 43px;
  flex: 0 0 auto;
  align-items: center;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.research-page__tabs {
  min-width: 0;
  flex: 1 1 auto;
}

.research-page__navigation :deep(.v-tab) {
  min-width: 76px;
  height: 42px;
  color: var(--tv-text-muted);
  font-size: 12px;
  font-weight: 650;
  letter-spacing: 0;
  text-transform: none;
}

.research-page__navigation :deep(.v-tab--selected) {
  color: var(--tv-text);
}

.research-page__navigation :deep(.v-tab__slider) {
  height: 2px;
  background: var(--tv-accent);
}

.research-page__navigation-actions {
  display: flex;
  min-width: max-content;
  height: 42px;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  padding: 0 8px 0 6px;
}

.research-page__section-nav {
  display: flex;
  min-height: 44px;
  flex: 0 0 auto;
  align-items: center;
  gap: 12px;
  padding: 6px 16px;
  overflow-x: auto;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.research-page__section-operations {
  flex: 0 0 auto;
  white-space: nowrap;
}

.research-page__section-spacer {
  flex: 1;
}

.research-page__body {
  min-height: 0;
  flex: 1;
  padding: 8px;
  overflow: hidden;
}

/* ---- 市场 section：二级导航 + 视图区 + 右侧行情详情栏 ---- */
.research-page__market-nav {
  display: flex;
  min-height: 44px;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 16px;
  overflow-x: auto;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.research-page__content {
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
}

.research-page__empty {
  display: grid;
  min-height: 160px;
  place-items: center;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface);
  color: var(--tv-text-dim);
  font-size: 12px;
}

.research-page__market-rail {
  position: relative;
  display: flex;
  width: 100%;
  height: 100%;
  min-height: 0;
  flex-direction: column;
}

.research-page__market-rail :deep(.quote-detail-rail) {
  width: 100%;
  max-width: none;
}

.research-page__rail-backdrop {
  display: none;
}

.research-page__rail-toggle {
  display: grid;
  width: 28px;
  height: 32px;
  flex: 0 0 auto;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--tv-text-muted);
  cursor: pointer;
  line-height: 1;
}

.research-page__rail-toggle-icon {
  width: 16px;
  height: 16px;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 1.8;
}

.research-page__rail-toggle:hover {
  background: var(--tv-bg-hover);
  color: var(--tv-text);
}

.research-page__rail-toggle:focus-visible {
  outline: 2px solid var(--tv-accent);
  outline-offset: -2px;
}

.research-page__shell.is-drawer {
  display: block !important;
}

.research-page__shell.is-drawer :deep(.splitpanes__splitter) {
  display: none;
}

.research-page__shell.is-drawer> :deep(.splitpanes__pane:first-child) {
  width: 100% !important;
}

.research-page__shell.is-drawer:not(.is-rail-collapsed)> :deep(.splitpanes__pane:last-child) {
  position: absolute;
  z-index: 42;
  top: 0;
  right: 0;
  bottom: 0;
  width: min(520px, calc(100% - 32px)) !important;
  height: auto;
  background: var(--tv-bg-app);
  box-shadow: -12px 0 28px rgb(0 0 0 / 32%);
}

.research-page__shell.is-drawer .research-page__rail-backdrop {
  position: absolute;
  z-index: 41;
  inset: 0;
  display: block;
  border: 0;
  background: rgb(0 0 0 / 42%);
}
</style>
