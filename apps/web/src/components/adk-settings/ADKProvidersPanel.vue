<script setup lang="ts">
import { ref } from "vue";

import type { ADKProvider } from "@/contracts";

const props = defineProps<{
  providerForm: {
    id: string;
    displayName: string;
    baseUrl: string;
    model: string;
    contextWindowTokens: number;
    requestTimeoutSeconds: number;
    apiKey: string;
    enabled: boolean;
  };
  runtimeSettingsForm: {
    runTimeoutSeconds: number;
    streamIdleTimeoutSeconds: number;
  };
  providers: ADKProvider[];
  saveProvider: () => void | Promise<void>;
  saveRuntimeSettings: () => void | Promise<void>;
  newProviderForm: () => void;
  editProvider: (provider: ADKProvider) => void;
  testProvider: (providerId: string) => void | Promise<void>;
  deleteProvider: (providerId: string) => void | Promise<void>;
  setDefaultProvider: (providerId: string) => void | Promise<void>;
}>();

const providerDialogOpen = ref(false);

function openNewProviderDialog(): void {
  props.newProviderForm();
  providerDialogOpen.value = true;
}

function openEditProviderDialog(provider: ADKProvider): void {
  props.editProvider(provider);
  providerDialogOpen.value = true;
}

async function submitProviderForm(): Promise<void> {
  await props.saveProvider();
  providerDialogOpen.value = false;
}
</script>

