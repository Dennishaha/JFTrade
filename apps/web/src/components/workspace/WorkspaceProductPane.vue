<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

import { useConsoleData } from "../../composables/useConsoleData";
import { useBrokerProviderSelection } from "../../composables/brokerProviderSelection";
import { getSharedLiveSocketHub } from "../../composables/sharedLiveSocket";
import { resolveProductUnderlying } from "../../composables/productUnderlying";
import { useWorkspaceTradingPrefs } from "../../composables/useWorkspaceLayout";
import {
  isWorkspaceProductTab,
  resolveWorkspaceProductClass,
  workspaceTabsForProduct,
  type WorkspaceProductTab,
} from "../../composables/workspaceProductTabs";
import CompanyWorkspacePanel from "../product/CompanyWorkspacePanel.vue";
import BrokerProviderTag from "../shared/BrokerProviderTag.vue";
import NewsWorkspacePanel from "../product/NewsWorkspacePanel.vue";
import OptionWorkspacePanel from "../product/OptionWorkspacePanel.vue";
import PredictionContractWorkspacePanel from "../product/PredictionContractWorkspacePanel.vue";
import ProductFeaturePanel from "../product/ProductFeaturePanel.vue";
import LightweightChart from "./LightweightChart.vue";

const route = useRoute();
const router = useRouter();
const { prefs, update } = useWorkspaceTradingPrefs();
const { selectedBrokerId } = useBrokerProviderSelection();
const liveHub = getSharedLiveSocketHub();
const providerOwnerID = liveHub.createOwnerId("workspace-provider");
const {
  activeMarketDataInstrumentId,
  currentMarketSecurityDetails,
  isLoadingMarketDataQuery,
  lastDataRefreshedAt,
  marketInstrumentReferences,
  selectedBrokerAccount,
  systemStatus,
} = useConsoleData();
const activeTab = ref<WorkspaceProductTab>("chart");
const providerStatusBarAvailable = ref(false);
const instrumentID = computed(
  () => `${prefs.value.market}.${prefs.value.symbol}`.toUpperCase(),
);
const encodedInstrument = computed(() => encodeURIComponent(instrumentID.value));
const securityDetails = computed(() => {
  const result = currentMarketSecurityDetails.value;
  return result?.request.instrumentId.trim().toUpperCase() === instrumentID.value
    ? result.security
    : null;
});
const referenceSecurityType = computed(() => {
  const reference = marketInstrumentReferences.value.find(
    (entry) =>
      entry.instrumentId.trim().toUpperCase() === instrumentID.value,
  );
  return reference?.securityType ?? null;
});
const isPrediction = computed(
  () =>
    prefs.value.marketSegment === "prediction" ||
    route.query.marketSegment === "prediction",
);
const productClass = computed(() =>
  isPrediction.value
    ? "event_contract"
    : resolveWorkspaceProductClass(
        securityDetails.value,
        referenceSecurityType.value,
      ),
);
const productIdentityPending = computed(
  () =>
    !isPrediction.value &&
    productClass.value === "unknown" &&
    (activeMarketDataInstrumentId.value.trim().toUpperCase() !==
      instrumentID.value ||
      isLoadingMarketDataQuery.value ||
      lastDataRefreshedAt.value === 0),
);
const productUnderlying = computed(() =>
  resolveProductUnderlying(
    instrumentID.value,
    productClass.value,
    securityDetails.value,
    productIdentityPending.value,
  ),
);
const encodedProductUnderlying = computed(() =>
  encodeURIComponent(productUnderlying.value.instrumentId),
);
const productUnderlyingMarket = computed(() => {
  const [market = ""] = productUnderlying.value.instrumentId.split(".", 1);
  return market || prefs.value.market;
});
const tabs = computed(() =>
  workspaceTabsForProduct(prefs.value.market, productClass.value),
);
const visibleTabValues = computed(
  () => new Set(tabs.value.map((tab) => tab.value)),
);
const activeSurfaceID = computed(
  () => tabs.value.find((tab) => tab.value === activeTab.value)?.surfaceId ?? "workspace.root",
);
const activeFeatureID = computed(
  () => tabs.value.find((tab) => tab.value === activeTab.value)?.featureId ?? "market.candles",
);
const featurePath = computed(() => {
  if (!visibleTabValues.value.has(activeTab.value)) {
    return "";
  }
  switch (activeTab.value) {
    case "options":
      return productUnderlying.value.instrumentId
        ? `/api/v1/market-data/options/chains/${encodedProductUnderlying.value}?pageSize=50`
        : "";
    case "warrants":
      return `/api/v1/market-data/warrants?market=${prefs.value.market}&underlying=${encodedInstrument.value}&pageSize=50`;
    case "news":
      return productUnderlying.value.instrumentId
        ? `/api/v1/market-data/news?market=${encodeURIComponent(productUnderlyingMarket.value)}&code=${encodedProductUnderlying.value}&pageSize=30`
        : "";
    case "company":
      return productUnderlying.value.instrumentId
        ? `/api/v1/research/instruments/${encodedProductUnderlying.value}?pageSize=50`
        : "";
    default:
      return "";
  }
});

function replaceRouteTab(value: WorkspaceProductTab): void {
  if (route.query.tab === value) return;
  void router.replace({ query: { ...route.query, tab: value } });
}

function defaultTab(): WorkspaceProductTab {
  return productClass.value === "event_contract" ? "contract" : "chart";
}

