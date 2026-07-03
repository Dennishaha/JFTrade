<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted } from "vue";
import { useRoute, useRouter } from "vue-router";

import FutuIntegrationSection from "../components/FutuIntegrationSection.vue";
import RuntimeDependenciesSection from "../components/RuntimeDependenciesSection.vue";
import SettingsAccountDiscoverySection from "../components/SettingsAccountDiscoverySection.vue";
import SettingsAppearanceSection from "../components/SettingsAppearanceSection.vue";
import SettingsExchangeCalendarSection from "../components/SettingsExchangeCalendarSection.vue";
import SettingsManagedAccountsSection from "../components/SettingsManagedAccountsSection.vue";
import SettingsPineWorkerSection from "../components/SettingsPineWorkerSection.vue";
import SettingsSecuritySection from "../components/SettingsSecuritySection.vue";
import { createSettingsManagedAccountsController } from "../composables/settingsManagedAccounts";
import { readLocalStorage, writeLocalStorage } from "../composables/safeStorage";
import { useConsoleData } from "../composables/useConsoleData";

const route = useRoute();
const router = useRouter();
const SettingsADKSection = defineAsyncComponent(() => import("../components/SettingsADKSection.vue"));
const SettingsDataManagementSection = defineAsyncComponent(() => import("../components/SettingsDataMigrationSection.vue"));

const SETTINGS_LAST_KEY = "jft.settings.section";

const {
  brokerRuntime,
  brokerSettings,
  createManagedBrokerAccount,
  deleteManagedBrokerAccount,
  updateManagedBrokerAccount,
} = useConsoleData();

const settingsMenu = [
  {
    index: "runtime-dependencies",
    label: "依赖项管理",
    description: "检查并配置策略运行需要的本机依赖。",
  },
  {
    index: "futu-integration",
    label: "富途接入",
    description: "配置 OpenD 连接参数与默认账户信息。",
  },
  {
    index: "managed-accounts",
    label: "托管账户",
    description: "维护用于切换账户范围的托管账户列表。",
  },
  {
    index: "account-discovery",
    label: "账户发现",
    description: "查看运行时发现到的 OpenD 账户，并一键导入。",
  },
  {
    index: "appearance",
    label: "界面外观",
    description: "设置 K 线、价格涨跌与买卖颜色。",
  },
  {
    index: "exchange-calendars",
    label: "交易所日历",
    description: "查看外部交易所日历源状态与错误推送。",
  },
  {
    index: "security",
    label: "安全",
    description: "配置管理员认证与访问保护。",
  },
  {
    index: "pine-worker",
    label: "PineTS Worker",
    description: "配置 Pine Script 计算 Worker 上限。",
  },
  {
    index: "adk",
    label: "智能体",
    description: "配置 AI 模型 Provider、Agent 定义与 Skill 安装。",
  },
  {
    index: "data-management",
    label: "数据管理",
    description: "统计数据库占用，清理历史数据并管理数据库重建。",
  },
] as const;

type MenuIndex = (typeof settingsMenu)[number]["index"];

const DEFAULT_SECTION: MenuIndex = "runtime-dependencies";

const activeMenu = computed<MenuIndex>(() => {
  const s = route.params.section as string | undefined;
  if (s === "data-migration") return "data-management";
  if (s && settingsMenu.some((e) => e.index === s)) {
    return s as MenuIndex;
  }
  return DEFAULT_SECTION;
});

onMounted(() => {
  if (route.params.section === "data-migration") {
    writeLocalStorage(SETTINGS_LAST_KEY, "data-management");
    void router.replace("/settings/data-management");
    return;
  }
  if (!route.params.section) {
    const storedValue = readLocalStorage(SETTINGS_LAST_KEY);
    const stored = storedValue === "data-migration" ? "data-management" : storedValue;
    const last = settingsMenu.some((entry) => entry.index === stored)
      ? stored as MenuIndex
      : DEFAULT_SECTION;
    void router.replace(`/settings/${last}`);
  }
});

const futuIntegration = computed(
  () =>
    brokerSettings.value.brokers.find(
      (broker) => broker.descriptor.id === "futu",
    )?.integration ?? null,
);

