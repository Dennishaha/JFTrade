<script setup lang="ts">
import type { BrokerRuntimeResponse } from "@/contracts";

import {
  formatAccountTypeLabel,
  formatMarketLabel,
  formatTradingEnvironment,
} from "../composables/consoleDataFormatting";
import SectionHeader from "./SectionHeader.vue";

const props = defineProps<{
  accounts: BrokerRuntimeResponse["accounts"];
  importRuntimeAccount: (
    account: BrokerRuntimeResponse["accounts"][number],
  ) => void | Promise<void>;
  unavailableMessage?: string | undefined;
}>();
</script>

<template>
  <div class="grid gap-6">
    <div class="settings-panel">
      <SectionHeader
        title="OpenD 发现的账户"
        description="查看 OpenD 运行时发现的账户，并一键导入到托管账户列表。"
      />

      <div class="mt-4 grid gap-3">
        <template v-if="props.accounts.length">
          <div
            v-for="account in props.accounts"
            :key="`${account.tradingEnvironment}-${account.accountId}`"
            class="rounded-2xl border border-slate-200 bg-white px-4 py-3"
          >
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <div class="text-base font-semibold text-slate-900">
                  {{ account.accountId }}
                </div>
                <div class="mt-1 text-xs text-slate-500">
                  {{ formatTradingEnvironment(account.tradingEnvironment) }} /
                  {{ formatAccountTypeLabel(account.accountType) }} /
                  {{
                    account.marketAuthorities
                      .map(formatMarketLabel)
                      .join(", ") || "暂无"
                  }}
                </div>
              </div>
              <v-btn
                variant="text"
                color="primary"
                @click="props.importRuntimeAccount(account)"
              >
                导入到账户管理
              </v-btn>
            </div>
          </div>
        </template>
        <v-empty-state
          v-else
          :text="
            props.unavailableMessage ||
            '当前没有从运行时检测到账户。若 OpenD 未登录或未授权，这里会保持为空。'
          "
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
