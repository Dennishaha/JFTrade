import { ref } from "vue";

const apiBaseUrl = (
  import.meta.env.VITE_API_BASE_URL as string | undefined
)?.replace(/\/$/, "");

function buildEventStreamUrl(path: string): string {
  return apiBaseUrl ? `${apiBaseUrl}${path}` : `http://127.0.0.1:3000${path}`;
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
