<script setup lang="ts">
import { useMutation, useQuery } from "@tanstack/vue-query";
import { computed, reactive, ref, watch } from "vue";

import type { components } from "@/generated/openapi";

import { fetchEnvelopeWithInit } from "../composables/apiClient";
import { queryClient, queryKeys } from "../composables/serverState";
import { resolveDesktopMode } from "../runtimeConfig";

type WebAccessSettings = Required<
  components["schemas"]["jftsettings.SecuritySettings"]
>;
type GeneratedWebAccessSettingsUpdate =
  components["schemas"]["jftsettings.SecuritySettingsUpdate"];
type WebAccessSettingsUpdate = GeneratedWebAccessSettingsUpdate & Required<Pick<
  GeneratedWebAccessSettingsUpdate,
  "webAccessEnabled" | "publicAccessEnabled" | "webPort"
>>;

const defaultSettings: WebAccessSettings = {
  webAccessEnabled: false,
  publicAccessEnabled: false,
  webPort: 6688,
  passwordConfigured: false,
};
const form = reactive({
  webAccessEnabled: false,
  publicAccessEnabled: false,
  webPort: 6688,
  newPassword: "",
  confirmPassword: "",
});
const errorMessage = ref("");
const successMessage = ref("");
const desktopMode = resolveDesktopMode();

const settingsQueryKey = queryKeys.settings("security");
const settingsQuery = useQuery({
  queryKey: settingsQueryKey,
  retry: false,
  queryFn: () =>
    fetchEnvelopeWithInit<WebAccessSettings>("/api/v1/settings/security", {
      method: "GET",
    }),
}, queryClient);
const saveSettingsMutation = useMutation({
  mutationFn: (next: WebAccessSettingsUpdate) =>
    fetchEnvelopeWithInit<WebAccessSettings>("/api/v1/settings/security", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(next),
    }),
  onSuccess: async (saved) => {
    queryClient.setQueryData(settingsQueryKey, saved);
    await queryClient.invalidateQueries({
      queryKey: settingsQueryKey,
      refetchType: "none",
    });
  },
}, queryClient);

const settings = computed(() => ({
  ...defaultSettings,
  ...(settingsQuery.data.value ?? {}),
}));
const loading = computed(() => settingsQuery.isLoading.value);
const saving = computed(() => saveSettingsMutation.isPending.value);
const settingsAvailable = computed(() => settingsQuery.data.value != null);
const controlsDisabled = computed(
  () => loading.value || saving.value || !settingsAvailable.value,
);
const queryErrorMessage = computed(() =>
  settingsQuery.error.value == null
    ? ""
    : "无法读取 Web 访问设置，当前状态未知。请重试后再修改。",
);
const visibleErrorMessage = computed(
  () => errorMessage.value || queryErrorMessage.value,
);
const passwordRequired = computed(
  () => form.webAccessEnabled && !settings.value.passwordConfigured,
);
const dirty = computed(
  () =>
    form.webAccessEnabled !== settings.value.webAccessEnabled ||
    form.publicAccessEnabled !== settings.value.publicAccessEnabled ||
    form.webPort !== settings.value.webPort ||
    form.newPassword !== "" ||
    form.confirmPassword !== "",
);
const statusLabel = computed(() => {
  if (loading.value) return "读取中";
  if (!settingsAvailable.value) return "状态未知";
  if (!settings.value.webAccessEnabled) return "未开启";
  return settings.value.publicAccessEnabled
    ? `已开启 · 目标：其他设备 · 端口 ${settings.value.webPort}`
    : `已开启 · 仅本机 · 端口 ${settings.value.webPort}`;
});
const localAccessUrl = computed(() => {
  if (!desktopMode && typeof window !== "undefined") {
    return window.location.origin;
  }
  return `http://127.0.0.1:${settings.value.webPort}`;
});
const networkAccessHint = computed(() => {
  try {
    const url = new URL(localAccessUrl.value);
    const port = url.port === "" ? "" : `:${url.port}`;
    return `${url.protocol}//<这台电脑的局域网 IP>${port}`;
  } catch {
    return "http://<这台电脑的局域网 IP>:端口";
  }
});

watch(
  () => settingsQuery.data.value,
  (saved) => {
    if (saved == null) return;
    form.webAccessEnabled = saved.webAccessEnabled;
    form.publicAccessEnabled = saved.publicAccessEnabled;
    form.webPort = saved.webPort ?? defaultSettings.webPort;
    form.newPassword = "";
    form.confirmPassword = "";
  },
  { immediate: true },
);

watch(
  () => form.webAccessEnabled,
  (enabled) => {
    if (!enabled) {
      form.publicAccessEnabled = false;
      form.newPassword = "";
      form.confirmPassword = "";
    }
    errorMessage.value = "";
    successMessage.value = "";
  },
);

function passwordByteLength(value: string): number {
  return new TextEncoder().encode(value).byteLength;
}

