import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, vi } from "vitest";
import { nextTick } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

import type {
  ADKApproval,
  ADKRun,
  ADKSessionComposerState,
  ADKSessionContextSnapshot,
  ADKTimelineEntry,
} from "@/types";

import { resetADKApprovalInFlightForTest } from "@/composables/adk/adkApprovalResolution";
import ADKPage from "../../src/pages/ADKPage.vue";
import { createResponse, flushRequests } from "../helpers";

export function registerADKPageTestLifecycle(
  mocks: Record<string, ReturnType<typeof vi.fn>>,
): void {
  beforeEach(() => {
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    resetADKApprovalInFlightForTest();
    mocks.streamADKChatMock?.mockReset();
    mocks.resumeADKChatStreamMock?.mockReset();
    mocks.resumeADKChatStreamMock?.mockResolvedValue(null);
    mocks.monitorADKRunContinuationMock?.mockReset();
    mocks.monitorADKRunContinuationMock?.mockImplementation(async (run) => run);
  });
}


type SessionContextTestResponse =
  | ADKSessionContextSnapshot
  | null
  | Error
  | Promise<ADKSessionContextSnapshot | null>;

export function mountADKPage(
  options: {
    providerHasKey?: boolean;
    providers?: Array<ReturnType<typeof buildProvider>>;
    agent?: Partial<ReturnType<typeof buildAgentBase>>;
    approvals?: ADKApproval[];
    approvalResolution?: unknown;
    approvalResolutionById?: Record<string, unknown>;
    cancelRunById?: Record<string, ADKRun>;
    pauseRunById?: Record<string, ADKRun>;
    resumeRunById?: Record<string, ADKRun>;
    runById?: Record<string, ADKRun>;
    sessions?: Array<ReturnType<typeof buildSession>>;
    createSession?: ReturnType<typeof buildSession>;
    composerStateBySession?: Record<string, ADKSessionComposerState>;
    composerStateSave?: (
      sessionId: string,
      patch: Partial<ADKSessionComposerState>,
    ) => Promise<ADKSessionComposerState>;
    sessionDetail?: {
      session: ReturnType<typeof buildSession>;
      timeline: ADKTimelineEntry[];
      runs?: ADKRun[];
      composerState?: ADKSessionComposerState;
    };
    sessionDetailSequence?: Array<{
      session: ReturnType<typeof buildSession>;
      timeline: ADKTimelineEntry[];
      runs?: ADKRun[];
      composerState?: ADKSessionComposerState;
    }>;
    sessionContext?: ADKSessionContextSnapshot | null;
    sessionContextSequence?: SessionContextTestResponse[];
  } = {},
) {
  document.body.innerHTML = "<div id='root'></div>";
  const state = {
    approvals: [...(options.approvals ?? [])],
    sessionDetailSequence: [
      ...(options.sessionDetailSequence ?? [
        options.sessionDetail ?? { session: buildSession(), timeline: [] },
      ]),
    ],
    sessionContextSequence: [
      ...(options.sessionContextSequence ?? [options.sessionContext ?? null]),
    ],
    composerStateBySession: { ...(options.composerStateBySession ?? {}) },
  };

  const fetchMock = vi.fn(
    async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/api/v1/adk/agents")) {
        return createResponse({ agents: [buildAgent(options.agent)] });
      }
      if (url.includes("/api/v1/adk/providers")) {
        return createResponse({
          providers: options.providers ?? [buildProvider(options.providerHasKey ?? true)],
        });
      }
      if (/\/api\/v1\/adk\/sessions\/[^/]+\/context$/.test(url)) {
        const context =
          state.sessionContextSequence.length > 1
            ? state.sessionContextSequence.shift()!
            : state.sessionContextSequence[0]!;
        const resolvedContext = await context;
        if (resolvedContext instanceof Error) {
          throw resolvedContext;
        }
        return createResponse(resolvedContext ?? null);
      }
      const composerStateMatch = url.match(
        /\/api\/v1\/adk\/sessions\/([^/]+)\/composer-state$/,
      );
      if (composerStateMatch) {
        const sessionId = decodeURIComponent(composerStateMatch[1]!);
        const patch = JSON.parse(
          String(init?.body ?? "{}"),
        ) as Partial<ADKSessionComposerState>;
        if (options.composerStateSave) {
          return createResponse(
            await options.composerStateSave(sessionId, patch),
          );
        }
        state.composerStateBySession[sessionId] = buildComposerState(
          sessionId,
          {
            ...(state.composerStateBySession[sessionId] ?? {}),
            ...patch,
            updatedAt: "2026-06-06T00:00:10Z",
          },
        );
        return createResponse(state.composerStateBySession[sessionId]);
      }
      if (url.includes("/api/v1/adk/sessions")) {
        if (init?.method === "DELETE") {
          return createResponse({});
        }
        if (/\/api\/v1\/adk\/sessions\/[^/]+$/.test(url)) {
          const detail =
            state.sessionDetailSequence.length > 1
              ? state.sessionDetailSequence.shift()!
              : state.sessionDetailSequence[0]!;
          return createResponse({
            ...detail,
            composerState:
              detail.composerState ??
              state.composerStateBySession[detail.session.id] ??
              buildComposerState(detail.session.id),
          });
        }
        if (init?.method === "POST" && options.createSession) {
          return createResponse(options.createSession);
        }
        return createResponse({
          sessions: options.sessions ?? [buildSession()],
        });
      }
      const cancelRunMatch = url.match(
        /\/api\/v1\/adk\/runs\/([^/]+)\/cancel$/,
      );
      if (cancelRunMatch) {
        const runId = decodeURIComponent(cancelRunMatch[1]!);
        return createResponse(
          options.cancelRunById?.[runId] ??
            buildRun({ id: runId, status: "CANCELLED", pendingApprovals: [] }),
        );
      }
      const pauseRunMatch = url.match(/\/api\/v1\/adk\/runs\/([^/]+)\/pause$/);
      if (pauseRunMatch) {
        const runId = decodeURIComponent(pauseRunMatch[1]!);
        return createResponse(
          options.pauseRunById?.[runId] ??
            buildRun({
              id: runId,
              status: "RUNNING",
              pauseRequestedAt: "2026-06-06T00:00:10Z",
            }),
        );
      }
      const resumeRunMatch = url.match(
        /\/api\/v1\/adk\/runs\/([^/]+)\/resume$/,
      );
      if (resumeRunMatch) {
        const runId = decodeURIComponent(resumeRunMatch[1]!);
        return createResponse(
          options.resumeRunById?.[runId] ??
            buildRun({
              id: runId,
              status: "RUNNING",
              resumeState: "user_resuming",
            }),
        );
      }
      const runDetailMatch = url.match(/\/api\/v1\/adk\/runs\/([^/]+)$/);
      if (runDetailMatch) {
        const runId = decodeURIComponent(runDetailMatch[1]!);
        return createResponse(options.runById?.[runId] ?? {});
      }
      const approvalActionMatch = url.match(
        /\/api\/v1\/adk\/approvals\/([^/]+)\/(approve|deny)$/,
      );
      if (approvalActionMatch) {
        const approvalId = approvalActionMatch[1]!;
        state.approvals = state.approvals.filter(
          (approval) => approval.id !== approvalId,
        );
        if (options.approvalResolutionById?.[approvalId] !== undefined) {
          return createResponse(options.approvalResolutionById[approvalId]);
        }
        return createResponse(await Promise.resolve(options.approvalResolution));
      }
      if (url.includes("/api/v1/adk/approvals")) {
        return createResponse({ approvals: state.approvals });
      }
      return createResponse({});
    },
  );
  vi.stubGlobal("fetch", fetchMock);

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/", component: { template: "<div />" } }],
  });
  mount(ADKPage, {
    attachTo: "#root",
    global: {
      plugins: [router],
      stubs: vuetifyStubs(),
    },
  });
  return fetchMock;
}

