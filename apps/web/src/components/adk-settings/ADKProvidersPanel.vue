<script setup lang="ts">
import type { ADKProvider } from "@jftrade/ui-contracts";

defineProps<{
  providerForm: {
    id: string;
    displayName: string;
    baseUrl: string;
    model: string;
    apiKey: string;
    enabled: boolean;
  };
  providers: ADKProvider[];
  saveProvider: () => void | Promise<void>;
  newProviderForm: () => void;
  editProvider: (provider: ADKProvider) => void;
  testProvider: (providerId: string) => void | Promise<void>;
  deleteProvider: (providerId: string) => void | Promise<void>;
}>();
</script>

<template>
  <section class="grid gap-5 lg:grid-cols-[1fr_1.4fr]">
    <v-card flat class="card-shell border-0">
      <v-card-title class="flex items-center justify-between gap-2">
        <span>{{ providerForm.id ? "编辑模型服务" : "添加模型服务" }}</span>
        <v-btn v-if="providerForm.id" size="x-small" variant="text" @click="newProviderForm">
          新建
        </v-btn>
      </v-card-title>
      <v-card-text class="grid gap-3">
        <v-text-field v-model="providerForm.displayName" label="名称" density="comfortable" />
        <v-text-field v-model="providerForm.baseUrl" label="服务地址" density="comfortable" />
        <v-text-field v-model="providerForm.model" label="默认模型" density="comfortable" />
        <v-text-field
          v-model="providerForm.apiKey"
          label="API 密钥"
          type="password"
          density="comfortable"
          :hint="providerForm.id ? '留空则保留原密钥' : ''"
          persistent-hint
        />
        <v-switch v-model="providerForm.enabled" label="启用" color="primary" hide-details />
        <v-btn color="primary" block @click="saveProvider">保存模型服务</v-btn>
      </v-card-text>
    </v-card>

    <div class="grid auto-rows-max gap-3">
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
                <span class="font-semibold text-slate-900">{{ provider.displayName }}</span>
                <v-chip
                  size="x-small"
                  :color="provider.enabled ? 'success' : 'default'"
                  variant="tonal"
                >
                  {{ provider.enabled ? "启用" : "停用" }}
                </v-chip>
              </div>
              <div class="mt-0.5 text-xs text-slate-500">{{ provider.baseUrl }} · {{ provider.model }}</div>
              <div class="text-xs text-slate-500">密钥：{{ provider.hasApiKey ? "已配置" : "未配置" }}</div>
              <div v-if="provider.capabilities" class="mt-1 flex flex-wrap gap-1">
                <v-chip
                  v-for="(supported, capability) in provider.capabilities"
                  :key="capability"
                  size="x-small"
                  :color="supported ? 'success' : 'default'"
                  variant="tonal"
                >
                  {{ capability }}：{{ supported ? "支持" : "不支持" }}
                </v-chip>
              </div>
            </div>
            <div class="flex shrink-0 gap-2">
              <v-btn size="x-small" variant="outlined" @click="editProvider(provider)">编辑</v-btn>
              <v-btn size="x-small" variant="outlined" @click="testProvider(provider.id)">测试</v-btn>
              <v-btn size="x-small" variant="outlined" color="error" @click="deleteProvider(provider.id)">删除</v-btn>
            </div>
          </div>
        </v-card-text>
      </v-card>
      <div v-if="providers.length === 0" class="text-sm text-slate-500">
        尚未配置任何模型服务。
      </div>
    </div>
  </section>
</template>