function validateForm(): string {
  if (!Number.isInteger(form.webPort) || form.webPort < 1024 || form.webPort > 65535) {
    return "Web 对外端口必须是 1024–65535 之间的整数。";
  }
  if (passwordRequired.value && form.newPassword === "") {
    return "首次开启 Web 访问时，请设置 Web 访问密码。";
  }
  if (form.newPassword === "" && form.confirmPassword !== "") {
    return "请先输入新的 Web 访问密码。";
  }
  if (form.newPassword !== "") {
    if (form.newPassword.trim() === "") {
      return "Web 访问密码不能只包含空格。";
    }
    if (Array.from(form.newPassword).length < 15) {
      return "Web 访问密码至少需要 15 个字符。";
    }
    if (passwordByteLength(form.newPassword) > 1024) {
      return "Web 访问密码不能超过 1024 个 UTF-8 字节。";
    }
    if (form.newPassword !== form.confirmPassword) {
      return "两次输入的 Web 访问密码不一致。";
    }
  }
  return "";
}

function resetForm(): void {
  form.webAccessEnabled = settings.value.webAccessEnabled;
  form.publicAccessEnabled = settings.value.publicAccessEnabled;
  form.webPort = settings.value.webPort;
  form.newPassword = "";
  form.confirmPassword = "";
  errorMessage.value = "";
  successMessage.value = "";
}

function clearFeedback(): void {
  errorMessage.value = "";
  successMessage.value = "";
}

function retrySettings(): void {
  errorMessage.value = "";
  successMessage.value = "";
  void settingsQuery.refetch();
}

async function saveSettings(): Promise<void> {
  if (controlsDisabled.value) return;
  const validationError = validateForm();
  if (validationError !== "") {
    errorMessage.value = validationError;
    successMessage.value = "";
    return;
  }

  const next: WebAccessSettingsUpdate = {
    webAccessEnabled: form.webAccessEnabled,
    publicAccessEnabled:
      form.webAccessEnabled && form.publicAccessEnabled,
    webPort: form.webPort,
  };
  if (form.newPassword !== "") {
    next.newPassword = form.newPassword;
  }

  errorMessage.value = "";
  successMessage.value = "";
  try {
    const webAccessChanged =
      next.webAccessEnabled !== settings.value.webAccessEnabled;
    const publicAccessChanged =
      next.publicAccessEnabled !== settings.value.publicAccessEnabled;
    const portChanged = next.webPort !== settings.value.webPort;
    const passwordChanged = next.newPassword != null;
    const saved = await saveSettingsMutation.mutateAsync(next);
    if (!saved.webAccessEnabled) {
      successMessage.value =
        "Web 访问已关闭，Web 端口已释放，已有浏览器会话已失效。";
    } else if (webAccessChanged || publicAccessChanged || portChanged) {
      successMessage.value =
        "Web 监听设置已立即生效，已有浏览器会话已失效。";
    } else if (passwordChanged && !webAccessChanged) {
      successMessage.value =
        "Web 访问密码已更新，已有浏览器会话将失效。";
    } else {
      successMessage.value = "Web 访问设置已保存。";
    }
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : "保存 Web 访问设置失败";
  }
}
</script>

