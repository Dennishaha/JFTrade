<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

import ProductFeaturePanel from "../components/product/ProductFeaturePanel.vue";
import OptionResearchPanel from "../components/product/OptionResearchPanel.vue";
import ProductPanelToolbar from "../components/product/ProductPanelToolbar.vue";
import PredictionResearchPanel from "../components/product/PredictionResearchPanel.vue";
import BrokerProviderTag from "../components/shared/BrokerProviderTag.vue";
import { productCompactMenuProps } from "../composables/productControlDensity";
import { useConsoleData } from "../composables/useConsoleData";
import { useWorkspaceTradingPrefs } from "../composables/useWorkspaceLayout";
import {
  capabilitySurfaceID,
  type CapabilitySurfaceID,
} from "../features/capabilitySurfaces";

type ResearchSection =
  | "market"
  | "screens"
  | "options"
  | "calendar"
  | "macro"
  | "institutions"
  | "industries"
  | "prediction";

const sections: Array<{
  value: ResearchSection;
  label: string;
  description: string;
  surfaceId: CapabilitySurfaceID;
  capabilities: string[];
  operations: Array<{ value: string; label: string; path: string }>;
}> = [
  {
    value: "market",
    surfaceId: capabilitySurfaceID("research.market"),
    label: "市场",
    description: "涨跌榜、热门榜、盘前盘后与市场分布",
    capabilities: ["涨跌榜", "热门榜", "盘前盘后", "热力分布"],
    operations: [
      {
        value: "top_movers",
        label: "涨跌榜",
        path: "/api/v1/research/rankings?market=US&operation=top_movers&pageSize=50",
      },
      {
        value: "hot",
        label: "热门榜",
        path: "/api/v1/research/rankings?market=US&operation=hot&pageSize=50",
      },
      {
        value: "pre_market",
        label: "盘前",
        path: "/api/v1/research/rankings?market=US&operation=pre_market&pageSize=50",
      },
      {
        value: "after_hours",
        label: "盘后",
        path: "/api/v1/research/rankings?market=US&operation=after_hours&pageSize=50",
      },
      {
        value: "overnight",
        label: "隔夜",
        path: "/api/v1/research/rankings?market=US&operation=overnight&pageSize=50",
      },
      {
        value: "heatmap",
        label: "热力图",
        path: "/api/v1/research/rankings?market=US&operation=heatmap&pageSize=50",
      },
      {
        value: "rise_fall_distribution",
        label: "涨跌分布",
        path: "/api/v1/research/rankings?market=US&operation=rise_fall_distribution&pageSize=50",
      },
    ],
  },
  {
    value: "screens",
    surfaceId: capabilitySurfaceID("research.screens"),
    label: "筛选器",
    description: "股票筛选 V2、期权和港股轮证统一入口",
    capabilities: ["股票筛选 V2", "期权筛选", "港股轮证筛选"],
    operations: [
      {
        value: "stock_v2",
        label: "股票筛选 V2",
        path: "/api/v1/research/screens?market=US&operation=stock_v2&pageSize=50",
      },
      {
        value: "option",
        label: "期权筛选",
        path: "/api/v1/market-data/options/screens?market=US&operation=screen&pageSize=50",
      },
      {
        value: "warrant",
        label: "港股轮证筛选",
        path: "/api/v1/market-data/warrants?market=HK&operation=screen&pageSize=50",
      },
    ],
  },
  {
    value: "options",
    surfaceId: capabilitySurfaceID("research.options"),
    label: "期权研究",
    description: "波动率、0DTE、财报期权、卖方、异动和排行",
    capabilities: ["IV/HV", "0DTE", "财报期权", "异动排行"],
    operations: [
      {
        value: "unusual",
        label: "异动",
        path: "/api/v1/market-data/options/events?market=US&operation=unusual&pageSize=50",
      },
      {
        value: "zero_dte",
        label: "0DTE",
        path: "/api/v1/market-data/options/events?market=US&operation=zero_dte&pageSize=50",
      },
      {
        value: "earnings",
        label: "财报期权",
        path: "/api/v1/market-data/options/events?market=US&operation=earnings&pageSize=50",
      },
      {
        value: "seller",
        label: "卖方筛选",
        path: "/api/v1/market-data/options/events?market=US&operation=seller&pageSize=50",
      },
      {
        value: "screen",
        label: "期权筛选",
        path: "/api/v1/market-data/options/screens?market=US&operation=screen&pageSize=50",
      },
    ],
  },
  {
    value: "calendar",
    surfaceId: capabilitySurfaceID("research.calendar"),
    label: "日历",
    description: "财报、派息及经济事件日历",
    capabilities: ["财报", "派息", "经济事件"],
    operations: [
      {
        value: "earnings",
        label: "财报日历",
        path: "/api/v1/research/calendars?market=US&operation=earnings&pageSize=50",
      },
      {
        value: "dividends",
        label: "派息日历",
        path: "/api/v1/research/calendars?market=US&operation=dividends&pageSize=50",
      },
      {
        value: "economic",
        label: "经济事件",
        path: "/api/v1/research/calendars?market=US&operation=economic&pageSize=50",
      },
      {
        value: "ipos",
        label: "IPO 日历",
        path: "/api/v1/research/calendars?market=US&operation=ipos&pageSize=50",
      },
    ],
  },
  {
    value: "macro",
    surfaceId: capabilitySurfaceID("research.macro"),
    label: "宏观",
    description: "宏观指标、FedWatch 利率与点阵图",
    capabilities: ["宏观指标", "FedWatch", "点阵图"],
    operations: [
      {
        value: "indicators",
        label: "宏观指标",
        path: "/api/v1/research/macro?market=US&operation=indicators&pageSize=50",
      },
      {
        value: "fed_target_rate",
        label: "FedWatch 利率",
        path: "/api/v1/research/macro?market=US&operation=fed_target_rate&pageSize=50",
      },
      {
        value: "fed_dot_plot",
        label: "点阵图",
        path: "/api/v1/research/macro?market=US&operation=fed_dot_plot&pageSize=50",
      },
    ],
  },
  {
    value: "institutions",
    surfaceId: capabilitySurfaceID("research.institutions"),
    label: "机构",
    description: "机构资料、持仓变化和 ARK 动态",
    capabilities: ["机构资料", "持仓变化", "ARK 动态"],
    operations: [
      {
        value: "list",
        label: "机构列表",
        path: "/api/v1/research/institutions?market=US&operation=list&pageSize=50",
      },
      {
        value: "holding_changes",
        label: "持仓变化",
        path: "/api/v1/research/institutions?market=US&operation=holding_changes&pageSize=50",
      },
      {
        value: "ark_fund_holdings",
        label: "ARK 持仓",
        path: "/api/v1/research/institutions?market=US&operation=ark_fund_holdings&pageSize=50",
      },
      {
        value: "ark_stock_activity",
        label: "ARK 动态",
        path: "/api/v1/research/institutions?market=US&operation=ark_stock_activity&pageSize=50",
      },
      {
        value: "ark_transactions",
        label: "ARK 交易",
        path: "/api/v1/research/institutions?market=US&operation=ark_transactions&pageSize=50",
      },
    ],
  },
  {
    value: "industries",
    surfaceId: capabilitySurfaceID("research.industries"),
    label: "产业链",
    description: "产业链、板块和关联股票",
    capabilities: ["产业链", "板块", "关联股票"],
    operations: [
      {
        value: "chains",
        label: "产业链",
        path: "/api/v1/research/industries?market=US&operation=chains&pageSize=50",
      },
      {
        value: "chain_detail",
        label: "产业链详情",
        path: "/api/v1/research/industries?market=US&operation=chain_detail&pageSize=50",
      },
      {
        value: "plate",
        label: "板块资料",
        path: "/api/v1/research/industries?market=US&operation=plate&pageSize=50",
      },
      {
        value: "plate_stocks",
        label: "板块股票",
        path: "/api/v1/research/industries?market=US&operation=plate_stocks&pageSize=50",
      },
    ],
  },
  {
    value: "prediction",
    surfaceId: capabilitySurfaceID("research.prediction"),
    label: "预测市场",
    description: "分类、赛事、系列、事件、合约及 Parlay 构建",
    capabilities: ["事件发现", "YES/NO 合约", "Parlay RFQ"],
    operations: [
      {
        value: "categories",
        label: "事件发现与 Parlay",
        path: "/api/v1/market-data/prediction/categories?pageSize=50",
      },
    ],
  },
];

