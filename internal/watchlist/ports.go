package watchlist

import "context"

type Repository interface {
	ListGroups(context.Context) ([]Group, error)
	GetGroup(context.Context, string) (Group, error)
	CreateGroup(context.Context, string) (Group, error)
	UpdateGroup(context.Context, string, string, int64) (Group, error)
	DeleteGroup(context.Context, string) error
	ListItems(context.Context, ListItemsOptions) (ItemPage, error)
	GetMemberships(context.Context, string) (Memberships, error)
	ReplaceMemberships(context.Context, ReplaceMembershipsInput) (Memberships, error)
	UpsertSource(context.Context, Source) error
	ListSources(context.Context) ([]Source, error)
	ReplaceRemoteGroups(context.Context, string, []RemoteGroup) error
	ListRemoteGroups(context.Context, string) ([]RemoteGroup, error)
	ListBindings(context.Context, string) ([]Binding, error)
	DeleteBinding(context.Context, string) error
	GroupInstrumentIDs(context.Context, string) ([]string, error)
	SaveImportPreview(context.Context, ImportPreview) error
	GetImportPreview(context.Context, string) (ImportPreview, error)
	CommitImport(context.Context, CommitImportStoreInput) (ImportRun, error)
	ListImportRuns(context.Context, string, string, int) (ImportRunPage, error)
}

type WatchlistSourceReader interface {
	Source(context.Context) (Source, error)
	ListGroups(context.Context) ([]RemoteGroup, error)
	ListGroupMembers(context.Context, string) ([]RemoteMember, error)
}

// FreshWatchlistSourceReader bypasses connector caches when an import commit
// revalidates the remote hash captured by its preview.
type FreshWatchlistSourceReader interface {
	ListGroupMembersFresh(context.Context, string) ([]RemoteMember, error)
}

// InstrumentMetadataWriter accepts best-effort display metadata enrichment
// for instruments that already exist in the local watchlist repository.
type InstrumentMetadataWriter interface {
	UpdateInstrumentMetadata(context.Context, []InstrumentMetadata) error
}

type BatchSnapshotSource interface {
	BatchSnapshots(context.Context, []string) ([]Quote, []QuoteError, error)
}
