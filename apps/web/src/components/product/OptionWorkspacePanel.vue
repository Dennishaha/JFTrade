<script setup lang="ts">
import AsyncPanelState from "@/components/shared/AsyncPanelState.vue";
import OptionChainTable from "./OptionChainTable.vue";
import OptionContractAnalysisDrawer from "./OptionContractAnalysisDrawer.vue";
import OptionResearchPanel from "./OptionResearchPanel.vue";
import ProductFeaturePanel from "./ProductFeaturePanel.vue";
import ProductPanelToolbar from "./ProductPanelToolbar.vue";
import {
  useOptionWorkspace,
  type OptionWorkspaceProps,
} from "@/composables/product/useOptionWorkspace";

const props = withDefaults(defineProps<OptionWorkspaceProps>(), {
  displayInstrumentId: "",
  underlyingPending: false,
  underlyingProductClass: "equity",
});
const emit = defineEmits<{ openInstrument: [instrumentId: string] }>();

const {
  section,
  expirationLoading,
  chainLoading,
  expirationError,
  chainError,
  snapshotError,
  expirationResult,
  chainsByExpiry,
  snapshots,
  selectedExpiry,
  showAllExpirations,
  strikeRange,
  chainPage,
  rowsPerPage,
  primaryExpiryLimit,
  chainRequests,
  snapshotPolling,
  analysisOperation,
  eventOperation,
  strategyType,
  selectedContract,
  comboDraft,
  sectionItems,
  eventItems,
  normalizedUnderlying,
  needsUnderlying,
  underlyingResolved,
  loading,
  expirations,
  primaryExpirations,
  remainingExpirations,
  furthestExpiry,
  nextExpiry,
  activeChain,
  optionRows,
  underlyingPrice,
  chainRows,
  rangedChainRows,
  chainPageCount,
  visibleChainRows,
  visibleOptionRows,
  atmStrike,
  comboContracts,
  snapshotDependencyKey,
  encodedInstrument,
  featureRequest,
  featurePath,
  snapshotForInstrument,
  selectExpiry,
  toggleAllExpirations,
  formatExpiry,
  openContract,
  selectComboLeg,
  nextExpiryAfter,
  requestExpiryChain,
  prefetchNextExpiry,
  loadSelectedChain,
  loadExpirationCatalog,
  loadVisibleSnapshots,
  selectedBrokerId,
  formatOptionMetric,
  productCompactMenuProps,
} = useOptionWorkspace(props);
</script>

