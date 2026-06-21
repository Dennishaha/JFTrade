import type {
  ADKAgent,
  ADKApproval,
  ADKProvider,
  ADKSession,
  ADKSessionComposerState,
  ADKToolDescriptor,
} from "@/contracts";

import { fetchEnvelope, fetchEnvelopeWithInit } from "./apiClient";

interface AgentsResponse { agents: ADKAgent[] }
interface ProvidersResponse { providers: ADKProvider[] }
interface SessionsResponse { sessions: ADKSession[] }
interface ApprovalsResponse { approvals: ADKApproval[] }
interface ToolsResponse { tools: ADKToolDescriptor[] }

export async function fetchADKPageSessionData(): Promise<{
  agents: ADKAgent[];
  approvals: ADKApproval[];
  providers: ADKProvider[];
  sessions: ADKSession[];
  tools: ADKToolDescriptor[];
}> {
  const [agents, providers, sessions, approvals, tools] = await Promise.all([
    fetchEnvelope<AgentsResponse>("/api/v1/adk/agents"),
    fetchEnvelope<ProvidersResponse>("/api/v1/adk/providers"),
    fetchEnvelope<SessionsResponse>("/api/v1/adk/sessions"),
    fetchEnvelope<ApprovalsResponse>("/api/v1/adk/approvals"),
    fetchEnvelope<ToolsResponse>("/api/v1/adk/tools"),
  ]);

  return {
    agents: agents.agents,
    approvals: approvals.approvals,
    providers: providers.providers,
    sessions: sessions.sessions,
    tools: tools.tools ?? [],
  };
}

export async function createADKPageSession(agentId: string): Promise<ADKSession> {
  return fetchEnvelopeWithInit<ADKSession>("/api/v1/adk/sessions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ agentId, title: "新会话" }),
  });
}

export async function deleteADKPageSession(sessionId: string): Promise<void> {
  await fetchEnvelopeWithInit(`/api/v1/adk/sessions/${encodeURIComponent(sessionId)}`, {
    method: "DELETE",
  });
}

export async function renameADKPageSession(sessionId: string, title: string): Promise<ADKSession> {
  return fetchEnvelopeWithInit<ADKSession>(
    `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title }),
    },
  );
}

export async function saveADKSessionComposerState(
  sessionId: string,
  state: Partial<
    Pick<
      ADKSessionComposerState,
      | "chatDraft"
      | "workModeOverride"
      | "permissionModeOverride"
      | "goalObjectiveDraft"
      | "goalObjectiveTouched"
    >
  >,
  options: { keepalive?: boolean } = {},
): Promise<ADKSessionComposerState> {
  return fetchEnvelopeWithInit<ADKSessionComposerState>(
    `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}/composer-state`,
    {
      method: "PATCH",
      keepalive: options.keepalive === true,
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(state),
    },
  );
}

export async function updateADKPageAgentProvider(agent: ADKAgent, providerId: string): Promise<ADKAgent> {
  return fetchEnvelopeWithInit<ADKAgent>("/api/v1/adk/agents", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
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
    }),
  });
}
