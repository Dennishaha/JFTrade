// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { defineComponent } from "vue";

import StrategyRuntimeActivityPanel from "../src/components/strategy-runtime/StrategyRuntimeActivityPanel.vue";

const dialogStub = defineComponent({
  props: ["modelValue"],
  template: "<div v-if=\"modelValue\" class='dialog-stub'><slot /></div>",
});

const monacoStub = defineComponent({
  props: ["modelValue", "testId"],
  template:
    "<pre class='monaco-stub' :data-testid=\"testId\">{{ modelValue }}</pre>",
});

function mountPanel(
  overrides: Partial<{
    isLoadingDetails: boolean;
    strategyLogs: string[];
    strategyAuditEntries: Array<{
      instanceId: string;
      kind: string;
      detail?: string | null;
      at: string;
    }>;
    selectedStrategyParamsJson: string;
  }> = {},
) {
  return mount(StrategyRuntimeActivityPanel, {
    props: {
      isLoadingDetails: false,
      strategyLogs: [
        "2026-07-03T01:00:00.000Z Order failed: timeout",
        "2026-07-03T02:00:00.000Z paused for retry",
        "manual heartbeat synced",
      ],
      strategyAuditEntries: [
        {
          instanceId: "instance-1",
          kind: "started",
          detail: "strategy booted",
          at: "2026-07-03T01:30:00.000Z",
        },
        {
          instanceId: "instance-1",
          kind: "failed",
          detail: "broker rejected order",
          at: "2026-07-03T03:30:00.000Z",
        },
      ],
      selectedStrategyParamsJson: "{\"market\":\"US\",\"symbol\":\"AAPL\"}",
      ...overrides,
    },
    global: {
      stubs: {
        "v-dialog": dialogStub,
        MonacoCodeEditor: monacoStub,
      },
    },
    attachTo: document.body,
  });
}

describe("StrategyRuntimeActivityPanel", () => {
  it("shows loading and empty states for logs and audit tabs", async () => {
    const wrapper = mountPanel({
      isLoadingDetails: true,
      strategyLogs: [],
      strategyAuditEntries: [],
    });

    expect(wrapper.text()).toContain("正在加载运行明细");

    await wrapper.setProps({ isLoadingDetails: false });
    expect(wrapper.text()).toContain("暂无日志。");

    await wrapper.get('[data-testid="strategy-activity-tab-audit"]').trigger("click");
    expect(wrapper.text()).toContain("暂无审计记录。");
  });

  it("classifies logs, filters by severity, opens detail dialogs, and shows parameter snapshots", async () => {
    const wrapper = mountPanel();

    expect(wrapper.get('[data-testid="strategy-activity-filter-all"]').text()).toContain(
      "3",
    );
    expect(
      wrapper.get('[data-testid="strategy-activity-filter-error"]').text(),
    ).toContain("1");
    expect(
      wrapper.get('[data-testid="strategy-activity-filter-warning"]').text(),
    ).toContain("1");
    expect(
      wrapper.get('[data-testid="strategy-activity-filter-info"]').text(),
    ).toContain("1");
    expect(wrapper.get('[data-testid="strategy-log-entry-0"]').text()).toContain(
      "需关注",
    );
    expect(wrapper.get('[data-testid="strategy-log-entry-1"]').text()).toContain(
      "高优先",
    );
    expect(wrapper.get('[data-testid="strategy-log-entry-2"]').text()).toContain(
      "未标注时间",
    );

    await wrapper.get('[data-testid="strategy-activity-filter-error"]').trigger("click");
    expect(wrapper.findAll('[data-testid^="strategy-log-entry-"]')).toHaveLength(1);
    expect(wrapper.text()).toContain("Order failed: timeout");

    await wrapper
      .get('[data-testid="strategy-log-detail-trigger-0"]')
      .trigger("click");
    expect(wrapper.get('[data-testid="strategy-activity-detail-dialog"]').text()).toContain(
      "运行日志",
    );
    expect(wrapper.text()).toContain("高优先");

    await wrapper
      .get('[data-testid="strategy-close-activity-detail-dialog"]')
      .trigger("click");
    expect(wrapper.find('[data-testid="strategy-activity-detail-dialog"]').exists()).toBe(
      false,
    );

    await wrapper.get('[data-testid="strategy-open-params-dialog"]').trigger("click");
    expect(wrapper.get('[data-testid="strategy-params-dialog"]').text()).toContain(
      "运行参数",
    );
    expect(wrapper.get('[data-testid="strategy-params-editor"]').text()).toContain(
      "\"symbol\":\"AAPL\"",
    );
    await wrapper.get('[data-testid="strategy-close-params-dialog"]').trigger("click");
    expect(wrapper.find('[data-testid="strategy-params-dialog"]').exists()).toBe(
      false,
    );
  });

  it("formats audit entries, resets invalid filters across tabs, and builds audit detail payloads", async () => {
    const wrapper = mountPanel({
      strategyAuditEntries: [
        {
          instanceId: "instance-1",
          kind: "started",
          detail: "strategy booted",
          at: "2026-07-03T01:30:00.000Z",
        },
        {
          instanceId: "instance-1",
          kind: "risk_rejected",
          detail: "risk rejected oversized order",
          at: "invalid-audit-time",
        },
      ],
    });

    await wrapper.get('[data-testid="strategy-activity-filter-warning"]').trigger("click");
    await wrapper.get('[data-testid="strategy-activity-tab-audit"]').trigger("click");

    expect(
      wrapper.get('[data-testid="strategy-activity-filter-all"]').classes(),
    ).toContain("is-active");
    expect(wrapper.findAll('[data-testid^="strategy-audit-entry-"]')).toHaveLength(2);
    expect(wrapper.get('[data-testid="strategy-audit-entry-0"]').text()).toContain(
      "已启动",
    );
    expect(wrapper.get('[data-testid="strategy-audit-entry-1"]').text()).toContain(
      "风控拒单",
    );

    await wrapper
      .get('[data-testid="strategy-audit-detail-trigger-1"]')
      .trigger("click");

    const detail = wrapper.get('[data-testid="strategy-activity-detail-dialog"]');
    expect(detail.text()).toContain("风控拒单");
    expect(detail.text()).toContain("原始类型");
    expect(detail.text()).toContain("risk_rejected");
    expect(detail.text()).toContain("instanceId: instance-1");
    expect(detail.text()).toContain("detail: risk rejected oversized order");
  });
});
