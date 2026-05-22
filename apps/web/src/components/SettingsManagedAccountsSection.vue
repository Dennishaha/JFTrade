<script setup lang="ts">
import type { BrokerSettingsResponse } from "@jftrade/ui-contracts";

import type { SettingsManagedAccountForm } from "../composables/settingsManagedAccounts";
import SectionHeader from "./SectionHeader.vue";

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
      <SectionHeader title="Managed accounts" description="这些账号会出现在顶部 Scope 切换器内。" />

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
                  {{ account.brokerId }} / {{ account.accountId }} / {{ account.tradingEnvironment }} / {{ account.market }}
                </div>
              </div>
              <div class="flex items-center gap-2">
                <v-chip :color="account.enabled ? 'success' : 'info'" variant="outlined" size="small">
                  {{ account.enabled ? "ENABLED" : "DISABLED" }}
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
          text="当前还没有保存任何 broker account，顶部 Scope 会回退到运行时探测出的账号。"
        />
      </div>
    </div>

    <div class="settings-panel">
      <SectionHeader
        :title="editingAccountId ? 'Edit account' : 'Create account'"
        :description="editingAccountId ? undefined : '手工创建或导入一个新的托管账号。'"
      >
        <template #extra>
          <v-btn variant="text" color="primary" @click="resetAccountForm">重置</v-btn>
        </template>
      </SectionHeader>

      <div class="mt-4 grid gap-4">
        <div class="grid gap-4 md:grid-cols-2">
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">Broker ID</label>
            <v-text-field v-model="accountForm.brokerId" density="compact" variant="outlined" />
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">Account ID</label>
            <v-text-field v-model="accountForm.accountId" density="compact" variant="outlined" />
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">Display Name（可选）</label>
            <v-text-field v-model="accountForm.displayName" density="compact" variant="outlined" />
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">Trading Environment</label>
            <v-select
              v-model="accountForm.tradingEnvironment"
              class="w-full"
              density="compact"
              variant="outlined"
              :items="['SIMULATE', 'REAL']"
            />
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">Market</label>
            <v-text-field v-model="accountForm.market" density="compact" variant="outlined" />
          </div>
          <div class="grid gap-1">
            <label class="text-sm font-medium text-slate-700">Security Firm（可选）</label>
            <v-text-field v-model="accountForm.securityFirm" density="compact" variant="outlined" />
          </div>
        </div>

        <div class="grid gap-1">
          <v-switch v-model="accountForm.enabled" color="indigo" label="启用该账号作为前端可切换 Scope" hide-details />
        </div>

        <div class="flex justify-end">
          <v-btn :loading="savingAccount" color="primary" @click="submitAccount">
            {{ editingAccountId ? "更新账号" : "新增账号" }}
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