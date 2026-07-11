package watchlist

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

type serviceTestRepository struct {
	listGroups          func(context.Context) ([]Group, error)
	getGroup            func(context.Context, string) (Group, error)
	createGroup         func(context.Context, string) (Group, error)
	updateGroup         func(context.Context, string, string, int64) (Group, error)
	deleteGroup         func(context.Context, string) error
	listItems           func(context.Context, ListItemsOptions) (ItemPage, error)
	getMemberships      func(context.Context, string) (Memberships, error)
	replaceMemberships  func(context.Context, ReplaceMembershipsInput) (Memberships, error)
	upsertSource        func(context.Context, Source) error
	listSources         func(context.Context) ([]Source, error)
	replaceRemoteGroups func(context.Context, string, []RemoteGroup) error
	listRemoteGroups    func(context.Context, string) ([]RemoteGroup, error)
	listBindings        func(context.Context, string) ([]Binding, error)
	deleteBinding       func(context.Context, string) error
	groupInstrumentIDs  func(context.Context, string) ([]string, error)
	saveImportPreview   func(context.Context, ImportPreview) error
	getImportPreview    func(context.Context, string) (ImportPreview, error)
	commitImport        func(context.Context, CommitImportStoreInput) (ImportRun, error)
	listImportRuns      func(context.Context, string, string, int) (ImportRunPage, error)
	updateMetadata      func(context.Context, []InstrumentMetadata) error
}

func (r *serviceTestRepository) ListGroups(ctx context.Context) ([]Group, error) {
	if r.listGroups == nil {
		return nil, nil
	}
	return r.listGroups(ctx)
}

func (r *serviceTestRepository) GetGroup(ctx context.Context, id string) (Group, error) {
	if r.getGroup == nil {
		return Group{}, nil
	}
	return r.getGroup(ctx, id)
}

func (r *serviceTestRepository) CreateGroup(ctx context.Context, name string) (Group, error) {
	if r.createGroup == nil {
		return Group{}, nil
	}
	return r.createGroup(ctx, name)
}

func (r *serviceTestRepository) UpdateGroup(ctx context.Context, id, name string, revision int64) (Group, error) {
	if r.updateGroup == nil {
		return Group{}, nil
	}
	return r.updateGroup(ctx, id, name, revision)
}

func (r *serviceTestRepository) DeleteGroup(ctx context.Context, id string) error {
	if r.deleteGroup == nil {
		return nil
	}
	return r.deleteGroup(ctx, id)
}

func (r *serviceTestRepository) ListItems(ctx context.Context, options ListItemsOptions) (ItemPage, error) {
	if r.listItems == nil {
		return ItemPage{}, nil
	}
	return r.listItems(ctx, options)
}

func (r *serviceTestRepository) GetMemberships(ctx context.Context, instrumentID string) (Memberships, error) {
	if r.getMemberships == nil {
		return Memberships{}, nil
	}
	return r.getMemberships(ctx, instrumentID)
}

func (r *serviceTestRepository) ReplaceMemberships(ctx context.Context, input ReplaceMembershipsInput) (Memberships, error) {
	if r.replaceMemberships == nil {
		return Memberships{}, nil
	}
	return r.replaceMemberships(ctx, input)
}

func (r *serviceTestRepository) UpsertSource(ctx context.Context, source Source) error {
	if r.upsertSource == nil {
		return nil
	}
	return r.upsertSource(ctx, source)
}

func (r *serviceTestRepository) ListSources(ctx context.Context) ([]Source, error) {
	if r.listSources == nil {
		return nil, nil
	}
	return r.listSources(ctx)
}

func (r *serviceTestRepository) ReplaceRemoteGroups(ctx context.Context, sourceID string, groups []RemoteGroup) error {
	if r.replaceRemoteGroups == nil {
		return nil
	}
	return r.replaceRemoteGroups(ctx, sourceID, groups)
}

func (r *serviceTestRepository) ListRemoteGroups(ctx context.Context, sourceID string) ([]RemoteGroup, error) {
	if r.listRemoteGroups == nil {
		return nil, nil
	}
	return r.listRemoteGroups(ctx, sourceID)
}

func (r *serviceTestRepository) ListBindings(ctx context.Context, sourceID string) ([]Binding, error) {
	if r.listBindings == nil {
		return nil, nil
	}
	return r.listBindings(ctx, sourceID)
}

func (r *serviceTestRepository) DeleteBinding(ctx context.Context, id string) error {
	if r.deleteBinding == nil {
		return nil
	}
	return r.deleteBinding(ctx, id)
}

func (r *serviceTestRepository) GroupInstrumentIDs(ctx context.Context, groupID string) ([]string, error) {
	if r.groupInstrumentIDs == nil {
		return nil, nil
	}
	return r.groupInstrumentIDs(ctx, groupID)
}

