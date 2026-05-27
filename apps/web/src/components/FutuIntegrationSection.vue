<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";

import {
  formatConnectivityLabel,
  formatDateTime,
  formatFutuProgramStatusLabel,
} from "../composables/consoleDataFormatting";
import SectionHeader from "./SectionHeader.vue";
import { useConsoleData } from "../composables/useConsoleData";

const {
  brokerRuntime,
  brokerSettings,
  futuOpenDHealth,
  isLoading,
  loadSystemState,
  requestFutuOpenDManualRetry,
  saveBrokerIntegration,
  unsubscribeAllMarketData,
} = useConsoleData();

const savingIntegration = ref(false);
const refreshingFutuConnection = ref(false);
const cancellingSubscriptions = ref(false);

type StatusTagType = "success" | "warning" | "danger" | "info";

const futuBroker = computed(
  () =>
    brokerSettings.value.brokers.find(
      (broker) => broker.descriptor.id === "futu",
    ) ?? null,
);

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

const futuConnectionLabel = computed(() =>
  formatConnectivityLabel(brokerRuntime.value.session.connectivity),
);

const futuConnectionTarget = computed(
  () =>
    `${brokerRuntime.value.session.connection.host}:${brokerRuntime.value.session.connection.apiPort}`,
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

watch(
  futuBroker,
  (broker) => {
    const source = broker?.integration?.config ?? broker?.defaults;
    if (source == null) {
      return;
    }

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
  },
  { immediate: true },
);

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
</script>

<template>
  <div class="grid gap-6">
    <div class="settings-panel">
      <SectionHeader
        title="富途接入"
        description="配置 OpenD 连接参数与默认账号信息。"
      />

      <div class="mt-4 rounded-2xl border border-slate-200 bg-white px-4 py-4">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-sm font-semibold text-slate-900">OpenD 连接状态</span>
              <v-chip :color="futuConnectionTagType === 'danger' ? 'error' : futuConnectionTagType" variant="tonal" size="small">
                {{ futuConnectionLabel }}
              </v-chip>
            </div>
            <p class="mt-2 text-sm leading-6 text-slate-600">
              当前检测目标：WebSocket {{ futuConnectionTarget }}；检测时间：
              {{ futuConnectionCheckedAt }}。保存参数后会自动重新检测，也可以手动刷新。
            </p>
          </div>
          <div class="flex flex-wrap gap-2">
            <v-btn
              :loading="refreshingFutuConnection || isLoading"
              variant="outlined"
              color="primary"
              @click="refreshFutuConnection"
            >
              刷新连接状态
            </v-btn>
            <v-btn
              :loading="refreshingFutuConnection || isLoading"
              variant="outlined"
              color="error"
              @click="manualRetryFutuConnection"
            >
              手动重试 OpenD
            </v-btn>
          </div>
        </div>

        <div class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <div class="rounded-xl bg-slate-50 px-3 py-3">
            <div class="text-xs text-slate-500">行情登录</div>
            <div class="mt-1 text-sm font-semibold text-slate-900">
              {{ futuQuoteLoginLabel }}
            </div>
          </div>
          <div class="rounded-xl bg-slate-50 px-3 py-3">
            <div class="text-xs text-slate-500">交易登录</div>
            <div class="mt-1 text-sm font-semibold text-slate-900">
              {{ futuTradeLoginLabel }}
            </div>
          </div>
          <div class="rounded-xl bg-slate-50 px-3 py-3">
            <div class="text-xs text-slate-500">程序状态</div>
            <div class="mt-1 text-sm font-semibold text-slate-900">
              {{ formatFutuProgramStatusLabel(brokerRuntime.session.globalState?.programStatus) }}
            </div>
          </div>
          <div class="rounded-xl bg-slate-50 px-3 py-3">
            <div class="text-xs text-slate-500">密码 / 密钥</div>
            <div class="mt-1 text-sm font-semibold text-slate-900">
              {{ websocketPasswordFormStatus }}
            </div>
          </div>
        </div>

        <v-alert
          v-if="futuOpenDManualRetryRequired"
          class="mt-4"
          type="error"
          :closable="false"
          title="OpenD 自动重试已暂停"
        >
          <p class="leading-6">
            {{ futuOpenDHealth.diagnosis.summary }}
          </p>
          <p class="mt-2 leading-6">
            建议先检查并重启 OpenD，再点击上方"手动重试 OpenD"。
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
        </v-alert>

        <v-alert
          v-else-if="brokerRuntime.session.lastError"
          class="mt-4"
          type="error"
          :closable="false"
          title="OpenD 连接错误"
        >
          <p class="leading-6">
            {{ brokerRuntime.session.lastError }}
          </p>
        </v-alert>

        <v-alert
          v-if="!futuOpenDManualRetryRequired && futuOpenDRestartRecommended"
          class="mt-4"
          type="warning"
          :closable="false"
          title="建议重启 OpenD 后再手动重试"
        >
          <p class="leading-6">
            {{ futuOpenDHealth.diagnosis.summary }}
          </p>
        </v-alert>

        <v-alert
          v-else-if="brokerRuntime.session.connectivity === 'connected'"
          class="mt-4"
          type="success"
          :closable="false"
        >
          <p class="leading-6">
            <span class="font-semibold">OpenD WebSocket 已连接。</span>
            当前参数已通过运行时检测，可以继续发现账号、查询行情或进行后续操作。
          </p>
        </v-alert>
      </div>

      <div class="mt-4 grid gap-4">
        <div class="grid gap-1">
          <v-switch v-model="integrationForm.enabled" color="indigo" label="启用富途接入配置" hide-details />
        </div>

        <div class="grid gap-4 md:grid-cols-2">
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">OpenD 主机</label>
            <v-text-field v-model="integrationForm.host" density="compact" variant="outlined" />
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">OpenD API 端口</label>
            <v-text-field
              v-model.number="integrationForm.apiPort"
              type="number"
              :min="1"
              :max="65535"
              density="compact"
              variant="outlined"
            />
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">OpenD WebSocket 端口</label>
            <v-text-field
              v-model.number="integrationForm.websocketPort"
              type="number"
              :min="1"
              :max="65535"
              density="compact"
              variant="outlined"
            />
            <div class="mt-1 text-xs text-amber-700">
              JFTrade 的 JavaScript 富途接入使用 WebSocket 端口；请先在 OpenD 中开启 WebSocket。
            </div>
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">OpenD WebSocket 并发上限</label>
            <v-text-field
              v-model.number="integrationForm.maxWebSocketConnections"
              type="number"
              :min="1"
              :max="128"
              density="compact"
              variant="outlined"
            />
            <div class="mt-1 text-xs text-slate-500">
              默认 20。JFTrade 会复用请求级 WebSocket 连接池，并限制同时连接 OpenD 的客户端数量，避免反复登录或并发查询耗尽 OpenD 连接。
            </div>
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">默认账号市场</label>
            <v-text-field v-model="integrationForm.tradeMarket" density="compact" variant="outlined" />
            <div class="mt-1 text-xs text-slate-500">
              仅用于手工创建账号时的默认值，不会限制 OpenD 可查询行情或账户授权市场。
            </div>
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">默认券商标识</label>
            <v-text-field v-model="integrationForm.securityFirm" density="compact" variant="outlined" />
            <div class="mt-1 text-xs text-slate-500">
              仅作为手工账号默认值；从 OpenD 导入账号时优先使用运行时探测到的券商机构。
            </div>
          </div>
        </div>

        <div class="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          请在 OpenD GUI 中确认 WebSocket 已开启；如果配置了 WebSocket 密码，请把这里的
          WebSocket 密码 / 密钥与 OpenD 图形界面或命令行版 FutuOpenD.xml（或
          <code>-cfg_file</code> 指定的参数文件）保持一致。API 端口主要用于记录当前 OpenD 的
          TCP API 监听配置。
        </div>

        <div class="grid gap-1">
          <label class="text-sm font-medium text-slate-700">WebSocket 密码 / 密钥</label>
          <v-text-field
            v-model="integrationForm.websocketKey"
            type="password"
            placeholder="未启用密码时可留空"
            density="compact"
            variant="outlined"
          />
          <div class="mt-1 text-xs text-slate-500">
            如果 OpenD GUI 或命令行版 FutuOpenD.xml / <code>-cfg_file</code>
            参数文件配置了 <code>websocket_key_md5</code>，请在这里填写对应的明文密码；也可通过
            <code>JFTRADE_FUTU_WEBSOCKET_KEY</code> 配置。不要填写 32 位 MD5 密文。
          </div>
        </div>

        <div class="grid gap-1">
          <v-switch v-model="integrationForm.useEncryption" color="indigo" label="启用加密连接" hide-details />
        </div>

        <div class="flex flex-wrap justify-end gap-3">
          <v-btn
            :loading="cancellingSubscriptions"
            variant="outlined"
            color="error"
            @click="cancelAllMarketDataSubscriptions"
          >
            取消全部实时行情订阅
          </v-btn>
          <v-btn :loading="savingIntegration" color="primary" @click="submitIntegration">
            保存富途配置
          </v-btn>
        </div>
      </div>
    </div>
  </div>
</template>