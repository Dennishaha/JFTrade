import type {
  ADKAgent,
  ADKApproval,
  ADKProvider,
  ADKSession,
  ADKSessionComposerState,
  ADKToolDescriptor,
} from "@/types";

import {
  apiDeletePath,
  apiGet,
  apiPatchPath,
  apiPost,
  apiPutPath,
} from "@/composables/shared/apiClient";
import {
  requireADKAgent,
  requireADKAgents,
  requireADKApprovals,
  requireADKComposerState,
  requireADKProviders,
  requireADKSession,
  requireADKSessions,
  requireADKToolDescriptors,
} from "@/composables/adk/adkApiMappers";

export async function fetchADKPageSessionData(): Promise<{
  agents: ADKAgent[];
  approvals: ADKApproval[];
  providers: ADKProvider[];
  sessions: ADKSession[];
  tools: ADKToolDescriptor[];
}> {
  const [agents, providers, sessions, approvals, tools] = await Promise.all([
    apiGet("/api/v1/adk/agents"),
    apiGet("/api/v1/adk/providers"),
    apiGet("/api/v1/adk/sessions"),
    apiGet("/api/v1/adk/approvals"),
    apiGet("/api/v1/adk/tools"),
  ]);

  return {
    agents: requireADKAgents(agents.agents),
    approvals: requireADKApprovals(approvals.approvals),
    providers: requireADKProviders(providers.providers),
    sessions: requireADKSessions(sessions.sessions),
    tools:
      tools.tools === undefined ? [] : requireADKToolDescriptors(tools.tools),
  };
}

export async function createADKPageSession(agentId: string): Promise<ADKSession> {
  return requireADKSession(
    await apiPost("/api/v1/adk/sessions", { agentId, title: "新会话" }),
  );
}

export async function deleteADKPageSession(sessionId: string): Promise<void> {
  await apiDeletePath(
    "/api/v1/adk/sessions/{sessionId}",
    `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}`,
  );
}

export async function renameADKPageSession(sessionId: string, title: string): Promise<ADKSession> {
  return requireADKSession(
    await apiPutPath(
      "/api/v1/adk/sessions/{sessionId}",
      `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}`,
      { title },
    ),
  );
}

export async function saveADKSessionComposerState(
  sessionId: string,
  state: Partial<
    Pick<
      ADKSessionComposerState,
      | "chatDraft"
      | "providerIdOverride"
      | "modelOverride"
      | "reasoningEffortOverride"
      | "workModeOverride"
      | "permissionModeOverride"
      | "goalObjectiveDraft"
      | "goalObjectiveTouched"
    >
  >,
  options: { keepalive?: boolean } = {},
): Promise<ADKSessionComposerState> {
  return requireADKComposerState(
    await apiPatchPath(
      "/api/v1/adk/sessions/{sessionId}/composer-state",
      `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}/composer-state`,
      state,
      { keepalive: options.keepalive === true },
    ),
  );
}

export async function updateADKPageAgentProvider(agent: ADKAgent, providerId: string): Promise<ADKAgent> {
  const payload = {
    id: agent.id,
    name: agent.name,
    instruction: agent.instruction,
    providerId,
    model: agent.model,
    tools: agent.tools,
    skills: agent.skills,
    permissionMode: agent.permissionMode,
    memoryEnabled: agent.memoryEnabled,
    status: agent.status,
    ...(agent.reasoningEffort === undefined
      ? {}
      : { reasoningEffort: agent.reasoningEffort }),
  };
  return requireADKAgent(
    await apiPost("/api/v1/adk/agents", payload),
  );
}