const accountDiscoveryUnavailableMessage = computed(() => {
  if (futuIntegration.value == null) {
    return "请先在富途接入中填写并保存连接配置，随后 JFTrade 才会尝试发现 OpenD 账户。";
  }
  if (!futuIntegration.value.enabled) {
    return "当前富途接入已停用。启用并保存后，JFTrade 才会尝试发现 OpenD 账户。";
  }
  if (brokerRuntime.value.session.connectivity !== "connected") {
    return "OpenD 尚未连接成功。连接恢复后，这里会显示发现到的账户。";
  }
  return undefined;
});

function handleMenuSelect(index: string): void {
  if (!settingsMenu.some((entry) => entry.index === index)) return;
  writeLocalStorage(SETTINGS_LAST_KEY, index);
  void router.push(`/settings/${index}`);
}

const managedAccountsController = createSettingsManagedAccountsController({
  brokerRuntime,
  brokerSettings,
  createManagedBrokerAccount,
  updateManagedBrokerAccount,
  deleteManagedBrokerAccount,
});

const {
  accountForm,
  deletingAccountId,
  editingAccountId,
  importRuntimeAccount,
  populateAccountForm,
  removeAccount,
  resetAccountForm,
  savingAccount,
  submitAccount,
} = managedAccountsController;
</script>

<template>
  <div class="settings-page grid gap-6">
    <section class="settings-page__layout grid lg:grid-cols-[220px_1fr]">
      <nav class="settings-page__nav border border-slate-200 bg-white">
        <button
          v-for="entry in settingsMenu"
          :key="entry.index"
          type="button"
          class="w-full px-4 py-3 text-left text-sm transition hover:bg-slate-50"
          :class="
            activeMenu === entry.index
              ? 'bg-slate-50 font-semibold text-slate-900'
              : 'text-slate-600'
          "
          @click="handleMenuSelect(entry.index)"
        >
          {{ entry.label }}
        </button>
      </nav>

      <div class="settings-page__content grid gap-6 p-5">
        <RuntimeDependenciesSection
          v-if="activeMenu === 'runtime-dependencies'"
          mode="settings"
        />

        <FutuIntegrationSection
          v-show="activeMenu === 'futu-integration'"
          mode="settings"
        />

        <SettingsManagedAccountsSection
          v-show="activeMenu === 'managed-accounts'"
          :accounts="brokerSettings.accounts"
          :account-form="accountForm"
          :deleting-account-id="deletingAccountId"
          :editing-account-id="editingAccountId"
          :saving-account="savingAccount"
          :populate-account-form="populateAccountForm"
          :remove-account="removeAccount"
          :reset-account-form="resetAccountForm"
          :submit-account="submitAccount"
        />

        <SettingsAccountDiscoverySection
          v-show="activeMenu === 'account-discovery'"
          :accounts="brokerRuntime.accounts"
          :import-runtime-account="importRuntimeAccount"
          :unavailable-message="accountDiscoveryUnavailableMessage"
        />

        <SettingsAppearanceSection v-show="activeMenu === 'appearance'" />

        <SettingsExchangeCalendarSection v-show="activeMenu === 'exchange-calendars'" />

        <SettingsSecuritySection v-if="activeMenu === 'security'" />

        <SettingsPineWorkerSection v-if="activeMenu === 'pine-worker'" />

        <SettingsADKSection v-show="activeMenu === 'adk'" />

        <SettingsDataManagementSection v-if="activeMenu === 'data-management'" />
      </div>
    </section>
  </div>
</template>

<style scoped>
.settings-page,
.settings-page__layout {
  min-height: calc(100dvh - 76px);
}

.settings-page__layout {
  align-items: stretch;
  flex: 1 1 auto;
}

.settings-page__nav {
  align-self: stretch;
  min-height: calc(100dvh - 76px);
}

.settings-page__content {
  align-content: start;
  min-width: 0;
}

@media (max-width: 1023px) {
  .settings-page,
  .settings-page__layout {
    min-height: auto;
  }

  .settings-page__layout {
    grid-template-rows: auto;
  }

  .settings-page__nav {
    min-height: auto;
  }
}
</style>
