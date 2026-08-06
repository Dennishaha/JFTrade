import { ref } from "vue";

import type { ADKProvider, ADKProviderAPIProtocol } from "@/types";

import {
  deleteADKProvider,
  saveADKProvider,
  setADKDefaultProvider,
  testADKProvider,
} from "@/composables/adk/adkSettingsApi";

type ADKProviderForm = {
  id: string;
  displayName: string;
  baseUrl: string;
  model: string;
  apiProtocol: ADKProviderAPIProtocol;
  contextWindowTokens: number;
  requestTimeoutSeconds: number;
  apiKey: string;
  enabled: boolean;
};

function createProviderForm(): ADKProviderForm {
  return {
    id: "",
    displayName: "OpenAI Compatible",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
    apiProtocol: "chat_completions",
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

  async function saveProvider(): Promise<boolean> {
    successMessage.value = "";
    errorMessage.value = "";
    let providerSaved = false;
    try {
      const provider = await saveADKProvider({
        ...providerForm.value,
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

  async function testProvider(providerId: string): Promise<void> {
    try {
      const result = await testADKProvider(providerId);
      successMessage.value = `Provider 测试成功：${String(result.reply ?? "ok")}`;
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "测试失败";
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
    providerForm.value = {
      id: provider.id,
      displayName: provider.displayName,
      baseUrl: provider.baseUrl,
      model: provider.model,
      apiProtocol: provider.apiProtocol ?? "chat_completions",
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
