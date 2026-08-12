<script setup lang="ts">
import { computed, ref } from "vue";

import type {
  ADKProvider,
  ADKProviderTestMode,
  ADKProviderTestResponse,
  ADKReasoningEffort,
} from "@/types";
import {
  ADK_REASONING_EFFORT_LABELS,
  defaultADKProviderReasoningConfig,
} from "@/composables/adk/adkReasoning";

const props = defineProps<{
  providerForm: {
    id: string;
    displayName: string;
    baseUrl: string;
    model: string;
    apiProtocol: "chat_completions" | "responses";
    reasoningRequestField: string;
    reasoningMappings: Array<{
      effort: ADKReasoningEffort;
      value: string;
      enabled: boolean;
    }>;
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
  saveProvider: () => Promise<boolean>;
  saveRuntimeSettings: () => void | Promise<void>;
  newProviderForm: () => void;
  editProvider: (provider: ADKProvider) => void;
  testProvider: (
    providerId: string,
    mode: ADKProviderTestMode,
  ) => Promise<ADKProviderTestResponse>;
  deleteProvider: (providerId: string) => void | Promise<void>;
  setDefaultProvider: (providerId: string) => void | Promise<void>;
}>();

const providerDialogOpen = ref(false);
const reasoningAdvancedOpen = ref(false);
const testingProviderId = ref("");
const testingMode = ref<ADKProviderTestMode | "">("");
const fullTestProviderId = ref("");
const providerTestFeedback = ref<{
  providerId: string;
  result?: ADKProviderTestResponse;
  error?: string;
} | null>(null);
const enabledReasoningMappings = computed(() =>
  props.providerForm.reasoningMappings.filter((mapping) => mapping.enabled),
);

function openNewProviderDialog(): void {
  props.newProviderForm();
  reasoningAdvancedOpen.value = false;
  providerDialogOpen.value = true;
}

function openEditProviderDialog(provider: ADKProvider): void {
  props.editProvider(provider);
  reasoningAdvancedOpen.value = false;
  providerDialogOpen.value = true;
}

async function submitProviderForm(): Promise<void> {
  if (await props.saveProvider()) {
    providerDialogOpen.value = false;
  }
}

async function runProviderTest(
  providerId: string,
  mode: ADKProviderTestMode,
): Promise<void> {
  if (testingProviderId.value !== "") return;
  providerTestFeedback.value = null;
  testingProviderId.value = providerId;
  testingMode.value = mode;
  try {
    const result = await props.testProvider(providerId, mode);
    providerTestFeedback.value = { providerId, result };
  } catch (error) {
    providerTestFeedback.value = {
      providerId,
      error: error instanceof Error ? error.message : "测试失败",
    };
  } finally {
    testingProviderId.value = "";
    testingMode.value = "";
  }
}

async function confirmFullProviderTest(): Promise<void> {
  const providerId = fullTestProviderId.value;
  fullTestProviderId.value = "";
  if (providerId !== "") await runProviderTest(providerId, "full");
}

function useCommonReasoningEfforts(): void {
  for (const mapping of props.providerForm.reasoningMappings) {
    mapping.enabled = mapping.effort === "medium" || mapping.effort === "high";
    if (mapping.enabled) mapping.value = mapping.effort;
  }
}

function disableReasoningEfforts(): void {
  for (const mapping of props.providerForm.reasoningMappings) {
    mapping.enabled = false;
  }
}

function resetReasoningRequestField(): void {
  props.providerForm.reasoningRequestField = defaultADKProviderReasoningConfig(
    props.providerForm.apiProtocol,
  ).requestField;
}

function reasoningMappingError(mapping: {
  enabled: boolean;
  value: string;
}): string {
  return mapping.enabled && mapping.value.trim() === ""
    ? "请输入 Provider 值"
    : "";
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
                  协议：{{ provider.apiProtocol === "responses" ? "Responses" : "Chat Completions" }}
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
                <div class="text-xs text-slate-500">
                  推理字段：{{ provider.reasoningConfig?.requestField || "未配置" }}
                </div>
                <div
                  v-if="provider.reasoningConfig"
                  class="mt-1 flex flex-wrap gap-1"
                >
                  <v-chip
                    v-for="mapping in provider.reasoningConfig.mappings"
                    :key="mapping.effort"
                    size="x-small"
                    color="primary"
                    variant="tonal"
                  >
                    {{ ADK_REASONING_EFFORT_LABELS[mapping.effort] }} · {{ mapping.value }}
                  </v-chip>
                  <span
                    v-if="provider.reasoningConfig.mappings.length === 0"
                    class="text-xs text-slate-400"
                  >
                    不支持显式推理等级
                  </span>
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
                  :loading="testingProviderId === provider.id && testingMode === 'quick'"
                  :disabled="testingProviderId !== ''"
                  @click="runProviderTest(provider.id, 'quick')"
                  >快速测试</v-btn
                >
                <v-btn
                  size="x-small"
                  variant="outlined"
                  :loading="testingProviderId === provider.id && testingMode === 'full'"
                  :disabled="testingProviderId !== ''"
                  @click="fullTestProviderId = provider.id"
                  >完整验证</v-btn
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
            <v-alert
              v-if="providerTestFeedback?.providerId === provider.id"
              :type="providerTestFeedback.error ? 'error' : providerTestFeedback.result?.ok ? 'success' : 'warning'"
              variant="tonal"
              density="compact"
              closable
              class="mt-3"
              @click:close="providerTestFeedback = null"
            >
              <div v-if="providerTestFeedback.error">
                {{ providerTestFeedback.error }}
              </div>
              <template v-else-if="providerTestFeedback.result">
                <div>
                  {{ providerTestFeedback.result.reasoning.mode === "full" ? "完整验证" : "快速测试" }}
                  {{ providerTestFeedback.result.ok ? "通过" : "存在失败档位" }}
                  <span v-if="providerTestFeedback.result.reasoning.requestField">
                    · {{ providerTestFeedback.result.reasoning.requestField }}
                  </span>
                </div>
                <div v-if="providerTestFeedback.result.reply" class="mt-1 text-xs">
                  连接回复：{{ providerTestFeedback.result.reply }}
                </div>
                <div class="mt-2 flex flex-wrap gap-1">
                  <v-chip
                    v-for="(supported, capability) in providerTestFeedback.result.capabilities"
                    :key="capability"
                    size="x-small"
                    :color="supported ? 'success' : 'default'"
                    variant="tonal"
                  >
                    {{ capability }} · {{ supported ? "支持" : "不支持" }}
                  </v-chip>
                </div>
                <div
                  v-if="providerTestFeedback.result.reasoning.results.length === 0"
                  class="mt-2 text-xs"
                >
                  未配置可调推理档位，本次未发送推理测试请求。
                </div>
                <div v-else class="mt-2 grid gap-1 text-xs">
                  <div
                    v-for="result in providerTestFeedback.result.reasoning.results"
                    :key="result.effort"
                    class="flex flex-wrap items-center gap-2"
                  >
                    <span class="font-medium">
                      {{ ADK_REASONING_EFFORT_LABELS[result.effort] }} ({{ result.value }})
                    </span>
                    <v-chip
                      size="x-small"
                      :color="result.ok ? 'success' : 'error'"
                      variant="tonal"
                    >
                      {{ result.ok ? "成功" : "失败" }}
                    </v-chip>
                    <span v-if="result.error" class="text-red-700">{{ result.error }}</span>
                  </div>
                </div>
              </template>
            </v-alert>
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
          <v-select
            v-model="providerForm.apiProtocol"
            label="API 协议"
            :items="[
              { title: 'Chat Completions', value: 'chat_completions' },
              { title: 'Responses', value: 'responses' },
            ]"
            density="comfortable"
          />
          <v-card
            flat
            class="border border-slate-200"
            data-testid="provider-reasoning-settings"
          >
            <v-card-title class="flex flex-wrap items-center justify-between gap-2 text-sm font-semibold">
              <span>思考等级</span>
              <span class="text-xs font-normal text-slate-500">
                {{ enabledReasoningMappings.length }} 档已启用
              </span>
            </v-card-title>
            <v-card-text class="grid gap-3">
              <div
                v-if="enabledReasoningMappings.length > 0"
                class="flex flex-wrap gap-1"
                aria-label="已启用思考等级"
              >
                <v-chip
                  v-for="mapping in enabledReasoningMappings"
                  :key="mapping.effort"
                  size="x-small"
                  color="primary"
                  variant="tonal"
                >
                  {{ ADK_REASONING_EFFORT_LABELS[mapping.effort] }} · {{ mapping.value }}
                </v-chip>
              </div>
              <v-alert
                v-else
                type="info"
                variant="tonal"
                density="compact"
              >
                不发送显式思考等级，将使用 Provider 或模型默认行为。
              </v-alert>
              <div
                v-for="mapping in providerForm.reasoningMappings"
                :key="mapping.effort"
                class="grid items-start gap-2 sm:grid-cols-[auto_7rem_minmax(0,1fr)]"
              >
                <v-switch
                  v-model="mapping.enabled"
                  :label="ADK_REASONING_EFFORT_LABELS[mapping.effort]"
                  color="primary"
                  density="compact"
                  hide-details
                />
                <span class="pt-3 text-xs text-slate-500">{{ mapping.effort }}</span>
                <v-text-field
                  v-model="mapping.value"
                  :label="`${mapping.effort} 的 Provider 值`"
                  density="compact"
                  :disabled="!mapping.enabled"
                  :error-messages="reasoningMappingError(mapping)"
                  hide-details="auto"
                />
              </div>
              <div class="flex flex-wrap gap-2">
                <v-btn variant="text" size="small" @click="useCommonReasoningEfforts">
                  使用常用中高档
                </v-btn>
                <v-btn variant="text" size="small" @click="disableReasoningEfforts">
                  全部关闭
                </v-btn>
              </div>
              <div class="rounded border border-slate-200 px-3 py-2">
                <v-btn
                  variant="text"
                  size="small"
                  :aria-expanded="reasoningAdvancedOpen"
                  @click="reasoningAdvancedOpen = !reasoningAdvancedOpen"
                >
                  高级配置
                </v-btn>
                <div
                  v-show="reasoningAdvancedOpen"
                  class="mt-3 grid gap-2"
                  data-testid="provider-reasoning-advanced"
                >
                  <v-text-field
                    v-model="providerForm.reasoningRequestField"
                    label="请求字段路径"
                    density="comfortable"
                    hint="例如 reasoning.effort 或 reasoning_effort；只允许对象字段点路径"
                    persistent-hint
                  />
                  <v-btn
                    variant="text"
                    size="small"
                    class="justify-self-start"
                    @click="resetReasoningRequestField"
                  >
                    恢复协议默认字段
                  </v-btn>
                </div>
              </div>
            </v-card-text>
          </v-card>
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

    <v-dialog :model-value="fullTestProviderId !== ''" max-width="480">
      <v-card>
        <v-card-title>完整验证推理映射</v-card-title>
        <v-card-text class="text-sm text-slate-600">
          将串行调用模型验证全部已配置推理档位，可能耗时较长并产生相应模型调用费用。
        </v-card-text>
        <v-card-actions class="justify-end gap-2">
          <v-btn variant="text" @click="fullTestProviderId = ''">取消</v-btn>
          <v-btn color="primary" @click="confirmFullProviderTest">继续验证</v-btn>
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
