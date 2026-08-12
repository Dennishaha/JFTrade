import { ref, watch } from "vue";

import type {
  ADKProvider,
  ADKProviderAPIProtocol,
  ADKProviderTestMode,
  ADKProviderTestResponse,
  ADKReasoningEffort,
} from "@/types";

import {
  deleteADKProvider,
  saveADKProvider,
  setADKDefaultProvider,
  testADKProvider,
} from "@/composables/adk/adkSettingsApi";
import {
  ADK_REASONING_EFFORTS,
  ADK_REASONING_EFFORT_LABELS,
  defaultADKProviderReasoningConfig,
  normalizedADKProviderReasoningConfig,
} from "@/composables/adk/adkReasoning";

const DEFAULT_ENABLED_REASONING_EFFORTS = new Set<ADKReasoningEffort>([
  "medium",
  "high",
]);

type ADKProviderForm = {
  id: string;
  displayName: string;
  baseUrl: string;
  model: string;
  apiProtocol: ADKProviderAPIProtocol;
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

function createProviderForm(): ADKProviderForm {
  const reasoningConfig = defaultADKProviderReasoningConfig("chat_completions");
  return {
    id: "",
    displayName: "OpenAI Compatible",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
    apiProtocol: "chat_completions",
    reasoningRequestField: reasoningConfig.requestField,
    reasoningMappings: ADK_REASONING_EFFORTS.map((effort) => ({
      effort,
      value: effort,
      enabled: DEFAULT_ENABLED_REASONING_EFFORTS.has(effort),
    })),
    contextWindowTokens: 0,
    requestTimeoutSeconds: 180,
    apiKey: "",
    enabled: true,
  };
}

export function useADKProviderForm(
  refreshAll: () => Promise<void>,
  successMessage: { value: string },
  errorMessage: { value: string },
) {
  const providerForm = ref(createProviderForm());

  watch(
    () => providerForm.value.apiProtocol,
    (protocol, previousProtocol) => {
      const currentField = providerForm.value.reasoningRequestField.trim();
      const previousDefault = defaultADKProviderReasoningConfig(
        previousProtocol,
      ).requestField;
      if (currentField === "" || currentField === previousDefault) {
        providerForm.value.reasoningRequestField =
          defaultADKProviderReasoningConfig(protocol).requestField;
      }
    },
  );

  async function saveProvider(): Promise<boolean> {
    successMessage.value = "";
    errorMessage.value = "";
    const invalidMapping = providerForm.value.reasoningMappings.find(
      (mapping) => mapping.enabled && mapping.value.trim() === "",
    );
    if (invalidMapping) {
      errorMessage.value = `${ADK_REASONING_EFFORT_LABELS[invalidMapping.effort]}思考等级缺少 Provider 值`;
      return false;
    }
    let providerSaved = false;
    try {
      const {
        reasoningRequestField,
        reasoningMappings,
        ...providerFields
      } = providerForm.value;
      const provider = await saveADKProvider({
        ...providerFields,
        reasoningConfig: {
          requestField: reasoningRequestField.trim(),
          mappings: reasoningMappings
            .filter((mapping) => mapping.enabled)
            .map(({ effort, value }) => ({ effort, value: value.trim() })),
        },
        contextWindowTokens: Math.max(
          0,
          Math.round(Number(providerForm.value.contextWindowTokens || 0)),
        ),
        requestTimeoutMs: Math.max(
          1,
          Math.round(
            Number(providerForm.value.requestTimeoutSeconds || 0) * 1000,
          ),
        ),
      });
      providerSaved = true;
      providerForm.value.id = provider.id;
      providerForm.value.apiKey = "";
      await refreshAll();
      successMessage.value = "Provider 已保存";
      return true;
    } catch (error) {
      successMessage.value = "";
      const message =
        error instanceof Error
          ? error.message
          : providerSaved
            ? "未知错误"
            : "保存失败";
      errorMessage.value = providerSaved
        ? `Provider 已保存，但刷新界面失败：${message}`
        : message;
      return false;
    }
  }

  async function testProvider(
    providerId: string,
    mode: ADKProviderTestMode = "quick",
  ): Promise<ADKProviderTestResponse> {
    successMessage.value = "";
    errorMessage.value = "";
    try {
      const result = await testADKProvider(providerId, mode);
      if (result.ok) {
        successMessage.value = mode === "full" ? "推理映射完整验证成功" : "Provider 快速测试成功";
      } else {
        errorMessage.value = "Provider 基础连接成功，但推理映射验证失败";
      }
      return result;
    } catch (error) {
      const message = error instanceof Error ? error.message : "测试失败";
      errorMessage.value = message;
      throw error;
    }
  }

  async function deleteProvider(providerId: string): Promise<void> {
    try {
      await deleteADKProvider(providerId);
      await refreshAll();
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "删除失败";
    }
  }

  async function setDefaultProvider(providerId: string): Promise<void> {
    try {
      await setADKDefaultProvider(providerId);
      successMessage.value = "默认模型已更新";
      await refreshAll();
    } catch (error) {
      errorMessage.value =
        error instanceof Error ? error.message : "设置默认模型失败";
    }
  }

  function newProviderForm(): void {
    providerForm.value = createProviderForm();
  }

  function editProvider(provider: ADKProvider): void {
    const reasoningConfig = normalizedADKProviderReasoningConfig(provider);
    providerForm.value = {
      id: provider.id,
      displayName: provider.displayName,
      baseUrl: provider.baseUrl,
      model: provider.model,
      apiProtocol: provider.apiProtocol ?? "chat_completions",
      reasoningRequestField: reasoningConfig.requestField,
      reasoningMappings: ADK_REASONING_EFFORTS.map((effort) => {
        const mapping = reasoningConfig.mappings.find((item) => item.effort === effort);
        return {
          effort,
          value: mapping?.value ?? effort,
          enabled: Boolean(mapping),
        };
      }),
      contextWindowTokens: provider.contextWindowTokens ?? 0,
      requestTimeoutSeconds: Math.max(
        1,
        Math.round((provider.requestTimeoutMs ?? 180_000) / 1000),
      ),
      apiKey: "",
      enabled: provider.enabled,
    };
  }

  return {
    providerForm,
    saveProvider,
    testProvider,
    deleteProvider,
    setDefaultProvider,
    newProviderForm,
    editProvider,
  };
}
