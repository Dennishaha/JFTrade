<script setup lang="ts">
import { computed } from "vue";

import InstrumentIdentity from "../market-data/InstrumentIdentity.vue";
import {
  formatBooleanLabel,
  formatConnectivityLabel,
  formatDateTime,
} from "../../../composables/consoleDataFormatting";
import { useConsoleData } from "../../../composables/useConsoleData";
import { formatMoney, formatNumber } from "../../../utils/numberFormat";

const props = defineProps<{
  marginRatioSymbols: string[];
  supportsCashFlows: boolean;
  supportsMarginRatios: boolean;
  matchesTradingEnvironment: (tradingEnvironment: string) => boolean;
}>();

const {
  brokerCashFlows,
  brokerFunds,
  brokerMarginRatios,
  isLoadingBrokerMarginRatios,
  portfolioCashBalances,
  selectedBrokerAccount,
} = useConsoleData();

const currency = computed(() => brokerFunds.value.summary?.currency ?? undefined);

const accountCashBalances = computed(() => {
  const selected = selectedBrokerAccount.value;
  if (selected == null) {
    return portfolioCashBalances.value.balances.filter((balance) =>
      props.matchesTradingEnvironment(balance.tradingEnvironment),
    );
  }

  return portfolioCashBalances.value.balances.filter(
    (balance) =>
      balance.brokerId === selected.brokerId &&
      balance.accountId === selected.accountId &&
      balance.tradingEnvironment === selected.tradingEnvironment,
  );
});

const recentBrokerCashFlows = computed(() =>
  brokerCashFlows.value.cashFlows.slice(0, 8),
);

const summary = computed(() => brokerFunds.value.summary);

const hasExposure = computed(() => summary.value?.exposureLevel != null);
const hasPdt = computed(
  () => summary.value?.isPdt != null || summary.value?.dtStatus != null,
);
const showMarginSection = computed(
  () => hasExposure.value || hasPdt.value,
);

function formatAmount(value: number | null | undefined): string {
  return formatNumber(value, { maximumFractionDigits: 4 });
}

function formatCash(value: number | null | undefined, cur?: string | null): string {
  return formatMoney(value, cur ?? currency.value, { maximumFractionDigits: 4 });
}
</script>

