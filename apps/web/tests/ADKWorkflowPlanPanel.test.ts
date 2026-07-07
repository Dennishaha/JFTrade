// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import type { ADKRun } from "@/contracts";

import ADKWorkflowPlanPanel from "../src/components/adk-page/ADKWorkflowPlanPanel.vue";

describe("ADKWorkflowPlanPanel", () => {
  it("does not render for chat runs or runs without a workflow plan", () => {
    expect(
      mount(ADKWorkflowPlanPanel, { props: { run: null } }).find(
        ".adk-workflow-plan-panel",
      ).exists(),
    ).toBe(false);
    expect(
      mount(ADKWorkflowPlanPanel, {
        props: { run: buildRun({ workMode: "chat", workflowPlan: steps(1) }) },
      })
        .find(".adk-workflow-plan-panel")
        .exists(),
    ).toBe(false);
    expect(
      mount(ADKWorkflowPlanPanel, {
        props: { run: buildRun({ workMode: "loop", workflowPlan: [] }) },
      })
        .find(".adk-workflow-plan-panel")
        .exists(),
    ).toBe(false);
  });

  it("renders workflow mode, objective, step statuses, and child metadata", async () => {
    const wrapper = mount(ADKWorkflowPlanPanel, {
      props: {
        run: buildRun({
          workMode: "loop",
          workflowStatus: "PAUSED",
          objective: "完成风险检查并生成交易建议",
          iteration: 2,
          childRunIds: ["child-1", "child-2"],
          workflowPlan: [
            {
              taskId: "step-1",
              title: "观察上下文",
              description: "读取当前账户和策略状态",
              status: "TODO",
            },
            {
              taskId: "step-2",
              title: "制定计划",
              status: "IN_PROGRESS",
              childRunId: "child-1",
              iteration: 2,
            },
            {
              taskId: "step-3",
              title: "等待审批",
              status: "BLOCKED",
            },
            {
              taskId: "step-4",
              title: "汇总结论",
              status: "DONE",
            },
          ],
        }),
      },
    });

    expect(wrapper.text()).toContain("执行计划");
    expect(wrapper.text()).toContain("观察上下文");

    await wrapper.find(".adk-workspace-queue__header").trigger("click");

    expect(wrapper.text()).toContain("完成风险检查并生成交易建议");
    expect(wrapper.text()).toContain("第 2 轮");
    expect(wrapper.text()).toContain("2 个子智能体");
    expect(wrapper.text()).toContain("待办");
    expect(wrapper.text()).toContain("进行中");
    expect(wrapper.text()).toContain("已阻断");
    expect(wrapper.text()).toContain("已完成");
    expect(wrapper.text()).not.toContain("IN_PROGRESS");
    expect(wrapper.text()).not.toContain("DONE");
    expect(wrapper.text()).toContain("子智能体 child-1");
    expect(wrapper.find(".adk-workspace-queue-status.is-warning").exists()).toBe(true);
    expect(wrapper.find(".adk-workspace-queue-status.is-success").exists()).toBe(true);
  });

  it("keeps long plans collapsed by default and expands into a scrollable queue", async () => {
    const wrapper = mount(ADKWorkflowPlanPanel, {
      props: {
        run: buildRun({
          workMode: "loop",
          workflowPlan: steps(7),
        }),
      },
    });

    expect(wrapper.text()).not.toContain("步骤 7");
    expect(wrapper.text()).toContain("展开");

    await wrapper.find(".adk-workspace-queue__header").trigger("click");

    expect(wrapper.text()).toContain("步骤 7");
    expect(wrapper.text()).toContain("收起");
    expect(wrapper.find(".adk-workspace-queue__body").exists()).toBe(true);
  });

  it("uses a completed parent workflow as the authoritative terminal plan state", async () => {
    const wrapper = mount(ADKWorkflowPlanPanel, {
      props: {
        run: buildRun({
          status: "COMPLETED",
          workMode: "loop",
          workflowStatus: "COMPLETED",
          workflowPlan: [
            {
              taskId: "step-stale",
              title: "任务步骤 1",
              status: "IN_PROGRESS",
              childRunId: "child-stale-running",
            },
          ],
        }),
      },
    });

    expect(wrapper.text()).toContain("已完成");
    expect(wrapper.text()).not.toContain("IN_PROGRESS");
    expect(wrapper.text()).not.toContain("DONE");
    expect(wrapper.find(".adk-workspace-queue__badge.is-success").exists()).toBe(true);

    await wrapper.find(".adk-workspace-queue__header").trigger("click");

    expect(wrapper.text()).toContain("任务步骤 1");
    expect(wrapper.text()).toContain("已完成");
    expect(wrapper.text()).not.toContain("IN_PROGRESS");
    expect(wrapper.text()).not.toContain("DONE");
    expect(wrapper.find(".adk-workspace-queue-status.is-success").exists()).toBe(true);
    expect(wrapper.find(".adk-workspace-queue__badge.is-error").exists()).toBe(false);
  });

  it("uses error tone only for failed workflow steps", async () => {
    const wrapper = mount(ADKWorkflowPlanPanel, {
      props: {
        run: buildRun({
          status: "FAILED",
          workMode: "loop",
          workflowStatus: "FAILED",
          workflowPlan: [
            {
              taskId: "step-failed",
              title: "失败分支",
              status: "FAILED",
              childRunId: "child-failed",
            },
          ],
        }),
      },
    });

    expect(wrapper.find(".adk-workspace-queue__badge.is-error").exists()).toBe(true);
    expect(wrapper.find(".adk-workspace-queue__badge.is-success").exists()).toBe(false);

    await wrapper.find(".adk-workspace-queue__header").trigger("click");

    expect(wrapper.text()).toContain("失败分支");
    expect(wrapper.find(".adk-workspace-queue-status.is-error").exists()).toBe(true);
    expect(wrapper.find(".adk-workspace-queue-status.is-success").exists()).toBe(false);
  });
});

function buildRun(overrides: Partial<ADKRun>): ADKRun {
  return {
    id: "run-1",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "RUNNING",
    message: "running",
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-06-06T00:00:00Z",
    updatedAt: "2026-06-06T00:00:00Z",
    ...overrides,
  };
}

function steps(count: number): ADKRun["workflowPlan"] {
  return Array.from({ length: count }, (_, index) => ({
    taskId: `step-${index + 1}`,
    title: `步骤 ${index + 1}`,
    status: index === 0 ? "IN_PROGRESS" : "TODO",
  }));
}
