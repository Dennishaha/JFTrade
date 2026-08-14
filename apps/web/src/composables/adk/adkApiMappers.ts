import type {
  ADKAgent,
  ADKApproval,
  ADKApprovalResolution,
  ADKAuditEvent,
  ADKChatResponse,
  ADKInputAnswer,
  ADKInputOption,
  ADKInputQuestion,
  ADKInputRequest,
  ADKInputResolution,
  ADKMemoryEntry,
  ADKMessage,
  ADKOptimizationRun,
  ADKOptimizationTask,
  ADKPermissionMode,
  ADKProvider,
  ADKProviderTestResponse,
  ADKRun,
  ADKSession,
  ADKSessionComposerState,
  ADKSessionContextSnapshot,
  ADKSkill,
  ADKTask,
  ADKTimelineEntry,
  ADKToolCall,
  ADKToolDescriptor,
  ADKWorkflowCanvasEdge,
  ADKWorkflowCanvasGraph,
  ADKWorkflowCanvasNode,
  ADKWorkflowDefinition,
  ADKWorkflowInvocationResult,
  ADKWorkflowNodeRun,
  ADKWorkflowResult,
  ADKWorkflowStepState,
  ADKWorkflowTrigger,
  ADKWorkflowTriggerLog,
  ADKWorkflowTriggerSaveResult,
  ADKWorkMode,
  MCPServerSettingsSnapshot,
  MCPServerTokenResetResult,
} from "@/types";
import type {
  ADKAgentWriteRequestDto,
  ADKRuntimeSettings,
} from "@/contracts";

import * as guards from "./adkApiGuards";
import type { ADKMetricsView, ADKPageEnvelope } from "./adkApiMapperModels";
import { normalizeADKMetricsWire } from "./adkApiWireNormalization";

export type { ADKMetricsView, ADKPageEnvelope } from "./adkApiMapperModels";

export const requireADKProvider = (value: unknown): ADKProvider =>
  guards.requireValue(value, guards.isADKProvider, "provider");
export const requireADKProviders = (value: unknown): ADKProvider[] =>
  guards.requireList(value, guards.isADKProvider, "providers");
export const requireADKProviderTestResponse = (
  value: unknown,
): ADKProviderTestResponse =>
  guards.requireValue(value, guards.isADKProviderTestResponse, "provider test");
export const requireADKAgent = (value: unknown): ADKAgent =>
  guards.requireValue(guards.normalizeADKAgentWire(value), guards.isADKAgent, "agent");
export const requireADKAgents = (value: unknown): ADKAgent[] =>
  guards.requireList(
    Array.isArray(value) ? value.map(guards.normalizeADKAgentWire) : value,
    guards.isADKAgent,
    "agents",
  );
export const requireADKAgentTemplates = (
  value: unknown,
): Array<Omit<ADKAgent, "createdAt" | "updatedAt">> =>
  guards.requireList(
    Array.isArray(value)
      ? value.map(guards.normalizeADKAgentTemplateWire)
      : value,
    guards.isADKAgentTemplate,
    "agent templates",
  );
export const requireADKToolDescriptors = (
  value: unknown,
): ADKToolDescriptor[] =>
  guards.requireList(
    Array.isArray(value) ? value.map(guards.normalizeADKToolDescriptor) : value,
    guards.isADKToolDescriptor,
    "tools",
  );
export const requireADKSkill = (value: unknown): ADKSkill =>
  guards.requireValue(value, guards.isADKSkill, "skill");
export const requireADKSkills = (value: unknown): ADKSkill[] =>
  guards.requireList(value, guards.isADKSkill, "skills");
export const requireADKRun = (value: unknown): ADKRun =>
  guards.requireValue(guards.normalizeRunWire(value), guards.isADKRun, "run");
export const requireADKRuns = (value: unknown): ADKRun[] => {
  if (!Array.isArray(value)) {
    throw new TypeError("ADK API response is invalid: runs");
  }
  return value.map(requireADKRun);
};
export const requireADKApproval = (value: unknown): ADKApproval =>
  guards.requireValue(value, guards.isADKApproval, "approval");
export const requireADKApprovals = (value: unknown): ADKApproval[] =>
  guards.requireList(value, guards.isADKApproval, "approvals");
export const requireADKApprovalResolution = (
  value: unknown,
): ADKApprovalResolution =>
  guards.requireValue(value, guards.isADKApprovalResolution, "approval resolution");
