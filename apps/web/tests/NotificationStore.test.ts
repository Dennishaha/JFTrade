// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { defineComponent } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  provideNotificationsStore,
  useNotifications,
} from "../src/composables/useNotifications";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("notifications store", () => {
  it("uses a stable fallback ID and manages the user-visible notification lifecycle", () => {
    vi.stubGlobal("crypto", undefined);
    let store: ReturnType<typeof provideNotificationsStore> | undefined;
    mount(defineComponent({
      setup() {
        store = provideNotificationsStore();
        return {};
      },
      template: "<div />",
    }));

    store!.push({ level: "warn", title: "Risk warning", at: "2026-07-16T00:00:00Z" });
    store!.push({ level: "error", title: "Order rejected", at: "2026-07-16T00:01:00Z" });
    expect(store!.items.value[0]?.id).toMatch(/^n-/);
    expect(store!.unreadCount.value).toBe(2);

    store!.markAllRead();
    expect(store!.unreadCount.value).toBe(0);
    const retainedId = store!.items.value[0]?.id ?? "";
    store!.remove(retainedId);
    expect(store!.items.value).toHaveLength(1);
    store!.clear();
    expect(store!.items.value).toEqual([]);
  });

  it("fails loudly when a consumer is mounted outside the notification provider", () => {
    expect(() => useNotifications()).toThrow("Notifications store not provided.");
  });
});
