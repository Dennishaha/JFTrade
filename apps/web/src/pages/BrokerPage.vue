<script setup lang="ts">
import { computed, ref } from "vue";

import PageHeader from "../components/PageHeader.vue";
import { useConsoleData } from "../composables/useConsoleData";

const {
  brokerCashFlows,
  brokerFunds,
  brokerOrders,
  brokerPositions,
  futuOpenDHealth,
  brokerRuntime,
  liveStreamCheckedAt,
  liveStreamStatus,
} = useConsoleData();

const brokerHeaderStats = computed(() => [
  {
    label: "Connectivity",
    value: brokerRuntime.value.session.connectivity.toUpperCase(),
    tone:
      brokerRuntime.value.session.connectivity === "connected"
        ? "good"
        : brokerRuntime.value.session.connectivity === "degraded"
          ? "warn"
          : "danger",
  },
  {
    label: "Accounts",
    value: brokerRuntime.value.accounts.length,
    hint: `${brokerRuntime.value.session.accountsDiscovered} discovered`,
  },
  {
    label: "Positions",
    value: brokerPositions.value.positions.length,
  },
  {
    label: "Orders",
    value: brokerOrders.value.orders.length,
    hint: `stream ${liveStreamStatus.value.toUpperCase()} / ${liveStreamCheckedAt.value || "waiting"}`,
  },
]);

const brokerActiveTab = ref("accounts");
</script>