<template>
  <div class="account-more">
    <details class="account-more__section" open>
      <summary>
        <span class="tv-panel-title">多币种现金余额</span>
        <span class="account-more__meta">{{ accountCashBalances.length }} 个币种</span>
      </summary>
      <div class="account-more__body is-flush">
        <table v-if="accountCashBalances.length" class="tv-table">
          <thead>
            <tr>
              <th>币种</th>
              <th class="tv-num">现金余额</th>
              <th>更新时间</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="balance in accountCashBalances"
              :key="`${balance.accountId}-${balance.currency}`"
            >
              <td>{{ balance.currency }}</td>
              <td class="tv-num">{{ formatCash(balance.cashBalance, balance.currency) }}</td>
              <td>{{ formatDateTime(balance.updatedAt) }}</td>
            </tr>
          </tbody>
        </table>
        <div v-else class="account-more__empty">当前账户暂无可展示的现金余额。</div>
      </div>
    </details>

    <details v-if="showMarginSection" class="account-more__section">
      <summary>
        <span class="tv-panel-title">持仓限额与日内交易</span>
        <span class="account-more__meta">保证金补充信息</span>
      </summary>
      <div class="account-more__body">
        <div v-if="hasExposure" class="account-more__kv-grid">
          <div class="account-more__kv">
            <span>限额等级</span>
            <b>{{ summary?.exposureLevel }}</b>
          </div>
          <div v-if="summary?.exposureLimit != null" class="account-more__kv">
            <span>持仓限额</span>
            <b class="tv-num">{{ formatCash(summary.exposureLimit) }}</b>
          </div>
          <div v-if="summary?.remainingLimit != null" class="account-more__kv">
            <span>剩余限额</span>
            <b class="tv-num">{{ formatCash(summary.remainingLimit) }}</b>
          </div>
        </div>

        <div v-if="hasPdt" class="account-more__pdt">
          <div class="account-more__pdt-title">美股 PDT / 日内交易</div>
          <div class="account-more__kv-grid">
            <div v-if="summary?.isPdt != null" class="account-more__kv">
              <span>PDT 账户</span>
              <b>{{ summary.isPdt ? "是" : "否" }}</b>
            </div>
            <div v-if="summary?.pdtSeq != null" class="account-more__kv">
              <span>日内交易次数</span>
              <b>{{ summary.pdtSeq }}</b>
            </div>
            <div v-if="summary?.remainingDTBP != null" class="account-more__kv">
              <span>剩余日内购买力</span>
              <b class="tv-num">{{ formatCash(summary.remainingDTBP) }}</b>
            </div>
            <div v-if="summary?.dtStatus != null" class="account-more__kv">
              <span>限制状态</span>
              <b>{{ summary.dtStatus }}</b>
            </div>
          </div>
        </div>
      </div>
    </details>

    <details class="account-more__section">
      <summary>
        <span class="tv-panel-title">最近资金流水</span>
        <span class="account-more__meta">
          {{ formatConnectivityLabel(brokerCashFlows.connectivity) }}
        </span>
      </summary>
      <div class="account-more__body is-flush">
        <div v-if="!supportsCashFlows" class="account-more__empty">
          当前券商未为该交易环境声明资金流水能力。
        </div>
        <div
          v-else-if="brokerCashFlows.lastError"
          class="account-more__error tv-status--warning tv-status-surface"
        >
          {{ brokerCashFlows.lastError }}
        </div>
        <table v-else-if="recentBrokerCashFlows.length" class="tv-table">
          <thead>
            <tr>
              <th>流水号</th>
              <th>清算日</th>
              <th>交收日</th>
              <th>类型 / 方向</th>
              <th class="tv-num">金额</th>
              <th>备注</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="flow in recentBrokerCashFlows"
              :key="flow.cashFlowId ?? `${flow.clearingDate ?? 'na'}-${flow.cashFlowType ?? 'na'}-${flow.cashFlowAmount ?? 'na'}`"
            >
              <td class="account-more__mono">{{ flow.cashFlowId ?? "—" }}</td>
              <td>{{ flow.clearingDate ?? "—" }}</td>
              <td>{{ flow.settlementDate ?? "—" }}</td>
              <td>
                <div>{{ flow.cashFlowType ?? "未分类" }}</div>
                <div class="account-more__dim">{{ flow.cashFlowDirection ?? "方向未标注" }}</div>
              </td>
              <td class="tv-num">{{ formatCash(flow.cashFlowAmount, flow.currency) }}</td>
              <td class="account-more__dim">{{ flow.cashFlowRemark ?? "—" }}</td>
            </tr>
          </tbody>
        </table>
        <div v-else class="account-more__empty">当前账户暂无券商资金流水。</div>
      </div>
    </details>

    <details class="account-more__section">
      <summary>
        <span class="tv-panel-title">融资融券参数</span>
        <span class="account-more__meta">
          {{ formatConnectivityLabel(brokerMarginRatios.connectivity) }}
        </span>
      </summary>
      <div class="account-more__body is-flush">
        <div v-if="!supportsMarginRatios" class="account-more__empty">
          当前券商未为该交易环境声明融资融券参数能力。
        </div>
        <div v-else-if="marginRatioSymbols.length === 0" class="account-more__empty">
          当前账户暂无持仓标的，融资融券参数按持仓标的查询。
        </div>
        <div v-else-if="isLoadingBrokerMarginRatios" class="account-more__empty">
          正在加载融资融券参数...
        </div>
        <div
          v-else-if="brokerMarginRatios.lastError"
          class="account-more__error tv-status--warning tv-status-surface"
        >
          {{ brokerMarginRatios.lastError }}
        </div>
        <table v-else-if="brokerMarginRatios.marginRatios.length" class="tv-table">
          <thead>
            <tr>
              <th>标的</th>
              <th>融资 / 融券</th>
              <th class="tv-num">券源余量</th>
              <th class="tv-num">融券费率</th>
              <th class="tv-num">预警比率</th>
              <th class="tv-num">初始保证金</th>
              <th class="tv-num">维持保证金</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="ratio in brokerMarginRatios.marginRatios" :key="ratio.symbol">
              <td>
                <InstrumentIdentity :market="ratio.market" :instrument-id="ratio.symbol" compact />
              </td>
              <td>
                {{ formatBooleanLabel(ratio.isLongPermit) }} / {{ formatBooleanLabel(ratio.isShortPermit) }}
              </td>
              <td class="tv-num">{{ formatAmount(ratio.shortPoolRemain) }}</td>
              <td class="tv-num">{{ formatAmount(ratio.shortFeeRate) }}</td>
              <td class="tv-num">
                {{ formatAmount(ratio.alertLongRatio) }} / {{ formatAmount(ratio.alertShortRatio) }}
              </td>
              <td class="tv-num">
                {{ formatAmount(ratio.initialMarginLongRatio) }} / {{ formatAmount(ratio.initialMarginShortRatio) }}
              </td>
              <td class="tv-num">
                {{ formatAmount(ratio.maintenanceLongRatio) }} / {{ formatAmount(ratio.maintenanceShortRatio) }}
              </td>
            </tr>
          </tbody>
        </table>
        <div v-else class="account-more__empty">当前账户暂无融资融券参数。</div>
      </div>
    </details>
  </div>
