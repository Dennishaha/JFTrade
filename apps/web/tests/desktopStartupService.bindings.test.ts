import { beforeEach, describe, expect, it, vi } from "vitest";

const runtime = vi.hoisted(() => ({
  byID: vi.fn(),
}));

vi.mock("@wailsio/runtime", () => ({
  Call: { ByID: runtime.byID },
}));

import {
  Quit,
  Snapshot,
} from "@/wails/github.com/jftrade/jftrade-main/cmd/jftrade-desktop/desktopstartupservice";

beforeEach(() => {
  runtime.byID.mockReset();
});

describe("DesktopStartupService Wails binding", () => {
  it("forwards Quit and Snapshot to their generated method IDs", async () => {
    runtime.byID
      .mockResolvedValueOnce(undefined)
      .mockResolvedValueOnce({ state: "ready" });

    await expect(Quit()).resolves.toBeUndefined();
    await expect(Snapshot()).resolves.toEqual({ state: "ready" });
    expect(runtime.byID.mock.calls).toEqual([[4016114987], [411152216]]);
  });
});
