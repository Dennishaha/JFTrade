import { afterEach, describe, expect, it, vi } from "vitest";

import type { ADKAgent } from "@/types";
import {
  renameADKPageSession,
  updateADKPageAgentProvider,
} from "@/composables/adk/adkPageSessionApi";
import { compactADKSessionContext } from "@/composables/adk/adkSessionContextApi";
import { createResponse } from "../helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ADK session and agent API contracts", () => {
  it("encodes session identifiers when renaming a conversation", async () => {
    const fetchMock = vi.fn(async () =>
      createResponse({
        id: "session/a b",
        agentId: "agent-1",
        title: "复盘会话",
        createdAt: "2026-07-16T00:00:00Z",
        updatedAt: "2026-07-16T00:00:00Z",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      renameADKPageSession("session/a b", "复盘会话"),
    ).resolves.toMatchObject({ title: "复盘会话" });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/adk/sessions/session%2Fa%20b",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ title: "复盘会话" }),
      }),
    );
  });

  it("updates only the selected provider while retaining the agent's executable contract", async () => {
    const agent: ADKAgent = {
      id: "researcher",
      name: "策略研究员",
      instruction: "检查交易假设",
      providerId: "old-provider",
      model: "model-a",
      reasoningEffort: "xhigh",
      tools: ["market.snapshot"],
      skills: ["research"],
      permissionMode: "approval",
      memoryEnabled: true,
      recentUserWindow: 8,
      workMode: "chat",
      loopMaxIterations: 4,
      status: "ENABLED",
      createdAt: "2026-07-16T00:00:00Z",
      updatedAt: "2026-07-16T00:00:00Z",
    };
    const fetchMock = vi.fn(async () => createResponse({
      ...agent,
      providerId: "new-provider",
    }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      updateADKPageAgentProvider(agent, "new-provider"),
    ).resolves.toMatchObject({ providerId: "new-provider" });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/adk/agents",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          id: "researcher",
          name: "策略研究员",
          instruction: "检查交易假设",
          providerId: "new-provider",
          model: "model-a",
          tools: ["market.snapshot"],
          skills: ["research"],
          permissionMode: "approval",
          memoryEnabled: true,
          status: "ENABLED",
          reasoningEffort: "xhigh",
        }),
      }),
    );
  });

  it("omits an unset Agent reasoning default when changing Providers", async () => {
    const agent: ADKAgent = {
      id: "researcher",
      name: "策略研究员",
      instruction: "检查交易假设",
      providerId: "old-provider",
      model: "model-a",
      tools: [],
      skills: [],
      permissionMode: "approval",
      memoryEnabled: true,
      recentUserWindow: 6,
      workMode: "chat",
      loopMaxIterations: 5,
      status: "ENABLED",
      createdAt: "2026-07-16T00:00:00Z",
      updatedAt: "2026-07-16T00:00:00Z",
    };
    const fetchMock = vi.fn(async () => createResponse({
      ...agent,
      providerId: "new-provider",
    }));
    vi.stubGlobal("fetch", fetchMock);

    await updateADKPageAgentProvider(agent, "new-provider");

    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).not.toHaveProperty(
      "reasoningEffort",
    );
  });

  it("sends the requested compaction mode to the session-context boundary", async () => {
    const fetchMock = vi.fn(async () => createResponse(buildContext()));
    vi.stubGlobal("fetch", fetchMock);

    await compactADKSessionContext("session/a b", "aggressive");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/adk/sessions/session%2Fa%20b/context/compact",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ mode: "aggressive" }),
      }),
    );
  });
});

function buildContext() {
  return {
    sessionId: "session/a b",
    currentInputTokens: 100,
    projectedNextTurnTokens: 120,
    contextWindowTokens: 4_096,
    usageRatio: 0.03,
    status: "healthy",
    recentUserWindow: 8,
    retainedRecentUserCount: 2,
    activeHandoffCount: 0,
    breakdown: {
      instructionTokens: 20,
      handoffTokens: 0,
      recentUserTokens: 50,
      protectedTailTokens: 0,
      otherVisibleTokens: 20,
      pendingUserTokens: 10,
      toolDeclarationTokens: 0,
    },
    autoCompacted: false,
    degradedSummary: false,
  };
}