<template>
  <section class="adk-provider-layout">
    <div class="grid auto-rows-max gap-5">
      <v-card flat class="card-shell border-0">
        <v-card-title class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="text-base font-semibold text-slate-900">模型服务</div>
            <div class="mt-1 text-xs text-slate-500">
              管理 OpenAI 兼容模型服务，并为上下文占用监控配置 context window。
            </div>
          </div>
        </v-card-title>
        <v-card-actions>
          <v-btn color="primary" size="small" @click="openNewProviderDialog">
            新增模型服务
          </v-btn>
        </v-card-actions>
      </v-card>

      <div class="grid auto-rows-max gap-3 md:grid-cols-2">
        <v-card
          v-for="provider in providers"
          :key="provider.id"
          flat
          class="card-shell border-0"
        >
          <v-card-text>
            <div class="flex items-start justify-between gap-3">
              <div>
                <div class="flex items-center gap-2">
                  <span class="font-semibold text-slate-900">{{
                    provider.displayName
                  }}</span>
                  <v-chip
                    size="x-small"
                    :color="provider.enabled ? 'success' : 'default'"
                    variant="tonal"
                  >
                    {{ provider.enabled ? "启用" : "停用" }}
                  </v-chip>
                  <v-chip
                    v-if="provider.default"
                    size="x-small"
                    color="primary"
                    variant="tonal"
                  >
                    默认
                  </v-chip>
                </div>
                <div class="mt-0.5 text-xs text-slate-500">
                  {{ provider.baseUrl }} · {{ provider.model }}
                </div>
                <div class="text-xs text-slate-500">
                  上下文窗口：{{ provider.contextWindowTokens || "未配置" }}
                </div>
                <div class="text-xs text-slate-500">
                  请求超时：{{
                    Math.round((provider.requestTimeoutMs ?? 180000) / 1000)
                  }}
                  秒
                </div>
                <div class="text-xs text-slate-500">
                  密钥：{{ provider.hasApiKey ? "已配置" : "未配置" }}
                </div>
                <div
                  v-if="provider.capabilities"
                  class="mt-1 flex flex-wrap gap-1"
                >
                  <v-chip
                    v-for="(supported, capability) in provider.capabilities"
                    :key="capability"
                    size="x-small"
                    :color="supported ? 'success' : 'default'"
                    variant="tonal"
                  >
                    {{ capability }} · {{ supported ? "支持" : "不支持" }}
                  </v-chip>
                </div>
              </div>
              <div class="flex shrink-0 flex-col gap-1">
                <v-btn
                  size="x-small"
                  variant="outlined"
                  @click="openEditProviderDialog(provider)"
                  >编辑</v-btn
                >
                <v-btn
                  size="x-small"
                  variant="outlined"
                  @click="testProvider(provider.id)"
                  >测试</v-btn
                >
                <v-btn
                  v-if="!provider.default"
                  size="x-small"
                  variant="outlined"
                  @click="setDefaultProvider(provider.id)"
                  >设为默认</v-btn
                >
                <v-btn
                  size="x-small"
                  variant="outlined"
                  color="error"
                  @click="deleteProvider(provider.id)"
                  >删除</v-btn
                >
              </div>
            </div>
          </v-card-text>
        </v-card>
        <v-card
          v-if="providers.length === 0"
          flat
          class="card-shell border-0 md:col-span-2"
        >
          <v-card-text class="text-sm text-slate-500">
            还没有配置模型服务。新增后可设置默认模型、超时和上下文窗口。
          </v-card-text>
        </v-card>
      </div>
    </div>

    <div class="grid auto-rows-max gap-5">
      <v-card flat class="card-shell border-0">
        <v-card-title>运行时超时</v-card-title>
        <v-card-text class="grid gap-3">
          <v-text-field
            v-model="runtimeSettingsForm.runTimeoutSeconds"
            label="运行总时长（秒）"
            type="number"
            density="comfortable"
            min="60"
            max="43200"
            hint="运行总时长是指整个运行过程的最长持续时间。"
            persistent-hint
          />
          <v-text-field
            v-model="runtimeSettingsForm.streamIdleTimeoutSeconds"
            label="流空闲超时（秒）"
            type="number"
            density="comfortable"
            min="30"
            max="900"
            hint="流空闲超时是指在流式响应中，如果在指定时间内没有新的数据传输，则认为连接空闲并关闭。"
            persistent-hint
          />
          <v-btn color="primary" block @click="saveRuntimeSettings"
            >保存运行时设置</v-btn
          >
        </v-card-text>
      </v-card>
    </div>

    <v-dialog
      v-model="providerDialogOpen"
      max-width="720"
      content-class="adk-provider-dialog-overlay"
    >
      <v-card class="adk-provider-dialog">
        <v-card-title class="flex items-center justify-between gap-3">
          <span>{{ providerForm.id ? "编辑模型服务" : "新增模型服务" }}</span>
          <v-btn
            icon="mdi-close"
            variant="text"
            size="small"
            @click="providerDialogOpen = false"
          />
        </v-card-title>
        <v-card-text class="adk-provider-dialog__body grid gap-3">
          <v-switch
            v-model="providerForm.enabled"
            label="启用"
            color="primary"
            hide-details
          />
          <v-text-field
            v-model="providerForm.displayName"
            label="名称"
            density="comfortable"
          />
          <v-text-field
            v-model="providerForm.baseUrl"
            label="服务地址"
            density="comfortable"
          />
          <v-text-field
            v-model="providerForm.model"
            label="默认模型"
            density="comfortable"
          />
          <v-text-field
            v-model="providerForm.contextWindowTokens"
            label="Context Window Tokens"
            type="number"
            density="comfortable"
            hint="0 表示未知，不启用上下文占用比例和自动压缩"
            persistent-hint
          />
          <v-text-field
            v-model="providerForm.apiKey"
            label="API 密钥"
            type="password"
            density="comfortable"
            :hint="providerForm.id ? '留空则保留原密钥' : ''"
            persistent-hint
          />
          <v-text-field
            v-model="providerForm.requestTimeoutSeconds"
            label="请求超时（秒）"
            type="number"
            density="comfortable"
          />
        </v-card-text>
        <v-card-actions class="justify-end gap-2">
          <v-btn variant="text" @click="providerDialogOpen = false">取消</v-btn>
          <v-btn color="primary" @click="submitProviderForm"
            >保存模型服务</v-btn
          >
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<style scoped>
.adk-provider-layout {
  display: grid;
  align-items: start;
  gap: 1.25rem;
}

@media (min-width: 1024px) {
  .adk-provider-layout {
    grid-template-columns: minmax(0, 1.4fr) minmax(18rem, 0.6fr);
  }
}

.adk-provider-dialog {
  display: flex;
  max-height: 80dvh;
  flex-direction: column;
  overflow: hidden;
  background: var(--tv-bg-surface);
  color: var(--card-text-1);
}

:global(.adk-provider-dialog-overlay) {
  background: var(--tv-bg-surface);
  border-radius: 4px;
}

.adk-provider-dialog__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  background: var(--tv-bg-surface);
}
</style>
