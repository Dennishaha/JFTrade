<script setup lang="ts">
import { computed, ref, watch } from "vue";

import type { MCPServerAuthMode, MCPServerStatus } from "@/contracts";

const props = defineProps<{
  form: {
    enabled: boolean;
    port: number;
    authMode: MCPServerAuthMode;
  };
  status: MCPServerStatus;
  tokenConfigured: boolean;
  oneTimeToken: string;
  saving: boolean;
  save: () => void | Promise<void>;
  resetToken: () => void | Promise<void>;
  clearOneTimeToken: () => void;
}>();

const tokenDialogOpen = ref(false);
const copyNotice = ref("");

const statusLabel = computed(() => {
  if (props.status.running) return "运行中";
  if (props.status.lastError) return "启动失败";
  return "已停止";
});

const statusColor = computed(() => {
  if (props.status.running) return "success";
  if (props.status.lastError) return "error";
  return "default";
});

const authOptions: Array<{ title: string; value: MCPServerAuthMode }> = [
  { title: "Bearer Token", value: "token" },
  { title: "无 Token", value: "none" },
];

watch(
  () => props.oneTimeToken,
  (token) => {
    if (token !== "") {
      copyNotice.value = "";
      tokenDialogOpen.value = true;
    }
  },
);

async function copyText(value: string, notice: string): Promise<void> {
  if (value === "") return;
  try {
    if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
    } else if (typeof document !== "undefined") {
      const textarea = document.createElement("textarea");
      textarea.value = value;
      textarea.setAttribute("readonly", "");
      textarea.style.position = "fixed";
      textarea.style.opacity = "0";
      document.body.appendChild(textarea);
      textarea.select();
      const copied = document.execCommand("copy");
      textarea.remove();
      if (!copied) throw new Error("clipboard copy failed");
    } else {
      throw new Error("clipboard is unavailable");
    }
    copyNotice.value = notice;
  } catch {
    copyNotice.value = "复制失败，请手动复制";
  }
}

function closeTokenDialog(): void {
  tokenDialogOpen.value = false;
  props.clearOneTimeToken();
  copyNotice.value = "";
}
</script>

<template>
  <section class="grid gap-4">
    <v-card flat class="card-shell border-0">
      <v-card-title class="flex flex-wrap items-center justify-between gap-3">
        <span class="text-base font-semibold text-slate-900">MCP 服务</span>
        <v-chip size="small" :color="statusColor" variant="tonal">
          {{ statusLabel }}
        </v-chip>
      </v-card-title>
      <v-card-text class="grid gap-4">
        <div class="grid gap-3 md:grid-cols-[minmax(0,1fr)_10rem_12rem] md:items-center">
          <v-switch
            v-model="form.enabled"
            label="启用本机 MCP 服务"
            color="primary"
            density="comfortable"
            hide-details
          />
          <v-text-field
            v-model.number="form.port"
            label="端口"
            type="number"
            min="1024"
            max="65535"
            density="comfortable"
            hide-details
          />
          <v-select
            v-model="form.authMode"
            label="鉴权"
            :items="authOptions"
            density="comfortable"
            hide-details
          />
        </div>

        <div class="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-start">
          <v-text-field
            :model-value="status.endpoint"
            label="MCP 端点"
            readonly
            density="comfortable"
            hide-details
          >
            <template #append-inner>
              <v-btn
                icon="fa-solid fa-copy"
                variant="text"
                size="small"
                title="复制 MCP 端点"
                @click="copyText(status.endpoint, '端点已复制')"
              />
            </template>
          </v-text-field>
          <div class="flex flex-wrap gap-2">
            <v-btn
              v-if="form.authMode === 'token'"
              variant="outlined"
              :loading="saving"
              @click="resetToken"
            >
              {{ tokenConfigured ? "重置 Token" : "生成 Token" }}
            </v-btn>
            <v-btn color="primary" :loading="saving" @click="save">保存</v-btn>
          </div>
        </div>

        <v-alert
          v-if="status.lastError"
          type="error"
          variant="tonal"
          density="compact"
        >
          {{ status.lastError }}
        </v-alert>
      </v-card-text>
    </v-card>

    <v-dialog
      :model-value="tokenDialogOpen"
      max-width="620"
      @update:model-value="(open) => { if (!open) closeTokenDialog(); }"
    >
      <v-card>
        <v-card-title class="flex items-center justify-between gap-2">
          <span>新的 MCP Token</span>
          <v-btn icon="mdi-close" variant="text" size="small" title="关闭" @click="closeTokenDialog" />
        </v-card-title>
        <v-card-text class="grid gap-3">
          <v-text-field
            :model-value="oneTimeToken"
            label="Bearer Token"
            readonly
            density="comfortable"
            hide-details
          >
            <template #append-inner>
              <v-btn
                icon="fa-solid fa-copy"
                variant="text"
                size="small"
                title="复制 Token"
                @click="copyText(oneTimeToken, 'Token 已复制')"
              />
            </template>
          </v-text-field>
          <span v-if="copyNotice" class="text-sm text-success">{{ copyNotice }}</span>
        </v-card-text>
        <v-card-actions class="px-6 pb-4">
          <v-spacer />
          <v-btn color="primary" @click="closeTokenDialog">完成</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>