func (r *serviceTestRepository) SaveImportPreview(ctx context.Context, preview ImportPreview) error {
	if r.saveImportPreview == nil {
		return nil
	}
	return r.saveImportPreview(ctx, preview)
}

func (r *serviceTestRepository) GetImportPreview(ctx context.Context, id string) (ImportPreview, error) {
	if r.getImportPreview == nil {
		return ImportPreview{}, nil
	}
	return r.getImportPreview(ctx, id)
}

func (r *serviceTestRepository) CommitImport(ctx context.Context, input CommitImportStoreInput) (ImportRun, error) {
	if r.commitImport == nil {
		return ImportRun{}, nil
	}
	return r.commitImport(ctx, input)
}

func (r *serviceTestRepository) ListImportRuns(ctx context.Context, sourceID, cursor string, limit int) (ImportRunPage, error) {
	if r.listImportRuns == nil {
		return ImportRunPage{}, nil
	}
	return r.listImportRuns(ctx, sourceID, cursor, limit)
}

func (r *serviceTestRepository) UpdateInstrumentMetadata(ctx context.Context, metadata []InstrumentMetadata) error {
	if r.updateMetadata == nil {
		return nil
	}
	return r.updateMetadata(ctx, metadata)
}

type serviceTestReader struct {
	source        Source
	sourceErr     error
	groups        []RemoteGroup
	groupsErr     error
	members       []RemoteMember
	membersErr    error
	freshMembers  []RemoteMember
	freshErr      error
	requestedIDs  []string
	freshGroupIDs []string
}

func (r *serviceTestReader) Source(context.Context) (Source, error) {
	return r.source, r.sourceErr
}

func (r *serviceTestReader) ListGroups(context.Context) ([]RemoteGroup, error) {
	return slices.Clone(r.groups), r.groupsErr
}

func (r *serviceTestReader) ListGroupMembers(_ context.Context, groupID string) ([]RemoteMember, error) {
	r.requestedIDs = append(r.requestedIDs, groupID)
	return slices.Clone(r.members), r.membersErr
}

func (r *serviceTestReader) ListGroupMembersFresh(_ context.Context, groupID string) ([]RemoteMember, error) {
	r.freshGroupIDs = append(r.freshGroupIDs, groupID)
	return slices.Clone(r.freshMembers), r.freshErr
}

type serviceTestNonFreshReader struct{ reader *serviceTestReader }

func (r serviceTestNonFreshReader) Source(ctx context.Context) (Source, error) {
	return r.reader.Source(ctx)
}

func (r serviceTestNonFreshReader) ListGroups(ctx context.Context) ([]RemoteGroup, error) {
	return r.reader.ListGroups(ctx)
}

func (r serviceTestNonFreshReader) ListGroupMembers(ctx context.Context, groupID string) ([]RemoteMember, error) {
	return r.reader.ListGroupMembers(ctx, groupID)
}

