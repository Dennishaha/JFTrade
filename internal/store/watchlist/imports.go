package watchlist

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
	"github.com/jmoiron/sqlx"
)

type sourceRow struct {
	ID          string `db:"source_id"`
	Broker      string `db:"broker"`
	DisplayName string `db:"display_name"`
	Status      string `db:"status"`
	Error       string `db:"last_error"`
	UpdatedAt   string `db:"updated_at"`
}

func (s *Store) UpsertSource(ctx context.Context, source domain.Source) error {
	if err := s.available(); err != nil {
		return err
	}
	updatedAt := source.UpdatedAt.UTC().Format(timeFormat)
	if source.UpdatedAt.IsZero() {
		updatedAt = nowText()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO watchlist_sources
		(source_id, broker, display_name, status, last_error, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET broker = excluded.broker, display_name = excluded.display_name,
			status = excluded.status, last_error = excluded.last_error, updated_at = excluded.updated_at`,
		source.ID, source.Broker, source.DisplayName, source.Status, source.Error, updatedAt)
	return err
}

func (s *Store) ListSources(ctx context.Context) ([]domain.Source, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	rows := []sourceRow{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT source_id, broker, display_name, status, last_error, updated_at FROM watchlist_sources ORDER BY source_id`); err != nil {
		return nil, err
	}
	result := make([]domain.Source, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.Source{ID: row.ID, Broker: row.Broker, DisplayName: row.DisplayName, Status: row.Status, Error: row.Error, UpdatedAt: parseTime(row.UpdatedAt)})
	}
	return result, nil
}

type remoteGroupRow struct {
	SourceID      string `db:"source_id"`
	RemoteGroupID string `db:"remote_group_id"`
	Name          string `db:"name"`
	Type          string `db:"group_type"`
	Ambiguous     int    `db:"ambiguous"`
	MemberCount   int    `db:"member_count"`
	RemoteHash    string `db:"remote_hash"`
	ObservedAt    string `db:"observed_at"`
}

func (s *Store) ReplaceRemoteGroups(ctx context.Context, sourceID string, groups []domain.RemoteGroup) error {
	if err := s.available(); err != nil {
		return err
	}
	return s.db.WriteTx(ctx, nil, func(tx *sqliteconn.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist_remote_groups WHERE source_id = ?`, sourceID); err != nil {
			return err
		}
		for _, group := range groups {
			observedAt := group.ObservedAt.UTC().Format(timeFormat)
			if group.ObservedAt.IsZero() {
				observedAt = nowText()
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO watchlist_remote_groups
				(source_id, remote_group_id, name, group_type, ambiguous, member_count, remote_hash, observed_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, sourceID, group.RemoteGroupID, group.Name, group.Type, boolInt(group.Ambiguous), group.MemberCount, group.RemoteHash, observedAt); err != nil {
				return mapWriteError(err)
			}
		}
		return nil
	})
}

func (s *Store) ListRemoteGroups(ctx context.Context, sourceID string) ([]domain.RemoteGroup, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	rows := []remoteGroupRow{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT source_id, remote_group_id, name, group_type, ambiguous, member_count, remote_hash, observed_at
		FROM watchlist_remote_groups WHERE source_id = ? ORDER BY name, remote_group_id`, sourceID); err != nil {
		return nil, err
	}
	result := make([]domain.RemoteGroup, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.RemoteGroup{SourceID: row.SourceID, RemoteGroupID: row.RemoteGroupID, Name: row.Name, Type: row.Type,
			Ambiguous: row.Ambiguous != 0, MemberCount: row.MemberCount, RemoteHash: row.RemoteHash, ObservedAt: parseTime(row.ObservedAt)})
	}
	return result, nil
}

type bindingRow struct {
	ID            string `db:"binding_id"`
	SourceID      string `db:"source_id"`
	RemoteGroupID string `db:"remote_group_id"`
	RemoteName    string `db:"remote_name"`
	LocalGroupID  string `db:"local_group_id"`
	CreatedAt     string `db:"created_at"`
	UpdatedAt     string `db:"updated_at"`
}

func (s *Store) ListBindings(ctx context.Context, sourceID string) ([]domain.Binding, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	query := `SELECT binding_id, source_id, remote_group_id, remote_name, local_group_id, created_at, updated_at FROM watchlist_bindings`
	args := []any{}
	if sourceID != "" {
		query += ` WHERE source_id = ?`
		args = append(args, sourceID)
	}
	query += ` ORDER BY source_id, remote_name, binding_id`
	rows := []bindingRow{}
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make([]domain.Binding, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.binding())
	}
	return result, nil
}

