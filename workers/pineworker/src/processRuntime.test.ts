import { describe, expect, it, vi } from "vitest";
import { createPeakRSSReader, installFatalErrorHandlers, peakRSSBytes } from "./processRuntime";

describe("Pine worker process runtime", () => {
  it("normalizes platform-specific maxRSS units", () => {
    expect(peakRSSBytes(48_000_000, "darwin")).toBe(48_000_000);
    expect(peakRSSBytes(48_000, "linux")).toBe(49_152_000);
    expect(() => peakRSSBytes(0, "linux")).toThrow("invalid Pine worker max RSS");
  });

  it("reads and validates peak RSS", () => {
    const read = createPeakRSSReader({ platform: "darwin", resourceUsage: () => ({ maxRSS: 12_345 }) });
    expect(read()).toBe(12_345);
  });

  it("logs fatal process errors and exits", () => {
    const handlers = new Map<string, (error: unknown) => void>();
    const logError = vi.fn();
    const exit = vi.fn() as unknown as (code?: number) => never;
    installFatalErrorHandlers(
      {
        on: (event, handler) => handlers.set(event, handler),
        exit,
      },
      logError,
    );

    const failure = new Error("broken state");
    handlers.get("uncaughtException")?.(failure);
    expect(logError).toHaveBeenCalledWith("pineworker uncaught exception", failure);
    expect(exit).toHaveBeenCalledWith(1);
  });
});