func TestServiceCRUDNormalizesAndDelegates(t *testing.T) {
	ctx := t.Context()
	var (
		createdName       string
		updatedID         string
		updatedName       string
		updatedRevision   int64
		deletedID         string
		listedOptions     []ListItemsOptions
		membershipID      string
		replacement       ReplaceMembershipsInput
		listedBindingID   string
		deletedBindingID  string
		listedRunSourceID string
		listedRunCursor   string
		listedRunLimit    int
	)
	repository := &serviceTestRepository{
		listGroups: func(context.Context) ([]Group, error) {
			return []Group{{ID: "group-1", Name: "Core"}}, nil
		},
		createGroup: func(_ context.Context, name string) (Group, error) {
			createdName = name
			return Group{ID: "group-2", Name: name}, nil
		},
		updateGroup: func(_ context.Context, id, name string, revision int64) (Group, error) {
			updatedID, updatedName, updatedRevision = id, name, revision
			return Group{ID: id, Name: name, Revision: revision + 1}, nil
		},
		deleteGroup: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
		listItems: func(_ context.Context, options ListItemsOptions) (ItemPage, error) {
			listedOptions = append(listedOptions, options)
			return ItemPage{Items: []Item{{Instrument: Instrument{ID: "US.AAPL"}}}}, nil
		},
		getMemberships: func(_ context.Context, instrumentID string) (Memberships, error) {
			membershipID = instrumentID
			return Memberships{InstrumentID: instrumentID}, nil
		},
		replaceMemberships: func(_ context.Context, input ReplaceMembershipsInput) (Memberships, error) {
			replacement = input
			return Memberships{InstrumentID: input.InstrumentID, Revision: 4}, nil
		},
		listBindings: func(_ context.Context, sourceID string) ([]Binding, error) {
			listedBindingID = sourceID
			return []Binding{{ID: "binding-1"}}, nil
		},
		deleteBinding: func(_ context.Context, id string) error {
			deletedBindingID = id
			return nil
		},
		listImportRuns: func(_ context.Context, sourceID, cursor string, limit int) (ImportRunPage, error) {
			listedRunSourceID, listedRunCursor, listedRunLimit = sourceID, cursor, limit
			return ImportRunPage{Items: []ImportRun{{ID: "run-1"}}}, nil
		},
	}
	service := NewService(repository, nil, WithClock(nil), WithPreviewTTL(0), WithQuoteCacheTTL(0))
	if !service.Available() || (*Service)(nil).Available() {
		t.Fatal("service availability did not reflect repository presence")
	}
	groups, err := service.ListGroups(ctx)
	if err != nil || len(groups) != 1 || groups[0].ID != "group-1" {
		t.Fatalf("ListGroups() = %#v, %v", groups, err)
	}
	created, err := service.CreateGroup(ctx, CreateGroupInput{Name: "  Growth  "})
	if err != nil || createdName != "Growth" || created.Name != "Growth" {
		t.Fatalf("CreateGroup() = %#v, %v; delegated name = %q", created, err, createdName)
	}
	updated, err := service.UpdateGroup(ctx, " group-2 ", UpdateGroupInput{Name: " Value ", ExpectedRevision: 3})
	if err != nil || updatedID != "group-2" || updatedName != "Value" || updatedRevision != 3 || updated.Revision != 4 {
		t.Fatalf("UpdateGroup() = %#v, %v; delegated = %q/%q/%d", updated, err, updatedID, updatedName, updatedRevision)
	}
	if err := service.DeleteGroup(ctx, " group-2 "); err != nil || deletedID != "group-2" {
		t.Fatalf("DeleteGroup() = %v; delegated id = %q", err, deletedID)
	}

	if _, err := service.ListItems(ctx, ListItemsOptions{GroupID: " group-1 ", Cursor: " next ", Query: " apple ", Market: " cnsh ", Limit: MaxPageLimit + 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListItems(ctx, ListItemsOptions{Market: " cn ", Limit: 0}); err != nil {
		t.Fatal(err)
	}
	if len(listedOptions) != 2 || listedOptions[0] != (ListItemsOptions{GroupID: "group-1", Cursor: "next", Query: "apple", Market: "SH", Limit: MaxPageLimit}) || listedOptions[1].Market != "CN" || listedOptions[1].Limit != DefaultPageLimit {
		t.Fatalf("normalized ListItems options = %#v", listedOptions)
	}
	if _, err := service.ListItems(ctx, ListItemsOptions{Market: "mars"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("ListItems(unsupported market) = %v", err)
	}

	memberships, err := service.GetMemberships(ctx, " us:aapl ")
	if err != nil || membershipID != "US.AAPL" || memberships.InstrumentID != "US.AAPL" {
		t.Fatalf("GetMemberships() = %#v, %v; delegated id = %q", memberships, err, membershipID)
	}
	replaced, err := service.ReplaceMemberships(ctx, ReplaceMembershipsInput{
		InstrumentID: "us:msft", GroupIDs: []string{" group-1 ", "", "group-1", "group-2"},
		NewGroupNames: []string{" Growth ", "growth", " Income "}, ExpectedRevision: 3,
	})
	if err != nil || replaced.Revision != 4 || replacement.InstrumentID != "US.MSFT" || !slices.Equal(replacement.GroupIDs, []string{"group-1", "group-2"}) || !slices.Equal(replacement.NewGroupNames, []string{"Growth", "Income"}) {
		t.Fatalf("ReplaceMemberships() = %#v, %v; delegated = %#v", replaced, err, replacement)
	}
	bindings, err := service.ListBindings(ctx, " futu:default ")
	if err != nil || len(bindings) != 1 || listedBindingID != "futu:default" {
		t.Fatalf("ListBindings() = %#v, %v; source = %q", bindings, err, listedBindingID)
	}
	if err := service.DeleteBinding(ctx, " binding-1 "); err != nil || deletedBindingID != "binding-1" {
		t.Fatalf("DeleteBinding() = %v; id = %q", err, deletedBindingID)
	}
	runs, err := service.ListImportRuns(ctx, " futu:default ", " cursor ", -1)
	if err != nil || len(runs.Items) != 1 || listedRunSourceID != "futu:default" || listedRunCursor != "cursor" || listedRunLimit != DefaultPageLimit {
		t.Fatalf("ListImportRuns() = %#v, %v; delegated = %q/%q/%d", runs, err, listedRunSourceID, listedRunCursor, listedRunLimit)
	}
}

func TestServiceRejectsInvalidDomainInputsBeforeRepositoryWrites(t *testing.T) {
	service := NewService(&serviceTestRepository{})
	tests := []struct {
		name string
		call func() error
	}{
		{name: "empty group name", call: func() error { _, err := service.CreateGroup(t.Context(), CreateGroupInput{Name: "  "}); return err }},
		{name: "long group name", call: func() error {
			_, err := service.CreateGroup(t.Context(), CreateGroupInput{Name: strings.Repeat("界", 65)})
			return err
		}},
		{name: "missing update id", call: func() error {
			_, err := service.UpdateGroup(t.Context(), " ", UpdateGroupInput{Name: "Name", ExpectedRevision: 1})
			return err
		}},
		{name: "missing update revision", call: func() error {
			_, err := service.UpdateGroup(t.Context(), "group", UpdateGroupInput{Name: "Name"})
			return err
		}},
		{name: "invalid update name", call: func() error {
			_, err := service.UpdateGroup(t.Context(), "group", UpdateGroupInput{Name: " ", ExpectedRevision: 1})
			return err
		}},
		{name: "missing delete id", call: func() error { return service.DeleteGroup(t.Context(), " ") }},
		{name: "invalid membership instrument", call: func() error { _, err := service.GetMemberships(t.Context(), "AAPL"); return err }},
		{name: "invalid replacement instrument", call: func() error {
			_, err := service.ReplaceMemberships(t.Context(), ReplaceMembershipsInput{InstrumentID: "AAPL"})
			return err
		}},
		{name: "empty new group name", call: func() error {
			_, err := service.ReplaceMemberships(t.Context(), ReplaceMembershipsInput{InstrumentID: "US.AAPL", NewGroupNames: []string{" "}})
			return err
		}},
		{name: "negative membership revision", call: func() error {
			_, err := service.ReplaceMemberships(t.Context(), ReplaceMembershipsInput{InstrumentID: "US.AAPL", ExpectedRevision: -1})
			return err
		}},
		{name: "missing binding id", call: func() error { return service.DeleteBinding(t.Context(), " ") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
		})
	}
	if !IsConflict(ErrConflict) || !IsConflict(ErrStalePreview) || !IsConflict(ErrPreviewExpired) || IsConflict(ErrValidation) {
		t.Fatal("IsConflict classification is inconsistent with retryable watchlist conflicts")
	}
}

func TestUnavailableServiceGuardsEveryRepositoryOperation(t *testing.T) {
	service := NewService(nil)
	assertUnavailable := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("%s error = %v, want ErrUnavailable", name, err)
		}
	}
	_, err := service.ListGroups(t.Context())
	assertUnavailable("ListGroups", err)
	_, err = service.CreateGroup(t.Context(), CreateGroupInput{Name: "Group"})
	assertUnavailable("CreateGroup", err)
	_, err = service.UpdateGroup(t.Context(), "group", UpdateGroupInput{Name: "Group", ExpectedRevision: 1})
	assertUnavailable("UpdateGroup", err)
	assertUnavailable("DeleteGroup", service.DeleteGroup(t.Context(), "group"))
	_, err = service.ListItems(t.Context(), ListItemsOptions{})
	assertUnavailable("ListItems", err)
	_, err = service.GetMemberships(t.Context(), "US.AAPL")
	assertUnavailable("GetMemberships", err)
	_, err = service.ReplaceMemberships(t.Context(), ReplaceMembershipsInput{InstrumentID: "US.AAPL"})
	assertUnavailable("ReplaceMemberships", err)
	_, err = service.ListSources(t.Context())
	assertUnavailable("ListSources", err)
	_, err = service.ListSourceGroups(t.Context(), "source")
	assertUnavailable("ListSourceGroups", err)
	_, err = service.ListBindings(t.Context(), "source")
	assertUnavailable("ListBindings", err)
	assertUnavailable("DeleteBinding", service.DeleteBinding(t.Context(), "binding"))
	_, err = service.PreviewImport(t.Context(), ImportPreviewRequest{})
	assertUnavailable("PreviewImport", err)
	_, err = service.CommitImport(t.Context(), CommitImportInput{})
	assertUnavailable("CommitImport", err)
	_, err = service.ListImportRuns(t.Context(), "source", "", 10)
	assertUnavailable("ListImportRuns", err)
	_, err = service.BatchQuotes(t.Context(), []string{"US.AAPL"})
	assertUnavailable("BatchQuotes", err)
	var nilService *Service
	_, err = nilService.BatchQuotes(t.Context(), []string{"US.AAPL"})
	assertUnavailable("nil BatchQuotes", err)
	nilService.RegisterSourceReader("source", &serviceTestReader{})
	nilService.RegisterBatchSnapshotSource(nil)
	service.RegisterSourceReader("", &serviceTestReader{})
	service.RegisterSourceReader("source", nil)
}

func TestServiceSynchronizesSourceHealthAndRemoteGroups(t *testing.T) {
	now := time.Date(2026, time.July, 11, 9, 30, 0, 0, time.UTC)
	ready := &serviceTestReader{
		source: Source{ID: "connector-id-is-not-authoritative", DisplayName: "OpenD"},
		groups: []RemoteGroup{
			{RemoteGroupID: "one", Name: " Core "},
			{RemoteGroupID: "two", Name: "core", ObservedAt: now.Add(-time.Hour)},
		},
	}
	unavailable := &serviceTestReader{sourceErr: errors.New("connector offline")}
	upserted := make(map[string]Source)
	var storedGroups []RemoteGroup
	repository := &serviceTestRepository{
		upsertSource: func(_ context.Context, source Source) error {
			upserted[source.ID] = source
			return nil
		},
		listSources: func(context.Context) ([]Source, error) {
			return []Source{upserted["futu:default"], upserted["legacy:default"]}, nil
		},
		replaceRemoteGroups: func(_ context.Context, sourceID string, groups []RemoteGroup) error {
			if sourceID != "futu:default" {
				t.Fatalf("source id = %q", sourceID)
			}
			storedGroups = slices.Clone(groups)
			return nil
		},
	}
	service := NewService(repository,
		WithClock(func() time.Time { return now }),
		WithSourceReader(" futu:default ", ready),
		WithSourceReader("legacy:default", unavailable),
	)
	sources, err := service.ListSources(t.Context())
	if err != nil || len(sources) != 2 {
		t.Fatalf("ListSources() = %#v, %v", sources, err)
	}
	if source := upserted["futu:default"]; source.ID != "futu:default" || source.Status != "ready" || !source.UpdatedAt.Equal(now) {
		t.Fatalf("ready source = %#v", source)
	}
	if source := upserted["legacy:default"]; source.Status != "unavailable" || source.Error != "connector offline" || !source.UpdatedAt.Equal(now) {
		t.Fatalf("unavailable source = %#v", source)
	}
	groups, err := service.ListSourceGroups(t.Context(), " futu:default ")
	if err != nil || len(groups) != 2 || len(storedGroups) != 2 {
		t.Fatalf("ListSourceGroups() = %#v, %v; stored = %#v", groups, err, storedGroups)
	}
	if groups[0].Name != "Core" || !groups[0].Ambiguous || !groups[0].ObservedAt.Equal(now) || !groups[1].Ambiguous || !groups[1].ObservedAt.Equal(now.Add(-time.Hour)) {
		t.Fatalf("normalized remote groups = %#v", groups)
	}

	ready.groups = nil
	groups, err = service.ListSourceGroups(t.Context(), "futu:default")
	if err != nil || groups == nil || len(groups) != 0 {
		t.Fatalf("nil connector groups were not normalized to an empty list: %#v, %v", groups, err)
	}
	if _, err := service.ListSourceGroups(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing source error = %v", err)
	}

	ready.groupsErr = errors.New("list failed")
	if _, err := service.ListSourceGroups(t.Context(), "futu:default"); err == nil || err.Error() != "list failed" {
		t.Fatalf("connector list error = %v", err)
	}
	ready.groupsErr = nil
	repository.replaceRemoteGroups = func(context.Context, string, []RemoteGroup) error { return errors.New("persist failed") }
	if _, err := service.ListSourceGroups(t.Context(), "futu:default"); err == nil || err.Error() != "persist failed" {
		t.Fatalf("remote group persistence error = %v", err)
	}

	upsertFailure := &serviceTestRepository{upsertSource: func(context.Context, Source) error { return errors.New("upsert failed") }}
	failingService := NewService(upsertFailure, WithSourceReader("futu:default", ready))
	if _, err := failingService.ListSources(t.Context()); err == nil || err.Error() != "upsert failed" {
		t.Fatalf("source persistence error = %v", err)
	}
}

func TestServiceBuildsImportPreviewFromFreshConnectorState(t *testing.T) {
	now := time.Date(2026, time.July, 11, 10, 0, 0, 0, time.UTC)
	reader := &serviceTestReader{
		groups: []RemoteGroup{{RemoteGroupID: "remote-1", Name: " Remote Core "}},
		members: []RemoteMember{
			{InstrumentID: "us:msft", Name: " Microsoft ", Type: " Equity "},
			{InstrumentID: "US.AAPL", Name: " Apple "},
			{InstrumentID: "US.MSFT", Name: "Microsoft"},
		},
	}
	var saved ImportPreview
	repository := &serviceTestRepository{
		replaceRemoteGroups: func(context.Context, string, []RemoteGroup) error { return nil },
		getGroup: func(_ context.Context, id string) (Group, error) {
			if id != "local-1" {
				t.Fatalf("local group id = %q", id)
			}
			return Group{ID: id, Revision: 7}, nil
		},
		groupInstrumentIDs: func(_ context.Context, id string) ([]string, error) {
			return []string{"US.AAPL", "US.TSLA"}, nil
		},
		saveImportPreview: func(_ context.Context, preview ImportPreview) error {
			saved = preview
			return nil
		},
	}
	service := NewService(repository,
		WithClock(func() time.Time { return now }),
		WithPreviewTTL(2*time.Minute),
		WithSourceReader("futu:default", reader),
	)
	preview, err := service.PreviewImport(t.Context(), ImportPreviewRequest{
		SourceID: " futu:default ", RemoteGroupID: " remote-1 ", LocalGroupID: " local-1 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(preview.ID, "wlpreview_") || saved.ID != preview.ID || saved.RemoteHash != preview.RemoteHash || preview.RemoteGroupName != "Remote Core" || preview.LocalGroupRevision != 7 || !preview.CreatedAt.Equal(now) || !preview.ExpiresAt.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("preview metadata = %#v; saved = %#v", preview, saved)
	}
	if len(preview.Added) != 1 || preview.Added[0].InstrumentID != "US.MSFT" || preview.Added[0].Name != "Microsoft" || len(preview.Unchanged) != 1 || preview.Unchanged[0].InstrumentID != "US.AAPL" || len(preview.LocalOnly) != 1 || preview.LocalOnly[0].InstrumentID != "US.TSLA" {
		t.Fatalf("preview diff = %#v", preview)
	}
	if !slices.Equal(reader.requestedIDs, []string{"remote-1"}) {
		t.Fatalf("connector group member requests = %#v", reader.requestedIDs)
	}

	preview, err = service.PreviewImport(t.Context(), ImportPreviewRequest{SourceID: "futu:default", RemoteGroupID: "remote-1"})
	if err != nil || preview.NewGroupName != "Remote Core" || preview.LocalGroupID != "" {
		t.Fatalf("preview with implicit local group = %#v, %v", preview, err)
	}
}

func TestServicePreviewImportRejectsUnsafeOrUnusableSnapshots(t *testing.T) {
	sentinel := errors.New("sentinel")
	newFixture := func() (*Service, *serviceTestRepository, *serviceTestReader) {
		reader := &serviceTestReader{
			groups:  []RemoteGroup{{RemoteGroupID: "remote-1", Name: "Remote"}},
			members: []RemoteMember{{InstrumentID: "US.AAPL"}},
		}
		repository := &serviceTestRepository{
			replaceRemoteGroups: func(context.Context, string, []RemoteGroup) error { return nil },
			getGroup:            func(context.Context, string) (Group, error) { return Group{Revision: 1}, nil },
			groupInstrumentIDs:  func(context.Context, string) ([]string, error) { return nil, nil },
			saveImportPreview:   func(context.Context, ImportPreview) error { return nil },
		}
		return NewService(repository, WithSourceReader("source", reader)), repository, reader
	}
	tests := []struct {
		name string
		run  func(*Service, *serviceTestRepository, *serviceTestReader) error
		want error
	}{
		{name: "missing source id", run: func(service *Service, _ *serviceTestRepository, _ *serviceTestReader) error {
			_, err := service.PreviewImport(t.Context(), ImportPreviewRequest{RemoteGroupID: "remote-1"})
			return err
		}, want: ErrValidation},
		{name: "two local targets", run: func(service *Service, _ *serviceTestRepository, _ *serviceTestReader) error {
			_, err := service.PreviewImport(t.Context(), ImportPreviewRequest{SourceID: "source", RemoteGroupID: "remote-1", LocalGroupID: "local", NewGroupName: "New"})
			return err
		}, want: ErrValidation},
		{name: "unknown connector", run: func(service *Service, _ *serviceTestRepository, _ *serviceTestReader) error {
			_, err := service.PreviewImport(t.Context(), ImportPreviewRequest{SourceID: "missing", RemoteGroupID: "remote-1"})
			return err
		}, want: ErrNotFound},
		{name: "missing remote group", run: func(service *Service, _ *serviceTestRepository, reader *serviceTestReader) error {
			reader.groups = nil
			_, err := service.PreviewImport(t.Context(), ImportPreviewRequest{SourceID: "source", RemoteGroupID: "remote-1"})
			return err
		}, want: ErrNotFound},
		{name: "ambiguous remote group", run: func(service *Service, _ *serviceTestRepository, reader *serviceTestReader) error {
			reader.groups[0].Ambiguous = true
			_, err := service.PreviewImport(t.Context(), ImportPreviewRequest{SourceID: "source", RemoteGroupID: "remote-1"})
			return err
		}, want: ErrAmbiguousRemoteGroup},
		{name: "remote group read failure", run: func(service *Service, _ *serviceTestRepository, reader *serviceTestReader) error {
			reader.groupsErr = sentinel
			_, err := service.PreviewImport(t.Context(), ImportPreviewRequest{SourceID: "source", RemoteGroupID: "remote-1"})
			return err
		}, want: sentinel},
		{name: "member read failure", run: func(service *Service, _ *serviceTestRepository, reader *serviceTestReader) error {
			reader.membersErr = sentinel
			_, err := service.PreviewImport(t.Context(), ImportPreviewRequest{SourceID: "source", RemoteGroupID: "remote-1"})
			return err
		}, want: sentinel},
		{name: "invalid remote instrument", run: func(service *Service, _ *serviceTestRepository, reader *serviceTestReader) error {
			reader.members = []RemoteMember{{InstrumentID: "AAPL"}}
			_, err := service.PreviewImport(t.Context(), ImportPreviewRequest{SourceID: "source", RemoteGroupID: "remote-1"})
			return err
		}, want: ErrValidation},
		{name: "missing local group", run: func(service *Service, repository *serviceTestRepository, _ *serviceTestReader) error {
			repository.getGroup = func(context.Context, string) (Group, error) { return Group{}, sentinel }
			_, err := service.PreviewImport(t.Context(), ImportPreviewRequest{SourceID: "source", RemoteGroupID: "remote-1", LocalGroupID: "local"})
			return err
		}, want: sentinel},
		{name: "local member read failure", run: func(service *Service, repository *serviceTestRepository, _ *serviceTestReader) error {
			repository.groupInstrumentIDs = func(context.Context, string) ([]string, error) { return nil, sentinel }
			_, err := service.PreviewImport(t.Context(), ImportPreviewRequest{SourceID: "source", RemoteGroupID: "remote-1", LocalGroupID: "local"})
			return err
		}, want: sentinel},
		{name: "preview persistence failure", run: func(service *Service, repository *serviceTestRepository, _ *serviceTestReader) error {
			repository.saveImportPreview = func(context.Context, ImportPreview) error { return sentinel }
			_, err := service.PreviewImport(t.Context(), ImportPreviewRequest{SourceID: "source", RemoteGroupID: "remote-1"})
			return err
		}, want: sentinel},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, repository, reader := newFixture()
			if err := test.run(service, repository, reader); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestServiceCommitImportRevalidatesAndConstrainsDeletes(t *testing.T) {
	now := time.Date(2026, time.July, 11, 11, 0, 0, 0, time.UTC)
	remoteMembers := []RemoteMember{{InstrumentID: "US.AAPL", Name: "Apple"}}
	preview := ImportPreview{
		ID: "preview-1", SourceID: "source", RemoteGroupID: "remote-1", LocalGroupID: "local-1",
		LocalGroupRevision: 4, RemoteHash: hashMembers(remoteMembers),
		LocalOnly: []ImportDiffItem{{InstrumentID: "US.TSLA"}}, ExpiresAt: now.Add(time.Minute),
	}
	reader := &serviceTestReader{freshMembers: remoteMembers}
	var committed CommitImportStoreInput
	repository := &serviceTestRepository{
		getImportPreview: func(_ context.Context, id string) (ImportPreview, error) {
			if id != preview.ID {
				t.Fatalf("preview id = %q", id)
			}
			return preview, nil
		},
		getGroup: func(context.Context, string) (Group, error) { return Group{ID: "local-1", Revision: 4}, nil },
		commitImport: func(_ context.Context, input CommitImportStoreInput) (ImportRun, error) {
			committed = input
			return ImportRun{ID: "run-1", PreviewID: input.Preview.ID}, nil
		},
	}
	service := NewService(repository, WithClock(func() time.Time { return now }), WithSourceReader("source", reader))
	run, err := service.CommitImport(t.Context(), CommitImportInput{PreviewID: " preview-1 ", DeleteInstrumentIDs: []string{"us:tsla", "US.TSLA"}})
	if err != nil || run.ID != "run-1" || committed.Preview.ID != "preview-1" || !slices.Equal(committed.RemoteMembers, remoteMembers) || !slices.Equal(committed.DeleteInstrumentIDs, []string{"US.TSLA"}) {
		t.Fatalf("CommitImport() = %#v, %v; store input = %#v", run, err, committed)
	}
	if !slices.Equal(reader.freshGroupIDs, []string{"remote-1"}) {
		t.Fatalf("fresh connector reads = %#v", reader.freshGroupIDs)
	}
}

func TestServiceCommitImportRejectsExpiredStaleAndInvalidRequests(t *testing.T) {
	baseNow := time.Date(2026, time.July, 11, 12, 0, 0, 0, time.UTC)
	sentinel := errors.New("sentinel")
	type fixture struct {
		service    *Service
		repository *serviceTestRepository
		reader     *serviceTestReader
		preview    *ImportPreview
		now        *func() time.Time
	}
	newFixture := func() fixture {
		members := []RemoteMember{{InstrumentID: "US.AAPL"}}
		preview := ImportPreview{
			ID: "preview", SourceID: "source", RemoteGroupID: "remote", LocalGroupID: "local",
			LocalGroupRevision: 2, RemoteHash: hashMembers(members), ExpiresAt: baseNow.Add(time.Minute),
			LocalOnly: []ImportDiffItem{{InstrumentID: "US.TSLA"}},
		}
		reader := &serviceTestReader{freshMembers: members}
		nowFunc := func() time.Time { return baseNow }
		repository := &serviceTestRepository{
			getImportPreview: func(context.Context, string) (ImportPreview, error) { return preview, nil },
			getGroup:         func(context.Context, string) (Group, error) { return Group{Revision: 2}, nil },
			commitImport:     func(context.Context, CommitImportStoreInput) (ImportRun, error) { return ImportRun{}, nil },
		}
		return fixture{
			service:    NewService(repository, WithClock(func() time.Time { return nowFunc() }), WithSourceReader("source", reader)),
			repository: repository, reader: reader, preview: &preview, now: &nowFunc,
		}
	}
	tests := []struct {
		name  string
		input CommitImportInput
		setup func(fixture) *Service
		want  error
	}{
		{name: "missing preview id", input: CommitImportInput{}, want: ErrValidation},
		{name: "preview lookup failure", input: CommitImportInput{PreviewID: "preview"}, setup: func(f fixture) *Service {
			f.repository.getImportPreview = func(context.Context, string) (ImportPreview, error) { return ImportPreview{}, sentinel }
			return f.service
		}, want: sentinel},
		{name: "expired preview", input: CommitImportInput{PreviewID: "preview"}, setup: func(f fixture) *Service {
			*f.now = func() time.Time { return baseNow.Add(time.Minute) }
			return f.service
		}, want: ErrPreviewExpired},
		{name: "local group lookup failure", input: CommitImportInput{PreviewID: "preview"}, setup: func(f fixture) *Service {
			f.repository.getGroup = func(context.Context, string) (Group, error) { return Group{}, sentinel }
			return f.service
		}, want: ErrStalePreview},
		{name: "local revision changed", input: CommitImportInput{PreviewID: "preview"}, setup: func(f fixture) *Service {
			f.repository.getGroup = func(context.Context, string) (Group, error) { return Group{Revision: 3}, nil }
			return f.service
		}, want: ErrStalePreview},
		{name: "connector unregistered", input: CommitImportInput{PreviewID: "preview"}, setup: func(f fixture) *Service {
			return NewService(f.repository, WithClock(func() time.Time { return baseNow }))
		}, want: ErrNotFound},
		{name: "connector lacks fresh reads", input: CommitImportInput{PreviewID: "preview"}, setup: func(f fixture) *Service {
			return NewService(f.repository, WithClock(func() time.Time { return baseNow }), WithSourceReader("source", serviceTestNonFreshReader{reader: f.reader}))
		}, want: ErrStalePreview},
		{name: "fresh read failure", input: CommitImportInput{PreviewID: "preview"}, setup: func(f fixture) *Service {
			f.reader.freshErr = sentinel
			return f.service
		}, want: ErrStalePreview},
		{name: "fresh read has invalid instrument", input: CommitImportInput{PreviewID: "preview"}, setup: func(f fixture) *Service {
			f.reader.freshMembers = []RemoteMember{{InstrumentID: "AAPL"}}
			return f.service
		}, want: ErrValidation},
		{name: "remote membership changed", input: CommitImportInput{PreviewID: "preview"}, setup: func(f fixture) *Service {
			f.reader.freshMembers = append(f.reader.freshMembers, RemoteMember{InstrumentID: "US.MSFT"})
			return f.service
		}, want: ErrStalePreview},
		{name: "preview expires during revalidation", input: CommitImportInput{PreviewID: "preview"}, setup: func(f fixture) *Service {
			calls := 0
			*f.now = func() time.Time {
				calls++
				if calls == 1 {
					return baseNow
				}
				return baseNow.Add(time.Minute)
			}
			return f.service
		}, want: ErrPreviewExpired},
		{name: "invalid requested deletion", input: CommitImportInput{PreviewID: "preview", DeleteInstrumentIDs: []string{"TSLA"}}, want: ErrValidation},
		{name: "deletion not in preview", input: CommitImportInput{PreviewID: "preview", DeleteInstrumentIDs: []string{"US.MSFT"}}, want: ErrValidation},
		{name: "store commit failure", input: CommitImportInput{PreviewID: "preview"}, setup: func(f fixture) *Service {
			f.repository.commitImport = func(context.Context, CommitImportStoreInput) (ImportRun, error) { return ImportRun{}, sentinel }
			return f.service
		}, want: sentinel},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixture()
			service := fixture.service
			if test.setup != nil {
				service = test.setup(fixture)
			}
			if _, err := service.CommitImport(t.Context(), test.input); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
