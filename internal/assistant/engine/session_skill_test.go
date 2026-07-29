package adk

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"strings"
	"testing"
	"time"

	adksession "google.golang.org/adk/v2/session"
)

func TestWrappedAndEmptySessionAccessorsPreserveProjectedState(t *testing.T) {
	base := &emptySession{
		id:             "session-1",
		appName:        "jftrade",
		userID:         "user-1",
		state:          &emptyState{values: map[string]any{"symbol": "US.AAPL"}},
		events:         &wrappedEvents{items: []*adksession.Event{{ID: "base"}}},
		lastUpdateTime: time.Date(2026, time.June, 20, 12, 0, 0, 0, time.UTC),
	}
	projected := &wrappedEvents{items: []*adksession.Event{{ID: "event-1"}, {ID: "event-2"}}}
	session := &wrappedSession{base: base, events: projected}

	if session.ID() != "session-1" || session.AppName() != "jftrade" || session.UserID() != "user-1" {
		t.Fatalf("wrapped session identity = %s/%s/%s", session.ID(), session.AppName(), session.UserID())
	}
	if !session.LastUpdateTime().Equal(base.lastUpdateTime) {
		t.Fatalf("LastUpdateTime = %s", session.LastUpdateTime())
	}
	value, err := session.State().Get("symbol")
	if err != nil || value != "US.AAPL" {
		t.Fatalf("State.Get(symbol) = %#v err=%v", value, err)
	}
	if _, err := session.State().Get("missing"); !errors.Is(err, adksession.ErrStateKeyNotExist) {
		t.Fatalf("State.Get(missing) err = %v", err)
	}
	if err := session.State().Set("timeframe", "1d"); err != nil {
		t.Fatalf("State.Set: %v", err)
	}

	allState := maps.Collect(session.State().All())
	if allState["symbol"] != "US.AAPL" || allState["timeframe"] != "1d" {
		t.Fatalf("State.All = %#v", allState)
	}

	events := session.Events()
	if events.Len() != 2 || events.At(-1) != nil || events.At(2) != nil || events.At(0).ID != "event-1" {
		t.Fatalf("projected events len/at = %d %#v %#v", events.Len(), events.At(-1), events.At(0))
	}
	projected.Append(&adksession.Event{ID: "event-2"})
	projected.Append(&adksession.Event{ID: "event-3"})
	projected.Append(nil)
	if events.Len() != 3 {
		t.Fatalf("projected events len after dedupe append = %d, want 3", events.Len())
	}
	ids := []string{}
	for event := range events.All() {
		ids = append(ids, event.ID)
	}
	if strings.Join(ids, ",") != "event-1,event-2,event-3" {
		t.Fatalf("projected event ids = %#v", ids)
	}
}

func TestAppendByModeRoutesVisibleAndReasoningText(t *testing.T) {
	var reply strings.Builder
	var reasoning strings.Builder

	appendByMode(&reply, &reasoning, reasoningModeReply, "visible")
	appendByMode(&reply, &reasoning, reasoningModeReasoning, "why")
	appendRuneByMode(&reply, &reasoning, reasoningModeReply, '!')
	appendRuneByMode(&reply, &reasoning, reasoningModeReasoning, '?')

	if reply.String() != "visible!" || reasoning.String() != "why?" {
		t.Fatalf("reply/reasoning = %q/%q", reply.String(), reasoning.String())
	}
}

func TestStoreListSkillsSortsBuiltinFirstAndDeleteProtectsBuiltins(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "adk.db"), filepath.Join(dir, "secrets", "adk.json"), filepath.Join(dir, "skills"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, skill := range []Skill{
		{ID: "external-z", DisplayName: "Zulu", Source: "filesystem", Enabled: true},
		{ID: "builtin-b", DisplayName: "Beta Builtin", Source: "builtin", Builtin: true, Enabled: true},
		{ID: "external-a", DisplayName: "Alpha External", Source: "filesystem", Enabled: true},
		{ID: "builtin-a", DisplayName: "Alpha Builtin", Source: "builtin", Builtin: true, Enabled: true},
	} {
		if _, err := store.SaveSkill(ctx, skill); err != nil {
			t.Fatalf("SaveSkill(%s): %v", skill.ID, err)
		}
	}

	skills, err := store.ListSkills(ctx)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	ids := make([]string, 0, len(skills))
	customIDs := make([]string, 0, 4)
	seenExternal := false
	for _, skill := range skills {
		ids = append(ids, skill.ID)
		if !skill.Builtin {
			seenExternal = true
		}
		if seenExternal && skill.Builtin {
			t.Fatalf("builtin skill %s appeared after external skills in %#v", skill.ID, ids)
		}
		switch skill.ID {
		case "builtin-a", "builtin-b", "external-a", "external-z":
			customIDs = append(customIDs, skill.ID)
		}
	}
	if strings.Join(customIDs, ",") != "builtin-a,builtin-b,external-a,external-z" {
		t.Fatalf("custom ListSkills order = %#v within full list %#v", customIDs, ids)
	}

	if err := store.DeleteSkill(ctx, "external-a"); err != nil {
		t.Fatalf("DeleteSkill external: %v", err)
	}
	if _, ok, err := store.Skill(ctx, "external-a"); err != nil || ok {
		t.Fatalf("external skill after delete ok=%v err=%v, want absent", ok, err)
	}

	if err := store.DeleteSkill(ctx, "builtin-a"); err != nil {
		t.Fatalf("DeleteSkill builtin: %v", err)
	}
	if skill, ok, err := store.Skill(ctx, "builtin-a"); err != nil || !ok || !skill.Builtin {
		t.Fatalf("builtin skill after delete = %+v ok=%v err=%v, want preserved", skill, ok, err)
	}

	if _, err := store.SaveSkill(ctx, Skill{DisplayName: "Skill With Spaces", Source: "filesystem", Enabled: true}); err != nil {
		t.Fatalf("SaveSkill generated ID: %v", err)
	}
	generated, err := store.ListSkills(ctx)
	if err != nil {
		t.Fatalf("ListSkills after generated ID: %v", err)
	}
	foundGenerated := false
	for _, skill := range generated {
		if skill.DisplayName == "Skill With Spaces" {
			foundGenerated = skill.ID == "skill-with-spaces" && skill.CreatedAt != "" && skill.UpdatedAt != ""
		}
	}
	if !foundGenerated {
		t.Fatalf("generated skill missing or unnormalized: %#v", generated)
	}
}
