<script setup lang="ts">
import { computed, ref } from "vue";

import FutuIntegrationSection from "../components/FutuIntegrationSection.vue";
import OpenDInstallGuideSection from "../components/OpenDInstallGuideSection.vue";
import SettingsAccountDiscoverySection from "../components/SettingsAccountDiscoverySection.vue";
import SettingsManagedAccountsSection from "../components/SettingsManagedAccountsSection.vue";
import PageHeader from "../components/PageHeader.vue";
import { createSettingsManagedAccountsController } from "../composables/settingsManagedAccounts";
import { useConsoleData } from "../composables/useConsoleData";

const {
  brokerRuntime,
  brokerSettings,
  createManagedBrokerAccount,
  deleteManagedBrokerAccount,
  updateManagedBrokerAccount,
} = useConsoleData();

const settingsMenu = [
  {
    index: "futu-integration",
    label: "富途接入",
    description: "配置 OpenD 连接参数与默认账号信息。",
  },
  {
    index: "managed-accounts",
    label: "托管账户",
    description: "维护用于切换账户范围的托管账号。",
  },
  {
    index: "account-discovery",
    label: "账户发现",
    description: "查看运行时探测到的 OpenD 账号并一键导入。",
  },
  {
    index: "plugin-manager",
    label: "OpenD 安装",
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

const settingsHeaderStats = computed(() => [
  {
    label: "托管券商",
    value: brokerSettings.value.brokers.length,
  },
  {
    label: "托管账户",
    value: brokerSettings.value.accounts.length,
  },
  {
    label: "运行时账户",
    value: brokerRuntime.value.accounts.length,
  },
]);
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
  <div class="grid gap-6">
    <PageHeader
      eyebrow="控制面"
      title="设置"
      description="统一维护券商接入配置与账号资料；顶部账户范围会基于这里的账号清单切换查询上下文。"
      :stats="settingsHeaderStats"
    />

    <v-breadcrumbs class="p-0 text-sm text-slate-500">
      <v-breadcrumbs-item :to="{ path: '/settings' }">控制台</v-breadcrumbs-item>
      <v-breadcrumbs-item>设置</v-breadcrumbs-item>
      <v-breadcrumbs-item>{{ activeMenuMeta.label }}</v-breadcrumbs-item>
    </v-breadcrumbs>

    <div class="settings-page-header flex items-center justify-between gap-3">
      <div>
        <div class="text-lg font-semibold text-slate-900">{{ activeMenuMeta.label }}</div>
        <div class="mt-1 text-xs text-slate-500">{{ activeMenuMeta.description }}</div>
      </div>
      <v-chip variant="outlined" size="small">{{ activeMenuMeta.label }}</v-chip>
    </div>

    <section class="grid gap-5 lg:grid-cols-[220px_1fr]">
      <nav class="rounded-lg border border-slate-200 bg-white">
        <button
          v-for="entry in settingsMenu"
          :key="entry.index"
          type="button"
          class="w-full px-4 py-3 text-left text-sm transition hover:bg-slate-50"
          :class="activeMenu === entry.index ? 'bg-slate-50 font-semibold text-slate-900' : 'text-slate-600'"
          @click="handleMenuSelect(entry.index)"
        >
          {{ entry.label }}
        </button>
      </nav>

      <div class="grid gap-6">
        <FutuIntegrationSection v-show="activeMenu === 'futu-integration'" />

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
        />

        <OpenDInstallGuideSection v-show="activeMenu === 'plugin-manager'" />
      </div>
    </section>
  </div>
</template>