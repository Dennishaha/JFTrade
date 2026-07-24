package research

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

type presetRepositoryStub struct {
	items      []ScreenPreset
	get        ScreenPreset
	err        error
	created    ScreenPreset
	updated    ScreenPreset
	deletedID  string
	createName string
	updateName string
}

func (r *presetRepositoryStub) ListScreenPresets(context.Context) ([]ScreenPreset, error) {
	return r.items, r.err
}

func (r *presetRepositoryStub) GetScreenPreset(_ context.Context, id string) (ScreenPreset, error) {
	if r.err != nil {
		return ScreenPreset{}, r.err
	}
	value := r.get
	value.ID = id
	return value, nil
}

func (r *presetRepositoryStub) CreateScreenPreset(
	_ context.Context,
	name string,
	definition broker.ScreenDefinitionV2,
	version int,
) (ScreenPreset, error) {
	r.createName = name
	r.created = ScreenPreset{Name: name, Definition: definition, QuerySchemaVersion: version}
	return r.created, r.err
}

func (r *presetRepositoryStub) UpdateScreenPreset(
	_ context.Context,
	id string,
	name string,
	definition broker.ScreenDefinitionV2,
	version int,
	revision int64,
) (ScreenPreset, error) {
	r.updateName = name
	r.updated = ScreenPreset{
		ID: id, Name: name, Definition: definition,
		QuerySchemaVersion: version, Revision: revision + 1,
	}
	return r.updated, r.err
}

func (r *presetRepositoryStub) DeleteScreenPreset(_ context.Context, id string) error {
	r.deletedID = id
	return r.err
}

func validPresetDefinition() broker.ScreenDefinitionV2 {
	return broker.ScreenDefinitionV2{
		BrokerID:           "futu",
		Market:             "US",
		CatalogVersion:     researchscreen.CatalogVersion,
		QuerySchemaVersion: broker.ScreenQuerySchemaVersionV2,
		Columns: []broker.ScreenColumn{{
			ID: "price",
			Factor: broker.FactorRef{
				InstanceID: "price",
				FactorKey:  "simple.price",
			},
		}},
	}
}

func TestServiceCreateListGetAndDeletePreset(t *testing.T) {
	repository := &presetRepositoryStub{
		items: []ScreenPreset{{ID: "one"}},
		get:   ScreenPreset{Name: "价值", Definition: validPresetDefinition(), Revision: 1},
	}
	service := NewService(repository)

	items, err := service.ListScreenPresets(t.Context())
	if err != nil || len(items) != 1 {
		t.Fatalf("ListScreenPresets = %#v, %v", items, err)
	}
	got, err := service.GetScreenPreset(t.Context(), " preset ")
	if err != nil || got.ID != "preset" {
		t.Fatalf("GetScreenPreset = %#v, %v", got, err)
	}
	created, err := service.CreateScreenPreset(t.Context(), CreateScreenPresetInput{
		Name: "  美股价值  ", Definition: validPresetDefinition(),
	})
	if err != nil || created.Name != "美股价值" || created.QuerySchemaVersion != QuerySchemaVersion {
		t.Fatalf("CreateScreenPreset = %#v, %v", created, err)
	}
	if err := service.DeleteScreenPreset(t.Context(), " preset "); err != nil || repository.deletedID != "preset" {
		t.Fatalf("DeleteScreenPreset id=%q err=%v", repository.deletedID, err)
	}
}

