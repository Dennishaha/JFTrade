<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";

import PageHeader from "../components/PageHeader.vue";
import SectionHeader from "../components/SectionHeader.vue";
import { useConsoleData } from "../composables/useConsoleData";

const {
  brokerRuntime,
  brokerSettings,
  createManagedBrokerAccount,
  deleteManagedBrokerAccount,
  formatDateTime,
  futuOpenDHealth,
  futuOpenDInstallGuide,
  isLoading,
  loadSystemState,
  requestFutuOpenDManualRetry,
  saveBrokerIntegration,
  unsubscribeAllMarketData,
  updateManagedBrokerAccount,
} = useConsoleData();

const savingIntegration = ref(false);
const refreshingFutuConnection = ref(false);
const savingAccount = ref(false);
const cancellingSubscriptions = ref(false);
const deletingAccountId = ref("");
const editingAccountId = ref<string | null>(null);

const settingsMenu = [
  {
    index: "futu-integration",
    label: "Futu Integration",
    description: "配置 OpenD 连接参数与默认账号信息。",
  },
  {
    index: "managed-accounts",
    label: "Managed Accounts",
    description: "维护用于切换 Scope 的托管账号。",
  },
  {
    index: "account-discovery",
    label: "Account Discovery",
    description: "查看运行时探测到的 OpenD 账号并一键导入。",
  },
  {
    index: "plugin-manager",
    label: "OpenD Install",
    description: "跳转富途官方 OpenD 安装文档并引导连接配置。",
  },
] as const;

type MenuIndex = (typeof settingsMenu)[number]["index"];

const activeMenu = ref<MenuIndex>("futu-integration");

const activeMenuMeta = computed(
  () =>
    settingsMenu.find((entry) => entry.index === activeMenu.value) ??
    settingsMenu[0],
);

function handleMenuSelect(index: string): void {
  activeMenu.value = index as MenuIndex;
}

const futuBroker = computed(
  () =>
    brokerSettings.value.brokers.find(
      (broker) => broker.descriptor.id === "futu",
    ) ?? null,
);

const settingsHeaderStats = computed(() => [
  {
    label: "Managed brokers",
    value: brokerSettings.value.brokers.length,
  },
  {
    label: "Managed accounts",
    value: brokerSettings.value.accounts.length,
  },
  {
    label: "Runtime accounts",
    value: brokerRuntime.value.accounts.length,
  },
]);

type StatusTagType = "success" | "warning" | "danger" | "info";

const futuConnectionTagType = computed<StatusTagType>(() => {
  switch (brokerRuntime.value.session.connectivity) {
    case "connected":
      return "success";
    case "degraded":
      return "warning";
    case "disconnected":
      return "danger";
    default:
      return "info";
  }
});

const futuConnectionLabel = computed(() => {
  switch (brokerRuntime.value.session.connectivity) {
    case "connected":
      return "已连接";
    case "degraded":
      return "部分可用";
    case "disconnected":
      return "未连接";
    default:
      return "未知";
  }
});

const futuConnectionTarget = computed(
  () =>
    `${brokerRuntime.value.session.connection.host}:${brokerRuntime.value.session.connection.port}`,
);

const futuConnectionCheckedAt = computed(() =>
  brokerRuntime.value.session.checkedAt === ""
    ? "尚未检测"
    : formatDateTime(brokerRuntime.value.session.checkedAt),
);

const futuQuoteLoginLabel = computed(() => {
  const loggedIn = brokerRuntime.value.session.globalState?.quoteLoggedIn;
  return loggedIn == null ? "未知" : loggedIn ? "已登录" : "未登录";
});

const futuTradeLoginLabel = computed(() => {
  const loggedIn = brokerRuntime.value.session.globalState?.tradeLoggedIn;
  return loggedIn == null ? "未知" : loggedIn ? "已登录" : "未登录";
});

const websocketPasswordFormStatus = computed(() =>
  integrationForm.websocketKey.trim().length > 0
    ? "当前表单已填写，保存后生效"
    : "当前表单未填写；OpenD 启用 WebSocket 密码时必须填写",
);

