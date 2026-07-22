type ProcessRuntime = {
  platform?: string;
  on?: (event: string, handler: (error: unknown) => void) => unknown;
  exit?: (code?: number) => never;
  resourceUsage?: () => { maxRSS: number };
};

export function peakRSSBytes(maxRSS: number, platform: string | undefined): number {
  if (!Number.isFinite(maxRSS) || maxRSS <= 0) {
    throw new Error(`invalid Pine worker max RSS: ${maxRSS}`);
  }
  // Node reports bytes on macOS and KiB on other supported platforms.
  return Math.round(platform === "darwin" ? maxRSS : maxRSS * 1024);
}

export function createPeakRSSReader(runtime: ProcessRuntime | undefined): () => number {
  if (!runtime?.resourceUsage) {
    throw new Error("process.resourceUsage is required for Pine worker RSS accounting");
  }
  const read = () => peakRSSBytes(runtime.resourceUsage!().maxRSS, runtime.platform);
  read();
  return read;
}

export function installFatalErrorHandlers(
  runtime: ProcessRuntime | undefined,
  logError: (message: string, error: unknown) => void = console.error,
): void {
  const fail = (message: string) => (error: unknown) => {
    logError(message, error);
    runtime?.exit?.(1);
  };
  runtime?.on?.("uncaughtException", fail("pineworker uncaught exception"));
  runtime?.on?.("unhandledRejection", fail("pineworker unhandled rejection"));
}
