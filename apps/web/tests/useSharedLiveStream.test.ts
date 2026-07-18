import { describe, expect, it, vi } from "vitest";

import { useSharedLiveStream } from "../src/composables/useSharedLiveStream";

describe("useSharedLiveStream", () => {
  it("fails fast when the app-shell provider is missing", () => {
    vi.spyOn(console, "warn").mockImplementation(() => undefined);

    expect(() => useSharedLiveStream()).toThrow(
      "Live stream not provided. Call provideLiveStreamStore() in AppShell.",
    );
  });
});
