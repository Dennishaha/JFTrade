// @vitest-environment jsdom

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick, ref } from "vue";

import { useADKPageRuntimePersistence } from "@/composables/adk/useADKPageRuntimePersistence";

afterEach(() => {
  window.localStorage.clear();
  window.sessionStorage.clear();
  vi.unstubAllGlobals();
});

describe("adk page runtime persistence boundaries", () => {
  it("returns an empty runtime state for blank session ids", () => {
    const { runtime } = mountRuntime();
    expect(runtime.sessionRuntimeState("  ")).toEqual(
      expect.objectContaining({ streamId: "", runId: "", sequence: 0 }),
    );
  });

  it("skips persisting active child runs when the selected session is blank", async () => {
    const { activeChildRunId, runtime } = mountRuntime();
    activeChildRunId.value = "child-run-2";
    await nextTick();
    expect(runtime.pageState.sessions).toEqual({});
  });
});

function mountRuntime() {
  const activeChildRunId = ref("child-run-1");
  const selectedSessionId = ref("");
  let runtime!: ReturnType<typeof useADKPageRuntimePersistence>;
  mount(
    defineComponent({
      setup() {
        runtime = useADKPageRuntimePersistence({
          activeChildRunId,
          selectedSessionId,
        });
        return () => h("div");
      },
    }),
  );
  return { activeChildRunId, runtime, selectedSessionId };
}