<template>
  <section class="rounded-lg border border-slate-200 bg-white px-5 py-5">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="text-base font-semibold text-slate-900">Web 访问</div>
        <div class="mt-1 text-xs text-slate-500">
          默认仅使用桌面应用，无需密码。需要从浏览器访问时再主动开启。
        </div>
      </div>
      <span class="rounded border border-slate-200 px-2 py-1 text-xs font-medium text-slate-600">
        {{ statusLabel }}
      </span>
    </div>

    <div
      v-if="!desktopMode"
      class="mt-5 rounded-md border border-slate-200 bg-slate-50 px-4 py-4"
    >
      <div class="text-sm font-medium text-slate-800">浏览器中仅可查看</div>
      <p class="mt-1 text-xs leading-5 text-slate-600">
        请在 JFTrade 桌面端修改 Web 访问密码、启停状态和网络范围。
      </p>
      <p v-if="settingsAvailable" class="mt-2 text-xs text-slate-500">
        Web 访问密码：{{ settings.passwordConfigured ? "已配置" : "未配置" }}
      </p>
      <p v-if="visibleErrorMessage" class="mt-2 text-xs font-medium text-rose-600">
        {{ visibleErrorMessage }}
      </p>
      <button
        v-if="queryErrorMessage"
        type="button"
        class="mt-3 rounded-md border border-slate-300 px-3 py-2 text-xs text-slate-700"
        @click="retrySettings"
      >
        重新读取
      </button>
    </div>

    <form
      v-else
      data-testid="web-access-settings-form"
      class="mt-5 grid gap-4"
      novalidate
      @submit.prevent="saveSettings"
    >
      <label class="flex items-center justify-between gap-4 rounded-md border border-slate-200 px-4 py-3">
        <span>
          <span class="block text-sm font-medium text-slate-800">开启 Web 访问</span>
          <span class="mt-1 block text-xs text-slate-500">
            开启后，浏览器必须使用你设置的 Web 访问密码登录；桌面应用仍然无需密码。
          </span>
        </span>
        <input
          v-model="form.webAccessEnabled"
          data-testid="web-access-enabled-toggle"
          type="checkbox"
          :disabled="controlsDisabled"
          class="h-5 w-5 cursor-pointer accent-slate-900 disabled:cursor-not-allowed"
        />
      </label>

      <label class="grid gap-1 rounded-md border border-slate-200 px-4 py-3 text-xs font-medium text-slate-600">
        Web 对外端口
        <input
          v-model.number="form.webPort"
          data-testid="web-access-port"
          type="number"
          inputmode="numeric"
          min="1024"
          max="65535"
          step="1"
          :disabled="controlsDisabled"
          class="w-40 rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900"
          @input="clearFeedback"
        />
        <span class="font-normal leading-5 text-slate-500">
          默认 6688。它与 Wails 桌面内部端口相互独立；保存后立即切换。
        </span>
      </label>

      <div
        v-if="form.webAccessEnabled"
        class="grid gap-3 rounded-md border border-slate-200 px-4 py-4"
      >
        <div>
          <div class="text-sm font-medium text-slate-800">
            {{ settings.passwordConfigured ? "修改 Web 访问密码" : "设置 Web 访问密码" }}
          </div>
          <div class="mt-1 text-xs text-slate-500">
            {{ settings.passwordConfigured
              ? "留空则保持当前密码。修改密码后，已有浏览器会话将失效。"
              : "首次开启必须设置密码。至少 15 个字符，支持空格和中文，建议使用容易记忆的长口令；无需刻意组合大小写、数字或符号。" }}
          </div>
        </div>
        <label class="grid gap-1 text-xs font-medium text-slate-600">
          新密码
          <input
            v-model="form.newPassword"
            data-testid="web-access-new-password"
            type="password"
            autocomplete="new-password"
            maxlength="256"
            :required="passwordRequired"
            :disabled="controlsDisabled"
            class="rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900"
            @input="clearFeedback"
          />
        </label>
        <label class="grid gap-1 text-xs font-medium text-slate-600">
          确认新密码
          <input
            v-model="form.confirmPassword"
            data-testid="web-access-confirm-password"
            type="password"
            autocomplete="new-password"
            maxlength="256"
            :required="passwordRequired || form.newPassword !== ''"
            :disabled="controlsDisabled"
            class="rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-900"
            @input="clearFeedback"
          />
        </label>
      </div>

      <label class="flex items-start justify-between gap-4 rounded-md border border-amber-200 bg-amber-50 px-4 py-3">
        <span>
          <span class="block text-sm font-medium text-amber-900">允许局域网/其他设备访问</span>
          <span class="mt-1 block text-xs leading-5 text-amber-800">
            开启后将立即监听所有网络接口。当前仅提供 HTTP，请只在可信局域网中使用；如需通过互联网访问，必须自行配置 HTTPS 反向代理。
          </span>
        </span>
        <input
          v-model="form.publicAccessEnabled"
          data-testid="public-access-enabled-toggle"
          type="checkbox"
          :disabled="!form.webAccessEnabled || controlsDisabled"
          class="mt-0.5 h-5 w-5 cursor-pointer accent-amber-700 disabled:cursor-not-allowed"
          @change="clearFeedback"
        />
      </label>

      <p v-if="visibleErrorMessage" class="text-xs font-medium text-rose-600">
        {{ visibleErrorMessage }}
      </p>
      <button
        v-if="queryErrorMessage"
        type="button"
        class="justify-self-start rounded-md border border-slate-300 px-3 py-2 text-xs text-slate-700"
        @click="retrySettings"
      >
        重新读取
      </button>
      <p v-if="successMessage" class="text-xs font-medium text-emerald-700">
        {{ successMessage }}
      </p>

      <div class="flex justify-end gap-2">
        <button
          type="button"
          :disabled="!dirty || controlsDisabled"
          class="rounded-md border border-slate-300 px-3 py-2 text-sm text-slate-700 disabled:opacity-50"
          @click="resetForm"
        >
          取消更改
        </button>
        <button
          data-testid="save-web-access-settings"
          type="submit"
          :disabled="!dirty || controlsDisabled"
          class="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
        >
          {{ saving ? "保存中…" : "保存 Web 访问设置" }}
        </button>
      </div>
    </form>

    <div
      v-if="settings.webAccessEnabled"
      class="mt-4 rounded-md border border-slate-200 bg-slate-50 px-4 py-3"
    >
      <div class="text-xs font-medium text-slate-600">
        {{ desktopMode ? "本机浏览器访问地址" : "当前浏览器访问地址" }}
      </div>
      <code data-testid="web-access-local-url" class="mt-1 block text-sm text-slate-900">
        {{ localAccessUrl }}
      </code>
      <template v-if="settings.publicAccessEnabled">
        <p class="mt-3 text-xs leading-5 text-slate-600">
          其他设备请将占位内容替换为这台电脑的局域网 IP：
        </p>
        <code data-testid="web-access-network-hint" class="mt-1 block text-sm text-slate-900">
          {{ networkAccessHint }}
        </code>
        <p class="mt-2 text-xs leading-5 text-amber-700">
          这里显示的是当前已生效的访问范围。
        </p>
      </template>
    </div>
  </section>
</template>