const futuOpenDManualRetryRequired = computed(
  () => futuOpenDHealth.value.diagnosis.manualRetryRequired,
);

const futuOpenDRestartRecommended = computed(
  () => futuOpenDHealth.value.diagnosis.restartOpenDRecommended,
);

const futuOpenDTopClientSummary = computed(() =>
  futuOpenDHealth.value.localSocketDiagnostics.topClientProcesses
    .map(
      (item) =>
        `${item.processName}(${item.pid}) x${item.establishedConnections}`,
    )
    .join(" / "),
);

const integrationForm = reactive({
  enabled: true,
  host: "127.0.0.1",
  apiPort: 11110,
  websocketPort: 11111,
  maxWebSocketConnections: 20,
  useEncryption: false,
  websocketKey: "",
  tradeMarket: "HK",
  securityFirm: "FUTUSECURITIES",
});

const accountForm = reactive({
  brokerId: "futu",
  accountId: "",
  displayName: "",
  tradingEnvironment: "SIMULATE",
  market: "HK",
  securityFirm: "FUTUSECURITIES",
  enabled: true,
});

watch(
  futuBroker,
  (broker) => {
    const source = broker?.integration?.config ?? broker?.defaults;

    if (source != null) {
      integrationForm.enabled = broker?.integration?.enabled ?? true;
      integrationForm.host = source.host;
      integrationForm.apiPort =
        "apiPort" in source && typeof source.apiPort === "number"
          ? source.apiPort
          : 11110;
      integrationForm.websocketPort =
        "websocketPort" in source && typeof source.websocketPort === "number"
          ? source.websocketPort
          : 11111;
      integrationForm.maxWebSocketConnections =
        "maxWebSocketConnections" in source &&
        typeof source.maxWebSocketConnections === "number"
          ? source.maxWebSocketConnections
          : 20;
      integrationForm.useEncryption = source.useEncryption;
      integrationForm.websocketKey = source.websocketKey;
      integrationForm.tradeMarket = source.tradeMarket;
      integrationForm.securityFirm = source.securityFirm;
    }

    if (editingAccountId.value == null && accountForm.accountId === "") {
      resetAccountForm();
    }
  },
  { immediate: true },
);

function resetAccountForm(): void {
  editingAccountId.value = null;
  accountForm.brokerId = futuBroker.value?.descriptor.id ?? "futu";
  accountForm.accountId = "";
  accountForm.displayName = "";
  accountForm.tradingEnvironment = "SIMULATE";
  accountForm.market = integrationForm.tradeMarket;
  accountForm.securityFirm = integrationForm.securityFirm;
  accountForm.enabled = true;
}

function populateAccountForm(
  account: (typeof brokerSettings.value.accounts)[number],
): void {
  editingAccountId.value = account.id;
  accountForm.brokerId = account.brokerId;
  accountForm.accountId = account.accountId;
  accountForm.displayName = account.displayName;
  accountForm.tradingEnvironment = account.tradingEnvironment;
  accountForm.market = account.market;
  accountForm.securityFirm =
    account.securityFirm ?? integrationForm.securityFirm;
  accountForm.enabled = account.enabled;
}

function importRuntimeAccount(
  account: (typeof brokerRuntime.value.accounts)[number],
): void {
  editingAccountId.value = null;
  accountForm.brokerId = futuBroker.value?.descriptor.id ?? "futu";
  accountForm.accountId = account.accountId;
  accountForm.displayName = account.accountId;
  accountForm.tradingEnvironment = account.tradingEnvironment;
  accountForm.market =
    account.marketAuthorities[0] ?? integrationForm.tradeMarket;
  accountForm.securityFirm =
    account.securityFirm ?? integrationForm.securityFirm;
  accountForm.enabled = true;
}

async function submitIntegration(): Promise<void> {
  savingIntegration.value = true;

  try {
    await saveBrokerIntegration("futu", {
      enabled: integrationForm.enabled,
      config: {
        type: "futu",
        host: integrationForm.host,
        apiPort: integrationForm.apiPort,
        websocketPort: integrationForm.websocketPort,
        maxWebSocketConnections: integrationForm.maxWebSocketConnections,
        useEncryption: integrationForm.useEncryption,
        websocketKey: integrationForm.websocketKey,
        tradeMarket: integrationForm.tradeMarket,
        securityFirm: integrationForm.securityFirm,
      },
    });
  } finally {
    savingIntegration.value = false;
  }
}

