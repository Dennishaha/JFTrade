import type { ADKRun } from "@/contracts";

import { fetchEnvelope } from "./apiClient";
import { buildRunObservationSignature } from "./adkChatRuntime";
import { isTerminalRunStatus } from "./adkChatPresentation";

export interface ADKRunContinuationOptions {
  pollIntervalMs?: number;
  timeoutMs?: number;
  onProgress?: (latestRun: ADKRun, previousRun: ADKRun) => void | Promise<void>;
  onTerminal?: (latestRun: ADKRun) => void | Promise<void>;
}

export async function monitorADKRunContinuation(
  run: ADKRun | undefined,
  options: ADKRunContinuationOptions = {},
): Promise<ADKRun | undefined> {
  if (!run || isTerminalRunStatus(run.status)) {
    return run;
  }
  const pollIntervalMs = options.pollIntervalMs ?? 900;
  const timeoutMs = options.timeoutMs ?? 15_000;
  const deadline = Date.now() + timeoutMs;
  let previousRun = run;
  let previousSignature = buildRunObservationSignature(run);

  while (Date.now() < deadline) {
    await delay(pollIntervalMs);
    const latestRun = await fetchLatestRun(run.id);
    const changed = await publishProgressIfChanged(
      latestRun,
      options,
      previousRun,
      previousSignature,
    );
    if (changed) {
      previousRun = latestRun;
      previousSignature = buildRunObservationSignature(latestRun);
    }
    if (isTerminalRunStatus(latestRun.status)) {
      await options.onTerminal?.(latestRun);
      return latestRun;
    }
  }

  const latestRun = await fetchLatestRun(run.id);
  const changed = await publishProgressIfChanged(
    latestRun,
    options,
    previousRun,
    previousSignature,
  );
  if (changed) {
    previousRun = latestRun;
  }
  if (isTerminalRunStatus(latestRun.status)) {
    await options.onTerminal?.(latestRun);
    return latestRun;
  }
  if (hasFailedToolSnapshot(latestRun)) {
    return latestRun;
  }
  return previousRun;
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

async function fetchLatestRun(runId: string): Promise<ADKRun> {
  return fetchEnvelope<ADKRun>(`/api/v1/adk/runs/${encodeURIComponent(runId)}`);
}

async function publishProgressIfChanged(
  latestRun: ADKRun,
  options: ADKRunContinuationOptions,
  previousRun: ADKRun,
  previousSignature: string,
): Promise<boolean> {
  const latestSignature = buildRunObservationSignature(latestRun);
  const changed =
    latestRun.status !== previousRun.status ||
    latestSignature !== previousSignature;
  if (changed) {
    await options.onProgress?.(latestRun, previousRun);
  }
  return changed;
}

function hasFailedToolSnapshot(run: ADKRun | undefined): boolean {
  if (!run) return false;
  return run.toolCalls.some(
    (toolCall) =>
      toolCall.status === "FAILED" || toolCall.status === "TIMED_OUT",
  );
}
