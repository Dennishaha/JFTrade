<script setup lang="ts">
import { useMutation, useQuery } from "@tanstack/vue-query";
import { computed, ref } from "vue";

import type { components } from "@/generated/openapi";

import { apiGet, apiPut } from "../composables/apiClient";
import { queryClient, queryKeys } from "../composables/serverState";

type SecuritySettings = Required<components["schemas"]["jftsettings.SecuritySettings"]>;

const defaultSettings: SecuritySettings = { adminAuthRequired: false };
const errorMessage = ref("");

const settingsQueryKey = queryKeys.settings("security");
const settingsQuery = useQuery({
  queryKey: settingsQueryKey,
  queryFn: () => apiGet<SecuritySettings, "/api/v1/settings/security">("/api/v1/settings/security"),
}, queryClient);
const saveSettingsMutation = useMutation({
  mutationFn: (next: SecuritySettings) =>
    apiPut<SecuritySettings, "/api/v1/settings/security">(
      "/api/v1/settings/security",
      next,
    ),
  onSuccess: async (saved) => {
    queryClient.setQueryData(settingsQueryKey, saved);
    await queryClient.invalidateQueries({ queryKey: settingsQueryKey, refetchType: "none" });
  },
}, queryClient);

const settings = computed(() => settingsQuery.data.value ?? defaultSettings);
const loading = computed(() => settingsQuery.isLoading.value);
const saving = computed(() => saveSettingsMutation.isPending.value);
const queryErrorMessage = computed(() =>
  settingsQuery.error.value instanceof Error ? settingsQuery.error.value.message : "",
);
const visibleErrorMessage = computed(() => errorMessage.value || queryErrorMessage.value);

async function updateAdminAuthRequired(event: Event): Promise<void> {
  const target = event.target as HTMLInputElement | null;
  if (target == null || saving.value) return;
  const next = { adminAuthRequired: target.checked };
  errorMessage.value = "";
  try {
    await saveSettingsMutation.mutateAsync(next);
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "保存安全设置失败";
  }
}
</script>

<template>
  <section class="rounded-lg border border-slate-200 bg-white px-5 py-5">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="text-base font-semibold text-slate-900">管理员认证</div>
        <div class="mt-1 text-xs text-slate-500">
          控制打开控制台和调用 API 时是否需要管理员密钥。
        </div>
      </div>
      <span class="rounded border border-slate-200 px-2 py-1 text-xs font-medium text-slate-600">
        {{ settings.adminAuthRequired ? "已开启" : "已关闭" }}
      </span>
    </div>

    <label class="mt-5 flex items-center justify-between gap-4 rounded-md border border-slate-200 px-4 py-3">
      <span>
        <span class="block text-sm font-medium text-slate-800">管理员认证</span>
        <span class="mt-1 block text-xs text-slate-500">
          开启后需要输入运行目录 <code>secrets/admin.key</code> 中的密钥。
        </span>
      </span>
      <input
        data-testid="admin-auth-required-toggle"
        type="checkbox"
        :checked="settings.adminAuthRequired"
        :disabled="loading || saving"
        class="h-5 w-5 cursor-pointer accent-slate-900 disabled:cursor-not-allowed"
        @change="updateAdminAuthRequired"
      />
    </label>

    <p v-if="visibleErrorMessage" class="mt-3 text-xs font-medium text-rose-600">
      {{ visibleErrorMessage }}
    </p>
  </section>
</template>