func TestServiceUpdatePresetMergesFieldsAndEnforcesRevision(t *testing.T) {
	current := ScreenPreset{
		Name: "旧名称", Definition: validPresetDefinition(), Revision: 3,
	}
	repository := &presetRepositoryStub{get: current}
	service := NewService(repository)
	name := "  新名称 "
	definition := validPresetDefinition()
	definition.Market = "HK"
	updated, err := service.UpdateScreenPreset(t.Context(), " preset ", UpdateScreenPresetInput{
		Name: &name, Definition: &definition, ExpectedRevision: 3,
	})
	if err != nil || updated.Name != "新名称" || updated.Definition.Market != "HK" ||
		updated.QuerySchemaVersion != QuerySchemaVersion || updated.Revision != 4 {
		t.Fatalf("UpdateScreenPreset = %#v, %v", updated, err)
	}

	onlyDefinition := validPresetDefinition()
	onlyDefinition.Market = "SH"
	repository.get = current
	updated, err = service.UpdateScreenPreset(t.Context(), "preset", UpdateScreenPresetInput{
		Definition: &onlyDefinition, ExpectedRevision: 3,
	})
	if err != nil || updated.Name != current.Name || updated.Definition.Market != "SH" {
		t.Fatalf("definition-only update = %#v, %v", updated, err)
	}

	repository.get = current
	if _, err := service.UpdateScreenPreset(t.Context(), "preset", UpdateScreenPresetInput{
		Name: &name, ExpectedRevision: 2,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update error = %v", err)
	}
}

func TestServiceRejectsUnavailableAndInvalidPresetOperations(t *testing.T) {
	var unavailable *Service
	if _, err := unavailable.ListScreenPresets(t.Context()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil list error = %v", err)
	}
	if _, err := unavailable.GetScreenPreset(t.Context(), "id"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil get error = %v", err)
	}
	if _, err := unavailable.CreateScreenPreset(t.Context(), CreateScreenPresetInput{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil create error = %v", err)
	}
	if _, err := unavailable.UpdateScreenPreset(t.Context(), "id", UpdateScreenPresetInput{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil update error = %v", err)
	}
	if err := unavailable.DeleteScreenPreset(t.Context(), "id"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil delete error = %v", err)
	}

	service := NewService(&presetRepositoryStub{})
	for _, name := range []string{"", strings.Repeat("名", 81)} {
		if _, err := service.CreateScreenPreset(t.Context(), CreateScreenPresetInput{
			Name: name, Definition: validPresetDefinition(),
		}); !errors.Is(err, ErrValidation) {
			t.Fatalf("create name %q error = %v", name, err)
		}
	}
	invalidDefinition := validPresetDefinition()
	invalidDefinition.QuerySchemaVersion = 1
	if _, err := service.CreateScreenPreset(t.Context(), CreateScreenPresetInput{
		Name: "invalid", Definition: invalidDefinition,
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("invalid definition error = %v", err)
	}
	if _, err := service.GetScreenPreset(t.Context(), " "); !errors.Is(err, ErrValidation) {
		t.Fatalf("blank get error = %v", err)
	}
	if err := service.DeleteScreenPreset(t.Context(), " "); !errors.Is(err, ErrValidation) {
		t.Fatalf("blank delete error = %v", err)
	}

	cases := []UpdateScreenPresetInput{
		{ExpectedRevision: 1},
		{ExpectedRevision: 0, Name: new("name")},
	}
	for _, input := range cases {
		if _, err := service.UpdateScreenPreset(t.Context(), "preset", input); !errors.Is(err, ErrValidation) {
			t.Fatalf("update %#v error = %v", input, err)
		}
	}
	if _, err := service.UpdateScreenPreset(t.Context(), " ", UpdateScreenPresetInput{
		Name: new("name"), ExpectedRevision: 1,
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("blank update id error = %v", err)
	}
}

func TestServicePropagatesRepositoryFailures(t *testing.T) {
	failure := errors.New("database unavailable")
	service := NewService(&presetRepositoryStub{err: failure})
	if _, err := service.ListScreenPresets(t.Context()); !errors.Is(err, failure) {
		t.Fatalf("list error = %v", err)
	}
	if _, err := service.GetScreenPreset(t.Context(), "id"); !errors.Is(err, failure) {
		t.Fatalf("get error = %v", err)
	}
	if err := service.DeleteScreenPreset(t.Context(), "id"); !errors.Is(err, failure) {
		t.Fatalf("delete error = %v", err)
	}
	if _, err := service.UpdateScreenPreset(t.Context(), "id", UpdateScreenPresetInput{
		Name: new("name"), ExpectedRevision: 1,
	}); !errors.Is(err, failure) {
		t.Fatalf("update read error = %v", err)
	}
}