export function buildProvider(
  hasApiKey: boolean,
  overrides: Partial<ReturnType<typeof buildProviderBase>> = {},
) {
  return {
    ...buildProviderBase(hasApiKey),
    ...overrides,
  };
}

export function buildProviderBase(hasApiKey: boolean) {
  return {
    id: "provider-1",
    displayName: "OpenAI",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-4o-mini",
    requestTimeoutMs: 180_000,
    enabled: true,
    default: true,
    hasApiKey,
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

export function buildApproval(id: string, runId = "run-1"): ADKApproval {
  return {
    id,
    runId,
    agentId: "agent-1",
    toolName: "strategy.save_draft",
    input: { query: "@strategy.save_draft" },
    status: "PENDING",
    reason: "needs approval",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

export function buildAgent(
  overrides: Partial<ReturnType<typeof buildAgentBase>> = {},
) {
  return {
    ...buildAgentBase(),
    ...overrides,
  };
}

export function buildAgentBase() {
  return {
    id: "agent-1",
    name: "投资分析助手",
    instruction: "test",
    providerId: "provider-1",
    model: "gpt-4o-mini",
    reasoningEffort: "",
    tools: ["strategy.save_draft"],
    skills: [],
    permissionMode: "approval",
    memoryEnabled: true,
    recentUserWindow: 6,
    workMode: "chat",
    loopMaxIterations: 5,
    status: "ENABLED",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

export function buildSession(
  overrides: Partial<{
    id: string;
    agentId: string;
    title: string;
    createdAt: string;
    updatedAt: string;
  }> = {},
) {
  return {
    id: "session-1",
    agentId: "agent-1",
    title: "测试会话",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
    ...overrides,
  };
}

export function buildComposerState(
  sessionId: string,
  overrides: Partial<ADKSessionComposerState> = {},
): ADKSessionComposerState {
  return {
    sessionId,
    chatDraft: "",
    providerIdOverride: "",
    modelOverride: "",
    reasoningEffortOverride: "",
    workModeOverride: "",
    permissionModeOverride: "",
    goalObjectiveDraft: "",
    goalObjectiveTouched: false,
    updatedAt: "2026-06-06T00:00:00Z",
    ...overrides,
  };
}

export function buildSessionContextSnapshot(
  overrides: Partial<ADKSessionContextSnapshot> = {},
): ADKSessionContextSnapshot {
  const breakdown = {
    instructionTokens: 900,
    handoffTokens: 0,
    recentUserTokens: 1200,
    protectedTailTokens: 0,
    otherVisibleTokens: 1600,
    pendingUserTokens: 200,
    toolDeclarationTokens: 300,
  };
  return {
    sessionId: "session-1",
    currentInputTokens: 4200,
    projectedNextTurnTokens: 4300,
    rawCurrentInputTokens: 4200,
    rawProjectedNextTurnTokens: 4300,
    contextWindowTokens: 10000,
    usageRatio: 0.42,
    status: "healthy",
    recentUserWindow: 6,
    retainedRecentUserCount: 1,
    activeHandoffCount: 0,
    breakdown,
    rawBreakdown: breakdown,
    trimmedToolResponseCount: 0,
    autoCompacted: false,
    degradedSummary: false,
    ...overrides,
  };
}

export function deferred<T>(): {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason?: unknown) => void;
} {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

export function buildToolCall(
  id: string,
  runId: string,
  toolName: string,
  status: string,
) {
  return {
    id,
    runId,
    toolName,
    permission: "write_strategy",
    status,
    input: { toolName },
    requiresUser: status === "PENDING_APPROVAL",
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
  };
}

export function buildRun(overrides: Partial<ADKRun>): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    reasoningEffort: "",
    status: "COMPLETED",
    message: "completed",
    userMessage: "@strategy.save_draft 保存策略",
    toolSummaries: [],
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
    ...overrides,
  };
}

export function buildWorkflowStep(
  taskId: string,
  title: string,
  status: string,
  childRunId?: string,
): NonNullable<ADKRun["workflowPlan"]>[number] {
  return {
    taskId,
    title,
    status,
    childRunId,
  };
}

export function buildTimelineEntry(
  kind: ADKTimelineEntry["kind"],
  overrides: Partial<ADKTimelineEntry> = {},
): ADKTimelineEntry {
  return {
    id: overrides.id ?? `entry-${kind}`,
    sessionId: overrides.sessionId ?? "session-1",
    kind,
    createdAt: overrides.createdAt ?? "2026-06-06T00:00:00Z",
    updatedAt: overrides.updatedAt,
    sequence: overrides.sequence ?? 1,
    status: overrides.status ?? "final",
    runId: overrides.runId,
    text: overrides.text,
    originalText: overrides.originalText,
    processedText: overrides.processedText,
    toolCalls: overrides.toolCalls,
    approvals: overrides.approvals,
  };
}

export function pendingApprovalTimeline(
  run: ADKRun,
  approvals: ADKApproval[],
  userText: string,
): ADKTimelineEntry[] {
  return [
    buildTimelineEntry("user_message", {
      id: `user-${run.id}`,
      text: userText,
      createdAt: "2026-06-06T00:00:00Z",
    }),
    buildTimelineEntry("tool_group", {
      id: `tools-${run.id}`,
      runId: run.id,
      toolCalls: run.toolCalls,
      createdAt: "2026-06-06T00:00:01Z",
    }),
    buildTimelineEntry("approval_group", {
      id: `approvals-${run.id}`,
      runId: run.id,
      approvals,
      createdAt: "2026-06-06T00:00:02Z",
    }),
  ];
}

export async function sendPageMessage(text: string): Promise<void> {
  const textarea = document.querySelector("textarea")!;
  textarea.value = text;
  textarea.dispatchEvent(new Event("input"));
  await nextTick();
  document.querySelector<HTMLButtonElement>(".adk-composer-send")?.click();
  await flushRequests();
}

export function clickButtonByText(text: string): void {
  Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
    .find((button) => button.textContent?.includes(text))
    ?.click();
}

export async function expandQueue(title: string): Promise<void> {
  const queue =
    document.querySelector<HTMLElement>(`[aria-label="${title}"]`) ??
    Array.from(
      document.querySelectorAll<HTMLElement>(".adk-workspace-queue"),
    ).find((candidate) => candidate.textContent?.includes(title));
  if (!queue || queue.querySelector(".adk-workspace-queue__body")) return;
  queue
    .querySelector<HTMLButtonElement>(".adk-workspace-queue__header")
    ?.click();
  await nextTick();
}

export function findWorkModeSelect(): HTMLSelectElement | undefined {
  return Array.from(
    document.querySelectorAll<HTMLSelectElement>("select"),
  ).find((select) =>
    Array.from(select.options).some((option) => option.value === "loop"),
  );
}

export function findProviderSelect(providerId: string): HTMLSelectElement | undefined {
  return Array.from(
    document.querySelectorAll<HTMLSelectElement>("select"),
  ).find((select) =>
    Array.from(select.options).some((option) => option.value === providerId),
  );
}

export function lastComposerStatePatch(
  fetchMock: ReturnType<typeof vi.fn>,
  sessionId: string,
): Record<string, unknown> | undefined {
  const calls = fetchMock.mock.calls.filter(([input]) =>
    String(input).includes(
      `/api/v1/adk/sessions/${encodeURIComponent(sessionId)}/composer-state`,
    ),
  );
  const body = calls.at(-1)?.[1]?.body;
  return body == null ? undefined : JSON.parse(String(body));
}

export function countApprovalActionCalls(
  fetchMock: ReturnType<typeof vi.fn>,
  approvalId: string,
  action: "approve" | "deny",
): number {
  return fetchMock.mock.calls.filter(([input]) =>
    String(input).includes(`/api/v1/adk/approvals/${approvalId}/${action}`),
  ).length;
}

export function vuetifyStubs() {
  return {
    "v-alert": { template: "<div><slot /></div>" },
    "v-btn": {
      props: ["disabled", "loading"],
      emits: ["click"],
      template:
        "<button type='button' :disabled='disabled' :class='$attrs.class' @click=\"$emit('click')\"><slot /></button>",
    },
    "v-card": { template: "<div><slot /></div>" },
    "v-card-text": { template: "<div><slot /></div>" },
    "v-card-title": { template: "<div><slot /></div>" },
    "v-chip": { template: "<span><slot /></span>" },
    "v-expansion-panel": { template: "<div><slot /></div>" },
    "v-expansion-panel-text": { template: "<div><slot /></div>" },
    "v-expansion-panel-title": { template: "<div><slot /></div>" },
    "v-expansion-panels": { template: "<div><slot /></div>" },
    "v-icon": { template: "<span><slot /></span>" },
    "v-menu": {
      template: "<div><slot name='activator' :props='{}' /><slot /></div>",
    },
    "v-progress-circular": { template: "<span />" },
    "v-progress-linear": { template: "<span />" },
    "v-list-item": { template: "<div><slot /></div>" },
    "v-select": {
      props: ["modelValue", "items"],
      emits: ["update:modelValue"],
      template:
        "<select :value='modelValue' @change=\"$emit('update:modelValue', $event.target.value)\"><option v-for='item in items' :key='item.value' :value='item.value'>{{ item.title }}</option></select>",
    },
    "v-textarea": {
      props: ["modelValue", "disabled"],
      emits: ["update:modelValue"],
      template:
        "<textarea :value='modelValue' :disabled='disabled' :class='$attrs.class' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
    "v-text-field": {
      props: ["modelValue", "disabled"],
      emits: ["update:modelValue"],
      template:
        "<input :value='modelValue' :disabled='disabled' @input=\"$emit('update:modelValue', $event.target.value)\" />",
    },
  };
}
