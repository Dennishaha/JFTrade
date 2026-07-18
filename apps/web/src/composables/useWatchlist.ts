import {
  useInfiniteQuery,
  useMutation,
  useQuery,
} from "@tanstack/vue-query";
import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
} from "vue";

import type {
  WatchlistMembershipUpdate,
  WatchlistQuote,
  WatchlistQuoteError,
} from "@/contracts";

import { queryClient, queryKeys } from "./serverState";
import {
  createWatchlistGroup,
  deleteWatchlistGroup,
  getWatchlistMembership,
  getWatchlistQuotes,
  listWatchlistGroups,
  listWatchlistItems,
  replaceWatchlistMembership,
  updateWatchlistGroup,
  type WatchlistItemsQuery,
} from "./watchlistApi";

export function useDocumentVisible(): Readonly<ReturnType<typeof ref<boolean>>> {
  const visible = ref(
    typeof document === "undefined" || document.visibilityState !== "hidden",
  );
  const sync = () => {
    visible.value = document.visibilityState !== "hidden";
  };
  onMounted(() => {
    if (typeof document !== "undefined") {
      document.addEventListener("visibilitychange", sync);
    }
  });
  onBeforeUnmount(() => {
    if (typeof document !== "undefined") {
      document.removeEventListener("visibilitychange", sync);
    }
  });
  return visible;
}

export function useWatchlistGroups() {
  return useQuery(
    {
      queryKey: queryKeys.watchlistGroups(),
      queryFn: listWatchlistGroups,
      gcTime: 0,
      staleTime: 0,
      refetchOnMount: "always",
    },
    queryClient,
  );
}

export function useWatchlistItems(
  filters: MaybeRefOrGetter<Omit<WatchlistItemsQuery, "cursor">>,
  enabled: MaybeRefOrGetter<boolean> = true,
) {
  const normalizedFilters = computed(() => {
    const value = toValue(filters);
    return {
      groupId: value.groupId || null,
      query: value.query?.trim() ?? "",
      market: value.market?.trim().toUpperCase() ?? "",
      limit: value.limit ?? 200,
    };
  });
  return useInfiniteQuery(
    {
      queryKey: computed(() =>
        queryKeys.watchlistItems(normalizedFilters.value),
      ),
      enabled: computed(() => toValue(enabled)),
      initialPageParam: null as string | null,
      queryFn: ({ pageParam }) =>
        listWatchlistItems({
          ...normalizedFilters.value,
          cursor: pageParam,
        }),
      getNextPageParam: (page) => page.nextCursor || undefined,
      gcTime: 0,
      staleTime: 0,
      refetchOnMount: "always",
    },
    queryClient,
  );
}

export function useWatchlistQuotes(
  instrumentIds: MaybeRefOrGetter<string[]>,
  enabled: MaybeRefOrGetter<boolean> = true,
) {
  const documentVisible = useDocumentVisible();
  const normalizedIds = computed(() =>
    Array.from(
      new Set(
        toValue(instrumentIds)
          .map((value) => value.trim().toUpperCase())
          .filter(Boolean),
      ),
    ).sort(),
  );
  const settledIds = ref<string[]>([...normalizedIds.value]);
  let settleTimer: number | null = null;
  watch(normalizedIds, (nextIds) => {
    if (settleTimer != null && typeof window !== "undefined") {
      window.clearTimeout(settleTimer);
      settleTimer = null;
    }
    // Pause immediately when no row is visible, and perform the first fetch
    // immediately. Subsequent scroll-window changes settle briefly so a fast
    // fling cannot turn every intermediate row boundary into an OpenD call.
    if (
      nextIds.length === 0 ||
      settledIds.value.length === 0 ||
      typeof window === "undefined"
    ) {
      settledIds.value = [...nextIds];
      return;
    }
    settleTimer = window.setTimeout(() => {
      settledIds.value = [...nextIds];
      settleTimer = null;
    }, 120);
  });
  onBeforeUnmount(() => {
    if (settleTimer != null && typeof window !== "undefined") {
      window.clearTimeout(settleTimer);
      settleTimer = null;
    }
  });
  const pollingEnabled = computed(
    () =>
      toValue(enabled) &&
      documentVisible.value &&
      settledIds.value.length > 0,
  );
  const query = useQuery(
    {
      queryKey: computed(() => queryKeys.watchlistQuotes(settledIds.value)),
      queryFn: () => getWatchlistQuotes(settledIds.value),
      enabled: pollingEnabled,
      staleTime: 2_500,
      refetchInterval: () => (pollingEnabled.value ? 3_000 : false),
      retry: false,
    },
    queryClient,
  );
  const quotesByInstrument = ref<Map<string, WatchlistQuote>>(new Map());
  watch(
    [settledIds, () => query.data.value?.quotes],
    ([activeIds, quotes]) => {
      const active = new Set(activeIds);
      const next = new Map(
        [...quotesByInstrument.value].filter(([instrumentId]) =>
          active.has(instrumentId),
        ),
      );
      for (const quote of quotes ?? []) {
        const instrumentId = quote.instrumentId.trim().toUpperCase();
        if (active.has(instrumentId)) {
          next.set(instrumentId, quote);
        }
      }
      quotesByInstrument.value = next;
    },
    { immediate: true },
  );
  const errorsByInstrument = computed<Map<string, WatchlistQuoteError>>(
    () =>
      new Map(
        (query.data.value?.errors ?? []).map((error) => [
          error.instrumentId.toUpperCase(),
          error,
        ]),
      ),
  );
  return { ...query, quotesByInstrument, errorsByInstrument };
}

