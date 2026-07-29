export type LiveEventSource =
  | "market-data"
  | "execution"
  | "notification"
  | "backtest"
  | "adk"
  | "system";

export interface LiveEventEnvelope<TPayload = unknown> {
  eventId: string;
  type: string;
  source: LiveEventSource;
  entityId: string;
  version?: number;
  serverTime: string;
  payload: TPayload;
}

type LiveEventListener = (event: LiveEventEnvelope) => void;

const MAX_SEEN_EVENT_IDS = 500;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function numberValue(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function sourceValue(value: unknown): LiveEventSource | null {
  switch (value) {
    case "market-data":
    case "execution":
    case "notification":
    case "backtest":
    case "adk":
    case "system":
      return value;
    default:
      return null;
  }
}

export function parseLiveEventEnvelope(raw: unknown): LiveEventEnvelope | null {
  if (!isRecord(raw)) {
    return null;
  }

  const type = stringValue(raw.type);
  if (type === "") {
    return null;
  }

  const explicitSource = sourceValue(raw.source);
  const explicitEventId = stringValue(raw.eventId);
  const explicitEntityId = stringValue(raw.entityId);
  const explicitServerTime = stringValue(raw.serverTime);

  if (explicitEventId === "" || explicitSource == null || explicitEntityId === "" || explicitServerTime === "") {
    return null;
  }
  if (!("payload" in raw)) {
    return null;
  }

  const version = numberValue(raw.version);
  return {
    eventId: explicitEventId,
    type,
    source: explicitSource,
    entityId: explicitEntityId,
    serverTime: explicitServerTime,
    payload: raw.payload,
    ...(version !== undefined ? { version } : {}),
  };
}

export class LiveEventBus {
  private readonly listeners = new Set<LiveEventListener>();
  private readonly seenEventIds: string[] = [];
  private readonly seenEventIdSet = new Set<string>();
  private readonly latestVersionByEntity = new Map<string, number>();

  subscribe(listener: LiveEventListener): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  publishRaw(raw: unknown): LiveEventEnvelope | null {
    const event = parseLiveEventEnvelope(raw);
    if (event == null) {
      return null;
    }
    return this.publish(event) ? event : null;
  }

  publish(event: LiveEventEnvelope): boolean {
    if (this.seenEventIdSet.has(event.eventId)) {
      return false;
    }

    const versionKey = `${event.source}|${event.type}|${event.entityId}`;
    if (event.version !== undefined) {
      const latest = this.latestVersionByEntity.get(versionKey);
      if (latest !== undefined && event.version <= latest) {
        this.rememberEventId(event.eventId);
        return false;
      }
      this.latestVersionByEntity.set(versionKey, event.version);
    }

    this.rememberEventId(event.eventId);
    for (const listener of this.listeners) {
      listener(event);
    }
    return true;
  }

  reset(): void {
    this.listeners.clear();
    this.seenEventIds.length = 0;
    this.seenEventIdSet.clear();
    this.latestVersionByEntity.clear();
  }

  private rememberEventId(eventId: string): void {
    this.seenEventIds.push(eventId);
    this.seenEventIdSet.add(eventId);
    while (this.seenEventIds.length > MAX_SEEN_EVENT_IDS) {
      const expired = this.seenEventIds.shift();
      if (expired !== undefined) {
        this.seenEventIdSet.delete(expired);
      }
    }
  }
}

let liveEventBus: LiveEventBus | null = null;

export function getLiveEventBus(): LiveEventBus {
  liveEventBus ??= new LiveEventBus();
  return liveEventBus;
}

export function resetLiveEventBusForTests(): void {
  liveEventBus?.reset();
}
