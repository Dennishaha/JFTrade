import type { Ref } from "vue";

import { buildApiUrl } from "./apiClient";
import { createEventSourceStream } from "./eventSourceStream";

type LiveStreamStatus = "disconnected" | "connected" | "degraded";

interface CreateConsoleDataConsoleStreamControllerOptions {
  liveStreamStatus: Ref<LiveStreamStatus>;
  liveStreamCheckedAt: Ref<string>;
  reloadSystemState: (options?: {
    background?: boolean;
    bypassCooldown?: boolean;
  }) => Promise<void>;
}

export function createConsoleDataConsoleStreamController(
  options: CreateConsoleDataConsoleStreamControllerOptions,
) {
  const consoleStream = createEventSourceStream<{ checkedAt?: string }>({
    onOpen: () => {
      options.liveStreamStatus.value = "connected";
    },
    onMessage: (payload) => {
      options.liveStreamCheckedAt.value = payload.checkedAt ?? "";
      void options.reloadSystemState({ background: true });
    },
    onError: () => {
      options.liveStreamStatus.value = "degraded";
    },
  });
  let initialized = false;

  function openConsoleStream(): void {
    consoleStream.connect(buildApiUrl("/api/sse/console"));
  }

  async function initialize(): Promise<void> {
    if (initialized) {
      return;
    }

    initialized = true;
    await options.reloadSystemState();
    openConsoleStream();
  }

  function dispose(): void {
    consoleStream.disconnect(false);
    initialized = false;
  }

  return {
    dispose,
    initialize,
  };
}