export function useWatchlistMembership(
  market: MaybeRefOrGetter<string>,
  symbol: MaybeRefOrGetter<string>,
) {
  const normalizedMarket = computed(() => toValue(market).trim().toUpperCase());
  const normalizedSymbol = computed(() => toValue(symbol).trim().toUpperCase());
  const queryKey = computed(() =>
    queryKeys.watchlistMembership(
      normalizedMarket.value,
      normalizedSymbol.value,
    ),
  );
  const query = useQuery(
    {
      queryKey,
      enabled: computed(
        () => normalizedMarket.value !== "" && normalizedSymbol.value !== "",
      ),
      queryFn: () =>
        getWatchlistMembership(normalizedMarket.value, normalizedSymbol.value),
      gcTime: 0,
      staleTime: 0,
      refetchOnMount: "always",
      retry: false,
    },
    queryClient,
  );
  const replaceMutation = useMutation(
    {
      mutationFn: (input: WatchlistMembershipUpdate) =>
        replaceWatchlistMembership(
          normalizedMarket.value,
          normalizedSymbol.value,
          input,
        ),
      onSuccess: async (membership) => {
        queryClient.setQueryData(queryKey.value, membership);
        await Promise.all([
          queryClient.invalidateQueries({
            queryKey: queryKeys.watchlistGroups(),
          }),
          queryClient.invalidateQueries({
            queryKey: queryKeys.watchlistItemsRoot(),
          }),
        ]);
      },
    },
    queryClient,
  );
  return { query, replaceMutation };
}

export function useCreateWatchlistGroup() {
  return useMutation(
    {
      mutationFn: (name: string) => createWatchlistGroup(name),
      onSuccess: async () => {
        await queryClient.invalidateQueries({
          queryKey: queryKeys.watchlistGroups(),
        });
      },
    },
    queryClient,
  );
}

export function useUpdateWatchlistGroup() {
  return useMutation(
    {
      mutationFn: (input: {
        groupId: string;
        name: string;
        expectedRevision: number;
      }) =>
        updateWatchlistGroup(input.groupId, {
          name: input.name,
          expectedRevision: input.expectedRevision,
        }),
      onSuccess: async () => {
        await Promise.all([
          queryClient.invalidateQueries({
            queryKey: queryKeys.watchlistGroups(),
          }),
          queryClient.invalidateQueries({
            queryKey: queryKeys.watchlistItemsRoot(),
          }),
          queryClient.invalidateQueries({
            queryKey: queryKeys.watchlistMembershipsRoot(),
          }),
        ]);
      },
    },
    queryClient,
  );
}

export function useDeleteWatchlistGroup() {
  return useMutation(
    {
      mutationFn: (groupId: string) => deleteWatchlistGroup(groupId),
      onSuccess: async () => {
        await Promise.all([
          queryClient.invalidateQueries({
            queryKey: queryKeys.watchlistGroups(),
          }),
          queryClient.invalidateQueries({
            queryKey: queryKeys.watchlistItemsRoot(),
          }),
          queryClient.invalidateQueries({
            queryKey: queryKeys.watchlistMembershipsRoot(),
          }),
          queryClient.invalidateQueries({
            queryKey: queryKeys.watchlistBindings(),
          }),
        ]);
      },
    },
    queryClient,
  );
}