</template>

<style scoped>
.account-more {
  display: grid;
  flex: 1;
  align-content: start;
  gap: 0;
  overflow: auto;
  scrollbar-width: thin;
}

.account-more__section {
  border-bottom: 1px solid var(--tv-border);
}

.account-more__section summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 12px;
  background: var(--tv-bg-surface-2);
  cursor: pointer;
  list-style: none;
  user-select: none;
}

.account-more__section summary::-webkit-details-marker {
  display: none;
}

.account-more__section summary::before {
  color: var(--tv-text-dim);
  content: "▸";
  font-size: 10px;
  transition: transform 0.12s ease;
}

.account-more__section[open] summary::before {
  transform: rotate(90deg);
}

.account-more__section summary .tv-panel-title {
  margin-right: auto;
  color: var(--tv-text-muted);
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.account-more__meta {
  color: var(--tv-text-dim);
  font-size: 10px;
}

.account-more__body {
  padding: 10px 12px 14px;
}

.account-more__body.is-flush {
  padding: 0;
}

.account-more__empty {
  padding: 18px 12px;
  color: var(--tv-text-dim);
  font-size: 11px;
}

.account-more__error {
  margin: 10px 12px;
  padding: 8px 10px;
  border: 1px solid;
  border-radius: 6px;
  font-size: 11px;
}

.account-more__kv-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 8px;
}

.account-more__kv {
  padding: 8px 10px;
  border: 1px solid var(--tv-border);
  border-radius: 6px;
  background: var(--tv-bg-surface-2);
}

.account-more__kv span {
  display: block;
  color: var(--tv-text-dim);
  font-size: 10px;
}

.account-more__kv b {
  display: block;
  margin-top: 2px;
  color: var(--tv-text);
  font-size: 12px;
  font-weight: 600;
}

.account-more__pdt {
  margin-top: 10px;
  padding: 10px;
  border: 1px solid color-mix(in srgb, var(--tv-warn) 40%, transparent);
  border-radius: 6px;
  background: color-mix(in srgb, var(--tv-warn) 6%, transparent);
}

.account-more__pdt-title {
  margin-bottom: 8px;
  color: var(--tv-warn);
  font-size: 10px;
  font-weight: 650;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.account-more__mono {
  font-family: ui-monospace, monospace;
  font-size: 10px;
}

.account-more__dim {
  color: var(--tv-text-dim);
  font-size: 10px;
}
</style>