func (row bindingRow) binding() domain.Binding {
	return domain.Binding{ID: row.ID, SourceID: row.SourceID, RemoteGroupID: row.RemoteGroupID, RemoteName: row.RemoteName,
		LocalGroupID: row.LocalGroupID, CreatedAt: parseTime(row.CreatedAt), UpdatedAt: parseTime(row.UpdatedAt)}
}

func (s *Store) DeleteBinding(ctx context.Context, bindingID string) error {
	if err := s.available(); err != nil {
		return err
	}
	return s.db.WriteTx(ctx, nil, func(tx *sqliteconn.Tx) error {
		var binding bindingRow
		if err := tx.GetContext(ctx, &binding, `SELECT binding_id, source_id, remote_group_id, remote_name, local_group_id, created_at, updated_at FROM watchlist_bindings WHERE binding_id = ?`, bindingID); errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		} else if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist_bindings WHERE binding_id = ?`, bindingID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM watchlist_membership_origins WHERE source_id = ? AND remote_group_id = ?`, binding.SourceID, binding.RemoteGroupID)
		return err
	})
}

type previewRow struct {
	ID                 string `db:"preview_id"`
	SourceID           string `db:"source_id"`
	RemoteGroupID      string `db:"remote_group_id"`
	RemoteGroupName    string `db:"remote_group_name"`
	LocalGroupID       string `db:"local_group_id"`
	NewGroupName       string `db:"new_group_name"`
	RemoteHash         string `db:"remote_hash"`
	LocalGroupRevision int64  `db:"local_group_revision"`
	AddedJSON          string `db:"added_json"`
	UnchangedJSON      string `db:"unchanged_json"`
	LocalOnlyJSON      string `db:"local_only_json"`
	Status             string `db:"status"`
	CreatedAt          string `db:"created_at"`
	ExpiresAt          string `db:"expires_at"`
}

