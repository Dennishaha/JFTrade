import type { Ref } from "vue";

import { buildApiUrl } from "./apiClient";

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
  let consoleStream: EventSource | null = null;
  let initialized = false;

  function openConsoleStream(): void {
    if (typeof EventSource === "undefined") {
      return;
    }

    consoleStream?.close();
    consoleStream = new EventSource(buildApiUrl("/api/v1/streams/console"));

    consoleStream.onopen = () => {
      options.liveStreamStatus.value = "connected";
    };

    consoleStream.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data) as { checkedAt?: string };
        options.liveStreamCheckedAt.value = payload.checkedAt ?? "";
      } catch {
        options.liveStreamCheckedAt.value = "";
      }

      void options.reloadSystemState({ background: true });
    };

    consoleStream.onerror = () => {
      options.liveStreamStatus.value = "degraded";
    };
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
    consoleStream?.close();
    consoleStream = null;
    initialized = false;
  }

  return {
    dispose,
    initialize,
  };
}