<script setup lang="ts">
import type { BrokerRuntimeResponse } from "@jftrade/ui-contracts";

import SectionHeader from "./SectionHeader.vue";

defineProps<{
  accounts: BrokerRuntimeResponse["accounts"];
  importRuntimeAccount: (
    account: BrokerRuntimeResponse["accounts"][number],
  ) => void;
}>();
</script>

<template>
  <div class="grid gap-6">
    <div class="settings-panel">
      <SectionHeader title="OpenD discovered accounts" description="OpenD 实时探测到的账号；可一键导入到托管列表。" />

      <div class="mt-4 grid gap-3">
        <template v-if="accounts.length">
          <div
            v-for="account in accounts"
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
              <v-btn variant="text" color="primary" @click="importRuntimeAccount(account)">
                导入到账号管理
              </v-btn>
            </div>
          </div>
        </template>
        <v-empty-state
          v-else
          text="当前没有从运行时探测到账号。OpenD 未登录时，这里会为空。"
        />
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