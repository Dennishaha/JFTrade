// @vitest-environment jsdom

import { flushPromises, mount } from "@vue/test-utils";
import { defineComponent, h, nextTick, ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  fetchSnapshot: vi.fn(),
  fetchMCP: vi.fn(),
  fetchRuns: vi.fn(),
  fetchApprovals: vi.fn(),
  fetchAudit: vi.fn(),
  saveMCP: vi.fn(),
  resetMCPToken: vi.fn(),
}));

vi.mock("../src/composables/adkSettingsApi", async () => {
  const actual = await vi.importActual<typeof import("../src/composables/adkSettingsApi")>(
    "../src/composables/adkSettingsApi",
  );
  return {
    ...actual,
    fetchADKSettingsSnapshot: api.fetchSnapshot,
    fetchMCPServerSettings: api.fetchMCP,
    fetchADKRunsPage: api.fetchRuns,
    fetchADKApprovalsPage: api.fetchApprovals,
    fetchADKAuditPage: api.fetchAudit,
    saveMCPServerSettings: api.saveMCP,
    resetMCPServerToken: api.resetMCPToken,
  };
});

vi.mock("../src/composables/useADKProviderForm", async () => {
  const { ref } = await import("vue");
  return {
    useADKProviderForm: () => ({
      providerForm: ref({}),
      deleteProvider: vi.fn(),
      editProvider: vi.fn(),
      newProviderForm: vi.fn(),
      saveProvider: vi.fn(),
      setDefaultProvider: vi.fn(),
      testProvider: vi.fn(),
    }),
  };
});

vi.mock("../src/composables/useADKAgentForm", async () => {
  const { ref } = await import("vue");
  return {
    useADKAgentForm: () => ({
      agentForm: ref({ id: "", skills: [] }),
      deleteAgent: vi.fn(),
      duplicateAgent: vi.fn(),
      editAgent: vi.fn(),
      newAgentForm: vi.fn(),
      saveAgent: vi.fn(),
    }),
  };
});

import { useADKSettingsSectionState } from "../src/composables/useADKSettingsSectionState";

function mcpSnapshot(port = 6697) {
  return {
    settings: { enabled: false, port, authMode: "token", tokenConfigured: true },
    status: { running: false, endpoint: `http://127.0.0.1:${port}/mcp` },
  };
}

function mountState() {
  let state: ReturnType<typeof useADKSettingsSectionState> | null = null;
  const Host = defineComponent({
    setup() {
      state = useADKSettingsSectionState();
      return () => h("div");
    },
  });
  mount(Host);
  if (state == null) throw new Error("state was not initialized");
  return state;
}

beforeEach(() => {
  vi.clearAllMocks();
  api.fetchSnapshot.mockResolvedValue({
    providers: [],
    agents: [],
    tools: [],
    skills: [],
    runtimeSettings: { runTimeoutMs: 1_800_000, streamIdleTimeoutMs: 300_000 },
    optimizationTasks: [],
    tasks: [],
    memoryEntries: [],
    agentTemplates: [],
    metrics: null,
  });
  api.fetchMCP.mockResolvedValue(mcpSnapshot());
  api.fetchRuns.mockResolvedValue({ runs: [], page: { limit: 20, offset: 0, total: 0, returned: 0, hasMore: false } });
  api.fetchApprovals.mockResolvedValue({ approvals: [], page: { limit: 10, offset: 0, total: 0, returned: 0, hasMore: false } });
  api.fetchAudit.mockResolvedValue({ events: [], page: { limit: 12, offset: 0, total: 0, returned: 0, hasMore: false } });
});

describe("useADKSettingsSectionState MCP boundaries", () => {
  it("rejects invalid MCP ports before calling the server", async () => {
    const state = mountState();
    await flushPromises();
    state.mcpServerForm.value.port = 1000;
    await state.saveMCPServerSettings();
    expect(state.errorMessage.value).toBe("MCP 服务端口必须在 1024 到 65535 之间");
    expect(api.saveMCP).not.toHaveBeenCalled();
  });

  it("saves MCP settings, preserves unsaved choices through token rotation, and exposes failures", async () => {
    const state = mountState();
    await flushPromises();
    state.mcpServerForm.value = { enabled: true, port: 7788, authMode: "token" };
    api.saveMCP.mockResolvedValue(mcpSnapshot(7788));
    await state.saveMCPServerSettings();
    expect(api.saveMCP).toHaveBeenCalledWith({ enabled: true, port: 7788, authMode: "token" });
    expect(state.successMessage.value).toBe("MCP 服务设置已保存");
    expect(state.mcpServerSaving.value).toBe(false);

    state.mcpServerForm.value = { enabled: true, port: 8999, authMode: "none" };
    api.resetMCPToken.mockResolvedValue({ ...mcpSnapshot(6697), token: "one-time-token" });
    await state.resetMCPServerToken();
    expect(state.mcpServerForm.value).toEqual({ enabled: true, port: 8999, authMode: "none" });
    expect(state.mcpServerOneTimeToken.value).toBe("one-time-token");
    state.clearMCPServerOneTimeToken();
    expect(state.mcpServerOneTimeToken.value).toBe("");

    api.saveMCP.mockRejectedValueOnce(new Error("端口被占用"));
    await state.saveMCPServerSettings();
    expect(state.errorMessage.value).toBe("端口被占用");
    api.resetMCPToken.mockRejectedValueOnce(new Error("token 服务不可用"));
    await state.resetMCPServerToken();
    expect(state.errorMessage.value).toBe("token 服务不可用");
    expect(state.mcpServerSaving.value).toBe(false);
  });

  it("filters runs by an explicit status after the initial attention view", async () => {
    api.fetchRuns.mockImplementation(async (_page: unknown, status: string) => ({
      runs: status === "RUNNING"
        ? [{ id: "running", status: "RUNNING" }]
        : [{ id: "failed", status: "FAILED" }],
      page: { limit: 20, offset: 0, total: 1, returned: 1, hasMore: false },
    }));
    const state = mountState();
    await flushPromises();
    state.runStatusFilter.value = "RUNNING";
    await nextTick();
    await flushPromises();
    expect(state.filteredRuns.value.map((run) => run.id)).toEqual(["running"]);
  });
});
