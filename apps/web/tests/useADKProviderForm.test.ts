// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import type { ADKProvider } from "../src/contracts";
import {
  deleteADKProvider,
  saveADKProvider,
  setADKDefaultProvider,
  testADKProvider,
} from "../src/composables/adkSettingsApi";
import { useADKProviderForm } from "../src/composables/useADKProviderForm";

vi.mock("../src/composables/adkSettingsApi", () => ({
  deleteADKProvider: vi.fn(),
  saveADKProvider: vi.fn(),
  setADKDefaultProvider: vi.fn(),
  testADKProvider: vi.fn(),
}));

const provider: ADKProvider = {
  id: "private-gateway",
  displayName: "Private Gateway",
  baseUrl: "https://llm.example/v1",
  model: "reasoning-large",
  contextWindowTokens: 128_000,
  requestTimeoutMs: 245_500,
  enabled: true,
  default: false,
  hasApiKey: true,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useADKProviderForm", () => {
  it("normalizes token and timeout inputs before saving operational settings", async () => {
    vi.mocked(saveADKProvider).mockResolvedValue(provider);
    const state = createState();
    state.providerForm.value = {
      ...state.providerForm.value,
      displayName: "Private Gateway",
      contextWindowTokens: 127_999.6,
      requestTimeoutSeconds: 0,
      apiKey: "secret",
    };

    await state.saveProvider();

    expect(saveADKProvider).toHaveBeenCalledWith(
      expect.objectContaining({
        contextWindowTokens: 128_000,
        requestTimeoutMs: 1,
        apiKey: "secret",
      }),
    );
    expect(state.providerForm.value.id).toBe("private-gateway");
    expect(state.providerForm.value.apiKey).toBe("");
    expect(state.successMessage.value).toBe("Provider 已保存");
    expect(state.refreshAll).toHaveBeenCalledOnce();
  });

  it("restores persisted provider settings without exposing its API key", () => {
    const state = createState();

    state.editProvider(provider);

    expect(state.providerForm.value).toEqual({
      id: "private-gateway",
      displayName: "Private Gateway",
      baseUrl: "https://llm.example/v1",
      model: "reasoning-large",
      contextWindowTokens: 128_000,
      requestTimeoutSeconds: 246,
      apiKey: "",
      enabled: true,
    });

    state.newProviderForm();
    expect(state.providerForm.value).toMatchObject({
      id: "",
      baseUrl: "https://api.openai.com/v1",
      requestTimeoutSeconds: 180,
      apiKey: "",
    });
  });

  it("reports provider health replies and updates the default provider", async () => {
    vi.mocked(testADKProvider).mockResolvedValue({ reply: "model ready" });
    vi.mocked(setADKDefaultProvider).mockResolvedValue({ ...provider, default: true });
    const state = createState();

    await state.testProvider("private-gateway");
    expect(state.successMessage.value).toBe("Provider 测试成功：model ready");

    await state.setDefaultProvider("private-gateway");
    expect(state.successMessage.value).toBe("默认模型已更新");
    expect(state.refreshAll).toHaveBeenCalledOnce();
  });

  it("uses a stable health-check message when the provider omits a reply", async () => {
    vi.mocked(testADKProvider).mockResolvedValue({});
    const state = createState();

    await state.testProvider("private-gateway");

    expect(state.successMessage.value).toBe("Provider 测试成功：ok");
  });

  it("refreshes after deletion and surfaces service failures", async () => {
    const state = createState();
    await state.deleteProvider("private-gateway");
    expect(deleteADKProvider).toHaveBeenCalledWith("private-gateway");
    expect(state.refreshAll).toHaveBeenCalledOnce();

    vi.mocked(saveADKProvider).mockRejectedValueOnce(new Error("duplicate provider"));
    await state.saveProvider();
    expect(state.errorMessage.value).toBe("duplicate provider");

    vi.mocked(testADKProvider).mockRejectedValueOnce("network down");
    await state.testProvider("private-gateway");
    expect(state.errorMessage.value).toBe("测试失败");

    vi.mocked(deleteADKProvider).mockRejectedValueOnce("locked");
    await state.deleteProvider("private-gateway");
    expect(state.errorMessage.value).toBe("删除失败");

    vi.mocked(setADKDefaultProvider).mockRejectedValueOnce(new Error("provider disabled"));
    await state.setDefaultProvider("private-gateway");
    expect(state.errorMessage.value).toBe("provider disabled");
  });

  it("treats refresh failure as an incomplete save instead of reporting stale state", async () => {
    vi.mocked(saveADKProvider).mockResolvedValue(provider);
    const state = createState();
    state.refreshAll.mockRejectedValueOnce(new Error("refresh failed"));

    await state.saveProvider();

    expect(state.errorMessage.value).toBe("refresh failed");
  });
});

function createState() {
  const successMessage = ref("");
  const errorMessage = ref("");
  const refreshAll = vi.fn(async () => {});
  return {
    ...useADKProviderForm(refreshAll, successMessage, errorMessage),
    refreshAll,
    successMessage,
    errorMessage,
  };
}
