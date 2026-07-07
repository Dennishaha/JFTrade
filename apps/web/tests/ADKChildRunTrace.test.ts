// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import type { ADKRun } from "@/contracts";

import ADKChildRunTrace from "../src/components/shared/ADKChildRunTrace.vue";
import type { ADKChildRunQueueItem } from "../src/composables/useADKWorkflowQueueState";

describe("ADKChildRunTrace", () => {
  it("renders collapsed child run state", () => {
    const wrapper = mount(ADKChildRunTrace, {
      props: {
        item: buildItem({ status: "RUNNING", stepTitle: "读取行情" }),
      },
    });

    expect(wrapper.text()).toContain("#1");
    expect(wrapper.text()).toContain("读取行情");
    expect(wrapper.text()).toContain("运行中");
    expect(wrapper.find(".adk-run-trace-detail").exists()).toBe(false);
  });

  it("uses a compact timeline variant without changing the queue default", () => {
    const timeline = mount(ADKChildRunTrace, {
      props: {
        item: buildItem({ status: "RUNNING", stepTitle: "读取行情" }),
        variant: "timeline",
      },
    });
    const queue = mount(ADKChildRunTrace, {
      props: {
        item: buildItem({ status: "RUNNING", stepTitle: "读取行情" }),
      },
    });

    expect(timeline.classes()).toContain("adk-child-run-trace--timeline");
    expect(queue.classes()).not.toContain("adk-child-run-trace--timeline");
  });

  it("expands failed child run details", async () => {
    const wrapper = mount(ADKChildRunTrace, {
      props: {
        item: buildItem({
          status: "FAILED",
          stepTitle: "模型调用",
          errorSummary: "模型调用失败：服务商余额不足",
          errorDetail: "provider returned 402: Insufficient Balance",
          errorCode: "MODEL_CALL_FAILED",
          parentRunId: "parent-run",
          run: buildRun({
            id: "child-run",
            status: "FAILED",
            failureReason: "provider returned 402: Insufficient Balance",
            errorCode: "MODEL_CALL_FAILED",
            userMessage: "请进行每日股票盘点",
          }),
        }),
      },
    });

    expect(wrapper.text()).toContain("模型调用失败：服务商余额不足");
    expect(wrapper.text()).not.toContain("provider returned 402");
    await wrapper.get(".adk-child-run-trace__card").trigger("click");

    expect(wrapper.text()).toContain("MODEL_CALL_FAILED");
    expect(wrapper.text()).toContain("child-run");
    expect(wrapper.text()).toContain("parent-run");
    expect(wrapper.text()).toContain("provider returned 402");
    expect(wrapper.text()).toContain("请进行每日股票盘点");
  });

  it("explains parent-terminated child runs", async () => {
    const wrapper = mount(ADKChildRunTrace, {
      props: {
        item: buildItem({
          status: "CANCELLED",
          errorSummary: "父工作流已终止，子智能体已取消",
          errorDetail: "parent workflow parent-run terminated",
          errorCode: "PARENT_RUN_TERMINATED",
          parentRunId: "parent-run",
          run: buildRun({
            status: "CANCELLED",
            failureReason: "parent workflow parent-run terminated",
            errorCode: "PARENT_RUN_TERMINATED",
            parentRunId: "parent-run",
          }),
        }),
      },
    });

    expect(wrapper.text()).toContain("父工作流已终止，子智能体已取消");
    await wrapper.get(".adk-child-run-trace__card").trigger("click");
    expect(wrapper.text()).toContain("PARENT_RUN_TERMINATED");
    expect(wrapper.text()).toContain("parent-run");
  });

  it("emits select from the enter action without expanding", async () => {
    const wrapper = mount(ADKChildRunTrace, {
      props: {
        item: buildItem({ id: "child-select" }),
        showEnter: true,
      },
    });

    await wrapper.get(".adk-workspace-queue-button").trigger("click");

    expect(wrapper.emitted("select")?.[0]).toEqual(["child-select"]);
    expect(wrapper.find(".adk-run-trace-detail").exists()).toBe(false);
  });
});

function buildItem(overrides: Partial<ADKChildRunQueueItem> = {}): ADKChildRunQueueItem {
  return {
    id: "child-run",
    index: 1,
    stepIndex: 1,
    stepTitle: "子智能体",
    status: "COMPLETED",
    updatedAt: "2026-07-03T10:00:00Z",
    pendingApprovalCount: 0,
    ...overrides,
  };
}

function buildRun(overrides: Partial<ADKRun> = {}): ADKRun {
  return {
    id: "child-run",
    sessionId: "session-1",
    agentId: "agent-1",
    status: "COMPLETED",
    message: "",
    toolCalls: [],
    pendingApprovals: [],
    createdAt: "2026-07-03T10:00:00Z",
    updatedAt: "2026-07-03T10:00:00Z",
    ...overrides,
  };
}
