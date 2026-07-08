<script setup lang="ts">
import { useMutation, useQuery } from "@tanstack/vue-query";
import { computed, ref } from "vue";

import { fetchEnvelopeWithInit } from "../composables/apiClient";
import { queryClient, queryKeys } from "../composables/serverState";

type NotificationMode = "important" | "all" | "custom";

interface SystemNotificationSettings {
  enabled: boolean;
  mode: NotificationMode;
  levels?: string[];
  categories?: string[];
  soundEnabled: boolean;
}

interface SystemNotificationDelivery {
  delivered: boolean;
  status: string;
  message?: string;
}

interface SystemNotificationTestResponse {
  event?: Record<string, unknown>;
  delivery?: SystemNotificationDelivery;
}

const defaultSettings: SystemNotificationSettings = {
  enabled: true,
  mode: "important",
  levels: ["warn", "error"],
  categories: ["broker.connection", "strategy.order.signal", "execution.order", "execution.fill"],
  soundEnabled: true,
};

const levelOptions = [
  { value: "info", label: "Info" },
  { value: "success", label: "Success" },
  { value: "warn", label: "Warn" },
  { value: "error", label: "Error" },
] as const;

const categoryOptions = [
  { value: "broker.connection", label: "连接状态" },
  { value: "strategy.order.signal", label: "策略订单信号" },
  { value: "execution.order", label: "订单事件" },
  { value: "execution.fill", label: "成交事件" },
  { value: "market.calendar.source", label: "交易日历源" },
  { value: "bbgo.notify", label: "BBGO 通知" },
  { value: "system.notification.test", label: "测试通知" },
] as const;

const modeOptions = [
  { value: "important", label: "重要通知" },
  { value: "all", label: "全部通知" },
  { value: "custom", label: "自定义" },
] as const;

const settingsQueryKey = queryKeys.settings("system-notifications");
const errorMessage = ref("");
const statusMessage = ref("");

const settingsQuery = useQuery({
  queryKey: settingsQueryKey,
  queryFn: () =>
    fetchEnvelopeWithInit<SystemNotificationSettings>(
      "/api/v1/settings/system-notifications",
      { method: "GET" },
    ),
}, queryClient);

const saveSettingsMutation = useMutation({
  mutationFn: (next: SystemNotificationSettings) =>
    fetchEnvelopeWithInit<SystemNotificationSettings>(
      "/api/v1/settings/system-notifications",
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(next),
      },
    ),
  onSuccess: async (saved) => {
    queryClient.setQueryData(settingsQueryKey, saved);
    await queryClient.invalidateQueries({ queryKey: settingsQueryKey, refetchType: "none" });
  },
}, queryClient);

const testNotificationMutation = useMutation({
  mutationFn: () =>
    fetchEnvelopeWithInit<SystemNotificationTestResponse>(
      "/api/v1/settings/system-notifications/test",
      { method: "POST" },
    ),
}, queryClient);

const settings = computed(() => settingsQuery.data.value ?? defaultSettings);
const loading = computed(() => settingsQuery.isLoading.value);
const saving = computed(() => saveSettingsMutation.isPending.value);
const testing = computed(() => testNotificationMutation.isPending.value);
const disabled = computed(() => loading.value || saving.value);
const queryErrorMessage = computed(() =>
  settingsQuery.error.value instanceof Error ? settingsQuery.error.value.message : "",
);
const visibleErrorMessage = computed(() => errorMessage.value || queryErrorMessage.value);

function selectedLevels(): string[] {
  return settings.value.levels ?? [];
}

function selectedCategories(): string[] {
  return settings.value.categories ?? [];
}

async function save(next: SystemNotificationSettings): Promise<void> {
  errorMessage.value = "";
  statusMessage.value = "";
  try {
    await saveSettingsMutation.mutateAsync(next);
    statusMessage.value = "系统通知设置已保存。";
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "保存系统通知设置失败";
  }
}

function toggleListValue(values: readonly string[], value: string, checked: boolean): string[] {
  if (checked) {
    return values.includes(value) ? [...values] : [...values, value];
  }
  return values.filter((item) => item !== value);
}

function updateEnabled(event: Event): void {
  const target = event.target as HTMLInputElement | null;
  if (target == null || disabled.value) return;
  void save({ ...settings.value, enabled: target.checked });
}

function updateSoundEnabled(event: Event): void {
  const target = event.target as HTMLInputElement | null;
  if (target == null || disabled.value) return;
  void save({ ...settings.value, soundEnabled: target.checked });
}

function updateMode(event: Event): void {
  const target = event.target as HTMLInputElement | null;
  if (target == null || disabled.value) return;
  void save({ ...settings.value, mode: target.value as NotificationMode });
}

function updateLevel(value: string, event: Event): void {
  const target = event.target as HTMLInputElement | null;
  if (target == null || disabled.value) return;
  void save({ ...settings.value, mode: "custom", levels: toggleListValue(selectedLevels(), value, target.checked) });
}

function updateCategory(value: string, event: Event): void {
  const target = event.target as HTMLInputElement | null;
  if (target == null || disabled.value) return;
  void save({ ...settings.value, mode: "custom", categories: toggleListValue(selectedCategories(), value, target.checked) });
}

