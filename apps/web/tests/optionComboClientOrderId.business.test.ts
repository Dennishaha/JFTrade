import { afterEach, describe, expect, it, vi } from "vitest";

import { createOptionComboClientOrderId } from "../src/features/optionComboBuilder";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("option combo client order identity", () => {
  it("uses a deterministic timestamp fallback when random UUID is unavailable", () => {
    vi.stubGlobal("crypto", {});
    vi.spyOn(Date, "now").mockReturnValue(1_753_747_200_000);
    vi.spyOn(Math, "random").mockReturnValue(0.5);

    expect(createOptionComboClientOrderId()).toBe(
      "jftrade-option-combo-1753747200000-8",
    );
  });
});
