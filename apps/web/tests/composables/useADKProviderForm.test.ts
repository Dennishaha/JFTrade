// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick, ref } from "vue";

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
  contextWindowTokens: 128_000,
  requestTimeoutMs: 245_500,
  enabled: true,
  default: false,
  hasApiKey: true,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

const successfulProviderTest = {
  ok: true,
  reply: "model ready",
  capabilities: { streaming: true, tools: true, reasoning: true },
  reasoning: {
    mode: "quick" as const,
    requestField: "reasoning.effort",
    ok: true,
    results: [{ effort: "xhigh" as const, value: "xhigh", ok: true }],
  },
  checkedAt: "2026-08-11T00:00:00Z",
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
        reasoningConfig: {
          requestField: "reasoning.effort",
          mappings: [
            { effort: "medium", value: "medium" },
            { effort: "high", value: "high" },
          ],
        },
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
      reasoningRequestField: "reasoning.effort",
      reasoningMappings: [
        { effort: "low", enabled: false, value: "low" },
        { effort: "medium", enabled: false, value: "medium" },
        { effort: "high", enabled: false, value: "high" },
        { effort: "xhigh", enabled: false, value: "xhigh" },
        { effort: "max", enabled: false, value: "max" },
      ],
    });

    state.newProviderForm();
    expect(state.providerForm.value).toMatchObject({
      id: "",
      baseUrl: "https://api.openai.com/v1",
      requestTimeoutSeconds: 180,
      apiKey: "",
    });
    expect(
      state.providerForm.value.reasoningMappings
        .filter((mapping) => mapping.enabled)
        .map((mapping) => mapping.effort),
    ).toEqual(["medium", "high"]);
  });

  it("round-trips only the persisted reasoning mappings when editing", async () => {
    const state = createState();

    state.editProvider({
      ...provider,
      reasoningConfig: {
        requestField: "vendor.thinking_effort",
        mappings: [
          { effort: "high", value: "deep" },
          { effort: "max", value: "maximum" },
        ],
      },
    });
    await nextTick();

    expect(state.providerForm.value.reasoningRequestField).toBe(
      "vendor.thinking_effort",
    );
    expect(state.providerForm.value.reasoningMappings).toEqual([
      { effort: "low", enabled: false, value: "low" },
      { effort: "medium", enabled: false, value: "medium" },
      { effort: "high", enabled: true, value: "deep" },
      { effort: "xhigh", enabled: false, value: "xhigh" },
      { effort: "max", enabled: true, value: "maximum" },
    ]);
  });

  it("rejects enabled empty mappings before issuing a save request", async () => {
    const state = createState();
    const medium = state.providerForm.value.reasoningMappings.find(
      (mapping) => mapping.effort === "medium",
    )!;
    medium.value = "   ";

    expect(await state.saveProvider()).toBe(false);
    expect(saveADKProvider).not.toHaveBeenCalled();
    expect(state.errorMessage.value).toBe("中思考等级缺少 Provider 值");
  });

  it("allows users to save an explicitly empty reasoning mapping list", async () => {
    vi.mocked(saveADKProvider).mockResolvedValue(provider);
    const state = createState();
    for (const mapping of state.providerForm.value.reasoningMappings) {
      mapping.enabled = false;
    }

    expect(await state.saveProvider()).toBe(true);
    expect(saveADKProvider).toHaveBeenCalledWith(
      expect.objectContaining({
        reasoningConfig: {
          requestField: "reasoning.effort",
          mappings: [],
        },
      }),
    );
  });

  it("defaults incomplete provider payloads to Responses with no mappings", () => {
    const state = createState();
    const legacyProvider = {
      ...provider,
      contextWindowTokens: undefined,
      requestTimeoutMs: undefined,
    } as unknown as ADKProvider;

    state.editProvider(legacyProvider);

    expect(state.providerForm.value.contextWindowTokens).toBe(0);
    expect(state.providerForm.value.requestTimeoutSeconds).toBe(180);
    expect(state.providerForm.value.reasoningMappings.every((mapping) => !mapping.enabled)).toBe(true);
  });

  it("reports provider test completion and updates the default provider", async () => {
    vi.mocked(testADKProvider).mockResolvedValue(successfulProviderTest);
    vi.mocked(setADKDefaultProvider).mockResolvedValue({ ...provider, default: true });
    const state = createState();

    const feedback = await state.testProvider("private-gateway");
    expect(feedback).toEqual(successfulProviderTest);
    expect(state.successMessage.value).toBe("Provider 快速测试成功");
    expect(testADKProvider).toHaveBeenCalledWith("private-gateway", "quick");

    await state.setDefaultProvider("private-gateway");
    expect(state.successMessage.value).toBe("默认模型已更新");
    expect(state.refreshAll).toHaveBeenCalledOnce();
  });

  it("surfaces partial reasoning failures without discarding per-level results", async () => {
    const partialResult = {
      ...successfulProviderTest,
      ok: false,
      reasoning: {
        ...successfulProviderTest.reasoning,
        ok: false,
        results: [{ effort: "max" as const, value: "max", ok: false, error: "unsupported" }],
      },
    };
    vi.mocked(testADKProvider).mockResolvedValue(partialResult);
    const state = createState();

    const feedback = await state.testProvider("private-gateway");

    expect(feedback).toEqual(partialResult);
    expect(state.successMessage.value).toBe("");
    expect(state.errorMessage.value).toContain("推理映射验证失败");
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
    await expect(state.testProvider("private-gateway")).rejects.toBe("network down");
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

    await expect(state.testProvider("private-gateway")).rejects.toThrow("probe timed out");
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
