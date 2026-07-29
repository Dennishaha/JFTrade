import { onScopeDispose, readonly, ref, type Ref } from "vue";

export interface PollingContext {
  runCount: number;
  stop: () => void;
}

export interface UsePollingOptions {
  intervalMs: number;
  immediate?: boolean;
  maxRuns?: number;
  onError?: (cause: unknown) => void;
}

export interface PollingStartOptions {
  immediate?: boolean;
  resetRunCount?: boolean;
}

export interface PollingController {
  isActive: Readonly<Ref<boolean>>;
  isRunning: Readonly<Ref<boolean>>;
  runCount: Readonly<Ref<number>>;
  start: (options?: PollingStartOptions) => void;
  stop: () => void;
}

export function usePolling(
  task: (context: PollingContext) => boolean | void | Promise<boolean | void>,
  options: UsePollingOptions,
): PollingController {
  if (!Number.isFinite(options.intervalMs) || options.intervalMs <= 0) {
    throw new Error("usePolling intervalMs must be a positive finite number");
  }

  const intervalMs = Math.trunc(options.intervalMs);
  const maxRuns = normalizeMaxRuns(options.maxRuns);
  const isActive = ref(false);
  const isRunning = ref(false);
  const runCount = ref(0);
  let timer: ReturnType<typeof setTimeout> | null = null;
  let generation = 0;

  function clearTimer(): void {
    if (timer == null) return;
    clearTimeout(timer);
    timer = null;
  }

  function stop(): void {
    generation += 1;
    clearTimer();
    isActive.value = false;
  }

  function schedule(expectedGeneration: number): void {
    if (
      !isActive.value ||
      generation !== expectedGeneration ||
      (maxRuns != null && runCount.value >= maxRuns)
    ) {
      stop();
      return;
    }
    clearTimer();
    timer = setTimeout(() => {
      timer = null;
      void execute(expectedGeneration);
    }, intervalMs);
  }

  async function execute(expectedGeneration: number): Promise<void> {
    if (!isActive.value || generation !== expectedGeneration) return;
    if (isRunning.value) {
      schedule(expectedGeneration);
      return;
    }

    isRunning.value = true;
    runCount.value += 1;
    let shouldContinue = true;
    try {
      shouldContinue =
        (await task({ runCount: runCount.value, stop })) !== false;
    } catch (cause) {
      shouldContinue = false;
      options.onError?.(cause);
    } finally {
      isRunning.value = false;
    }

    if (
      !shouldContinue ||
      !isActive.value ||
      generation !== expectedGeneration
    ) {
      if (generation === expectedGeneration) stop();
      return;
    }
    schedule(expectedGeneration);
  }

  function start(startOptions: PollingStartOptions = {}): void {
    stop();
    if (startOptions.resetRunCount ?? true) runCount.value = 0;
    isActive.value = true;
    const expectedGeneration = generation;
    if (startOptions.immediate ?? options.immediate ?? false) {
      void execute(expectedGeneration);
      return;
    }
    schedule(expectedGeneration);
  }

  onScopeDispose(stop);

  return {
    isActive: readonly(isActive),
    isRunning: readonly(isRunning),
    runCount: readonly(runCount),
    start,
    stop,
  };
}

function normalizeMaxRuns(value: number | undefined): number | null {
  if (value == null) return null;
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error("usePolling maxRuns must be a positive finite number");
  }
  return Math.trunc(value);
}