function reconcileRouteTab(): void {
  const rawTab = String(route.query.tab ?? "").trim();
  if (rawTab === "") {
    activeTab.value = defaultTab();
    return;
  }
  if (!isWorkspaceProductTab(rawTab)) {
    activeTab.value = defaultTab();
    replaceRouteTab(defaultTab());
    return;
  }
  if (
    productIdentityPending.value &&
    rawTab !== "chart" &&
    rawTab !== "news"
  ) {
    // Render the chart while the canonical product identity is loading. This
    // loads security details without issuing a restricted product request.
    activeTab.value = "chart";
    return;
  }
  if (!visibleTabValues.value.has(rawTab)) {
    activeTab.value = defaultTab();
    replaceRouteTab(defaultTab());
    return;
  }
  activeTab.value = rawTab;
}

watch(
  [
    () => route.query.tab,
    instrumentID,
    productClass,
    productIdentityPending,
    () => tabs.value.map((tab) => tab.value).join(","),
  ],
  reconcileRouteTab,
  { immediate: true },
);

onMounted(async () => {
  await nextTick();
  providerStatusBarAvailable.value =
    document.getElementById("workspace-provider-statusbar") != null;
});

function selectTab(value: unknown): void {
  if (!isWorkspaceProductTab(value) || !visibleTabValues.value.has(value)) {
    return;
  }
  activeTab.value = value;
  replaceRouteTab(value);
}

function openInstrument(value: string): void {
  const [market, ...symbolParts] = value.split(".");
  const symbol = symbolParts.join(".");
  if (!market || !symbol) return;
  const openingOption = activeTab.value === "options";
  update({
    market,
    symbol,
    marketSegment: openingOption ? "derivatives" : "securities",
    productClass: openingOption ? "option" : "unknown",
  });
  activeTab.value = "chart";
  replaceRouteTab("chart");
}

watch(
  () => route.query.marketSegment,
  (value) => {
    if (value !== "prediction" || prefs.value.marketSegment === "prediction") {
      return;
    }
    update({ marketSegment: "prediction", productClass: "event_contract" });
  },
  { immediate: true },
);

watch(
  selectedBrokerId,
  (brokerId) => {
    liveHub.setProviderBrokerId(providerOwnerID, brokerId || null);
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  liveHub.setProviderBrokerId(providerOwnerID, null);
});
</script>

<template>
  <div class="workspace-product-pane">
    <div class="workspace-product-pane__navigation">
      <v-tabs :model-value="activeTab" density="compact" class="workspace-product-pane__tabs"
        @update:model-value="selectTab">
        <v-tab v-for="tab in tabs" :key="tab.value" :value="tab.value" :data-capability-surface="tab.surfaceId">
          {{ tab.label }}
        </v-tab>
      </v-tabs>
      <Teleport
        to="#workspace-provider-statusbar"
        :disabled="!providerStatusBarAvailable"
      >
        <BrokerProviderTag
          :feature-id="activeFeatureID"
          :market="prefs.market"
          :preferred-broker-id="selectedBrokerAccount?.brokerId"
          :default-broker-id="systemStatus.defaultBroker"
          :connection-state="liveHub.connectionState.value"
          :transport-mode="liveHub.lastHeartbeatEvent.value?.transport?.mode"
          menu-location="top end"
        />
      </Teleport>
    </div>
    <div class="workspace-product-pane__content" :data-capability-surface="activeSurfaceID">
      <PredictionContractWorkspacePanel
        v-if="productClass === 'event_contract'"
        :instrument-id="instrumentID"
        :view="activeTab === 'options' || activeTab === 'warrants' || activeTab === 'news' || activeTab === 'company' ? 'contract' : activeTab"
      />
      <LightweightChart v-else-if="activeTab === 'chart'" />
      <OptionWorkspacePanel v-else-if="activeTab === 'options'" :instrument-id="productUnderlying.instrumentId"
        :display-instrument-id="instrumentID" :underlying-pending="productUnderlying.pending" :market="prefs.market"
        :underlying-product-class="productClass === 'index' ? 'index' : 'equity'"
        @open-instrument="openInstrument" />
      <CompanyWorkspacePanel v-else-if="activeTab === 'company'" :instrument-id="productUnderlying.instrumentId"
        :market="productUnderlyingMarket" @open-instrument="openInstrument" />
      <NewsWorkspacePanel v-else-if="activeTab === 'news'" :instrument-id="productUnderlying.instrumentId"
        :path="featurePath" @open-instrument="openInstrument" />
      <ProductFeaturePanel v-else :title="tabs.find((item) => item.value === activeTab)?.label ?? ''"
        :path="featurePath" @open-instrument="openInstrument" />
    </div>
  </div>
</template>

<style scoped>
.workspace-product-pane {
  display: flex;
  min-height: 0;
  height: 100%;
  flex-direction: column;
  overflow: hidden;
  background: var(--tv-bg-surface);
  color: var(--tv-text);
}

.workspace-product-pane__navigation {
  display: flex;
  min-height: 42px;
  flex: 0 0 auto;
  align-items: center;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.workspace-product-pane__tabs {
  min-width: 0;
  flex: 1;
}

.workspace-product-pane__content {
  min-height: 0;
  flex: 1;
  overflow: hidden;
}

.workspace-product-pane__tabs :deep(.v-slide-group__content) {
  gap: 2px;
  padding: 0 8px;
}

.workspace-product-pane__tabs :deep(.v-tab) {
  min-width: 68px;
  height: 41px;
  padding: 0 11px;
  color: var(--tv-text-muted);
  font-size: 11px;
  font-weight: 620;
  letter-spacing: 0;
  text-transform: none;
}

.workspace-product-pane__tabs :deep(.v-tab:hover) {
  color: var(--tv-text);
}

.workspace-product-pane__tabs :deep(.v-tab--selected) {
  color: var(--tv-text);
}

.workspace-product-pane__tabs :deep(.v-tab__slider) {
  height: 2px;
  background: var(--tv-accent);
}
</style>
