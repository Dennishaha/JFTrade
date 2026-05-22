<script setup lang="ts">
import { computed } from "vue";

import PageHeader from "../components/PageHeader.vue";
import SectionHeader from "../components/SectionHeader.vue";
import {
  resolvePortfolioReconciliationStatusLabel,
  resolvePortfolioReconciliationTagType,
} from "../composables/consoleDataFormatting";
import { useConsoleData } from "../composables/useConsoleData";

const {
  portfolioCashBalances,
  portfolioCashReconciliation,
  portfolioPositions,
  portfolioReconciliation,
} = useConsoleData();

const portfolioHeaderStats = computed(() => [
  {
    label: "Projected Positions",
    value: portfolioPositions.value.positions.length,
  },
  {
    label: "Cash Balances",
    value: portfolioCashBalances.value.balances.length,
  },
  {
    label: "Cash Recon",
    value: portfolioCashReconciliation.value.connectivity.toUpperCase(),
    tone:
      portfolioCashReconciliation.value.connectivity === "connected"
        ? "good"
        : "warn",
  },
  {
    label: "Position Recon",
    value: portfolioReconciliation.value.connectivity.toUpperCase(),
    tone:
      portfolioReconciliation.value.connectivity === "connected"
        ? "good"
        : "warn",
  },
]);

const kpiMetrics = computed(() => {
  const firstCashBalance = portfolioCashBalances.value.balances[0];
  const totalCash = firstCashBalance?.cashBalance ?? 0;
  const totalEquity = portfolioPositions.value.positions.reduce(
    (sum, pos) => sum + (pos.marketValue || 0),
    0,
  );
  const positionCount = portfolioPositions.value.positions.length;

  return [
    {
      label: "Total Cash",
      value: totalCash,
    },
    {
      label: "Total Equity",
      value: totalEquity,
    },
    {
      label: "Position Count",
      value: positionCount,
    },
    {
      label: "Connectivity",
      value:
        portfolioReconciliation.value.connectivity === "connected"
          ? "Connected"
          : "Disconnected",
    },
  ];
});
</script>

