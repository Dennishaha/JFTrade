<script setup lang="ts">
import type { BrokerSettingsResponse } from "@jftrade/ui-contracts";

import {
  formatGenericStatusLabel,
  formatMarketLabel,
  formatTradingEnvironment,
} from "../composables/consoleDataFormatting";
import type { SettingsManagedAccountForm } from "../composables/settingsManagedAccounts";
import SectionHeader from "./SectionHeader.vue";

const tradingEnvironmentOptions = [
  { value: "SIMULATE", title: "模拟" },
  { value: "REAL", title: "实盘" },
];

defineProps<{
  accounts: BrokerSettingsResponse["accounts"];
  accountForm: SettingsManagedAccountForm;
  deletingAccountId: string;
  editingAccountId: string | null;
  savingAccount: boolean;
  populateAccountForm: (
    account: BrokerSettingsResponse["accounts"][number],
  ) => void;
  removeAccount: (accountId: string) => void | Promise<void>;
  resetAccountForm: () => void;
  submitAccount: () => void | Promise<void>;
}>();
</script>

<template>
  <div class="grid gap-6">
    <div class="settings-panel">
      <SectionHeader title="托管账户" description="这些账号会出现在顶部账户范围切换器内。" />

      <div class="mt-4 grid gap-3">
        <template v-if="accounts.length">
          <div
            v-for="account in accounts"
            :key="account.id"
            class="rounded-2xl border border-slate-200 bg-white px-4 py-3"
          >
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <div class="text-base font-semibold text-slate-900">{{ account.displayName }}</div>
                <div class="mt-1 text-xs text-slate-500">
                  {{ account.brokerId }} / {{ account.accountId }} / {{ formatTradingEnvironment(account.tradingEnvironment) }} / {{ formatMarketLabel(account.market) }}
                </div>
              </div>
              <div class="flex items-center gap-2">
                <v-chip :color="account.enabled ? 'success' : 'info'" variant="outlined" size="small">
                  {{ formatGenericStatusLabel(account.enabled ? "ENABLED" : "DISABLED") }}
                </v-chip>
                <v-btn variant="text" color="primary" @click="populateAccountForm(account)">编辑</v-btn>
                <v-btn
                  variant="text"
                  color="error"
                  :loading="deletingAccountId === account.id"
                  @click="removeAccount(account.id)"
                >
                  删除
                </v-btn>
              </div>
            </div>
          </div>
        </template>
        <v-empty-state
          v-else
          text="当前还没有保存任何券商账户，顶部账户范围会回退到运行时探测出的账号。"
        />
      </div>
    </div>

    <div v-if="editingAccountId" class="settings-panel">
      <SectionHeader
        title="编辑账户"
        description="仅支持编辑已导入的托管账户，账户基础信息由券商提供。"
      >
        <template #extra>
          <v-btn variant="text" color="primary" @click="resetAccountForm">取消编辑</v-btn>
        </template>
      </SectionHeader>

      <div class="mt-4 grid gap-4">
        <div class="grid gap-4 md:grid-cols-2">
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">券商标识</label>
            <v-text-field v-model="accountForm.brokerId" density="compact" variant="outlined" />
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">账户号</label>
            <v-text-field v-model="accountForm.accountId" density="compact" variant="outlined" />
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">显示名称（可选）</label>
            <v-text-field v-model="accountForm.displayName" density="compact" variant="outlined" />
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">交易环境</label>
            <v-select
              v-model="accountForm.tradingEnvironment"
              class="w-full"
              density="compact"
              variant="outlined"
              :items="tradingEnvironmentOptions"
              item-title="title"
              item-value="value"
            />
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">市场</label>
            <v-text-field v-model="accountForm.market" density="compact" variant="outlined" />
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">券商机构（可选）</label>
            <v-text-field v-model="accountForm.securityFirm" density="compact" variant="outlined" />
          </div>
        </div>

        <div class="grid gap-1">
          <v-switch v-model="accountForm.enabled" color="indigo" label="启用该账号作为前端可切换账户范围" hide-details />
        </div>

        <div class="flex justify-end">
          <v-btn :loading="savingAccount" color="primary" @click="submitAccount">
            更新账号
          </v-btn>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings-panel {
  border-radius: 1.25rem;
  border: 1px solid var(--card-border);
  background: var(--card-surface);
  padding: 1.25rem 1.5rem;
}
</style>