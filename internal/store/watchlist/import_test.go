package watchlist_test

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

type fakeSourceReader struct {
	source   domain.Source
	groups   []domain.RemoteGroup
	members  map[string][]domain.RemoteMember
	freshErr error
}

type nonFreshSourceReader struct{ base *fakeSourceReader }

func (r *nonFreshSourceReader) Source(ctx context.Context) (domain.Source, error) {
	return r.base.Source(ctx)
}

func (r *nonFreshSourceReader) ListGroups(ctx context.Context) ([]domain.RemoteGroup, error) {
	return r.base.ListGroups(ctx)
}

func (r *nonFreshSourceReader) ListGroupMembers(ctx context.Context, groupID string) ([]domain.RemoteMember, error) {
	return r.base.ListGroupMembers(ctx, groupID)
}

func (f *fakeSourceReader) Source(_ context.Context) (domain.Source, error) { return f.source, nil }
func (f *fakeSourceReader) ListGroups(_ context.Context) ([]domain.RemoteGroup, error) {
	return append([]domain.RemoteGroup(nil), f.groups...), nil
}
func (f *fakeSourceReader) ListGroupMembers(_ context.Context, groupID string) ([]domain.RemoteMember, error) {
	return append([]domain.RemoteMember(nil), f.members[groupID]...), nil
}
func (f *fakeSourceReader) ListGroupMembersFresh(_ context.Context, groupID string) ([]domain.RemoteMember, error) {
	if f.freshErr != nil {
		return nil, f.freshErr
	}
	return append([]domain.RemoteMember(nil), f.members[groupID]...), nil
}

