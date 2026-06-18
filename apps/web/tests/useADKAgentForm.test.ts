// @vitest-environment jsdom

import { describe, expect, it, vi } from "vitest";
import { ref } from "vue";

import type { ADKAgent } from "../src/contracts";
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

function createState() {
  return useADKAgentForm(
    ref([]),
    ref([]),
    ref([]),
    vi.fn(async () => {}),
    ref(""),
    ref(""),
  );
}
