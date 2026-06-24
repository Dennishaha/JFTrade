// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import type { ADKAgent, ADKToolDescriptor } from "../src/contracts";
import { useADKAgentForm } from "../src/composables/useADKAgentForm";

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

    state.editAgent(buildAgent("task"));
    expect(state.agentForm.value.workMode).toBe("task");
  });

  it("restores supported work modes when editing and duplicating", () => {
    const state = createState();

    for (const mode of ["chat", "task", "loop"] as const) {
      state.editAgent(buildAgent(mode));
      expect(state.agentForm.value.workMode).toBe(mode);

      state.duplicateAgent(buildAgent(mode));
      expect(state.agentForm.value.workMode).toBe(mode);
    }
  });
});

function createState(toolNames: string[] = []) {
  return useADKAgentForm(
    ref([]),
    ref(toolNames.map(buildTool)),
    ref([]),
    vi.fn(async () => {}),
    ref(""),
    ref(""),
  );
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
