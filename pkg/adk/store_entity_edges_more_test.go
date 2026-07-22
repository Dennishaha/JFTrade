package adk

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestStoreEntityAndCoreAdditionalEdgeBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("provider and agent helpers surface deterministic lookup failures", func(t *testing.T) {
		store := newBusinessStore(t)
		if _, err := store.SetDefaultProvider(ctx, " "); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("SetDefaultProvider blank err = %v, want os.ErrNotExist", err)
		}
		if _, err := store.SetDefaultProvider(ctx, "missing-provider"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("SetDefaultProvider missing err = %v, want os.ErrNotExist", err)
		}
		if err := store.DeleteProvider(ctx, " "); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("DeleteProvider blank err = %v, want os.ErrNotExist", err)
		}

		if _, err := store.DefaultAgent(ctx); err != nil {
			t.Fatalf("DefaultAgent creates builtin when needed: %v", err)
		}
		if _, err := store.db.ExecContext(ctx, `DROP TABLE `+tableAgents); err != nil {
			t.Fatalf("drop agents: %v", err)
		}
		if _, err := store.DefaultAgent(ctx); err == nil || !strings.Contains(err.Error(), tableAgents) {
			t.Fatalf("DefaultAgent dropped agents err = %v, want %s", err, tableAgents)
		}
	})

	t.Run("session mutations reject invalid input and surface write failures", func(t *testing.T) {
		store := newBusinessStore(t)
		if _, err := store.RenameSession(ctx, "missing-session", "new"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("RenameSession missing err = %v, want os.ErrNotExist", err)
		}
		session, err := store.CreateSession(ctx, "agent", "original")
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if _, err := store.RenameSession(ctx, session.ID, " "); err == nil || !strings.Contains(err.Error(), "session title is required") {
			t.Fatalf("RenameSession blank title err = %v", err)
		}
		longTitle := strings.Repeat("长", 120)
		renamed, err := store.RenameSession(ctx, session.ID, longTitle)
		if err != nil {
			t.Fatalf("RenameSession long title: %v", err)
		}
		if got := len([]rune(renamed.Title)); got != 80 {
			t.Fatalf("renamed title rune length = %d, want 80", got)
		}

		errorStore := newBusinessStore(t)
		errorSession, err := errorStore.CreateSession(ctx, "agent", "will fail")
		if err != nil {
			t.Fatalf("CreateSession error store: %v", err)
		}
		if _, err := errorStore.db.ExecContext(ctx, `DROP TABLE `+tableSessions); err != nil {
			t.Fatalf("drop sessions: %v", err)
		}
		if _, err := errorStore.RenameSession(ctx, errorSession.ID, "new"); err == nil || !strings.Contains(err.Error(), tableSessions) {
			t.Fatalf("RenameSession dropped table err = %v, want %s", err, tableSessions)
		}
		if _, err := errorStore.CreateSession(ctx, "agent", "new"); err == nil || !strings.Contains(err.Error(), tableSessions) {
			t.Fatalf("CreateSession dropped table err = %v, want %s", err, tableSessions)
		}
	})

	t.Run("delete session surfaces each cascade table failure", func(t *testing.T) {
		for _, table := range []string{
			tableToolInvocations, tableRunLeases, tableApprovals, tableTasks, tableRuns, tableSessionContexts, tableSessionContextLive,
			tableHandoffSegments, tableSessionNotices, tableSessionComposer, tableSessions,
		} {
			store := newBusinessStore(t)
			if _, err := store.CreateSession(ctx, "agent", "delete "+table); err != nil {
				t.Fatalf("CreateSession %s: %v", table, err)
			}
			if _, err := store.db.ExecContext(ctx, `DROP TABLE `+table); err != nil {
				t.Fatalf("drop %s: %v", table, err)
			}
			if err := store.DeleteSession(ctx, "session-any"); err == nil || !strings.Contains(err.Error(), table) {
				t.Fatalf("DeleteSession after dropping %s err = %v, want table error", table, err)
			}
		}
	})

	t.Run("store internals reject malformed json and unsupported payloads", func(t *testing.T) {
		store := newBusinessStore(t)
		if err := store.saveJSON(ctx, tableAgents, "bad-json-value", nowString(), nowString(), map[string]any{"bad": make(chan int)}); err == nil {
			t.Fatal("saveJSON accepted unsupported JSON payload")
		}
		if err := currentErrOrNotFound(nil, false); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("currentErrOrNotFound missing err = %v, want os.ErrNotExist", err)
		}
		now := nowString()
		if _, err := store.db.ExecContext(ctx, `INSERT INTO `+tableSessions+` (id, agent_id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			"session-invalid-json", "agent", `{"broken":`, now, now,
		); err != nil {
			t.Fatalf("insert invalid session payload: %v", err)
		}
		var sessions []Session
		if err := store.listJSON(ctx, tableSessions, "", &sessions); err == nil {
			t.Fatal("listJSON accepted invalid raw JSON payload")
		}
		if _, err := store.listJSONPage(ctx, tableSessions, nil, nil, "", 10, 0, &sessions); err == nil {
			t.Fatal("listJSONPage accepted invalid raw JSON payload")
		}
		if got := normalizeRecentUserWindow(1); got != 2 {
			t.Fatalf("normalizeRecentUserWindow(1) = %d, want 2", got)
		}
		if err := (secretStore{path: string([]byte{0})}).write(map[string]string{"provider": "sk"}); err == nil {
			t.Fatal("secretStore.write accepted invalid path")
		}
	})

	t.Run("provider save and lookup paths surface malformed payload and write failures", func(t *testing.T) {
		listErrorStore := newBusinessStore(t)
		now := nowString()
		if _, err := listErrorStore.db.ExecContext(ctx, `INSERT INTO `+tableProviders+` (id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			"provider-malformed-list", `{"id":`, now, now,
		); err != nil {
			t.Fatalf("insert malformed provider payload: %v", err)
		}
		if _, err := listErrorStore.SaveProvider(ctx, ProviderWriteRequest{ID: "provider-after-malformed", Enabled: true}); err == nil {
			t.Fatal("SaveProvider accepted malformed provider list payload")
		}
		if _, err := listErrorStore.UpdateProviderCapabilities(ctx, "provider-malformed-list", map[string]bool{"chat": true}); err == nil {
			t.Fatal("UpdateProviderCapabilities accepted malformed provider payload")
		}
		if err := listErrorStore.DeleteProvider(ctx, "provider-malformed-list"); err == nil {
			t.Fatal("DeleteProvider accepted malformed provider payload")
		}

		saveErrorStore := newBusinessStore(t)
		installFailTrigger(t, NewRuntime(saveErrorStore, nil), "fail_provider_save_insert", tableProviders, "INSERT", "provider save failed")
		if _, err := saveErrorStore.SaveProvider(ctx, ProviderWriteRequest{ID: "provider-save-fails", Enabled: true}); err == nil || !strings.Contains(err.Error(), "provider save failed") {
			t.Fatalf("SaveProvider write err = %v, want provider save failed", err)
		}
	})

	t.Run("provider default-selection and deletion follow-up branches stay covered", func(t *testing.T) {
		t.Run("SaveProvider surfaces ensure-default failures after insert", func(t *testing.T) {
			store := newBusinessStore(t)
			if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER save_provider_corrupt_after_insert AFTER INSERT ON `+tableProviders+` WHEN NEW.id = 'provider-ensure-default-error' BEGIN UPDATE `+tableProviders+` SET payload_json = '{' WHERE id = NEW.id; END`); err != nil {
				t.Fatalf("create corrupt-after-insert trigger: %v", err)
			}
			if _, err := store.SaveProvider(ctx, ProviderWriteRequest{ID: "provider-ensure-default-error", Enabled: true}); err == nil {
				t.Fatal("SaveProvider accepted provider whose ensure-default pass should fail")
			}
		})

		t.Run("SaveProvider surfaces readback errors and fallback return after default normalization updates", func(t *testing.T) {
			errorStore := newBusinessStore(t)
			if _, err := errorStore.db.ExecContext(ctx, `CREATE TRIGGER save_provider_flip_default AFTER INSERT ON `+tableProviders+` WHEN NEW.id = 'provider-readback-error' BEGIN UPDATE `+tableProviders+` SET payload_json = json_set(payload_json, '$.default', json('false')) WHERE id = NEW.id; END`); err != nil {
				t.Fatalf("create flip-default trigger: %v", err)
			}
			if _, err := errorStore.db.ExecContext(ctx, `CREATE TRIGGER save_provider_corrupt_readback AFTER UPDATE ON `+tableProviders+` WHEN NEW.id = 'provider-readback-error' AND json_extract(NEW.payload_json, '$.default') = 1 BEGIN UPDATE `+tableProviders+` SET payload_json = '{' WHERE id = NEW.id; END`); err != nil {
				t.Fatalf("create corrupt-readback trigger: %v", err)
			}
			if _, err := errorStore.SaveProvider(ctx, ProviderWriteRequest{ID: "provider-readback-error", Enabled: true}); err == nil {
				t.Fatal("SaveProvider accepted corrupted provider readback state")
			}

			fallbackStore := newBusinessStore(t)
			if _, err := fallbackStore.db.ExecContext(ctx, `CREATE TRIGGER save_provider_flip_default_missing AFTER INSERT ON `+tableProviders+` WHEN NEW.id = 'provider-readback-missing' BEGIN UPDATE `+tableProviders+` SET payload_json = json_set(payload_json, '$.default', json('false')) WHERE id = NEW.id; END`); err != nil {
				t.Fatalf("create flip-default-missing trigger: %v", err)
			}
			if _, err := fallbackStore.db.ExecContext(ctx, `CREATE TRIGGER save_provider_delete_readback AFTER UPDATE ON `+tableProviders+` WHEN NEW.id = 'provider-readback-missing' AND json_extract(NEW.payload_json, '$.default') = 1 BEGIN DELETE FROM `+tableProviders+` WHERE id = NEW.id; END`); err != nil {
				t.Fatalf("create delete-readback trigger: %v", err)
			}
			saved, err := fallbackStore.SaveProvider(ctx, ProviderWriteRequest{ID: "provider-readback-missing", Enabled: true})
			if err != nil || saved.ID != "provider-readback-missing" {
				t.Fatalf("SaveProvider fallback saved=%+v err=%v, want original provider returned", saved, err)
			}
		})

		t.Run("SetDefaultProvider surfaces readback corruption and disappearance", func(t *testing.T) {
			errorStore := newBusinessStore(t)
			mustSaveProvider(t, NewRuntime(errorStore, nil), ProviderWriteRequest{ID: "provider-default-a", Enabled: true})
			target := mustSaveProvider(t, NewRuntime(errorStore, nil), ProviderWriteRequest{ID: "provider-default-b", Enabled: true})
			if _, err := errorStore.db.ExecContext(ctx, `CREATE TRIGGER set_default_corrupt_readback AFTER UPDATE ON `+tableProviders+` WHEN NEW.id = '`+target.ID+`' AND json_extract(NEW.payload_json, '$.default') = 1 BEGIN UPDATE `+tableProviders+` SET payload_json = '{' WHERE id = NEW.id; END`); err != nil {
				t.Fatalf("create set-default corrupt trigger: %v", err)
			}
			if _, err := errorStore.SetDefaultProvider(ctx, target.ID); err == nil {
				t.Fatal("SetDefaultProvider accepted corrupted selected provider payload")
			}

			missingStore := newBusinessStore(t)
			mustSaveProvider(t, NewRuntime(missingStore, nil), ProviderWriteRequest{ID: "provider-default-c", Enabled: true})
			target = mustSaveProvider(t, NewRuntime(missingStore, nil), ProviderWriteRequest{ID: "provider-default-d", Enabled: true})
			if _, err := missingStore.db.ExecContext(ctx, `CREATE TRIGGER set_default_delete_readback AFTER UPDATE ON `+tableProviders+` WHEN NEW.id = '`+target.ID+`' AND json_extract(NEW.payload_json, '$.default') = 1 BEGIN DELETE FROM `+tableProviders+` WHERE id = NEW.id; END`); err != nil {
				t.Fatalf("create set-default delete trigger: %v", err)
			}
			if _, err := missingStore.SetDefaultProvider(ctx, target.ID); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("SetDefaultProvider delete-after-update err = %v, want os.ErrNotExist", err)
			}
		})

		t.Run("DeleteProvider surfaces delete failures and default re-selection errors", func(t *testing.T) {
			deleteStore := newBusinessStore(t)
			provider := mustSaveProvider(t, NewRuntime(deleteStore, nil), ProviderWriteRequest{ID: "provider-delete-fails", Enabled: true})
			if _, err := deleteStore.db.ExecContext(ctx, `CREATE TRIGGER fail_provider_delete BEFORE DELETE ON `+tableProviders+` WHEN OLD.id = '`+provider.ID+`' BEGIN SELECT RAISE(FAIL, 'provider delete failed'); END`); err != nil {
				t.Fatalf("create provider delete trigger: %v", err)
			}
			if err := deleteStore.DeleteProvider(ctx, provider.ID); err == nil || !strings.Contains(err.Error(), "provider delete failed") {
				t.Fatalf("DeleteProvider err = %v, want provider delete failed", err)
			}

			reselectStore := newBusinessStore(t)
			defaultProvider := mustSaveProvider(t, NewRuntime(reselectStore, nil), ProviderWriteRequest{ID: "provider-default-delete", Enabled: true})
			now := nowString()
			if _, err := reselectStore.db.ExecContext(ctx, `INSERT INTO `+tableProviders+` (id, payload_json, created_at, updated_at) VALUES (?, ?, ?, ?)`,
				"provider-malformed-remaining", `{"id":`, now, now,
			); err != nil {
				t.Fatalf("insert malformed remaining provider: %v", err)
			}
			if err := reselectStore.DeleteProvider(ctx, defaultProvider.ID); err == nil {
				t.Fatal("DeleteProvider accepted malformed remaining provider during default reselection")
			}
		})
	})

	t.Run("delete session surfaces run delete failures", func(t *testing.T) {
		store := newBusinessStore(t)
		session, err := store.CreateSession(ctx, "agent", "delete runs failure")
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if err := store.SaveRun(ctx, Run{
			ID: "session-delete-run-fail", SessionID: session.ID, AgentID: "agent",
			Status: RunStatusRunning, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		}); err != nil {
			t.Fatalf("SaveRun: %v", err)
		}
		if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER fail_session_run_delete BEFORE DELETE ON `+tableRuns+` BEGIN SELECT RAISE(FAIL, 'run delete failed'); END`); err != nil {
			t.Fatalf("create session run delete trigger: %v", err)
		}
		if err := store.DeleteSession(ctx, session.ID); err == nil || !strings.Contains(err.Error(), "run delete failed") {
			t.Fatalf("DeleteSession err = %v, want run delete failed", err)
		}
	})
}
