import { ref, type Ref } from "vue";

export type EventSourceConnectionState =
  | "idle"
  | "connecting"
  | "connected"
  | "disconnected"
  | "error"
  | "unsupported";

interface CreateEventSourceStreamOptions<T> {
  parseEvent?: (payload: unknown) => T;
  onMessage?: (event: T) => void;
  onOpen?: () => void;
  onDisconnect?: () => void;
  onError?: (hasConnected: boolean) => void;
}

export interface EventSourceStreamController<T> {
  activeUrl: Ref<string | null>;
  connect: (url: string) => EventSource | null;
  connectionState: Ref<EventSourceConnectionState>;
  disconnect: (markDisconnected?: boolean) => void;
  reconnect: () => EventSource | null;
}

export function createEventSourceStream<T = unknown>(
  options: CreateEventSourceStreamOptions<T> = {},
): EventSourceStreamController<T> {
  const connectionState = ref<EventSourceConnectionState>("idle");
  const activeUrl = ref<string | null>(null);
  let stream: EventSource | null = null;
  let currentAttemptConnected = false;

  function closeActiveStream(markDisconnected: boolean): void {
    const activeStream = stream;
    stream = null;
    activeStream?.close();

    if (markDisconnected && connectionState.value !== "unsupported") {
      connectionState.value = "disconnected";
      options.onDisconnect?.();
    }
  }

  function disconnect(markDisconnected = true): void {
    if (!markDisconnected) {
      activeUrl.value = null;
    }
    currentAttemptConnected = false;
    closeActiveStream(markDisconnected);
  }

  function connect(url: string): EventSource | null {
    if (typeof EventSource === "undefined") {
      connectionState.value = "unsupported";
      return null;
    }

    activeUrl.value = url;
    currentAttemptConnected = false;
    closeActiveStream(false);
    connectionState.value = "connecting";

    const nextStream = new EventSource(url);
    stream = nextStream;

    nextStream.onopen = () => {
      if (stream !== nextStream) {
        return;
      }

      connectionState.value = "connected";
      currentAttemptConnected = true;
      options.onOpen?.();
    };

    nextStream.onmessage = (event) => {
      if (stream !== nextStream) {
        return;
      }

      try {
        const rawPayload = JSON.parse(event.data as string) as unknown;
        const payload = options.parseEvent
          ? options.parseEvent(rawPayload)
          : (rawPayload as T);
        options.onMessage?.(payload);
        connectionState.value = "connected";
        currentAttemptConnected = true;
      } catch {
        connectionState.value = "error";
      }
    };

    nextStream.onerror = () => {
      if (stream !== nextStream) {
        return;
      }

      connectionState.value = currentAttemptConnected
        ? "error"
        : "disconnected";
      options.onError?.(currentAttemptConnected);
    };

    return nextStream;
  }

  function reconnect(): EventSource | null {
    if (activeUrl.value == null) {
      return null;
    }
    return connect(activeUrl.value);
  }

  return {
    activeUrl,
    connect,
    connectionState,
    disconnect,
    reconnect,
  };
}