func (s *Store) SaveImportPreview(ctx context.Context, preview domain.ImportPreview) error {
	if err := s.available(); err != nil {
		return err
	}
	added, err := jsonText(preview.Added)
	if err != nil {
		return err
	}
	unchanged, err := jsonText(preview.Unchanged)
	if err != nil {
		return err
	}
	localOnly, err := jsonText(preview.LocalOnly)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO watchlist_import_previews
		(preview_id, source_id, remote_group_id, remote_group_name, local_group_id, new_group_name, remote_hash,
		 local_group_revision, added_json, unchanged_json, local_only_json, status, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`, preview.ID, preview.SourceID, preview.RemoteGroupID,
		preview.RemoteGroupName, preview.LocalGroupID, preview.NewGroupName, preview.RemoteHash, preview.LocalGroupRevision,
		added, unchanged, localOnly, preview.CreatedAt.UTC().Format(timeFormat), preview.ExpiresAt.UTC().Format(timeFormat))
	return err
}

func (s *Store) GetImportPreview(ctx context.Context, previewID string) (domain.ImportPreview, error) {
	if err := s.available(); err != nil {
		return domain.ImportPreview{}, err
	}
	return getImportPreview(ctx, s.db, previewID)
}

func getImportPreview(ctx context.Context, query sqlx.QueryerContext, previewID string) (domain.ImportPreview, error) {
	var row previewRow
	err := sqlx.GetContext(ctx, query, &row, `SELECT preview_id, source_id, remote_group_id, remote_group_name, local_group_id,
		new_group_name, remote_hash, local_group_revision, added_json, unchanged_json, local_only_json, status, created_at, expires_at
		FROM watchlist_import_previews WHERE preview_id = ?`, previewID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ImportPreview{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ImportPreview{}, err
	}
	if row.Status != "pending" {
		return domain.ImportPreview{}, domain.ErrStalePreview
	}
	preview := domain.ImportPreview{ID: row.ID, SourceID: row.SourceID, RemoteGroupID: row.RemoteGroupID, RemoteGroupName: row.RemoteGroupName,
		LocalGroupID: row.LocalGroupID, NewGroupName: row.NewGroupName, RemoteHash: row.RemoteHash, LocalGroupRevision: row.LocalGroupRevision,
		CreatedAt: parseTime(row.CreatedAt), ExpiresAt: parseTime(row.ExpiresAt), Added: []domain.ImportDiffItem{}, Unchanged: []domain.ImportDiffItem{}, LocalOnly: []domain.ImportDiffItem{}}
	if err := scanJSON(row.AddedJSON, &preview.Added); err != nil {
		return domain.ImportPreview{}, err
	}
	if err := scanJSON(row.UnchangedJSON, &preview.Unchanged); err != nil {
		return domain.ImportPreview{}, err
	}
	if err := scanJSON(row.LocalOnlyJSON, &preview.LocalOnly); err != nil {
		return domain.ImportPreview{}, err
	}
	return preview, nil
}

func (s *Store) CommitImport(ctx context.Context, input domain.CommitImportStoreInput) (domain.ImportRun, error) {
	if err := s.available(); err != nil {
		return domain.ImportRun{}, err
	}
	var run domain.ImportRun
	err := s.db.WriteTx(ctx, nil, func(tx *sqliteconn.Tx) error {
		var err error
		run, err = commitImportTx(ctx, tx, input)
		return err
	})
	return run, err
}

type importCommitState struct {
	preview      domain.ImportPreview
	groupID      string
	createdGroup bool
	current      map[string]struct{}
	remote       map[string]domain.RemoteMember
	deleteSet    map[string]struct{}
	added        int
	removed      int
	unchanged    int
}

func commitImportTx(ctx context.Context, tx *sqliteconn.Tx, input domain.CommitImportStoreInput) (domain.ImportRun, error) {
	state, err := prepareImportCommit(ctx, tx, input)
	if err != nil {
		return domain.ImportRun{}, err
	}
	if err := applyImportedMemberships(ctx, tx, input.RemoteMembers, state); err != nil {
		return domain.ImportRun{}, err
	}
	if err := replaceImportProvenance(ctx, tx, input.RemoteMembers, state); err != nil {
		return domain.ImportRun{}, err
	}
	if err := upsertImportBinding(ctx, tx, state); err != nil {
		return domain.ImportRun{}, err
	}
	run, err := recordImportRun(ctx, tx, state)
	if err != nil {
		return domain.ImportRun{}, err
	}
	if err := markImportPreviewCommitted(ctx, tx, state.preview.ID); err != nil {
		return domain.ImportRun{}, err
	}
	return run, nil
}

func prepareImportCommit(ctx context.Context, tx *sqliteconn.Tx, input domain.CommitImportStoreInput) (*importCommitState, error) {
	preview, err := getImportPreview(ctx, tx, input.Preview.ID)
	if err != nil {
		return nil, err
	}
	if preview.RemoteHash != input.Preview.RemoteHash {
		return nil, domain.ErrStalePreview
	}
	groupID, created, err := resolveImportGroup(ctx, tx, preview)
	if err != nil {
		return nil, err
	}
	currentIDs := []string{}
	if err := tx.SelectContext(ctx, &currentIDs, `SELECT instrument_id FROM watchlist_memberships WHERE group_id = ?`, groupID); err != nil {
		return nil, err
	}
	state := &importCommitState{preview: preview, groupID: groupID, createdGroup: created,
		current: make(map[string]struct{}, len(currentIDs)), remote: make(map[string]domain.RemoteMember, len(input.RemoteMembers)),
		deleteSet: make(map[string]struct{}, len(input.DeleteInstrumentIDs))}
	for _, instrumentID := range currentIDs {
		state.current[instrumentID] = struct{}{}
	}
	for _, member := range input.RemoteMembers {
		state.remote[member.InstrumentID] = member
	}
	for _, instrumentID := range input.DeleteInstrumentIDs {
		state.deleteSet[instrumentID] = struct{}{}
	}
	return state, nil
}

func resolveImportGroup(ctx context.Context, tx *sqliteconn.Tx, preview domain.ImportPreview) (string, bool, error) {
	if preview.LocalGroupID != "" {
		group, err := getGroup(ctx, tx, preview.LocalGroupID)
		if err != nil || group.Revision != preview.LocalGroupRevision {
			return "", false, domain.ErrStalePreview
		}
		return preview.LocalGroupID, false, nil
	}
	groupID, err := insertGroup(ctx, tx, preview.NewGroupName)
	return groupID, true, err
}

func applyImportedMemberships(ctx context.Context, tx *sqliteconn.Tx, members []domain.RemoteMember, state *importCommitState) error {
	affected := make(map[string]struct{})
	for _, member := range members {
		if err := ensureInstrument(ctx, tx, member); err != nil {
			return err
		}
		if err := insertAlias(ctx, tx, state.preview.SourceID, "broker_code", member.BrokerCode, member.InstrumentID); err != nil {
			return err
		}
		if err := insertAlias(ctx, tx, state.preview.SourceID, "security_id", member.SecurityID, member.InstrumentID); err != nil {
			return err
		}
		if _, exists := state.current[member.InstrumentID]; exists {
			state.unchanged++
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO watchlist_memberships (group_id, instrument_id, created_at) VALUES (?, ?, ?)`, state.groupID, member.InstrumentID, nowText()); err != nil {
			return mapWriteError(err)
		}
		state.added++
		affected[member.InstrumentID] = struct{}{}
	}
	for instrumentID := range state.deleteSet {
		if _, exists := state.current[instrumentID]; !exists {
			continue
		}
		if _, exists := state.remote[instrumentID]; exists {
			return fmt.Errorf("%w: cannot delete a remote member", domain.ErrValidation)
		}
		if err := deleteImportedMembership(ctx, tx, state.groupID, instrumentID); err != nil {
			return err
		}
		state.removed++
		affected[instrumentID] = struct{}{}
	}
	return updateImportedRevisions(ctx, tx, state, affected)
}