const route = useRoute();
const router = useRouter();
const { update } = useWorkspaceTradingPrefs();
const { selectedBrokerAccount, systemStatus } = useConsoleData();
const validSection = (value: unknown): ResearchSection => {
  const candidate = String(value ?? "");
  return sections.some((item) => item.value === candidate)
    ? (candidate as ResearchSection)
    : "market";
};
const activeSection = ref<ResearchSection>(validSection(route.query.section));
const activeConfig = computed(() =>
  sections.find((item) => item.value === activeSection.value)!,
);
const activeOperation = ref(activeConfig.value.operations[0]?.value ?? "");
const activePath = computed(
  () =>
    activeConfig.value.operations.find(
      (operation) => operation.value === activeOperation.value,
    )?.path ??
    activeConfig.value.operations[0]?.path ??
    "",
);
const optionResearchOperation = computed(
  () =>
    (["unusual", "zero_dte", "earnings", "seller"].includes(
      activeOperation.value,
    )
      ? activeOperation.value
      : "unusual") as "unusual" | "zero_dte" | "earnings" | "seller",
);
const activeFeatureID = computed(() => {
  switch (activeSection.value) {
    case "market":
      return "research.rankings";
    case "screens":
      if (activeOperation.value === "option") return "derivatives.option_screen";
      if (activeOperation.value === "warrant") return "derivatives.warrants";
      return "research.screen";
    case "options":
      return activeOperation.value === "screen"
        ? "derivatives.option_screen"
        : "derivatives.option_events";
    case "calendar":
      return "research.calendar";
    case "macro":
      return "research.macro";
    case "institutions":
      return "research.institutions";
    case "industries":
      return "research.industry";
    case "prediction":
      return "prediction.discover";
  }
});
const activeMarket = computed(() =>
  activeSection.value === "screens" && activeOperation.value === "warrant"
    ? "HK"
    : "US",
);

