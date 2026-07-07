import { describe, expect, it } from "vitest";

import type { ADKWorkflowDefinition, ADKWorkflowTrigger } from "@/contracts";
import {
  canvasGraphToWorkflowPayload,
  legacyWorkflowToCanvasGraph,
  sanitizeWorkflowCanvasGraph,
  triggerNodeId,
  validateWorkflowCanvasGraph,
  workflowToCanvasGraph,
} from "../src/features/adkWorkflowCanvasGraph";
import {
  createTriggerForm,
  createWorkflowForm,
  createWorkflowInputRow,
  cronExpressionToPreset,
  cronPresetToExpression,
  defaultWorkflowPromptTemplate,
  triggerFormToPayload,
  triggerToForm,
  workflowRunLink,
  workflowFormToPayload,
  workflowToForm,
} from "../src/features/adkWorkflowForms";

describe("adkWorkflowForms", () => {
  it("maps workflow input rows to defaultInputs while preserving complex fields", () => {
    const workflow: ADKWorkflowDefinition = {
      id: "workflow-1",
      name: "Daily Review",
      status: "DISABLED",
      agentId: "agent-1",
      workMode: "loop",
      permissionMode: "approval",
      promptTemplate: "Run {{ .symbol }}",
      defaultInputs: {
        symbol: "US.AAPL",
        limit: 3,
        enabled: true,
        nested: { keep: true },
      },
      createdAt: "2026-07-01T00:00:00Z",
      updatedAt: "2026-07-01T00:00:00Z",
    };

    const form = workflowToForm(workflow);
    form.inputRows.push(createWorkflowInputRow("threshold", 100));
    const payload = workflowFormToPayload(form);

    expect(payload.defaultInputs).toEqual({
      symbol: "US.AAPL",
      limit: 3,
      enabled: true,
      threshold: 100,
      nested: { keep: true },
    });
  });

  it("normalizes nullable legacy workflow fields into editable defaults", () => {
    const form = workflowToForm({
      ...buildWorkflow(),
      status: "",
      workMode: "",
      permissionMode: undefined,
      promptTemplate: "",
      defaultInputs: undefined,
    });

    expect(form).toMatchObject({
      status: "DISABLED",
      workMode: "loop",
      permissionMode: "approval",
      inputRows: [],
      preservedDefaultInputs: {},
    });
    expect(form.promptTemplate).toBe(defaultWorkflowPromptTemplate());
    expect(createWorkflowInputRow("nullable", null)).toMatchObject({
      type: "string",
      value: "",
    });
  });

  it("trims workflow payload fields, drops empty keys, and keeps non-finite numbers out", () => {
    const form = createWorkflowForm(" agent-1 ", defaultWorkflowPromptTemplate());
    form.id = " workflow-1 ";
    form.name = " Daily Review ";
    form.description = " Morning checklist ";
    form.providerId = " provider-1 ";
    form.model = " gpt-test ";
    form.promptTemplate = " Run ";
    form.objectiveTemplate = " Objective ";
    form.tagsText = " alpha, , beta ";
    form.preservedDefaultInputs = { nested: { keep: true } };
    form.inputRows = [
      createWorkflowInputRow(" symbol ", "US.AAPL"),
      { key: "badNumber", type: "number", value: "NaN", booleanValue: false },
      { key: "enabled", type: "boolean", value: "", booleanValue: false },
      createWorkflowInputRow("   ", "ignored"),
    ];

    const payload = workflowFormToPayload(form);

    expect(payload).toMatchObject({
      id: "workflow-1",
      name: "Daily Review",
      description: "Morning checklist",
      agentId: "agent-1",
      providerId: "provider-1",
      model: "gpt-test",
      promptTemplate: "Run",
      objectiveTemplate: "Objective",
      tags: ["alpha", "beta"],
    });
    expect(payload.defaultInputs).toEqual({
      nested: { keep: true },
      symbol: "US.AAPL",
      enabled: false,
    });

    form.id = "";
    expect(workflowFormToPayload(form).id).toBeUndefined();
  });

  it("maps schedule trigger presets to cron and back", () => {
    expect(
      cronPresetToExpression({
        frequency: "weekdays",
        time: "09:30",
        weekdays: [],
        customCron: "",
      }),
    ).toBe("30 9 * * 1-5");
    expect(cronExpressionToPreset("30 9 * * 1-5")).toMatchObject({
      frequency: "weekdays",
      time: "09:30",
    });
    expect(
      cronPresetToExpression({
        frequency: "daily",
        time: "25:99",
        weekdays: [],
        customCron: "",
      }),
    ).toBe("59 23 * * *");
    expect(
      cronPresetToExpression({
        frequency: "weekly",
        time: "bad",
        weekdays: [],
        customCron: "",
      }),
    ).toBe("0 8 * * 1");
    expect(
      cronPresetToExpression({
        frequency: "weekly",
        time: "08:15",
        weekdays: ["1", "3", "5"],
        customCron: "",
      }),
    ).toBe("15 8 * * 1,3,5");
    expect(
      cronPresetToExpression({
        frequency: "custom",
        time: "09:30",
        weekdays: [],
        customCron: " 15 10 * * 2 ",
      }),
    ).toBe("15 10 * * 2");
    expect(cronExpressionToPreset("15 10 * * *")).toMatchObject({
      frequency: "daily",
      time: "10:15",
    });
    expect(cronExpressionToPreset("15 10 * * 2,4")).toMatchObject({
      frequency: "weekly",
      weekdays: ["2", "4"],
    });
    expect(cronExpressionToPreset("15 10 1 * *")).toMatchObject({
      frequency: "custom",
      time: "10:15",
    });
    expect(cronExpressionToPreset("@daily")).toMatchObject({
      frequency: "custom",
      time: "08:00",
      customCron: "@daily",
    });

    const scheduleForm = createTriggerForm("schedule");
    scheduleForm.title = " 开盘复盘 ";
    scheduleForm.schedule.frequency = "daily";
    scheduleForm.schedule.time = "09:30";
    scheduleForm.schedule.timezone = " ";
    scheduleForm.preservedConfig = { jitterSec: 30 };
    expect(triggerFormToPayload(scheduleForm)).toMatchObject({
      title: "开盘复盘",
      config: {
        cron: "30 9 * * *",
        timezone: "Asia/Shanghai",
        jitterSec: 30,
      },
    });

    const scheduleTrigger = triggerToForm({
      ...buildTrigger(),
      config: {
        cron: "15 10 * * 2,4",
        timezone: "",
        jitterSec: 30,
      },
    });
    expect(scheduleTrigger.schedule).toMatchObject({
      frequency: "weekly",
      time: "10:15",
      weekdays: ["2", "4"],
      timezone: "Asia/Shanghai",
      customCron: "15 10 * * 2,4",
    });
    expect(scheduleTrigger.preservedConfig).toEqual({ jitterSec: 30 });

    const legacySchedule = triggerToForm({
      ...buildTrigger(),
      status: "",
      config: undefined,
    });
    expect(legacySchedule).toMatchObject({
      status: "DISABLED",
      schedule: {
        frequency: "weekdays",
        timezone: "Asia/Shanghai",
        customCron: "0 8 * * 1-5",
      },
    });
  });

  it("preserves unknown trigger config fields when saving structured fields", () => {
    const trigger: ADKWorkflowTrigger = {
      id: "trigger-1",
      workflowId: "workflow-1",
      type: "market_threshold",
      title: "Breakout",
      status: "ENABLED",
      config: {
        instrumentIds: ["US.AAPL"],
        snapshotPath: "snapshot.price",
        operator: ">",
        value: 100,
        edge: "cross_up",
        cooldownSec: 900,
        state: { lastValues: { "US.AAPL": 99 } },
      },
      createdAt: "2026-07-01T00:00:00Z",
      updatedAt: "2026-07-01T00:00:00Z",
    };

    const form = triggerToForm(trigger);
    form.market.value = "120";
    const payload = triggerFormToPayload(form);

    expect(payload.config).toMatchObject({
      instrumentIds: ["US.AAPL"],
      snapshotPath: "snapshot.price",
      operator: ">",
      value: 120,
      edge: "cross_up",
      cooldownSec: 900,
      state: { lastValues: { "US.AAPL": 99 } },
    });
  });

  it("maps trigger forms for event, webhook, and market edge cases", () => {
    const form = createTriggerForm("event");
    form.id = " event-trigger ";
    form.title = " Event trigger ";
    form.event.source = "notification";
    form.event.eventType = "system.notification";
    form.event.entityId = "account-1";
    form.event.category = "broker.connection";
    form.event.level = "warn";
    form.event.cooldownSec = "600";

    expect(triggerFormToPayload(form)).toMatchObject({
      id: "event-trigger",
      title: "Event trigger",
      config: {
        source: "notification",
        eventType: "system.notification",
        entityId: "account-1",
        category: "broker.connection",
        level: "warn",
        cooldownSec: 600,
      },
    });

    const eventTrigger: ADKWorkflowTrigger = {
      ...buildTrigger(),
      type: "event",
      config: {
        source: null,
        eventType: "system.notification",
        entityId: "account-1",
        category: "broker.connection",
        level: "warn",
        cooldownSec: 600,
        advanced: true,
      },
    };
    const eventForm = triggerToForm(eventTrigger);
    expect(eventForm.event).toEqual({
      source: "",
      eventType: "system.notification",
      entityId: "account-1",
      category: "broker.connection",
      level: "warn",
      cooldownSec: "600",
    });
    expect(eventForm.preservedConfig).toEqual({ advanced: true });

    eventForm.event.source = " ";
    eventForm.event.eventType = "";
    eventForm.event.entityId = "";
    eventForm.event.category = "";
    eventForm.event.level = "";
    eventForm.event.cooldownSec = "bad";
    eventForm.preservedConfig = {
      source: "old",
      eventType: "old",
      entityId: "old",
      category: "old",
      level: "old",
      cooldownSec: 123,
      advanced: true,
    };
    expect(triggerFormToPayload(eventForm).config).toEqual({ advanced: true });

    const webhookForm = createTriggerForm("webhook");
    webhookForm.preservedConfig = { allowAnonymous: false };
    expect(triggerFormToPayload(webhookForm)).toMatchObject({
      type: "webhook",
      resetSecret: true,
      config: { allowAnonymous: false },
    });
    expect(
      triggerToForm({
        ...buildTrigger(),
        type: "webhook",
        config: { allowAnonymous: false },
      }).preservedConfig,
    ).toEqual({ allowAnonymous: false });

    const marketForm = createTriggerForm("market_threshold");
    marketForm.market.instrumentIdsText = "US.AAPL,\nHK.00700";
    marketForm.market.snapshotPath = "";
    marketForm.market.operator = "";
    marketForm.market.value = "bad";
    marketForm.market.edge = "";
    marketForm.market.cooldownSec = "bad";
    marketForm.preservedConfig = { value: 100, cooldownSec: 900, state: { last: 99 } };
    expect(triggerFormToPayload(marketForm).config).toEqual({
      instrumentIds: ["US.AAPL", "HK.00700"],
      snapshotPath: "snapshot.price",
      operator: ">",
      edge: "cross_up",
      state: { last: 99 },
    });

    const marketTrigger = triggerToForm({
      ...buildTrigger(),
      type: "market_threshold",
      config: {
        instrumentIds: "US.AAPL",
        value: 120,
      },
    });
    expect(marketTrigger.market.instrumentIdsText).toBe("");
  });

  it("builds a canvas graph for legacy workflows and trigger nodes", () => {
    const workflow = buildWorkflow();
    const trigger = buildTrigger();

    const graph = legacyWorkflowToCanvasGraph(workflow, [trigger]);

    expect(validateWorkflowCanvasGraph(graph)).toBe(true);
    expect(graph.nodes?.map((node) => node.id)).toEqual([
      "start",
      "trigger:trigger-1",
      "agent",
      "monitor",
    ]);
    expect(graph.edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ source: "trigger:trigger-1", target: "start" }),
        expect.objectContaining({ source: "start", target: "agent" }),
      ]),
    );
  });

  it("labels all legacy trigger node types for canvas fallback graphs", () => {
    const graph = legacyWorkflowToCanvasGraph(buildWorkflow(), [
      { ...buildTrigger(), id: "schedule", title: "", type: "schedule" },
      { ...buildTrigger(), id: "webhook", title: "", type: "webhook" },
      { ...buildTrigger(), id: "event", title: "", type: "event" },
      { ...buildTrigger(), id: "market", title: "", type: "market_threshold" },
      { ...buildTrigger(), id: "manual", title: "", type: "manual" },
    ]);

    expect(
      graph.nodes
        ?.filter((node) => node.type === "trigger")
        .map((node) => node.data?.title),
    ).toEqual(["定时", "网络回调", "事件", "行情阈值", "手动"]);
  });

  it("creates draft trigger nodes when legacy workflows have no trigger resources", () => {
    const graph = legacyWorkflowToCanvasGraph(
      { ...buildWorkflow(), name: "", defaultInputs: undefined },
      [],
    );

    expect(graph.nodes?.find((node) => node.id === "trigger:draft")).toMatchObject({
      type: "trigger",
      data: { title: "手动", status: "DISABLED" },
    });
    expect(graph.nodes?.find((node) => node.id === "agent")?.data?.title).toBe("智能体");
    expect(triggerNodeId("   ")).toBe("trigger:draft");
  });

  it("preserves saved canvas graph while merging missing trigger nodes", () => {
    const workflow = {
      ...buildWorkflow(),
      canvasGraph: {
        version: "adk-workflow-canvas/v1",
        nodes: [
          { id: "start", type: "start", position: { x: 1, y: 2 } },
          { id: "agent", type: "agent", position: { x: 3, y: 4 } },
          { id: "monitor", type: "monitor", position: { x: 5, y: 6 } },
        ],
        edges: [{ id: "start->agent", source: "start", target: "agent" }],
      },
    };

    const graph = workflowToCanvasGraph(workflow, [buildTrigger()]);

    expect(graph.nodes?.some((node) => node.id === "trigger:trigger-1")).toBe(true);
    expect(graph.edges?.some((edge) => edge.source === "trigger:trigger-1")).toBe(true);
  });

  it("sanitizes saved canvas graphs before round-tripping them to the backend", () => {
    const dirtyGraph = {
      version: " ",
      nodes: [
        {
          id: " start ",
          type: " start ",
          position: { x: Number.NaN, y: 2 },
          data: { label: "开始" },
        },
        { id: "agent", type: "agent", position: { x: 10, y: Number.POSITIVE_INFINITY } },
        { id: "bad", type: "trigger", position: { x: "bad", y: 0 } },
      ],
      edges: [
        {
          id: " e1 ",
          source: " start ",
          target: "agent",
          sourceHandle: " out ",
          targetHandle: " in ",
          type: " smoothstep ",
          data: { guarded: true },
        },
        { id: "dangling", source: "missing", target: "agent" },
      ],
      viewport: null,
    } as unknown;

    const graph = sanitizeWorkflowCanvasGraph(dirtyGraph as Parameters<typeof sanitizeWorkflowCanvasGraph>[0]);

    expect(graph).toMatchObject({
      version: "adk-workflow-canvas/v1",
      nodes: [
        { id: "start", type: "start", position: { x: 0, y: 2 }, data: { label: "开始" } },
        { id: "agent", type: "agent", position: { x: 10, y: 0 } },
      ],
      edges: [
        {
          id: "e1",
          source: "start",
          target: "agent",
          sourceHandle: "out",
          targetHandle: "in",
          type: "smoothstep",
          data: { guarded: true },
        },
      ],
      viewport: { x: 0, y: 0, zoom: 1 },
    });
    expect(validateWorkflowCanvasGraph(null)).toBe(false);
    expect(validateWorkflowCanvasGraph({ nodes: [], edges: "bad" })).toBe(false);
    expect(validateWorkflowCanvasGraph({ nodes: [], edges: [{ id: "bad" }] })).toBe(false);
    expect(validateWorkflowCanvasGraph({
      nodes: [{ id: 1, type: "start", position: { x: 0, y: 0 } }],
      edges: [],
    })).toBe(false);

    expect(
      sanitizeWorkflowCanvasGraph({ viewport: { x: 4, y: 5, zoom: 0.8 } } as never),
    ).toEqual({
      version: "adk-workflow-canvas/v1",
      nodes: [],
      edges: [],
      viewport: { x: 4, y: 5, zoom: 0.8 },
    });
  });

  it("keeps existing trigger nodes and avoids dangling trigger edges without a Start node", () => {
    const workflow = {
      ...buildWorkflow(),
      canvasGraph: {
        version: "adk-workflow-canvas/v1",
        nodes: [{ id: "trigger:trigger-1", type: "trigger", position: { x: 1, y: 2 } }],
        edges: [],
      },
    };

    const graph = workflowToCanvasGraph(workflow, [buildTrigger()]);

    expect(graph.nodes).toHaveLength(1);
    expect(graph.edges).toEqual([]);
  });

  it("uses the trigger type when a newly merged legacy trigger has no title", () => {
    const workflow = {
      ...buildWorkflow(),
      canvasGraph: {
        version: "adk-workflow-canvas/v1",
        nodes: [{ id: "start", type: "start", position: { x: 1, y: 2 } }],
        edges: [],
      },
    };

    const graph = workflowToCanvasGraph(workflow, [
      { ...buildTrigger(), id: "untitled", title: "", type: "event" },
    ]);

    expect(graph.nodes.find((node) => node.id === "trigger:untitled")?.data?.title).toBe("事件");
  });

  it("writes canvas graph into workflow payload with form values", () => {
    const form = workflowToForm(buildWorkflow());
    form.inputRows.push(createWorkflowInputRow("threshold", 100));
    const graph = legacyWorkflowToCanvasGraph(buildWorkflow(), [buildTrigger()]);

    const payload = canvasGraphToWorkflowPayload(form, graph);

    expect(payload.defaultInputs).toMatchObject({ symbol: "US.AAPL", threshold: 100 });
    expect(payload.canvasGraph?.nodes?.map((node) => node.id)).toContain("agent");
  });

  it("builds workflow run links with session and run ids", () => {
    expect(
      workflowRunLink({
        log: { sessionId: "session-workflow", runId: "run-workflow" },
      }),
    ).toBe("/adk/agents?sessionId=session-workflow&runId=run-workflow");
    expect(
      workflowRunLink({
        response: { session: { id: "session-response" }, run: { id: "run-response" } },
        log: { sessionId: "session-log", runId: "run-log" },
      }),
    ).toBe("/adk/agents?sessionId=session-response&runId=run-response");
    expect(workflowRunLink({})).toBe("/adk/agents");
  });
});

function buildWorkflow(): ADKWorkflowDefinition {
  return {
    id: "workflow-1",
    name: "Daily Review",
    status: "DISABLED",
    agentId: "agent-1",
    workMode: "loop",
    permissionMode: "approval",
    promptTemplate: "Run {{ .symbol }}",
    defaultInputs: { symbol: "US.AAPL" },
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
  };
}

function buildTrigger(): ADKWorkflowTrigger {
  return {
    id: "trigger-1",
    workflowId: "workflow-1",
    type: "schedule",
    title: "Market open",
    status: "ENABLED",
    config: { cron: "30 9 * * 1-5", timezone: "Asia/Shanghai" },
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
  };
}