async function sendTestNotification(): Promise<void> {
  errorMessage.value = "";
  statusMessage.value = "";
  try {
    const result = await testNotificationMutation.mutateAsync();
    const delivery = result.delivery;
    if (delivery?.delivered) {
      statusMessage.value = "测试通知已发送到系统通知中心。";
    } else if (delivery?.message) {
      statusMessage.value = `测试通知已记录：${delivery.message}`;
    } else {
      statusMessage.value = "测试通知已记录，但当前桌面通知状态未知。";
    }
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : "发送测试通知失败";
  }
}
</script>

<template>
  <section class="rounded-lg border border-slate-200 bg-white px-5 py-5">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="text-base font-semibold text-slate-900">系统通知</div>
        <div class="mt-1 text-xs text-slate-500">控制后端事件进入操作系统通知中心的范围。</div>
      </div>
      <span class="rounded border border-slate-200 px-2 py-1 text-xs font-medium text-slate-600">
        {{ settings.enabled ? "已开启" : "已关闭" }}
      </span>
    </div>

    <div class="mt-5 grid gap-4">
      <label class="flex items-center justify-between gap-4 rounded-md border border-slate-200 px-4 py-3">
        <span>
          <span class="block text-sm font-medium text-slate-800">系统通知</span>
          <span class="mt-1 block text-xs text-slate-500">开启后符合策略的 live notification 会弹出系统通知。</span>
        </span>
        <input
          type="checkbox"
          :checked="settings.enabled"
          :disabled="disabled"
          class="h-5 w-5 cursor-pointer accent-slate-900 disabled:cursor-not-allowed"
          @change="updateEnabled"
        />
      </label>

      <fieldset class="rounded-md border border-slate-200 px-4 py-3">
        <legend class="px-1 text-sm font-medium text-slate-800">通知范围</legend>
        <div class="mt-3 grid gap-2 sm:grid-cols-3">
          <label
            v-for="option in modeOptions"
            :key="option.value"
            class="flex items-center gap-2 rounded border border-slate-200 px-3 py-2 text-sm text-slate-700"
          >
            <input
              type="radio"
              name="system-notification-mode"
              :value="option.value"
              :checked="settings.mode === option.value"
              :disabled="disabled"
              class="accent-slate-900"
              @change="updateMode"
            />
            {{ option.label }}
          </label>
        </div>
      </fieldset>

      <fieldset class="rounded-md border border-slate-200 px-4 py-3">
        <legend class="px-1 text-sm font-medium text-slate-800">级别</legend>
        <div class="mt-3 grid gap-2 sm:grid-cols-4">
          <label
            v-for="option in levelOptions"
            :key="option.value"
            class="flex items-center gap-2 text-sm text-slate-700"
          >
            <input
              type="checkbox"
              :checked="selectedLevels().includes(option.value)"
              :disabled="disabled || settings.mode !== 'custom'"
              class="accent-slate-900 disabled:cursor-not-allowed"
              @change="updateLevel(option.value, $event)"
            />
            {{ option.label }}
          </label>
        </div>
      </fieldset>

      <fieldset class="rounded-md border border-slate-200 px-4 py-3">
        <legend class="px-1 text-sm font-medium text-slate-800">类别</legend>
        <div class="mt-3 grid gap-2 sm:grid-cols-2">
          <label
            v-for="option in categoryOptions"
            :key="option.value"
            class="flex items-center gap-2 text-sm text-slate-700"
          >
            <input
              type="checkbox"
              :checked="selectedCategories().includes(option.value)"
              :disabled="disabled || settings.mode !== 'custom'"
              class="accent-slate-900 disabled:cursor-not-allowed"
              @change="updateCategory(option.value, $event)"
            />
            {{ option.label }}
          </label>
        </div>
      </fieldset>

      <label class="flex items-center justify-between gap-4 rounded-md border border-slate-200 px-4 py-3">
        <span class="text-sm font-medium text-slate-800">通知声音</span>
        <input
          type="checkbox"
          :checked="settings.soundEnabled"
          :disabled="disabled"
          class="h-5 w-5 cursor-pointer accent-slate-900 disabled:cursor-not-allowed"
          @change="updateSoundEnabled"
        />
      </label>

      <div class="flex flex-wrap items-center gap-3">
        <button
          type="button"
          class="rounded-md bg-slate-900 px-4 py-2 text-sm font-semibold text-white transition hover:bg-slate-700 disabled:cursor-not-allowed disabled:bg-slate-300"
          :disabled="disabled || testing"
          @click="sendTestNotification"
        >
          {{ testing ? "发送中" : "发送测试通知" }}
        </button>
        <span v-if="saving" class="text-xs text-slate-500">保存中...</span>
      </div>
    </div>

    <p v-if="statusMessage" class="mt-3 text-xs font-medium text-emerald-700">
      {{ statusMessage }}
    </p>
    <p v-if="visibleErrorMessage" class="mt-3 text-xs font-medium text-rose-600">
      {{ visibleErrorMessage }}
    </p>
  </section>
</template>
