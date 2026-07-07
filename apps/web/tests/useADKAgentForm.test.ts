// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import type { ADKAgent, ADKSkill, ADKToolDescriptor } from "../src/contracts";
import { deleteADKAgent, saveADKAgent } from "../src/composables/adkSettingsApi";
import { useADKAgentForm } from "../src/composables/useADKAgentForm";

vi.mock("../src/composables/adkSettingsApi", () => ({
  deleteADKAgent: vi.fn(),
  saveADKAgent: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

function buildAgent(workMode: ADKAgent["workMode"] | "sequential" | "parallel"): ADKAgent {
  return {
    id: `agent-${workMode}`,
    name: "Agent",
    instruction: "Instruction",
    providerId: "provider",
    model: "model",
    tools: [],
    skills: [],
    permissionMode: "approval",
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: workMode as ADKAgent["workMode"],
    loopMaxIterations: 5,
    status: "ENABLED",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

describe("useADKAgentForm", () => {
  it("defaults new agents to chat mode", () => {
    const state = createState();

    state.newAgentForm();

    expect(state.agentForm.value.workMode).toBe("chat");
    expect(state.agentForm.value.loopMaxIterations).toBe(5);
  });

  it("keeps new agents on the dynamic default provider", () => {
    const state = createState();

    state.newAgentForm();

    expect(state.agentForm.value.providerId).toBe("");
  });

  it("keeps new agent tools empty so the backend treats it as all tools", () => {
    const state = createState(["tool.1", "tool.2", "tool.3", "tool.4", "tool.5", "tool.6", "tool.7", "tool.8", "tool.9"]);

    state.newAgentForm();

    expect(state.agentForm.value.tools).toEqual([]);
  });

  it("coerces hidden sequential and parallel defaults to chat when editing or duplicating", () => {
    const state = createState();

    state.editAgent(buildAgent("sequential"));
    expect(state.agentForm.value.workMode).toBe("chat");

    state.editAgent(buildAgent("parallel"));
    expect(state.agentForm.value.workMode).toBe("chat");

    state.duplicateAgent(buildAgent("sequential"));
    expect(state.agentForm.value.workMode).toBe("chat");

    state.editAgent(buildAgent("loop"));
    expect(state.agentForm.value.workMode).toBe("loop");

    state.editAgent(buildAgent("loop"));
    expect(state.agentForm.value.workMode).toBe("loop");
  });

  it("restores supported work modes when editing and duplicating", () => {
    const state = createState();

    for (const mode of ["chat", "loop"] as const) {
      state.editAgent(buildAgent(mode));
      expect(state.agentForm.value.workMode).toBe(mode);

      state.duplicateAgent(buildAgent(mode));
      expect(state.agentForm.value.workMode).toBe(mode);
    }
  });

  it("selects installed skills for a new agent while leaving tools backend-managed", () => {
    const state = createState(["market.quote"], [buildSkill("risk"), buildSkill("research")]);

    state.newAgentForm();

    expect(state.agentForm.value.skills).toEqual(["risk", "research"]);
    expect(state.agentForm.value.tools).toEqual([]);
  });

  it("requires confirmation before disabling an existing agent", async () => {
    const state = createState();
    state.editAgent({ ...buildAgent("chat"), status: "DISABLED" });
    vi.spyOn(window, "confirm").mockReturnValue(false);

    await state.saveAgent();

    expect(saveADKAgent).not.toHaveBeenCalled();
    expect(state.refreshAll).not.toHaveBeenCalled();
  });

  it("saves a confirmed disabled agent and refreshes the persisted list", async () => {
    const state = createState();
    state.editAgent({ ...buildAgent("loop"), status: "DISABLED" });
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.mocked(saveADKAgent).mockResolvedValue(buildAgent("loop"));

    await state.saveAgent();

    expect(saveADKAgent).toHaveBeenCalledWith(
      expect.objectContaining({ status: "DISABLED", workMode: "loop" }),
    );
    expect(state.successMessage.value).toBe("Agent 已保存");
    expect(state.refreshAll).toHaveBeenCalledOnce();
  });

  it("does not prompt when saving a new or enabled agent", async () => {
    const state = createState();
    const confirm = vi.spyOn(window, "confirm");
    vi.mocked(saveADKAgent).mockResolvedValue(buildAgent("chat"));

    await state.saveAgent();

    expect(confirm).not.toHaveBeenCalled();
    expect(saveADKAgent).toHaveBeenCalledOnce();
  });

  it("surfaces save and delete failures without hiding non-Error responses", async () => {
    const state = createState();
    vi.mocked(saveADKAgent).mockRejectedValueOnce(new Error("agent name already exists"));

    await state.saveAgent();
    expect(state.errorMessage.value).toBe("agent name already exists");

    vi.mocked(saveADKAgent).mockRejectedValueOnce("offline");
    await state.saveAgent();
    expect(state.errorMessage.value).toBe("保存失败");

    vi.mocked(deleteADKAgent).mockRejectedValueOnce("locked");
    await state.deleteAgent("agent-1");
    expect(state.errorMessage.value).toBe("删除失败");
  });

  it("deletes agents and treats refresh errors as incomplete operations", async () => {
    const state = createState();
    await state.deleteAgent("agent-1");
    expect(deleteADKAgent).toHaveBeenCalledWith("agent-1");
    expect(state.refreshAll).toHaveBeenCalledOnce();

    state.refreshAll.mockRejectedValueOnce(new Error("refresh failed"));
    await state.deleteAgent("agent-2");
    expect(state.errorMessage.value).toBe("refresh failed");
  });
});

function createState(toolNames: string[] = [], skills: ADKSkill[] = []) {
  const refreshAll = vi.fn(async () => {});
  const successMessage = ref("");
  const errorMessage = ref("");
  return {
    ...useADKAgentForm(
    ref([]),
    ref(toolNames.map(buildTool)),
    ref(skills),
    refreshAll,
    successMessage,
    errorMessage,
    ),
    refreshAll,
    successMessage,
    errorMessage,
  };
}

function buildTool(name: string): ADKToolDescriptor {
  return {
    name,
    displayName: name,
    description: "",
    category: "test",
    permission: "read",
    allowedModes: ["approval", "less_approval", "all"],
    requiresApprovalIn: [],
  };
}

function buildSkill(id: string): ADKSkill {
  return {
    id,
    displayName: id,
    description: "",
    source: "builtin",
    installPath: "",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}
