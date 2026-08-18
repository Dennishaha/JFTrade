<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

import { useConsoleData } from "@/composables/workspace/useConsoleData";
import { useBrokerProviderSelection } from "@/composables/trading/brokerProviderSelection";
import { getSharedLiveSocketHub } from "@/composables/market-data/sharedLiveSocket";
import { resolveProductUnderlying } from "@/composables/product/productUnderlying";
import { productFeaturePath, type ProductFeatureRequest } from "@/composables/product/productFeatureApi";
import { useWorkspaceTradingPrefs } from "@/composables/workspace/useWorkspaceLayout";
import AppTabs from "@/components/shared/AppTabs.vue";
import {
  isWorkspaceProductTab,
  resolveWorkspaceProductClass,
  workspaceTabsForProduct,
  type WorkspaceProductTab,
} from "@/composables/workspace/workspaceProductTabs";
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
  reloadMarketDataProvider,
} = useConsoleData();
const activeTab = ref<WorkspaceProductTab>("chart");
const providerStatusBarAvailable = ref(false);
const instrumentID = computed(
  () => `${prefs.value.market}.${prefs.value.symbol}`.toUpperCase(),
);
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
const featureRequest = computed<ProductFeatureRequest | null>(() => {
  if (!visibleTabValues.value.has(activeTab.value)) {
    return null;
  }
  switch (activeTab.value) {
    case "options":
      return productUnderlying.value.instrumentId
        ? { scope: "market-feature", resource: "option-chains", instrumentId: productUnderlying.value.instrumentId, pageSize: 50 }
        : null;
    case "warrants":
      return { scope: "market-feature", resource: "warrants", market: prefs.value.market, underlying: instrumentID.value, pageSize: 50 };
    case "news":
      return productUnderlying.value.instrumentId
        ? { scope: "market-feature", resource: "news", market: productUnderlyingMarket.value, code: productUnderlying.value.instrumentId, pageSize: 30 }
        : null;
    case "company":
      return productUnderlying.value.instrumentId
        ? { scope: "research", family: "instrument", instrumentId: productUnderlying.value.instrumentId, pageSize: 50 }
        : null;
    default:
      return null;
  }
});
const featurePath = computed(() =>
  featureRequest.value == null ? "" : productFeaturePath(featureRequest.value),
);

function handleMarketDataProviderChanged(): void {
  // Provider invalidation increments the query generation. Complete the
  // reconciliation with one fresh load so a chart request started by the
  // provider selector cannot be discarded without a replacement request.
  void reloadMarketDataProvider({ load: true });
}

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
      <AppTabs :model-value="activeTab" :items="tabs" class="workspace-product-pane__tabs"
        label="产品工作区视图" @update:model-value="selectTab" />
      <Teleport
        to="#workspace-provider-statusbar"
        :disabled="!providerStatusBarAvailable"
      >
        <BrokerProviderTag
          :feature-id="activeFeatureID"
          :market="prefs.market"
          enable-embedded-market-data-provider
          :connection-state="liveHub.connectionState.value"
          :transport-mode="liveHub.lastHeartbeatEvent.value?.transport?.mode"
          menu-location="top end"
          @provider-changed="handleMarketDataProviderChanged"
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
        :request="featureRequest" @open-instrument="openInstrument" />
      <ProductFeaturePanel v-else :title="tabs.find((item) => item.value === activeTab)?.label ?? ''"
        :request="featureRequest" @open-instrument="openInstrument" />
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

.workspace-product-pane__tabs :deep(.app-tabs__tab) {
  min-width: 68px;
  height: 41px;
  padding: 0 11px;
  color: var(--tv-text-muted);
  font-size: var(--jf-text-5);
  font-weight: 620;
}
</style>
