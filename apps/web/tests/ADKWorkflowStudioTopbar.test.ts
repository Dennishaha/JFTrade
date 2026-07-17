// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { describe, expect, it } from "vitest";

import ADKWorkflowStudioTopbar from "../src/components/adk-page/ADKWorkflowStudioTopbar.vue";

const toolbarActionStub = defineComponent({
  name: "ToolbarActionButton",
  inheritAttrs: false,
  props: ["icon", "label", "tone", "disabled", "loading"],
  emits: ["click"],
  template:
    "<button type='button' v-bind='$attrs' :disabled='disabled || loading' @click='$emit(\"click\", $event)'>{{ label }}</button>",
});

const menuStub = defineComponent({
  template:
    "<div><slot name='activator' :props='{}' /><div data-testid='stub-menu-content'><slot /></div></div>",
});

const listStub = defineComponent({
  template: "<div><slot /></div>",
});

const listItemStub = defineComponent({
  props: ["disabled"],
  emits: ["click"],
  template:
    "<button type='button' class='v-list-item' :disabled='disabled' @click='$emit(\"click\", $event)'><slot name='prepend' /><slot /></button>",
});

const listItemTitleStub = defineComponent({
  template: "<span><slot /></span>",
});

const iconStub = defineComponent({
  template: "<i aria-hidden='true'><slot /></i>",
});

function mountTopbar() {
  return mount(ADKWorkflowStudioTopbar, {
    props: {
      title: "测试工作流",
      description: "移动端顶栏",
      status: "draft",
      statusTone: "is-muted",
      statusLabel: "草稿",
      loading: false,
      saving: false,
      runningWorkflow: false,
      logLoading: false,
      hasWorkflow: true,
    },
    global: {
      stubs: {
        ToolbarActionButton: toolbarActionStub,
        "v-menu": menuStub,
        "v-list": listStub,
        "v-list-item": listItemStub,
        "v-list-item-title": listItemTitleStub,
        "v-icon": iconStub,
      },
    },
  });
}

describe("ADKWorkflowStudioTopbar", () => {
  it("keeps primary actions visible and routes secondary actions through the mobile more menu", async () => {
    const wrapper = mountTopbar();

    expect(wrapper.get('[data-testid="adk-workflow-run-button"]').text()).toBe("运行");
    expect(wrapper.get('[data-testid="adk-workflow-mobile-more"]').text()).toBe("更多");

    const moreItems = wrapper.findAll(".v-list-item");
    expect(moreItems.map((item) => item.text().replace(/^fa-solid fa-[a-z-]+/, ""))).toEqual([
      "刷新",
      "显示右栏",
      "添加触发器",
      "添加智能体",
      "触发日志",
      "调试",
      "复制",
      "存为模板",
      "删除工作流",
    ]);

    await moreItems[3]!.trigger("click");
    await moreItems[8]!.trigger("click");

    expect(wrapper.emitted("addAgent")).toHaveLength(1);
    expect(wrapper.emitted("remove")).toHaveLength(1);
  });

  it("forwards every available desktop and overflow action", async () => {
    const wrapper = mountTopbar();

    const desktopActions = [
      ["刷新", "refresh"],
      ["显示右栏", "showInspector"],
      ["添加触发器", "addTrigger"],
      ["添加智能体", "addAgent"],
      ["触发日志", "openLogs"],
      ["运行", "run"],
      ["调试", "debug"],
      ["复制", "duplicate"],
      ["存为模板", "saveTemplate"],
      ["删除工作流", "remove"],
      ["保存", "save"],
    ] as const;
    for (const [label] of desktopActions) {
      const button = wrapper.findAll("button").find((candidate) =>
        candidate.text() === label,
      );
      expect(button, `missing ${label}`).toBeDefined();
      await button?.trigger("click");
    }

    for (const [, event] of desktopActions) {
      expect(wrapper.emitted(event)).toHaveLength(1);
    }

    const overflowItems = wrapper.findAll(".v-list-item");
    for (const item of overflowItems) {
      await item.trigger("click");
    }
    for (const event of [
      "refresh",
      "showInspector",
      "addTrigger",
      "addAgent",
      "openLogs",
      "debug",
      "duplicate",
      "saveTemplate",
      "remove",
    ]) {
      expect(wrapper.emitted(event)).toHaveLength(2);
    }
  });

  it("uses draft copy and disables workflow-dependent actions while saving", async () => {
    const wrapper = mountTopbar();
    await wrapper.setProps({
      title: "",
      description: "",
      saving: true,
      hasWorkflow: false,
    });

    expect(wrapper.text()).toContain("新的工作流");
    expect(wrapper.text()).toContain("开始 -> 智能体 -> 监控");
    expect(wrapper.findAll("button").some((button) => button.text() === "删除工作流")).toBe(false);
    for (const label of ["复制", "存为模板", "运行", "调试"]) {
      const button = wrapper.findAll("button").find((candidate) =>
        candidate.text() === label,
      );
      expect(button, `missing ${label}`).toBeDefined();
      expect(button?.attributes("disabled")).toBeDefined();
    }
  });

  it("forwards every enabled mobile-overflow command through Vuetify list-item events", async () => {
    const wrapper = mountTopbar();
    const items = wrapper.findAllComponents(listItemStub);

    expect(items).toHaveLength(9);
    for (const item of items) {
      item.vm.$emit("click");
    }
    await wrapper.vm.$nextTick();

    for (const event of [
      "refresh",
      "showInspector",
      "addTrigger",
      "addAgent",
      "openLogs",
      "debug",
      "duplicate",
      "saveTemplate",
      "remove",
    ]) {
      expect(wrapper.emitted(event)).toHaveLength(1);
    }
  });
});
