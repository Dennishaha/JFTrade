<script setup lang="ts">
import { computed } from "vue";

import { useConsoleData } from "../../../composables/useConsoleData";
import { formatMoney } from "../../../utils/numberFormat";

interface MetricItem {
  label: string;
  value: number | string | null | undefined;
  tone?: "up" | "down" | "warn" | "plain";
  isMoney?: boolean;
}

const { brokerFunds } = useConsoleData();

const summary = computed(() => brokerFunds.value.summary);
const currency = computed(() => summary.value?.currency ?? undefined);
const fundsError = computed(() => brokerFunds.value.lastError?.trim() ?? "");

const marginRiskState = computed<"warning" | "success" | "unknown">(() => {
  const value = summary.value?.marginCallMargin;
  if (value == null) return "unknown";
  if (value > 0) return "warning";
  return value === 0 ? "success" : "unknown";
});

const riskTone = computed<"warn" | "plain">(() =>
  marginRiskState.value === "warning" ? "warn" : "plain",
);

const riskDotClass = computed(() => {
  if (marginRiskState.value === "success") return "tv-status--success";
  return "tv-status--warning";
});

const riskTitle = computed(() => {
  switch (marginRiskState.value) {
    case "warning":
      return "存在追保保证金，请关注账户风险";
    case "success":
      return "追保保证金为零";
    default:
      return "风控数据不可用，无法判断账户风险";
  }
});

function pnlTone(value: number | null | undefined): "up" | "down" | "plain" {
  if (value == null || value === 0) return "plain";
  return value > 0 ? "up" : "down";
}

const sections = computed<Array<{ title: string; items: MetricItem[] }>>(() => {
  const s = summary.value;
  return [
    {
      title: "资产",
      items: [
        { label: "总资产", value: s?.totalAssets, isMoney: true },
        { label: "证券市值", value: s?.marketValue, isMoney: true },
        { label: "多头市值", value: s?.longMarketValue, isMoney: true },
        { label: "空头市值", value: s?.shortMarketValue, isMoney: true },
        {
          label: "未实现盈亏",
          value: s?.unrealizedPnl,
          tone: pnlTone(s?.unrealizedPnl),
          isMoney: true,
        },
        {
          label: "已实现盈亏",
          value: s?.realizedPnl,
          tone: pnlTone(s?.realizedPnl),
          isMoney: true,
        },
      ],
    },
    {
      title: "现金明细",
      items: [
        { label: "现金", value: s?.cash, isMoney: true },
        { label: "可用资金", value: s?.availableFunds, isMoney: true },
        { label: "在途资产", value: s?.pendingAsset, isMoney: true },
        { label: "冻结资金", value: s?.frozenCash, isMoney: true },
        { label: "计息金额", value: s?.debtCash, isMoney: true },
      ],
    },
    {
      title: "购买力",
      items: [
        { label: "最大购买力", value: s?.purchasingPower, isMoney: true },
        { label: "现金购买力", value: s?.netCashPower, isMoney: true },
        { label: "卖空购买力", value: s?.shortSellingPower, isMoney: true },
        { label: "可取现金", value: s?.availableWithdrawalCash, isMoney: true },
        { label: "融资可提", value: s?.maxWithdrawal, isMoney: true },
      ],
    },
    {
      title: "账户风控",
      items: [
        { label: "风险等级", value: s?.riskStatus ?? null, tone: riskTone.value },
        { label: "初始保证金", value: s?.initialMargin, isMoney: true },
        { label: "维持保证金", value: s?.maintenanceMargin, isMoney: true },
        {
          label: "追保保证金",
          value: s?.marginCallMargin,
          tone: riskTone.value,
          isMoney: true,
        },
        { label: "剩余限额", value: s?.remainingLimit, isMoney: true },
      ],
    },
  ];
});

function displayValue(item: MetricItem): string {
  if (item.value == null || item.value === "") return "--";
  if (item.isMoney === true && typeof item.value === "number") {
    return formatMoney(item.value, currency.value, { maximumFractionDigits: 2 });
  }
  return String(item.value);
}

function toneClass(item: MetricItem): string {
  switch (item.tone) {
    case "up":
      return "tv-up";
    case "down":
      return "tv-down";
    case "warn":
      return "is-warn";
    default:
      return "";
  }
}
</script>

<template>
  <div class="asset-strip" aria-label="资产指标">
    <div v-if="fundsError" class="asset-strip__error" role="status">
      资金数据暂不可用：{{ fundsError }}
    </div>
    <section v-for="section in sections" :key="section.title" class="asset-strip__section">
      <header class="asset-strip__title">
        {{ section.title }}
        <i
          v-if="section.title === '账户风控'"
          class="tv-state-dot"
          :class="riskDotClass"
          :title="riskTitle"
        ></i>
      </header>
      <div class="asset-strip__grid">
        <div v-for="item in section.items" :key="item.label" class="asset-strip__item">
          <span>{{ item.label }}</span>
          <b class="tv-num" :class="toneClass(item)" :title="displayValue(item)">
            {{ displayValue(item) }}
          </b>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.asset-strip {
  display: grid;
  flex: 0 0 auto;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  border-bottom: 1px solid var(--tv-border);
  background: var(--tv-bg-surface-2);
}

.asset-strip__section {
  min-width: 0;
  padding: 10px 14px 12px;
  border-left: 1px solid var(--tv-border);
}

.asset-strip__error {
  grid-column: 1 / -1;
  padding: 7px 14px;
  border-bottom: 1px solid color-mix(in srgb, var(--tv-warn) 35%, var(--tv-border));
  background: color-mix(in srgb, var(--tv-warn) 9%, var(--tv-bg-surface-2));
  color: var(--tv-warn);
  font-size: 11px;
}

.asset-strip__section:first-child {
  border-left: 0;
}

.asset-strip__title {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  color: var(--tv-text-muted);
  font-size: 10px;
  font-weight: 650;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.asset-strip__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px 12px;
}

.asset-strip__item {
  min-width: 0;
}

.asset-strip__item span {
  display: block;
  color: var(--tv-text-dim);
  font-size: 10px;
}

.asset-strip__item b {
  display: block;
  overflow: hidden;
  margin-top: 1px;
  color: var(--tv-text);
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.asset-strip__item b.is-warn {
  color: var(--tv-warn);
}

@media (max-width: 1180px) {
  .asset-strip {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .asset-strip__section:nth-child(odd) {
    border-left: 0;
  }

  .asset-strip__section:nth-child(n + 3) {
    border-top: 1px solid var(--tv-border);
  }
}
</style>