async function refreshFutuConnection(): Promise<void> {
  refreshingFutuConnection.value = true;

  try {
    await loadSystemState();
  } finally {
    refreshingFutuConnection.value = false;
  }
}

async function manualRetryFutuConnection(): Promise<void> {
  refreshingFutuConnection.value = true;

  try {
    await requestFutuOpenDManualRetry();
  } finally {
    refreshingFutuConnection.value = false;
  }
}

async function cancelAllMarketDataSubscriptions(): Promise<void> {
  cancellingSubscriptions.value = true;

  try {
    await unsubscribeAllMarketData();
  } finally {
    cancellingSubscriptions.value = false;
  }
}

async function submitAccount(): Promise<void> {
  savingAccount.value = true;

  try {
    const payload = {
      brokerId: accountForm.brokerId,
      accountId: accountForm.accountId,
      displayName: accountForm.displayName,
      tradingEnvironment: accountForm.tradingEnvironment,
      market: accountForm.market,
      securityFirm: accountForm.securityFirm,
      enabled: accountForm.enabled,
    };

    if (editingAccountId.value == null) {
      await createManagedBrokerAccount(payload);
    } else {
      await updateManagedBrokerAccount(editingAccountId.value, payload);
    }

    resetAccountForm();
  } finally {
    savingAccount.value = false;
  }
}

async function removeAccount(accountId: string): Promise<void> {
  deletingAccountId.value = accountId;

  try {
    await deleteManagedBrokerAccount(accountId);

    if (editingAccountId.value === accountId) {
      resetAccountForm();
    }
  } finally {
    deletingAccountId.value = "";
  }
}
</script>

