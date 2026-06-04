import {
  getSharedLiveSocketHub,
  type LiveSocketConnectionState,
  type LiveStreamEvent,
  type MarketDataTickLiveEvent,
  type SystemNotificationLiveStreamEvent,
} from "./sharedLiveSocket";

export type LiveStreamConnectionState = LiveSocketConnectionState;
export type MarketDataTickLiveStreamEvent = MarketDataTickLiveEvent;
export type { LiveStreamEvent, SystemNotificationLiveStreamEvent };

export function useLiveStream() {
  const hub = getSharedLiveSocketHub();

  return {
    addEventListener: hub.addEventListener.bind(hub),
    connect: hub.connect.bind(hub),
    connectionState: hub.connectionState,
    createOwnerId: hub.createOwnerId.bind(hub),
    disconnect: hub.disconnect.bind(hub),
    events: hub.events,
    lastHeartbeat: hub.lastHeartbeat,
    reconnect: hub.reconnect.bind(hub),
    setActiveInstrument: hub.setActiveInstrument.bind(hub),
    setConsoleRefreshEnabled: hub.setConsoleRefreshEnabled.bind(hub),
    setDepthTarget: hub.setDepthTarget.bind(hub),
    setSecurityDetailsTarget: hub.setSecurityDetailsTarget.bind(hub),
    snapshotSubscriptions: hub.snapshotSubscriptions.bind(hub),
  };
}
