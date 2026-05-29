import { ref } from "vue";

import { buildRuntimeApiUrl } from "../runtimeConfig";

function buildEventStreamUrl(path: string): string {
  return buildRuntimeApiUrl(path);
}

export function useLiveStream() {
  const isConnected = ref(false);
  const lastHeartbeat = ref<string | null>(null);
  const events = ref<Array<{ type: string; at: string }>>([]);

  function connect(url = buildEventStreamUrl("/api/v1/stream/live")) {
    const es = new EventSource(url);

    es.onmessage = (e) => {
      const data = JSON.parse(e.data as string) as { type: string; at: string };

      if (data.type === "heartbeat") {
        lastHeartbeat.value = data.at;
      }

      isConnected.value = true;
      events.value.push(data);
    };

    es.onerror = () => {
      isConnected.value = false;
    };

    return es;
  }

  return { isConnected, lastHeartbeat, events, connect };
}