export const requireADKInputResolution = (
  value: unknown,
): ADKInputResolution =>
  guards.requireValue(value, guards.isADKInputResolution, "input resolution");
export const requireADKSession = (value: unknown): ADKSession =>
  guards.requireValue(value, guards.isADKSession, "session");
export const requireADKSessions = (value: unknown): ADKSession[] =>
  guards.requireList(value, guards.isADKSession, "sessions");
export const requireADKComposerState = (
  value: unknown,
): ADKSessionComposerState =>
  guards.requireValue(value, guards.isADKSessionComposerState, "session composer state");
export const requireADKContextSnapshot = (
  value: unknown,
): ADKSessionContextSnapshot =>
  guards.requireValue(value, guards.isADKSessionContextSnapshot, "session context");
export const requireADKTimeline = (value: unknown): ADKTimelineEntry[] => {
  if (!Array.isArray(value)) {
    throw new TypeError("ADK API response is invalid: timeline");
  }
  return value.map((entry) =>
    guards.requireValue(guards.normalizeTimelineWire(entry), guards.isADKTimelineEntry, "timeline"),
  );
};
export const requireADKTask = (value: unknown): ADKTask =>
  guards.requireValue(value, guards.isADKTask, "task");
export const requireADKTasks = (value: unknown): ADKTask[] =>
  guards.requireList(value, guards.isADKTask, "tasks");
export const requireADKMemoryEntry = (value: unknown): ADKMemoryEntry =>
  guards.requireValue(value, guards.isADKMemoryEntry, "memory entry");
export const requireADKMemoryEntries = (value: unknown): ADKMemoryEntry[] =>
  guards.requireList(value, guards.isADKMemoryEntry, "memory entries");
export const requireADKOptimizationTask = (
  value: unknown,
): ADKOptimizationTask =>
  guards.requireValue(value, guards.isADKOptimizationTask, "optimization task");
export const requireADKOptimizationTasks = (
  value: unknown,
): ADKOptimizationTask[] =>
  guards.requireList(value, guards.isADKOptimizationTask, "optimization tasks");
export const requireADKAuditEvents = (value: unknown): ADKAuditEvent[] =>
  guards.requireList(value, guards.isADKAuditEvent, "audit events");
export const requireADKPage = (value: unknown): ADKPageEnvelope =>
  guards.requireValue(value, guards.isPageEnvelope, "page");
export const requireADKRuntimeSettings = (
  value: unknown,
): ADKRuntimeSettings =>
  guards.requireValue(value, guards.isRuntimeSettings, "runtime settings");
export const requireMCPSettingsSnapshot = (
  value: unknown,
): MCPServerSettingsSnapshot =>
  guards.requireValue(value, guards.isMCPSettingsSnapshot, "MCP settings");
export const requireMCPTokenResetResult = (
  value: unknown,
): MCPServerTokenResetResult =>
  guards.requireValue(value, guards.isMCPTokenResetResult, "MCP token reset");
export const requireADKMetrics = (value: unknown): ADKMetricsView =>
  guards.requireValue(normalizeADKMetricsWire(value), guards.isMetricsView, "metrics");
export const requireADKWorkflowDefinition = (
  value: unknown,
): ADKWorkflowDefinition =>
  guards.requireValue(value, guards.isADKWorkflowDefinition, "workflow");
export const requireADKWorkflowDefinitions = (
  value: unknown,
): ADKWorkflowDefinition[] =>
  guards.requireList(value, guards.isADKWorkflowDefinition, "workflows");
export const requireADKWorkflowTrigger = (
  value: unknown,
): ADKWorkflowTrigger =>
  guards.requireValue(value, guards.isADKWorkflowTrigger, "workflow trigger");
export const requireADKWorkflowTriggers = (
  value: unknown,
): ADKWorkflowTrigger[] =>
  guards.requireList(value, guards.isADKWorkflowTrigger, "workflow triggers");
export const requireADKWorkflowTriggerLogs = (
  value: unknown,
): ADKWorkflowTriggerLog[] =>
  guards.requireList(value, guards.isADKWorkflowTriggerLog, "workflow trigger logs");
export const requireADKWorkflowInvocation = (
  value: unknown,
): ADKWorkflowInvocationResult =>
  guards.requireValue(value, guards.isADKWorkflowInvocation, "workflow invocation");
export const requireADKWorkflowTriggerSave = (
  value: unknown,
): ADKWorkflowTriggerSaveResult =>
  guards.requireValue(value, guards.isADKWorkflowTriggerSaveResult, "workflow trigger save");
