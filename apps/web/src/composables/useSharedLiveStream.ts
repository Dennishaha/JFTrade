import { type InjectionKey, inject, provide } from "vue";

import { useLiveStream as createLiveStream } from "./useLiveStream";

type LiveStreamStore = ReturnType<typeof createLiveStream>;

const liveStreamKey: InjectionKey<LiveStreamStore> = Symbol(
  "jftrade-live-stream",
);

export function provideLiveStreamStore(): LiveStreamStore {
  const store = createLiveStream();
  provide(liveStreamKey, store);
  return store;
}

export function useSharedLiveStream(): LiveStreamStore {
  const store = inject(liveStreamKey);
  if (!store) {
    throw new Error(
      "Live stream not provided. Call provideLiveStreamStore() in AppShell.",
    );
  }
  return store;
}