func TestImportPreviewCommitRepeatAndStaleGuards(t *testing.T) {
	repository := openStore(t)
	reader := &fakeSourceReader{
		source: domain.Source{ID: "futu:default", Broker: "futu", DisplayName: "Futu OpenD"},
		groups: []domain.RemoteGroup{{RemoteGroupID: "custom:核心", Name: "核心", Type: "custom"}},
		members: map[string][]domain.RemoteMember{"custom:核心": {
			{InstrumentID: "US.AAPL", Name: "Apple", BrokerCode: "AAPL"},
			{InstrumentID: "US.MSFT", Name: "Microsoft", BrokerCode: "MSFT"},
		}},
	}
	service := domain.NewService(repository, domain.WithSourceReader("futu:default", reader))
	group, err := service.CreateGroup(t.Context(), domain.CreateGroupInput{Name: "本地核心"})
	if err != nil {
		t.Fatal(err)
	}
	for _, instrumentID := range []string{"US.AAPL", "US.TSLA"} {
		if _, err := service.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{InstrumentID: instrumentID, GroupIDs: []string{group.ID}, ExpectedRevision: 0}); err != nil {
			t.Fatal(err)
		}
	}

	preview, err := service.PreviewImport(t.Context(), domain.ImportPreviewRequest{SourceID: "futu:default", RemoteGroupID: "custom:核心", LocalGroupID: group.ID})
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if len(preview.Added) != 1 || preview.Added[0].InstrumentID != "US.MSFT" || len(preview.Unchanged) != 1 || len(preview.LocalOnly) != 1 || preview.LocalOnly[0].Selected {
		t.Fatalf("preview diff = %#v", preview)
	}
	run, err := service.CommitImport(t.Context(), domain.CommitImportInput{PreviewID: preview.ID, DeleteInstrumentIDs: []string{"US.TSLA"}})
	if err != nil {
		t.Fatalf("CommitImport: %v", err)
	}
	if run.AddedCount != 1 || run.RemovedCount != 1 || run.UnchangedCount != 1 {
		t.Fatalf("run = %#v", run)
	}
	ids, err := repository.GroupInstrumentIDs(t.Context(), group.ID)
	if err != nil || !equalStrings(ids, []string{"US.AAPL", "US.MSFT"}) {
		t.Fatalf("group ids = %#v, %v", ids, err)
	}
	bindings, err := service.ListBindings(t.Context(), "futu:default")
	if err != nil || len(bindings) != 1 || bindings[0].LocalGroupID != group.ID {
		t.Fatalf("bindings = %#v, %v", bindings, err)
	}

	repeat, err := service.PreviewImport(t.Context(), domain.ImportPreviewRequest{SourceID: "futu:default", RemoteGroupID: "custom:核心", LocalGroupID: group.ID})
	if err != nil || len(repeat.Added) != 0 || len(repeat.Unchanged) != 2 || len(repeat.LocalOnly) != 0 {
		t.Fatalf("repeat preview = %#v, %v", repeat, err)
	}
	repeatRun, err := service.CommitImport(t.Context(), domain.CommitImportInput{PreviewID: repeat.ID})
	if err != nil || repeatRun.AddedCount != 0 || repeatRun.UnchangedCount != 2 {
		t.Fatalf("repeat run = %#v, %v", repeatRun, err)
	}

	staleLocal, err := service.PreviewImport(t.Context(), domain.ImportPreviewRequest{SourceID: "futu:default", RemoteGroupID: "custom:核心", LocalGroupID: group.ID})
	if err != nil {
		t.Fatal(err)
	}
	membership, err := service.GetMemberships(t.Context(), "US.NVDA")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReplaceMemberships(t.Context(), domain.ReplaceMembershipsInput{InstrumentID: "US.NVDA", GroupIDs: []string{group.ID}, ExpectedRevision: membership.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitImport(t.Context(), domain.CommitImportInput{PreviewID: staleLocal.ID}); !errors.Is(err, domain.ErrStalePreview) {
		t.Fatalf("local stale commit = %v", err)
	}

	remoteStale, err := service.PreviewImport(t.Context(), domain.ImportPreviewRequest{SourceID: "futu:default", RemoteGroupID: "custom:核心", LocalGroupID: group.ID})
	if err != nil {
		t.Fatal(err)
	}
	reader.members["custom:核心"] = append(reader.members["custom:核心"], domain.RemoteMember{InstrumentID: "US.AMD"})
	if _, err := service.CommitImport(t.Context(), domain.CommitImportInput{PreviewID: remoteStale.ID}); !errors.Is(err, domain.ErrStalePreview) {
		t.Fatalf("remote stale commit = %v", err)
	}

	reader.members["custom:核心"] = reader.members["custom:核心"][:2]
	freshFailure, err := service.PreviewImport(t.Context(), domain.ImportPreviewRequest{SourceID: "futu:default", RemoteGroupID: "custom:核心", LocalGroupID: group.ID})
	if err != nil {
		t.Fatal(err)
	}
	reader.freshErr = errors.New("OpenD timeout")
	if _, err := service.CommitImport(t.Context(), domain.CommitImportInput{PreviewID: freshFailure.ID}); !errors.Is(err, domain.ErrStalePreview) {
		t.Fatalf("fresh read failure commit = %v", err)
	}
}

func TestImportCommitRejectsConnectorWithoutFreshRevalidation(t *testing.T) {
	repository := openStore(t)
	base := &fakeSourceReader{
		source:  domain.Source{ID: "legacy:default", Broker: "legacy", DisplayName: "Legacy"},
		groups:  []domain.RemoteGroup{{RemoteGroupID: "group-1", Name: "Legacy"}},
		members: map[string][]domain.RemoteMember{"group-1": {{InstrumentID: "US.AAPL"}}},
	}
	reader := &nonFreshSourceReader{base: base}
	service := domain.NewService(repository, domain.WithSourceReader("legacy:default", reader))
	group, err := service.CreateGroup(t.Context(), domain.CreateGroupInput{Name: "Legacy import"})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewImport(t.Context(), domain.ImportPreviewRequest{
		SourceID: "legacy:default", RemoteGroupID: "group-1", LocalGroupID: group.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitImport(t.Context(), domain.CommitImportInput{PreviewID: preview.ID}); !errors.Is(err, domain.ErrStalePreview) {
		t.Fatalf("commit without fresh reader = %v", err)
	}
}

func TestImportSupportsMultipleSourcesExpiryAndLocalOnlyUnbind(t *testing.T) {
	repository := openStore(t)
	now := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	first := &fakeSourceReader{
		source:  domain.Source{ID: "futu:default", Broker: "futu", DisplayName: "Futu"},
		groups:  []domain.RemoteGroup{{RemoteGroupID: "favorites", Name: "Favorites"}},
		members: map[string][]domain.RemoteMember{"favorites": {{InstrumentID: "US.AAPL", Name: "Apple"}}},
	}
	second := &fakeSourceReader{
		source:  domain.Source{ID: "broker-b:default", Broker: "broker-b", DisplayName: "Broker B"},
		groups:  []domain.RemoteGroup{{RemoteGroupID: "second", Name: "Second"}},
		members: map[string][]domain.RemoteMember{"second": {{InstrumentID: "HK.00700"}}},
	}
	service := domain.NewService(
		repository,
		domain.WithClock(func() time.Time { return now }),
		domain.WithPreviewTTL(time.Minute),
		domain.WithSourceReader("futu:default", first),
		domain.WithSourceReader("broker-b:default", second),
	)
	sources, err := service.ListSources(t.Context())
	if err != nil || len(sources) != 2 || sources[0].ID == sources[1].ID {
		t.Fatalf("sources = %#v err=%v", sources, err)
	}
	group, err := service.CreateGroup(t.Context(), domain.CreateGroupInput{Name: "Imported"})
	if err != nil {
		t.Fatal(err)
	}
	expired, err := service.PreviewImport(t.Context(), domain.ImportPreviewRequest{
		SourceID: "broker-b:default", RemoteGroupID: "second", LocalGroupID: group.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := service.CommitImport(t.Context(), domain.CommitImportInput{PreviewID: expired.ID}); !errors.Is(err, domain.ErrPreviewExpired) {
		t.Fatalf("expired commit = %v", err)
	}

	preview, err := service.PreviewImport(t.Context(), domain.ImportPreviewRequest{
		SourceID: "futu:default", RemoteGroupID: "favorites", LocalGroupID: group.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitImport(t.Context(), domain.CommitImportInput{PreviewID: preview.ID}); err != nil {
		t.Fatal(err)
	}
	bindings, err := service.ListBindings(t.Context(), "futu:default")
	if err != nil || len(bindings) != 1 {
		t.Fatalf("bindings = %#v err=%v", bindings, err)
	}
	if err := service.DeleteBinding(t.Context(), bindings[0].ID); err != nil {
		t.Fatal(err)
	}
	ids, err := repository.GroupInstrumentIDs(t.Context(), group.ID)
	if err != nil || !equalStrings(ids, []string{"US.AAPL"}) {
		t.Fatalf("unbind changed local membership: ids=%#v err=%v", ids, err)
	}
	page, err := service.ListItems(t.Context(), domain.ListItemsOptions{GroupID: group.ID, Limit: 10})
	if err != nil || len(page.Items) != 1 || len(page.Items[0].SourceIDs) != 0 {
		t.Fatalf("unbind origins = %#v err=%v", page, err)
	}
}

func TestDuplicateRemoteGroupNamesAreAmbiguous(t *testing.T) {
	repository := openStore(t)
	reader := &fakeSourceReader{groups: []domain.RemoteGroup{
		{RemoteGroupID: "1", Name: "核心", Type: "custom"}, {RemoteGroupID: "2", Name: " 核心 ", Type: "system"},
	}, members: map[string][]domain.RemoteMember{}}
	service := domain.NewService(repository, domain.WithSourceReader("futu:default", reader))
	groups, err := service.ListSourceGroups(t.Context(), "futu:default")
	if err != nil || len(groups) != 2 || !groups[0].Ambiguous || !groups[1].Ambiguous {
		t.Fatalf("groups = %#v, %v", groups, err)
	}
	_, err = service.PreviewImport(t.Context(), domain.ImportPreviewRequest{SourceID: "futu:default", RemoteGroupID: "1", NewGroupName: "本地"})
	if !errors.Is(err, domain.ErrAmbiguousRemoteGroup) {
		t.Fatalf("PreviewImport ambiguous = %v", err)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