func deleteImportedMembership(ctx context.Context, tx *sqliteconn.Tx, groupID, instrumentID string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist_memberships WHERE group_id = ? AND instrument_id = ?`, groupID, instrumentID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM watchlist_membership_origins WHERE group_id = ? AND instrument_id = ?`, groupID, instrumentID)
	return err
}

func updateImportedRevisions(ctx context.Context, tx *sqliteconn.Tx, state *importCommitState, affected map[string]struct{}) error {
	for instrumentID := range affected {
		if _, err := tx.ExecContext(ctx, `UPDATE watchlist_instruments SET membership_revision = membership_revision + 1, updated_at = ? WHERE instrument_id = ?`, nowText(), instrumentID); err != nil {
			return err
		}
	}
	if state.createdGroup || (state.added == 0 && state.removed == 0) {
		return nil
	}
	_, err := tx.ExecContext(ctx, `UPDATE watchlist_groups SET revision = revision + 1, updated_at = ? WHERE group_id = ?`, nowText(), state.groupID)
	return err
}

func replaceImportProvenance(ctx context.Context, tx *sqliteconn.Tx, members []domain.RemoteMember, state *importCommitState) error {
	preview := state.preview
	if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist_remote_memberships WHERE source_id = ? AND remote_group_id = ?`, preview.SourceID, preview.RemoteGroupID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist_membership_origins WHERE source_id = ? AND remote_group_id = ?`, preview.SourceID, preview.RemoteGroupID); err != nil {
		return err
	}
	observedAt := nowText()
	for _, member := range members {
		if _, err := tx.ExecContext(ctx, `INSERT INTO watchlist_remote_memberships
			(source_id, remote_group_id, instrument_id, remote_hash, observed_at) VALUES (?, ?, ?, ?, ?)`,
			preview.SourceID, preview.RemoteGroupID, member.InstrumentID, preview.RemoteHash, observedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO watchlist_membership_origins
			(group_id, instrument_id, source_id, remote_group_id, last_imported_at) VALUES (?, ?, ?, ?, ?)`,
			state.groupID, member.InstrumentID, preview.SourceID, preview.RemoteGroupID, observedAt); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `UPDATE watchlist_remote_groups SET remote_hash = ?, member_count = ?, observed_at = ?
		WHERE source_id = ? AND remote_group_id = ?`, preview.RemoteHash, len(members), observedAt, preview.SourceID, preview.RemoteGroupID)
	return err
}

func upsertImportBinding(ctx context.Context, tx *sqliteconn.Tx, state *importCommitState) error {
	preview := state.preview
	bindingID := "wlbind_" + uuid.NewString()
	var existingID string
	if err := tx.GetContext(ctx, &existingID, `SELECT binding_id FROM watchlist_bindings WHERE source_id = ? AND remote_group_id = ?`, preview.SourceID, preview.RemoteGroupID); err == nil {
		bindingID = existingID
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := nowText()
	_, err := tx.ExecContext(ctx, `INSERT INTO watchlist_bindings
		(binding_id, source_id, remote_group_id, remote_name, local_group_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id, remote_group_id) DO UPDATE SET remote_name = excluded.remote_name,
			local_group_id = excluded.local_group_id, updated_at = excluded.updated_at`,
		bindingID, preview.SourceID, preview.RemoteGroupID, preview.RemoteGroupName, state.groupID, now, now)
	return err
}

func recordImportRun(ctx context.Context, tx *sqliteconn.Tx, state *importCommitState) (domain.ImportRun, error) {
	now, preview := nowText(), state.preview
	run := domain.ImportRun{ID: "wlrun_" + uuid.NewString(), PreviewID: preview.ID, SourceID: preview.SourceID,
		RemoteGroupID: preview.RemoteGroupID, RemoteGroupName: preview.RemoteGroupName, LocalGroupID: state.groupID,
		Status: "completed", AddedCount: state.added, RemovedCount: state.removed, UnchangedCount: state.unchanged,
		RemoteHash: preview.RemoteHash, CreatedAt: parseTime(now), CompletedAt: parseTime(now)}
	_, err := tx.ExecContext(ctx, `INSERT INTO watchlist_import_runs
		(run_id, preview_id, source_id, remote_group_id, remote_group_name, local_group_id, status,
		 added_count, removed_count, unchanged_count, remote_hash, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, run.ID, run.PreviewID, run.SourceID, run.RemoteGroupID,
		run.RemoteGroupName, run.LocalGroupID, run.Status, run.AddedCount, run.RemovedCount, run.UnchangedCount,
		run.RemoteHash, now, now)
	return run, err
}

func markImportPreviewCommitted(ctx context.Context, tx *sqliteconn.Tx, previewID string) error {
	result, err := tx.ExecContext(ctx, `UPDATE watchlist_import_previews SET status = 'committed' WHERE preview_id = ? AND status = 'pending'`, previewID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return domain.ErrStalePreview
	}
	return nil
}

type importRunRow struct {
	ID              string `db:"run_id"`
	PreviewID       string `db:"preview_id"`
	SourceID        string `db:"source_id"`
	RemoteGroupID   string `db:"remote_group_id"`
	RemoteGroupName string `db:"remote_group_name"`
	LocalGroupID    string `db:"local_group_id"`
	Status          string `db:"status"`
	AddedCount      int    `db:"added_count"`
	RemovedCount    int    `db:"removed_count"`
	UnchangedCount  int    `db:"unchanged_count"`
	RemoteHash      string `db:"remote_hash"`
	CreatedAt       string `db:"created_at"`
	CompletedAt     string `db:"completed_at"`
}

func (s *Store) ListImportRuns(ctx context.Context, sourceID, cursor string, limit int) (domain.ImportRunPage, error) {
	if err := s.available(); err != nil {
		return domain.ImportRunPage{}, err
	}
	query := `SELECT run_id, preview_id, source_id, remote_group_id, remote_group_name, local_group_id, status,
		added_count, removed_count, unchanged_count, remote_hash, created_at, completed_at FROM watchlist_import_runs WHERE 1 = 1`
	args := []any{}
	if sourceID != "" {
		query += ` AND source_id = ?`
		args = append(args, sourceID)
	}
	if cursor != "" {
		var cursorCreatedAt string
		if err := s.db.GetContext(ctx, &cursorCreatedAt, `SELECT created_at FROM watchlist_import_runs WHERE run_id = ?`, cursor); errors.Is(err, sql.ErrNoRows) {
			return domain.ImportRunPage{}, domain.ErrNotFound
		} else if err != nil {
			return domain.ImportRunPage{}, err
		}
		query += ` AND (created_at < ? OR (created_at = ? AND run_id < ?))`
		args = append(args, cursorCreatedAt, cursorCreatedAt, cursor)
	}
	query += ` ORDER BY created_at DESC, run_id DESC LIMIT ?`
	args = append(args, limit+1)
	rows := []importRunRow{}
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return domain.ImportRunPage{}, err
	}
	page := domain.ImportRunPage{Items: []domain.ImportRun{}}
	if len(rows) > limit {
		page.NextCursor = rows[limit-1].ID
		rows = rows[:limit]
	}
	for _, row := range rows {
		page.Items = append(page.Items, domain.ImportRun{ID: row.ID, PreviewID: row.PreviewID, SourceID: row.SourceID,
			RemoteGroupID: row.RemoteGroupID, RemoteGroupName: row.RemoteGroupName, LocalGroupID: row.LocalGroupID,
			Status: row.Status, AddedCount: row.AddedCount, RemovedCount: row.RemovedCount, UnchangedCount: row.UnchangedCount,
			RemoteHash: row.RemoteHash, CreatedAt: parseTime(row.CreatedAt), CompletedAt: parseTime(row.CompletedAt)})
	}
	return page, nil
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

var _ sqlx.ExtContext = (*sqliteconn.Tx)(nil)