<template>
  <section class="option-workspace">
    <ProductPanelToolbar title="期权工作台">
      <div class="option-workspace__stats">
        <span
          >标的价
          <strong>{{ formatOptionMetric(underlyingPrice) }}</strong></span
        >
        <span
          >到期 <strong>{{ expirations.length || "—" }}</strong></span
        >
        <span
          >合约 <strong>{{ optionRows.length || "—" }}</strong></span
        >
        <span
          >ATM <strong>{{ formatOptionMetric(atmStrike) }}</strong></span
        >
      </div>
    </ProductPanelToolbar>

    <nav class="option-workspace__sections" aria-label="期权工作台视图">
      <v-btn-toggle
        v-model="section"
        class="product-segmented-control tv-scrollbar"
        mandatory
        density="compact"
      >
        <v-btn
          v-for="item in sectionItems"
          :key="item.value"
          :value="item.value"
        >
          <span>{{ item.label }}</span>
        </v-btn>
      </v-btn-toggle>

      <v-select
        v-if="section === 'analysis'"
        v-model="analysisOperation"
        class="product-compact-control"
        :items="[
          { title: '标的概览', value: 'underlying_overview' },
          { title: 'Put / Call 与市场统计', value: 'market_statistics' },
          { title: '历史统计', value: 'historical_statistics' },
          { title: '历史波动率', value: 'historical_volatility' },
        ]"
        :menu-props="productCompactMenuProps"
        density="compact"
        variant="outlined"
        hide-details
        label="分析"
      />
      <v-select
        v-else-if="section === 'events'"
        v-model="eventOperation"
        class="product-compact-control"
        :items="eventItems"
        :menu-props="productCompactMenuProps"
        density="compact"
        variant="outlined"
        hide-details
        label="事件"
      />
      <v-select
        v-else-if="section === 'strategy'"
        v-model="strategyType"
        class="product-compact-control"
        :items="[
          { title: '跨式', value: '1' },
          { title: '宽跨', value: '2' },
          { title: '垂直价差', value: '3' },
          { title: '日历价差', value: '4' },
          { title: '蝶式', value: '5' },
        ]"
        :menu-props="productCompactMenuProps"
        density="compact"
        variant="outlined"
        hide-details
        label="策略"
      />
    </nav>

    <AsyncPanelState
      :loading="loading"
      progress-class="option-workspace__chain-progress"
    >
      <v-alert
        v-if="section === 'chain' && expirationError"
        type="warning"
        variant="tonal"
        density="compact"
      >
        {{ expirationError }}
      </v-alert>
      <v-alert
        v-if="section === 'chain' && chainError"
        type="warning"
        variant="tonal"
        density="compact"
      >
        {{ chainError }}
      </v-alert>
      <v-alert
        v-if="section === 'chain' && snapshotError"
        type="warning"
        variant="tonal"
        density="compact"
      >
        {{ snapshotError }}
      </v-alert>
    </AsyncPanelState>

    <div
      v-if="needsUnderlying && !underlyingResolved"
      class="option-workspace__resolution"
    >
      <span class="option-workspace__resolution-icon">⌁</span>
      <strong>
        {{
          underlyingPending
            ? "正在识别当前期权合约的正股标的"
            : "当前产品没有可用的期权标的"
        }}
      </strong>
      <p>未解析成功前不会使用当前合约代码查询期权链或期权分析。</p>
    </div>

    <div
      v-else-if="section === 'chain'"
      class="option-workspace__chain tv-scrollbar"
    >
      <div class="option-workspace__expiry-bar">
        <div class="option-workspace__expiry-list">
          <button
            v-for="expiry in primaryExpirations"
            :key="expiry.date"
            type="button"
            :class="{ 'is-active': expiry.date === selectedExpiry }"
            @click="selectExpiry(expiry.date)"
          >
            <strong>{{ formatExpiry(expiry.date) }}</strong>
            <span>{{ expiry.daysToExpiry }}天{{ expiry.cycleLabel ? ` · ${expiry.cycleLabel}` : "" }}</span>
          </button>
        </div>
        <button
          v-if="remainingExpirations.length > 0"
          type="button"
          class="option-workspace__expiry-expand"
          :class="{ 'is-expanded': showAllExpirations }"
          :aria-expanded="showAllExpirations"
          :aria-label="
            showAllExpirations ? '收起全部到期日' : '展开全部到期日'
          "
          @click="toggleAllExpirations"
        >
          <span class="fa-solid fa-chevron-down" aria-hidden="true" />
        </button>
      </div>
      <div
        v-if="showAllExpirations && remainingExpirations.length > 0"
        class="option-workspace__expiry-more tv-scrollbar"
        role="group"
        aria-label="其余全部到期日"
      >
        <button
          v-for="expiry in remainingExpirations"
          :key="expiry.date"
          type="button"
          :class="{ 'is-active': expiry.date === selectedExpiry }"
          @click="selectExpiry(expiry.date)"
        >
          <strong>{{ formatExpiry(expiry.date) }}</strong>
          <span>{{ expiry.daysToExpiry }}天{{ expiry.cycleLabel ? ` · ${expiry.cycleLabel}` : "" }}</span>
        </button>
      </div>

      <div class="option-workspace__filters">
        <div class="option-workspace__expiry-coverage">
          <span>到期日范围：全部未到期</span>
          <strong v-if="furthestExpiry"
            >覆盖至 {{ formatExpiry(furthestExpiry) }}</strong
          >
        </div>
        <div class="option-workspace__range-toggle">
          <button
            type="button"
            :class="{ 'is-active': strikeRange === 'all' }"
            @click="strikeRange = 'all'"
          >
            全部行权价
          </button>
          <button
            type="button"
            :class="{ 'is-active': strikeRange === 'near_atm' }"
            @click="strikeRange = 'near_atm'"
          >
            ATM 附近
          </button>
        </div>
      </div>

      <div class="option-workspace__table tv-scrollbar">
        <OptionChainTable
          :rows="visibleChainRows"
          :underlying-instrument-id="normalizedUnderlying"
          :underlying-price="underlyingPrice"
          :selected-legs="comboDraft.legs.value"
          @open-contract="openContract"
          @select-leg="selectComboLeg"
        />
      </div>
      <v-pagination
        v-if="chainPageCount > 1"
        v-model="chainPage"
        :length="chainPageCount"
        :total-visible="7"
        density="compact"
      />
      <div
        v-if="expirations.length === 0 && !loading && !expirationError"
        class="option-workspace__empty"
      >
        当前标的暂无未到期期权合约。
      </div>
      <div
        v-else-if="activeChain != null && optionRows.length === 0 && !loading && !chainError"
        class="option-workspace__empty"
      >
        该到期日暂无期权合约。
      </div>
    </div>

    <OptionResearchPanel
      v-else-if="section === 'events'"
      :market="market"
      :operation="
        eventOperation as 'unusual' | 'zero_dte' | 'earnings' | 'seller'
      "
      scope="underlying"
      :underlying-instrument-id="normalizedUnderlying"
      :underlying-product-class="underlyingProductClass"
      @open-instrument="
        (instrumentId) => emit('openInstrument', instrumentId)
      "
    />

    <ProductFeaturePanel
      v-else
      :key="JSON.stringify(featureRequest)"
      :title="section === 'strategy' ? '合法价差与策略' : '期权研究'"
      :request="featureRequest"
      :active="featureRequest != null"
      @open-instrument="emit('openInstrument', $event)"
    />

    <OptionContractAnalysisDrawer
      :contract="selectedContract"
      :market="market"
      @close="selectedContract = null"
      @open-workspace="emit('openInstrument', $event)"
    />
  </section>
</template>

<style scoped src="./optionWorkspace.css"></style>