<template>
  <div class="grid gap-6">
    <PageHeader
      eyebrow="Broker monitor"
      title="Broker / Runtime"
      description="把 OpenD 会话、账户、资金、持仓与券商订单按运行态优先级分层展示，先看连接，再看账户，再看资金和订单。"
      :stats="brokerHeaderStats"
    />

    <!-- Connectivity Strip (stays outside tabs) -->
    <section class="grid gap-5 lg:grid-cols-[1fr_1fr]">
      <v-card flat class="card-shell border-0">
        <div class="flex items-center justify-between gap-3 px-4 pt-4">
          <div class="text-xl font-semibold text-slate-900">Futu Broker Runtime</div>
          <v-chip
            :color="brokerRuntime.session.connectivity === 'connected' ? 'success' : brokerRuntime.session.connectivity === 'degraded' ? 'warning' : 'error'"
            variant="outlined"
            size="small"
          >
            {{ brokerRuntime.session.connectivity.toUpperCase() }}
          </v-chip>
        </div>
        <v-card-text>
          <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <div class="rounded-3xl bg-slate-50 px-4 py-4">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">OpenD Host</div>
              <div class="mt-2 text-xl font-semibold text-slate-900">{{ brokerRuntime.session.connection.host }}</div>
            </div>
            <div class="rounded-3xl bg-slate-50 px-4 py-4">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">OpenD WebSocket Port</div>
              <div class="mt-2 text-xl font-semibold text-slate-900">{{ brokerRuntime.session.connection.port }}</div>
            </div>
            <div class="rounded-3xl bg-slate-50 px-4 py-4">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Quote Login</div>
              <div class="mt-2 text-xl font-semibold text-slate-900">
                {{ brokerRuntime.session.globalState?.quoteLoggedIn ? 'YES' : 'NO' }}
              </div>
            </div>
            <div class="rounded-3xl bg-slate-50 px-4 py-4">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Accounts</div>
              <div class="mt-2 text-xl font-semibold text-slate-900">{{ brokerRuntime.session.accountsDiscovered }}</div>
            </div>
          </div>

          <div class="mt-5 grid gap-3 text-sm text-slate-600">
            <v-alert
              v-if="futuOpenDHealth.diagnosis.manualRetryRequired"
              type="error"
              :closable="false"
              title="OpenD 自动重试已暂停"
            >
              {{ futuOpenDHealth.diagnosis.summary }}
            </v-alert>
            <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Program Status</div>
              <div class="mt-2 font-medium text-slate-900">
                {{ brokerRuntime.session.globalState?.programStatus ?? 'Unavailable' }}
              </div>
            </div>
            <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
              <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Live Stream</div>
              <div class="mt-2 flex items-center gap-3">
                <v-chip
                  :color="liveStreamStatus === 'connected' ? 'success' : liveStreamStatus === 'degraded' ? 'warning' : undefined"
                  variant="outlined"
                  size="small"
                >
                  {{ liveStreamStatus.toUpperCase() }}
                </v-chip>
                <span class="text-xs text-slate-500">{{ liveStreamCheckedAt || 'waiting for first event' }}</span>
              </div>
            </div>
          </div>
        </v-card-text>
      </v-card>
    </section>

    <!-- Tabs for remaining content -->
    <v-tabs v-model="brokerActiveTab" bg-color="transparent" class="tv-page-tabs">
      <v-tab value="accounts">Accounts</v-tab>
      <v-tab value="funds">Funds &amp; Cash</v-tab>
      <v-tab value="positions">Positions</v-tab>
      <v-tab value="orders">Orders</v-tab>
    </v-tabs>
    <v-window v-model="brokerActiveTab">
      <!-- Accounts Tab -->
      <v-window-item value="accounts">
        <v-card flat class="card-shell border-0">
          <div class="text-xl font-semibold text-slate-900 px-4 pt-4">Discovered Accounts</div>
          <v-card-text>
            <div v-if="brokerRuntime.accounts.length" class="grid gap-3">
              <div
                v-for="account in brokerRuntime.accounts"
                :key="account.accountId"
                class="rounded-3xl border border-slate-200 bg-white px-4 py-4"
              >
                <div class="flex items-center justify-between gap-3">
                  <div class="text-base font-semibold text-slate-900">{{ account.accountId }}</div>
                  <v-chip variant="outlined" size="small">{{ account.tradingEnvironment }}</v-chip>
                </div>
                <div class="mt-2 text-sm text-slate-600">
                  {{ account.accountType }}
                  <span v-if="account.simulatedAccountType"> / {{ account.simulatedAccountType }}</span>
                  <span v-if="account.securityFirm"> / {{ account.securityFirm }}</span>
                </div>
              </div>
            </div>
            <v-empty-state v-else text="当前未发现可用交易账户。若 OpenD 未启动或未登录，这里会保持为空。" />
          </v-card-text>
        </v-card>
      </v-window-item>

      <!-- Funds & Cash Tab -->
      <v-window-item value="funds">
        <v-card flat class="card-shell border-0">
          <div class="flex items-center justify-between gap-3 px-4 pt-4">
            <div class="text-xl font-semibold text-slate-900">Broker Funds</div>
            <v-chip :color="brokerFunds.connectivity === 'connected' ? 'success' : 'warning'" variant="outlined" size="small">
              {{ brokerFunds.connectivity.toUpperCase() }}
            </v-chip>
          </div>
          <v-card-text>
            <div v-if="brokerFunds.summary" class="grid gap-4 lg:grid-cols-[1.05fr_0.95fr]">
              <div class="grid gap-4">
                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="flex items-center justify-between gap-3">
                    <div>
                      <div class="text-base font-semibold text-slate-900">
                        {{ brokerFunds.summary.accountId }} / {{ brokerFunds.summary.tradingEnvironment }}
                      </div>
                      <div class="mt-1 text-sm text-slate-500">
                        {{ brokerFunds.summary.market }} / {{ brokerFunds.summary.currency ?? 'Base Currency' }}
                      </div>
                    </div>
                    <v-chip variant="outlined" size="small">资金快照</v-chip>
                  </div>
                  <div class="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                    <div class="rounded-2xl bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Total Assets</div>
                      <div class="mt-2 text-lg font-semibold text-slate-900">{{ brokerFunds.summary.totalAssets ?? 'N/A' }}</div>
                    </div>
                    <div class="rounded-2xl bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Cash</div>
                      <div class="mt-2 text-lg font-semibold text-slate-900">{{ brokerFunds.summary.cash ?? 'N/A' }}</div>
                    </div>
                    <div class="rounded-2xl bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Buying Power</div>
                      <div class="mt-2 text-lg font-semibold text-slate-900">{{ brokerFunds.summary.purchasingPower ?? 'N/A' }}</div>
                    </div>
                    <div class="rounded-2xl bg-slate-50 px-3 py-3">
                      <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Market Value</div>
                      <div class="mt-2 text-lg font-semibold text-slate-900">{{ brokerFunds.summary.marketValue ?? 'N/A' }}</div>
                    </div>
                  </div>
                </div>
              </div>

              <div class="grid gap-4">
                <div class="rounded-3xl border border-slate-200 bg-white px-4 py-4">
                  <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Broker Cash Flows</div>
                  <div v-if="brokerFunds.summary?.tradingEnvironment !== 'REAL'" class="mt-3 text-sm text-slate-500">
                    当前仅在真实交易环境下查询券商现金流水。
                  </div>
                  <div v-else-if="brokerCashFlows.lastError" class="mt-3 text-sm text-amber-700">
                    {{ brokerCashFlows.lastError }}
                  </div>
                  <div v-else-if="brokerCashFlows.cashFlows.length" class="mt-3 grid gap-2">
                    <div
                      v-for="cashFlow in brokerCashFlows.cashFlows.slice(0, 5)"
                      :key="cashFlow.cashFlowId"
                      class="rounded-2xl bg-slate-50 px-3 py-3"
                    >
                      <div class="flex items-center justify-between gap-3">
                        <div class="font-medium text-slate-900">{{ cashFlow.type ?? 'Unknown Flow' }}</div>
                        <div class="text-sm text-slate-600">{{ cashFlow.amount ?? 'N/A' }} {{ cashFlow.currency ?? '' }}</div>
                      </div>
                    </div>
                  </div>
                  <div v-else class="mt-3 text-sm text-slate-500">当前清算日没有可展示的现金流水。</div>
                </div>
              </div>
            </div>
          </v-card-text>
        </v-card>
      </v-window-item>

      <!-- Positions Tab -->
      <v-window-item value="positions">
        <v-card flat class="card-shell border-0">
          <div class="flex items-center justify-between gap-3 px-4 pt-4">
            <div class="text-xl font-semibold text-slate-900">Broker Positions</div>
            <v-chip :color="brokerPositions.connectivity === 'connected' ? 'success' : 'error'" variant="outlined" size="small">
              {{ brokerPositions.connectivity.toUpperCase() }}
            </v-chip>
          </div>
          <v-card-text>
            <div v-if="brokerPositions.positions.length" class="grid gap-3">
              <div
                v-for="position in brokerPositions.positions.slice(0, 5)"
                :key="`${position.accountId}-${position.symbol}`"
                class="rounded-3xl border border-slate-200 bg-white px-4 py-4"
              >
                <div class="flex items-center justify-between gap-3">
                  <div>
                    <div class="text-base font-semibold text-slate-900">{{ position.symbol }}</div>
                    <div class="mt-1 text-sm text-slate-500">{{ position.symbolName ?? 'Unnamed Security' }}</div>
                  </div>
                  <v-chip variant="outlined" size="small">{{ position.market }}</v-chip>
                </div>
                <div class="mt-4 grid gap-3 sm:grid-cols-3">
                  <div class="rounded-2xl bg-slate-50 px-3 py-3">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Quantity</div>
                    <div class="mt-2 text-lg font-semibold text-slate-900">{{ position.quantity }}</div>
                  </div>
                  <div class="rounded-2xl bg-slate-50 px-3 py-3">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Market Value</div>
                    <div class="mt-2 text-lg font-semibold text-slate-900">{{ position.marketValue }}</div>
                  </div>
                  <div class="rounded-2xl bg-slate-50 px-3 py-3">
                    <div class="text-xs uppercase tracking-[0.2em] text-slate-500">Unrealized PnL</div>
                    <div class="mt-2 text-lg font-semibold text-slate-900">{{ position.unrealizedPnl ?? 'N/A' }}</div>
                  </div>
                </div>
              </div>
            </div>
            <v-empty-state v-else text="当前没有持仓信息。" />
          </v-card-text>
        </v-card>
      </v-window-item>

      <!-- Orders Tab -->
      <v-window-item value="orders">
        <v-card flat class="card-shell border-0">
          <div class="flex items-center justify-between gap-3 px-4 pt-4">
            <div class="text-xl font-semibold text-slate-900">Recent Orders</div>
            <v-chip :color="brokerOrders.connectivity === 'connected' ? 'success' : 'error'" variant="outlined" size="small">
              {{ brokerOrders.connectivity.toUpperCase() }}
            </v-chip>
          </div>
          <v-card-text>
            <div v-if="brokerOrders.orders.length" class="grid gap-3">
              <div
                v-for="order in brokerOrders.orders.slice(0, 5)"
                :key="order.brokerOrderId"
                class="rounded-3xl border border-slate-200 bg-white px-4 py-4"
              >
                <div class="flex items-center justify-between gap-3">
                  <div>
                    <div class="text-base font-semibold text-slate-900">{{ order.symbol }}</div>
                    <div class="mt-1 text-sm text-slate-500">{{ order.side }} / {{ order.orderType }}</div>
                  </div>
                  <v-chip variant="outlined" size="small">{{ order.status }}</v-chip>
                </div>
                <div class="mt-3 text-xs text-slate-500">Submitted {{ order.submittedAt || 'N/A' }}</div>
              </div>
            </div>
            <v-empty-state v-else text="当前没有订单信息。" />
          </v-card-text>
        </v-card>
      </v-window-item>
    </v-window>
  </div>
</template>
