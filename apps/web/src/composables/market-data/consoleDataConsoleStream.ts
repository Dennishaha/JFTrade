import { watch, type Ref } from "vue";

import {
  getSharedLiveSocketHub,
  type ConsoleRefreshLiveStreamEvent,
} from "@/composables/market-data/sharedLiveSocket";

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
  const hub = getSharedLiveSocketHub();
  let initialized = false;
  const ownerId = hub.createOwnerId("console-refresh");
  let removeListener: (() => void) | null = null;
  let stopConnectionStateWatch: (() => void) | null = null;

  function startConsoleRefreshSubscription(): void {
    hub.setConsoleRefreshEnabled(ownerId, true);
    removeListener = hub.addEventListener((event) => {
      if (!isConsoleRefreshEvent(event)) {
        if (isLiveResyncEvent(event)) {
          void options.reloadSystemState({
            background: true,
            bypassCooldown: true,
          });
        }
        return;
      }
      options.liveStreamCheckedAt.value = event.checkedAt ?? "";
      void options.reloadSystemState({ background: true });
    });
    stopConnectionStateWatch = watch(
      hub.connectionState,
      (state, previousState) => {
        options.liveStreamStatus.value =
          state === "connected" ? "connected" : state === "error" ? "degraded" : "disconnected";

        // The server does not replay console.refresh events that were
        // published while this socket was disconnected.  Reconcile the
        // server-owned state whenever a live channel recovers after the
        // initial subscription; otherwise a background disconnect can leave
        // every page showing a stale execution/settings snapshot indefinitely.
        if (
          state === "connected" &&
          previousState !== undefined &&
          previousState !== "connected"
        ) {
          void options.reloadSystemState({
            background: true,
            bypassCooldown: true,
          });
        }
      },
      { immediate: true },
    );
  }

  async function initialize(): Promise<void> {
    if (initialized) {
      return;
    }

    initialized = true;
    await options.reloadSystemState();
    startConsoleRefreshSubscription();
  }

  function dispose(): void {
    hub.setConsoleRefreshEnabled(ownerId, false);
    removeListener?.();
    removeListener = null;
    stopConnectionStateWatch?.();
    stopConnectionStateWatch = null;
    initialized = false;
  }

  return {
    dispose,
    initialize,
  };
}

function isConsoleRefreshEvent(
  event: { type: string },
): event is ConsoleRefreshLiveStreamEvent {
  return event.type === "console.refresh";
}

function isLiveResyncEvent(
  event: { type: string },
): event is { type: "live.resync" } {
  return event.type === "live.resync";
}