watch(
  () => route.query.section,
  (value) => {
    activeSection.value = validSection(value);
    activeOperation.value =
      sections.find((item) => item.value === activeSection.value)?.operations[0]
        ?.value ?? "";
  },
);
watch(activeSection, (value) => {
  activeOperation.value =
    sections.find((item) => item.value === value)?.operations[0]?.value ?? "";
  if (route.query.section === value) return;
  void router.replace({ query: { ...route.query, section: value } });
});

function openInstrument(
  instrumentID: string,
  marketSegment: "securities" | "derivatives" | "prediction" = "securities",
  productClass:
    | "equity"
    | "fund"
    | "option"
    | "warrant"
    | "cbbc"
    | "future"
    | "event_contract"
    | "index"
    | "bond"
    | "plate"
    | "unknown" = "unknown",
): void {
  const [market, ...symbolParts] = instrumentID.split(".");
  const symbol = symbolParts.join(".");
  if (!market || !symbol) return;
  update({ market, symbol, marketSegment, productClass });
  void router.push({
    path: "/workspace",
    query: {
      tab: marketSegment === "prediction" ? "contract" : "chart",
      marketSegment,
    },
  });
}

function openOptionResearchInstrument(
  instrumentID: string,
  productClass: "option" | "equity",
): void {
  openInstrument(
    instrumentID,
    productClass === "option" ? "derivatives" : "securities",
    productClass,
  );
}
</script>