<template>
  <div class="grid gap-6">
    <PageHeader
      eyebrow="Portfolio control"
      title="Portfolio / Projection"
      description="统一展示 execution 投影出的现金、持仓与券商对账结果，把投影状态和偏差值放到一个组合视图里。"
      :stats="portfolioHeaderStats"
    />

    <!-- KPI Strip -->
    <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
      <div
        v-for="metric in kpiMetrics"
        :key="metric.label"
        class="rounded-lg border border-slate-200 bg-white px-4 py-3"
      >
        <div class="text-xs uppercase tracking-[0.2em] text-slate-500">
          {{ metric.label }}
        </div>
        <div class="mt-2 text-lg font-semibold text-slate-900">
          {{ metric.value ?? "—" }}
        </div>
      </div>
    </div>

    <!-- Projected Cash Section -->
    <section class="grid gap-5">
      <SectionHeader
        title="Projected Cash"
        description="Current cash balances across accounts"
      />

      <div
        v-if="portfolioCashBalances.balances.length"
        class="overflow-x-auto rounded-lg border border-slate-200 bg-white"
      >
        <table class="w-full text-sm">
          <thead class="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th class="px-4 py-3 text-left">Currency</th>
              <th class="px-4 py-3 text-left">Environment</th>
              <th class="px-4 py-3 text-left">Account ID</th>
              <th class="px-4 py-3 text-right">Cash Balance</th>
              <th class="px-4 py-3 text-left">Updated At</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in portfolioCashBalances.balances.slice(0, 5)" :key="row.accountId" class="border-b border-slate-100 last:border-0">
              <td class="px-4 py-3">{{ row.currency }}</td>
              <td class="px-4 py-3">{{ row.tradingEnvironment }}</td>
              <td class="px-4 py-3">{{ row.accountId }}</td>
              <td class="px-4 py-3 text-right">{{ row.cashBalance }}</td>
              <td class="px-4 py-3">{{ row.updatedAt }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <v-empty-state v-else text="No cash balances available" />
    </section>

    <!-- Positions / Holdings Section -->
    <section class="grid gap-5">
      <SectionHeader
        title="Projected Portfolio"
        description="Current holdings and positions"
      />

      <div
        v-if="portfolioPositions.positions.length"
        class="overflow-x-auto rounded-lg border border-slate-200 bg-white"
      >
        <table class="w-full text-sm">
          <thead class="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th class="px-4 py-3 text-left">Symbol</th>
              <th class="px-4 py-3 text-left">Market</th>
              <th class="px-4 py-3 text-left">Environment</th>
              <th class="px-4 py-3 text-left">Account</th>
              <th class="px-4 py-3 text-right">Quantity</th>
              <th class="px-4 py-3 text-right">Average Price</th>
              <th class="px-4 py-3 text-right">Market Value</th>
              <th class="px-4 py-3 text-left">Broker</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in portfolioPositions.positions.slice(0, 6)" :key="`${row.accountId}-${row.symbol}`" class="border-b border-slate-100 last:border-0">
              <td class="px-4 py-3">{{ row.symbol }}</td>
              <td class="px-4 py-3">{{ row.market }}</td>
              <td class="px-4 py-3">{{ row.tradingEnvironment }}</td>
              <td class="px-4 py-3">{{ row.accountId }}</td>
              <td class="px-4 py-3 text-right">{{ row.quantity }}</td>
              <td class="px-4 py-3 text-right">{{ row.averagePrice }}</td>
              <td class="px-4 py-3 text-right">{{ row.marketValue }}</td>
              <td class="px-4 py-3">{{ row.brokerId }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <v-empty-state v-else text="No positions available" />
    </section>

    <!-- Portfolio Reconciliation & Secondary Details -->
    <section class="grid gap-5">
      <SectionHeader
        title="Portfolio Reconciliation"
        description="Position and cash reconciliation details"
      />

      <!-- Reconciliation Table -->
      <div
        v-if="portfolioReconciliation.positions.length"
        class="overflow-x-auto rounded-lg border border-slate-200 bg-white"
      >
        <table class="w-full text-sm">
          <thead class="border-b border-slate-200 bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
            <tr>
              <th class="px-4 py-3 text-left">Symbol</th>
              <th class="px-4 py-3 text-left">Security Name</th>
              <th class="px-4 py-3 text-left">Market</th>
              <th class="px-4 py-3 text-left">Account</th>
              <th class="px-4 py-3 text-left">Status</th>
              <th class="px-4 py-3 text-right">Projected Qty</th>
              <th class="px-4 py-3 text-right">Broker Qty</th>
              <th class="px-4 py-3 text-right">Qty Delta</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in portfolioReconciliation.positions.slice(0, 6)" :key="`${row.accountId}-${row.symbol}`" class="border-b border-slate-100 last:border-0">
              <td class="px-4 py-3">{{ row.symbol }}</td>
              <td class="px-4 py-3">{{ row.symbolName }}</td>
              <td class="px-4 py-3">{{ row.market }}</td>
              <td class="px-4 py-3">{{ row.accountId }}</td>
              <td class="px-4 py-3">
                <v-chip :color="resolvePortfolioReconciliationTagType(row.status) === 'danger' ? 'error' : resolvePortfolioReconciliationTagType(row.status)" variant="outlined" size="small">
                  {{ resolvePortfolioReconciliationStatusLabel(row.status) }}
                </v-chip>
              </td>
              <td class="px-4 py-3 text-right">{{ row.projectedQuantity ?? "—" }}</td>
              <td class="px-4 py-3 text-right">{{ row.brokerQuantity ?? "—" }}</td>
              <td class="px-4 py-3 text-right">{{ row.quantityDelta }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <v-empty-state v-else text="No reconciliation data available" />

      <!-- Collapsible Details -->
      <v-expansion-panels variant="accordion">
        <v-expansion-panel title="Detailed Reconciliation Info">
          <v-expansion-panel-text>
            <div v-if="portfolioReconciliation.positions.length" class="grid gap-4">
              <div
                v-for="item in portfolioReconciliation.positions.slice(0, 6)"
                :key="`${item.accountId}-${item.market}-${item.symbol}`"
                class="rounded-lg border border-slate-200 bg-slate-50 p-4"
              >
                <div class="flex items-center justify-between gap-3">
                  <div>
                    <div class="font-semibold text-slate-900">{{ item.symbol }}</div>
                    <div class="text-sm text-slate-500">
                      {{ item.symbolName ?? "Unknown Security" }} / {{ item.accountId }} / {{ item.market }}
                    </div>
                  </div>
                  <v-chip :color="resolvePortfolioReconciliationTagType(item.status) === 'danger' ? 'error' : resolvePortfolioReconciliationTagType(item.status)" variant="outlined" size="small">
                    {{ resolvePortfolioReconciliationStatusLabel(item.status) }}
                  </v-chip>
                </div>
                <div class="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Projected Qty</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">
                      {{ item.projectedQuantity ?? "—" }}
                    </div>
                  </div>
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Broker Qty</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">
                      {{ item.brokerQuantity ?? "—" }}
                    </div>
                  </div>
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Qty Delta</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">{{ item.quantityDelta }}</div>
                  </div>
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Projected Price</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">
                      {{ item.projectedAveragePrice ?? "—" }}
                    </div>
                  </div>
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Broker Cost Price</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">
                      {{ item.brokerAverageCostPrice ?? "—" }}
                    </div>
                  </div>
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Price Delta</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">{{ item.averagePriceDelta }}</div>
                  </div>
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Projected PnL</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">
                      {{ item.projectedRealizedPnl ?? "—" }}
                    </div>
                  </div>
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Broker PnL</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">
                      {{ item.brokerRealizedPnl ?? "—" }}
                    </div>
                  </div>
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">PnL Delta</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">{{ item.realizedPnlDelta }}</div>
                  </div>
                </div>
              </div>
            </div>
          </v-expansion-panel-text>
        </v-expansion-panel>

        <v-expansion-panel title="Cash Reconciliation Details">
          <v-expansion-panel-text>
            <div v-if="portfolioCashReconciliation.balances.length" class="grid gap-4">
              <div
                v-for="item in portfolioCashReconciliation.balances.slice(0, 5)"
                :key="`${item.accountId}-${item.currency}`"
                class="rounded-lg border border-slate-200 bg-slate-50 p-4"
              >
                <div class="flex items-center justify-between gap-3">
                  <div class="text-base font-semibold text-slate-900">{{ item.currency }}</div>
                  <v-chip :color="resolvePortfolioReconciliationTagType(item.status) === 'danger' ? 'error' : resolvePortfolioReconciliationTagType(item.status)" variant="outlined" size="small">
                    {{ resolvePortfolioReconciliationStatusLabel(item.status) }}
                  </v-chip>
                </div>
                <div class="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Projected Cash</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">
                      {{ item.projectedCashBalance ?? "—" }}
                    </div>
                  </div>
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Broker Cash</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">{{ item.brokerCash ?? "—" }}</div>
                  </div>
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Cash Delta</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">{{ item.cashDelta }}</div>
                  </div>
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Available Withdrawal</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">
                      {{ item.brokerAvailableWithdrawalCash ?? "—" }}
                    </div>
                  </div>
                  <div>
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Net Cash Power</div>
                    <div class="mt-2 text-sm font-semibold text-slate-900">
                      {{ item.brokerNetCashPower ?? "—" }}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </v-expansion-panel-text>
        </v-expansion-panel>
      </v-expansion-panels>
    </section>
  </div>
</template>