<template>
  <div class="grid gap-6">
    <PageHeader
      eyebrow="Control plane"
      title="Settings / Configuration"
      description="统一维护券商接入配置与账号资料；顶部 Scope 会基于这里的账号清单切换查询上下文。"
      :stats="settingsHeaderStats"
    />

    <el-breadcrumb separator="/" class="text-sm text-slate-500">
      <el-breadcrumb-item :to="{ path: '/settings' }">Console</el-breadcrumb-item>
      <el-breadcrumb-item>Settings</el-breadcrumb-item>
      <el-breadcrumb-item>{{ activeMenuMeta.label }}</el-breadcrumb-item>
    </el-breadcrumb>

    <el-page-header
      class="settings-page-header"
      :icon="null"
      @back="activeMenu = settingsMenu[0].index"
    >
      <template #content>
        <div>
          <div class="text-lg font-semibold text-slate-900">
            {{ activeMenuMeta.label }}
          </div>
          <div class="mt-1 text-xs text-slate-500">
            {{ activeMenuMeta.description }}
          </div>
        </div>
      </template>
      <template #extra>
        <el-tag effect="plain">{{ activeMenu }}</el-tag>
      </template>
    </el-page-header>

    <section class="grid gap-5 lg:grid-cols-[220px_1fr]">
      <el-menu
        :default-active="activeMenu"
        class="rounded-lg border border-slate-200"
        @select="handleMenuSelect"
      >
        <el-menu-item
          v-for="entry in settingsMenu"
          :key="entry.index"
          :index="entry.index"
        >
          <span>{{ entry.label }}</span>
        </el-menu-item>
      </el-menu>

      <div class="grid gap-6">
        <!-- Futu Integration Section -->
        <div v-show="activeMenu === 'futu-integration'" class="grid gap-6">
          <div class="settings-panel">
            <SectionHeader
              title="Futu Integration"
              description="配置 OpenD 连接参数与默认账号信息。"
            />

            <div class="mt-4 rounded-2xl border border-slate-200 bg-white px-4 py-4">
              <div class="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="text-sm font-semibold text-slate-900">OpenD 连接状态</span>
                    <el-tag :type="futuConnectionTagType" effect="dark">
                      {{ futuConnectionLabel }}
                    </el-tag>
                  </div>
                  <p class="mt-2 text-sm leading-6 text-slate-600">
                    当前检测目标：WebSocket {{ futuConnectionTarget }}；检测时间：
                    {{ futuConnectionCheckedAt }}。保存参数后会自动重新检测，也可以手动刷新。
                  </p>
                </div>
                <div class="flex flex-wrap gap-2">
                  <el-button
                    :loading="refreshingFutuConnection || isLoading"
                    type="primary"
                    plain
                    @click="refreshFutuConnection"
                  >
                    刷新连接状态
                  </el-button>
                  <el-button
                    :loading="refreshingFutuConnection || isLoading"
                    type="danger"
                    plain
                    @click="manualRetryFutuConnection"
                  >
                    手动重试 OpenD
                  </el-button>
                </div>
              </div>

              <div class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                <div class="rounded-xl bg-slate-50 px-3 py-3">
                  <div class="text-xs uppercase tracking-wide text-slate-500">Quote Login</div>
                  <div class="mt-1 text-sm font-semibold text-slate-900">
                    {{ futuQuoteLoginLabel }}
                  </div>
                </div>
                <div class="rounded-xl bg-slate-50 px-3 py-3">
                  <div class="text-xs uppercase tracking-wide text-slate-500">Trade Login</div>
                  <div class="mt-1 text-sm font-semibold text-slate-900">
                    {{ futuTradeLoginLabel }}
                  </div>
                </div>
                <div class="rounded-xl bg-slate-50 px-3 py-3">
                  <div class="text-xs uppercase tracking-wide text-slate-500">Program Status</div>
                  <div class="mt-1 text-sm font-semibold text-slate-900">
                    {{ brokerRuntime.session.globalState?.programStatus ?? "Unavailable" }}
                  </div>
                </div>
                <div class="rounded-xl bg-slate-50 px-3 py-3">
                  <div class="text-xs uppercase tracking-wide text-slate-500">Password / Key</div>
                  <div class="mt-1 text-sm font-semibold text-slate-900">
                    {{ websocketPasswordFormStatus }}
                  </div>
                </div>
              </div>

              <el-alert
                v-if="futuOpenDManualRetryRequired"
                class="mt-4"
                type="error"
                show-icon
                :closable="false"
                title="OpenD 自动重试已暂停"
              >
                <p class="leading-6">
                  {{ futuOpenDHealth.diagnosis.summary }}
                </p>
                <p class="mt-2 leading-6">
                  建议先检查并重启 OpenD，再点击上方“手动重试 OpenD”。
                </p>
                <p
                  v-if="futuOpenDHealth.localSocketDiagnostics.websocketEstablishedConnections > 0"
                  class="mt-2 text-sm leading-6"
                >
                  当前检测到
                  {{ futuOpenDHealth.localSocketDiagnostics.websocketEstablishedConnections }}
                  条本地已建立 WebSocket 连接
                  <span v-if="futuOpenDTopClientSummary">（{{ futuOpenDTopClientSummary }}）</span>
                  。
                </p>
              </el-alert>

              <el-alert
                v-else-if="brokerRuntime.session.lastError"
                class="mt-4"
                type="error"
                show-icon
                :closable="false"
                title="OpenD 连接错误"
              >
                <p class="leading-6">
                  {{ brokerRuntime.session.lastError }}
                </p>
              </el-alert>

              <el-alert
                v-if="!futuOpenDManualRetryRequired && futuOpenDRestartRecommended"
                class="mt-4"
                type="warning"
                show-icon
                :closable="false"
                title="建议重启 OpenD 后再手动重试"
              >
                <p class="leading-6">
                  {{ futuOpenDHealth.diagnosis.summary }}
                </p>
              </el-alert>

              <el-alert
                v-else-if="brokerRuntime.session.connectivity === 'connected'"
                class="mt-4"
                type="success"
                show-icon
                :closable="false"
              >
                <p class="leading-6">
                  <span class="font-semibold">OpenD WebSocket 已连接。</span>
                  当前参数已通过运行时检测，可以继续发现账号、查询行情或进行后续操作。
                </p>
              </el-alert>
            </div>

            <el-form label-position="top" class="mt-4 grid gap-4">
              <el-form-item>
                <el-switch v-model="integrationForm.enabled" active-text="启用富途接入配置" />
              </el-form-item>

              <div class="grid gap-4 md:grid-cols-2">
                <el-form-item label="OpenD Host">
                  <el-input v-model="integrationForm.host" />
                </el-form-item>
                <el-form-item label="OpenD API Port">
                  <el-input-number
                    v-model="integrationForm.apiPort"
                    :min="1"
                    :max="65535"
                    class="!w-full"
                    controls-position="right"
                  />
                </el-form-item>
                <el-form-item label="OpenD WebSocket Port">
                  <template #default>
                    <el-input-number
                      v-model="integrationForm.websocketPort"
                      :min="1"
                      :max="65535"
                      class="!w-full"
                      controls-position="right"
                    />
                    <div class="mt-1 text-xs text-amber-700">
                      JFTrade 的 JavaScript 富途接入使用 WebSocket 端口；请先在 OpenD 中开启 WebSocket。
                    </div>
                  </template>
                </el-form-item>
                <el-form-item label="OpenD WebSocket 并发上限">
                  <template #default>
                    <el-input-number
                      v-model="integrationForm.maxWebSocketConnections"
                      :min="1"
                      :max="128"
                      class="!w-full"
                      controls-position="right"
                    />
                    <div class="mt-1 text-xs text-slate-500">
                      默认 20。JFTrade 会复用请求级 WebSocket 连接池，并限制同时连接 OpenD 的 client 数量，避免反复登录或并发查询耗尽 OpenD 连接。
                    </div>
                  </template>
                </el-form-item>
                <el-form-item label="默认账号市场">
                  <template #default>
                    <el-input v-model="integrationForm.tradeMarket" />
                    <div class="mt-1 text-xs text-slate-500">
                      仅用于手工创建账号时的默认值，不会限制 OpenD 可查询行情或账户授权市场。
                    </div>
                  </template>
                </el-form-item>
                <el-form-item label="默认券商标识">
                  <template #default>
                    <el-input v-model="integrationForm.securityFirm" />
                    <div class="mt-1 text-xs text-slate-500">
                      仅作为手工账号默认值；从 OpenD 导入账号时优先使用运行时探测到的 security firm。
                    </div>
                  </template>
                </el-form-item>
              </div>

              <div class="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
                请在 OpenD GUI 中确认 WebSocket 已开启；如果配置了 WebSocket 密码，请把这里的
                WebSocket Password / Key 与 OpenD GUI 或命令行版 FutuOpenD.xml（或
                <code>-cfg_file</code> 指定的参数文件）保持一致。API Port 主要用于记录当前 OpenD 的
                TCP API 监听配置。
              </div>

              <el-form-item label="WebSocket Password / Key">
                <template #default>
                  <el-input
                    v-model="integrationForm.websocketKey"
                    show-password
                    type="password"
                    placeholder="未启用密码时可留空"
                  />
                  <div class="mt-1 text-xs text-slate-500">
                    如果 OpenD GUI 或命令行版 FutuOpenD.xml / <code>-cfg_file</code>
                    参数文件配置了 <code>websocket_key_md5</code>，请在这里填写对应的明文密码；也可通过
                    <code>JFTRADE_FUTU_WEBSOCKET_KEY</code> 配置。不要填写 32 位 MD5 密文。
                  </div>
                </template>
              </el-form-item>

              <el-form-item>
                <el-switch v-model="integrationForm.useEncryption" active-text="启用加密连接" />
              </el-form-item>

              <div class="flex flex-wrap justify-end gap-3">
                <el-button
                  :loading="cancellingSubscriptions"
                  type="danger"
                  plain
                  @click="cancelAllMarketDataSubscriptions"
                >
                  取消全部实时行情订阅
                </el-button>
                <el-button :loading="savingIntegration" type="primary" @click="submitIntegration">
                  保存富途配置
                </el-button>
              </div>
            </el-form>
          </div>
        </div>

        <!-- Managed Accounts Section -->
        <div v-show="activeMenu === 'managed-accounts'" class="grid gap-6">
          <div class="settings-panel">
            <SectionHeader title="Managed accounts" description="这些账号会出现在顶部 Scope 切换器内。" />

            <div class="mt-4 grid gap-3">
              <template v-if="brokerSettings.accounts.length">
                <div
                  v-for="account in brokerSettings.accounts"
                  :key="account.id"
                  class="rounded-2xl border border-slate-200 bg-white px-4 py-3"
                >
                  <div class="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <div class="text-base font-semibold text-slate-900">{{ account.displayName }}</div>
                      <div class="mt-1 text-xs text-slate-500">
                        {{ account.brokerId }} / {{ account.accountId }} / {{ account.tradingEnvironment }} / {{ account.market }}
                      </div>
                    </div>
                    <div class="flex items-center gap-2">
                      <el-tag :type="account.enabled ? 'success' : 'info'" effect="plain">
                        {{ account.enabled ? "ENABLED" : "DISABLED" }}
                      </el-tag>
                      <el-button text type="primary" @click="populateAccountForm(account)">编辑</el-button>
                      <el-button
                        text
                        type="danger"
                        :loading="deletingAccountId === account.id"
                        @click="removeAccount(account.id)"
                      >
                        删除
                      </el-button>
                    </div>
                  </div>
                </div>
              </template>
              <el-empty
                v-else
                description="当前还没有保存任何 broker account，顶部 Scope 会回退到运行时探测出的账号。"
              />
            </div>
          </div>

          <div class="settings-panel">
            <SectionHeader
              :title="editingAccountId ? 'Edit account' : 'Create account'"
              :description="editingAccountId ? undefined : '手工创建或导入一个新的托管账号。'"
            >
              <template #extra>
                <el-button text type="primary" @click="resetAccountForm">重置</el-button>
              </template>
            </SectionHeader>

            <el-form label-position="top" class="mt-4 grid gap-4">
              <div class="grid gap-4 md:grid-cols-2">
                <el-form-item label="Broker ID">
                  <el-input v-model="accountForm.brokerId" />
                </el-form-item>
                <el-form-item label="Account ID">
                  <el-input v-model="accountForm.accountId" />
                </el-form-item>
                <el-form-item label="Display Name（可选）">
                  <el-input v-model="accountForm.displayName" />
                </el-form-item>
                <el-form-item label="Trading Environment">
                  <el-select v-model="accountForm.tradingEnvironment" class="!w-full">
                    <el-option label="SIMULATE" value="SIMULATE" />
                    <el-option label="REAL" value="REAL" />
                  </el-select>
                </el-form-item>
                <el-form-item label="Market">
                  <el-input v-model="accountForm.market" />
                </el-form-item>
                <el-form-item label="Security Firm（可选）">
                  <el-input v-model="accountForm.securityFirm" />
                </el-form-item>
              </div>

              <el-form-item>
                <el-switch v-model="accountForm.enabled" active-text="启用该账号作为前端可切换 Scope" />
              </el-form-item>

              <div class="flex justify-end">
                <el-button :loading="savingAccount" type="primary" @click="submitAccount">
                  {{ editingAccountId ? "更新账号" : "新增账号" }}
                </el-button>
              </div>
            </el-form>
          </div>
        </div>

        <!-- Account Discovery Section -->
        <div v-show="activeMenu === 'account-discovery'" class="grid gap-6">
          <div class="settings-panel">
            <SectionHeader title="OpenD discovered accounts" description="OpenD 实时探测到的账号；可一键导入到托管列表。" />

            <div class="mt-4 grid gap-3">
              <template v-if="brokerRuntime.accounts.length">
                <div
                  v-for="account in brokerRuntime.accounts"
                  :key="`${account.tradingEnvironment}-${account.accountId}`"
                  class="rounded-2xl border border-slate-200 bg-white px-4 py-3"
                >
                  <div class="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <div class="text-base font-semibold text-slate-900">{{ account.accountId }}</div>
                      <div class="mt-1 text-xs text-slate-500">
                        {{ account.tradingEnvironment }} / {{ account.accountType }} / {{ account.marketAuthorities.join(", ") || "N/A" }}
                      </div>
                    </div>
                    <el-button text type="primary" @click="importRuntimeAccount(account)">
                      导入到账号管理
                    </el-button>
                  </div>
                </div>
              </template>
              <el-empty
                v-else
                description="当前没有从运行时探测到账号。OpenD 未登录时，这里会为空。"
              />
            </div>
          </div>
        </div>

        <!-- OpenD Install Section -->
        <div v-show="activeMenu === 'plugin-manager'" class="grid gap-6">
          <div class="settings-panel">
            <SectionHeader
              title="OpenD install guide"
              description="JFTrade 不安装 OpenD，只提供富途官方图形交互版与命令行版入口；安装完成后请回到 Futu Integration 填写连接信息。"
            >
              <template #extra>
                <el-tag effect="plain">Official docs</el-tag>
              </template>
            </SectionHeader>

            <div class="mt-4 grid gap-4">
              <div class="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600">
                <div class="font-semibold text-slate-900">
                  {{ futuOpenDInstallGuide.title || "Futu OpenD 安装引导" }}
                </div>
                <p class="mt-2 leading-6">
                  {{ futuOpenDInstallGuide.description }}
                </p>
              </div>

              <div class="grid gap-3 lg:grid-cols-2">
                <div
                  v-for="option in futuOpenDInstallGuide.options"
                  :key="option.id"
                  class="rounded-2xl border border-slate-200 bg-white px-4 py-4"
                >
                  <div class="flex items-start justify-between gap-3">
                    <div>
                      <div class="text-base font-semibold text-slate-900">
                        {{ option.label }}
                      </div>
                      <div class="mt-1 text-xs text-slate-500">
                        {{ option.id === "gui" ? "GUI / Desktop" : "CLI / Server" }}
                      </div>
                    </div>
                    <el-tag
                      :type="option.recommended ? 'success' : 'info'"
                      effect="plain"
                    >
                      {{ option.recommended ? "推荐" : "可选" }}
                    </el-tag>
                  </div>

                  <p class="mt-3 text-sm leading-6 text-slate-600">
                    {{ option.description }}
                  </p>

                  <div class="mt-4 flex flex-wrap justify-end gap-2">
                    <el-button
                      type="primary"
                      tag="a"
                      :href="option.url"
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      打开官方文档
                    </el-button>
                  </div>
                </div>
              </div>

              <div class="rounded-2xl border border-slate-200 bg-white px-4 py-4">
                <div class="text-sm font-semibold text-slate-900">安装后设置</div>
                <p class="mt-2 text-sm leading-6 text-slate-600">
                  默认 Host 为 {{ futuOpenDInstallGuide.settings.host }}，API Port 为
                  {{ futuOpenDInstallGuide.settings.apiPort }}，WebSocket Port 为
                  {{ futuOpenDInstallGuide.settings.websocketPort }}，加密连接：
                  {{ futuOpenDInstallGuide.settings.useEncryption ? "开启" : "关闭" }}。
                  安装并登录 OpenD 后，请先确认已开启 WebSocket；若 OpenD 配置了 WebSocket
                  密码，请在 Futu Integration 的 WebSocket Password / Key 中填写同一明文密码。
                  命令行版 OpenD 可在 FutuOpenD.xml 或 <code>-cfg_file</code> 指定的参数文件中配置
                  <code>websocket_key_md5</code>。
                </p>
                <ol class="mt-3 list-decimal space-y-1 pl-5 text-sm text-slate-600">
                  <li
                    v-for="step in futuOpenDInstallGuide.nextSteps"
                    :key="step"
                  >
                    {{ step }}
                  </li>
                </ol>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.settings-panel {
  border-radius: 1.25rem;
  border: 1px solid rgb(226 232 240);
  background: #fff;
  padding: 1.25rem 1.5rem;
}

.settings-page-header :deep(.el-page-header__left) {
  display: none;
}
</style>
