import { afterEach, describe, expect, it, vi } from "vitest";

import type { ADKAgent } from "@/contracts";
import {
  renameADKPageSession,
  updateADKPageAgentProvider,
} from "../src/composables/adkPageSessionApi";
import { compactADKSessionContext } from "../src/composables/adkSessionContextApi";
import { createResponse } from "./helpers";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ADK session and agent API contracts", () => {
  it("encodes session identifiers when renaming a conversation", async () => {
    const fetchMock = vi.fn(async () => createResponse({
      id: "session/a b",
      title: "复盘会话",
    }));
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
        }),
      }),
    );
  });

  it("sends the requested compaction mode to the session-context boundary", async () => {
    const fetchMock = vi.fn(async () => createResponse({}));
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
