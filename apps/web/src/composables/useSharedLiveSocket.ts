import { type InjectionKey, inject, provide } from "vue";

import { useLiveSocket as createLiveSocket } from "./useLiveSocket";

type LiveSocketStore = ReturnType<typeof createLiveSocket>;

const liveSocketKey: InjectionKey<LiveSocketStore> = Symbol(
  "jftrade-live-socket",
);

export function provideLiveSocketStore(): LiveSocketStore {
  const store = createLiveSocket();
  provide(liveSocketKey, store);
  return store;
}

export function useSharedLiveSocket(): LiveSocketStore {
  const store = inject(liveSocketKey);
  if (!store) {
    throw new Error(
      "Live socket not provided. Call provideLiveSocketStore() in AppShell.",
    );
  }
  return store;
}
