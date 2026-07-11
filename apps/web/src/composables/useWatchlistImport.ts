import { useMutation, useQuery } from "@tanstack/vue-query";
import { computed, toValue, type MaybeRefOrGetter } from "vue";

import type {
  WatchlistImportCommitRequest,
  WatchlistImportPreviewRequest,
} from "@/contracts";

import { queryClient, queryKeys } from "./serverState";
import {
  commitWatchlistImport,
  deleteWatchlistBinding,
  listWatchlistBindings,
  listWatchlistSourceGroups,
  listWatchlistSources,
  previewWatchlistImport,
} from "./watchlistApi";

export function useWatchlistImport(
  open: MaybeRefOrGetter<boolean>,
  sourceId: MaybeRefOrGetter<string>,
) {
  const normalizedSourceId = computed(() => toValue(sourceId).trim());
  const sourcesQuery = useQuery(
    {
      queryKey: queryKeys.watchlistSources(),
      queryFn: listWatchlistSources,
      enabled: computed(() => toValue(open)),
      staleTime: 15_000,
      retry: false,
    },
    queryClient,
  );
  const sourceGroupsQuery = useQuery(
    {
      queryKey: computed(() =>
        queryKeys.watchlistSourceGroups(normalizedSourceId.value),
      ),
      queryFn: () => listWatchlistSourceGroups(normalizedSourceId.value),
      enabled: computed(
        () => toValue(open) && normalizedSourceId.value !== "",
      ),
      staleTime: 5_000,
      retry: false,
    },
    queryClient,
  );
  const bindingsQuery = useQuery(
    {
      queryKey: queryKeys.watchlistBindings(),
      queryFn: listWatchlistBindings,
      enabled: computed(() => toValue(open)),
      gcTime: 0,
      staleTime: 0,
      refetchOnMount: "always",
      retry: false,
    },
    queryClient,
  );
  const previewMutation = useMutation(
    {
      mutationFn: (input: WatchlistImportPreviewRequest) =>
        previewWatchlistImport(input),
    },
    queryClient,
  );
  const commitMutation = useMutation(
    {
      mutationFn: (input: {
        previewId: string;
        body: WatchlistImportCommitRequest;
      }) => commitWatchlistImport(input.previewId, input.body),
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
          queryClient.invalidateQueries({
            queryKey: queryKeys.watchlistImportRuns(),
          }),
        ]);
      },
    },
    queryClient,
  );
  const unlinkMutation = useMutation(
    {
      mutationFn: (bindingId: string) => deleteWatchlistBinding(bindingId),
      onSuccess: async () => {
        await Promise.all([
          queryClient.invalidateQueries({
            queryKey: queryKeys.watchlistBindings(),
          }),
          queryClient.invalidateQueries({
            queryKey: queryKeys.watchlistItemsRoot(),
          }),
        ]);
      },
    },
    queryClient,
  );

  return {
    sourcesQuery,
    sourceGroupsQuery,
    bindingsQuery,
    previewMutation,
    commitMutation,
    unlinkMutation,
  };
}
