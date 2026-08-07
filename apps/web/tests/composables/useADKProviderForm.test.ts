// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import type { ADKProvider } from "../../src/types";
import {
  deleteADKProvider,
  saveADKProvider,
  setADKDefaultProvider,
  testADKProvider,
} from "@/composables/adk/adkSettingsApi";
import { useADKProviderForm } from "@/composables/adk/useADKProviderForm";

vi.mock("@/composables/adk/adkSettingsApi", () => ({
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
  apiProtocol: "responses",
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

    const saved = await state.saveProvider();

    expect(saved).toBe(true);
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
      apiProtocol: "responses",
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

  it("defaults incomplete legacy provider payloads to Chat Completions", () => {
    const state = createState();
    const legacyProvider = {
      ...provider,
      apiProtocol: undefined,
      contextWindowTokens: undefined,
      requestTimeoutMs: undefined,
    } as unknown as ADKProvider;

    state.editProvider(legacyProvider);

    expect(state.providerForm.value.apiProtocol).toBe("chat_completions");
    expect(state.providerForm.value.contextWindowTokens).toBe(0);
    expect(state.providerForm.value.requestTimeoutSeconds).toBe(180);
  });

  it("reports provider test completion and updates the default provider", async () => {
    vi.mocked(testADKProvider).mockResolvedValue({ reply: "model ready" });
    vi.mocked(setADKDefaultProvider).mockResolvedValue({ ...provider, default: true });
    const state = createState();

    const feedback = await state.testProvider("private-gateway");
    expect(feedback).toEqual({ ok: true, message: "Provider 测试成功" });
    expect(state.successMessage.value).toBe("Provider 测试成功");

    await state.setDefaultProvider("private-gateway");
    expect(state.successMessage.value).toBe("默认模型已更新");
    expect(state.refreshAll).toHaveBeenCalledOnce();
  });

  it("uses a stable health-check message when the provider omits a reply", async () => {
    vi.mocked(testADKProvider).mockResolvedValue({});
    const state = createState();

    const feedback = await state.testProvider("private-gateway");

    expect(feedback).toEqual({ ok: true, message: "Provider 测试成功" });
    expect(state.successMessage.value).toBe("Provider 测试成功");
  });

  it("refreshes after deletion and surfaces service failures", async () => {
    const state = createState();
    await state.deleteProvider("private-gateway");
    expect(deleteADKProvider).toHaveBeenCalledWith("private-gateway");
    expect(state.refreshAll).toHaveBeenCalledOnce();

    vi.mocked(saveADKProvider).mockRejectedValueOnce(new Error("duplicate provider"));
    state.successMessage.value = "stale success";
    expect(await state.saveProvider()).toBe(false);
    expect(state.errorMessage.value).toBe("duplicate provider");
    expect(state.successMessage.value).toBe("");

    vi.mocked(testADKProvider).mockRejectedValueOnce("network down");
    expect(await state.testProvider("private-gateway")).toEqual({
      ok: false,
      message: "测试失败",
    });
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

    const saved = await state.saveProvider();

    expect(saved).toBe(false);
    expect(state.errorMessage.value).toBe(
      "Provider 已保存，但刷新界面失败：refresh failed",
    );
    expect(state.successMessage.value).toBe("");
  });

  it("keeps non-Error save failures distinct before and after a provider write", async () => {
    const state = createState();

    vi.mocked(saveADKProvider).mockRejectedValueOnce("offline");
    expect(await state.saveProvider()).toBe(false);
    expect(state.errorMessage.value).toBe("保存失败");

    vi.mocked(saveADKProvider).mockResolvedValueOnce(provider);
    state.refreshAll.mockRejectedValueOnce(undefined);
    expect(await state.saveProvider()).toBe(false);
    expect(state.errorMessage.value).toBe(
      "Provider 已保存，但刷新界面失败：未知错误",
    );
  });

  it("preserves Error messages from provider test failures", async () => {
    const state = createState();
    vi.mocked(testADKProvider).mockRejectedValueOnce(new Error("probe timed out"));

    expect(await state.testProvider("private-gateway")).toEqual({
      ok: false,
      message: "probe timed out",
    });
    expect(state.errorMessage.value).toBe("probe timed out");
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