<template>
  <div class="research-page" :data-capability-surface="activeConfig.surfaceId">
    <ProductPanelToolbar
      title="研究中心"
      description="研究结果统一进入交易工作区"
    >
      <BrokerProviderTag
        :feature-id="activeFeatureID"
        :market="activeMarket"
        :preferred-broker-id="selectedBrokerAccount?.brokerId"
        :default-broker-id="systemStatus.defaultBroker"
      />
      <v-btn to="/workspace" variant="text" size="small">返回工作区</v-btn>
    </ProductPanelToolbar>

    <div class="research-page__navigation">
      <v-tabs v-model="activeSection" density="compact" show-arrows>
        <v-tab
          v-for="section in sections"
          :key="section.value"
          :value="section.value"
          :data-capability-surface="section.surfaceId"
        >
          {{ section.label }}
        </v-tab>
      </v-tabs>
    </div>

    <div class="research-page__capabilities">
      <div class="research-page__context">
        <span>当前研究域</span>
        <strong>{{ activeConfig.label }}</strong>
        <small>{{ activeConfig.description }}</small>
      </div>
      <div class="research-page__chips">
        <v-chip
          v-for="capability in activeConfig.capabilities"
          :key="capability"
          size="small"
          variant="tonal"
        >
          {{ capability }}
        </v-chip>
      </div>
      <v-select
        v-if="activeSection !== 'prediction'"
        v-model="activeOperation"
        class="research-page__operation product-compact-control"
        :items="activeConfig.operations"
        :menu-props="productCompactMenuProps"
        item-title="label"
        item-value="value"
        label="数据视图"
        density="compact"
        variant="outlined"
        hide-details
      />
    </div>

    <main class="research-page__body">
      <PredictionResearchPanel
        v-if="activeSection === 'prediction'"
        @open-instrument="openInstrument"
      />
      <OptionResearchPanel
        v-else-if="activeSection === 'options' && activeOperation !== 'screen'"
        market="US"
        :operation="optionResearchOperation"
        scope="market"
        @open-instrument="openOptionResearchInstrument"
      />
      <ProductFeaturePanel
        v-else
        :key="activeConfig.value"
        :title="activeConfig.label"
        :description="activeConfig.description"
        :path="activePath"
        @open-instrument="openInstrument"
      />
    </main>
  </div>
</template>

<style scoped>
.research-page {
  display: flex;
  min-height: 0;
  height: 100%;
  flex-direction: column;
  overflow: hidden;
  background: var(--tv-bg-app);
  color: var(--tv-text);
}

.research-page__navigation {
  min-height: 43px;
  flex: 0 0 auto;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.research-page__navigation :deep(.v-tab) {
  min-width: 76px;
  height: 42px;
  color: var(--tv-text-muted);
  font-size: 10px;
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

.research-page__capabilities {
  display: flex;
  min-height: 58px;
  flex: 0 0 auto;
  align-items: center;
  gap: 12px;
  padding: 7px 16px;
  overflow-x: auto;
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface);
}

.research-page__context {
  display: grid;
  min-width: 190px;
  grid-template-columns: auto 1fr;
  column-gap: 7px;
}

.research-page__context span {
  align-self: center;
  grid-row: span 2;
  color: var(--tv-text-dim);
  font-size: 8px;
  writing-mode: vertical-rl;
}

.research-page__context strong {
  font-size: 11px;
}

.research-page__context small {
  color: var(--tv-text-muted);
  font-size: 8px;
  white-space: nowrap;
}

.research-page__chips {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 5px;
  overflow-x: auto;
}

.research-page__chips :deep(.v-chip) {
  border: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
  color: var(--tv-text-muted);
  font-size: 8px;
}

.research-page__operation {
  min-width: 158px;
  max-width: 190px;
  margin-left: auto;
}

.research-page__body {
  min-height: 0;
  flex: 1;
  overflow: hidden;
}
</style>
