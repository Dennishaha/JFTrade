import { ref } from "vue";

import type { ADKProvider } from "@jftrade/ui-contracts";

import {
  deleteADKProvider,
  saveADKProvider,
  testADKProvider,
} from "./adkSettingsApi";

function createProviderForm() {
  return {
    id: "",
    displayName: "OpenAI Compatible",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
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

  async function saveProvider(): Promise<void> {
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
      providerForm.value.id = provider.id;
      providerForm.value.apiKey = "";
      successMessage.value = "Provider 已保存";
      await refreshAll();
    } catch (error) {
      errorMessage.value = error instanceof Error ? error.message : "保存失败";
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

  function newProviderForm(): void {
    providerForm.value = createProviderForm();
  }

  function editProvider(provider: ADKProvider): void {
    providerForm.value = {
      id: provider.id,
      displayName: provider.displayName,
      baseUrl: provider.baseUrl,
      model: provider.model,
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
    newProviderForm,
    editProvider,
  };
}
