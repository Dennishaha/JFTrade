<script setup lang="ts">
import { computed } from "vue";

import PageHeader from "../components/PageHeader.vue";
import SectionHeader from "../components/SectionHeader.vue";
import { useConsoleData } from "../composables/useConsoleData";

const {
  portfolioCashBalances,
  portfolioCashReconciliation,
  portfolioPositions,
  portfolioReconciliation,
  resolvePortfolioReconciliationStatusLabel,
  resolvePortfolioReconciliationTagType,
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
        <el-table :data="portfolioCashBalances.balances.slice(0, 5)" stripe>
          <el-table-column prop="currency" label="Currency" width="100" />
          <el-table-column prop="tradingEnvironment" label="Environment" width="150" />
          <el-table-column prop="accountId" label="Account ID" />
          <el-table-column prop="cashBalance" label="Cash Balance" align="right">
            <template #default="{ row }">
              {{ row.cashBalance }}
            </template>
          </el-table-column>
          <el-table-column prop="updatedAt" label="Updated At" width="200">
            <template #default="{ row }">
              {{ row.updatedAt }}
            </template>
          </el-table-column>
        </el-table>
      </div>
      <el-empty v-else description="No cash balances available" :image-size="80" />
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
        <el-table :data="portfolioPositions.positions.slice(0, 6)" stripe>
          <el-table-column prop="symbol" label="Symbol" width="120" sortable />
          <el-table-column prop="market" label="Market" width="80" />
          <el-table-column prop="tradingEnvironment" label="Environment" width="120" />
          <el-table-column prop="accountId" label="Account" />
          <el-table-column prop="quantity" label="Quantity" align="right" sortable>
            <template #default="{ row }">
              {{ row.quantity }}
            </template>
          </el-table-column>
          <el-table-column prop="averagePrice" label="Average Price" align="right" sortable>
            <template #default="{ row }">
              {{ row.averagePrice }}
            </template>
          </el-table-column>
          <el-table-column prop="marketValue" label="Market Value" align="right" sortable>
            <template #default="{ row }">
              {{ row.marketValue }}
            </template>
          </el-table-column>
          <el-table-column prop="brokerId" label="Broker" width="100" />
        </el-table>
      </div>
      <el-empty v-else description="No positions available" :image-size="80" />
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
        <el-table :data="portfolioReconciliation.positions.slice(0, 6)" stripe>
          <el-table-column prop="symbol" label="Symbol" width="120" sortable />
          <el-table-column prop="symbolName" label="Security Name" />
          <el-table-column prop="market" label="Market" width="80" />
          <el-table-column prop="accountId" label="Account" />
          <el-table-column prop="status" label="Status" width="100">
            <template #default="{ row }">
              <el-tag :type="resolvePortfolioReconciliationTagType(row.status)" effect="plain">
                {{ resolvePortfolioReconciliationStatusLabel(row.status) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="projectedQuantity" label="Projected Qty" align="right" sortable>
            <template #default="{ row }">
              {{ row.projectedQuantity ?? "—" }}
            </template>
          </el-table-column>
          <el-table-column prop="brokerQuantity" label="Broker Qty" align="right" sortable>
            <template #default="{ row }">
              {{ row.brokerQuantity ?? "—" }}
            </template>
          </el-table-column>
          <el-table-column prop="quantityDelta" label="Qty Delta" align="right" sortable>
            <template #default="{ row }">
              {{ row.quantityDelta }}
            </template>
          </el-table-column>
        </el-table>
      </div>
      <el-empty v-else description="No reconciliation data available" :image-size="80" />

      <!-- Collapsible Details -->
      <el-collapse>
        <el-collapse-item title="Detailed Reconciliation Info" name="details">
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
                <el-tag :type="resolvePortfolioReconciliationTagType(item.status)" effect="plain">
                  {{ resolvePortfolioReconciliationStatusLabel(item.status) }}
                </el-tag>
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
        </el-collapse-item>

        <el-collapse-item title="Cash Reconciliation Details" name="cash">
          <div v-if="portfolioCashReconciliation.balances.length" class="grid gap-4">
            <div
              v-for="item in portfolioCashReconciliation.balances.slice(0, 5)"
              :key="`${item.accountId}-${item.currency}`"
              class="rounded-lg border border-slate-200 bg-slate-50 p-4"
            >
              <div class="flex items-center justify-between gap-3">
                <div class="text-base font-semibold text-slate-900">{{ item.currency }}</div>
                <el-tag :type="resolvePortfolioReconciliationTagType(item.status)" effect="plain">
                  {{ resolvePortfolioReconciliationStatusLabel(item.status) }}
                </el-tag>
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
        </el-collapse-item>
      </el-collapse>
    </section>
  </div>
</template>