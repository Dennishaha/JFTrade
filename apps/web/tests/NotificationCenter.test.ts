// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { defineComponent, nextTick } from "vue";

import NotificationCenter from "../src/layout/NotificationCenter.vue";
import { provideNotificationsStore } from "../src/composables/useNotifications";

const NotificationCenterHarness = defineComponent({
  components: { NotificationCenter },
  setup() {
    const notifications = provideNotificationsStore();
    notifications.push({
      level: "warn",
      title: "OpenD 连接状态变化",
      message: "行情未登录，交易未登录。",
      source: "futu-opend",
      category: "broker.connection",
      at: "2026-05-21T10:00:00.000Z",
    });
    notifications.push({
      level: "info",
      title: "Futu API 订阅额度更新",
      message: "剩余 16，当前连接已用 4，总已用 8。",
      source: "futu-opend",
      category: "broker.quota",
      at: "2026-05-21T10:01:00.000Z",
    });
    notifications.push({
      level: "warn",
      title: "BBGO 通知",
      message: "Strategy risk warning",
      source: "bbgo.notify",
      category: "bbgo.notify",
      at: "2026-05-21T10:02:00.000Z",
    });
    return {};
  },
  template: "<NotificationCenter />",
});

describe("NotificationCenter", () => {
  it("filters notifications by level, source, and category", async () => {
    const wrapper = mount(NotificationCenterHarness);

    expect(wrapper.text()).toContain("OpenD 连接状态变化");
    expect(wrapper.text()).toContain("Futu API 订阅额度更新");
    expect(wrapper.text()).toContain("Strategy risk warning");

    await wrapper.get('[data-testid="notification-category-filter"]').setValue("broker.connection");
    await nextTick();

    expect(wrapper.text()).toContain("OpenD 连接状态变化");
    expect(wrapper.text()).not.toContain("Futu API 订阅额度更新");
    expect(wrapper.text()).not.toContain("Strategy risk warning");

    await wrapper.get('[data-testid="notification-category-filter"]').setValue("all");
    await wrapper.get('[data-testid="notification-source-filter"]').setValue("bbgo.notify");
    await nextTick();

    expect(wrapper.text()).toContain("Strategy risk warning");
    expect(wrapper.text()).not.toContain("OpenD 连接状态变化");
    expect(wrapper.text()).not.toContain("Futu API 订阅额度更新");

    await wrapper.get('[data-testid="notification-source-filter"]').setValue("all");
    await wrapper.get('[data-testid="notification-level-filter"]').setValue("info");
    await nextTick();

    expect(wrapper.text()).toContain("Futu API 订阅额度更新");
    expect(wrapper.text()).not.toContain("OpenD 连接状态变化");
    expect(wrapper.text()).not.toContain("Strategy risk warning");
  });
});